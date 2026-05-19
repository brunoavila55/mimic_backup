package handlers

import (
	"encoding/csv"
	"fmt"
	"io"
	"mimic/internal/models"
	"mimic/internal/services/sftp"
	"mimic/pkg/crypto"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type FormHandler struct {
	DB    *gorm.DB
	Store *session.Store
	Sftp  *sftp.SftpService
}

// ── Node Forms ─────────────────────────────────────────

func (h *FormHandler) NewNode(c *fiber.Ctx) error {
	var routines []models.BackupRoutine
	h.DB.Find(&routines)

	var credentials []models.Credential
	h.DB.Find(&credentials)

	var agents []models.AccessAgent
	h.DB.Find(&agents)

	return c.Render("node_form", fiber.Map{
		"Title":        "New Node",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Routines":     routines,
		"Credentials":  credentials,
		"Agents":       agents,
	}, "base")
}

func (h *FormHandler) EditNode(c *fiber.Ctx) error {
	id := c.Params("id")
	var node models.Node
	if err := h.DB.Where("id = ?", id).First(&node).Error; err != nil {
		return c.Status(404).SendString("Node not found")
	}

	var routines []models.BackupRoutine
	h.DB.Find(&routines)

	var credentials []models.Credential
	h.DB.Find(&credentials)

	var agents []models.AccessAgent
	h.DB.Find(&agents)

	return c.Render("node_form", fiber.Map{
		"Title":        "Edit " + node.Name,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Node":         node,
		"Routines":     routines,
		"Credentials":  credentials,
		"Agents":       agents,
	}, "base")
}

func (h *FormHandler) SaveNode(c *fiber.Ctx) error {
	id := c.Params("id")
	var node models.Node

	if id != "" {
		h.DB.Where("id = ?", id).First(&node)
	}

	node.Name = c.FormValue("name")
	node.IP = c.FormValue("ip")
	node.Vendor = c.FormValue("vendor")
	node.Username = c.FormValue("username")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return c.Status(500).SendString("Error encrypting password: " + err.Error())
		}
		node.Password = encPass
	}

	port, _ := strconv.Atoi(c.FormValue("port"))
	node.Port = port
	node.Group = c.FormValue("group")
	node.Tags = c.FormValue("tags")
	node.ScheduleType = c.FormValue("schedule_type")
	node.Frequency = c.FormValue("frequency")
	node.BackupHour = c.FormValue("backup_hour")
	node.BackupDay = c.FormValue("backup_day")

	routineID, _ := strconv.Atoi(c.FormValue("routine_id"))
	if routineID > 0 {
		rid := uint(routineID)
		node.RoutineID = &rid
	} else {
		node.RoutineID = nil
	}

	credentialID, _ := strconv.Atoi(c.FormValue("credential_id"))
	if credentialID > 0 {
		cid := uint(credentialID)
		node.CredentialID = &cid
	} else {
		node.CredentialID = nil
	}

	agentID, _ := strconv.Atoi(c.FormValue("access_agent_id"))
	if agentID > 0 {
		aid := uint(agentID)
		node.AccessAgentID = &aid
	} else {
		node.AccessAgentID = nil
	}

	node.Enabled = c.FormValue("enabled") == "on"

	if err := h.DB.Save(&node).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	c.Set("HX-Trigger", `{"showNotification": {"message": "Node saved successfully", "type": "success"}}`)
	return c.Redirect("/nodes")
}

func (h *FormHandler) DeleteNodeConfirm(c *fiber.Ctx) error {
	id := c.Params("id")
	var node models.Node
	if err := h.DB.Where("id = ?", id).First(&node).Error; err != nil {
		return c.Status(404).SendString("Node not found")
	}

	return c.Render("node_confirm_delete", fiber.Map{
		"Title":        "Delete " + node.Name,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Node":         node,
	}, "base")
}

func (h *FormHandler) DeleteNode(c *fiber.Ctx) error {
	id := c.Params("id")
	// Also delete associated backups to avoid orphan records
	h.DB.Where("node_id = ?", id).Delete(&models.NodeBackup{})
	h.DB.Where("id = ?", id).Delete(&models.Node{})

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Trigger", `{"showNotification": {"message": "Node deleted", "type": "success"}}`)
		return c.SendString("")
	}

	return c.Redirect("/nodes")
}

// ── Node Import/Export ─────────────────────────────────

func (h *FormHandler) ImportNodesForm(c *fiber.Ctx) error {
	return c.Render("node_import", fiber.Map{
		"Title":        "Import Nodes",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
	}, "base")
}

func (h *FormHandler) ImportNodesCSV(c *fiber.Ctx) error {
	file, err := c.FormFile("csv_file")
	if err != nil {
		return c.Render("node_import", fiber.Map{
			"Title":        "Import Nodes",
			"Username":     c.Locals("username"),
			"Avatar":       c.Locals("avatar"),
			"Role":         c.Locals("role"),
			"CurrentRoute": "nodes",
			"Error":        "No file selected.",
		}, "base")
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).SendString("Error opening file")
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true

	// Read header row
	header, err := reader.Read()
	if err != nil {
		return c.Render("node_import", fiber.Map{
			"Title":        "Import Nodes",
			"Username":     c.Locals("username"),
			"Avatar":       c.Locals("avatar"),
			"Role":         c.Locals("role"),
			"CurrentRoute": "nodes",
			"Error":        "Empty or invalid CSV file.",
		}, "base")
	}

	// Map header columns to indices
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	// Validate required columns
	requiredCols := []string{"name", "ip", "vendor"}
	for _, col := range requiredCols {
		if _, ok := colMap[col]; !ok {
			return c.Render("node_import", fiber.Map{
				"Title":        "Import Nodes",
				"Username":     c.Locals("username"),
				"Avatar":       c.Locals("avatar"),
				"Role":         c.Locals("role"),
				"CurrentRoute": "nodes",
				"Error":        fmt.Sprintf("Required column '%s' not found in CSV.", col),
			}, "base")
		}
	}

	successCount := 0
	errorCount := 0
	var errors []string
	lineNum := 1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		if err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d: read error", lineNum))
			continue
		}

		getCol := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx])
			}
			return ""
		}

		nodeName := getCol("name")
		nodeIP := getCol("ip")
		nodeVendor := getCol("vendor")

		if nodeName == "" || nodeIP == "" {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d: empty name or IP", lineNum))
			continue
		}

		port, _ := strconv.Atoi(getCol("port"))
		if port == 0 {
			port = 22
		}

		frequency := getCol("frequency")
		if frequency == "" {
			frequency = "24"
		}

		group := getCol("group")
		if group == "" {
			group = "General"
		}

		enabled := true
		enabledStr := strings.ToLower(getCol("enabled"))
		if enabledStr == "false" || enabledStr == "0" || enabledStr == "no" {
			enabled = false
		}

		node := models.Node{
			Name:      nodeName,
			Vendor:    nodeVendor,
			IP:        nodeIP,
			Port:      port,
			Username:  getCol("username"),
			Group:     group,
			Tags:      getCol("tags"),
			Frequency: frequency,
			Enabled:   enabled,
		}

		// Encrypt password if provided
		rawPass := getCol("password")
		if rawPass != "" {
			encPass, err := crypto.Encrypt(rawPass)
			if err == nil {
				node.Password = encPass
			}
		}

		if err := h.DB.Create(&node).Error; err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): %v", lineNum, nodeName, err))
			continue
		}

		successCount++
	}

	if successCount == 0 && errorCount == 0 {
		return c.Render("node_import", fiber.Map{
			"Title":        "Import Nodes",
			"Username":     c.Locals("username"),
			"Avatar":       c.Locals("avatar"),
			"Role":         c.Locals("role"),
			"CurrentRoute": "nodes",
			"Error":        "No nodes found in CSV file.",
		}, "base")
	}

	return c.Render("node_import", fiber.Map{
		"Title":        "Import Nodes",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Success":      fmt.Sprintf("%d nodes successfully imported.", successCount),
		"ErrorCount":   errorCount,
		"Errors":       errors,
	}, "base")
}

// ── User Forms ─────────────────────────────────────────

func (h *FormHandler) NewUser(c *fiber.Ctx) error {
	return c.Render("user_form", fiber.Map{
		"Title":        "New User",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
	}, "base")
}

func (h *FormHandler) EditUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var user models.User
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	return c.Render("user_form", fiber.Map{
		"Title":        "Edit " + user.Username,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"EditUser":     user,
	}, "base")
}

func (h *FormHandler) SaveUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var user models.User

	if id != "" {
		h.DB.Where("id = ?", id).First(&user)
	}

	user.Username = c.FormValue("username")
	user.Email = c.FormValue("email")
	user.Role = c.FormValue("role")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		user.Password = string(hash)
	}

	if err := h.DB.Save(&user).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	c.Set("HX-Trigger", `{"showNotification": {"message": "User saved successfully", "type": "success"}}`)
	return c.Redirect("/settings/users")
}

func (h *FormHandler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	if c.Locals("user_id") != nil {
		localId := fmt.Sprintf("%v", c.Locals("user_id"))
		if id == localId {
			return c.Status(400).SendString("Cannot delete your own user")
		}
	}

	h.DB.Where("id = ?", id).Delete(&models.User{})

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Trigger", `{"showNotification": {"message": "User deleted", "type": "success"}}`)
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/users")
}

// ── Credential Forms ───────────────────────────────────

func (h *FormHandler) NewCredential(c *fiber.Ctx) error {
	return c.Render("credential_form", fiber.Map{
		"Title":        "New Credential",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
	}, "base")
}

func (h *FormHandler) EditCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	var credential models.Credential
	if err := h.DB.Where("id = ?", id).First(&credential).Error; err != nil {
		return c.Status(404).SendString("Credential not found")
	}

	return c.Render("credential_form", fiber.Map{
		"Title":        "Edit " + credential.Name,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"Credential":   credential,
	}, "base")
}

func (h *FormHandler) SaveCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	var credential models.Credential

	if id != "" {
		h.DB.Where("id = ?", id).First(&credential)
	}

	credential.Name = c.FormValue("name")
	credential.Username = c.FormValue("username")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return c.Status(500).SendString("Error encrypting password: " + err.Error())
		}
		credential.Password = encPass
	}

	port, _ := strconv.Atoi(c.FormValue("port"))
	if port == 0 {
		port = 22
	}
	credential.Port = port

	if err := h.DB.Save(&credential).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	c.Set("HX-Trigger", `{"showNotification": {"message": "Credential saved successfully", "type": "success"}}`)
	return c.Redirect("/settings/credentials")
}

func (h *FormHandler) DeleteCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.Credential{})

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Trigger", `{"showNotification": {"message": "Credential deleted", "type": "success"}}`)
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/credentials")
}

// ── Routine Forms ──────────────────────────────────────

func (h *FormHandler) NewRoutine(c *fiber.Ctx) error {
	return c.Render("routine_form", fiber.Map{
		"Title":        "New Routine",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
	}, "base")
}

func (h *FormHandler) EditRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	var routine models.BackupRoutine
	if err := h.DB.Where("id = ?", id).First(&routine).Error; err != nil {
		return c.Status(404).SendString("Routine not found")
	}

	return c.Render("routine_form", fiber.Map{
		"Title":        "Edit " + routine.Name,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"Routine":      routine,
	}, "base")
}

func (h *FormHandler) SaveRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	var routine models.BackupRoutine

	if id != "" {
		h.DB.Where("id = ?", id).First(&routine)
	}

	routine.Name = c.FormValue("name")
	routine.Description = c.FormValue("description")
	routine.Frequency = c.FormValue("frequency")
	routine.BackupHour = c.FormValue("backup_hour")
	routine.BackupDay = c.FormValue("backup_day")
	routine.Enabled = c.FormValue("enabled") == "on"

	if err := h.DB.Save(&routine).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	c.Set("HX-Trigger", `{"showNotification": {"message": "Routine saved successfully", "type": "success"}}`)
	return c.Redirect("/settings/routines")
}

func (h *FormHandler) DeleteRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.BackupRoutine{})

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Trigger", `{"showNotification": {"message": "Routine deleted", "type": "success"}}`)
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/routines")
}

// ── Settings Save (SFTP) ───────────────────────────────

func (h *FormHandler) SaveSettings(c *fiber.Ctx) error {
	var settings models.SftpSettings
	h.DB.First(&settings)

	settings.Host = c.FormValue("host")
	port, _ := strconv.Atoi(c.FormValue("port"))
	settings.Port = port
	settings.Username = c.FormValue("username")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return c.Status(500).SendString("Error encrypting password: " + err.Error())
		}
		settings.Password = encPass
	}

	settings.Path = c.FormValue("path")

	h.DB.Save(&settings)
	c.Set("HX-Trigger", `{"showNotification": {"message": "SFTP settings saved", "type": "success"}}`)
	return c.Redirect("/settings/sftp")
}

// ── Profile ────────────────────────────────────────────

func (h *FormHandler) SaveProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	var user models.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	user.Username = c.FormValue("username")
	user.Email = c.FormValue("email")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		user.Password = string(hash)
	}

	if err := h.DB.Save(&user).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	// Update session with new info
	sess, err := h.Store.Get(c)
	if err == nil {
		sess.Set("username", user.Username)
		sess.Set("avatar", user.Avatar)
		sess.Save()
	}

	c.Set("HX-Trigger", `{"showNotification": {"message": "Profile updated", "type": "success"}}`)
	return c.Redirect("/settings/profile")
}

// ── Export ──────────────────────────────────────────────

func (h *FormHandler) ExportBackup(c *fiber.Ctx) error {
	backupID := c.Params("backup_id")
	var backup models.NodeBackup
	if err := h.DB.Preload("Node").Where("id = ?", backupID).First(&backup).Error; err != nil {
		return c.Status(404).SendString("Backup not found")
	}

	var settings models.SftpSettings
	if err := h.DB.First(&settings).Error; err != nil {
		return c.Status(400).SendString("SFTP not configured")
	}

	if err := h.Sftp.Export(&backup, &settings); err != nil {
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Trigger", fmt.Sprintf(`{"showNotification": {"message": "Export failed: %v", "type": "error"}}`, err))
			return c.SendStatus(200)
		}
		return c.Status(500).SendString(fmt.Sprintf("Export failed: %v", err))
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Trigger", `{"showNotification": {"message": "Backup successfully exported", "type": "success"}}`)
		return c.SendStatus(200)
	}

	return c.SendString("Backup successfully exported")
}

func (h *FormHandler) PostSync(c *fiber.Ctx) error {
	var nodes []models.Node
	h.DB.Where("enabled = ?", true).Find(&nodes)

	var settings models.SftpSettings
	if err := h.DB.First(&settings).Error; err != nil {
		return c.Status(400).SendString("SFTP not configured")
	}

	successCount := 0
	for _, node := range nodes {
		var lastBackup models.NodeBackup
		err := h.DB.Where("node_id = ? AND status = ?", node.ID, "success").Order("created_at desc").First(&lastBackup).Error
		if err == nil {
			lastBackup.Node = node
			if err := h.Sftp.Export(&lastBackup, &settings); err == nil {
				successCount++
			}
		}
	}

	now := time.Now()
	settings.LastExportAt = &now
	settings.LastExportStatus = "success"
	h.DB.Save(&settings)

	c.Set("HX-Trigger", fmt.Sprintf(`{"showNotification": {"message": "Sync complete: %d nodes exported", "type": "success"}}`, successCount))
	return c.SendStatus(200)
}
