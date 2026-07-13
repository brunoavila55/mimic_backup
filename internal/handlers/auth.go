package handlers

import (
	"mimic/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB    *gorm.DB
	Store *session.Store
}

func (h *AuthHandler) GetLogin(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err == nil && sess.Get("user_id") != nil {
		return c.Redirect("/", 302)
	}
	return c.Render("login", fiber.Map{"Title": "Login"})
}

func (h *AuthHandler) PostLogin(c *fiber.Ctx) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	var user models.User
	if err := h.DB.Where("username = ?", username).First(&user).Error; err != nil {
		writeAuditLog(h.DB, c, "warning", "auth", "Failed login attempt", "username="+username)
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Invalid credentials", "LoginUsername": username})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		writeAuditLog(h.DB, c, "warning", "auth", "Failed login attempt", "username="+username)
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Invalid credentials", "LoginUsername": username})
	}

	sess, err := h.Store.Get(c)
	if err != nil {
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Session error", "LoginUsername": username})
	}
	if err := sess.Regenerate(); err != nil {
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Session error", "LoginUsername": username})
	}

	sess.Set("user_id", user.ID)
	sess.Set("username", user.Username)
	sess.Set("role", user.Role)
	sess.Set("avatar", user.Avatar)
	if err := sess.Save(); err != nil {
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Error saving session", "LoginUsername": username})
	}
	writeAuditLog(h.DB, c, "success", "auth", "User logged in", "username="+user.Username)

	return c.Redirect("/", 302)
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	writeAuditLog(h.DB, c, "info", "auth", "User logged out", "")
	sess, err := h.Store.Get(c)
	if err == nil {
		if err := sess.Destroy(); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Could not destroy session")
		}
	}
	return c.Redirect("/login", 302)
}
