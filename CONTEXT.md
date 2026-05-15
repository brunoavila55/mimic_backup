# Contexto do Projeto: Mimic Backup Systems v2.0

Este documento serve como referência centralizada para desenvolvedores e agentes de IA entenderem o estado atual, a arquitetura e as decisões do projeto.

## 📌 Visão Geral

O **Mimic** é um sistema de automação para backup de equipamentos de rede (MikroTik, Cisco, Huawei, Juniper, Ubiquiti). Conecta-se via SSH, coleta a configuração, normaliza o texto e armazena versões no banco de dados com hash SHA-256 para deduplicação.

## 🏗️ Arquitetura

- **Backend**: Go 1.25 + framework **Fiber v2**.
- **Frontend**: **Go Templates** renderizados no servidor + **HTMX** para interatividade reativa + **Alpine.js** para estados de UI (modais, menus).
- **Estilização**: CSS customizado com design system neutro/escuro (Inter + JetBrains Mono). Sem frameworks CSS externos.
- **ORM**: **GORM** com driver PostgreSQL.
- **Concorrência**: Goroutines com Worker Pool para backups paralelos.
- **Criptografia**: AES-GCM 256-bit para senhas de rede e credenciais SSH (pacote `pkg/crypto`).
- **Sessão**: Fiber session middleware com cookies + bcrypt para autenticação.

## 📂 Estrutura de Pastas

```
cmd/mimic/main.go          # Ponto de entrada, rotas, middleware, scheduler (carrega .env)
internal/
  handlers/
    auth.go                    # Login, logout (AuthHandler)
    handlers.go                # Dashboard, Nodes, Settings hub (DashboardHandler, NodeHandler, SettingsHandler)
    forms.go                   # CRUD completo: nodes, users, credentials, routines, SFTP, profile, export (FormHandler)
    setup.go                   # Setup wizard: confirmação DB + criação de superuser (SetupHandler)
  middleware/
    auth.go                    # RequireSetup (primeiro acesso) + RequireAuth (sessão)
  models/
    models.go                  # User, Node, NodeBackup, BackupRoutine, AccessAgent, Credential, SftpSettings, SystemLog
  services/
    ssh/                       # Motor SSH nativo
      vendors/                 # Drivers por fabricante (mikrotik, cisco, etc.)
    scheduler/                 # Agendador interno (verifica NextBackupAt a cada 1 min)
    sftp/                      # Exportação de backups para servidor SFTP
pkg/crypto/                    # AES-GCM encrypt/decrypt + bcrypt helpers
templates/                     # Go Templates (.html)
  base.html                    # Layout principal (sidebar + header + content)
  login.html                   # Página de login (standalone)
  setup_database.html          # Setup step 1 — confirmação DB
  setup_superuser.html         # Setup step 2 — criação de admin
  dashboard.html               # Dashboard com stats
  node_list.html               # Lista de nodes com busca
  node_details.html            # Detalhes + histórico de backups do node
  node_form.html               # Criar/editar node
  node_confirm_delete.html     # Confirmação de exclusão
  settings.html                # Hub de configurações com tabs verticais
  credential_form.html         # Criar/editar credencial SSH
  user_form.html               # Criar/editar usuário
  routine_form.html            # Criar/editar rotina
  partials/                    # Fragmentos HTMX
    dashboard_stats.html       # Stats + atividade recente
    node_table.html            # Tabela de nodes
    node_table_body.html       # Linhas da tabela
    backup_view.html           # Visualização de backup (modal)
    settings_users.html        # Tab: usuários
    settings_credentials.html  # Tab: credenciais
    settings_routines.html     # Tab: rotinas
    settings_sftp.html         # Tab: configuração SFTP
    settings_export.html       # Tab: exportação
    settings_logs.html         # Tab: logs do sistema
    settings_profile.html      # Tab: perfil pessoal
static/css/style.css           # Design system completo (~780 linhas)
```

## 📊 Models (GORM)

| Model | Descrição |
|-------|-----------|
| `User` | Usuários do sistema (username, email, password bcrypt, role, avatar) |
| `Node` | Equipamento de rede (nome, IP, vendor, credenciais, agendamento, status) |
| `NodeBackup` | Versão de backup (config, hash SHA-256, status, versão incremental) |
| `BackupRoutine` | Agendamento reutilizável (frequência, horário, dia da semana) |
| `Credential` | Credencial SSH reutilizável (nome, username, senha AES-GCM, porta) |
| `AccessAgent` | Legado — agente de acesso (mantido para compatibilidade) |
| `SftpSettings` | Configuração do servidor SFTP para exportação |
| `SystemLog` | Log de atividade do sistema (nível, categoria, mensagem) |

## ⚙️ Fluxo de Funcionamento

### Primeiro Acesso (Setup Wizard)
1. App inicia → `AutoMigrate` cria as tabelas → `RequireSetup` detecta 0 usuários.
2. Redireciona para `/setup` — confirmação visual do banco de dados.
3. Redireciona para `/setup/superuser` — formulário de criação do administrador.
4. Após criação, redireciona para `/login`.

### Operação Normal
1. **Cadastro**: Usuário cria um `Node` com IP, vendor e credenciais (diretas ou via `Credential`).
2. **Criptografia**: Senhas criptografadas com `SECRET_KEY` (lida do `.env` via `godotenv`) via AES-GCM antes de salvar no Postgres.
3. **Segurança**: Consultas ao banco de dados usam queries parametrizadas (`Where("id = ?", id)`) para prevenir SQL Injection (auditado via Snyk).
4. **Agendamento**: O `Scheduler` verifica `NextBackupAt` a cada minuto.
5. **Execução**: Goroutine abre SSH, identifica o driver em `ssh/vendors`, executa comando de coleta, normaliza via RegEx, salva `NodeBackup` se o hash SHA-256 mudou.
6. **Exportação**: Usuário pode enviar backups para SFTP (individual ou sync em massa).

## 🔐 Middleware

| Middleware | Descrição |
|-----------|-----------|
| `RequireSetup` | Redireciona para `/setup` se não há usuários. Cacheia o resultado após sucesso. |
| `RequireAuth` | Verifica sessão autenticada. Para HTMX, retorna header `HX-Redirect`. |

**Ordem**: Static Files → RequireSetup → Setup/Auth Routes → RequireAuth → Protected Routes.

## 🎨 Design System

- **Tema**: Dark neutro profissional (sem glassmorphism, gradients ou glow).
- **Paleta**: `#0f1117` (bg) → `#232730` (hover), accent `#3b82f6` (azul).
- **Tipografia**: Inter (UI) + JetBrains Mono (IPs, configs).
- **Componentes**: `.card`, `.btn`, `.form-input`, `.table-wrap`, `.badge`, `.stat-card`, `.settings-layout`.
- **Layout**: Sidebar fixa (240px) + main content scrollable.

## 🔌 Hub de Configurações

A rota `/settings` é um hub unificado com **7 tabs** navegáveis via HTMX:

| Tab | Rota | Conteúdo |
|-----|------|----------|
| Usuários | `/settings/users` | CRUD de usuários + papéis (Admin/Viewer) |
| Credenciais | `/settings/credentials` | CRUD de credenciais SSH reutilizáveis |
| Rotinas | `/settings/routines` | CRUD de agendamentos de backup |
| SFTP | `/settings/sftp` | Configuração do servidor SFTP |
| Exportar | `/settings/export` | Sync em massa + status por node |
| Logs | `/settings/logs` | Últimos 200 logs do sistema |
| Meu Perfil | `/settings/profile` | Editar username, email, senha |

Cada tab usa `hx-get` para carregar parciais sem reload. Navegação direta via URL também funciona (full page render).

## 🚀 Como Continuar o Desenvolvimento

- **Novos Vendors**: Crie um novo arquivo em `internal/services/ssh/vendors/` implementando a interface `Driver` e registrando-o no `init()`.
- **Novas Páginas**: Crie template em `templates/`, handler em `internal/handlers/`, rota em `main.go`.
- **Novas Tabs de Settings**: Crie partial em `templates/partials/settings_*.html`, método em `SettingsHandler`, rota GET em `main.go`, e adicione o link no `settings.html`.
- **Template Functions**: `seq(start, end)` e `deref(*uint)` estão registradas no engine.

## ⚠️ Observações

- Variável `SECRET_KEY` (mínimo 32 caracteres) no arquivo `.env` é obrigatória para criptografia de credenciais.
- `AutoMigrate` roda no startup — adicionar campos em models é seguro (nunca remove colunas).
- O `AccessAgent` é legado; novos desenvolvimentos devem usar `Credential`.
- Instalação funciona com binário Go único.

---
*Documento atualizado em 11 de maio de 2026.*
