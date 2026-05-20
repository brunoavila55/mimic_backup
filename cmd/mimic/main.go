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
/_/  /_/_/_/ /_/ /_/_/\___/  Backup Systems v0.1.x
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
	)

	// Ensure at least one SFTP settings record exists
	var sftpCount int64
	db.Model(&models.SftpSettings{}).Count(&sftpCount)
	if sftpCount == 0 {
		db.Create(&models.SftpSettings{Port: 22})
	}

	// Session Store
	store := session.New()

	// Scheduler
	sch := scheduler.NewScheduler(db)
	sch.Start()

	// Get Version from Git or Build Flags
	appVersion := AppVersion
	if appVersion == "" {
		appVersion = "0.0.10" // fallback
		if out, err := exec.Command("git", "rev-list", "--count", "HEAD").Output(); err == nil {
			count := strings.TrimSpace(string(out))
			appVersion = "0.1." + count
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
		AppName: "Mimic Backup Systems v0.1.x",
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
	app.Get("/nodes/new", formHandler.NewNode)
	app.Get("/nodes/import", formHandler.ImportNodesForm)
	app.Post("/nodes/import", formHandler.ImportNodesCSV)
	app.Get("/nodes/export", nodeHandler.ExportNodesCSV)
	app.Get("/nodes/:id", nodeHandler.NodeDetails)
	app.Get("/nodes/:id/edit", formHandler.EditNode)
	app.Get("/nodes/:id/delete", formHandler.DeleteNodeConfirm)
	app.Post("/nodes/save", formHandler.SaveNode)
	app.Post("/nodes/save/:id", formHandler.SaveNode)
	app.Post("/nodes/:id/delete", formHandler.DeleteNode)
	app.Delete("/nodes/:id", formHandler.DeleteNode)
	app.Get("/backups/:id/content", nodeHandler.GetBackupContent)
	app.Get("/backups/:id/diff", nodeHandler.GetBackupDiff)
	app.Get("/backups/diff/compare", nodeHandler.CompareBackups)

	// ── Settings Hub ──────────────────────────────────
	app.Get("/settings", settingsHandler.GetSettings)
	app.Get("/settings/users", settingsHandler.GetUsersTab)
	app.Get("/settings/credentials", settingsHandler.GetCredentialsTab)
	app.Get("/settings/routines", settingsHandler.GetRoutinesTab)
	app.Get("/settings/sftp", settingsHandler.GetSFTPTab)
	app.Get("/settings/export", settingsHandler.GetExportTab)
	app.Get("/settings/logs", settingsHandler.GetLogsTab)
	app.Get("/settings/profile", settingsHandler.GetProfileTab)

	// ── Settings Forms ────────────────────────────────
	app.Get("/settings/users/new", formHandler.NewUser)
	app.Get("/settings/users/:id/edit", formHandler.EditUser)
	app.Get("/settings/credentials/new", formHandler.NewCredential)
	app.Get("/settings/credentials/:id/edit", formHandler.EditCredential)
	app.Get("/settings/routines/new", formHandler.NewRoutine)
	app.Get("/settings/routines/:id/edit", formHandler.EditRoutine)

	// ── Settings Actions ──────────────────────────────
	app.Post("/settings/users/save", formHandler.SaveUser)
	app.Post("/settings/users/save/:id", formHandler.SaveUser)
	app.Delete("/settings/users/:id", formHandler.DeleteUser)
	app.Post("/settings/credentials/save", formHandler.SaveCredential)
	app.Post("/settings/credentials/save/:id", formHandler.SaveCredential)
	app.Delete("/settings/credentials/:id", formHandler.DeleteCredential)
	app.Post("/settings/routines/save", formHandler.SaveRoutine)
	app.Post("/settings/routines/save/:id", formHandler.SaveRoutine)
	app.Delete("/settings/routines/:id", formHandler.DeleteRoutine)
	app.Post("/settings/sftp/save", formHandler.SaveSettings)
	app.Post("/settings/profile/save", formHandler.SaveProfile)
	app.Post("/settings/export/sync", formHandler.PostSync)
	app.Post("/backups/:backup_id/export", formHandler.ExportBackup)

	// ── HTMX Actions ──────────────────────────────────
	app.Post("/trigger-backups", func(c *fiber.Ctx) error {
		sch.CheckBackups()
		return c.SendStatus(200)
	})

	app.Post("/nodes/:id/trigger", func(c *fiber.Ctx) error {
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
