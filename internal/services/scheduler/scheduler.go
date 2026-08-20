package scheduler

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"mimic/internal/models"
	"mimic/internal/services/alert"
	"mimic/internal/services/audit"
	"mimic/internal/services/sftp"
	"mimic/internal/services/ssh"
	"mimic/pkg/diff"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type SchedulerService struct {
	db        *gorm.DB
	ssh       *ssh.SSHService
	sftp      *sftp.SftpService
	isRunning int32
	nodeLocks sync.Map
	stop      chan struct{}
	stopped   chan struct{}
	stopOnce  sync.Once
	jobs      sync.WaitGroup
}

func NewScheduler(db *gorm.DB) *SchedulerService {
	return &SchedulerService{
		db:      db,
		ssh:     &ssh.SSHService{DB: db},
		sftp:    &sftp.SftpService{DB: db},
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (s *SchedulerService) Start() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		defer close(s.stopped)
		for {
			select {
			case <-ticker.C:
				s.CheckBackups()
				s.CheckExports()
			case <-s.stop:
				return
			}
		}
	}()
}

func (s *SchedulerService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
		<-s.stopped
		s.jobs.Wait()
	})
}

func (s *SchedulerService) CheckBackups() {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 0, 1) {
		log.Println("[Scheduler] Previous cycle is still running, skipping this tick.")
		return
	}
	defer atomic.StoreInt32(&s.isRunning, 0)
	s.checkBackupsLocked()
}

// StartBackupCycle starts a full cycle without holding the HTTP request open.
func (s *SchedulerService) StartBackupCycle() bool {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 0, 1) {
		return false
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer atomic.StoreInt32(&s.isRunning, 0)
		s.checkBackupsLocked()
	}()
	return true
}

func (s *SchedulerService) checkBackupsLocked() {
	var nodes []models.Node
	now := time.Now()
	// Find nodes that need backup
	if err := s.db.Preload("Credential").
		Preload("AccessAgent").
		Preload("Routine").
		Joins("LEFT JOIN backup_routines ON backup_routines.id = nodes.routine_id AND backup_routines.deleted_at IS NULL").
		Where("nodes.enabled = ? AND (nodes.next_backup_at IS NULL OR nodes.next_backup_at <= ?) AND (nodes.schedule_type <> ? OR backup_routines.enabled = ?)", true, now, "routine", true).
		Find(&nodes).Error; err != nil {
		log.Printf("[Scheduler] Failed to load pending nodes: %v", err)
		s.recordLog("error", "backup", "Failed to load pending backup jobs", err.Error())
		return
	}

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
	if !s.tryLockNode(node.ID) {
		log.Printf("[Scheduler] Backup for node %s is already running; duplicate request skipped.", node.Name)
		return
	}
	defer s.unlockNode(node.ID)
	s.runBackup(node)
}

// StartBackup reserves the node before starting an asynchronous job.
func (s *SchedulerService) StartBackup(node *models.Node) bool {
	if !s.tryLockNode(node.ID) {
		return false
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer s.unlockNode(node.ID)
		s.runBackup(node)
	}()
	return true
}

func (s *SchedulerService) tryLockNode(nodeID uint) bool {
	value, _ := s.nodeLocks.LoadOrStore(nodeID, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *SchedulerService) unlockNode(nodeID uint) {
	if value, ok := s.nodeLocks.Load(nodeID); ok {
		<-value.(chan struct{})
	}
}

func (s *SchedulerService) recordLog(level, category, message, details string) {
	if err := s.db.Create(&models.SystemLog{Level: level, Category: category, Message: message, Details: details}).Error; err != nil {
		log.Printf("[Audit] Failed to persist system log: %v", err)
	}
}

// nextRunAfter computes the next scheduled backup time after `after`.
//
// When no preferred time is configured it falls back to a simple rolling
// interval (after + freqHours), matching the previous behavior. When a
// preferred backup_hour (and, for weekly cadences, backup_day) is set, the
// result is anchored to that time of day so the node actually runs when the
// operator configured it to, instead of drifting from whenever the last
// backup happened to finish.
func nextRunAfter(after time.Time, freqHours int, backupHour, backupDay string) time.Time {
	if freqHours <= 0 {
		freqHours = 24
	}
	interval := time.Duration(freqHours) * time.Hour

	if backupHour == "" {
		return after.Add(interval)
	}
	anchor, err := time.Parse("15:04", backupHour)
	if err != nil {
		return after.Add(interval)
	}
	hour, minute := anchor.Hour(), anchor.Minute()

	// Weekly cadence: anchor to a specific weekday and time of day.
	if freqHours%(24*7) == 0 {
		weekday := after.Weekday()
		if dayIdx, err := strconv.Atoi(backupDay); err == nil && dayIdx >= 0 && dayIdx <= 6 {
			// Stored as 0=Monday..6=Sunday; time.Weekday is 0=Sunday..6=Saturday.
			weekday = time.Weekday((dayIdx + 1) % 7)
		}
		candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, after.Location())
		for candidate.Weekday() != weekday || !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	}

	// Daily (or slower, non-weekly-aligned) cadence: anchor to the same time
	// of day, moving to the next day if that time has already passed today.
	if freqHours%24 == 0 {
		candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, after.Location())
		if !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	}

	// Sub-daily cadence: anchor to fixed slots starting at backupHour and
	// repeating every freqHours (e.g. hour=02:00, freq=6 -> 02:00, 08:00,
	// 14:00, 20:00, ...), so displayed and actual run times always match.
	candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, after.Location())
	for !candidate.After(after) {
		candidate = candidate.Add(interval)
	}
	return candidate
}

func (s *SchedulerService) runBackup(node *models.Node) {
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
		if err := s.db.Where("node_id = ? AND status = 'success'", node.ID).Order("version desc").First(&lastBackup).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[Scheduler] Failed to load backup history for node %s: %v", node.Name, err)
			s.recordLog("error", "backup", fmt.Sprintf("Failed to load backup history for %s", node.Name), err.Error())
			return
		}

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
				if err := s.db.Create(&backup).Error; err != nil {
					log.Printf("[Scheduler] Failed to persist backup for node %s: %v", node.Name, err)
					s.recordLog("error", "backup", fmt.Sprintf("Failed to persist backup for %s", node.Name), err.Error())
					return
				}

				// Disparar alerta se o diff ocorreu
				isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
				if !isSnoozed {
					var rules []models.AlertRule
					s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
					for _, rule := range rules {
						if rule.Enabled && rule.AlertOnDiff {
							msg := fmt.Sprintf("*Configuration Drift Detected*\n\n*Node:* %s\n*IP:* %s\n*Vendor:* %s\n\n*Changes:* +%d additions, -%d deletions\n*New Version:* v%d", node.Name, node.IP, node.Vendor, diffRes.Additions, diffRes.Deletions, version)
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
			if err := s.db.Create(&backup).Error; err != nil {
				log.Printf("[Scheduler] Failed to persist first backup for node %s: %v", node.Name, err)
				s.recordLog("error", "backup", fmt.Sprintf("Failed to persist backup for %s", node.Name), err.Error())
				return
			}
		}

		// Calculate Security & Compliance Score
		newViolations := audit.RunAudit(s.db, node, version, config)

		// Alerta de Compliance (Security Violation)
		if len(newViolations) > 0 {
			isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
			if !isSnoozed {
				var rules []models.AlertRule
				s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
				for _, rule := range rules {
					if rule.Enabled && rule.AlertOnSecurity {
						var viols []string
						for _, v := range newViolations {
							viols = append(viols, fmt.Sprintf("- %s (Penalty: %d)", v.Rule.Name, v.Rule.Penalty))
						}

						msg := fmt.Sprintf("*Security Violation Detected*\n\n*Node:* %s\n*IP:* %s\n*Current Score:* %d/100\n\n*New Violations:*\n%s",
							node.Name, node.IP, node.SecurityScore, strings.Join(viols, "\n"))
						alert.Dispatch(s.db, rule, msg)
					}
				}
			} else {
				log.Printf("[Scheduler] Security alert suppressed for node %s due to snooze mode.", node.Name)
			}
		}

		// Alerta de Recovery: Se o último status foi erro, significa que o serviço recuperou
		if node.LastStatus == "error" {
			isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
			if !isSnoozed {
				var rules []models.AlertRule
				s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
				for _, rule := range rules {
					if rule.Enabled && rule.AlertOnFailure {
						msg := fmt.Sprintf("*Backup Recovered*\n\n*Node:* %s\n*IP:* %s\n\nBackup is now succeeding again.", node.Name, node.IP)
						alert.Dispatch(s.db, rule, msg)
					}
				}
			}
		}

	} else {
		// Deduplicação (State Transition): Só alerta falha se o node não estava falhando antes
		var lastBackup models.NodeBackup
		version := 0
		if err := s.db.Where("node_id = ? AND status = 'success'", node.ID).Order("version desc").First(&lastBackup).Error; err == nil {
			version = lastBackup.Version
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[Scheduler] Failed to load last successful backup for node %s: %v", node.Name, err)
		}
		failedBackup := models.NodeBackup{NodeID: node.ID, Version: version, Status: status, Error: errorMessage}
		if err := s.db.Create(&failedBackup).Error; err != nil {
			log.Printf("[Scheduler] Failed to persist failed attempt for node %s: %v", node.Name, err)
		}

		if node.LastStatus != "error" {
			isSnoozed := node.AlertSnoozeUntil != nil && time.Now().Before(*node.AlertSnoozeUntil)
			if !isSnoozed {
				var rules []models.AlertRule
				s.db.Where("target_group = ? OR target_group = '*' OR target_group = 'Global'", node.Group).Find(&rules)
				for _, rule := range rules {
					if rule.Enabled && rule.AlertOnFailure {
						msg := fmt.Sprintf("*Backup Failed*\n\n*Node:* %s\n*IP:* %s\n*Error:* %s", node.Name, node.IP, errorMessage)
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

	// Calculate next backup time based on frequency, anchored to the
	// operator's preferred backup time/day when one is configured.
	freqHours := 24
	backupHour := node.BackupHour
	backupDay := node.BackupDay
	if node.ScheduleType == "routine" && node.RoutineID != nil {
		var routine models.BackupRoutine
		s.db.First(&routine, node.RoutineID)
		freqHours, _ = strconv.Atoi(routine.Frequency)
		backupHour = routine.BackupHour
		backupDay = routine.BackupDay
	} else {
		freqHours, _ = strconv.Atoi(node.Frequency)
	}
	if freqHours == 0 {
		freqHours = 24
	}

	next := nextRunAfter(now, freqHours, backupHour, backupDay)
	node.NextBackupAt = &next

	if err := s.db.Model(&models.Node{}).Where("id = ?", node.ID).Updates(map[string]any{
		"last_status": status, "last_error": errorMessage,
		"last_backup_at": node.LastBackupAt, "next_backup_at": node.NextBackupAt,
	}).Error; err != nil {
		log.Printf("[Scheduler] Failed to update node %s after backup: %v", node.Name, err)
		s.recordLog("error", "backup", fmt.Sprintf("Failed to update node %s after backup", node.Name), err.Error())
		return
	}
	if status == "success" {
		s.recordLog("success", "backup", fmt.Sprintf("Backup completed for %s", node.Name), "")
	} else {
		s.recordLog("error", "backup", fmt.Sprintf("Backup failed for %s", node.Name), errorMessage)
	}
}

func (s *SchedulerService) CheckExports() {
	var settings models.SftpSettings
	if err := s.db.First(&settings).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[Scheduler] Failed to load SFTP settings: %v", err)
		}
		return
	}

	if !settings.Enabled {
		return
	}

	if strings.TrimSpace(settings.Host) == "" || strings.TrimSpace(settings.Username) == "" || strings.TrimSpace(settings.Password) == "" {
		settings.LastExportStatus = "error"
		settings.LastExportError = "SFTP settings are incomplete"
		if err := s.db.Save(&settings).Error; err != nil {
			log.Printf("[Scheduler] Failed to save incomplete SFTP status: %v", err)
		}
		return
	}

	now := time.Now()
	if settings.SyncTime != "" {
		scheduledClock, err := time.Parse("15:04", settings.SyncTime)
		if err != nil {
			log.Printf("[Scheduler] Invalid SFTP sync time %q", settings.SyncTime)
			return
		}
		scheduledAt := time.Date(now.Year(), now.Month(), now.Day(), scheduledClock.Hour(), scheduledClock.Minute(), 0, 0, now.Location())
		alreadyCompleted := settings.LastExportStatus == "success" && settings.LastExportAt != nil && !settings.LastExportAt.Before(scheduledAt)
		if now.Before(scheduledAt) || alreadyCompleted {
			return
		}
	}

	var backups []models.NodeBackup
	// Only fetch successful backups that haven't been exported yet
	if err := s.db.Preload("Node").Where("status = ? AND exported = ?", "success", false).Find(&backups).Error; err != nil {
		log.Printf("[Scheduler] Failed to load pending SFTP exports: %v", err)
		s.recordLog("error", "export", "Failed to load pending SFTP exports", err.Error())
		return
	}

	if len(backups) == 0 {
		return
	}

	log.Printf("[Scheduler] Found %d pending SFTP exports.", len(backups))

	successCount := 0
	var lastErr string
	for _, backup := range backups {
		if err := s.sftp.Export(&backup, &settings); err == nil {
			backup.Exported = true
			if err := s.db.Save(&backup).Error; err != nil {
				lastErr = err.Error()
				log.Printf("[Scheduler] Export completed but backup state could not be saved for ID %d: %v", backup.ID, err)
				continue
			}
			successCount++
			log.Printf("[Scheduler] Successfully exported backup ID %d to SFTP.", backup.ID)
		} else {
			lastErr = err.Error()
			log.Printf("[Scheduler] Failed to export backup ID %d: %v", backup.ID, err)
		}
	}

	settings.LastExportAt = &now
	if successCount == len(backups) {
		settings.LastExportStatus = "success"
		settings.LastExportError = ""
	} else if successCount > 0 {
		settings.LastExportStatus = "partial"
		settings.LastExportError = lastErr
	} else {
		settings.LastExportStatus = "error"
		settings.LastExportError = lastErr
	}
	if err := s.db.Save(&settings).Error; err != nil {
		log.Printf("[Scheduler] Failed to save SFTP export status: %v", err)
	}
	if settings.LastExportStatus == "success" {
		s.recordLog("success", "export", fmt.Sprintf("Scheduled SFTP export completed (%d backups)", successCount), "")
	} else {
		s.recordLog("error", "export", fmt.Sprintf("Scheduled SFTP export finished with status %s", settings.LastExportStatus), settings.LastExportError)
	}
}
