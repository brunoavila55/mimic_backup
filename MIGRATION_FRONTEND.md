# Migração do frontend — inventário

Este documento registra o estado anterior à migração React. O frontend legado
permanece disponível; a API e a SPA são adicionadas em paralelo.

## Estado implementado em 19/09/2026

### Concluído

- Auditoria das rotas, templates, permissões, dependências HTMX/Alpine e dados
  sensíveis.
- Fundação de `/api/v1` com envelopes de erro JSON, autenticação por sessão,
  RBAC no backend e respostas JSON para 401, 403, 404 e rejeições de origem.
- `POST /api/v1/auth/login`, `POST /api/v1/auth/logout` e
  `GET /api/v1/auth/me`, preservando rate limit, `Session.Regenerate`, dummy
  bcrypt hash, cookies existentes e audit log.
- Scaffold React + Vite em JavaScript com React Router, TanStack Query, wrapper
  de `fetch`, `AuthProvider`, `ProtectedRoute`, `AppShell`, modal, toast,
  estados de carregamento/erro e error boundary.
- Dashboard React com a mesma consulta de `loadDashboard` e atualização a cada
  45 segundos.
- Nodes read-only em React: filtros, listagem, detalhes, histórico, conteúdo de
  backup e diff calculado por `pkg/diff` no backend.
- Settings read-only em React para profile, credentials, routines, users, logs,
  alerts, SFTP e export. As tabs respeitam as permissões retornadas pela API e
  todos os endpoints aplicam RBAC novamente no backend.
- DTOs explícitos. Senhas, hashes, chaves SSH, tokens Telegram, URLs de webhook
  e configurações completas de backup não aparecem em listagens.
- Build Docker multi-stage, artefatos Vite servidos pelo Fiber, execução como
  usuário não-root e healthcheck.
- Testes de middleware JSON, ausência de segredos nos DTOs e testes mínimos do
  frontend. Lint, build, testes Go, `go vet` e audit npm foram validados.

### Cutover e compatibilidade

- A imagem Docker define `SPA_ENABLED=true` e entrega React em `/login`, `/`,
  `/nodes`, `/nodes/:id` e nas telas read-only de Settings.
- Em desenvolvimento local, `SPA_ENABLED=false` mantém o frontend Go Templates;
  depois de gerar `frontend/dist`, a flag pode ser ativada para usar a SPA.
- Os templates, HTMX e Alpine não foram removidos. Formulários e mutações
  continuam acessíveis pelo frontend legado e retornam para as telas React.
- Nas telas de Settings, `?legacy=1` abre explicitamente a visualização antiga
  quando uma configuração ainda precisa ser alterada.

### Próximas fases

- Extrair a persistência e validação de Nodes para uma camada compartilhada e
  implementar CRUD JSON + `NodeForm` React.
- Migrar import/export de Nodes, mantendo todas as proteções de CSV.
- Migrar as mutações de Settings por domínio, na ordem definida no roteiro.
- Remover cada template somente depois dos testes de paridade do respectivo
  domínio.

## Rotas atuais

Todas as rotas abaixo passam por `SecurityHeadersAndOrigin`. Exceto setup,
login e health, também passam por `RequireAuth`.

| Método e rota | Handler | Resposta/template | Permissão | Tipo / dependência |
|---|---|---|---|---|
| GET `/health` | inline | JSON | pública | leitura |
| GET/POST `/setup` | `GetDatabaseSetup` / `PostDatabaseSetup` | `setup_database` / redirect | pública | leitura/mutação |
| GET/POST `/setup/superuser` | `GetCreateSuperuser` / `PostCreateSuperuser` | `setup_superuser` / redirect | pública + limiter no POST | leitura/mutação |
| GET/POST `/login` | `GetLogin` / `PostLogin` | `login` / redirect | pública + limiter no POST | leitura/mutação |
| POST `/logout` | `Logout` | redirect | autenticado | mutação |
| GET `/` | `GetDashboard` | `dashboard` + `partials/dashboard_stats` | autenticado | leitura; HTMX a cada 45 s |
| GET `/nodes` | `ListNodes` | `node_list` + `partials/node_table` | autenticado | leitura; filtros HTMX |
| GET `/nodes/new` | `NewNode` | `node_form` | `manage_nodes` | leitura; Alpine |
| GET `/nodes/:id` | `NodeDetails` | `node_details` | autenticado | leitura; HTMX em backups |
| GET `/nodes/:id/edit` | `EditNode` | `node_form` | `manage_nodes` | leitura; Alpine |
| GET `/nodes/:id/delete` | `DeleteNodeConfirm` | `node_confirm_delete` | `manage_nodes` | leitura |
| POST `/nodes/save[/:id]` | `SaveNode` | redirect ou re-render | `manage_nodes` | mutação |
| POST `/nodes/:id/snooze` | `SnoozeNode` | status + `HX-Trigger`/`HX-Redirect` | `manage_nodes` | mutação HTMX |
| POST/DELETE `/nodes/:id[/delete]` | `DeleteNode` | redirect ou vazio | `manage_nodes` | mutação HTMX |
| GET/POST `/nodes/import` | `ImportNodesForm` / `ImportNodesCSV` | `node_import` | `manage_nodes` | leitura/mutação multipart |
| GET `/nodes/export` | `ExportNodesCSV` | CSV | `manage_nodes` | leitura/download |
| GET `/backups/:id/content` | `GetBackupContent` | `partials/backup_view` | autenticado | leitura HTMX |
| GET `/backups/:id/diff` | `GetBackupDiff` | `partials/diff_view` | autenticado | leitura HTMX |
| GET `/backups/diff/compare` | `CompareBackups` | `partials/diff_body` | administrador | leitura HTMX |
| GET `/settings` | `GetSettings` | redirect por papel | autenticado | leitura |
| GET `/settings/users` | `GetUsersTab` | `settings`/`partials/settings_users` | administrador | leitura HTMX/Alpine |
| GET `/settings/credentials` | `GetCredentialsTab` | lista/partial | `manage_operations` | leitura HTMX/Alpine |
| GET `/settings/routines` | `GetRoutinesTab` | lista/partial | `manage_operations` | leitura HTMX/Alpine |
| GET `/settings/sftp` | `GetSFTPTab` | `settings`/partial | `manage_system` | leitura HTMX/Alpine |
| GET `/settings/sftp/explore` | `GetSFTPExplore` | `partials/sftp_explorer` ou `SendString` | `manage_system` | leitura HTMX |
| GET `/settings/export` | `GetExportTab` | `settings`/partial | `export_backups` | leitura HTMX |
| GET `/settings/alerts` | `GetAlertsTab` | `settings`/partial | `manage_system` | leitura HTMX |
| GET `/settings/logs` | `GetLogsTab` | `settings`/partial | `view_audit` | leitura HTMX/Alpine |
| GET `/settings/profile` | `GetProfileTab` | `settings`/partial | autenticado | leitura HTMX |
| GET `/settings/{users,credentials,routines,alerts}/new` | respectivos `New*` | formulário do domínio | permissão do domínio | leitura Alpine |
| GET `/settings/{users,credentials,routines,alerts}/:id/edit` | respectivos `Edit*` | formulário do domínio | permissão do domínio | leitura Alpine |
| POST `/settings/{users,credentials,routines,alerts}/save[/:id]` | respectivos `Save*` | redirect ou re-render | permissão do domínio | mutação |
| DELETE `/settings/{users,credentials,routines,alerts}/:id` | respectivos `Delete*` | vazio + `HX-Trigger` | permissão do domínio | mutação HTMX |
| POST `/settings/alerts/test` | `TestAlertRule` | status + `HX-Trigger` | `manage_system` | mutação HTMX |
| POST `/settings/sftp/save` | `SaveSettings` | re-render | `manage_system` | mutação HTMX |
| POST `/settings/sftp/test` | `TestSFTPConnection` | status + `HX-Trigger` | `manage_system` | mutação HTMX |
| POST `/settings/profile/save` | `SaveProfile` | redirect/re-render | autenticado | mutação multipart |
| POST `/settings/export/sync` | `PostSync` | status + `HX-Trigger` | `export_backups` | mutação HTMX |
| POST `/backups/:backup_id/export` | `ExportBackup` | status + `HX-Trigger` | `export_backups` | mutação HTMX |
| POST `/trigger-backups` | inline | status + `HX-Trigger` | `run_backups` | mutação HTMX |
| POST `/nodes/:id/trigger` | inline | status + `HX-Trigger` | `run_backups` | mutação HTMX |

`Render`, redirects e `SendString` aparecem em todos os fluxos legados de
autenticação, formulário e erro. A API não pode reutilizar essas respostas;
ela precisa compartilhar apenas consulta, validação e persistência.

## Dependências do frontend legado

- `base.html` carrega HTMX e Alpine por CDN e fornece sidebar, modal, loader e
  notificações globais.
- Dashboard usa polling HTMX de 45 segundos, refresh manual, modal de diff e
  eventos globais de notificação.
- Nodes usa filtros com `hx-include`/`hx-push-url`, ações de backup, snooze e
  exclusão; formulários alternam credencial/agendamento com Alpine.
- Settings troca tabs por HTMX. Users, credentials, routines e logs filtram
  localmente com Alpine. SFTP usa Alpine para senha e HTMX para teste/explorer.
- O diff alterna entre visões split/unified no Alpine, mas o cálculo é feito
  corretamente no backend por `pkg/diff`.
- Login e setup contêm JavaScript inline; login também carrega `tsparticles`.

## Dados sensíveis

Nunca serializar models GORM diretamente. Campos proibidos ou restritos:

- `User.Password` (hash bcrypt).
- `Credential.Password` e `AccessAgent.Password` (segredos criptografados).
- `Node.Password`, `Node.SSHPrivateKey`, `Node.SSHPublicFingerprint` e relações
  completas de Credential/AccessAgent.
- `SftpSettings.Password` e, por precaução, fingerprint completo quando não
  for necessário à tela.
- `AlertRule.TelegramToken`, `WebhookURL` e `TelegramChatID`; retornar flags de
  configuração e valores mascarados quando suficiente.
- `NodeBackup.Config` somente no endpoint explícito de visualização; nunca em
  listas ou dashboard. `SystemLog.Details` pode conter contexto operacional e
  deve ser limitado a perfis com `view_audit`.

## Mapa `/api/v1`

| Domínio | Endpoints |
|---|---|
| Base/auth | `GET /health`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` |
| Dashboard | `GET /dashboard`, `POST /backups/run` |
| Nodes | `GET/POST /nodes`, `GET/PUT/DELETE /nodes/:id`, `POST /nodes/:id/snooze`, `POST /nodes/:id/backup`, `POST /nodes/import`, `GET /nodes/export` |
| Backups | `GET /backups/:id`, `GET /backups/:id/diff`, `GET /backups/diff/compare`, `POST /backups/:id/export` |
| Profile | `GET/PUT /profile`, `POST /profile/avatar` |
| Credentials | `GET/POST /credentials`, `GET/PUT/DELETE /credentials/:id` |
| Routines | `GET/POST /routines`, `GET/PUT/DELETE /routines/:id` |
| Users | `GET/POST /users`, `GET/PUT/DELETE /users/:id` |
| SFTP | `GET/PUT /sftp`, `POST /sftp/test`, `GET /sftp/explore` |
| Alerts | `GET/POST /alerts`, `GET/PUT/DELETE /alerts/:id`, `POST /alerts/test` |
| Auditoria/export | `GET /logs`, `GET /export`, `POST /export/sync` |

## Riscos e ordem de tratamento

1. Vazamento de segredo por serialização de model: usar DTO em toda resposta.
2. API retornar redirect/HTML: middleware e erros JSON exclusivos sob `/api/v1`.
3. Divergência entre HTML e JSON: extrair carregamento/validação antes das
   mutações, mantendo uma única regra de negócio.
4. Regressão de RBAC: autorização obrigatória no backend e testes 401/403.
5. CSRF/origem: manter cookies SameSite/HttpOnly/Secure e
   `SecurityHeadersAndOrigin`; não adicionar CORS/JWT.
6. CSV, upload e integrações: preservar BOM, normalização, `csvSafe`, limites
   de upload, auditoria e mensagens por linha.
7. Cutover: a SPA é ativada por configuração; templates continuam no artefato
   até a paridade de cada domínio ser validada.
