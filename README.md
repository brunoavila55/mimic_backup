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

Clone the repository to your local environment:

```bash
git clone https://github.com/brunoavila55/mimic_backup.git
cd mimic_backup
```

Configure the required environment variables by creating a `.env` file in the root directory:

```bash
DATABASE_URL=postgres://username:password@localhost:5432/mimic_db?sslmode=disable
SECRET_KEY=a-random-32-character-secret-key
```

*Note: The `SECRET_KEY` must be at least 32 characters long and is strictly required to enable credential encryption.*

### Manual Execution

Compile the application binary and execute it:

```bash
go mod tidy
go build -o mimic ./cmd/mimic/main.go
./mimic
```

### Docker Execution (Recommended)

Mimic fully supports containerized execution via Docker, which significantly simplifies the deployment process and automatically provisions the PostgreSQL database. This is particularly useful for modern deployment strategies like Green/Blue deployments.

To start the application along with its database, ensure you have Docker and Docker Compose installed, then run the following command in the root directory:

```bash
docker-compose up --build -d
```

The system will be accessible at `http://localhost:3000`. The database connection string and required environment variables are already pre-configured within the `docker-compose.yml` file.

## First Setup

Upon the first execution with an empty database, the system will automatically restrict access and redirect to the initial setup wizard. This guided process ensures that the database connection is healthy and allows the creation of the primary Administrator account. Once completed, the system will redirect to the standard authentication screen.

## Extensibility

Mimic is built with extensibility in mind. Support for new hardware vendors can be introduced seamlessly by implementing the standard driver interface. This allows the system to send vendor-specific commands to retrieve configurations and apply custom normalization rules to filter out volatile data (such as uptime or dynamic timestamps) before version comparison.

## License

Developed by Mimic Backup Systems.
For bug reports and feature requests, please utilize the standard issue tracking system.
