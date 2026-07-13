package handlers

import (
	"log"
	"mimic/internal/models"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type SetupHandler struct {
	DB *gorm.DB
}

func (h *SetupHandler) GetDatabaseSetup(c *fiber.Ctx) error {
	if h.hasUsers() {
		return c.Redirect("/login")
	}

	sqlDB, err := h.DB.DB()
	dbOk := err == nil && sqlDB.Ping() == nil
	tablesOk := h.DB.Migrator().HasTable(&models.User{})

	return c.Render("setup_database", fiber.Map{
		"Title":    "Initial Setup",
		"Step":     1,
		"DBOk":     dbOk,
		"TablesOk": tablesOk,
	})
}

func (h *SetupHandler) PostDatabaseSetup(c *fiber.Ctx) error {
	return c.Redirect("/setup/superuser")
}

func (h *SetupHandler) GetCreateSuperuser(c *fiber.Ctx) error {
	if h.hasUsers() {
		return c.Redirect("/login")
	}

	return c.Render("setup_superuser", fiber.Map{
		"Title": "Create Administrator",
		"Step":  2,
	})
}

func (h *SetupHandler) PostCreateSuperuser(c *fiber.Ctx) error {
	userMutationMu.Lock()
	defer userMutationMu.Unlock()
	if h.hasUsers() {
		return c.Redirect("/login")
	}

	username := strings.TrimSpace(c.FormValue("username"))
	email := strings.TrimSpace(c.FormValue("email"))
	password := c.FormValue("password")
	confirm := c.FormValue("confirm_password")

	if username == "" || password == "" {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Create Administrator",
			"Step":  2,
			"Error": "Username and password are required",
		})
	}

	if strings.ContainsAny(username, " \t\r\n") {
		return c.Render("setup_superuser", fiber.Map{"Title": "Create Administrator", "Step": 2, "Error": "Username cannot contain spaces"})
	}
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return c.Render("setup_superuser", fiber.Map{"Title": "Create Administrator", "Step": 2, "Error": "Enter a valid email address"})
		}
	}

	if len(password) < 8 {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Create Administrator",
			"Step":  2,
			"Error": "Password must be at least 8 characters long",
		})
	}

	if password != confirm {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Create Administrator",
			"Step":  2,
			"Error": "Passwords do not match",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Create Administrator",
			"Step":  2,
			"Error": "Error hashing password",
		})
	}
	user := models.User{
		Username: username,
		Email:    email,
		Password: string(hash),
		Role:     "Administrator",
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Create Administrator",
			"Step":  2,
			"Error": "Could not create the administrator user",
		})
	}
	writeAuditLog(h.DB, c, "success", "user", "Initial administrator created", "username="+user.Username)

	return c.Redirect("/login")
}

func (h *SetupHandler) hasUsers() bool {
	if !h.DB.Migrator().HasTable(&models.User{}) {
		return false
	}
	var count int64
	if err := h.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		log.Printf("[Setup] Failed to count users: %v", err)
		return false
	}
	return count > 0
}
