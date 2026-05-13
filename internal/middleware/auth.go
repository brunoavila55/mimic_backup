package middleware

import (
	"strings"

	"mimic/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

// RequireSetup redirects to /setup if no users exist (first-time setup flow).
// Once setup is confirmed complete, it caches the result and becomes a no-op.
func RequireSetup(db *gorm.DB) fiber.Handler {
	setupDone := false

	return func(c *fiber.Ctx) error {
		if setupDone {
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
		db.Model(&models.User{}).Count(&count)
		if count == 0 {
			return c.Redirect("/setup")
		}

		// Setup is complete, cache and never check again
		setupDone = true
		return c.Next()
	}
}

// RequireAuth ensures the user is authenticated via session.
func RequireAuth(store *session.Store) fiber.Handler {
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

		c.Locals("user_id", userId)
		c.Locals("username", sess.Get("username"))
		c.Locals("role", sess.Get("role"))
		c.Locals("avatar", sess.Get("avatar"))

		return c.Next()
	}
}
