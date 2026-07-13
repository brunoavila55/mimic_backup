package handlers

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInitialAdmin(t *testing.T) {
	tests := []struct {
		name     string
		username string
		email    string
		password string
		confirm  string
		wantErr  bool
	}{
		{name: "valid", username: "admin", email: "admin@example.com", password: "secure-pass-123", confirm: "secure-pass-123"},
		{name: "optional email", username: "admin", password: "secure-pass-123", confirm: "secure-pass-123"},
		{name: "missing username", password: "secure-pass-123", confirm: "secure-pass-123", wantErr: true},
		{name: "short username", username: "ab", password: "secure-pass-123", confirm: "secure-pass-123", wantErr: true},
		{name: "username spaces", username: "system admin", password: "secure-pass-123", confirm: "secure-pass-123", wantErr: true},
		{name: "invalid email", username: "admin", email: "invalid", password: "secure-pass-123", confirm: "secure-pass-123", wantErr: true},
		{name: "short password", username: "admin", password: "short", confirm: "short", wantErr: true},
		{name: "password mismatch", username: "admin", password: "secure-pass-123", confirm: "secure-pass-456", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := validateInitialAdmin(tt.username, tt.email, tt.password, tt.confirm)
			if tt.wantErr && errMsg == "" {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && errMsg != "" {
				t.Fatalf("unexpected validation error: %s", errMsg)
			}
		})
	}
}

func TestSetupTemplatesRender(t *testing.T) {
	funcs := template.FuncMap{"AppVersion": func() string { return "test" }}
	tests := []struct {
		file string
		data any
		want []string
	}{
		{
			file: "setup_database.html",
			data: map[string]any{"Status": SetupReadiness{
				DatabaseOK: true, SchemaOK: true, EncryptionOK: true, Ready: true, DatabaseName: "mimic_test",
			}},
			want: []string{"Confirm the environment", "Connected to mimic_test", "Environment ready"},
		},
		{
			file: "setup_superuser.html",
			data: setupAdminView("admin", "admin@example.com", "Review this field"),
			want: []string{"Create your Administrator", `value="admin"`, `value="admin@example.com"`, "Review this field"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "templates", tt.file)
			tmpl, err := template.New(tt.file).Funcs(funcs).ParseFiles(path)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := tmpl.ExecuteTemplate(&output, tt.file, tt.data); err != nil {
				t.Fatal(err)
			}
			for _, expected := range tt.want {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("rendered output does not contain %q", expected)
				}
			}
		})
	}
}
