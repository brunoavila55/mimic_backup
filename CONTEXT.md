# Project Context: Mimic Backup Systems

This document serves as a centralized reference for developers and AI agents to understand the current state, architecture, code organization, and feature functionalities of Mimic.

## 📌 Overview

**Mimic** is an automation system focused on versioning and centralizing the backup of configurations for network equipment (such as MikroTik routers, Cisco switches, Huawei OLTs, Juniper, etc.). Instead of network administrators manually accessing each equipment to export configurations, Mimic does this on a scheduled basis.

**How the core works:**
1. **Connection**: Mimic uses SSH credentials (stored securely using AES-GCM encryption) to connect to devices.
2. **Collection & Normalization**: It runs vendor-specific commands to extract text configurations and removes volatile data (like uptime) via Regex to avoid false positives in differences.
3. **Deduplication & Diffing**: The text is hashed using SHA-256. The system compares this hash with the latest saved version. If the hash is identical, no action is taken. If different, a new backup version is saved in the PostgreSQL database, and administrators can see exactly what changed via a built-in diff viewer.

## 🏗️ Architecture & Code Explanation

- **Backend**: Go 1.25 + **Fiber v2** framework. The `cmd/mimic/main.go` file acts as the entry point, defining routes, middleware, and starting the internal scheduler.
- **Frontend**: **Go Templates** (`templates/`) rendered server-side + **HTMX** for reactive interactivity without page reloads + **Alpine.js** for client-side UI states (modals, mobile menus). The entire interface is in **English**.
- **Styling**: Custom CSS (`static/css/style.css`) utilizing a neutral/dark design system (Inter + JetBrains Mono fonts) without external CSS frameworks.
- **ORM**: **GORM** with PostgreSQL driver (`internal/models/models.go`). Uses parameterized queries for security.
- **Concurrency**: Goroutines with a Worker Pool for parallel backup execution.
- **Cryptography**: The `pkg/crypto` package uses AES-GCM 256-bit encryption for network passwords/SSH credentials and bcrypt for user authentication. The secret key is stored in `.mimic_secret` or provided via environment variables.
- **Session**: Managed via Fiber session middleware (`internal/middleware/auth.go`) utilizing cookies.
- **Scheduler**: A background service (`internal/services/scheduler/`) that checks the `NextBackupAt` field every minute and spawns worker goroutines.
- **SSH Engine**: Native Go SSH implementation with Interactive Shell and PTY (`internal/services/ssh/vendors/`). Vendor drivers implement preparation commands (to bypass pagination like `--More--`) and backup commands.

## 📂 Folder Structure

```text
cmd/mimic/main.go          # Entry point, routes, middleware, scheduler initialization
internal/
  handlers/
    auth.go                    # Login, logout handling
    handlers.go                # Dashboard, Nodes, Settings hub controllers
    forms.go                   # Forms for CRUD operations (nodes, users, credentials, etc.)
    setup.go                   # Setup wizard for DB confirmation and superuser
  middleware/
    auth.go                    # RequireSetup and RequireAuth protection
  models/
    models.go                  # GORM entities (User, Node, NodeBackup, AlertRule, SecurityRule, etc.)
  services/
    audit/                     # Security rule validation, evaluation, scoring, and violation lifecycle
    ssh/                       # Native SSH Engine
      vendors/                 # Per-vendor drivers (mikrotik, cisco, huawei, etc.)
    scheduler/                 # Internal cron-like scheduler
    sftp/                      # SFTP Export engine
pkg/crypto/                    # AES-GCM and bcrypt encryption helpers
pkg/diff/                      # Pure Go text diff algorithm (Myers-like LCS)
templates/                     # Go HTML Templates
  base.html                    # Main layout
  login.html                   # Standalone login
  dashboard.html               # Main dashboard stats
  node_*.html                  # Node management pages (list, details, form, import)
  settings.html                # Main settings hub
  *_form.html                  # Various entity creation/edit forms
  partials/                    # HTMX fragments for dynamic loading
static/css/                    # Custom stylesheet
```

## 📊 Models & Features Breakdown

The system is data-driven, heavily relying on GORM models. Here is a breakdown of the models and their respective features:

### Core Management
| Model | Feature Details |
|-------|-----------------|
| `User` | **Access Control:** Manages system users (Admin/Viewer) using bcrypt for passwords. The system forces a Setup Wizard on first boot if no users exist. |
| `Credential` | **Reusable Auth:** Instead of defining passwords per node, you create an SSH Credential and link it to hundreds of nodes. When a password changes, you only update it here. Encrypted using AES-GCM. |
| `BackupRoutine` | **Scheduling:** Defines when backups happen (e.g., "Every 24h", "Tuesdays at 02:00"). Linked to nodes. |
| `Node` | **Inventory:** Network equipment. Tracks IP, Vendor, linked Credential, Routine, and `SecurityScore`. Features include manual backup execution, configuration viewing, and diff analysis. |
| `NodeBackup` | **Versioning:** Stores the actual configuration snapshot, version number, SHA-256 hash, and Diff additions/deletions. |

### External Integrations & Logging
| Model | Feature Details |
|-------|-----------------|
| `SftpSettings` | **Disaster Recovery:** Allows mirroring local backups to an external SFTP server, either manually or via bulk sync. |
| `SystemLog` | **Auditing:** Records system activity, internal operations, SSH connection failures, and application errors for troubleshooting. |
| `AlertRule` | **Notifications:** Sends alerts via Webhook or Telegram based on triggers (Diff changes, Failure, Security violations). |

### Security & Compliance
| Model | Feature Details |
|-------|-----------------|
| `SecurityRule` | **Compliance Checking:** Defines enabled/disabled RE2 Regex policies with category, vendor, node-group scope, severity, penalty, optional context block, and remediation guidance. |
| `SecurityViolation` | **Current Findings:** Records active rule failures. Continuing findings retain their original `CreatedAt` while updating the latest backup version. Resolved findings are removed. |
| `NodeRuleException` | **Overrides:** Allows ignoring specific security rules for specific nodes. |
| `GoldenConfig` | **Templates:** Baseline configuration templates grouped by target or vendor for compliance tracking. |

### Security Rules Engine

The audit engine lives in `internal/services/audit/` and separates rule validation/evaluation from database orchestration:

- `ValidateRule` rejects missing names, invalid match types, penalties outside `0-100`, and invalid main/context Regex patterns.
- `EvaluateRule` deterministically evaluates one rule against configuration text and is shared by automated tests.
- `contains` creates a violation when the pattern exists; `not_contains` creates one when the required pattern is absent.
- `ContextBlock` optionally limits matching to an indented configuration section. A missing context counts as a missing pattern.
- Only enabled rules matching `(vendor OR *)` and `(target group OR *)` apply to a node.
- Node-specific exceptions are skipped before evaluation.
- The score starts at `100`, subtracts each active rule penalty, and is clamped at `0`.
- Saving or deleting a rule re-evaluates all nodes with successful backups so stale findings are removed when scope, vendor, status, or logic changes.
- Only newly introduced violations are returned for alerting; continuing violations do not repeatedly trigger new-finding alerts.

## ⚙️ Operation Flow

1. **First Access**: `RequireSetup` middleware detects 0 users -> `/setup` DB check -> `/setup/superuser` creation -> `/login`.
2. **Normal Operation**: User creates `Credential` -> Creates `Node` linked to it.
3. **Execution**: The `Scheduler` identifies it's time for a backup. It spawns a goroutine -> Opens SSH via PTY -> Identifies vendor in `ssh/vendors` -> Runs prep commands -> Runs backup command -> Runs Regex normalizations.
4. **Validation**: Calculates SHA-256 of the new config. If different from the last, it calculates the Diff using `pkg/diff`, runs `SecurityRule` compliance checks, calculates the `SecurityScore`, and saves the `NodeBackup`.
5. **Alerting**: If configured, triggers `AlertRule` webhooks or Telegram messages based on the backup result.
6. **Export**: Backups can be synced to SFTP via `sftp` service.

## 🎨 Frontend & Settings Hub (`/settings`)

The `/settings` route is a unified hub with multiple tabs loaded via **HTMX** for seamless navigation.

- **Users (`/settings/users`)**: User CRUD and role assignment.
- **Credentials (`/settings/credentials`)**: Reusable encrypted SSH credentials.
- **Routines (`/settings/routines`)**: Centralized backup schedules.
- **SFTP (`/settings/sftp`)**: Remote backup mirroring config.
- **Export (`/settings/export`)**: Manual/Bulk sync control for SFTP.
- **Logs (`/settings/logs`)**: System activity logs.
- **Profile (`/settings/profile`)**: Logged-in user's profile and password change.
- **Alerts (`/settings/alerts`)**: Webhook and Telegram notification rules.
- **Security Rules (`/settings/security`)**: Policy catalog with metrics, search, severity/status filters, vendor/group scope, and enabled state.

### Security Rule Builder (`/settings/security/new`, `/settings/security/:id/edit`)

The rule editor is a guided three-step Alpine.js interface designed for administrators who may not be Regex experts:

1. **Policy intent:** Name, risk description, category, severity, and score impact.
2. **Device scope:** Vendor and node group, with syntax guidance for MikroTik RouterOS, Cisco IOS/IOS-XE, Huawei VRP, and Juniper Junos.
3. **Detection logic:** Plain-language `Flag when found` / `Flag when missing` choices, Regex pattern, optional advanced context block, remediation guidance, and enabled state.

The editor includes vendor-aware starter checks for Telnet, NTP, and public SNMP, plus compliant/risky sample configurations. Its live sandbox previews match and compliance state in the browser; the Go engine validates the rule again when it is saved. The CSS asset query in `templates/base.html` must be bumped after significant stylesheet changes to prevent stale browser caching.

The design utilizes a **Professional neutral dark theme** (`#0f1117` background, `#232730` surfaces) focused on performance, with fixed sidebars, mobile hamburger menus, and swipeable tables.

## 🚀 Development Guidelines

- **New Vendors**: Create a file in `internal/services/ssh/vendors/` implementing the `Driver` interface (`GetPrepCommands`, `GetBackupCommand`, `Normalize`) and register it in `init()`.
- **Database Migrations**: GORM's `AutoMigrate` runs on startup, safely adding columns (it does not drop data).
- **Security**: The `SECRET_KEY` is mandatory. Do not commit `.mimic_secret`. All queries must use parameterized binding.

---
*Document last updated in July 2026.*
