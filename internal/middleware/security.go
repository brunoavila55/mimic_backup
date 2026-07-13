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
					return c.SendStatus(fiber.StatusForbidden)
				}
			} else if parsed, err := url.Parse(origin); err != nil || !strings.EqualFold(parsed.Host, c.Get("Host")) {
				return c.SendStatus(fiber.StatusForbidden)
			}
			return c.Next()
		}

		if referer := c.Get(fiber.HeaderReferer); referer != "" {
			parsed, err := url.Parse(referer)
			if err != nil || !strings.EqualFold(parsed.Host, c.Get("Host")) {
				return c.SendStatus(fiber.StatusForbidden)
			}
		}
		return c.Next()
	}
}
