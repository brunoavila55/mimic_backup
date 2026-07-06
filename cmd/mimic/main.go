package main

import (
	"fmt"
	"log"
	"mimic/internal/handlers"
	"mimic/internal/middleware"
	"mimic/internal/models"
	"mimic/internal/services/scheduler"
	"mimic/internal/services/sftp"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var AppVersion string

func main() {

	fmt.Println(`
    __  ____              _      
   /  |/  (_)___ ___  (_)____
  / /|_/ / / __ ` + "`" + `__ \/ / ___/
 / /  / / / / / / / / / /__  
/_/  /_/_/_/ /_/ /_/_/\___/  Backup Systems v0.5.1
________________________________________________`)

	// Database connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=123456 dbname=mimic_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Auto Migrate - ensures schema is always current
	db.AutoMigrate(
		&models.User{},
		&models.Node{},
		&models.BackupRoutine{},
		&models.NodeBackup{},
		&models.AccessAgent{},
		&models.Credential{},
		&models.SftpSettings{},
		&models.SystemLog{},
		&models.AlertRule{},
		&models.SecurityRule{},
		&models.SecurityViolation{},
		&models.NodeRuleException{},
	)

	// Ensure at least one SFTP settings record exists
	var sftpCount int64
	db.Model(&models.SftpSettings{}).Count(&sftpCount)
	if sftpCount == 0 {
		db.Create(&models.SftpSettings{Port: 22})
	}

	// Session Store
	store := session.New(session.Config{
		Expiration:     24 * time.Hour,
		CookieHTTPOnly: true,
		CookieSameSite: "Strict",
	})

	// Scheduler
	sch := scheduler.NewScheduler(db)
	sch.Start()

	// Get Version from Git or Build Flags
	appVersion := AppVersion
	if appVersion == "" {
		appVersion = "0.5.1" // fallback
		if out, err := exec.Command("git", "rev-list", "--count", "HEAD").Output(); err == nil {
			count := strings.TrimSpace(string(out))
			appVersion = "0.5." + count
		}
	}

	// Template Engine
	engine := html.New("./templates", ".html")
	engine.Reload(true)
	engine.AddFunc("AppVersion", func() string {
		return appVersion
	})
	engine.AddFunc("seq", func(start, end int) []int {
		s := make([]int, end-start+1)
		for i := range s {
			s[i] = start + i
		}
		return s
	})
	engine.AddFunc("deref", func(p *uint) uint {
		if p == nil {
			return 0
		}
		return *p
	})
	engine.AddFunc("split", strings.Split)
	engine.AddFunc("trimSpace", strings.TrimSpace)

	app := fiber.New(fiber.Config{
		Views:   engine,
		AppName: "Mimic Backup Systems v0.5.1",
	})

	// Static Files
	app.Static("/static", "./static")

	// Handlers
	sftpService := &sftp.SftpService{}
	setupHandler := &handlers.SetupHandler{DB: db}
	authHandler := &handlers.AuthHandler{DB: db, Store: store}
	dashboardHandler := &handlers.DashboardHandler{DB: db}
	nodeHandler := &handlers.NodeHandler{DB: db}
	settingsHandler := &handlers.SettingsHandler{DB: db, Store: store, Sftp: sftpService}
	formHandler := &handlers.FormHandler{DB: db, Store: store, Sftp: sftpService}

	// ── Setup Middleware ───────────────────────────────
	app.Use(middleware.RequireSetup(db))

	// ── Setup Routes (public) ─────────────────────────
	app.Get("/setup", setupHandler.GetDatabaseSetup)
	app.Post("/setup", setupHandler.PostDatabaseSetup)
	app.Get("/setup/superuser", setupHandler.GetCreateSuperuser)
	app.Post("/setup/superuser", setupHandler.PostCreateSuperuser)

	// ── Auth Routes (public) ──────────────────────────
	app.Get("/login", authHandler.GetLogin)
	app.Post("/login", authHandler.PostLogin)
	app.Post("/logout", authHandler.Logout)

	// ── Auth Middleware ───────────────────────────────
	app.Use(middleware.RequireAuth(store))

	// ── Dashboard ─────────────────────────────────────
	app.Get("/", dashboardHandler.GetDashboard)

	// ── Nodes ─────────────────────────────────────────
	app.Get("/nodes", nodeHandler.ListNodes)
	app.Get("/nodes/new", middleware.RequireAdmin(), formHandler.NewNode)
	app.Get("/nodes/import", middleware.RequireAdmin(), formHandler.ImportNodesForm)
	app.Post("/nodes/import", middleware.RequireAdmin(), formHandler.ImportNodesCSV)
	app.Get("/nodes/export", middleware.RequireAdmin(), nodeHandler.ExportNodesCSV)
	app.Get("/nodes/:id", nodeHandler.NodeDetails)
	app.Get("/nodes/:id/edit", middleware.RequireAdmin(), formHandler.EditNode)
	app.Get("/nodes/:id/delete", middleware.RequireAdmin(), formHandler.DeleteNodeConfirm)
	app.Post("/nodes/save", middleware.RequireAdmin(), formHandler.SaveNode)
	app.Post("/nodes/save/:id", middleware.RequireAdmin(), formHandler.SaveNode)
	app.Post("/nodes/:id/snooze", middleware.RequireAdmin(), formHandler.SnoozeNode)
	app.Post("/nodes/:id/delete", middleware.RequireAdmin(), formHandler.DeleteNode)
	app.Delete("/nodes/:id", middleware.RequireAdmin(), formHandler.DeleteNode)
	app.Get("/backups/:id/content", nodeHandler.GetBackupContent)
	app.Get("/backups/:id/diff", nodeHandler.GetBackupDiff)
	app.Get("/backups/diff/compare", middleware.RequireAdmin(), nodeHandler.CompareBackups)

	// ── Settings Hub ──────────────────────────────────
	app.Get("/settings", middleware.RequireAdmin(), settingsHandler.GetSettings)
	app.Get("/settings/users", middleware.RequireAdmin(), settingsHandler.GetUsersTab)
	app.Get("/settings/credentials", middleware.RequireAdmin(), settingsHandler.GetCredentialsTab)
	app.Get("/settings/routines", middleware.RequireAdmin(), settingsHandler.GetRoutinesTab)
	app.Get("/settings/sftp", middleware.RequireAdmin(), settingsHandler.GetSFTPTab)
	app.Get("/settings/sftp/explore", middleware.RequireAdmin(), settingsHandler.GetSFTPExplore)
	app.Get("/settings/export", middleware.RequireAdmin(), settingsHandler.GetExportTab)
	app.Get("/settings/alerts", middleware.RequireAdmin(), settingsHandler.GetAlertsTab)
	app.Get("/settings/logs", middleware.RequireAdmin(), settingsHandler.GetLogsTab)
	app.Get("/settings/profile", settingsHandler.GetProfileTab)

	// ── Settings Forms ────────────────────────────────
	app.Get("/settings/users/new", middleware.RequireAdmin(), formHandler.NewUser)
	app.Get("/settings/users/:id/edit", middleware.RequireAdmin(), formHandler.EditUser)
	app.Get("/settings/credentials/new", middleware.RequireAdmin(), formHandler.NewCredential)
	app.Get("/settings/credentials/:id/edit", middleware.RequireAdmin(), formHandler.EditCredential)
	app.Get("/settings/routines/new", middleware.RequireAdmin(), formHandler.NewRoutine)
	app.Get("/settings/routines/:id/edit", middleware.RequireAdmin(), formHandler.EditRoutine)
	app.Get("/settings/alerts/new", middleware.RequireAdmin(), formHandler.NewAlertRule)
	app.Get("/settings/alerts/:id/edit", middleware.RequireAdmin(), formHandler.EditAlertRule)
	app.Get("/settings/security/new", middleware.RequireAdmin(), formHandler.NewSecurityRule)
	app.Get("/settings/security/:id/edit", middleware.RequireAdmin(), formHandler.EditSecurityRule)

	// ── Settings Actions ──────────────────────────────
	app.Post("/settings/users/save", middleware.RequireAdmin(), formHandler.SaveUser)
	app.Post("/settings/users/save/:id", middleware.RequireAdmin(), formHandler.SaveUser)
	app.Post("/settings/credentials/save", middleware.RequireAdmin(), formHandler.SaveCredential)
	app.Post("/settings/credentials/save/:id", middleware.RequireAdmin(), formHandler.SaveCredential)
	app.Post("/settings/routines/save", middleware.RequireAdmin(), formHandler.SaveRoutine)
	app.Post("/settings/routines/save/:id", middleware.RequireAdmin(), formHandler.SaveRoutine)
	app.Post("/settings/alerts/save", middleware.RequireAdmin(), formHandler.SaveAlertRule)
	app.Post("/settings/alerts/save/:id", middleware.RequireAdmin(), formHandler.SaveAlertRule)
	app.Post("/settings/alerts/test", middleware.RequireAdmin(), formHandler.TestAlertRule)
	app.Post("/settings/security/save", middleware.RequireAdmin(), formHandler.SaveSecurityRule)
	app.Post("/settings/security/save/:id", middleware.RequireAdmin(), formHandler.SaveSecurityRule)
	app.Post("/nodes/:id/exceptions/:rule_id", middleware.RequireAdmin(), formHandler.AddRuleException)

	// ── Delete Actions ────────────────────────────────
	app.Delete("/settings/users/:id", middleware.RequireAdmin(), formHandler.DeleteUser)
	app.Delete("/settings/credentials/:id", middleware.RequireAdmin(), formHandler.DeleteCredential)
	app.Delete("/settings/routines/:id", middleware.RequireAdmin(), formHandler.DeleteRoutine)
	app.Delete("/settings/alerts/:id", middleware.RequireAdmin(), formHandler.DeleteAlertRule)
	app.Delete("/settings/security/:id", middleware.RequireAdmin(), formHandler.DeleteSecurityRule)
	app.Delete("/nodes/:id/exceptions/:rule_id", middleware.RequireAdmin(), formHandler.RemoveRuleException)
	app.Post("/settings/sftp/save", middleware.RequireAdmin(), formHandler.SaveSettings)
	app.Post("/settings/sftp/test", middleware.RequireAdmin(), formHandler.TestSFTPConnection)
	app.Post("/settings/profile/save", formHandler.SaveProfile)
	app.Post("/settings/export/sync", middleware.RequireAdmin(), formHandler.PostSync)
	app.Post("/backups/:backup_id/export", middleware.RequireAdmin(), formHandler.ExportBackup)

	// ── HTMX Actions ──────────────────────────────────
	app.Post("/trigger-backups", middleware.RequireAdmin(), func(c *fiber.Ctx) error {
		sch.CheckBackups()
		return c.SendStatus(200)
	})

	app.Post("/nodes/:id/trigger", middleware.RequireAdmin(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		var node models.Node
		if err := db.Preload("Credential").Preload("AccessAgent").Where("id = ?", id).First(&node).Error; err == nil {
			go sch.RunBackup(&node)
			c.Set("HX-Trigger", `{"showNotification": {"message": "Manual backup started", "type": "success"}}`)
		}
		return c.SendStatus(200)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("[System] Mimic Engine running on port %s", port)
	log.Printf("[Database] Connected to PostgreSQL")
	log.Printf("[Scheduler] Backup engine started (Interval: 1m)")

	log.Fatal(app.Listen(fmt.Sprintf(":%s", port)))
}
