#!/bin/bash
set -e

# Cores para o terminal
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "    __  ____              _      "
echo "   /  |/  (_)___ ___  (_)____"
echo "  / /|_/ / / __ \`__ \/ / ___/"
echo " / /  / / / / / / / / / /__  "
echo "/_/  /_/_/_/ /_/ /_/_/\___/  Installer v0.1.X"
echo "________________________________________________"
echo -e "${NC}"

# 1. Verificação de dependências
echo -e "${BLUE}[1/4] Verificando dependências...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Erro: Go não está instalado. Instale o Go 1.22+ antes de continuar.${NC}"
    exit 1
fi

if ! command -v git &> /dev/null; then
    echo -e "${RED}Erro: Git não está instalado.${NC}"
    exit 1
fi

# 2. Compilação
echo -e "${BLUE}[2/4] Compilando binário do sistema...${NC}"
go mod tidy
go build -o mimic ./cmd/mimic/main.go
echo -e "${GREEN}Sucesso: Binário 'mimic' gerado.${NC}"

# 3. Configuração de ambiente
echo -e "${BLUE}[3/4] Configurando variáveis de ambiente...${NC}"
if [ ! -f .env ]; then
    echo "DATABASE_URL=host=localhost user=postgres password=postgres dbname=mimic_db port=5432 sslmode=disable" > .env
    echo "SECRET_KEY=$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 32)" >> .env
    echo "PORT=3000" >> .env
    echo -e "${GREEN}Arquivo .env criado com valores padrão.${NC}"
else
    echo -e "${GREEN}Arquivo .env já existe. Pulando.${NC}"
fi

# 4. Configuração do Systemd (Opcional)
echo -e "${BLUE}[4/4] Deseja configurar como um serviço do sistema (systemd)? [y/N]${NC}"
read -r response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    APP_PATH=$(pwd)
    USER_NAME=$(whoami)
    
    SERVICE_TEMPLATE="[Unit]
Description=Mimic Backup Systems Service
After=network.target postgresql.service

[Service]
User=$USER_NAME
Group=$USER_NAME
WorkingDirectory=$APP_PATH
EnvironmentFile=$APP_PATH/.env
ExecStart=$APP_PATH/mimic
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target"

    echo "$SERVICE_TEMPLATE" | sudo tee /etc/systemd/system/mimic.service > /dev/null
    sudo systemctl daemon-reload
    echo -e "${GREEN}Serviço 'mimic.service' criado com sucesso!${NC}"
    echo -e "Use: ${BLUE}sudo systemctl start mimic${NC} para iniciar."
fi

# 5. Finalização
echo -e "\n${GREEN}Instalação concluída com sucesso!${NC}"
echo -e "Para rodar manualmente: ${BLUE}./mimic${NC}"
echo "Acesse o painel em: http://localhost:3000"
