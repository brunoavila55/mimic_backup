package handlers

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mimic/internal/models"
	"mimic/internal/services/alert"
	"mimic/internal/services/audit"
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

func setNotification(c *fiber.Ctx, message, kind string) {
	payload, err := json.Marshal(fiber.Map{
		"showNotification": fiber.Map{
			"message": message,
			"type":    kind,
		},
	})
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
		return c.Status(500).SendString(err.Error())
	}

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
	// Also delete associated backups to avoid orphan records
	h.DB.Where("node_id = ?", id).Delete(&models.NodeBackup{})
	h.DB.Where("id = ?", id).Delete(&models.Node{})

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
		if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
			return c.Status(404).SendString("User not found")
		}
	}

	user.Username = strings.TrimSpace(c.FormValue("username"))
	user.Email = strings.TrimSpace(c.FormValue("email"))
	user.Role = strings.TrimSpace(c.FormValue("role"))
	if user.Username == "" {
		return c.Status(400).SendString("Username is required")
	}
	if user.Role != "Administrator" && user.Role != "Viewer" {
		return c.Status(400).SendString("Invalid user role")
	}

	rawPass := c.FormValue("password")
	if rawPass != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).SendString("Error hashing password")
		}
		user.Password = string(hash)
	}

	if err := h.DB.Save(&user).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}

	setNotification(c, "User saved successfully", "success")
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
		setNotification(c, "User deleted", "success")
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

	setNotification(c, "Credential saved successfully", "success")
	return c.Redirect("/settings/credentials")
}

func (h *FormHandler) DeleteCredential(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.Credential{})

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Credential deleted", "success")
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

	setNotification(c, "Routine saved successfully", "success")
	return c.Redirect("/settings/routines")
}

func (h *FormHandler) DeleteRoutine(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.BackupRoutine{})

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Routine deleted", "success")
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
	if port < 1 || port > 65535 {
		port = 22
	}
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
	settings.Enabled = c.FormValue("enabled") == "on"
	settings.SyncTime = c.FormValue("sync_time")

	if err := h.DB.Save(&settings).Error; err != nil {
		return c.Status(500).SendString(err.Error())
	}
	setNotification(c, "SFTP settings saved", "success")
	return c.Redirect("/settings/sftp")
}

func (h *FormHandler) TestSFTPConnection(c *fiber.Ctx) error {
	var settings models.SftpSettings

	// We populate from form, but fallback to DB for password if empty
	h.DB.First(&settings)

	settings.Host = c.FormValue("host")
	port, _ := strconv.Atoi(c.FormValue("port"))
	if port < 1 || port > 65535 {
		port = 22
	}
	settings.Port = port
	settings.Username = c.FormValue("username")
	settings.Path = c.FormValue("path")

	rawPass := c.FormValue("password")
	if rawPass != "" {
		encPass, err := crypto.Encrypt(rawPass)
		if err != nil {
			setNotification(c, "Connection failed: error encrypting password", "error")
			return c.SendStatus(200)
		}
		settings.Password = encPass
	}

	err := h.Sftp.TestConnection(&settings)
	if err != nil {
		setNotification(c, fmt.Sprintf("Connection failed: %v", err), "error")
		return c.SendStatus(200)
	}

	setNotification(c, "Connection successful! Directory verified.", "success")
	return c.SendStatus(200)
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
		hash, err := bcrypt.GenerateFromPassword([]byte(rawPass), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).SendString("Error hashing password")
		}
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

	if err := h.Sftp.Export(&backup, &settings); err != nil {
		if c.Get("HX-Request") == "true" {
			setNotification(c, fmt.Sprintf("Export failed: %v", err), "error")
			return c.SendStatus(200)
		}
		return c.Status(500).SendString(fmt.Sprintf("Export failed: %v", err))
	}

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

	setNotification(c, fmt.Sprintf("Sync complete: %d nodes exported", successCount), "success")
	return c.SendStatus(200)
}

// ── Alert Rule Forms ───────────────────────────────────

func (h *FormHandler) NewAlertRule(c *fiber.Ctx) error {
	return c.Render("alert_form", fiber.Map{
		"Title":        "New Alert Rule",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"Alert":        models.AlertRule{TargetGroup: "Global", Enabled: true, Provider: "webhook"},
	}, "base")
}

func (h *FormHandler) EditAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule
	if err := h.DB.Where("id = ?", id).First(&rule).Error; err != nil {
		return c.Status(404).SendString("Alert Rule not found")
	}

	return c.Render("alert_form", fiber.Map{
		"Title":        "Edit Alert Rule",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "settings",
		"Alert":        rule,
	}, "base")
}

func (h *FormHandler) SaveAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule

	if id != "" {
		h.DB.Where("id = ?", id).First(&rule)
	}

	rule.Name = c.FormValue("name")
	rule.TargetGroup = c.FormValue("target_group")
	rule.Enabled = c.FormValue("enabled") == "on"
	rule.Provider = c.FormValue("provider")

	whURL := c.FormValue("webhook_url")
	if whURL != "" {
		enc, _ := crypto.Encrypt(whURL)
		rule.WebhookURL = enc
	}

	tToken := c.FormValue("telegram_token")
	if tToken != "" {
		enc, _ := crypto.Encrypt(tToken)
		rule.TelegramToken = enc
	}

	tChatID := c.FormValue("telegram_chat_id")
	if tChatID != "" {
		enc, _ := crypto.Encrypt(tChatID)
		rule.TelegramChatID = enc
	}

	rule.AlertOnDiff = c.FormValue("alert_on_diff") == "on"
	rule.AlertOnFailure = c.FormValue("alert_on_failure") == "on"
	rule.AlertOnSecurity = c.FormValue("alert_on_security") == "on"

	if err := h.DB.Save(&rule).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to save rule: %v", err), "error")
		return c.SendStatus(500)
	}

	setNotification(c, "Alert rule saved successfully", "success")
	return c.Redirect("/settings/alerts")
}

func (h *FormHandler) DeleteAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.AlertRule{})

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Alert rule deleted", "success")
		return c.SendStatus(200)
	}

	return c.Redirect("/settings/alerts")
}

func (h *FormHandler) TestAlertRule(c *fiber.Ctx) error {
	provider := c.FormValue("provider")
	id := c.FormValue("id")
	msg := "👋 Test message from Mimic Backup System!"

	if provider == "webhook" {
		whURL := c.FormValue("webhook_url")
		if whURL == "" && id != "" && id != "0" {
			var rule models.AlertRule
			if h.DB.Where("id = ?", id).First(&rule).Error == nil && rule.WebhookURL != "" {
				whURL, _ = crypto.Decrypt(rule.WebhookURL)
			}
		}
		if whURL == "" {
			setNotification(c, "Webhook URL is required.", "error")
			return c.SendStatus(400)
		}

		err := alert.SendWebhook(whURL, msg)
		if err != nil {
			setNotification(c, fmt.Sprintf("Webhook test failed: %v", err), "error")
			return c.SendStatus(500)
		}
	} else if provider == "telegram" {
		token := c.FormValue("telegram_token")
		chatID := c.FormValue("telegram_chat_id")

		if (token == "" || chatID == "") && id != "" && id != "0" {
			var rule models.AlertRule
			if h.DB.Where("id = ?", id).First(&rule).Error == nil {
				if token == "" && rule.TelegramToken != "" {
					token, _ = crypto.Decrypt(rule.TelegramToken)
				}
				if chatID == "" && rule.TelegramChatID != "" {
					chatID, _ = crypto.Decrypt(rule.TelegramChatID)
				}
			}
		}

		if token == "" || chatID == "" {
			setNotification(c, "Telegram Token and Chat ID are required.", "error")
			return c.SendStatus(400)
		}

		err := alert.SendTelegram(token, chatID, msg)
		if err != nil {
			setNotification(c, fmt.Sprintf("Telegram test failed: %v", err), "error")
			return c.SendStatus(500)
		}
	}

	setNotification(c, "Test successful! Check your app.", "success")
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

	h.DB.Save(&node)

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

	h.reEvaluateAllNodesAsync()

	setNotification(c, "Security rule saved. Nodes are being re-evaluated in the background.", "success")
	return c.Redirect("/settings/security")
}

func (h *FormHandler) DeleteSecurityRule(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Where("id = ?", id).Delete(&models.SecurityRule{}).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to delete security rule: %v", err), "error")
		return c.SendStatus(500)
	}

	h.DB.Where("rule_id = ?", id).Delete(&models.SecurityViolation{})
	h.DB.Where("rule_id = ?", id).Delete(&models.NodeRuleException{})
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
		h.DB.Where("id = ?", id).First(&gc)
	}

	gc.Name = c.FormValue("name")
	gc.Vendor = c.FormValue("vendor")
	if gc.Vendor == "" {
		gc.Vendor = "*"
	}
	gc.TargetGroup = c.FormValue("target_group")
	if gc.TargetGroup == "" {
		gc.TargetGroup = "*"
	}
	gc.ConfigTemplate = c.FormValue("config_template")

	if err := h.DB.Save(&gc).Error; err != nil {
		setNotification(c, fmt.Sprintf("Failed to save golden config: %v", err), "error")
		return c.SendStatus(500)
	}

	setNotification(c, "Golden config saved successfully", "success")
	return c.Redirect("/settings?tab=golden")
}

func (h *FormHandler) DeleteGoldenConfig(c *fiber.Ctx) error {
	id := c.Params("id")
	h.DB.Where("id = ?", id).Delete(&models.GoldenConfig{})

	if c.Get("HX-Request") == "true" {
		setNotification(c, "Golden config deleted", "success")
		return c.SendStatus(200)
	}

	return c.Redirect("/settings?tab=golden")
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

	h.reEvaluateNodeAsync(uint(nodeID))

	setNotification(c, "Rule ignored for this node.", "info")
	c.Set("HX-Redirect", fmt.Sprintf("/nodes/%d", nodeID))
	return c.SendStatus(200)
}

func (h *FormHandler) RemoveRuleException(c *fiber.Ctx) error {
	nodeID, _ := strconv.Atoi(c.Params("id"))
	ruleID, _ := strconv.Atoi(c.Params("rule_id"))

	h.DB.Unscoped().Where("node_id = ? AND rule_id = ?", nodeID, ruleID).Delete(&models.NodeRuleException{})

	h.reEvaluateNodeAsync(uint(nodeID))

	setNotification(c, "Exception revoked. Rule is active again.", "success")
	c.Set("HX-Redirect", fmt.Sprintf("/nodes/%d", nodeID))
	return c.SendStatus(200)
}
