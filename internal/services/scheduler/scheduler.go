package scheduler

import (
	"crypto/sha256"
	"fmt"
	"log"
	"mimic/internal/models"
	"mimic/internal/services/alert"
	"mimic/internal/services/sftp"
	"mimic/internal/services/ssh"
	"mimic/pkg/diff"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type SchedulerService struct {
	db   *gorm.DB
	ssh  *ssh.SSHService
	sftp *sftp.SftpService
	isRunning int32
}

func NewScheduler(db *gorm.DB) *SchedulerService {
	return &SchedulerService{
		db:   db,
		ssh:  &ssh.SSHService{DB: db},
		sftp: &sftp.SftpService{},
	}
}

func (s *SchedulerService) Start() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			s.CheckBackups()
			s.CheckExports()
		}
	}()
}

func (s *SchedulerService) CheckBackups() {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 0, 1) {
		log.Println("[Scheduler] Previous cycle is still running, skipping this tick.")
		return
	}
	defer atomic.StoreInt32(&s.isRunning, 0)

	var nodes []models.Node
	now := time.Now()
	// Find nodes that need backup
	s.db.Preload("Credential").Preload("AccessAgent").Preload("Routine").Where("enabled = ? AND (next_backup_at IS NULL OR next_backup_at <= ?)", true, now).Find(&nodes)

	if len(nodes) == 0 {
		return
	}

	log.Printf("[Scheduler] Found %d nodes pending backup. Starting worker pool...", len(nodes))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 20) // Limit concurrency to 20

	for _, node := range nodes {
		n := node // Shadow variable for goroutine safety
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore slot

		go func(target *models.Node) {
			defer wg.Done()
			defer func() { <-sem }() // Release slot
			s.RunBackup(target)
		}(&n)
	}

	wg.Wait()
	log.Println("[Scheduler] All backups in this cycle have completed.")
}

func (s *SchedulerService) RunBackup(node *models.Node) {
	log.Printf("Starting backup for node: %s", node.Name)
	config, err := s.ssh.PerformBackup(node)
	
	status := "success"
	errorMessage := ""
	configHash := ""
	if err != nil {
		status = "error"
		errorMessage = err.Error()
		log.Printf("Backup failed for node %s: %v", node.Name, err)
	} else {
		configHash = fmt.Sprintf("%x", sha256.Sum256([]byte(config)))
	}

	// Logic for versioning: only create new backup if successful and hash changed
	if status == "success" {
		var lastBackup models.NodeBackup
		s.db.Where("node_id = ? AND status = 'success'", node.ID).Order("version desc").First(&lastBackup)

		version := 1
		if lastBackup.ID != 0 {
			version = lastBackup.Version
			if lastBackup.Hash != configHash {
				version++
				
				diffRes := diff.GenerateDiff(lastBackup.Config, config)
				
				backup := models.NodeBackup{
					NodeID:        node.ID,
					Config:        config,
					Hash:          configHash,
					Status:        status,
					Error:         errorMessage,
					Version:       version,
					DiffAdditions: diffRes.Additions,
					DiffDeletions: diffRes.Deletions,
				}
				s.db.Create(&backup)

				// Disparar alerta se o diff ocorreu
				isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
				if !isSnoozed {
					var rules []models.AlertRule
					s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
					for _, rule := range rules {
						if rule.Enabled && rule.AlertOnDiff {
							msg := fmt.Sprintf("⚠️ *Configuration Drift Detected*\n\n*Node:* %s\n*IP:* %s\n*Vendor:* %s\n\n*Changes:* +%d additions, -%d deletions\n*New Version:* v%d", node.Name, node.IP, node.Vendor, diffRes.Additions, diffRes.Deletions, version)
							alert.Dispatch(s.db, rule, msg)
						}
					}
				} else {
					log.Printf("[Scheduler] Alert suppressed for node %s (Drift) due to active snooze/maintenance mode.", node.Name)
				}
			}
		} else {
			backup := models.NodeBackup{
				NodeID:  node.ID,
				Config:  config,
				Hash:    configHash,
				Status:  status,
				Error:   errorMessage,
				Version: 1,
			}
			s.db.Create(&backup)
		}

		// Alerta de Recovery: Se o último status foi erro, significa que o serviço recuperou
		if node.LastStatus == "error" {
			isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
			if !isSnoozed {
				var rules []models.AlertRule
				s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
				for _, rule := range rules {
					if rule.Enabled && rule.AlertOnFailure {
						msg := fmt.Sprintf("✅ *Backup Recovered*\n\n*Node:* %s\n*IP:* %s\n\nBackup is now succeeding again.", node.Name, node.IP)
						alert.Dispatch(s.db, rule, msg)
					}
				}
			}
		}

	} else {
		// Deduplicação (State Transition): Só alerta falha se o node não estava falhando antes
		if node.LastStatus != "error" {
			isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
			if !isSnoozed {
				var rules []models.AlertRule
				s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
				for _, rule := range rules {
					if rule.Enabled && rule.AlertOnFailure {
						msg := fmt.Sprintf("❌ *Backup Failed*\n\n*Node:* %s\n*IP:* %s\n*Error:* %s", node.Name, node.IP, errorMessage)
						alert.Dispatch(s.db, rule, msg)
					}
				}
			} else {
				log.Printf("[Scheduler] Alert suppressed for node %s (Failure) due to active snooze/maintenance mode.", node.Name)
			}
		} else {
			log.Printf("[Scheduler] Backup failed for %s, but skipping alert to prevent fatigue.", node.Name)
		}
	}

	// Update Node status
	node.LastStatus = status
	node.LastError = errorMessage
	now := time.Now()
	node.LastBackupAt = &now
	
	// Calculate next backup time based on frequency
	freqHours := 24
	if node.ScheduleType == "routine" && node.RoutineID != nil {
		var routine models.BackupRoutine
		s.db.First(&routine, node.RoutineID)
		freqHours, _ = strconv.Atoi(routine.Frequency)
	} else {
		freqHours, _ = strconv.Atoi(node.Frequency)
	}
	if freqHours == 0 { freqHours = 24 }
	
	next := now.Add(time.Duration(freqHours) * time.Hour)
	node.NextBackupAt = &next
	
	s.db.Save(node)
}

func (s *SchedulerService) CheckExports() {
	var settings models.SftpSettings
	if err := s.db.First(&settings).Error; err != nil {
		return // No settings found
	}

	if !settings.Enabled {
		return
	}

	now := time.Now()
	// SyncTime is "HH:MM". We check if current time matches.
	if settings.SyncTime != "" && now.Format("15:04") != settings.SyncTime {
		return
	}

	var backups []models.NodeBackup
	// Only fetch successful backups that haven't been exported yet
	s.db.Preload("Node").Where("status = ? AND exported = ?", "success", false).Find(&backups)

	if len(backups) == 0 {
		return
	}

	log.Printf("[Scheduler] Found %d pending SFTP exports.", len(backups))

	for _, backup := range backups {
		if err := s.sftp.Export(&backup, &settings); err == nil {
			backup.Exported = true
			s.db.Save(&backup)
			log.Printf("[Scheduler] Successfully exported backup ID %d to SFTP.", backup.ID)
		} else {
			log.Printf("[Scheduler] Failed to export backup ID %d: %v", backup.ID, err)
		}
	}

	settings.LastExportAt = &now
	settings.LastExportStatus = "success"
	s.db.Save(&settings)
}
