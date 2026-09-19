package middleware

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"mimic/internal/access"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

func TestRequireAPIAuthReturnsJSON401WithoutRedirect(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/auth/me", RequireAPIAuth(session.New(), nil), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/auth/me", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", response.StatusCode)
	}
	if response.Header.Get("Location") != "" {
		t.Fatal("API authentication must not redirect")
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "application/json") || !strings.Contains(string(body), `"code":"unauthenticated"`) {
		t.Fatalf("unexpected API error response: %s", body)
	}
}

func TestRequireAPIPermissionReturnsJSON403(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("role", access.RoleViewer)
		return c.Next()
	})
	app.Get("/api/v1/admin", RequireAPIPermission(access.ManageUsers), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/admin", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != fiber.StatusForbidden || !strings.Contains(string(body), `"code":"forbidden"`) {
		t.Fatalf("unexpected response %d: %s", response.StatusCode, body)
	}
}
