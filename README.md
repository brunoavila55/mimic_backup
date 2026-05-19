# Mimic — Backup Systems v0.1.X

Plataforma de alta performance para automação, versionamento e centralização de backups de equipamentos de rede (switches, roteadores, OLTs, firewalls). Conecta-se aos dispositivos via SSH, captura a configuração e mantém um histórico completo para auditoria e recuperação.

Desenvolvido em **Golang** com interface web moderna e reativa.

---

## Principais Funcionalidades

- **Setup Wizard**: Fluxo guiado de primeiro acesso — confirmação do banco de dados e criação do administrador.
- **Dashboard**: Métricas essenciais (nodes, backups, falhas) com atividade recente em tempo real.
- **Gestão de Nodes**: Cadastro, busca, backup manual e automático de equipamentos de rede.
- **Credenciais SSH**: Credenciais reutilizáveis com criptografia AES-GCM 256-bit.
- **Rotinas de Backup**: Agendamentos automáticos com frequência configurável.
- **Exportação SFTP**: Sincronização dos backups com servidor SFTP remoto.
- **Gestão de Usuários**: Controle de acesso com papéis (Administrador / Visualizador).
- **Logs do Sistema**: Registro de toda atividade (backups, exportações, erros).
- **Hub de Configurações**: Interface unificada com tabs para todas as configurações do sistema.

---

## Arquitetura

| Camada | Tecnologia |
|--------|------------|
| Backend | Go 1.25 + Fiber v2 |
| Frontend | Go Templates + HTMX + Alpine.js |
| Estilização | CSS customizado (design system neutro/escuro) |
| ORM | GORM (PostgreSQL) |
| Conexão de Rede | SSH nativo (`golang.org/x/crypto/ssh`) |
| Criptografia | AES-GCM 256-bit (`pkg/crypto`) |
| Concorrência | Goroutines + Worker Pool |

---

## Pré-requisitos

- **Go 1.22+** (apenas para compilação)
- **PostgreSQL 15+**

---

## Instalação

```bash
git clone https://github.com/brunoavila55/mimic_backup.git
cd mimic_backup
```

Configure as variáveis de ambiente criando um arquivo `.env` na raiz:

```bash
DATABASE_URL=postgres://postgres:123456@localhost:5432/mimic_db?sslmode=disable
SECRET_KEY=uma-chave-aleatoria-de-32-caracteres
```

Compile e execute:

```bash
go mod tidy
go build -o mimic ./cmd/mimic/main.go
./mimic
```

---

## Primeiro Acesso

Na primeira execução (sem usuários no banco), o sistema redireciona automaticamente para o **Setup Wizard**:

1. **Banco de Dados** — Confirma que a conexão e as tabelas estão prontas.
2. **Administrador** — Cria o primeiro usuário com papel de Administrador.
3. **Login** — Redireciona para a tela de autenticação.

---

## Estrutura de Rotas

| Rota | Descrição |
|------|-----------|
| `/setup` | Setup wizard (primeiro acesso) |
| `/login` | Autenticação |
| `/` | Dashboard |
| `/nodes` | Gerenciamento de nodes |
| `/nodes/:id` | Detalhes e backups do node |
| `/settings` | Hub de configurações |
| `/settings/users` | Gestão de usuários |
| `/settings/credentials` | Credenciais SSH |
| `/settings/routines` | Rotinas de backup |
| `/settings/sftp` | Configuração SFTP |
| `/settings/export` | Exportação em massa |
| `/settings/logs` | Logs do sistema |
| `/settings/profile` | Perfil do usuário |

---

## Extensão

### Novos Fabricantes (Vendors)
Crie um novo arquivo Go em `internal/services/ssh/vendors/` (ex: `mikrotik.go`) implementando a interface `Driver` (métodos `GetBackupCommand` e `NormalizeConfig`) e registre-o usando a função `Register` no bloco `init()`.

### Novas Páginas
Crie o template em `templates/`, o handler em `internal/handlers/` e registre a rota em `cmd/mimic/main.go`.

---

## Licença

Desenvolvido por **Mimic Backup Systems**.
Para bugs e sugestões, utilize o sistema de Issues do GitHub.
