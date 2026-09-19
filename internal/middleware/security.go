package middleware

import (
	"net/url"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SecurityHeadersAndOrigin adds baseline browser protections and rejects
// cross-origin state-changing requests. Same-origin requests and non-browser
// clients without Origin/Referer continue to work.
func SecurityHeadersAndOrigin() fiber.Handler {
	configuredOrigin := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_ORIGIN")), "/")
	return func(c *fiber.Ctx) error {
		rejectOrigin := func() error {
			if strings.HasPrefix(c.Path(), "/api/") {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": fiber.Map{"code": "origin_forbidden", "message": "Cross-origin request rejected."},
				})
			}
			return c.SendStatus(fiber.StatusForbidden)
		}
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set("Referrer-Policy", "same-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		method := c.Method()
		if method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions {
			return c.Next()
		}

		origin := strings.TrimRight(c.Get(fiber.HeaderOrigin), "/")
		if origin != "" {
			if configuredOrigin != "" {
				if !strings.EqualFold(origin, configuredOrigin) {
					return rejectOrigin()
				}
			} else if parsed, err := url.Parse(origin); err != nil || !strings.EqualFold(parsed.Host, c.Get("Host")) {
				return rejectOrigin()
			}
			return c.Next()
		}

		referer := c.Get(fiber.HeaderReferer)
		if referer == "" {
			// Neither Origin nor Referer was sent on a state-changing request.
			// Fail closed instead of allowing it through unchecked.
			return rejectOrigin()
		}
		parsed, err := url.Parse(referer)
		if err != nil || !strings.EqualFold(parsed.Host, c.Get("Host")) {
			return rejectOrigin()
		}
		return c.Next()
	}
}
