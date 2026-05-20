# Mimic Backup Installation

Prepare the Linux server (Ubuntu 24.04 / Debian 12 recommended). Connect via command line and follow the instructions below.

> **Note:** These instructions assume you are the `root` user. If you are not, prepend `sudo` to the shell commands or temporarily become root with `sudo -s` or `sudo -i`.

## 1. Install Required Packages

Update your repositories and install the basic dependencies:
```bash
apt update
apt install curl git postgresql postgresql-contrib nginx-full
```

## 2. Install Go (Golang)
Mimic is written in Go and requires version 1.22+.

```bash
wget https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

## 3. Add System User
Create a dedicated user to run the application for security reasons:
```bash
useradd mimic -d /opt/mimic -M -r -s "$(which bash)"
```

## 4. Download Mimic
Go to the `/opt` folder and clone the official repository:
```bash
cd /opt
git clone https://github.com/brunoavila55/mimic_backup.git mimic
```

## 5. Set Permissions
Assign folder ownership to the user we just created:
```bash
chown -R mimic:mimic /opt/mimic
chmod -R 775 /opt/mimic
```

## 6. Build the System
Switch to the `mimic` user to compile safely:
```bash
su - mimic
cd /opt/mimic
go mod download
go build -o mimic_bin ./cmd/mimic/main.go
exit
```
*(The `exit` command will return your session to the root user)*

## 7. Configure PostgreSQL
Access the database prompt:
```bash
sudo -u postgres psql
```
Inside the database console (prompt `postgres=#`), run the commands below.

> **Warning:** Change `password` to a secure password.
```sql
CREATE DATABASE mimic_db;
CREATE USER mimic WITH ENCRYPTED PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE mimic_db TO mimic;
\q
```

## 8. Configure the Service (Systemd)
To ensure Mimic starts with the server and restarts on failures, let's create a system service:
```bash
cat <<EOF > /etc/systemd/system/mimic.service
[Unit]
Description=Mimic Backup Systems
After=network.target postgresql.service

[Service]
User=mimic
Group=mimic
WorkingDirectory=/opt/mimic
Environment="DATABASE_URL=postgres://mimic:password@localhost:5432/mimic_db?sslmode=disable"
Environment="PORT=3000"
ExecStart=/opt/mimic/mimic_bin
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```
Enable and start the service:
```bash
systemctl daemon-reload
systemctl enable mimic
systemctl start mimic
```

## 9. Configure Web Server (NGINX)
We'll use NGINX as a reverse proxy to receive web connections on port 80 and forward them to our application.

```bash
nano /etc/nginx/sites-available/mimic.conf
```
Add the content below. If you have a domain, change `mimic.example.com`. Otherwise, you can put the server's IP:
```nginx
server {
    listen 80;
    server_name mimic.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```
Enable the configuration and restart NGINX:
```bash
ln -s /etc/nginx/sites-available/mimic.conf /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
systemctl restart nginx
```

## Final Steps
All done! Access `http://mimic.example.com` (or your server's IP) in your browser.
On the first run, the system will redirect you to the **First Setup** screen, where you can create your Administrator user.

> **Security Note:** This tutorial has not covered installing SSL certificates (HTTPS). To expose your server to the internet, we recommend installing **Certbot/Let's Encrypt** to secure the platform.
