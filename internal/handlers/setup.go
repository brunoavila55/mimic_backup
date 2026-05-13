package handlers

import (
	"mimic/internal/models"

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
		"Title":    "Configuração Inicial",
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
		"Title": "Criar Administrador",
		"Step":  2,
	})
}

func (h *SetupHandler) PostCreateSuperuser(c *fiber.Ctx) error {
	username := c.FormValue("username")
	email := c.FormValue("email")
	password := c.FormValue("password")
	confirm := c.FormValue("confirm_password")

	if username == "" || password == "" {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Criar Administrador",
			"Step":  2,
			"Error": "Usuário e senha são obrigatórios",
		})
	}

	if len(password) < 6 {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Criar Administrador",
			"Step":  2,
			"Error": "A senha deve ter pelo menos 6 caracteres",
		})
	}

	if password != confirm {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Criar Administrador",
			"Step":  2,
			"Error": "As senhas não coincidem",
		})
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user := models.User{
		Username: username,
		Email:    email,
		Password: string(hash),
		Role:     "Administrator",
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return c.Render("setup_superuser", fiber.Map{
			"Title": "Criar Administrador",
			"Step":  2,
			"Error": "Erro ao criar usuário: " + err.Error(),
		})
	}

	return c.Redirect("/login")
}

func (h *SetupHandler) hasUsers() bool {
	if !h.DB.Migrator().HasTable(&models.User{}) {
		return false
	}
	var count int64
	h.DB.Model(&models.User{}).Count(&count)
	return count > 0
}
