package middleware

import (
	"strings"
	"sync/atomic"

	"mimic/internal/access"
	"mimic/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// RequireSetup redirects to /setup if no users exist (first-time setup flow).
// Once setup is confirmed complete, it caches the result and becomes a no-op.
func RequireSetup(db *gorm.DB) fiber.Handler {
	var setupDone atomic.Bool

	return func(c *fiber.Ctx) error {
		if setupDone.Load() {
			return c.Next()
		}

		path := c.Path()

		// Always allow setup routes and static files through
		if strings.HasPrefix(path, "/setup") || strings.HasPrefix(path, "/static/") {
			return c.Next()
		}

		// Check if tables exist
		if !db.Migrator().HasTable(&models.User{}) {
			return c.Redirect("/setup")
		}

		// Check if at least one user exists
		var count int64
		if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Database temporarily unavailable")
		}
		if count == 0 {
			return c.Redirect("/setup")
		}

		// Setup is complete, cache and never check again
		setupDone.Store(true)
		return c.Next()
	}
}

// RequireAuth ensures the user is authenticated via session.
func RequireAuth(store *session.Store, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return c.Redirect("/login", 302)
		}

		userId := sess.Get("user_id")
		if userId == nil {
			if c.Get("HX-Request") == "true" {
				c.Set("HX-Redirect", "/login")
				return c.SendStatus(fiber.StatusUnauthorized)
			}
			return c.Redirect("/login", 302)
		}

		var user models.User
		if err := db.Select("id", "username", "role", "avatar").First(&user, userId).Error; err != nil {
			_ = sess.Destroy()
			if c.Get("HX-Request") == "true" {
				c.Set("HX-Redirect", "/login")
				return c.SendStatus(fiber.StatusUnauthorized)
			}
			return c.Redirect("/login", fiber.StatusFound)
		}

		c.Locals("user_id", user.ID)
		c.Locals("username", user.Username)
		c.Locals("role", user.Role)
		c.Locals("avatar", user.Avatar)

		return c.Next()
	}
}

func RequirePermission(permission access.Permission) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if !access.Allows(role, permission) {
			if c.Get("HX-Request") == "true" {
				c.Set("HX-Trigger", `{"showNotification": {"message": "Access denied for your permission level.", "type": "error"}}`)
				return c.SendStatus(fiber.StatusForbidden)
			}
			return c.Status(fiber.StatusForbidden).SendString("Forbidden: insufficient permission")
		}
		return c.Next()
	}
}

// RequireAdmin ensures the authenticated user has the Administrator role.
// It assumes RequireAuth has already run and populated c.Locals("role").
func RequireAdmin() fiber.Handler {
	return RequirePermission(access.ManageUsers)
}
