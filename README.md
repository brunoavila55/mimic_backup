# Mimic Backup Systems

Mimic is a high-performance platform designed for the automation, versioning, and centralization of configuration backups for network equipment such as switches, routers, OLTs, and firewalls. By establishing secure SSH connections to network devices, Mimic captures their current configurations and maintains a comprehensive history to ensure seamless auditing and disaster recovery.

## Overview

Modern network infrastructures require strict tracking of configuration changes to maintain stability and security. Mimic addresses this need by providing an automated solution that periodically collects configuration states from devices across the network. It intelligently compares new configurations against previous versions, securely storing only meaningful changes. This minimizes storage overhead while providing a clear audit trail of network modifications over time.

## Key Features

- **Automated Backup Routines**: Schedule periodic backups for individual nodes or logical groups, ensuring configuration history is always up to date without manual intervention.
- **Visual Difference Tracking**: Inspect configuration changes easily through an integrated side-by-side differential viewer, allowing administrators to pinpoint exact modifications between versions.
- **Secure Credential Management**: Store and reuse SSH credentials securely. All sensitive information is encrypted at rest using industry-standard AES-GCM 256-bit encryption.
- **Centralized Dashboard**: Monitor the health of your backup routines through a comprehensive dashboard that highlights recent activities, scheduled tasks, and execution failures.
- **SFTP Synchronization**: Automatically mirror collected configuration backups to external SFTP servers for off-site disaster recovery compliance.
- **Role-Based Access Control**: Manage system access through distinct user roles, ensuring that only authorized personnel can initiate backups or alter system settings.
- **Comprehensive Audit Logs**: Maintain a centralized log of all system activities, including backup executions, credential modifications, and synchronization events.

## Prerequisites

- PostgreSQL 15 or higher

## Installation and Configuration

Mimic offers two official installation paths to suit your infrastructure. Choose your preferred method:

1. **Bare-Metal Installation (Classic):** Ideal for those who want full control over the server, manually installing native dependencies (Postgres, Nginx).
   👉 **[View Manual Installation Guide (TUTORIAL.md)](TUTORIAL.md)**

2. **Docker Installation (Recommended):** The fastest and most modern way. Spins up the application and database in isolated containers with a single command.
   👉 **[View Docker Installation Guide (DOCKER.md)](DOCKER.md)**

## First Setup

Upon the first execution with an empty database, the system will automatically restrict access and redirect to the initial setup wizard. This guided process ensures that the database connection is healthy and allows the creation of the primary Administrator account. Once completed, the system will redirect to the standard authentication screen.

## Extensibility

Mimic is built with extensibility in mind. Support for new hardware vendors can be introduced seamlessly by implementing the standard driver interface. This allows the system to send vendor-specific commands to retrieve configurations and apply custom normalization rules to filter out volatile data (such as uptime or dynamic timestamps) before version comparison.

## Recent Updates (v0.5.0)

- **Interactive SSH Shell Refactor:** Upgraded the SSH service from the basic "exec" channel to use an Interactive Shell with a Pseudo-Terminal (PTY). This natively resolves `--More--` pagination timeouts for complex network vendors like Cisco, Huawei, and Juniper by sending preparation commands (`terminal length 0`, etc.) in the same session.
- **Security Hardening:** Implemented Role-Based Access Control (RBAC) to ensure "Viewer" roles cannot execute destructive or state-modifying actions. Upgraded session cookie security (HTTPOnly & Strict SameSite). SFTP path traversal protection was also introduced.
- **CSV Import/Export Rework:** Rebuilt the CSV functionality to support all new scheduling and relationship fields. Users can now seamlessly import configurations by writing the *names* of Credentials, Routines, and Agents directly in the CSV instead of needing internal IDs.
- **Premium UI Refinements:** The node registration and routine interfaces have been redesigned with a modern, clean aesthetic.
- **Smart Grouping:** The node groups field now features an intelligent autocomplete datalist that suggests previously used groups.
- **Timezone Support for Self-Hosted Deployments:** Added native support for configuring the application timezone via the `.env` file, ensuring scheduled tasks execute accurately according to the server's local time.
- **Improved Backup Scheduling:** Replaced free-text time inputs with intuitive dropdown selections to streamline routine creation.

## License

Developed by Mimic Backup Systems.
For bug reports and feature requests, please utilize the standard issue tracking system.
