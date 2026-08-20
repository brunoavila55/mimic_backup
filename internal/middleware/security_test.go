package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSecurityHeadersAndOrigin(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeadersAndOrigin())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Post("/save", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	getReq := httptest.NewRequest(fiber.MethodGet, "/", nil)
	getReq.Host = "mimic.local"
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := getResp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}

	crossOrigin := httptest.NewRequest(fiber.MethodPost, "/save", nil)
	crossOrigin.Host = "mimic.local"
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	resp, err := app.Test(crossOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected cross-origin POST to be forbidden, got %d", resp.StatusCode)
	}

	sameOrigin := httptest.NewRequest(fiber.MethodPost, "/save", nil)
	sameOrigin.Host = "mimic.local"
	sameOrigin.Header.Set("Origin", "http://mimic.local")
	resp, err = app.Test(sameOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected same-origin POST to succeed, got %d", resp.StatusCode)
	}

	noHeaders := httptest.NewRequest(fiber.MethodPost, "/save", nil)
	noHeaders.Host = "mimic.local"
	resp, err = app.Test(noHeaders)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected POST without Origin/Referer to be forbidden, got %d", resp.StatusCode)
	}
}
