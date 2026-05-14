package handlers

import (
	"encoding/csv"
	"fmt"
	"mimic/internal/models"
	"mimic/internal/services/sftp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// ── Dashboard ──────────────────────────────────────────

type DashboardHandler struct {
	DB *gorm.DB
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

	var recentLogs []models.SystemLog
	h.DB.Order("created_at desc").Limit(8).Find(&recentLogs)

	var recentBackups []models.NodeBackup
	h.DB.Preload("Node").Order("created_at desc").Limit(5).Find(&recentBackups)

	data := fiber.Map{
		"Title":         "Dashboard",
		"Username":      c.Locals("username"),
		"Avatar":        c.Locals("avatar"),
		"Role":          c.Locals("role"),
		"CurrentRoute":  "dashboard",
		"Stats":         stats,
		"RecentLogs":    recentLogs,
		"RecentBackups": recentBackups,
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
		query = query.Where("name ILIKE ? OR ip ILIKE ? OR vendor ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
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
		return c.Status(404).SendString("Node não encontrado")
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
		return c.Status(404).SendString("Backup não encontrado")
	}

	return c.Render("partials/backup_view", fiber.Map{
		"Backup": backup,
	})
}

func (h *NodeHandler) ExportNodesCSV(c *fiber.Ctx) error {
	var nodes []models.Node
	h.DB.Order("name asc").Find(&nodes)

	filename := fmt.Sprintf("mimic_nodes_%s.csv", time.Now().Format("2006-01-02"))
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// Write BOM for Excel UTF-8 compatibility
	c.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c)

	// Header
	writer.Write([]string{
		"name", "vendor", "ip", "port", "username", "password",
		"group", "tags", "frequency", "enabled",
	})

	for _, node := range nodes {
		enabled := "true"
		if !node.Enabled {
			enabled = "false"
		}

		writer.Write([]string{
			node.Name,
			node.Vendor,
			node.IP,
			fmt.Sprintf("%d", node.Port),
			node.Username,
			"", // password omitted for security
			node.Group,
			node.Tags,
			node.Frequency,
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
		"users":       "Usuários",
		"credentials": "Credenciais SSH",
		"routines":    "Rotinas de Backup",
		"sftp":        "Configuração SFTP",
		"export":      "Exportação",
		"logs":        "Logs do Sistema",
		"profile":     "Meu Perfil",
	}

	data["Title"] = titles[tab]
	if data["Title"] == "" {
		data["Title"] = "Configurações"
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
