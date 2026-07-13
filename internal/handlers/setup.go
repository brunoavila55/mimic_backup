package handlers

import (
	"fmt"
	"log"
	"mimic/internal/models"
	"mimic/pkg/crypto"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type SetupHandler struct {
	DB    *gorm.DB
	Store *session.Store
}

type SetupReadiness struct {
	DatabaseOK   bool
	SchemaOK     bool
	EncryptionOK bool
	Ready        bool
	DatabaseName string
	Error        string
}

func (h *SetupHandler) readiness() SetupReadiness {
	status := SetupReadiness{}
	sqlDB, err := h.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		status.Error = "Mimic could not reach PostgreSQL. Check DATABASE_URL and restart the service."
		return status
	}
	status.DatabaseOK = true

	if err := h.DB.Raw("SELECT current_database()").Scan(&status.DatabaseName).Error; err != nil {
		status.DatabaseName = "PostgreSQL"
	}
	coreTables := []any{&models.User{}, &models.Node{}, &models.NodeBackup{}, &models.SftpSettings{}}
	status.SchemaOK = true
	for _, model := range coreTables {
		if !h.DB.Migrator().HasTable(model) {
			status.SchemaOK = false
			break
		}
	}
	if !status.SchemaOK {
		status.Error = "The database schema is incomplete. Review the migration logs and restart Mimic."
		return status
	}

	if err := crypto.ValidateSecretKey(); err != nil {
		status.Error = "The encryption key is unavailable or invalid. Check SECRET_KEY and restart Mimic."
		return status
	}
	status.EncryptionOK = true
	status.Ready = true
	return status
}

func setupAdminView(username, email, errMsg string) fiber.Map {
	return fiber.Map{
		"Title": "Create Administrator", "Step": 2,
		"UsernameValue": username, "EmailValue": email, "Error": errMsg,
	}
}

func validateInitialAdmin(username, email, password, confirm string) string {
	if username == "" || password == "" || confirm == "" {
		return "Username, password, and password confirmation are required."
	}
	if len(username) < 3 || len(username) > 64 {
		return "Username must contain between 3 and 64 characters."
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return "Username cannot contain spaces."
	}
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(address.Address, email) {
			return "Enter a valid email address."
		}
	}
	if len(password) < 8 {
		return "Password must contain at least 8 characters."
	}
	if len(password) > 128 {
		return "Password must contain no more than 128 characters."
	}
	if password != confirm {
		return "Passwords do not match."
	}
	return ""
}

func (h *SetupHandler) GetDatabaseSetup(c *fiber.Ctx) error {
	hasUsers, err := h.hasUsers()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).Render("setup_database", fiber.Map{
			"Title": "Initial Setup", "Step": 1, "Status": SetupReadiness{Error: "Could not verify the installation state."},
		})
	}
	if hasUsers {
		return c.Redirect("/login")
	}
	status := h.readiness()
	return c.Render("setup_database", fiber.Map{
		"Title": "Initial Setup", "Step": 1, "Status": status,
	})
}

func (h *SetupHandler) PostDatabaseSetup(c *fiber.Ctx) error {
	hasUsers, err := h.hasUsers()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Could not verify the installation state")
	}
	if hasUsers {
		return c.Redirect("/login")
	}
	status := h.readiness()
	if !status.Ready {
		return c.Status(fiber.StatusServiceUnavailable).Render("setup_database", fiber.Map{
			"Title": "Initial Setup", "Step": 1, "Status": status,
		})
	}
	return c.Redirect("/setup/superuser")
}

func (h *SetupHandler) GetCreateSuperuser(c *fiber.Ctx) error {
	hasUsers, err := h.hasUsers()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Could not verify the installation state")
	}
	if hasUsers {
		return c.Redirect("/login")
	}
	if status := h.readiness(); !status.Ready {
		return c.Redirect("/setup")
	}
	return c.Render("setup_superuser", setupAdminView("", "", ""))
}

func (h *SetupHandler) PostCreateSuperuser(c *fiber.Ctx) error {
	userMutationMu.Lock()
	defer userMutationMu.Unlock()
	hasUsers, err := h.hasUsers()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).Render("setup_superuser", setupAdminView("", "", "Could not verify the installation state."))
	}
	if hasUsers {
		return c.Redirect("/login")
	}

	username := strings.TrimSpace(c.FormValue("username"))
	email := strings.TrimSpace(c.FormValue("email"))
	password := c.FormValue("password")
	confirm := c.FormValue("confirm_password")

	if validationError := validateInitialAdmin(username, email, password, confirm); validationError != "" {
		return c.Status(fiber.StatusBadRequest).Render("setup_superuser", setupAdminView(username, email, validationError))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).Render("setup_superuser", setupAdminView(username, email, "Could not protect the password."))
	}
	user := models.User{
		Username: username,
		Email:    email,
		Password: string(hash),
		Role:     "Administrator",
	}

	if err := h.DB.Create(&user).Error; err != nil {
		log.Printf("[Setup] Failed to create initial administrator: %v", err)
		return c.Status(fiber.StatusInternalServerError).Render("setup_superuser", setupAdminView(username, email, "Could not create the administrator user."))
	}
	writeAuditLog(h.DB, c, "success", "user", "Initial administrator created", "username="+user.Username)

	if h.Store == nil {
		return c.Redirect("/login")
	}
	sess, err := h.Store.Get(c)
	if err != nil || sess.Regenerate() != nil {
		return c.Redirect("/login")
	}
	sess.Set("user_id", user.ID)
	sess.Set("username", user.Username)
	sess.Set("role", user.Role)
	if err := sess.Save(); err != nil {
		log.Printf("[Setup] Administrator created but session could not be saved: %v", err)
		return c.Redirect("/login")
	}
	return c.Redirect("/")
}

func (h *SetupHandler) hasUsers() (bool, error) {
	if !h.DB.Migrator().HasTable(&models.User{}) {
		return false, nil
	}
	var count int64
	if err := h.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		log.Printf("[Setup] Failed to count users: %v", err)
		return false, fmt.Errorf("count users: %w", err)
	}
	return count > 0, nil
}
