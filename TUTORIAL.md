# Instalação do Mimic Backup

Prepare o servidor Linux (recomendado Ubuntu 24.04 / Debian 12). Conecte-se na linha de comando e siga as instruções abaixo.

> **Atenção:** Estas instruções assumem que você é o usuário `root`. Se não for, coloque `sudo` antes dos comandos ou torne-se root temporariamente com `sudo -s` ou `sudo -i`.

## 1. Instalar Pacotes Necessários

Atualize os repositórios e instale as dependências básicas:
```bash
apt update
apt install curl git postgresql postgresql-contrib nginx-full
```

## 2. Instalar Go (Golang)
O Mimic é escrito em Go e requer a versão 1.22+.

```bash
wget https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

## 3. Adicionar o Usuário do Sistema
Crie um usuário dedicado para rodar a aplicação por motivos de segurança:
```bash
useradd mimic -d /opt/mimic -M -r -s "$(which bash)"
```

## 4. Download do Mimic
Vá para a pasta `/opt` e clone o repositório oficial:
```bash
cd /opt
git clone https://github.com/brunoavila55/mimic_backup.git mimic
```

## 5. Definir Permissões
Atribua a propriedade da pasta ao usuário que acabamos de criar:
```bash
chown -R mimic:mimic /opt/mimic
chmod -R 775 /opt/mimic
```

## 6. Compilar o Sistema
Mude para o usuário `mimic` para fazer a compilação de forma segura:
```bash
su - mimic
cd /opt/mimic
go mod download
go build -o mimic_bin ./cmd/mimic/main.go
exit
```
*(O comando `exit` retornará a sua sessão para o usuário root)*

## 7. Configurar o PostgreSQL
Acesse o prompt do banco de dados:
```bash
sudo -u postgres psql
```
Dentro do console do banco (prompt `postgres=#`), execute os comandos abaixo. 

> **Atenção:** Troque `password` por uma senha segura.
```sql
CREATE DATABASE mimic_db;
CREATE USER mimic WITH ENCRYPTED PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE mimic_db TO mimic;
\q
```

## 8. Configurar Variáveis de Ambiente
O sistema precisa de um arquivo `.env` para rodar.
```bash
su - mimic
cd /opt/mimic
nano .env
```
Cole o conteúdo abaixo, prestando atenção para colocar a mesma senha do banco que você criou no passo anterior, e gerando uma chave aleatória para o `SECRET_KEY`:
```env
DATABASE_URL=postgres://mimic:password@localhost:5432/mimic_db?sslmode=disable
SECRET_KEY=cole_aqui_uma_chave_aleatoria_de_32_caracteres
PORT=3000
```
Salve e feche o arquivo (`Ctrl+O`, `Enter`, `Ctrl+X`), e retorne ao root:
```bash
exit
```

## 9. Configurar o Serviço (Systemd)
Para garantir que o Mimic inicie junto com o servidor e reinicie em caso de falhas, vamos criar um serviço no sistema operacional:
```bash
cat <<EOF > /etc/systemd/system/mimic.service
[Unit]
Description=Mimic Backup Systems
After=network.target postgresql.service

[Service]
User=mimic
Group=mimic
WorkingDirectory=/opt/mimic
EnvironmentFile=/opt/mimic/.env
ExecStart=/opt/mimic/mimic_bin
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```
Ative e inicie o serviço:
```bash
systemctl daemon-reload
systemctl enable mimic
systemctl start mimic
```

## 10. Configurar Web Server (NGINX)
Vamos usar o NGINX como proxy reverso para receber as conexões web na porta 80 e enviar para a nossa aplicação.

```bash
nano /etc/nginx/sites-available/mimic.conf
```
Adicione o conteúdo abaixo. Se você tiver um domínio, troque `mimic.example.com`. Caso contrário, pode colocar o IP do servidor:
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
Ative a configuração e reinicie o NGINX:
```bash
ln -s /etc/nginx/sites-available/mimic.conf /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
systemctl restart nginx
```

## Passos Finais
Tudo pronto! Acesse no seu navegador `http://mimic.example.com` (ou o IP do servidor).
Na primeira execução, o sistema irá direcioná-lo para a tela de **First Setup**, onde você poderá criar o seu usuário Administrador.

> **Nota de Segurança:** Este tutorial não abordou a instalação de certificados SSL (HTTPS). Para expor seu servidor na internet, recomendamos instalar o **Certbot/Let's Encrypt** para garantir a segurança da plataforma.
