package handlers

import (
	"encoding/csv"
	"fmt"
	"mimic/internal/models"
	"mimic/internal/services/sftp"
	"mimic/pkg/diff"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// ── Dashboard ──────────────────────────────────────────

// silentThresholdHours define o tempo (em horas) sem backup bem-sucedido
// após o qual um node é considerado "silencioso" na dashboard.
// TODO: candidato a virar configurável em /settings no futuro.
const silentThresholdHours = 48

type DashboardHandler struct {
	DB *gorm.DB
}

type VendorFailure struct {
	Vendor string
	Count  int64
}

func (h *DashboardHandler) GetDashboard(c *fiber.Ctx) error {
	var stats struct {
		TotalNodes    int64
		ActiveNodes   int64
		TotalBackups  int64
		FailedBackups int64
	}
	h.DB.Model(&models.Node{}).Count(&stats.TotalNodes)
	h.DB.Model(&models.Node{}).Where("enabled = ?", true).Count(&stats.ActiveNodes)
	h.DB.Model(&models.NodeBackup{}).Where("status = ?", "success").Count(&stats.TotalBackups)
	h.DB.Model(&models.NodeBackup{}).Where("status = ?", "error").Count(&stats.FailedBackups)

	var failedNodes []models.Node
	h.DB.Where("enabled = ? AND last_status = ?", true, "error").Order("updated_at desc").Find(&failedNodes)

	var silentNodes []models.Node
	thresholdTime := time.Now().Add(-silentThresholdHours * time.Hour)
	h.DB.Where("enabled = ? AND last_status != ? AND (last_backup_at IS NULL OR last_backup_at < ?)", true, "error", thresholdTime).Order("last_backup_at asc").Find(&silentNodes)

	var vendorFailures []VendorFailure
	h.DB.Model(&models.Node{}).Select("vendor, count(*) as count").Where("enabled = ? AND last_status = ?", true, "error").Group("vendor").Order("count desc").Scan(&vendorFailures)

	var sftpUnsyncedCount int64
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", false).Select("count(distinct node_id)").Scan(&sftpUnsyncedCount)

	var nextBackups []models.Node
	now := time.Now()
	h.DB.Where("enabled = ? AND next_backup_at > ?", true, now).Order("next_backup_at asc").Limit(5).Find(&nextBackups)

	var recentLogs []models.SystemLog
	h.DB.Order("created_at desc").Limit(8).Find(&recentLogs)

	var recentBackups []models.NodeBackup
	h.DB.Preload("Node").Order("created_at desc").Limit(5).Find(&recentBackups)

	data := fiber.Map{
		"Title":             "Dashboard",
		"Username":          c.Locals("username"),
		"Avatar":            c.Locals("avatar"),
		"Role":              c.Locals("role"),
		"CurrentRoute":      "dashboard",
		"Stats":             stats,
		"RecentLogs":        recentLogs,
		"RecentBackups":     recentBackups,
		"FailedNodes":       failedNodes,
		"SilentNodes":       silentNodes,
		"VendorFailures":    vendorFailures,
		"SftpUnsyncedCount": sftpUnsyncedCount,
		"NextBackups":       nextBackups,
	}

	if c.Get("HX-Request") == "true" && c.Get("HX-Target") == "dashboard-content" {
		return c.Render("partials/dashboard_stats", data)
	}

	return c.Render("dashboard", data, "base")
}

// ── Nodes ──────────────────────────────────────────────

type NodeHandler struct {
	DB *gorm.DB
}

func (h *NodeHandler) ListNodes(c *fiber.Ctx) error {
	var nodes []models.Node
	query := h.DB.Preload("Routine").Preload("Credential")

	search := c.Query("search")
	if search != "" {
		query = query.Where("name ILIKE ? OR ip ILIKE ? OR vendor ILIKE ? OR \"group\" ILIKE ? OR tags ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	query.Order("name asc").Find(&nodes)

	data := fiber.Map{
		"Title":        "Nodes",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Nodes":        nodes,
		"Search":       search,
	}

	if c.Get("HX-Request") == "true" && c.Get("HX-Target") == "node-table-container" {
		return c.Render("partials/node_table", data)
	}

	return c.Render("node_list", data, "base")
}

func (h *NodeHandler) NodeDetails(c *fiber.Ctx) error {
	id := c.Params("id")
	var node models.Node
	if err := h.DB.Preload("Backups", func(db *gorm.DB) *gorm.DB {
		return db.Order("version desc")
	}).Preload("Credential").Where("id = ?", id).First(&node).Error; err != nil {
		return c.Status(404).SendString("Node not found")
	}

	data := fiber.Map{
		"Title":        node.Name,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Node":         node,
	}

	return c.Render("node_details", data, "base")
}

func (h *NodeHandler) GetBackupContent(c *fiber.Ctx) error {
	id := c.Params("id")
	var backup models.NodeBackup
	if err := h.DB.Preload("Node").Where("id = ?", id).First(&backup).Error; err != nil {
		return c.Status(404).SendString("Backup not found")
	}

	return c.Render("partials/backup_view", fiber.Map{
		"Backup": backup,
	})
}

func (h *NodeHandler) GetBackupDiff(c *fiber.Ctx) error {
	id := c.Params("id")
	var backup models.NodeBackup
	if err := h.DB.Preload("Node").Where("id = ?", id).First(&backup).Error; err != nil {
		return c.Status(404).SendString("Backup not found")
	}

	// Fetch all successful backups for this node to compare
	var backups []models.NodeBackup
	h.DB.Where("node_id = ? AND status = ?", backup.NodeID, "success").Order("version desc").Find(&backups)

	// Find the preceding backup version of the same node
	var prevBackup models.NodeBackup
	var hasPrev bool
	if backup.Version > 1 {
		err := h.DB.Where("node_id = ? AND version = ? AND status = ?", backup.NodeID, backup.Version-1, "success").First(&prevBackup).Error
		if err == nil {
			hasPrev = true
		}
	}

	// If no predecessor was found, try finding any backup with a lower version
	if !hasPrev {
		err := h.DB.Where("node_id = ? AND version < ? AND status = ?", backup.NodeID, backup.Version, "success").Order("version desc").First(&prevBackup).Error
		if err == nil {
			hasPrev = true
		}
	}

	// Compute initial diff
	var leftContent string
	var leftVersion string = "None"
	var leftID uint
	if hasPrev {
		leftContent = prevBackup.Config
		leftVersion = fmt.Sprintf("v%d", prevBackup.Version)
		leftID = prevBackup.ID
	}

	diffRes := diff.GenerateDiff(leftContent, backup.Config)

	return c.Render("partials/diff_view", fiber.Map{
		"Node":          backup.Node,
		"CurrentBackup": backup,
		"Backups":       backups,
		"HasPrev":       hasPrev,
		"LeftVersion":   leftVersion,
		"LeftID":        leftID,
		"RightID":       backup.ID,
		"SplitRows":     diffRes.SplitRows,
		"UnifiedRows":   diffRes.UnifiedRows,
		"Additions":     diffRes.Additions,
		"Deletions":     diffRes.Deletions,
	})
}

func (h *NodeHandler) CompareBackups(c *fiber.Ctx) error {
	leftID := c.Query("left_id")
	rightID := c.Query("right_id")

	var leftBackup models.NodeBackup
	var rightBackup models.NodeBackup

	var leftContent string
	var leftVersion string = "None"
	if leftID != "" && leftID != "0" {
		if err := h.DB.Where("id = ?", leftID).First(&leftBackup).Error; err == nil {
			leftContent = leftBackup.Config
			leftVersion = fmt.Sprintf("v%d", leftBackup.Version)
		}
	}

	if rightID == "" {
		return c.Status(400).SendString("Right backup ID is required")
	}

	if err := h.DB.Where("id = ?", rightID).First(&rightBackup).Error; err != nil {
		return c.Status(404).SendString("Right backup not found")
	}

	diffRes := diff.GenerateDiff(leftContent, rightBackup.Config)

	return c.Render("partials/diff_body", fiber.Map{
		"LeftVersion":  leftVersion,
		"RightVersion": fmt.Sprintf("v%d", rightBackup.Version),
		"SplitRows":    diffRes.SplitRows,
		"UnifiedRows":  diffRes.UnifiedRows,
		"Additions":    diffRes.Additions,
		"Deletions":    diffRes.Deletions,
	})
}


func (h *NodeHandler) ExportNodesCSV(c *fiber.Ctx) error {
	var nodes []models.Node
	h.DB.Preload("Credential").Preload("Routine").Preload("AccessAgent").Order("name asc").Find(&nodes)

	filename := fmt.Sprintf("mimic_nodes_%s.csv", time.Now().Format("2006-01-02"))
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// Write BOM for Excel UTF-8 compatibility
	c.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c)

	// Header
	writer.Write([]string{
		"name", "vendor", "ip", "port", "username", "password",
		"group", "schedule_type", "frequency", "backup_hour", "backup_day",
		"credential_name", "routine_name", "agent_name", "enabled",
	})

	for _, node := range nodes {
		enabled := "true"
		if !node.Enabled {
			enabled = "false"
		}

		credName := ""
		if node.Credential != nil {
			credName = node.Credential.Name
		}
		
		routineName := ""
		if node.Routine != nil {
			routineName = node.Routine.Name
		}

		agentName := ""
		if node.AccessAgent != nil {
			agentName = node.AccessAgent.Name
		}

		writer.Write([]string{
			node.Name,
			node.Vendor,
			node.IP,
			fmt.Sprintf("%d", node.Port),
			node.Username,
			"", // password omitted for security
			node.Group,
			node.ScheduleType,
			node.Frequency,
			node.BackupHour,
			node.BackupDay,
			credName,
			routineName,
			agentName,
			enabled,
		})
	}

	writer.Flush()
	return nil
}

// ── Settings Hub ───────────────────────────────────────

type SettingsHandler struct {
	DB    *gorm.DB
	Store *session.Store
	Sftp  *sftp.SftpService
}

func (h *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	return c.Redirect("/settings/users")
}

func (h *SettingsHandler) renderTab(c *fiber.Ctx, tab string, data fiber.Map) error {
	titles := map[string]string{
		"users":       "Users",
		"credentials": "SSH Credentials",
		"routines":    "Backup Routines",
		"sftp":        "SFTP Configuration",
		"export":      "Export",
		"logs":        "System Logs",
		"profile":     "My Profile",
	}

	data["Title"] = titles[tab]
	if data["Title"] == "" {
		data["Title"] = "Settings"
	}

	data["Username"] = c.Locals("username")
	data["Avatar"] = c.Locals("avatar")
	data["Role"] = c.Locals("role")
	data["CurrentRoute"] = tab
	data["ActiveTab"] = tab

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Push-Url", "/settings/"+tab)
		return c.Render("partials/settings_"+tab, data)
	}

	// Standalone rendering for sidebar-promoted tabs
	if tab == "credentials" || tab == "routines" {
		return c.Render(tab+"_list", data, "base")
	}

	return c.Render("settings", data, "base")
}

func (h *SettingsHandler) GetUsersTab(c *fiber.Ctx) error {
	var users []models.User
	h.DB.Order("created_at asc").Find(&users)

	return h.renderTab(c, "users", fiber.Map{
		"Users": users,
	})
}

func (h *SettingsHandler) GetCredentialsTab(c *fiber.Ctx) error {
	var credentials []models.Credential
	h.DB.Order("name asc").Find(&credentials)

	return h.renderTab(c, "credentials", fiber.Map{
		"Credentials": credentials,
	})
}

func (h *SettingsHandler) GetRoutinesTab(c *fiber.Ctx) error {
	var routines []models.BackupRoutine
	h.DB.Order("name asc").Find(&routines)

	return h.renderTab(c, "routines", fiber.Map{
		"Routines": routines,
	})
}

func (h *SettingsHandler) GetSFTPTab(c *fiber.Ctx) error {
	var settings models.SftpSettings
	h.DB.First(&settings)

	return h.renderTab(c, "sftp", fiber.Map{
		"Sftp": settings,
	})
}

func (h *SettingsHandler) GetExportTab(c *fiber.Ctx) error {
	var nodes []models.Node
	h.DB.Where("enabled = ?", true).Find(&nodes)

	var settings models.SftpSettings
	h.DB.First(&settings)

	return h.renderTab(c, "export", fiber.Map{
		"Nodes": nodes,
		"Sftp":  settings,
	})
}

func (h *SettingsHandler) GetAlertsTab(c *fiber.Ctx) error {
	var settings models.AlertSettings
	h.DB.First(&settings)

	return h.renderTab(c, "alerts", fiber.Map{
		"Alerts": settings,
	})
}

func (h *SettingsHandler) GetLogsTab(c *fiber.Ctx) error {
	var logs []models.SystemLog
	h.DB.Order("created_at desc").Limit(200).Find(&logs)

	return h.renderTab(c, "logs", fiber.Map{
		"Logs": logs,
	})
}

func (h *SettingsHandler) GetProfileTab(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	var user models.User
	h.DB.First(&user, userID)

	return h.renderTab(c, "profile", fiber.Map{
		"ProfileUser": user,
	})
}

func (h *SettingsHandler) GetSFTPExplore(c *fiber.Ctx) error {
	var settings models.SftpSettings
	h.DB.First(&settings)

	remotePath := c.Query("path")
	if remotePath == "" {
		remotePath = settings.Path
	}
	if remotePath == "" {
		remotePath = "/"
	}

	files, err := h.Sftp.ListDir(&settings, remotePath)
	if err != nil {
		return c.Status(500).SendString(fmt.Sprintf("<div class='alert alert-error' style='color: #ef4444; background: #fee2e2; padding: 12px; border-radius: 6px; margin-top: 16px;'>Failed to list directory: %v</div>", err))
	}

	parentPath := ""
	if remotePath != "/" {
		// Calculate parent path using string manipulation or path package.
		// To avoid importing path just for this, we can do simple parsing or import path.
		// Since we didn't add "path" to imports yet, let's just use string parsing or actually we will add the import.
		// It's safer to use simple string manipulation:
		lastSlash := -1
		for i := len(remotePath) - 1; i >= 0; i-- {
			if remotePath[i] == '/' {
				lastSlash = i
				break
			}
		}
		if lastSlash > 0 {
			parentPath = remotePath[:lastSlash]
		} else {
			parentPath = "/"
		}
	}

	return c.Render("partials/sftp_explorer", fiber.Map{
		"Files":       files,
		"CurrentPath": remotePath,
		"ParentPath":  parentPath,
	})
}
