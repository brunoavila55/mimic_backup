# Mimic Backup Installation with Docker

Installation via Docker is the fastest and recommended way for modern environments, as it packages the PostgreSQL database and the application pre-configured in isolated containers.

## Prerequisites
- [Docker](https://docs.docker.com/get-docker/) installed on the server.
- [Docker Compose](https://docs.docker.com/compose/install/) installed (or `docker compose` in newer Docker versions).

---

## 1. Clone the Repository
Download the code to the server where you want to run it:

```bash
git clone https://github.com/brunoavila55/mimic_backup.git
cd mimic_backup
```

## 2. Optional Configurations (Environment Variables)
The `docker-compose.yml` file comes with all credentials and addresses correctly pointed to the database container (named `db`).

If you want to change the **timezone**, **database password**, or **secret key**, we recommend creating an `.env` file based on our example:

```bash
cp .env.example .env
```
Open the `.env` file in your preferred editor (like `nano` or `vim`) and update the `TZ` variable to match your local timezone (e.g., `America/New_York`), and optionally define your `SECRET_KEY`.

> **Note on SECRET_KEY:** The `SECRET_KEY` is automatically generated on the first run and persisted securely. If you prefer to manage it manually (e.g., in a distributed Docker environment), define the `SECRET_KEY` in the `.env` file.

## 3. Spin Up the Containers
While inside the `mimic_backup` folder, run the command:

```bash
docker compose up --build -d
```
*Note: If you are using an older version of Docker, the command might be `docker-compose up --build -d` (with a hyphen).*

Docker will download the Postgres image, compile the Mimic code locally (thanks to our multi-stage `Dockerfile`), and start both containers. The `-d` ensures the containers run in the background (detached mode).

## 4. Check the Logs
If you want to make sure the system and database started correctly, look at the logs:
```bash
docker compose logs -f
```
To exit the logs, press `Ctrl+C`.

## 5. Access
All set! Access `http://localhost:3000` in your browser (or replace `localhost` with your server's IP). On the first screen, you'll go through the **First Setup** to create your first administrator.

## Updating (Redeploy)
When a new version of Mimic is released and you want to update, just do the following process in the same folder:
```bash
git pull
docker compose down
docker compose up --build -d
```
Your database will not be lost, as the data is persisted in a Docker Volume configured in `docker-compose.yml`.
