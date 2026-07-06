# Project Context: Mimic Backup Systems v0.5.X

This document serves as a centralized reference for developers and AI agents to understand the current state, architecture, and project decisions.

## 📌 Overview

**Mimic** is an automation system for backing up network equipment (MikroTik, Cisco, Huawei, Juniper). It connects via SSH, collects configurations, normalizes the text, and stores versions in the database with a SHA-256 hash for deduplication.

## 🏗️ Architecture

- **Backend**: Go 1.25 + **Fiber v2** framework.
- **Frontend**: **Go Templates** rendered on the server + **HTMX** for reactive interactivity + **Alpine.js** for UI states (modals, mobile menus). The entire interface and text are in **English**.
- **Styling**: Custom CSS with a neutral/dark design system (Inter + JetBrains Mono). Mobile responsive. No external CSS frameworks.
- **Versioning**: The system dynamically counts the number of commits (`git rev-list`) on boot and injects the version (`v0.5.X`) into all templates via the `AppVersion` function.
- **ORM**: **GORM** with PostgreSQL driver.
- **Concurrency**: Goroutines with a Worker Pool for parallel backups.
- **Cryptography**: AES-GCM 256-bit for network passwords and SSH credentials (`pkg/crypto` package).
- **Session**: Fiber session middleware with cookies + bcrypt for authentication.

## 📂 Folder Structure

```
cmd/mimic/main.go          # Entry point, routes, middleware, scheduler
internal/
  handlers/
    auth.go                    # Login, logout (AuthHandler)
    handlers.go                # Dashboard, Nodes, Settings hub
    forms.go                   # Full CRUD: nodes, users, credentials, routines, SFTP, profile, export
    setup.go                   # Setup wizard: DB confirmation + superuser creation
  middleware/
    auth.go                    # RequireSetup (first access) + RequireAuth (session)
  models/
    models.go                  # User, Node, NodeBackup, BackupRoutine, AccessAgent, Credential, SftpSettings, SystemLog
  services/
    ssh/                       # Native SSH Engine (Interactive Shell + PTY)
      vendors/                 # Per-vendor drivers (mikrotik, cisco, etc.)
    scheduler/                 # Internal scheduler (checks NextBackupAt every 1 min)
    sftp/                      # Backup export to SFTP server
pkg/crypto/                    # AES-GCM encrypt/decrypt + bcrypt helpers
pkg/diff/                      # Pure Go text diff algorithm (Myers-like LCS)
templates/                     # Go Templates (.html)
  base.html                    # Main layout (sidebar + header + content + mobile overlay)
  login.html                   # Login page (standalone)
  setup_database.html          # Setup step 1 — DB confirmation
  setup_superuser.html         # Setup step 2 — Admin creation
  dashboard.html               # Dashboard with stats
  node_list.html               # Node list with search
  node_details.html            # Node details + backup history
  node_form.html               # Create/edit node
  node_confirm_delete.html     # Deletion confirmation
  settings.html                # Settings hub with vertical tabs
  credential_form.html         # Create/edit SSH credential
  user_form.html               # Create/edit user
  routine_form.html            # Create/edit routine
  partials/                    # HTMX fragments
    dashboard_stats.html       # Stats + recent activity
    node_table.html            # Node table
    node_table_body.html       # Table rows
    backup_view.html           # Backup viewer (modal)
    diff_view.html             # Diff viewer with HTMX and modal
    diff_body.html             # Tabular structure containing differences
    settings_users.html        # Tab: users
    settings_credentials.html  # Tab: credentials
    settings_routines.html     # Tab: routines
    settings_sftp.html         # Tab: SFTP config
    settings_export.html       # Tab: export
    settings_logs.html         # Tab: system logs
    settings_profile.html      # Tab: personal profile
static/css/style.css           # Full design system (mobile responsive)
```

## 📊 Models (GORM)

| Model | Description |
|-------|-------------|
| `User` | System users (username, email, bcrypt password, role, avatar) |
| `Node` | Network equipment (name, IP, vendor, credentials, schedule, status) |
| `NodeBackup` | Backup version (config, SHA-256 hash, status, incremental version) |
| `BackupRoutine` | Reusable schedule (frequency, time, day of week) |
| `Credential` | Reusable SSH credential (name, username, AES-GCM password, port) |
| `AccessAgent` | Legacy — access agent (kept for compatibility) |
| `SftpSettings` | SFTP server configuration for export |
| `SystemLog` | System activity log (level, category, message) |

## ⚙️ Operation Flow

### First Access (Setup Wizard)
1. App starts → `AutoMigrate` creates tables → `RequireSetup` detects 0 users.
2. Redirects to `/setup` — visual database confirmation.
3. Redirects to `/setup/superuser` — admin creation form.
4. After creation, redirects to `/login`.

### Normal Operation
1. **Registration**: User creates a `Node` with IP, vendor, and credentials (direct or via `Credential`).
2. **Cryptography**: Passwords encrypted with `SECRET_KEY` (auto-generated and persisted in `.mimic_secret` if not in env) via AES-GCM before saving to Postgres.
3. **Security**: DB queries use parameterized queries (`Where("id = ?", id)`) to prevent SQL Injection.
4. **Scheduling**: The `Scheduler` checks `NextBackupAt` every minute.
5. **Execution**: Goroutine opens an Interactive SSH Shell via PTY, identifies the driver in `ssh/vendors`, executes preparation commands (to bypass pagination like `--More--`), executes collection command, normalizes via RegEx, saves `NodeBackup` if the SHA-256 hash changed.
6. **Export**: User can send backups to SFTP (individual or bulk sync).

## 🔐 Middleware

| Middleware | Description |
|-----------|-------------|
| `RequireSetup` | Redirects to `/setup` if no users exist. Caches result on success. |
| `RequireAuth` | Checks authenticated session. For HTMX, returns `HX-Redirect` header. |

**Order**: Static Files → RequireSetup → Setup/Auth Routes → RequireAuth → Protected Routes.

## 🎨 Design System

- **Theme**: Professional neutral dark (no glassmorphism, gradients, or glow).
- **Palette**: `#0f1117` (bg) → `#232730` (hover), accent `#3b82f6` (blue).
- **Typography**: Inter (UI) + JetBrains Mono (IPs, configs).
- **Components**: `.card`, `.btn`, `.form-input`, `.table-wrap`, `.badge`, `.stat-card`, `.settings-layout`.
- **Layout**: Fixed sidebar (240px) + main content scrollable. Mobile responsive with off-canvas hamburger menu and swipeable tables.

## 🔌 Settings Hub

The `/settings` route is a unified hub with **7 tabs** navigable via HTMX:

| Tab | Route | Content |
|-----|-------|---------|
| Users | `/settings/users` | Users CRUD + roles (Admin/Viewer) |
| Credentials | `/settings/credentials` | Reusable SSH credentials CRUD |
| Routines | `/settings/routines` | Backup schedules CRUD |
| SFTP | `/settings/sftp` | SFTP server configuration |
| Export | `/settings/export` | Bulk sync + per node status |
| Logs | `/settings/logs` | Last 200 system logs |
| Profile | `/settings/profile` | Edit username, email, password |

Each tab uses `hx-get` to load partials without reloading. Direct URL navigation also works (full page render).

## 🚀 How to Continue Development

- **New Vendors**: Create a new file in `internal/services/ssh/vendors/` implementing the `Driver` interface (with `GetPrepCommands` and `GetBackupCommand`) and registering it in `init()`.
- **New Pages**: Create template in `templates/`, handler in `internal/handlers/`, route in `main.go`.
- **New Settings Tabs**: Create partial in `templates/partials/settings_*.html`, method in `SettingsHandler`, GET route in `main.go`, and add the link in `settings.html`.
- **Template Functions**: `seq(start, end)` and `deref(*uint)` are registered in the engine.

## ⚠️ Notes

- The `SECRET_KEY` is mandatory for encryption. If not defined via env var, the system generates one securely on first boot and saves it in `.mimic_secret`.
- `AutoMigrate` runs on startup — adding fields to models is safe (it never drops columns).
- Legacy scripts (`translate.go`, `fix_ui.go`, etc) that caused `main` package collisions were permanently removed.
- `AccessAgent` is legacy; new developments should use `Credential`.
- Installation works with a single Go binary.

---
*Document updated on July 6, 2026.*
