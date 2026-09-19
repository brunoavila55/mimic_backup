# Migração do frontend para JavaScript

Repositório: <https://github.com/brunoavila55/mimic_backup>

## Objetivo

Migrar gradualmente o frontend atual, baseado em Go Templates, HTMX e Alpine.js, para React + Vite em JavaScript, mantendo o backend Go/Fiber e preservando o comportamento, a segurança e os fluxos operacionais existentes.

Este roteiro foi desenhado para evitar uma reescrita *big bang*. A estratégia é criar uma API JSON paralela, levantar o novo frontend e migrar cada domínio separadamente, mantendo o frontend legado funcional até que sua substituição esteja validada.

## Arquitetura alvo

```text
PostgreSQL
    │
    ▼
Go / Fiber
├── /api/v1/*
│   ├── auth
│   ├── dashboard
│   ├── nodes
│   ├── backups
│   ├── credentials
│   ├── routines
│   ├── users
│   ├── sftp
│   ├── alerts
│   ├── logs
│   └── profile
│
└── frontend/dist/*
          ▲
          │
React + Vite + JavaScript
├── React Router
├── TanStack Query
├── wrapper de fetch
└── CSS atual reaproveitado inicialmente
```

### Decisões arquiteturais

- Manter o backend Go/Fiber.
- Usar React + Vite em JavaScript, sem TypeScript nesta primeira migração.
- Manter frontend e backend na mesma origem.
- Continuar usando a sessão Fiber com cookies `HttpOnly`, `SameSite` e `Secure` conforme a configuração atual.
- Não introduzir JWT ou CORS sem necessidade concreta.
- Reaproveitar o CSS e a linguagem visual existentes; não introduzir Tailwind durante a migração.
- Preservar `pkg/diff`, scheduler, SSH, crypto, GORM e serviços de backup em Go.
- Criar DTOs explícitos para impedir a serialização acidental de senhas, tokens, chaves ou campos criptografados.
- Extrair regras de negócio dos handlers durante a migração, para que HTML e JSON usem temporariamente a mesma lógica.

## Regras de execução

1. Execute os prompts na ordem apresentada.
2. Não avance enquanto testes, lint e build relevantes não estiverem verdes.
3. Faça commits e PRs pequenos, isolados e reversíveis.
4. Não remova o frontend legado antes de existir equivalência funcional validada.
5. Não duplique regras de negócio entre handlers HTML e JSON.
6. Aplique RBAC no backend; ocultar elementos na interface não constitui segurança.
7. Não serialize models sensíveis diretamente, mesmo que pareçam conter apenas os campos esperados.
8. Preserve sessões, validações, auditoria e proteções de origem em todas as etapas.

---

## Prompt 0 — Auditoria antes de tocar no código

```text
Analise este repositório antes de modificar qualquer arquivo.

Objetivo futuro:
migrar o frontend atual baseado em Go Templates + HTMX + Alpine.js para React + Vite em JavaScript, mantendo o backend Go/Fiber.

Nesta etapa NÃO implemente a migração.

Faça:

1. Inventarie todas as rotas definidas em cmd/mimic/main.go.
2. Para cada rota, identifique:
   - handler
   - template/partial utilizado
   - método HTTP
   - permissão necessária
   - se é leitura ou mutação
   - se depende de HTMX
3. Identifique todas as respostas que hoje são HTML, redirects, HX-Trigger ou SendString.
4. Identifique dados sensíveis existentes nos models que NÃO podem ser serializados diretamente para JSON.
5. Mapeie dependências entre templates, Alpine.js, HTMX e JavaScript inline.
6. Proponha um mapa equivalente de endpoints /api/v1.
7. Não faça mudanças funcionais.
8. Gere MIGRATION_FRONTEND.md contendo o inventário e o plano.

Preserve:
- sessões atuais
- RBAC
- validações
- audit logs
- proteção Origin/Referer
- scheduler
- SSH
- SFTP
- alertas
- import/export CSV

Ao final, mostre os principais riscos encontrados.
```

### Critério de conclusão

- Inventário de rotas, templates, permissões e dependências completo.
- Mapa proposto de `/api/v1` revisado.
- Campos sensíveis explicitamente identificados.
- Nenhuma alteração funcional realizada.

---

## Prompt 1 — Fundação da API

```text
Implemente somente a fundação da nova API JSON.

Contexto:
O backend continua sendo Go + Fiber.
O frontend legado Go Templates + HTMX deve continuar funcionando.
A nova API deve ficar sob /api/v1.

Objetivos:

1. Criar uma estrutura de handlers para API separada dos handlers HTML atuais.
2. Criar resposta JSON padronizada para erros.
3. Criar middleware de autenticação da API:
   - usuário não autenticado => HTTP 401 JSON
   - usuário sem permissão => HTTP 403 JSON
   - nunca retornar redirect ou HTML em /api/v1
4. Reutilizar a sessão Fiber existente.
5. Reutilizar internal/access para RBAC.
6. NÃO usar JWT.
7. NÃO habilitar CORS desnecessariamente.
8. Preservar SecurityHeadersAndOrigin.
9. Criar DTOs explícitos. Nunca serializar models sensíveis diretamente.

Criar inicialmente:

GET /api/v1/health
GET /api/v1/auth/me

GET /api/v1/auth/me deve retornar apenas dados públicos como:
- id
- username
- email, se apropriado
- role
- avatar
- permissions

Nunca retornar:
- password
- credenciais SSH
- tokens
- segredos
- campos criptografados

Adicione testes para:
- 401
- 403
- sessão válida
- ausência de dados sensíveis

Não altere nenhuma rota HTML existente.
Execute os testes Go antes de finalizar.
```

### Critério de conclusão

- `/api/v1/health` e `/api/v1/auth/me` funcionam.
- Erros da API nunca retornam HTML ou redirects.
- Testes de autorização e ausência de campos sensíveis passam.
- Todas as rotas HTML existentes continuam funcionando.

---

## Prompt 2 — Login e logout em JSON

```text
Migre a autenticação para também suportar a nova API, mantendo o fluxo HTML atual intacto.

Criar:

POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/auth/me

Requisitos:

- continuar usando Fiber session
- manter Session.Regenerate no login
- manter proteção contra username timing attack usando dummyPasswordHash
- manter rate limiting equivalente ao /login atual
- manter audit log
- retornar JSON, nunca redirects
- credentials inválidas => 401
- login válido => 200
- logout => destruir sessão
- não devolver hash ou qualquer informação de senha
- cookies devem manter HttpOnly, SameSite e Secure conforme configuração atual

Não remover /login ou /logout legados.

Crie testes de integração para o fluxo:
login -> me -> logout -> me retorna 401.
```

### Critério de conclusão

- Fluxo completo de autenticação via API coberto por teste de integração.
- Proteções contra timing attack, fixation, brute force e vazamento de dados preservadas.
- Login e logout legados continuam operacionais.

---

## Prompt 3 — Scaffold React/Vite

```text
Crie o novo frontend em /frontend.

Tecnologias:
- Vite
- React
- JavaScript, não TypeScript
- React Router
- TanStack Query
- fetch nativo através de um wrapper centralizado

Não use Redux.

Estrutura desejada:

frontend/src/
  api/
  components/
  layouts/
  pages/
  hooks/
  lib/
  styles/
  routes/

Crie:

- App
- AuthProvider ou mecanismo equivalente
- ProtectedRoute
- AppShell
- Sidebar
- Header
- Notification/Toast system
- Modal
- ErrorBoundary
- Loading states

Rotas inicialmente:
- /login
- /
- /nodes
- /nodes/:id
- /settings/*

O frontend deve usar:

credentials: 'include'

para chamadas à API quando necessário.

Reaproveite a linguagem visual definida em DESIGN.md.
Não redesenhe o produto.
Não introduza Tailwind.
Não altere o design system nesta etapa.

Configure proxy de desenvolvimento do Vite para o backend Go.

Não remova templates, HTMX ou Alpine ainda.

Garanta:
- npm build funcionando
- npm lint funcionando, se lint for adicionado
```

### Critério de conclusão

- Aplicação React inicia em desenvolvimento e gera build de produção.
- Rotas protegidas tratam corretamente os estados autenticado e não autenticado.
- Wrapper de API, estados de erro e carregamento estão centralizados.
- Nenhuma tela legada foi removida.

---

## Prompt 4 — Dashboard API + React

```text
Migre somente o Dashboard.

Backend:

Transforme a estrutura já existente em dashboard.go em um endpoint:

GET /api/v1/dashboard

Reaproveite loadDashboard e a lógica atual.
Não duplique queries.

Crie DTOs JSON explícitos para:
- stats
- attention
- trend
- upcoming
- recent_changes
- recent_activity
- sftp
- last_successful_at
- updated_at

Não exponha models completos caso contenham dados desnecessários.

Frontend:

Crie DashboardPage em React reproduzindo o dashboard atual.

Use TanStack Query.

Substitua o comportamento HTMX:
hx-trigger="every 45s"

por refetchInterval equivalente de 45 segundos.

Preserve:
- métricas
- estados
- labels
- tabela/listas
- permissões
- design
- responsive behavior

Não apague dashboard.html ainda.

Adicione testes Go para o endpoint e testes mínimos do frontend.
```

### Critério de conclusão

- Dashboard React possui paridade funcional e visual com a versão legada.
- Atualização automática acontece a cada 45 segundos.
- Endpoint usa a lógica existente sem duplicar queries.
- Testes do endpoint e da tela passam.

---

## Prompt 5 — Nodes somente leitura

```text
Migre a parte read-only de Nodes.

Criar API:

GET /api/v1/nodes
GET /api/v1/nodes/:id
GET /api/v1/backups/:id
GET /api/v1/backups/:id/diff
GET /api/v1/backups/diff/compare

GET /api/v1/nodes deve suportar os filtros atuais:
- search
- status
- vendor
- group

Preserve exatamente as regras atuais de filtragem.

Crie DTOs que não exponham:
- Password
- SSHPrivateKey
- credenciais criptografadas
- qualquer segredo

Para Node Details, retornar backups de maneira paginável ou preparada para paginação.

Para diff, continuar usando pkg/diff no backend.
Não reimplementar o algoritmo de diff em JavaScript.

Frontend:

Criar:
- NodesPage
- NodeDetailsPage
- BackupViewer
- DiffViewer
- NodeFilters

Preservar comportamento e visual atuais.

Não implementar create/edit/delete nesta etapa.

Remover dependência de HTMX apenas destas telas depois que a versão React estiver validada.
```

### Critério de conclusão

- Listagem, detalhes, backups e diff possuem paridade funcional.
- Filtros retornam os mesmos resultados do frontend legado.
- Nenhum segredo aparece nos payloads.
- Nenhuma mutação de Nodes foi introduzida nesta etapa.

---

## Prompt 6 — CRUD de Nodes

```text
Agora migre as mutações de Nodes para API JSON.

Endpoints:

POST   /api/v1/nodes
PUT    /api/v1/nodes/:id
DELETE /api/v1/nodes/:id
POST   /api/v1/nodes/:id/snooze
POST   /api/v1/nodes/:id/backup

Preserve todas as validações existentes em forms.go:
- vendors suportados
- porta 1-65535
- schedule_type
- frequency
- backup_hour
- backup_day
- normalização de group
- normalização de tags
- credential_id
- routine_id
- access_agent_id
- encryption de password
- audit logs

Refatore validação e persistência para funções/services reutilizáveis.
Evite manter uma implementação para HTML e outra para JSON.

Formato de erro esperado:

{
  "error": {
    "code": "validation_error",
    "message": "...",
    "fields": {
      "campo": "mensagem"
    }
  }
}

Frontend:
crie NodeForm reutilizado por create/edit.

Use mutations do TanStack Query e invalide caches relevantes após sucesso.

Não remova os handlers HTML antes dos testes do novo fluxo passarem.
```

### Critério de conclusão

- Todas as mutações respeitam as validações e permissões atuais.
- HTML e JSON chamam a mesma camada de regras de negócio.
- Erros de campo são estruturados e exibidos corretamente no formulário.
- Cache do frontend é invalidado após mutações bem-sucedidas.

---

## Prompt 7 — Importação e exportação

```text
Migre importação e exportação de Nodes.

Preserve rigorosamente as proteções existentes:
- delimiter detection
- BOM
- normalizeCSVHeader
- parseCSVBool
- normalizeNodeVendor
- normalizeNodeGroup
- normalizeNodeTags
- csvSafe contra spreadsheet/formula injection
- password nunca deve aparecer na exportação

Endpoints:

POST /api/v1/nodes/import
GET  /api/v1/nodes/export

Import deve continuar aceitando multipart/form-data.

Retorne erros de importação estruturados em JSON contendo número da linha e motivo quando possível.

Export deve continuar retornando CSV para download, não JSON.

Frontend:
criar interface de upload, resultado e download equivalente à atual.
```

### Critério de conclusão

- Importação mantém normalizações, validações e diagnóstico por linha.
- Exportação mantém proteção contra formula injection e nunca inclui senha.
- Upload e download funcionam no novo frontend.

---

## Prompt 8 — Settings

```text
Migre Settings para a nova API e frontend React por domínio, sem fazer tudo em um único commit.

Ordem:

1. profile
2. credentials
3. routines
4. users
5. logs
6. alerts
7. sftp
8. export

Para cada domínio:

- primeiro criar GET read-only
- criar DTO explícito
- adicionar testes de autorização
- implementar tela React
- depois implementar mutations
- somente depois retirar o template correspondente

RBAC deve continuar sendo aplicado no BACKEND.
Ocultar botão no frontend não é segurança.

Especial atenção:

Credentials:
nunca retornar Password.

SFTP:
nunca retornar Password.
Pode retornar has_password boolean.

Alerts:
não retornar TelegramToken completo.
Não retornar segredo de webhook caso exista informação sensível.
Use flags como has_token / configured quando possível.

Users:
jamais retornar Password hash.

Profile:
trate upload de avatar sem mudar as validações atuais.

Logs:
preservar RequirePermission(ViewAudit).

Não modifique scheduler, SSH engine ou serviços de backup.
```

### Critério de conclusão

- Cada domínio foi migrado e validado separadamente.
- Testes de RBAC incluem cenários permitidos e negados.
- Senhas, hashes, tokens e segredos não são expostos.
- Templates só são retirados depois da validação de cada domínio.

---

## Prompt 9 — Build e Docker

```text
Integre o frontend React ao build de produção.

Atualize Dockerfile para multi-stage:

1. stage Node:
   - instalar dependências de frontend
   - npm run build

2. stage Go:
   - compilar mimic_bin

3. stage final:
   - copiar mimic_bin
   - copiar frontend/dist
   - copiar somente assets ainda necessários

Configure Fiber para:

- servir assets compilados
- entregar index.html para rotas do frontend React
- NÃO interceptar /api/*
- NÃO interceptar /health
- permitir client-side routing

Frontend e API devem permanecer na mesma origem.

Não adicionar CORS se não houver necessidade real.

Preservar execução como usuário não-root.

Adicionar healthcheck/build validation quando apropriado.
```

### Critério de conclusão

- Imagem de produção compila frontend e backend de forma reproduzível.
- Rotas de SPA funcionam ao serem acessadas diretamente.
- `/api/*` e `/health` não são capturados pelo fallback da SPA.
- Contêiner continua executando como usuário não-root.

---

## Prompt 10 — Remoção do legado

```text
Faça uma auditoria final antes de remover o frontend legado.

Somente remova Go Templates, HTMX e Alpine se TODAS estas áreas estiverem migradas:

- setup
- login/logout
- dashboard
- nodes
- node details
- backup content
- diff
- node create/edit/delete
- import/export CSV
- credentials
- routines
- users
- alerts
- SFTP
- export
- logs
- profile

Procure por:
- c.Render
- HX-Request
- HX-Target
- HX-Trigger
- HX-Push-Url
- htmx
- x-data
- x-show
- x-cloak
- Alpine

Verifique se ainda existem consumidores legítimos.

Depois:

- remover github.com/gofiber/template/html/v2 se não utilizado
- remover configuração do template engine de main.go
- remover templates/
- remover scripts HTMX/Alpine
- remover CSS morto somente após auditoria
- atualizar Dockerfile
- atualizar CONTEXT.md
- atualizar README.md
- atualizar DESIGN.md somente onde arquitetura frontend mudou

Execute:
- go test ./...
- frontend lint
- frontend tests
- frontend build

Não remova nenhuma regra de segurança apenas porque pertencia ao frontend antigo.
```

### Critério de conclusão

- Todas as áreas listadas possuem substituição funcional validada.
- Busca por referências legadas foi revisada manualmente antes da exclusão.
- Suite Go, lint, testes e build do frontend passam.
- Documentação reflete a arquitetura final.

---

## Sequência recomendada de PRs

```text
PR 01 - docs: frontend migration inventory
PR 02 - feat(api): API foundation and auth/me
PR 03 - feat(api): JSON authentication
PR 04 - feat(frontend): Vite React shell
PR 05 - feat(dashboard): API + React dashboard
PR 06 - feat(nodes): read API + React nodes
PR 07 - feat(nodes): mutations + forms
PR 08 - feat(nodes): import/export
PR 09 - feat(settings): profile/credentials/routines
PR 10 - feat(settings): users/logs
PR 11 - feat(settings): alerts/SFTP/export
PR 12 - feat(setup): React onboarding/login
PR 13 - build: integrated frontend Docker build
PR 14 - refactor: remove templates/HTMX/Alpine
PR 15 - cleanup: CSS and dead frontend code
```

## Checklist transversal por PR

- [ ] Escopo limitado ao domínio ou etapa indicada.
- [ ] Rotas e comportamentos legados não relacionados permanecem intactos.
- [ ] Autenticação e RBAC são verificados no backend.
- [ ] DTOs não expõem senhas, hashes, chaves, tokens ou campos criptografados.
- [ ] Audit logs são mantidos nas operações relevantes.
- [ ] Proteções `Origin`/`Referer`, cookies e rate limiting são preservadas.
- [ ] Regras de negócio não foram copiadas entre adaptadores HTML e JSON.
- [ ] Testes novos cobrem sucesso, validação, `401` e `403`, quando aplicável.
- [ ] Testes Go existentes continuam passando.
- [ ] Lint, testes e build do frontend passam, quando aplicável.
- [ ] Documentação foi atualizada se a arquitetura ou operação mudou.

## Riscos principais

### Vazamento de dados sensíveis

Models como `Credential`, `Node`, `SftpSettings` e `AlertRule` podem conter senhas, tokens, chaves ou campos criptografados. Nunca retorne esses models diretamente com `c.JSON(model)`. Use DTOs explícitos e testes de ausência de dados sensíveis.

### Divergência entre HTML e API

Durante a transição, dois adaptadores atenderão os mesmos fluxos. Validação, normalização, persistência, auditoria e autorização devem ser extraídas para funções ou serviços reutilizáveis. Evite criar uma cópia de `forms.go` para a API.

### Regressões de autenticação

A API deve reutilizar as sessões existentes, mas responder com JSON e códigos HTTP adequados. Preserve regeneração de sessão, dummy hash, rate limiting, cookies seguros, logs de auditoria e proteção de origem.

### Remoção prematura do legado

Templates, HTMX e Alpine só devem ser removidos após validação de equivalência funcional. A presença de poucos usos restantes ainda pode representar fluxos críticos, como setup, importação, testes SFTP ou administração.

### Escopo inflado

Esta é uma migração de frontend. Não use a mudança para reescrever scheduler, SSH, crypto, diff, persistência ou serviços de backup sem uma necessidade demonstrada e um escopo separado.

## Regra de ouro

Não transforme `forms.go` em duas implementações paralelas. À medida que cada domínio for migrado, extraia a regra de negócio do handler para uma camada reutilizável. Enquanto houver convivência, HTML e JSON devem chamar a mesma lógica. Quando o frontend legado for removido, deve desaparecer apenas o adaptador HTML — não metade do comportamento da aplicação.

## Primeiro marco recomendado

Comece por **PR 01 + PR 02**: inventário completo e fundação de `/api/v1`. Só crie componentes React depois de os contratos, DTOs e comportamentos de autenticação da API estarem definidos e testados.
