package handlers

import (
	"fmt"
	"strings"
	"time"

	"mimic/internal/access"
	"mimic/internal/models"
	"mimic/pkg/diff"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type APIHandler struct {
	DB    *gorm.DB
	Store *session.Store
}

type apiUserDTO struct {
	ID          uint                `json:"id"`
	Username    string              `json:"username"`
	Email       string              `json:"email,omitempty"`
	Role        string              `json:"role"`
	Avatar      string              `json:"avatar,omitempty"`
	Permissions []access.Permission `json:"permissions"`
}

type apiNodeDTO struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	Vendor           string     `json:"vendor"`
	IP               string     `json:"ip"`
	Port             int        `json:"port"`
	Username         string     `json:"username,omitempty"`
	Enabled          bool       `json:"enabled"`
	Group            string     `json:"group"`
	Tags             string     `json:"tags,omitempty"`
	ScheduleType     string     `json:"schedule_type"`
	RoutineID        *uint      `json:"routine_id,omitempty"`
	RoutineName      string     `json:"routine_name,omitempty"`
	CredentialID     *uint      `json:"credential_id,omitempty"`
	CredentialName   string     `json:"credential_name,omitempty"`
	AccessAgentID    *uint      `json:"access_agent_id,omitempty"`
	Frequency        string     `json:"frequency,omitempty"`
	BackupHour       string     `json:"backup_hour,omitempty"`
	BackupDay        string     `json:"backup_day,omitempty"`
	LastStatus       string     `json:"last_status"`
	LastError        string     `json:"last_error,omitempty"`
	LastBackupAt     *time.Time `json:"last_backup_at,omitempty"`
	NextBackupAt     *time.Time `json:"next_backup_at,omitempty"`
	AlertSnoozeUntil *time.Time `json:"alert_snooze_until,omitempty"`
	IsOnline         bool       `json:"is_online"`
	VerifyHostKey    bool       `json:"verify_host_key"`
}

type apiBackupDTO struct {
	ID            uint      `json:"id"`
	NodeID        uint      `json:"node_id"`
	NodeName      string    `json:"node_name,omitempty"`
	Version       int       `json:"version"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	Exported      bool      `json:"exported"`
	DiffAdditions int       `json:"diff_additions"`
	DiffDeletions int       `json:"diff_deletions"`
	CreatedAt     time.Time `json:"created_at"`
	Config        string    `json:"config,omitempty"`
}

type apiErrorEnvelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func writeAPIError(c *fiber.Ctx, status int, code, message string, fields map[string]string) error {
	response := apiErrorEnvelope{}
	response.Error.Code = code
	response.Error.Message = message
	response.Error.Fields = fields
	return c.Status(status).JSON(response)
}

func nodeDTO(node models.Node) apiNodeDTO {
	dto := apiNodeDTO{
		ID: node.ID, Name: node.Name, Vendor: node.Vendor, IP: node.IP,
		Port: node.Port, Username: node.Username, Enabled: node.Enabled,
		Group: node.Group, Tags: node.Tags, ScheduleType: node.ScheduleType,
		RoutineID: node.RoutineID, CredentialID: node.CredentialID,
		AccessAgentID: node.AccessAgentID, Frequency: node.Frequency,
		BackupHour: node.BackupHour, BackupDay: node.BackupDay,
		LastStatus: node.LastStatus, LastError: node.LastError,
		LastBackupAt: node.LastBackupAt, NextBackupAt: node.NextBackupAt,
		AlertSnoozeUntil: node.AlertSnoozeUntil, IsOnline: node.IsOnline,
		VerifyHostKey: node.VerifyHostKey,
	}
	if node.Routine != nil {
		dto.RoutineName = node.Routine.Name
	}
	if node.Credential != nil {
		dto.CredentialName = node.Credential.Name
	}
	return dto
}

func backupDTO(backup models.NodeBackup, includeConfig bool) apiBackupDTO {
	dto := apiBackupDTO{
		ID: backup.ID, NodeID: backup.NodeID, NodeName: backup.Node.Name,
		Version: backup.Version, Status: backup.Status, Error: backup.Error,
		Exported: backup.Exported, DiffAdditions: backup.DiffAdditions,
		DiffDeletions: backup.DiffDeletions, CreatedAt: backup.CreatedAt,
	}
	if includeConfig {
		dto.Config = backup.Config
	}
	return dto
}

func currentAPIUser(c *fiber.Ctx) apiUserDTO {
	role, _ := c.Locals("role").(string)
	id, _ := c.Locals("user_id").(uint)
	username, _ := c.Locals("username").(string)
	email, _ := c.Locals("email").(string)
	avatar, _ := c.Locals("avatar").(string)
	return apiUserDTO{
		ID: id, Username: username, Email: email, Role: role, Avatar: avatar,
		Permissions: access.PermissionsForRole(role),
	}
}

func (h *APIHandler) Me(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": currentAPIUser(c)})
}

func (h *APIHandler) Login(c *fiber.Ctx) error {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&input); err != nil {
		return writeAPIError(c, fiber.StatusBadRequest, "invalid_request", "The request body is invalid.", nil)
	}
	input.Username = strings.TrimSpace(input.Username)

	var user models.User
	userFound := h.DB.Where("username = ?", input.Username).First(&user).Error == nil
	hash := dummyPasswordHash
	if userFound && user.Password != "" {
		hash = user.Password
	}
	passwordMatches := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) == nil
	if !userFound || !passwordMatches {
		writeAuditLog(h.DB, c, "warning", "auth", "Failed API login attempt", "username="+input.Username)
		return writeAPIError(c, fiber.StatusUnauthorized, "invalid_credentials", "Invalid username or password.", nil)
	}

	sess, err := h.Store.Get(c)
	if err != nil || sess.Regenerate() != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "session_error", "Could not create the session.", nil)
	}
	sess.Set("user_id", user.ID)
	sess.Set("username", user.Username)
	sess.Set("role", user.Role)
	sess.Set("avatar", user.Avatar)
	if err := sess.Save(); err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "session_error", "Could not save the session.", nil)
	}
	writeAuditLog(h.DB, c, "success", "auth", "User logged in through API", "username="+user.Username)
	return c.JSON(fiber.Map{"data": apiUserDTO{
		ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role,
		Avatar: user.Avatar, Permissions: access.PermissionsForRole(user.Role),
	}})
}

func (h *APIHandler) Logout(c *fiber.Ctx) error {
	writeAuditLog(h.DB, c, "info", "auth", "User logged out through API", "")
	sess, err := h.Store.Get(c)
	if err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "session_error", "Could not open the session.", nil)
	}
	if err := sess.Destroy(); err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "session_error", "Could not destroy the session.", nil)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"logged_out": true}})
}

type apiDashboardDTO struct {
	Stats            DashboardStats      `json:"stats"`
	Attention        []apiAttentionDTO   `json:"attention"`
	Trend            []DashboardTrendDay `json:"trend"`
	Upcoming         []apiNodeDTO        `json:"upcoming"`
	RecentChanges    []apiBackupDTO      `json:"recent_changes"`
	RecentActivity   []apiActivityDTO    `json:"recent_activity"`
	SFTP             DashboardSFTPStatus `json:"sftp"`
	LastSuccessfulAt *time.Time          `json:"last_successful_at,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type apiAttentionDTO struct {
	Node     apiNodeDTO `json:"node"`
	Severity string     `json:"severity"`
	Label    string     `json:"label"`
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Context  string     `json:"context"`
}

type apiActivityDTO struct {
	ID        uint      `json:"id"`
	Level     string    `json:"level"`
	Category  string    `json:"category"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *APIHandler) Dashboard(c *fiber.Ctx) error {
	snapshot, err := (&DashboardHandler{DB: h.DB}).loadDashboard(time.Now())
	if err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "dashboard_unavailable", "Dashboard data could not be loaded.", nil)
	}
	dto := apiDashboardDTO{
		Stats: snapshot.Stats, Trend: snapshot.Trend, SFTP: snapshot.SFTP,
		LastSuccessfulAt: snapshot.LastSuccessfulAt, UpdatedAt: snapshot.UpdatedAt,
		Attention:      make([]apiAttentionDTO, 0, len(snapshot.Attention)),
		Upcoming:       make([]apiNodeDTO, 0, len(snapshot.Upcoming)),
		RecentChanges:  make([]apiBackupDTO, 0, len(snapshot.RecentChanges)),
		RecentActivity: make([]apiActivityDTO, 0, len(snapshot.RecentActivity)),
	}
	for _, item := range snapshot.Attention {
		dto.Attention = append(dto.Attention, apiAttentionDTO{
			Node: nodeDTO(item.Node), Severity: item.Severity, Label: item.Label,
			Title: item.Title, Detail: item.Detail, Context: item.Context,
		})
	}
	for _, node := range snapshot.Upcoming {
		dto.Upcoming = append(dto.Upcoming, nodeDTO(node))
	}
	for _, backup := range snapshot.RecentChanges {
		dto.RecentChanges = append(dto.RecentChanges, backupDTO(backup, false))
	}
	for _, entry := range snapshot.RecentActivity {
		dto.RecentActivity = append(dto.RecentActivity, apiActivityDTO{
			ID: entry.ID, Level: entry.Level, Category: entry.Category,
			Message: entry.Message, CreatedAt: entry.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"data": dto})
}

func (h *APIHandler) ListNodes(c *fiber.Ctx) error {
	filters := NodeFilters{
		Search: c.Query("search"), Status: c.Query("status"),
		Vendor: c.Query("vendor"), Group: c.Query("group"),
	}
	result, err := (&NodeHandler{DB: h.DB}).loadNodeList(filters, time.Now())
	if err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "nodes_unavailable", "Nodes could not be loaded.", nil)
	}
	nodes := make([]apiNodeDTO, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodes = append(nodes, nodeDTO(node))
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"nodes": nodes, "stats": result.Stats, "groups": result.Groups,
		"filters": fiber.Map{"search": filters.Search, "status": filters.Status, "vendor": filters.Vendor, "group": filters.Group},
	}})
}

func (h *APIHandler) Node(c *fiber.Ctx) error {
	var node models.Node
	if err := h.DB.Preload("Routine").Preload("Credential").Where("id = ?", c.Params("id")).First(&node).Error; err != nil {
		return writeAPIError(c, fiber.StatusNotFound, "not_found", "Node not found.", nil)
	}
	var backups []models.NodeBackup
	if err := h.DB.Where("node_id = ?", node.ID).Order("version desc").Limit(100).Find(&backups).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "backups_unavailable", "Backup history could not be loaded.", nil)
	}
	items := make([]apiBackupDTO, 0, len(backups))
	for _, backup := range backups {
		items = append(items, backupDTO(backup, false))
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"node": nodeDTO(node), "backups": items}})
}

func (h *APIHandler) Backup(c *fiber.Ctx) error {
	var backup models.NodeBackup
	if err := h.DB.Preload("Node").Where("id = ?", c.Params("id")).First(&backup).Error; err != nil {
		return writeAPIError(c, fiber.StatusNotFound, "not_found", "Backup not found.", nil)
	}
	return c.JSON(fiber.Map{"data": backupDTO(backup, true)})
}

type apiSplitRowDTO struct {
	LeftNum, LeftLine, LeftClass, RightNum, RightLine, RightClass string
}

type apiUnifiedRowDTO struct {
	Num, Sign, Line, Class string
}

func diffPayload(result diff.DiffResult, leftVersion, rightVersion string) fiber.Map {
	split := make([]fiber.Map, 0, len(result.SplitRows))
	for _, row := range result.SplitRows {
		split = append(split, fiber.Map{
			"left_num": row.LeftNum, "left_line": row.LeftLine, "left_class": row.LeftClass,
			"right_num": row.RightNum, "right_line": row.RightLine, "right_class": row.RightClass,
		})
	}
	unified := make([]fiber.Map, 0, len(result.UnifiedRows))
	for _, row := range result.UnifiedRows {
		unified = append(unified, fiber.Map{"num": row.Num, "sign": row.Sign, "line": row.Line, "class": row.Class})
	}
	return fiber.Map{
		"left_version": leftVersion, "right_version": rightVersion,
		"split_rows": split, "unified_rows": unified,
		"additions": result.Additions, "deletions": result.Deletions,
	}
}

func (h *APIHandler) BackupDiff(c *fiber.Ctx) error {
	var current models.NodeBackup
	if err := h.DB.Where("id = ?", c.Params("id")).First(&current).Error; err != nil {
		return writeAPIError(c, fiber.StatusNotFound, "not_found", "Backup not found.", nil)
	}
	var previous models.NodeBackup
	err := h.DB.Where("node_id = ? AND version < ? AND status = ?", current.NodeID, current.Version, "success").Order("version desc").First(&previous).Error
	left, label := "", "None"
	if err == nil {
		left, label = previous.Config, fmt.Sprintf("v%d", previous.Version)
	} else if err != gorm.ErrRecordNotFound {
		return writeAPIError(c, fiber.StatusInternalServerError, "diff_unavailable", "The comparison could not be loaded.", nil)
	}
	return c.JSON(fiber.Map{"data": diffPayload(diff.GenerateDiff(left, current.Config), label, fmt.Sprintf("v%d", current.Version))})
}

func (h *APIHandler) CompareBackups(c *fiber.Ctx) error {
	rightID := c.Query("right_id")
	if rightID == "" {
		return writeAPIError(c, fiber.StatusBadRequest, "validation_error", "right_id is required.", map[string]string{"right_id": "Select a target backup."})
	}
	var right models.NodeBackup
	if err := h.DB.Where("id = ?", rightID).First(&right).Error; err != nil {
		return writeAPIError(c, fiber.StatusNotFound, "not_found", "Target backup not found.", nil)
	}
	leftContent, leftLabel := "", "None"
	if leftID := c.Query("left_id"); leftID != "" && leftID != "0" {
		var left models.NodeBackup
		if err := h.DB.Where("id = ? AND node_id = ?", leftID, right.NodeID).First(&left).Error; err != nil {
			return writeAPIError(c, fiber.StatusNotFound, "not_found", "Base backup not found for this node.", nil)
		}
		leftContent, leftLabel = left.Config, fmt.Sprintf("v%d", left.Version)
	}
	return c.JSON(fiber.Map{"data": diffPayload(diff.GenerateDiff(leftContent, right.Config), leftLabel, fmt.Sprintf("v%d", right.Version))})
}

func (h *APIHandler) Profile(c *fiber.Ctx) error {
	var user models.User
	if err := h.DB.Select("id", "username", "email", "role", "avatar", "created_at").First(&user, c.Locals("user_id")).Error; err != nil {
		return writeAPIError(c, fiber.StatusNotFound, "not_found", "Profile not found.", nil)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": user.ID, "username": user.Username, "email": user.Email,
		"role": user.Role, "avatar": user.Avatar, "created_at": user.CreatedAt,
	}})
}

func (h *APIHandler) Credentials(c *fiber.Ctx) error {
	var credentials []models.Credential
	if err := h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name", "credential_id").Order("name asc") }).Order("name asc").Find(&credentials).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "credentials_unavailable", "Credentials could not be loaded.", nil)
	}
	items := make([]fiber.Map, 0, len(credentials))
	for _, credential := range credentials {
		nodes := make([]fiber.Map, 0, len(credential.Nodes))
		for _, node := range credential.Nodes {
			nodes = append(nodes, fiber.Map{"id": node.ID, "name": node.Name})
		}
		items = append(items, fiber.Map{
			"id": credential.ID, "name": credential.Name, "username": credential.Username,
			"port": credential.Port, "nodes": nodes, "node_count": len(nodes),
			"has_password": strings.TrimSpace(credential.Password) != "",
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"credentials": items}})
}

func (h *APIHandler) Routines(c *fiber.Ctx) error {
	var routines []models.BackupRoutine
	if err := h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name", "routine_id").Order("name asc") }).Order("name asc").Find(&routines).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "routines_unavailable", "Routines could not be loaded.", nil)
	}
	items := make([]fiber.Map, 0, len(routines))
	for _, routine := range routines {
		nodes := make([]fiber.Map, 0, len(routine.Nodes))
		for _, node := range routine.Nodes {
			nodes = append(nodes, fiber.Map{"id": node.ID, "name": node.Name})
		}
		items = append(items, fiber.Map{
			"id": routine.ID, "name": routine.Name, "description": routine.Description,
			"frequency": routine.Frequency, "backup_hour": routine.BackupHour,
			"backup_day": routine.BackupDay, "enabled": routine.Enabled,
			"nodes": nodes, "node_count": len(nodes),
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"routines": items}})
}

func (h *APIHandler) Users(c *fiber.Ctx) error {
	var users []models.User
	if err := h.DB.Select("id", "username", "email", "role", "avatar", "created_at").Order("created_at asc").Find(&users).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "users_unavailable", "Users could not be loaded.", nil)
	}
	items := make([]fiber.Map, 0, len(users))
	for _, user := range users {
		items = append(items, fiber.Map{
			"id": user.ID, "username": user.Username, "email": user.Email,
			"role": user.Role, "avatar": user.Avatar, "created_at": user.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"users": items, "current_user_id": c.Locals("user_id")}})
}

func (h *APIHandler) Logs(c *fiber.Ctx) error {
	var logs []models.SystemLog
	if err := h.DB.Order("created_at desc").Limit(200).Find(&logs).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "logs_unavailable", "Audit logs could not be loaded.", nil)
	}
	items := make([]fiber.Map, 0, len(logs))
	for _, entry := range logs {
		items = append(items, fiber.Map{
			"id": entry.ID, "level": entry.Level, "category": entry.Category,
			"message": entry.Message, "details": entry.Details, "created_at": entry.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"logs": items}})
}

func (h *APIHandler) Alerts(c *fiber.Ctx) error {
	var rules []models.AlertRule
	if err := h.DB.Order("enabled desc, name asc").Find(&rules).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "alerts_unavailable", "Alert rules could not be loaded.", nil)
	}
	items := make([]fiber.Map, 0, len(rules))
	for _, rule := range rules {
		items = append(items, fiber.Map{
			"id": rule.ID, "name": rule.Name, "target_group": rule.TargetGroup,
			"enabled": rule.Enabled, "provider": rule.Provider,
			"alert_on_diff": rule.AlertOnDiff, "alert_on_failure": rule.AlertOnFailure,
			"configured": (rule.Provider == "telegram" && strings.TrimSpace(rule.TelegramToken) != "") || (rule.Provider == "webhook" && strings.TrimSpace(rule.WebhookURL) != ""),
			"has_token":  strings.TrimSpace(rule.TelegramToken) != "", "has_webhook": strings.TrimSpace(rule.WebhookURL) != "",
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"alerts": items}})
}

func (h *APIHandler) SFTPSettings(c *fiber.Ctx) error {
	var settings models.SftpSettings
	if err := h.DB.First(&settings).Error; err != nil && err != gorm.ErrRecordNotFound {
		return writeAPIError(c, fiber.StatusInternalServerError, "sftp_unavailable", "SFTP settings could not be loaded.", nil)
	}
	hasPassword := strings.TrimSpace(settings.Password) != ""
	return c.JSON(fiber.Map{"data": fiber.Map{
		"id": settings.ID, "host": settings.Host, "port": settings.Port,
		"username": settings.Username, "path": settings.Path, "enabled": settings.Enabled,
		"sync_time": settings.SyncTime, "has_password": hasPassword,
		"configured":     strings.TrimSpace(settings.Host) != "" && strings.TrimSpace(settings.Username) != "" && hasPassword,
		"last_export_at": settings.LastExportAt, "last_export_status": settings.LastExportStatus,
		"last_export_error": settings.LastExportError,
	}})
}

func (h *APIHandler) ExportStatus(c *fiber.Ctx) error {
	var nodes []models.Node
	if err := h.DB.Where("enabled = ?", true).Order("name asc").Find(&nodes).Error; err != nil {
		return writeAPIError(c, fiber.StatusInternalServerError, "export_unavailable", "Export status could not be loaded.", nil)
	}
	rows := make([]fiber.Map, 0, len(nodes))
	for _, node := range nodes {
		var backup models.NodeBackup
		err := h.DB.Where("node_id = ? AND status = ?", node.ID, "success").Order("created_at desc").First(&backup).Error
		state := "unavailable"
		var backupAt *time.Time
		if err == nil {
			value := backup.CreatedAt
			backupAt = &value
			state = "pending"
			if backup.Exported {
				state = "exported"
			}
		} else if err != gorm.ErrRecordNotFound {
			return writeAPIError(c, fiber.StatusInternalServerError, "export_unavailable", "Export status could not be loaded.", nil)
		}
		rows = append(rows, fiber.Map{
			"id": node.ID, "name": node.Name, "vendor": node.Vendor, "ip": node.IP,
			"group": node.Group, "backup_at": backupAt, "export_state": state,
		})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"nodes": rows}})
}
