package handlers

import (
	"fmt"
	"log"

	"mimic/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func writeAuditLog(db *gorm.DB, c *fiber.Ctx, level, category, message, details string) {
	actor := "anonymous"
	if c != nil && c.Locals("username") != nil {
		actor = fmt.Sprint(c.Locals("username"))
	}
	entry := models.SystemLog{
		Level: level, Category: category, Message: message,
		Details: fmt.Sprintf("actor=%s %s", actor, details),
	}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("[Audit] Failed to persist audit log: %v", err)
	}
}
