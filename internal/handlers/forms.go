package handlers

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mimic/internal/access"
	"mimic/internal/models"
	"mimic/internal/services/alert"
	"mimic/internal/services/audit"
	"mimic/internal/services/sftp"
	"mimic/pkg/crypto"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

var userMutationMu sync.Mutex

func setNotification(c *fiber.Ctx, message, kind string) {
	setNotificationWithEvents(c, message, kind)
}

func setNotificationWithEvents(c *fiber.Ctx, message, kind string, events ...string) {
	trigger := fiber.Map{
		"showNotification": fiber.Map{
			"message": message,
			"type":    kind,
		},
	}
	for _, event := range events {
		trigger[event] = fiber.Map{}
	}
	payload, err := json.Marshal(trigger)
	if err != nil {
		return
	}
	c.Set("HX-Trigger", string(payload))
}

// ── Node Forms ─────────────────────────────────────────

var supportedNodeVendors = map[string]bool{
	"cisco":    true,
	"mikrotik": true,
	"huawei":   true,
	"juniper":  true,
}

var supportedNodeFrequencies = map[string]bool{
	"1":   true,
	"6":   true,
	"12":  true,
	"24":  true,
	"168": true,
}

func normalizeCSVHeader(col string) string {
	col = strings.TrimSpace(col)
	col = strings.TrimPrefix(col, "\ufeff")
	return strings.ToLower(strings.TrimSpace(col))
}

func parseCSVBool(value string, fallback bool) (bool, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, true
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y", "on", "sim":
		return true, true
	case "false", "0", "no", "n", "off", "nao":
		return false, true
	default:
		return fallback, false
	}
}

func normalizeNodeVendor(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeNodeGroup(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "General"
	}

	parts := strings.Fields(value)
	for i, part := range parts {
		lower := strings.ToLower(part)
		if len(lower) > 0 {
			parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
		}
	}
	return strings.Join(parts, " ")
}

func normalizeNodeTags(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})

	seen := make(map[string]bool)
	tags := make([]string, 0, len(fields))
	for _, field := range fields {
		tag := strings.ToLower(strings.TrimSpace(field))
		tag = strings.Join(strings.Fields(tag), "-")
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	return strings.Join(tags, ", ")
}

func validateNodeSchedule(scheduleType, frequency, backupHour, backupDay string) error {
	if scheduleType != "individual" && scheduleType != "routine" {
		return fmt.Errorf("invalid schedule type")
	}
	if frequency != "" && !supportedNodeFrequencies[frequency] {
		return fmt.Errorf("invalid backup frequency")
	}
	if backupHour != "" {
		if _, err := time.Parse("15:04", backupHour); err != nil {
			return fmt.Errorf("invalid backup time")
		}
	}
	if backupDay != "" {
		parsedDay, err := strconv.Atoi(backupDay)
		if err != nil || parsedDay < 0 || parsedDay > 6 {
			return fmt.Errorf("invalid backup day")
		}
	}
	return nil
}

func detectCSVDelimiter(reader *bufio.Reader) rune {
	sample, err := reader.Peek(4096)
	if err != nil && len(sample) == 0 {
		return ','
	}

	firstLine := string(sample)
	if idx := strings.IndexAny(firstLine, "\r\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}

	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		return ';'
	}

	return ','
}

func renderNodeImport(c *fiber.Ctx, data fiber.Map) error {
	data["Title"] = "Import Nodes"
	data["Username"] = c.Locals("username")
	data["Avatar"] = c.Locals("avatar")
	data["Role"] = c.Locals("role")
	data["CurrentRoute"] = "nodes"
	return c.Render("node_import", data, "base")
}

func (h *FormHandler) nodeGroups() []string {
	var rawGroups []string
	h.DB.Model(&models.Node{}).
		Where("\"group\" IS NOT NULL AND \"group\" != ''").
		Distinct().
		Order("\"group\" asc").
		Pluck("group", &rawGroups)

	seen := make(map[string]bool)
	groups := make([]string, 0, len(rawGroups))
	for _, group := range rawGroups {
		normalized := normalizeNodeGroup(group)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		groups = append(groups, normalized)
	}
	return groups
}

func (h *FormHandler) NewNode(c *fiber.Ctx) error {
	var routines []models.BackupRoutine
	h.DB.Find(&routines)

	var credentials []models.Credential
	h.DB.Find(&credentials)

	var agents []models.AccessAgent
	h.DB.Find(&agents)

	groups := h.nodeGroups()

	return c.Render("node_form", fiber.Map{
		"Title":        "New Node",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "nodes",
		"Routines":     routines,
		"Credentials":  credentials,
		"Agents":       agents,
		"Groups":       groups,
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

	groups := h.nodeGroups()

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
		"Groups":       groups,
	}, "base")
}

func (h *FormHandler) SaveNode(c *fiber.Ctx) error {
	id := c.Params("id")
	var node models.Node

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&node).Error; err != nil {
			return c.Status(404).SendString("Node not found")
		}
	}

	node.Name = strings.TrimSpace(c.FormValue("name"))
	node.IP = strings.TrimSpace(c.FormValue("ip"))
	node.Vendor = normalizeNodeVendor(c.FormValue("vendor"))
	node.Username = strings.TrimSpace(c.FormValue("username"))

	if node.Name == "" || node.IP == "" {
		return c.Status(400).SendString("Node name and IP are required")
	}
	if !supportedNodeVendors[node.Vendor] {
		return c.Status(400).SendString("Invalid node vendor")
	}

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return c.Status(500).SendString("Error encrypting password: " + err.Error())
		}
		node.Password = encPass
	}

	port, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("port")))
	if port < 1 || port > 65535 {
		port = 22
	}
	node.Port = port
	node.Group = normalizeNodeGroup(c.FormValue("group"))
	node.Tags = normalizeNodeTags(c.FormValue("tags"))
	node.ScheduleType = strings.ToLower(strings.TrimSpace(c.FormValue("schedule_type")))
	if node.ScheduleType == "" {
		node.ScheduleType = "individual"
	}
	node.Frequency = strings.TrimSpace(c.FormValue("frequency"))
	if node.Frequency == "" {
		node.Frequency = "24"
	}
	node.BackupHour = strings.TrimSpace(c.FormValue("backup_hour"))
	node.BackupDay = strings.TrimSpace(c.FormValue("backup_day"))

	if err := validateNodeSchedule(node.ScheduleType, node.Frequency, node.BackupHour, node.BackupDay); err != nil {
		return c.Status(400).SendString(err.Error())
	}

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
		log.Printf("[Node] Failed to save node: %v", err)
		return c.Status(500).SendString("Could not save node")
	}
	writeAuditLog(h.DB, c, "success", "node", "Node saved", "node="+node.Name)

	setNotification(c, "Node saved successfully", "success")
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
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, target := range []any{&models.NodeBackup{}, &models.SecurityViolation{}, &models.NodeRuleException{}} {
			if err := tx.Where("node_id = ?", id).Delete(target).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).Delete(&models.Node{}).Error
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete node")
	}
	writeAuditLog(h.DB, c, "warning", "node", "Node deleted", "node_id="+id)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Node deleted", "success")
		return c.SendString("")
	}

	return c.Redirect("/nodes")
}

// ── Node Import/Export ─────────────────────────────────

func (h *FormHandler) ImportNodesForm(c *fiber.Ctx) error {
	return renderNodeImport(c, fiber.Map{})
}

func (h *FormHandler) ImportNodesCSV(c *fiber.Ctx) error {
	file, err := c.FormFile("csv_file")
	if err != nil {
		return renderNodeImport(c, fiber.Map{"Error": "No file selected."})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).SendString("Error opening file")
	}
	defer f.Close()

	bufferedFile := bufio.NewReader(f)
	reader := csv.NewReader(bufferedFile)
	reader.Comma = detectCSVDelimiter(bufferedFile)
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header row
	header, err := reader.Read()
	if err != nil {
		return renderNodeImport(c, fiber.Map{"Error": "Empty or invalid CSV file."})
	}

	// Map header columns to indices
	colMap := make(map[string]int)
	for i, col := range header {
		normalized := normalizeCSVHeader(col)
		if normalized != "" {
			colMap[normalized] = i
		}
	}

	// Validate required columns
	requiredCols := []string{"name", "ip", "vendor"}
	for _, col := range requiredCols {
		if _, ok := colMap[col]; !ok {
			return renderNodeImport(c, fiber.Map{"Error": fmt.Sprintf("Required column '%s' not found in CSV.", col)})
		}
	}

	successCount := 0
	errorCount := 0
	warningCount := 0
	rowCount := 0
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
			errors = append(errors, fmt.Sprintf("Line %d: read error: %v", lineNum, err))
			continue
		}

		getCol := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx])
			}
			return ""
		}

		blankRow := true
		for _, col := range record {
			if strings.TrimSpace(col) != "" {
				blankRow = false
				break
			}
		}
		if blankRow {
			continue
		}
		rowCount++

		nodeName := getCol("name")
		nodeIP := getCol("ip")
		nodeVendor := normalizeNodeVendor(getCol("vendor"))

		if nodeName == "" || nodeIP == "" || nodeVendor == "" {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d: name, vendor and ip are required.", lineNum))
			continue
		}

		if !supportedNodeVendors[nodeVendor] {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): unsupported vendor '%s'. Use cisco, mikrotik, huawei or juniper.", lineNum, nodeName, getCol("vendor")))
			continue
		}

		port := 22
		if rawPort := getCol("port"); rawPort != "" {
			parsedPort, err := strconv.Atoi(rawPort)
			if err != nil || parsedPort < 1 || parsedPort > 65535 {
				errorCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): invalid SSH port '%s'.", lineNum, nodeName, rawPort))
				continue
			}
			port = parsedPort
		}
		if port == 0 {
			port = 22
		}

		scheduleType := strings.ToLower(getCol("schedule_type"))
		if scheduleType == "" {
			scheduleType = "individual"
		}
		if scheduleType != "individual" && scheduleType != "routine" {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): invalid schedule_type '%s'. Use individual or routine.", lineNum, nodeName, getCol("schedule_type")))
			continue
		}

		frequency := getCol("frequency")
		if frequency == "" {
			frequency = "24"
		}
		if !supportedNodeFrequencies[frequency] {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): invalid frequency '%s'. Use 1, 6, 12, 24 or 168.", lineNum, nodeName, frequency))
			continue
		}

		backupHour := getCol("backup_hour")
		if backupHour != "" {
			if _, err := time.Parse("15:04", backupHour); err != nil {
				errorCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): invalid backup_hour '%s'. Use HH:MM.", lineNum, nodeName, backupHour))
				continue
			}
		}

		backupDay := getCol("backup_day")
		if backupDay != "" {
			parsedDay, err := strconv.Atoi(backupDay)
			if err != nil || parsedDay < 0 || parsedDay > 6 {
				errorCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): invalid backup_day '%s'. Use 0-6.", lineNum, nodeName, backupDay))
				continue
			}
		}

		group := normalizeNodeGroup(getCol("group"))

		enabled, ok := parseCSVBool(getCol("enabled"), true)
		if !ok {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): invalid enabled value '%s'. Use true/false, yes/no or 1/0.", lineNum, nodeName, getCol("enabled")))
			continue
		}

		node := models.Node{
			Name:         nodeName,
			Vendor:       nodeVendor,
			IP:           nodeIP,
			Port:         port,
			Username:     getCol("username"),
			Group:        group,
			Tags:         normalizeNodeTags(getCol("tags")),
			ScheduleType: scheduleType,
			Frequency:    frequency,
			BackupHour:   backupHour,
			BackupDay:    backupDay,
			Enabled:      enabled,
		}

		// Encrypt password if provided
		rawPass := getCol("password")
		if rawPass != "" {
			encPass, err := crypto.Encrypt(rawPass)
			if err != nil {
				errorCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): failed to encrypt password: %v", lineNum, nodeName, err))
				continue
			}
			node.Password = encPass
		}

		// Resolve Foreign Keys by Name
		credName := getCol("credential_name")
		if credName != "" {
			var cred models.Credential
			if err := h.DB.Where("LOWER(name) = ?", strings.ToLower(credName)).First(&cred).Error; err == nil {
				node.CredentialID = &cred.ID
			} else {
				warningCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): Credential '%s' not found, imported without credential.", lineNum, nodeName, credName))
			}
		}

		routineName := getCol("routine_name")
		if scheduleType == "routine" || routineName != "" {
			if routineName == "" {
				warningCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): schedule_type is routine but routine_name is empty, falling back to individual schedule.", lineNum, nodeName))
				node.ScheduleType = "individual"
			} else {
				var routine models.BackupRoutine
				if err := h.DB.Where("LOWER(name) = ?", strings.ToLower(routineName)).First(&routine).Error; err == nil {
					node.RoutineID = &routine.ID
					node.ScheduleType = "routine"
				} else {
					warningCount++
					errors = append(errors, fmt.Sprintf("Line %d (%s): Routine '%s' not found, falling back to individual schedule.", lineNum, nodeName, routineName))
					node.ScheduleType = "individual"
				}
			}
		}

		agentName := getCol("agent_name")
		if agentName != "" {
			var agent models.AccessAgent
			if err := h.DB.Where("LOWER(name) = ?", strings.ToLower(agentName)).First(&agent).Error; err == nil {
				node.AccessAgentID = &agent.ID
			} else {
				warningCount++
				errors = append(errors, fmt.Sprintf("Line %d (%s): Access Agent '%s' not found.", lineNum, nodeName, agentName))
			}
		}

		if err := h.DB.Create(&node).Error; err != nil {
			errorCount++
			errors = append(errors, fmt.Sprintf("Line %d (%s): %v", lineNum, nodeName, err))
			continue
		}

		successCount++
	}

	issueCount := errorCount + warningCount
	if rowCount == 0 {
		return renderNodeImport(c, fiber.Map{"Error": "No nodes found in CSV file."})
	}

	if successCount == 0 {
		return renderNodeImport(c, fiber.Map{
			"Error":      "No nodes were imported. Review the row errors below.",
			"ErrorCount": issueCount,
			"Errors":     errors,
		})
	}

	return renderNodeImport(c, fiber.Map{
		"Success":    fmt.Sprintf("%d nodes successfully imported.", successCount),
		"ErrorCount": issueCount,
		"Errors":     errors,
	})
}

// ── User Forms ─────────────────────────────────────────

func (h *FormHandler) renderUserForm(c *fiber.Ctx, user models.User, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	title := "New User"
	if user.ID != 0 {
		title = "Edit " + user.Username
	}

	var adminCount int64
	h.DB.Model(&models.User{}).Where("role = ?", "Administrator").Count(&adminCount)

	return c.Render("user_form", fiber.Map{
		"Title":        title,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "users",
		"ActiveTab":    "users",
		"EditUser":     user,
		"IsEdit":       user.ID != 0,
		"IsSelf":       fmt.Sprintf("%v", user.ID) == fmt.Sprintf("%v", c.Locals("user_id")),
		"AdminCount":   adminCount,
		"Error":        errMsg,
	}, "base")
}

func (h *FormHandler) NewUser(c *fiber.Ctx) error {
	return h.renderUserForm(c, models.User{Role: access.RoleViewer}, "", 0)
}

func (h *FormHandler) EditUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var user models.User
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}

	return h.renderUserForm(c, user, "", 0)
}

func (h *FormHandler) SaveUser(c *fiber.Ctx) error {
	userMutationMu.Lock()
	defer userMutationMu.Unlock()
	id := c.Params("id")
	var user models.User

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
			return c.Status(404).SendString("User not found")
		}
	}

	user.Username = strings.TrimSpace(c.FormValue("username"))
	user.Email = strings.TrimSpace(c.FormValue("email"))
	user.Role = strings.TrimSpace(c.FormValue("role"))
	if user.Role == "" {
		user.Role = "Viewer"
	}

	if user.Username == "" {
		return h.renderUserForm(c, user, "Username is required.", fiber.StatusBadRequest)
	}
	if !access.ValidRole(user.Role) {
		return h.renderUserForm(c, user, "Invalid user role.", fiber.StatusBadRequest)
	}
	if strings.ContainsAny(user.Username, " \t\r\n") {
		return h.renderUserForm(c, user, "Username cannot contain spaces.", fiber.StatusBadRequest)
	}
	if user.Email != "" {
		if _, err := mail.ParseAddress(user.Email); err != nil {
			return h.renderUserForm(c, user, "Enter a valid email address.", fiber.StatusBadRequest)
		}
	}

	var duplicateCount int64
	duplicateQuery := h.DB.Model(&models.User{}).Where("LOWER(username) = ?", strings.ToLower(user.Username))
	if user.ID != 0 {
		duplicateQuery = duplicateQuery.Where("id <> ?", user.ID)
	}
	duplicateQuery.Count(&duplicateCount)
	if duplicateCount > 0 {
		return h.renderUserForm(c, user, "A user with this username already exists.", fiber.StatusBadRequest)
	}

	rawPass := c.FormValue("password")
	if rawPass == "" && user.Password == "" {
		return h.renderUserForm(c, user, "Password is required for new users.", fiber.StatusBadRequest)
	}
	if rawPass != "" {
		if len(rawPass) < 8 {
			return h.renderUserForm(c, user, "Password must be at least 8 characters long.", fiber.StatusBadRequest)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		if err != nil {
			return h.renderUserForm(c, user, "Error hashing password.", fiber.StatusInternalServerError)
		}
		user.Password = string(hash)
	}

	isSelf := fmt.Sprintf("%v", user.ID) == fmt.Sprintf("%v", c.Locals("user_id"))
	if isSelf && user.Role != access.RoleAdministrator {
		return h.renderUserForm(c, user, "You cannot remove your own administrator access.", fiber.StatusBadRequest)
	}

	if user.ID != 0 && user.Role != access.RoleAdministrator {
		var existing models.User
		if err := h.DB.Select("id", "role").First(&existing, user.ID).Error; err == nil && existing.Role == access.RoleAdministrator {
			var otherAdmins int64
			h.DB.Model(&models.User{}).Where("role = ? AND id <> ?", access.RoleAdministrator, user.ID).Count(&otherAdmins)
			if otherAdmins == 0 {
				return h.renderUserForm(c, user, "At least one administrator must remain.", fiber.StatusBadRequest)
			}
		}
	}

	if err := h.DB.Save(&user).Error; err != nil {
		log.Printf("[User] Failed to save user: %v", err)
		return h.renderUserForm(c, user, "Could not save user.", fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "success", "user", "User saved", "username="+user.Username+" role="+user.Role)

	if isSelf {
		sess, err := h.Store.Get(c)
		if err == nil {
			sess.Set("username", user.Username)
			sess.Set("role", user.Role)
			sess.Set("avatar", user.Avatar)
			if err := sess.Save(); err != nil {
				return h.renderUserForm(c, user, "User saved, but the session could not be updated.", fiber.StatusInternalServerError)
			}
		}
	}

	setNotification(c, "User saved successfully", "success")
	return c.Redirect("/settings/users")
}

func (h *FormHandler) DeleteUser(c *fiber.Ctx) error {
	userMutationMu.Lock()
	defer userMutationMu.Unlock()
	id := c.Params("id")
	if c.Locals("user_id") != nil {
		localId := fmt.Sprintf("%v", c.Locals("user_id"))
		if id == localId {
			if c.Get("HX-Request") == "true" {
				setNotification(c, "You cannot delete your own user.", "error")
				c.Set("HX-Reswap", "none")
				return c.SendString("")
			}
			return c.Status(400).SendString("Cannot delete your own user")
		}
	}

	var user models.User
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "User not found", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(404).SendString("User not found")
	}

	if user.Role == access.RoleAdministrator {
		var adminCount int64
		h.DB.Model(&models.User{}).Where("role = ?", access.RoleAdministrator).Count(&adminCount)
		if adminCount <= 1 {
			if c.Get("HX-Request") == "true" {
				setNotification(c, "At least one administrator must remain.", "error")
				c.Set("HX-Reswap", "none")
				return c.SendString("")
			}
			return c.Status(fiber.StatusConflict).SendString("At least one administrator must remain")
		}
	}

	if err := h.DB.Delete(&user).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Could not delete user", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(500).SendString("Could not delete user")
	}
	writeAuditLog(h.DB, c, "warning", "user", "User deleted", "username="+user.Username)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "User deleted", "success")
		return c.SendString("")
	}

	return c.Redirect("/settings/users")
}

// ── Credential Forms ───────────────────────────────────

func (h *FormHandler) renderCredentialForm(c *fiber.Ctx, credential models.Credential, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	if credential.ID != 0 {
		h.DB.Where("credential_id = ?", credential.ID).Order("name asc").Find(&credential.Nodes)
	}

	title := "New Credential"
	if credential.ID != 0 {
		title = "Edit " + credential.Name
	}

	return c.Render("credential_form", fiber.Map{
		"Title":        title,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "credentials",
		"ActiveTab":    "credentials",
		"Credential":   credential,
		"IsEdit":       credential.ID != 0,
		"Error":        errMsg,
	}, "base")
}

func (h *FormHandler) NewCredential(c *fiber.Ctx) error {
	return h.renderCredentialForm(c, models.Credential{Port: 22}, "", 0)
}

func (h *FormHandler) EditCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	var credential models.Credential
	if err := h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB {
		return db.Order("name asc")
	}).Where("id = ?", id).First(&credential).Error; err != nil {
		return c.Status(404).SendString("Credential not found")
	}

	return h.renderCredentialForm(c, credential, "", 0)
}

func (h *FormHandler) SaveCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	var credential models.Credential

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&credential).Error; err != nil {
			return c.Status(404).SendString("Credential not found")
		}
	}

	credential.Name = strings.TrimSpace(c.FormValue("name"))
	credential.Username = strings.TrimSpace(c.FormValue("username"))

	if credential.Name == "" {
		return h.renderCredentialForm(c, credential, "Credential name is required.", fiber.StatusBadRequest)
	}
	if credential.Username == "" {
		return h.renderCredentialForm(c, credential, "Username is required.", fiber.StatusBadRequest)
	}

	var duplicateCount int64
	duplicateQuery := h.DB.Model(&models.Credential{}).Where("LOWER(name) = ?", strings.ToLower(credential.Name))
	if credential.ID != 0 {
		duplicateQuery = duplicateQuery.Where("id <> ?", credential.ID)
	}
	duplicateQuery.Count(&duplicateCount)
	if duplicateCount > 0 {
		return h.renderCredentialForm(c, credential, "A credential with this name already exists.", fiber.StatusBadRequest)
	}

	rawPass := c.FormValue("password")
	if rawPass == "" && credential.Password == "" {
		return h.renderCredentialForm(c, credential, "Password is required for new credentials.", fiber.StatusBadRequest)
	}
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return h.renderCredentialForm(c, credential, "Error encrypting password: "+err.Error(), fiber.StatusInternalServerError)
		}
		credential.Password = encPass
	}

	port := 22
	rawPort := strings.TrimSpace(c.FormValue("port"))
	if rawPort != "" {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return h.renderCredentialForm(c, credential, "SSH port must be a number between 1 and 65535.", fiber.StatusBadRequest)
		}
		port = parsedPort
	}
	credential.Port = port

	if err := h.DB.Save(&credential).Error; err != nil {
		log.Printf("[Credential] Failed to save credential: %v", err)
		return h.renderCredentialForm(c, credential, "Could not save credential.", fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "success", "credential", "SSH credential saved", "name="+credential.Name)

	setNotification(c, "Credential saved successfully", "success")
	return c.Redirect("/settings/credentials")
}

func (h *FormHandler) DeleteCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	var credential models.Credential
	if err := h.DB.Where("id = ?", id).First(&credential).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Credential not found", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(404).SendString("Credential not found")
	}

	var linkedNodes int64
	h.DB.Model(&models.Node{}).Where("credential_id = ?", credential.ID).Count(&linkedNodes)
	if linkedNodes > 0 {
		message := fmt.Sprintf("Credential is linked to %d node(s). Remove it from nodes before deleting.", linkedNodes)
		if c.Get("HX-Request") == "true" {
			setNotification(c, message, "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(fiber.StatusConflict).SendString(message)
	}

	if err := h.DB.Delete(&credential).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Could not delete credential", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(500).SendString("Could not delete credential")
	}
	writeAuditLog(h.DB, c, "warning", "credential", "SSH credential deleted", "name="+credential.Name)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Credential deleted", "success")
		return c.SendString("")
	}

	return c.Redirect("/settings/credentials")
}

// ── Routine Forms ──────────────────────────────────────

func (h *FormHandler) renderRoutineForm(c *fiber.Ctx, routine models.BackupRoutine, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	if routine.ID != 0 {
		h.DB.Where("routine_id = ?", routine.ID).Order("name asc").Find(&routine.Nodes)
	}

	title := "New Routine"
	if routine.ID != 0 {
		title = "Edit " + routine.Name
	}

	return c.Render("routine_form", fiber.Map{
		"Title":        title,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "routines",
		"ActiveTab":    "routines",
		"Routine":      routine,
		"IsEdit":       routine.ID != 0,
		"Error":        errMsg,
	}, "base")
}

func (h *FormHandler) NewRoutine(c *fiber.Ctx) error {
	return h.renderRoutineForm(c, models.BackupRoutine{Frequency: "24", BackupHour: "02:00", Enabled: true}, "", 0)
}

func (h *FormHandler) EditRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	var routine models.BackupRoutine
	if err := h.DB.Preload("Nodes", func(db *gorm.DB) *gorm.DB {
		return db.Order("name asc")
	}).Where("id = ?", id).First(&routine).Error; err != nil {
		return c.Status(404).SendString("Routine not found")
	}

	return h.renderRoutineForm(c, routine, "", 0)
}

func (h *FormHandler) SaveRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	var routine models.BackupRoutine

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&routine).Error; err != nil {
			return c.Status(404).SendString("Routine not found")
		}
	}

	routine.Name = strings.TrimSpace(c.FormValue("name"))
	routine.Description = strings.TrimSpace(c.FormValue("description"))
	routine.Frequency = strings.TrimSpace(c.FormValue("frequency"))
	if routine.Frequency == "" {
		routine.Frequency = "24"
	}
	routine.BackupHour = strings.TrimSpace(c.FormValue("backup_hour"))
	routine.BackupDay = strings.TrimSpace(c.FormValue("backup_day"))
	routine.Enabled = c.FormValue("enabled") == "on"

	if routine.Name == "" {
		return h.renderRoutineForm(c, routine, "Routine name is required.", fiber.StatusBadRequest)
	}
	if !supportedNodeFrequencies[routine.Frequency] {
		return h.renderRoutineForm(c, routine, "Invalid backup frequency.", fiber.StatusBadRequest)
	}
	if routine.BackupHour != "" {
		if _, err := time.Parse("15:04", routine.BackupHour); err != nil {
			return h.renderRoutineForm(c, routine, "Backup time must use HH:MM format.", fiber.StatusBadRequest)
		}
	}
	if routine.BackupDay != "" {
		parsedDay, err := strconv.Atoi(routine.BackupDay)
		if err != nil || parsedDay < 0 || parsedDay > 6 {
			return h.renderRoutineForm(c, routine, "Backup day must be between Monday and Sunday.", fiber.StatusBadRequest)
		}
	}

	var duplicateCount int64
	duplicateQuery := h.DB.Model(&models.BackupRoutine{}).Where("LOWER(name) = ?", strings.ToLower(routine.Name))
	if routine.ID != 0 {
		duplicateQuery = duplicateQuery.Where("id <> ?", routine.ID)
	}
	duplicateQuery.Count(&duplicateCount)
	if duplicateCount > 0 {
		return h.renderRoutineForm(c, routine, "A routine with this name already exists.", fiber.StatusBadRequest)
	}

	if err := h.DB.Save(&routine).Error; err != nil {
		log.Printf("[Routine] Failed to save routine: %v", err)
		return h.renderRoutineForm(c, routine, "Could not save routine.", fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "success", "routine", "Backup routine saved", "name="+routine.Name)

	setNotification(c, "Routine saved successfully", "success")
	return c.Redirect("/settings/routines")
}

func (h *FormHandler) DeleteRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	var routine models.BackupRoutine
	if err := h.DB.Where("id = ?", id).First(&routine).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Routine not found", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(404).SendString("Routine not found")
	}

	var linkedNodes int64
	h.DB.Model(&models.Node{}).Where("routine_id = ?", routine.ID).Count(&linkedNodes)
	if linkedNodes > 0 {
		message := fmt.Sprintf("Routine is linked to %d node(s). Move those nodes before deleting.", linkedNodes)
		if c.Get("HX-Request") == "true" {
			setNotification(c, message, "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(fiber.StatusConflict).SendString(message)
	}

	if err := h.DB.Delete(&routine).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Could not delete routine", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(500).SendString("Could not delete routine")
	}
	writeAuditLog(h.DB, c, "warning", "routine", "Backup routine deleted", "name="+routine.Name)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Routine deleted", "success")
		return c.SendString("")
	}

	return c.Redirect("/settings/routines")
}

// ── Settings Save (SFTP) ───────────────────────────────

func normalizeSFTPPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}

func (h *FormHandler) sftpStatusStats(settings models.SftpSettings) SftpStatusStats {
	stats := SftpStatusStats{
		Enabled:     settings.Enabled,
		HasPassword: strings.TrimSpace(settings.Password) != "",
		LastStatus:  settings.LastExportStatus,
	}
	if stats.LastStatus == "" {
		stats.LastStatus = "never"
	}
	stats.Configured = strings.TrimSpace(settings.Host) != "" && strings.TrimSpace(settings.Username) != "" && stats.HasPassword
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", false).Count(&stats.PendingBackups)
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", false).Distinct("node_id").Count(&stats.PendingNodes)
	h.DB.Model(&models.NodeBackup{}).Where("status = ? AND exported = ?", "success", true).Count(&stats.ExportedBackups)
	return stats
}

func (h *FormHandler) renderSFTPSettings(c *fiber.Ctx, settings models.SftpSettings, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	return c.Render("settings", fiber.Map{
		"Title":        "SFTP Configuration",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "sftp",
		"ActiveTab":    "sftp",
		"Sftp":         settings,
		"SftpStats":    h.sftpStatusStats(settings),
		"Error":        errMsg,
	}, "base")
}

func validateSFTPSettings(settings models.SftpSettings, requireConnection bool) error {
	if settings.Port < 1 || settings.Port > 65535 {
		return fmt.Errorf("SFTP port must be between 1 and 65535")
	}
	if settings.SyncTime != "" {
		if _, err := time.Parse("15:04", settings.SyncTime); err != nil {
			return fmt.Errorf("sync time must use HH:MM format")
		}
	}
	if requireConnection || settings.Enabled {
		if strings.TrimSpace(settings.Host) == "" {
			return fmt.Errorf("SFTP host is required")
		}
		if strings.ContainsAny(settings.Host, " \t\r\n") {
			return fmt.Errorf("SFTP host cannot contain spaces")
		}
		if strings.TrimSpace(settings.Username) == "" {
			return fmt.Errorf("SFTP username is required")
		}
		if strings.TrimSpace(settings.Password) == "" {
			return fmt.Errorf("SFTP password is required")
		}
	}
	return nil
}

func (h *FormHandler) SaveSettings(c *fiber.Ctx) error {
	var settings models.SftpSettings
	if err := h.DB.First(&settings).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not load SFTP settings")
	}
	previousHost := settings.Host
	previousPort := settings.Port

	settings.Host = strings.TrimSpace(c.FormValue("host"))
	port, err := strconv.Atoi(strings.TrimSpace(c.FormValue("port")))
	if err != nil || port == 0 {
		port = 22
	}
	settings.Port = port
	if !strings.EqualFold(previousHost, settings.Host) || previousPort != settings.Port {
		settings.HostFingerprint = ""
	}
	settings.Username = strings.TrimSpace(c.FormValue("username"))

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			return h.renderSFTPSettings(c, settings, "Error encrypting password: "+err.Error(), fiber.StatusInternalServerError)
		}
		settings.Password = encPass
	}

	settings.Path = normalizeSFTPPath(c.FormValue("path"))
	settings.Enabled = c.FormValue("enabled") == "on"
	settings.SyncTime = strings.TrimSpace(c.FormValue("sync_time"))
	if settings.SyncTime == "" {
		settings.SyncTime = "23:00"
	}

	if err := validateSFTPSettings(settings, false); err != nil {
		return h.renderSFTPSettings(c, settings, err.Error(), fiber.StatusBadRequest)
	}

	if err := h.DB.Save(&settings).Error; err != nil {
		log.Printf("[SFTP] Failed to save settings: %v", err)
		return h.renderSFTPSettings(c, settings, "Could not save SFTP settings.", fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "success", "system", "SFTP settings updated", "host="+settings.Host)
	setNotification(c, "SFTP settings saved", "success")
	return c.Redirect("/settings/sftp")
}

func (h *FormHandler) TestSFTPConnection(c *fiber.Ctx) error {
	var settings models.SftpSettings

	// We populate from form, but fallback to DB for password if empty
	if err := h.DB.First(&settings).Error; err != nil {
		setNotification(c, "Connection failed: could not load SFTP settings", "error")
		return c.SendStatus(fiber.StatusOK)
	}
	previousHost := settings.Host
	previousPort := settings.Port

	settings.Host = strings.TrimSpace(c.FormValue("host"))
	port, err := strconv.Atoi(strings.TrimSpace(c.FormValue("port")))
	if err != nil || port == 0 {
		port = 22
	}
	settings.Port = port
	if !strings.EqualFold(previousHost, settings.Host) || previousPort != settings.Port {
		setNotification(c, "Save the SFTP server address before testing it", "warning")
		return c.SendStatus(fiber.StatusOK)
	}
	settings.Username = strings.TrimSpace(c.FormValue("username"))
	settings.Path = normalizeSFTPPath(c.FormValue("path"))
	settings.SyncTime = strings.TrimSpace(c.FormValue("sync_time"))

	rawPass := c.FormValue("password")
	if rawPass != "" {
		settings.Password, err = crypto.Encrypt(rawPass)
		if err != nil {
			setNotification(c, "Connection failed: could not protect the supplied password", "error")
			return c.SendStatus(fiber.StatusOK)
		}
	}

	if err := validateSFTPSettings(settings, true); err != nil {
		setNotification(c, "Connection failed: "+err.Error(), "error")
		return c.SendStatus(200)
	}

	err = h.Sftp.TestConnection(&settings)
	if err != nil {
		setNotification(c, fmt.Sprintf("Connection failed: %v", err), "error")
		return c.SendStatus(200)
	}

	setNotification(c, "Connection successful! Directory verified.", "success")
	return c.SendStatus(200)
}

// ── Profile ────────────────────────────────────────────

const maxAvatarSize = 2 * 1024 * 1024

type avatarUpload struct {
	data []byte
	ext  string
}

func readAvatarUpload(c *fiber.Ctx) (*avatarUpload, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, fmt.Errorf("invalid profile form")
	}
	files := form.File["avatar"]
	if len(files) == 0 || files[0].Size == 0 {
		return nil, nil
	}
	fileHeader := files[0]
	if fileHeader.Size > maxAvatarSize {
		return nil, fmt.Errorf("profile photo must be 2 MB or smaller")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("could not read the profile photo")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAvatarSize+1))
	if err != nil || len(data) > maxAvatarSize {
		return nil, fmt.Errorf("could not read the profile photo")
	}

	var ext string
	switch http.DetectContentType(data) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	default:
		return nil, fmt.Errorf("profile photo must be a JPEG or PNG image")
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 32 || config.Height < 32 || config.Width > 4096 || config.Height > 4096 {
		return nil, fmt.Errorf("profile photo dimensions must be between 32 and 4096 pixels")
	}

	return &avatarUpload{data: data, ext: ext}, nil
}

func storeAvatar(userID uint, upload *avatarUpload) (string, error) {
	directory := filepath.Join("static", "uploads", "avatars")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("could not prepare profile photo storage")
	}
	filename := fmt.Sprintf("user-%d-%d%s", userID, time.Now().UnixNano(), upload.ext)
	if err := os.WriteFile(filepath.Join(directory, filename), upload.data, 0o644); err != nil {
		return "", fmt.Errorf("could not save the profile photo")
	}
	return "/static/uploads/avatars/" + filename, nil
}

func removeStoredAvatar(avatarPath string) {
	const prefix = "/static/uploads/avatars/"
	if !strings.HasPrefix(avatarPath, prefix) {
		return
	}
	filename := filepath.Base(strings.TrimPrefix(avatarPath, prefix))
	if filename == "." || filename == "" {
		return
	}
	if err := os.Remove(filepath.Join("static", "uploads", "avatars", filename)); err != nil && !os.IsNotExist(err) {
		// Avatar cleanup is best-effort and must not roll back a valid profile update.
		return
	}
}

func (h *FormHandler) renderProfileForm(c *fiber.Ctx, user models.User, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}
	role, _ := c.Locals("role").(string)
	return c.Render("settings", fiber.Map{
		"Title":               "My Profile",
		"Username":            c.Locals("username"),
		"Avatar":              c.Locals("avatar"),
		"Role":                role,
		"CurrentRoute":        "profile",
		"ActiveTab":           "profile",
		"ProfileUser":         user,
		"Error":               errMsg,
		"CanManageUsers":      access.Allows(role, access.ManageUsers),
		"CanManageOperations": access.Allows(role, access.ManageOperations),
		"CanManagePolicies":   access.Allows(role, access.ManagePolicies),
		"CanManageSystem":     access.Allows(role, access.ManageSystem),
		"CanExportBackups":    access.Allows(role, access.ExportBackups),
		"CanViewAudit":        access.Allows(role, access.ViewAudit),
	}, "base")
}

func (h *FormHandler) SaveProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	var user models.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		return c.Status(404).SendString("User not found")
	}
	oldAvatar := user.Avatar
	newAvatar := ""

	user.Username = strings.TrimSpace(c.FormValue("username"))
	user.Email = strings.TrimSpace(c.FormValue("email"))
	if user.Username == "" {
		return h.renderProfileForm(c, user, "Username is required.", fiber.StatusBadRequest)
	}
	if strings.ContainsAny(user.Username, " \t\r\n") {
		return h.renderProfileForm(c, user, "Username cannot contain spaces.", fiber.StatusBadRequest)
	}
	if user.Email != "" {
		if _, err := mail.ParseAddress(user.Email); err != nil {
			return h.renderProfileForm(c, user, "Enter a valid email address.", fiber.StatusBadRequest)
		}
	}

	var duplicateCount int64
	h.DB.Model(&models.User{}).
		Where("LOWER(username) = ? AND id <> ?", strings.ToLower(user.Username), user.ID).
		Count(&duplicateCount)
	if duplicateCount > 0 {
		return h.renderProfileForm(c, user, "A user with this username already exists.", fiber.StatusConflict)
	}

	upload, err := readAvatarUpload(c)
	if err != nil {
		return h.renderProfileForm(c, user, err.Error(), fiber.StatusBadRequest)
	}
	if c.FormValue("remove_avatar") == "1" {
		user.Avatar = ""
	}
	if upload != nil {
		avatarPath, err := storeAvatar(user.ID, upload)
		if err != nil {
			return h.renderProfileForm(c, user, err.Error(), fiber.StatusInternalServerError)
		}
		user.Avatar = avatarPath
		newAvatar = avatarPath
	}

	rawPass := c.FormValue("password")
	if rawPass != "" {
		if len(rawPass) < 8 {
			removeStoredAvatar(newAvatar)
			return h.renderProfileForm(c, user, "New password must be at least 8 characters long.", fiber.StatusBadRequest)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		if err != nil {
			removeStoredAvatar(newAvatar)
			return h.renderProfileForm(c, user, "Error hashing password.", fiber.StatusInternalServerError)
		}
		user.Password = string(hash)
	}

	if err := h.DB.Save(&user).Error; err != nil {
		removeStoredAvatar(newAvatar)
		return h.renderProfileForm(c, user, "Could not save the profile.", fiber.StatusInternalServerError)
	}
	if oldAvatar != "" && oldAvatar != user.Avatar {
		removeStoredAvatar(oldAvatar)
	}
	writeAuditLog(h.DB, c, "success", "user", "Profile updated", "username="+user.Username)

	// Update session with new info
	sess, err := h.Store.Get(c)
	if err == nil {
		sess.Set("username", user.Username)
		sess.Set("avatar", user.Avatar)
		if err := sess.Save(); err != nil {
			return c.Status(500).SendString("Error saving session")
		}
	}

	setNotification(c, "Profile updated", "success")
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
	if err := validateSFTPSettings(settings, true); err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Export failed: "+err.Error(), "error")
			return c.SendStatus(200)
		}
		return c.Status(400).SendString(err.Error())
	}

	if err := h.Sftp.Export(&backup, &settings); err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, fmt.Sprintf("Export failed: %v", err), "error")
			return c.SendStatus(200)
		}
		return c.Status(500).SendString(fmt.Sprintf("Export failed: %v", err))
	}

	backup.Exported = true
	if err := h.DB.Save(&backup).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Backup exported, but its state could not be saved")
	}
	writeAuditLog(h.DB, c, "success", "export", "Backup exported", fmt.Sprintf("backup_id=%d node=%s", backup.ID, backup.Node.Name))

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Backup successfully exported", "success")
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
	if err := validateSFTPSettings(settings, true); err != nil {
		setNotification(c, "Sync failed: "+err.Error(), "error")
		return c.SendStatus(200)
	}

	successCount := 0
	failureCount := 0
	lastErr := ""
	for _, node := range nodes {
		var lastBackup models.NodeBackup
		err := h.DB.Where("node_id = ? AND status = ?", node.ID, "success").Order("created_at desc").First(&lastBackup).Error
		if err == nil {
			lastBackup.Node = node
			if err := h.Sftp.Export(&lastBackup, &settings); err == nil {
				lastBackup.Exported = true
				h.DB.Save(&lastBackup)
				successCount++
			} else {
				failureCount++
				lastErr = err.Error()
			}
		}
	}

	now := time.Now()
	settings.LastExportAt = &now
	if failureCount == 0 {
		settings.LastExportStatus = "success"
		settings.LastExportError = ""
	} else if successCount > 0 {
		settings.LastExportStatus = "partial"
		settings.LastExportError = lastErr
	} else {
		settings.LastExportStatus = "error"
		settings.LastExportError = lastErr
	}
	if err := h.DB.Save(&settings).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Sync completed, but its state could not be saved")
	}
	writeAuditLog(h.DB, c, "success", "export", "Manual SFTP sync completed", fmt.Sprintf("exported=%d failed=%d", successCount, failureCount))

	if failureCount > 0 {
		setNotificationWithEvents(c, fmt.Sprintf("Sync partial: %d exported, %d failed", successCount, failureCount), "warning", "exportSyncCompleted")
	} else {
		setNotificationWithEvents(c, fmt.Sprintf("Sync complete: %d nodes exported", successCount), "success", "exportSyncCompleted")
	}
	return c.SendStatus(200)
}

// ── Alert Rule Forms ───────────────────────────────────

var supportedAlertProviders = map[string]bool{
	"webhook":  true,
	"telegram": true,
}

func (h *FormHandler) renderAlertRuleForm(c *fiber.Ctx, rule models.AlertRule, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	title := "New Alert Rule"
	if rule.ID != 0 {
		title = "Edit " + rule.Name
	}

	var groups []string
	h.DB.Model(&models.Node{}).
		Distinct(`"group"`).
		Where(`"group" <> ''`).
		Order(`"group" asc`).
		Pluck("group", &groups)

	return c.Render("alert_form", fiber.Map{
		"Title":        title,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "alerts",
		"ActiveTab":    "alerts",
		"Alert":        rule,
		"IsEdit":       rule.ID != 0,
		"Groups":       groups,
		"Error":        errMsg,
	}, "base")
}

func normalizeAlertTargetGroup(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "global") || value == "*" {
		return "*"
	}
	return value
}

func validateAlertURL(value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("enter a valid webhook URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook URL must start with http:// or https://")
	}
	return nil
}

func validateAlertRule(rule models.AlertRule, hasWebhook bool, hasTelegramToken bool, hasTelegramChat bool) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(rule.Name) > 100 {
		return fmt.Errorf("rule name must be 100 characters or less")
	}
	if rule.TargetGroup == "" {
		return fmt.Errorf("target group is required")
	}
	if !supportedAlertProviders[rule.Provider] {
		return fmt.Errorf("choose a valid alert provider")
	}
	if !rule.AlertOnDiff && !rule.AlertOnFailure && !rule.AlertOnSecurity {
		return fmt.Errorf("select at least one event that should trigger this rule")
	}
	if rule.Provider == "webhook" && !hasWebhook {
		return fmt.Errorf("webhook URL is required for webhook alerts")
	}
	if rule.Provider == "telegram" && (!hasTelegramToken || !hasTelegramChat) {
		return fmt.Errorf("bot token and chat ID are required for Telegram alerts")
	}
	return nil
}

func (h *FormHandler) NewAlertRule(c *fiber.Ctx) error {
	return h.renderAlertRuleForm(c, models.AlertRule{
		TargetGroup:    "*",
		Enabled:        true,
		Provider:       "webhook",
		AlertOnDiff:    true,
		AlertOnFailure: true,
	}, "", 0)
}

func (h *FormHandler) EditAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule
	if err := h.DB.Where("id = ?", id).First(&rule).Error; err != nil {
		return c.Status(404).SendString("Alert rule not found")
	}

	return h.renderAlertRuleForm(c, rule, "", 0)
}

func (h *FormHandler) SaveAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&rule).Error; err != nil {
			return c.Status(404).SendString("Alert rule not found")
		}
	}

	rule.Name = strings.TrimSpace(c.FormValue("name"))
	rule.TargetGroup = normalizeAlertTargetGroup(c.FormValue("target_group"))
	rule.Enabled = c.FormValue("enabled") == "on"
	rule.Provider = strings.TrimSpace(c.FormValue("provider"))
	if rule.Provider == "" {
		rule.Provider = "webhook"
	}
	rule.AlertOnDiff = c.FormValue("alert_on_diff") == "on"
	rule.AlertOnFailure = c.FormValue("alert_on_failure") == "on"
	rule.AlertOnSecurity = c.FormValue("alert_on_security") == "on"

	whURL := strings.TrimSpace(c.FormValue("webhook_url"))
	if whURL != "" {
		if err := validateAlertURL(whURL); err != nil {
			return h.renderAlertRuleForm(c, rule, err.Error()+".", fiber.StatusBadRequest)
		}
		enc, err := crypto.Encrypt(whURL)
		if err != nil {
			return h.renderAlertRuleForm(c, rule, "Failed to encrypt webhook URL.", fiber.StatusInternalServerError)
		}
		rule.WebhookURL = enc
	}

	tToken := strings.TrimSpace(c.FormValue("telegram_token"))
	if tToken != "" {
		enc, err := crypto.Encrypt(tToken)
		if err != nil {
			return h.renderAlertRuleForm(c, rule, "Failed to encrypt Telegram token.", fiber.StatusInternalServerError)
		}
		rule.TelegramToken = enc
	}

	tChatID := strings.TrimSpace(c.FormValue("telegram_chat_id"))
	if tChatID != "" {
		enc, err := crypto.Encrypt(tChatID)
		if err != nil {
			return h.renderAlertRuleForm(c, rule, "Failed to encrypt Telegram chat ID.", fiber.StatusInternalServerError)
		}
		rule.TelegramChatID = enc
	}

	if err := validateAlertRule(rule, rule.WebhookURL != "", rule.TelegramToken != "", rule.TelegramChatID != ""); err != nil {
		return h.renderAlertRuleForm(c, rule, err.Error()+".", fiber.StatusBadRequest)
	}

	if err := h.DB.Save(&rule).Error; err != nil {
		log.Printf("[Alert] Failed to save alert rule: %v", err)
		return h.renderAlertRuleForm(c, rule, "Could not save alert rule.", fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "success", "alert", "Alert rule saved", "name="+rule.Name+" provider="+rule.Provider)

	setNotification(c, "Alert rule saved successfully", "success")
	return c.Redirect("/settings/alerts")
}

func (h *FormHandler) DeleteAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule
	if err := h.DB.Where("id = ?", id).First(&rule).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Alert rule not found", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(404).SendString("Alert rule not found")
	}

	if err := h.DB.Delete(&rule).Error; err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, "Could not delete alert rule", "error")
			c.Set("HX-Reswap", "none")
			return c.SendString("")
		}
		return c.Status(500).SendString("Could not delete alert rule")
	}
	writeAuditLog(h.DB, c, "warning", "alert", "Alert rule deleted", "name="+rule.Name)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Alert rule deleted", "success")
		return c.SendString("")
	}

	return c.Redirect("/settings/alerts")
}

func (h *FormHandler) TestAlertRule(c *fiber.Ctx) error {
	provider := strings.TrimSpace(c.FormValue("provider"))
	if provider == "" {
		provider = "webhook"
	}
	if !supportedAlertProviders[provider] {
		setNotification(c, "Choose a valid alert provider.", "error")
		return c.SendStatus(fiber.StatusBadRequest)
	}
	id := c.FormValue("id")
	msg := "Test message from Mimic Backup System."

	if provider == "webhook" {
		whURL := strings.TrimSpace(c.FormValue("webhook_url"))
		if whURL == "" && id != "" && id != "0" {
			var rule models.AlertRule
			if h.DB.Where("id = ?", id).First(&rule).Error == nil && rule.WebhookURL != "" {
				decrypted, err := crypto.Decrypt(rule.WebhookURL)
				if err != nil {
					setNotification(c, "Stored webhook URL could not be decrypted.", "error")
					return c.SendStatus(fiber.StatusInternalServerError)
				}
				whURL = decrypted
			}
		}
		if whURL == "" {
			setNotification(c, "Webhook URL is required.", "error")
			return c.SendStatus(fiber.StatusBadRequest)
		}
		if err := validateAlertURL(whURL); err != nil {
			setNotification(c, err.Error()+".", "error")
			return c.SendStatus(fiber.StatusBadRequest)
		}

		if err := alert.SendWebhook(whURL, msg); err != nil {
			setNotification(c, fmt.Sprintf("Webhook test failed: %v", err), "error")
			return c.SendStatus(fiber.StatusBadGateway)
		}
	} else if provider == "telegram" {
		token := strings.TrimSpace(c.FormValue("telegram_token"))
		chatID := strings.TrimSpace(c.FormValue("telegram_chat_id"))

		if (token == "" || chatID == "") && id != "" && id != "0" {
			var rule models.AlertRule
			if h.DB.Where("id = ?", id).First(&rule).Error == nil {
				if token == "" && rule.TelegramToken != "" {
					decrypted, err := crypto.Decrypt(rule.TelegramToken)
					if err != nil {
						setNotification(c, "Stored Telegram token could not be decrypted.", "error")
						return c.SendStatus(fiber.StatusInternalServerError)
					}
					token = decrypted
				}
				if chatID == "" && rule.TelegramChatID != "" {
					decrypted, err := crypto.Decrypt(rule.TelegramChatID)
					if err != nil {
						setNotification(c, "Stored Telegram chat ID could not be decrypted.", "error")
						return c.SendStatus(fiber.StatusInternalServerError)
					}
					chatID = decrypted
				}
			}
		}

		if token == "" || chatID == "" {
			setNotification(c, "Telegram token and chat ID are required.", "error")
			return c.SendStatus(fiber.StatusBadRequest)
		}

		if err := alert.SendTelegram(token, chatID, msg); err != nil {
			setNotification(c, fmt.Sprintf("Telegram test failed: %v", err), "error")
			return c.SendStatus(fiber.StatusBadGateway)
		}
	}

	setNotification(c, "Test successful. Check your alert destination.", "success")
	return c.SendStatus(200)
}

func (h *FormHandler) SnoozeNode(c *fiber.Ctx) error {
	id := c.Params("id")
	hours, _ := strconv.Atoi(c.Query("hours", "0"))

	var node models.Node
	if err := h.DB.First(&node, id).Error; err != nil {
		setNotification(c, "Node not found", "error")
		return c.SendStatus(404)
	}

	if hours > 0 {
		snoozeTime := time.Now().Add(time.Duration(hours) * time.Hour)
		node.AlertSnoozeUntil = &snoozeTime
		setNotification(c, fmt.Sprintf("Alerts muted for %d hours", hours), "success")
	} else {
		node.AlertSnoozeUntil = nil
		setNotification(c, "Alerts unmuted", "success")
	}

	if err := h.DB.Model(&models.Node{}).Where("id = ?", node.ID).Update("alert_snooze_until", node.AlertSnoozeUntil).Error; err != nil {
		setNotification(c, "Could not update alert mute state", "error")
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "info", "node", "Node alert mute state updated", fmt.Sprintf("node=%s hours=%d", node.Name, hours))

	c.Set("HX-Redirect", "/nodes")
	return c.SendStatus(200)
}

// ── Security Rules ────────────────────────────────
var supportedSecuritySeverities = map[string]bool{
	"Info":     true,
	"Warning":  true,
	"Critical": true,
}

var supportedSecurityVendors = map[string]bool{
	"*":        true,
	"cisco":    true,
	"mikrotik": true,
	"huawei":   true,
	"juniper":  true,
}

func renderSecurityRuleForm(c *fiber.Ctx, rule models.SecurityRule, errMsg string, status int) error {
	if status != 0 {
		c.Status(status)
	}

	title := "New Security Rule"
	if rule.ID != 0 {
		title = "Edit Security Rule"
	}

	return c.Render("security_form", fiber.Map{
		"Title":        title,
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"ActiveTab":    "security",
		"Rule":         rule,
		"IsEdit":       rule.ID != 0,
		"Error":        errMsg,
	}, "base")
}

func (h *FormHandler) reEvaluateAllNodesAsync() {
	db := h.DB
	go func() {
		var nodes []models.Node
		db.Find(&nodes)

		for _, node := range nodes {
			var lastBackup models.NodeBackup
			if err := db.Where("node_id = ? AND status = 'success'", node.ID).Order("version desc").First(&lastBackup).Error; err == nil {
				audit.RunAudit(db, &node, lastBackup.Version, lastBackup.Config)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func (h *FormHandler) reEvaluateNodeAsync(nodeID uint) {
	db := h.DB
	go func() {
		var node models.Node
		if err := db.First(&node, nodeID).Error; err != nil {
			return
		}

		var lastBackup models.NodeBackup
		if err := db.Where("node_id = ? AND status = 'success'", node.ID).Order("version desc").First(&lastBackup).Error; err == nil {
			audit.RunAudit(db, &node, lastBackup.Version, lastBackup.Config)
		}
	}()
}

func (h *FormHandler) NewSecurityRule(c *fiber.Ctx) error {
	return renderSecurityRuleForm(c, models.SecurityRule{
		Category:    "General",
		Vendor:      "*",
		TargetGroup: "*",
		Enabled:     true,
		MatchType:   "contains",
		Penalty:     10,
		Severity:    "Warning",
	}, "", 0)
}

func (h *FormHandler) EditSecurityRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.SecurityRule
	if err := h.DB.First(&rule, id).Error; err != nil {
		return c.Redirect("/settings/security")
	}

	return renderSecurityRuleForm(c, rule, "", 0)
}

func (h *FormHandler) SaveSecurityRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.SecurityRule

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&rule).Error; err != nil {
			return c.Status(404).SendString("Security rule not found")
		}
	}

	rule.Name = strings.TrimSpace(c.FormValue("name"))
	rule.Description = strings.TrimSpace(c.FormValue("description"))
	rule.Category = strings.TrimSpace(c.FormValue("category"))
	if rule.Category == "" {
		rule.Category = "General"
	}
	rule.Vendor = strings.ToLower(strings.TrimSpace(c.FormValue("vendor")))
	if rule.Vendor == "" {
		rule.Vendor = "*"
	}
	rule.TargetGroup = strings.TrimSpace(c.FormValue("target_group"))
	if rule.TargetGroup == "" {
		rule.TargetGroup = "*"
	}
	rule.Enabled = c.FormValue("enabled") == "on"
	rule.RegexPattern = strings.TrimSpace(c.FormValue("regex_pattern"))
	rule.ContextBlock = strings.TrimSpace(c.FormValue("context_block"))
	rule.MatchType = strings.TrimSpace(c.FormValue("match_type"))
	if rule.MatchType == "" {
		rule.MatchType = "contains"
	}

	rule.Severity = strings.TrimSpace(c.FormValue("severity"))
	if rule.Severity == "" {
		rule.Severity = "Warning"
	}

	penalty, err := strconv.Atoi(strings.TrimSpace(c.FormValue("penalty")))
	if err != nil {
		return renderSecurityRuleForm(c, rule, "Score impact must be a number between 0 and 100.", fiber.StatusBadRequest)
	}
	rule.Penalty = penalty
	rule.Remediation = strings.TrimSpace(c.FormValue("remediation"))

	if !supportedSecurityVendors[rule.Vendor] {
		return renderSecurityRuleForm(c, rule, "Invalid vendor scope.", fiber.StatusBadRequest)
	}
	if !supportedSecuritySeverities[rule.Severity] {
		return renderSecurityRuleForm(c, rule, "Invalid severity.", fiber.StatusBadRequest)
	}

	if err := audit.ValidateRule(rule); err != nil {
		return renderSecurityRuleForm(c, rule, err.Error(), fiber.StatusBadRequest)
	}

	if err := h.DB.Save(&rule).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to save security rule: %v", err), "error")
		return c.SendStatus(500)
	}
	writeAuditLog(h.DB, c, "success", "security", "Security rule saved", "name="+rule.Name)

	h.reEvaluateAllNodesAsync()

	setNotification(c, "Security rule saved. Nodes are being re-evaluated in the background.", "success")
	return c.Redirect("/settings/security")
}

func (h *FormHandler) DeleteSecurityRule(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("rule_id = ?", id).Delete(&models.SecurityViolation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", id).Delete(&models.NodeRuleException{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&models.SecurityRule{}).Error
	}); err != nil {
		setNotification(c, fmt.Sprintf("Failed to delete security rule: %v", err), "error")
		return c.SendStatus(500)
	}
	writeAuditLog(h.DB, c, "warning", "security", "Security rule deleted", "rule_id="+id)
	h.reEvaluateAllNodesAsync()

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Security rule deleted. Nodes are being re-evaluated.", "success")
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/security")
}

// ── Golden Configs ────────────────────────────────

func (h *FormHandler) GoldenForm(c *fiber.Ctx) error {
	id := c.Params("id")
	var gc models.GoldenConfig

	if id != "" {
		if err := h.DB.First(&gc, id).Error; err != nil {
			return c.Status(404).SendString("Golden Config not found")
		}
	}

	return c.Render("golden_form", fiber.Map{
		"Title":  "Golden Config",
		"Config": gc,
	}, "base")
}

func (h *FormHandler) SaveGoldenConfig(c *fiber.Ctx) error {
	id := c.Params("id")
	var gc models.GoldenConfig

	if id != "" {
		if err := h.DB.Where("id = ?", id).First(&gc).Error; err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Golden Config not found")
		}
	}

	gc.Name = strings.TrimSpace(c.FormValue("name"))
	gc.Vendor = strings.ToLower(strings.TrimSpace(c.FormValue("vendor")))
	if gc.Vendor == "" {
		gc.Vendor = "*"
	}
	gc.TargetGroup = strings.TrimSpace(c.FormValue("target_group"))
	if gc.TargetGroup == "" {
		gc.TargetGroup = "*"
	}
	gc.ConfigTemplate = strings.TrimSpace(c.FormValue("config_template"))
	if gc.Name == "" || gc.ConfigTemplate == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Name and configuration template are required")
	}
	if !supportedSecurityVendors[gc.Vendor] {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid vendor scope")
	}

	if err := h.DB.Save(&gc).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to save golden config: %v", err), "error")
		return c.SendStatus(500)
	}
	writeAuditLog(h.DB, c, "success", "golden", "Golden config saved", "name="+gc.Name)

	setNotification(c, "Golden config saved successfully", "success")
	return c.Redirect("/settings/golden")
}

func (h *FormHandler) DeleteGoldenConfig(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Where("id = ?", id).Delete(&models.GoldenConfig{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Could not delete golden config")
	}
	writeAuditLog(h.DB, c, "warning", "golden", "Golden config deleted", "config_id="+id)

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Golden config deleted", "success")
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/golden")
}

// ── Rule Exceptions ────────────────────────────────

func (h *FormHandler) AddRuleException(c *fiber.Ctx) error {
	nodeID, _ := strconv.Atoi(c.Params("id"))
	ruleID, _ := strconv.Atoi(c.Params("rule_id"))

	var node models.Node
	if err := h.DB.First(&node, nodeID).Error; err != nil {
		setNotification(c, "Node not found", "error")
		return c.SendStatus(404)
	}

	var rule models.SecurityRule
	if err := h.DB.First(&rule, ruleID).Error; err != nil {
		setNotification(c, "Security rule not found", "error")
		return c.SendStatus(404)
	}

	exception := models.NodeRuleException{
		NodeID: uint(nodeID),
		RuleID: uint(ruleID),
		Reason: "Whitelisted by user",
	}
	if err := h.DB.Where("node_id = ? AND rule_id = ?", nodeID, ruleID).FirstOrCreate(&exception).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to ignore rule: %v", err), "error")
		return c.SendStatus(500)
	}
	writeAuditLog(h.DB, c, "warning", "security", "Security rule exception added", fmt.Sprintf("node=%s rule=%s", node.Name, rule.Name))

	h.reEvaluateNodeAsync(uint(nodeID))

	setNotification(c, "Rule ignored for this node.", "info")
	c.Set("HX-Redirect", fmt.Sprintf("/nodes/%d", nodeID))
	return c.SendStatus(200)
}

func (h *FormHandler) RemoveRuleException(c *fiber.Ctx) error {
	nodeID, _ := strconv.Atoi(c.Params("id"))
	ruleID, _ := strconv.Atoi(c.Params("rule_id"))

	if err := h.DB.Unscoped().Where("node_id = ? AND rule_id = ?", nodeID, ruleID).Delete(&models.NodeRuleException{}).Error; err != nil {
		setNotification(c, "Could not revoke the exception", "error")
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	writeAuditLog(h.DB, c, "info", "security", "Security rule exception removed", fmt.Sprintf("node_id=%d rule_id=%d", nodeID, ruleID))

	h.reEvaluateNodeAsync(uint(nodeID))

	setNotification(c, "Exception revoked. Rule is active again.", "success")
	c.Set("HX-Redirect", fmt.Sprintf("/nodes/%d", nodeID))
	return c.SendStatus(200)
}
