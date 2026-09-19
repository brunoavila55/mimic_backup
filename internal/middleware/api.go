package middleware

import (
	"mimic/internal/access"
	"mimic/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"gorm.io/gorm"
)

func apiError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{"code": code, "message": message},
	})
}

// RequireAPIAuth mirrors RequireAuth but guarantees JSON responses and never
// redirects a client under /api/v1.
func RequireAPIAuth(store *session.Store, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil || sess.Get("user_id") == nil {
			return apiError(c, fiber.StatusUnauthorized, "unauthenticated", "Authentication is required.")
		}

		var user models.User
		if err := db.Select("id", "username", "email", "role", "avatar").First(&user, sess.Get("user_id")).Error; err != nil {
			_ = sess.Destroy()
			return apiError(c, fiber.StatusUnauthorized, "unauthenticated", "The session is no longer valid.")
		}

		c.Locals("user_id", user.ID)
		c.Locals("username", user.Username)
		c.Locals("email", user.Email)
		c.Locals("role", user.Role)
		c.Locals("avatar", user.Avatar)
		return c.Next()
	}
}

func RequireAPIPermission(permission access.Permission) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if !access.Allows(role, permission) {
			return apiError(c, fiber.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
		}
		return c.Next()
	}
}
