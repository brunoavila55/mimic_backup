package handlers

import (
	"strings"
	"testing"

	"github.com/gofiber/template/html/v2"
)

func TestTemplatesParse(t *testing.T) {
	engine := html.New("../../templates", ".html")
	engine.AddFunc("AppVersion", func() string {
		return "test"
	})
	engine.AddFunc("seq", func(start, end int) []int {
		values := make([]int, end-start+1)
		for i := range values {
			values[i] = start + i
		}
		return values
	})
	engine.AddFunc("deref", func(p *uint) uint {
		if p == nil {
			return 0
		}
		return *p
	})
	engine.AddFunc("split", strings.Split)
	engine.AddFunc("trimSpace", strings.TrimSpace)

	if err := engine.Load(); err != nil {
		t.Fatalf("templates failed to parse: %v", err)
	}
}
