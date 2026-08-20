package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"mimic/internal/access"
	"mimic/internal/handlers"
	"mimic/internal/middleware"
	"mimic/internal/models"
	"mimic/internal/services/scheduler"
	"mimic/internal/services/sftp"
	appcrypto "mimic/pkg/crypto"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var AppVersion string

func assetFingerprint(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}

	sum := sha256.Sum256(contents)
	return fmt.Sprintf("%x", sum[:8])
}

func main() {

	fmt.Println(`    __  ____           _       `)
	fmt.Println(`   /  |/  (_)___ ___  (_)____  `)
	fmt.Println(`  / /|_/ / / __ ` + "`" + `__ \/ / ___/  `)
	fmt.Println(` / /  / / / / / / / / / /__    `)
	fmt.Println(`/_/  /_/_/_/ /_/ /_/_/\___/  Backup Systems v0.8.1`)
	fmt.Println("===================================================")

	// Database connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if err := appcrypto.ValidateSecretKey(); err != nil {
		log.Fatalf("invalid encryption key configuration: %v", err)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Auto Migrate - ensures schema is always current
	if err := db.AutoMigrate(
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
		&models.GoldenConfig{},
		&models.SecurityViolation{},
		&models.NodeRuleException{},
	); err != nil {
		log.Fatalf("failed to migrate database schema: %v", err)
	}
	for _, statement := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username)) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_node_backups_success_version ON node_backups (node_id, version) WHERE deleted_at IS NULL AND status = 'success'",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_security_violations_node_rule ON security_violations (node_id, rule_id) WHERE deleted_at IS NULL",
		"CREATE INDEX IF NOT EXISTS idx_node_backups_lookup ON node_backups (node_id, status, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_system_logs_created_at ON system_logs (created_at DESC)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			log.Fatalf("failed to install database integrity constraint: %v", err)
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to initialize database pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Ensure at least one SFTP settings record exists
	var sftpCount int64
	if err := db.Model(&models.SftpSettings{}).Count(&sftpCount).Error; err != nil {
		log.Fatalf("failed to inspect SFTP settings: %v", err)
	}
	if sftpCount == 0 {
		if err := db.Create(&models.SftpSettings{Port: 22}).Error; err != nil {
			log.Fatalf("failed to initialize SFTP settings: %v", err)
		}
	}

	// Session Store
	store := session.New(session.Config{
		Expiration:     24 * time.Hour,
		CookieHTTPOnly: true,
		CookieSameSite: "Strict",
		CookieSecure:   strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),
	})

	// Scheduler
	sch := scheduler.NewScheduler(db)
	sch.Start()

	// Get Version from Git or Build Flags
	appVersion := AppVersion
	if appVersion == "" {
		appVersion = "0.8.1"
	}

	// Template Engine
	engine := html.New("./templates", ".html")
	engine.Reload(true)
	engine.AddFunc("AppVersion", func() string {
		return appVersion
	})
	assetVersions := map[string]string{
		"/static/css/style.css": assetFingerprint("./static/css/style.css"),
		"/static/css/login.css": assetFingerprint("./static/css/login.css"),
	}
	engine.AddFunc("AssetVersion", func(path string) string {
		if version, ok := assetVersions[path]; ok {
			return version
		}
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
		Views:     engine,
		AppName:   "Mimic Backup Systems v0.8.1",
		BodyLimit: 8 * 1024 * 1024,
	})
	app.Use(middleware.SecurityHeadersAndOrigin())
	app.Get("/health", func(c *fiber.Ctx) error {
		if err := sqlDB.Ping(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unhealthy"})
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Static Files
	app.Static("/static", "./static")

	// Handlers
	sftpService := &sftp.SftpService{DB: db}
	setupHandler := &handlers.SetupHandler{DB: db, Store: store}
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
	setupLimiter := limiter.New(limiter.Config{Max: 10, Expiration: time.Minute})
	app.Post("/setup/superuser", setupLimiter, setupHandler.PostCreateSuperuser)

	// ── Auth Routes (public) ──────────────────────────
	app.Get("/login", authHandler.GetLogin)
	loginLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).Render("login", fiber.Map{
				"Title": "Login", "Error": "Too many login attempts. Try again in one minute.",
			})
		},
	})
	app.Post("/login", loginLimiter, authHandler.PostLogin)
	app.Post("/logout", authHandler.Logout)

	// ── Auth Middleware ───────────────────────────────
	app.Use(middleware.RequireAuth(store, db))

	// ── Dashboard ─────────────────────────────────────
	app.Get("/", dashboardHandler.GetDashboard)

	// ── Nodes ─────────────────────────────────────────
	app.Get("/nodes", nodeHandler.ListNodes)
	app.Get("/nodes/new", middleware.RequirePermission(access.ManageNodes), formHandler.NewNode)
	app.Get("/nodes/import", middleware.RequirePermission(access.ManageNodes), formHandler.ImportNodesForm)
	app.Post("/nodes/import", middleware.RequirePermission(access.ManageNodes), formHandler.ImportNodesCSV)
	app.Get("/nodes/export", middleware.RequirePermission(access.ManageNodes), nodeHandler.ExportNodesCSV)
	app.Get("/nodes/:id", nodeHandler.NodeDetails)
	app.Get("/nodes/:id/edit", middleware.RequirePermission(access.ManageNodes), formHandler.EditNode)
	app.Get("/nodes/:id/delete", middleware.RequirePermission(access.ManageNodes), formHandler.DeleteNodeConfirm)
	app.Post("/nodes/save", middleware.RequirePermission(access.ManageNodes), formHandler.SaveNode)
	app.Post("/nodes/save/:id", middleware.RequirePermission(access.ManageNodes), formHandler.SaveNode)
	app.Post("/nodes/:id/snooze", middleware.RequirePermission(access.ManageNodes), formHandler.SnoozeNode)
	app.Post("/nodes/:id/delete", middleware.RequirePermission(access.ManageNodes), formHandler.DeleteNode)
	app.Delete("/nodes/:id", middleware.RequirePermission(access.ManageNodes), formHandler.DeleteNode)
	app.Get("/backups/:id/content", nodeHandler.GetBackupContent)
	app.Get("/backups/:id/diff", nodeHandler.GetBackupDiff)
	app.Get("/nodes/:id/golden-diff", nodeHandler.GoldenDiffView)
	app.Get("/backups/diff/compare", middleware.RequireAdmin(), nodeHandler.CompareBackups)

	// ── Settings Hub ──────────────────────────────────
	app.Get("/settings", settingsHandler.GetSettings)
	app.Get("/settings/users", middleware.RequireAdmin(), settingsHandler.GetUsersTab)
	app.Get("/settings/credentials", middleware.RequirePermission(access.ManageOperations), settingsHandler.GetCredentialsTab)
	app.Get("/settings/routines", middleware.RequirePermission(access.ManageOperations), settingsHandler.GetRoutinesTab)
	app.Get("/settings/sftp", middleware.RequireAdmin(), settingsHandler.GetSFTPTab)
	app.Get("/settings/sftp/explore", middleware.RequireAdmin(), settingsHandler.GetSFTPExplore)
	app.Get("/settings/export", middleware.RequirePermission(access.ExportBackups), settingsHandler.GetExportTab)
	app.Get("/settings/alerts", middleware.RequireAdmin(), settingsHandler.GetAlertsTab)
	app.Get("/settings/logs", middleware.RequirePermission(access.ViewAudit), settingsHandler.GetLogsTab)
	app.Get("/settings/security", middleware.RequirePermission(access.ViewAudit), settingsHandler.GetSecurityTab)
	app.Get("/settings/golden", middleware.RequirePermission(access.ViewAudit), settingsHandler.GetGoldenTab)
	app.Get("/settings/profile", settingsHandler.GetProfileTab)

	// ── Settings Forms ────────────────────────────────
	app.Get("/settings/users/new", middleware.RequireAdmin(), formHandler.NewUser)
	app.Get("/settings/users/:id/edit", middleware.RequireAdmin(), formHandler.EditUser)
	app.Get("/settings/credentials/new", middleware.RequirePermission(access.ManageOperations), formHandler.NewCredential)
	app.Get("/settings/credentials/:id/edit", middleware.RequirePermission(access.ManageOperations), formHandler.EditCredential)
	app.Get("/settings/routines/new", middleware.RequirePermission(access.ManageOperations), formHandler.NewRoutine)
	app.Get("/settings/routines/:id/edit", middleware.RequirePermission(access.ManageOperations), formHandler.EditRoutine)
	app.Get("/settings/alerts/new", middleware.RequireAdmin(), formHandler.NewAlertRule)
	app.Get("/settings/alerts/:id/edit", middleware.RequireAdmin(), formHandler.EditAlertRule)
	app.Get("/settings/security/new", middleware.RequireAdmin(), formHandler.NewSecurityRule)
	app.Get("/settings/security/:id/edit", middleware.RequireAdmin(), formHandler.EditSecurityRule)

	// ── Settings Actions ──────────────────────────────
	app.Post("/settings/users/save", middleware.RequireAdmin(), formHandler.SaveUser)
	app.Post("/settings/users/save/:id", middleware.RequireAdmin(), formHandler.SaveUser)
	app.Post("/settings/credentials/save", middleware.RequirePermission(access.ManageOperations), formHandler.SaveCredential)
	app.Post("/settings/credentials/save/:id", middleware.RequirePermission(access.ManageOperations), formHandler.SaveCredential)
	app.Post("/settings/routines/save", middleware.RequirePermission(access.ManageOperations), formHandler.SaveRoutine)
	app.Post("/settings/routines/save/:id", middleware.RequirePermission(access.ManageOperations), formHandler.SaveRoutine)
	app.Post("/settings/alerts/save", middleware.RequireAdmin(), formHandler.SaveAlertRule)
	app.Post("/settings/alerts/save/:id", middleware.RequireAdmin(), formHandler.SaveAlertRule)
	app.Post("/settings/alerts/test", middleware.RequireAdmin(), formHandler.TestAlertRule)
	app.Get("/settings/golden/new", middleware.RequireAdmin(), formHandler.GoldenForm)
	app.Get("/settings/golden/:id/edit", middleware.RequireAdmin(), formHandler.GoldenForm)
	app.Post("/settings/golden/save", middleware.RequireAdmin(), formHandler.SaveGoldenConfig)
	app.Post("/settings/golden/save/:id", middleware.RequireAdmin(), formHandler.SaveGoldenConfig)
	app.Post("/settings/security/save", middleware.RequireAdmin(), formHandler.SaveSecurityRule)
	app.Post("/settings/security/save/:id", middleware.RequireAdmin(), formHandler.SaveSecurityRule)
	app.Post("/nodes/:id/exceptions/:rule_id", middleware.RequireAdmin(), formHandler.AddRuleException)

	// ── Delete Actions ────────────────────────────────
	app.Delete("/settings/users/:id", middleware.RequireAdmin(), formHandler.DeleteUser)
	app.Delete("/settings/credentials/:id", middleware.RequirePermission(access.ManageOperations), formHandler.DeleteCredential)
	app.Delete("/settings/routines/:id", middleware.RequirePermission(access.ManageOperations), formHandler.DeleteRoutine)
	app.Delete("/settings/alerts/:id", middleware.RequireAdmin(), formHandler.DeleteAlertRule)
	app.Delete("/settings/golden/:id", middleware.RequireAdmin(), formHandler.DeleteGoldenConfig)
	app.Delete("/settings/security/:id", middleware.RequireAdmin(), formHandler.DeleteSecurityRule)
	app.Delete("/nodes/:id/exceptions/:rule_id", middleware.RequireAdmin(), formHandler.RemoveRuleException)
	app.Post("/settings/sftp/save", middleware.RequireAdmin(), formHandler.SaveSettings)
	app.Post("/settings/sftp/test", middleware.RequireAdmin(), formHandler.TestSFTPConnection)
	app.Post("/settings/profile/save", formHandler.SaveProfile)
	app.Post("/settings/export/sync", middleware.RequirePermission(access.ExportBackups), formHandler.PostSync)
	app.Post("/backups/:backup_id/export", middleware.RequirePermission(access.ExportBackups), formHandler.ExportBackup)

	// ── HTMX Actions ──────────────────────────────────
	app.Post("/trigger-backups", middleware.RequirePermission(access.RunBackups), func(c *fiber.Ctx) error {
		if !sch.StartBackupCycle() {
			c.Set("HX-Trigger", `{"showNotification": {"message": "A backup cycle is already running", "type": "warning"}}`)
			return c.SendStatus(fiber.StatusConflict)
		}
		return c.SendStatus(fiber.StatusAccepted)
	})

	app.Post("/nodes/:id/trigger", middleware.RequirePermission(access.RunBackups), func(c *fiber.Ctx) error {
		id := c.Params("id")
		var node models.Node
		if err := db.Preload("Credential").Preload("AccessAgent").Where("id = ?", id).First(&node).Error; err != nil {
			c.Set("HX-Trigger", `{"showNotification": {"message": "Node not found", "type": "error"}}`)
			return c.SendStatus(fiber.StatusNotFound)
		}
		if !sch.StartBackup(&node) {
			c.Set("HX-Trigger", `{"showNotification": {"message": "A backup for this node is already running", "type": "warning"}}`)
			return c.SendStatus(fiber.StatusConflict)
		}
		c.Set("HX-Trigger", `{"showNotification": {"message": "Manual backup started", "type": "success"}}`)
		return c.SendStatus(200)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("[System] Mimic Engine running on port %s", port)
	log.Printf("[Database] Connected to PostgreSQL")
	log.Printf("[Scheduler] Backup engine started (Interval: 1m)")

	serverErr := make(chan error, 1)
	go func() { serverErr <- app.Listen(fmt.Sprintf(":%s", port)) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("[System] Received %s, shutting down", sig)
	case err := <-serverErr:
		if err != nil {
			log.Printf("[System] HTTP server stopped: %v", err)
		}
	}

	if err := app.Shutdown(); err != nil {
		log.Printf("[System] Graceful HTTP shutdown failed: %v", err)
	}
	sch.Stop()
	if err := sqlDB.Close(); err != nil {
		log.Printf("[Database] Failed to close connection pool: %v", err)
	}
}
