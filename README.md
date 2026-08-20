# Mimic Backup Systems

**Current interface version:** `0.8.1`

Mimic is a platform for automating, versioning, auditing, and centralizing configuration backups for network equipment such as switches, routers, OLTs, and firewalls. It connects to devices over SSH, captures their configurations, compares versions, and keeps a searchable history for operational visibility, compliance, and disaster recovery.

## Overview

Modern network environments need reliable configuration history, clear change tracking, and quick visibility into backup failures. Mimic schedules configuration captures, stores meaningful backup versions, highlights drift, and helps teams understand which devices need action.

The interface is designed around operational workflows: dashboard triage, node inventory, settings management, CSV import/export, backup history, security rule auditing, alert routing, and SFTP synchronization.

## Key Features

- **Automated Backup Routines:** Schedule backups per node or through reusable routines.
- **Node Inventory:** Manage vendors, groups, tags, credentials, access agents, health status, and schedules.
- **CSV Import/Export:** Import nodes in bulk with validation, delimiter detection, normalized names/groups/tags, and clear row-level feedback.
- **Visual Diff Viewer:** Compare configuration versions and inspect additions/removals.
- **Centralized Dashboard:** Monitor active nodes, failed backups, silent nodes, SFTP sync pending items, upcoming executions, and recent configuration changes.
- **Security Rules Engine:** Audit configurations using vendor-aware rules, severity, regex matching, score penalties, exceptions, and remediation guidance.
- **Golden Config Checks:** Compare device backups against expected baseline configurations.
- **Secure Credential Management:** Store SSH credentials encrypted at rest using AES-GCM.
- **SFTP Synchronization:** Export successful backups to an external SFTP destination.
- **Alerting Rules:** Route drift, failure, recovery, and security notifications to Webhook or Telegram destinations.
- **Role-Based Access Control:** Administrators have full control, Operators manage network operations, Auditors review policies and logs, and Viewers have read-only access.
- **Audit Logs:** Track operational activity and backup/export events.

## Supported Vendors

The current UI and backend normalize the following vendor scopes:

- Cisco
- MikroTik
- Huawei
- Juniper

Security rules can also target all vendors using `*`.

## Prerequisites

- PostgreSQL 15 or higher
- Go runtime for local builds, or Docker for containerized deployment

## Installation

Mimic supports two installation paths:

1. **Docker Installation (Recommended):** Fastest path for running the app and database in containers. See [DOCKER.md](DOCKER.md).
2. **Bare-Metal Installation:** Manual setup for environments that manage PostgreSQL, service files, and reverse proxies directly. See [TUTORIAL.md](TUTORIAL.md).

## First Setup

On first run with an empty database, Mimic redirects to the setup wizard. The wizard verifies database readiness and creates the initial Administrator account. After setup, users authenticate through the login screen.

## Security Rules

The Security Rules Engine automatically evaluates successful backups for security vulnerabilities and compliance deviations.

Rules can target:

- **Authentication / Access:** Plain-text passwords, weak access patterns, Telnet/HTTP exposure.
- **Network / Exposure:** VTY access controls, SNMP community usage, firewall-related checks.
- **Logging / Auditing:** Syslog, NTP, and operational audit requirements.

Rules support vendor filters, group filters, regex matching, context blocks, severities, penalties, remediation text, and per-node exceptions.

## Recent Updates (v0.8.1)

- **Backup Scheduling Fix:** The preferred backup time and weekday (individual nodes and routines) are now actually honored by the scheduler instead of being cosmetic; next-run times are anchored to the configured time of day.
- **Node Form Reliability:** Saving a node with a validation error now redisplays the form with the entered data and an inline error message, instead of a blank error page.
- **Login Hardening:** Failed logins take the same time whether the username exists or not, closing a timing side-channel.
- **CSRF Hardening:** State-changing requests without an Origin or Referer header are now rejected instead of allowed through.
- **Webhook SSRF Guard:** Alert webhook URLs are validated against loopback/private/link-local destinations before every dispatch.
- **Container Hardening:** The Docker image now runs as a non-root user.

## Recent Updates (v0.8.0)

- **Clean B2B Interface:** Removed decorative icons and emoji-style messaging, reduced visual noise, and kept actions readable with text labels.
- **Settings Rework:** Rebuilt Users, SSH Credentials, Backup Routines, SFTP, and Alerting Rules with cards, metrics, search/filter controls, clearer empty states, and focused edit forms.
- **SFTP Backend Hardening:** Added stronger validation for server settings, path normalization, safer remote explorer output, manual export state updates, and clearer scheduled sync status handling.
- **Alerting Rules Rework:** Added provider-aware validation, encrypted destination handling, safer test delivery, rule metrics, and clean routing for Webhook or Telegram notifications.
- **User and Access Safety:** Includes duplicate checks, server-side permission enforcement, live role refresh, last-admin protection, self-delete protection, and validated profile-photo uploads.
- **Credential and Routine Safety:** Added unique-name validation and blocked deletion when nodes still depend on a credential or routine.
- **Template Reliability:** Added template parsing coverage to catch broken views before deployment.

## Extensibility

Mimic is built to support additional network vendors. New drivers can implement vendor-specific SSH commands and configuration normalization while preserving the same backup, diff, audit, and dashboard workflows.

## License

Developed by Mimic Backup Systems.

For bug reports and feature requests, use the project issue tracker.
