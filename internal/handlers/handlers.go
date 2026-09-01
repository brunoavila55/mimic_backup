package handlers

import (
	"encoding/csv"
	"fmt"
	"html"
	"mimic/internal/access"
	"mimic/internal/models"
	"mimic/internal/services/sftp"
	"mimic/pkg/diff"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// ── Dashboard ──────────────────────────────────────────

// silentThresholdHours define o tempo (em horas) sem backup bem-sucedido
// após o qual um node é considerado "silencioso" na dashboard.
// TODO: candidato a virar configurável em /settings no futuro.
// ── Nodes ──────────────────────────────────────────────

type NodeHandler struct {
	DB *gorm.DB
}

type NodeListStats struct {
	Total     int64
	Active    int64
	Attention int64
	Muted     int64
}

func (h *NodeHandler) ListNodes(c *fiber.Ctx) error {
	var nodes []models.Node
	query := h.DB.Preload("Routine").Preload("Credential")

	search := c.Query("search")
	status := c.Query("status")
	vendor := c.Query("vendor")
	group := c.Query("group")

	if search != "" {
		query = query.Where("name ILIKE ? OR ip ILIKE ? OR vendor ILIKE ? OR \"group\" ILIKE ? OR tags ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	if vendor != "" {
		query = query.Where("vendor = ?", vendor)
	}

	if group != "" {
		query = query.Where("\"group\" = ?", group)
	}

	now := time.Now()
	switch status {
	case "active":
		query = query.Where("enabled = ?", true)
	case "inactive":
		query = query.Where("enabled = ?", false)
	case "success":
		query = query.Where("last_status = ?", "success")
	case "error":
		query = query.Where("last_status = ?", "error")
	case "pending":
		query = query.Where("(last_status IS NULL OR last_status = '' OR last_status = ? OR last_status = ?)", "never", "pending")
	case "muted":
		query = query.Where("alert_snooze_until IS NOT NULL AND alert_snooze_until > ?", now)
	}

	query.Order("name asc").Find(&nodes)

	var stats NodeListStats
	h.DB.Model(&models.Node{}).Count(&stats.Total)
	h.DB.Model(&models.Node{}).Where("enabled = ?", true).Count(&stats.Active)
	h.DB.Model(&models.Node{}).Where("enabled = ? AND last_status = ?", true, "error").Count(&stats.Attention)
	h.DB.Model(&models.Node{}).Where("alert_snooze_until IS NOT NULL AND alert_snooze_until > ?", now).Count(&stats.Muted)

	var groups []string
	h.DB.Model(&models.Node{}).
		Where("\"group\" IS NOT NULL AND \"group\" != ''").
		Distinct().
		Order("\"group\" asc").
		Pluck("group", &groups)

	data := fiber.Map{
		"Title":        "Nodes",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Nodes":        nodes,
		"Search":       search,
		"FilterStatus": status,
		"FilterVendor": vendor,
		"FilterGroup":  group,
		"NodeStats":    stats,
		"Groups":       groups,
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

// csvSafe neutralizes leading characters that spreadsheet applications (Excel,
// Google Sheets, LibreOffice) interpret as the start of a formula, preventing
// CSV/formula injection when a user-controlled field (e.g. node name or tags)
// is exported and later opened by an operator.
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
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
		"group", "tags", "schedule_type", "frequency", "backup_hour", "backup_day",
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
			csvSafe(node.Name),
			node.Vendor,
			csvSafe(node.IP),
			fmt.Sprintf("%d", node.Port),
			csvSafe(node.Username),
			"", // password omitted for security
			csvSafe(node.Group),
			csvSafe(node.Tags),
			node.ScheduleType,
			node.Frequency,
			node.BackupHour,
			node.BackupDay,
			csvSafe(credName),
			csvSafe(routineName),
			csvSafe(agentName),
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

type UserListStats struct {
	Total          int
	Administrators int
	Operators      int
	Auditors       int
	Viewers        int
}

type CredentialListStats struct {
	Total       int
	LinkedNodes int
	Unused      int
	CustomPorts int
}

type RoutineListStats struct {
	Total       int
	Active      int
	Paused      int
	LinkedNodes int
	Weekly      int
}

type SftpStatusStats struct {
	Configured      bool
	Enabled         bool
	HasPassword     bool
	PendingBackups  int64
	PendingNodes    int64
	ExportedBackups int64
	LastStatus      string
}

type ExportNodeRow struct {
	ID          uint
	Name        string
	Vendor      string
	IP          string
	Group       string
	BackupAt    *time.Time
	ExportState string
}

type ExportPageStats struct {
	ActiveNodes int
	Pending     int
	Exported    int
	Unavailable int
	Configured  bool
}

type LogPageStats struct {
	Loaded   int
	Errors   int
	Warnings int
	Success  int
}

type AlertRuleStats struct {
	Total         int
	Enabled       int
	Disabled      int
	Webhook       int
	Telegram      int
	Global        int
	MissingEvents int
}

func (h *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	switch role {
	case access.RoleAdministrator:
		return c.Redirect("/settings/users")
	case access.RoleOperator:
		return c.Redirect("/settings/credentials")
	case access.RoleAuditor:
		return c.Redirect("/settings/logs")
	default:
		return c.Redirect("/settings/profile")
	}
}

func (h *SettingsHandler) renderTab(c *fiber.Ctx, tab string, data fiber.Map) error {
	titles := map[string]string{
		"users":       "Users",
		"credentials": "SSH Credentials",
		"routines":    "Backup Routines",
		"sftp":        "SFTP Configuration",
		"export":      "Export",
		"logs":        "System Logs",
		"alerts":      "Alerting Rules",
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
	role, _ := c.Locals("role").(string)
	data["CanManageUsers"] = access.Allows(role, access.ManageUsers)
	data["CanManageNodes"] = access.Allows(role, access.ManageNodes)
	data["CanManageOperations"] = access.Allows(role, access.ManageOperations)
	data["CanManageSystem"] = access.Allows(role, access.ManageSystem)
	data["CanExportBackups"] = access.Allows(role, access.ExportBackups)
	data["CanViewAudit"] = access.Allows(role, access.ViewAudit)

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

	stats := UserListStats{Total: len(users)}
	for _, user := range users {
		switch user.Role {
		case access.RoleAdministrator:
			stats.Administrators++
		case access.RoleOperator:
			stats.Operators++
		case access.RoleAuditor:
			stats.Auditors++
		default:
			stats.Viewers++
		}
	}

	return h.renderTab(c, "users", fiber.Map{
		"Users":               users,
		"UserStats":           stats,
		"CurrentUserID":       c.Locals("user_id"),
		"CurrentUserIDString": fmt.Sprintf("%v", c.Locals("user_id")),
	})
}

func (h *SettingsHandler) GetCredentialsTab(c *fiber.Ctx) error {
	var credentials []models.Credential
	h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB {
		return db.Order("name asc")
	}).Order("name asc").Find(&credentials)

	stats := CredentialListStats{Total: len(credentials)}
	for _, credential := range credentials {
		nodeCount := len(credential.Nodes)
		stats.LinkedNodes += nodeCount
		if nodeCount == 0 {
			stats.Unused++
		}
		if credential.Port != 22 {
			stats.CustomPorts++
		}
	}

	return h.renderTab(c, "credentials", fiber.Map{
		"Credentials":     credentials,
		"CredentialStats": stats,
	})
}

func (h *SettingsHandler) GetRoutinesTab(c *fiber.Ctx) error {
	var routines []models.BackupRoutine
	h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB {
		return db.Order("name asc")
	}).Order("name asc").Find(&routines)

	stats := RoutineListStats{Total: len(routines)}
	for _, routine := range routines {
		stats.LinkedNodes += len(routine.Nodes)
		if routine.Enabled {
			stats.Active++
		} else {
			stats.Paused++
		}
		if routine.Frequency == "168" {
			stats.Weekly++
		}
	}

	return h.renderTab(c, "routines", fiber.Map{
		"Routines":     routines,
		"RoutineStats": stats,
	})
}

func (h *SettingsHandler) GetSFTPTab(c *fiber.Ctx) error {
	var settings models.SftpSettings
	h.DB.First(&settings)

	var stats SftpStatusStats
	stats.Enabled = settings.Enabled
	stats.HasPassword = strings.TrimSpace(settings.Password) != ""
	stats.Configured = strings.TrimSpace(settings.Host) != "" && strings.TrimSpace(settings.Username) != "" && stats.HasPassword
	stats.LastStatus = settings.LastExportStatus
	if stats.LastStatus == "" {
		stats.LastStatus = "never"
	}
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", false).Count(&stats.PendingBackups)
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", false).Distinct("node_id").Count(&stats.PendingNodes)
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", true).Count(&stats.ExportedBackups)

	return h.renderTab(c, "sftp", fiber.Map{
		"Sftp":      settings,
		"SftpStats": stats,
	})
}

func (h *SettingsHandler) GetExportTab(c *fiber.Ctx) error {
	var nodes []models.Node
	h.DB.Where("enabled = ?", true).Order("name asc").Find(&nodes)

	var settings models.SftpSettings
	h.DB.First(&settings)

	latestBackups := make(map[uint]models.NodeBackup, len(nodes))
	if len(nodes) > 0 {
		nodeIDs := make([]uint, 0, len(nodes))
		for _, node := range nodes {
			nodeIDs = append(nodeIDs, node.ID)
		}

		var backups []models.NodeBackup
		h.DB.Where("node_id IN ? AND status = ?", nodeIDs, "success").
			Order("created_at desc").Find(&backups)
		for _, backup := range backups {
			if _, exists := latestBackups[backup.NodeID]; !exists {
				latestBackups[backup.NodeID] = backup
			}
		}
	}

	rows := make([]ExportNodeRow, 0, len(nodes))
	stats := ExportPageStats{
		ActiveNodes: len(nodes),
		Configured: strings.TrimSpace(settings.Host) != "" &&
			strings.TrimSpace(settings.Username) != "" &&
			strings.TrimSpace(settings.Password) != "" &&
			settings.Port > 0 && settings.Port <= 65535,
	}
	for _, node := range nodes {
		row := ExportNodeRow{
			ID: node.ID, Name: node.Name, Vendor: node.Vendor,
			IP: node.IP, Group: node.Group, ExportState: "unavailable",
		}
		if backup, exists := latestBackups[node.ID]; exists {
			backupAt := backup.CreatedAt
			row.BackupAt = &backupAt
			if backup.Exported {
				row.ExportState = "exported"
				stats.Exported++
			} else {
				row.ExportState = "pending"
				stats.Pending++
			}
		} else {
			stats.Unavailable++
		}
		rows = append(rows, row)
	}

	return h.renderTab(c, "export", fiber.Map{
		"ExportNodes": rows,
		"ExportStats": stats,
		"Sftp":        settings,
	})
}

func (h *SettingsHandler) GetAlertsTab(c *fiber.Ctx) error {
	var rules []models.AlertRule
	h.DB.Order("enabled desc, name asc").Find(&rules)

	stats := AlertRuleStats{Total: len(rules)}
	for _, rule := range rules {
		if rule.Enabled {
			stats.Enabled++
		} else {
			stats.Disabled++
		}
		switch rule.Provider {
		case "telegram":
			stats.Telegram++
		default:
			stats.Webhook++
		}
		if rule.TargetGroup == "*" || strings.EqualFold(rule.TargetGroup, "global") {
			stats.Global++
		}
		if !rule.AlertOnDiff && !rule.AlertOnFailure {
			stats.MissingEvents++
		}
	}

	return h.renderTab(c, "alerts", fiber.Map{
		"Alerts":     rules,
		"AlertStats": stats,
	})
}

func (h *SettingsHandler) GetLogsTab(c *fiber.Ctx) error {
	var logs []models.SystemLog
	h.DB.Order("created_at desc").Limit(200).Find(&logs)

	stats := LogPageStats{Loaded: len(logs)}
	for _, entry := range logs {
		switch strings.ToLower(entry.Level) {
		case "error":
			stats.Errors++
		case "warning":
			stats.Warnings++
		case "success":
			stats.Success++
		}
	}

	var categories []string
	h.DB.Model(&models.SystemLog{}).
		Where("category <> ''").Distinct("category").Order("category asc").Pluck("category", &categories)

	return h.renderTab(c, "logs", fiber.Map{
		"Logs":          logs,
		"LogStats":      stats,
		"LogCategories": categories,
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
	remotePath = normalizeSFTPPath(remotePath)

	files, err := h.Sftp.ListDir(&settings, remotePath)
	if err != nil {
		return c.Status(200).SendString(fmt.Sprintf("<div class='sftp-explorer-error'><i class='bx bx-error-circle'></i><strong>Unable to open remote directory</strong><span>%s</span></div>", html.EscapeString(err.Error())))
	}

	parentPath := ""
	if remotePath != "/" {
		parentPath = path.Dir(remotePath)
		if parentPath == "." {
			parentPath = "/"
		}
	}

	return c.Render("partials/sftp_explorer", fiber.Map{
		"Files":       files,
		"CurrentPath": remotePath,
		"ParentPath":  parentPath,
	})
}
