#!/bin/bash
set -e

# Cores para o terminal
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}>>> [Mimic] Iniciando processo de atualização nativa (Go)...${NC}"

# 1. Puxar últimas alterações
echo -e "${BLUE}[1/3] Puxando últimas alterações do Git...${NC}"
git pull origin golang

# 2. Recompilar binário
echo -e "${BLUE}[2/3] Atualizando dependências e recompilando...${NC}"
go mod tidy
go build -o mimic ./cmd/mimic/main.go

# 3. Notificar usuário ou reiniciar serviço
echo -e "${BLUE}[3/3] Finalizando...${NC}"
if systemctl is-active --quiet mimic; then
    echo -e "${GREEN}>>> [Mimic] Reiniciando serviço mimic via systemd...${NC}"
    sudo systemctl restart mimic
    echo -e "${GREEN}>>> Atualização concluída com sucesso!${NC}"
else
    echo -e "${GREEN}>>> [Mimic] Binário 'mimic' recompilado com sucesso!${NC}"
    echo -e ">>> Inicie o processo manualmente: ${BLUE}./mimic${NC}"
fi
