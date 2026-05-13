# Guia de Implantação em Produção — Mimic v2.0

Este tutorial descreve o passo a passo para instalar e configurar o **Mimic** em um ambiente de produção Linux de forma profissional e segura.

---

## 1. Instalação de Dependências

Se você está em um servidor novo (Ubuntu/Debian), instale os requisitos básicos:

### Git e PostgreSQL
```bash
sudo apt update
sudo apt install git postgresql postgresql-contrib -y
```

### Go (Golang)
Recomendamos a instalação da versão estável mais recente:
```bash
# Baixar o instalador (exemplo para v1.22.2)
wget https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz

# Adicionar ao PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verificar instalação
go version
```

### Configuração Inicial do Postgres
Crie o banco de dados para o Mimic:
```bash
sudo -u postgres psql -c "CREATE DATABASE mimic_db;"
sudo -u postgres psql -c "ALTER USER postgres WITH PASSWORD 'sua_senha_aqui';"
```

---

## 2. Pré-requisitos

Antes de começar, certifique-se de que o seu servidor (Ubuntu/Debian recomendado) possui:

- **Go 1.22+**: Necessário para compilar o binário.
- **PostgreSQL 15+**: Banco de dados para armazenar configurações e backups.
- **Git**: Para clonar e atualizar o repositório.

---

## 3. Instalação e Compilação

1.  **Clone o repositório:**
    ```bash
    git clone https://github.com/brunoavila55/mimic_backup.git
    cd mimic_backup
    ```

2.  **Execute o script de instalação automática:**
    ```bash
    chmod +x install.sh
    ./install.sh
    ```
    *   Este script irá baixar as dependências, compilar o binário e criar o arquivo `.env`.
    *   **Importante:** Quando o script perguntar se deseja configurar o **Systemd**, responda `y`.

---

## 4. Configuração do Banco de Dados

O Mimic usa um arquivo `.env` para gerenciar credenciais sensíveis.

1.  **Edite o arquivo `.env`:**
    ```bash
    nano .env
    ```
2.  **Configure a URL de conexão do Postgres:**
    ```bash
    DATABASE_URL=postgres://usuario:senha@localhost:5432/mimic_db?sslmode=disable
    ```
3.  **Defina a `SECRET_KEY`:**
    *   Certifique-se de usar uma chave de 32 caracteres. Ela é vital para a criptografia AES das senhas dos equipamentos.

---

## 5. Gerenciamento do Serviço (Systemd)

O Mimic roda como um serviço do sistema para garantir que ele reinicie automaticamente em caso de falhas ou após o reboot do servidor.

-   **Iniciar o serviço:**
    ```bash
    sudo systemctl start mimic
    ```
-   **Verificar o status:**
    ```bash
    sudo systemctl status mimic
    ```
-   **Ver logs em tempo real:**
    ```bash
    journalctl -u mimic -f
    ```
-   **Reiniciar após alterações no .env:**
    ```bash
    sudo systemctl restart mimic
    ```

---

## 6. Atualização do Sistema

Para atualizar o Mimic com as últimas correções e funcionalidades:

1.  **Execute o script de redeploy:**
    ```bash
    chmod +x REDEPLOY.sh
    ./REDEPLOY.sh
    ```
    *   Este script faz o `git pull`, recompila o binário e reinicia o serviço automaticamente.

---

## 7. Configuração de Proxy Reverso (Nginx) - Opcional

Para usar um domínio (ex: `mimic.suaempresa.com`) e HTTPS, utilize o Nginx:

1.  **Instale o Nginx:** `sudo apt install nginx`
2.  **Crie a configuração:**
    ```nginx
    server {
        listen 80;
        server_name mimic.suaempresa.com;

        location / {
            proxy_pass http://localhost:3000;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }
    }
    ```
3.  **Ative o SSL** usando o [Certbot/Let's Encrypt](https://certbot.eff.org/).

---

## Suporte

Se encontrar problemas durante a instalação, verifique os logs do sistema usando o comando `journalctl`.
