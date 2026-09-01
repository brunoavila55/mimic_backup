// Dados estruturados da auditoria de segurança do Mimic Backup Systems.
// Edite este arquivo para atualizar o conteúdo e rode `node generate-report.js`
// seguido de `node render-pdf.js` para regerar o PDF.

module.exports = {
  project: {
    name: "Mimic Backup Systems",
    version: "0.8.1",
    repo: "mimic_backup",
    date: "31 de agosto de 2026",
    scope:
      "Código-fonte completo do backend Go (cmd/mimic, internal/handlers, internal/middleware, " +
      "internal/access, internal/models, internal/services/{ssh,sftp,alert,scheduler}, pkg/crypto), " +
      "todos os templates Go HTML server-side (templates/**), arquivos de deploy (Dockerfile, " +
      "docker-compose.yml, .env.example), documentação de instalação (TUTORIAL.md, DOCKER.md) e " +
      "histórico completo do git (git log --all -p) em busca de segredos commitados.",
    stack: [
      ["Linguagem / Runtime", "Go 1.25"],
      ["Framework web", "Fiber v2 (gofiber/fiber)"],
      ["ORM / Banco", "GORM + PostgreSQL 15 (queries parametrizadas)"],
      ["Autenticação", "Sessão via cookie (gofiber/session), bcrypt para senhas"],
      ["Autorização", "RBAC por papel (Administrator/Operator/Auditor/Viewer) via middleware Fiber"],
      ["Frontend", "Go html/template (server-side, auto-escape) + HTMX + Alpine.js"],
      ["Criptografia de segredos", "AES-GCM 256-bit (pkg/crypto) para credenciais SSH/SFTP/Webhook"],
      ["Deploy", "Docker multi-stage + docker-compose (Postgres + app), usuário não-root"],
    ],
    methodology: [
      [
        "1. Banco sem tranca (isolamento de inquilino)",
        "O projeto não usa Supabase/RLS e não possui modelo multi-tenant (não há OrgID/TenantID/WorkspaceID " +
          "em nenhuma entidade GORM). É uma aplicação single-tenant: um deployment atende uma única " +
          "organização. O mecanismo de isolamento real é o RBAC (papéis Administrator/Operator/Auditor/Viewer) " +
          "aplicado via middleware Fiber (RequirePermission/RequireAdmin) em cada rota. A auditoria verificou " +
          "a presença desse middleware em todas as 47 rotas registradas em cmd/mimic/main.go.",
      ],
      [
        "2. Permissão definida no navegador",
        "Cada elemento de UI condicionado por papel nos templates Go (CanManageUsers, CanManageNodes, " +
          "comparações {{if eq .Role \"...\"}}) foi cruzado com o middleware da rota correspondente que o " +
          "botão/link aciona em cmd/mimic/main.go.",
      ],
      [
        "3. IDOR",
        "Todo handler que recebe um :id via c.Params/c.Query/FormValue (nodes, backups, users, credentials, " +
          "routines, alert rules) foi lido individualmente em internal/handlers/handlers.go e " +
          "internal/handlers/forms.go (arquivos completos, não amostras) para confirmar existência de " +
          "verificação de posse/permissão antes de ler, alterar ou excluir o recurso.",
      ],
      [
        "4. Chaves expostas",
        "Busca por regex de segredos (api key, secret key, senha literal, tokens, blocos BEGIN PRIVATE KEY) " +
          "em todos os arquivos rastreados pelo git (git grep) e em todo o histórico de commits " +
          "(git log --all -p), além de leitura manual de pkg/crypto/crypto.go, docker-compose.yml, " +
          "Dockerfile, .env.example e TUTORIAL.md/DOCKER.md.",
      ],
      [
        "5. Inputs sem tratamento (XSS)",
        "Go html/template escapa por padrão qualquer valor interpolado em contexto HTML/atributo/URL/JS. " +
          "A auditoria buscou (grep) por template.HTML, funções 'safe'/'noescape' e concatenação manual de " +
          "HTML nos handlers. Como não há SPA (React/Vue), buscas por innerHTML/v-html/dangerouslySetInnerHTML " +
          "foram feitas nos templates HTMX/Alpine.js para confirmar que hx-swap=innerHTML sempre recebe " +
          "fragmentos já renderizados (e escapados) pelo próprio servidor, não dados brutos via JS.",
      ],
    ],
  },

  // Contagem de achados por severidade (para o gráfico de rosca).
  severityCounts: [
    { key: "critica", label: "Crítica", value: 0, color: "#B91C1C" },
    { key: "alta", label: "Alta", value: 0, color: "#EA580C" },
    { key: "media", label: "Média", value: 0, color: "#D97706" },
    { key: "baixa", label: "Baixa", value: 2, color: "#2563EB" },
    { key: "informativa", label: "Informativa", value: 3, color: "#64748B" },
  ],

  // Contagem de achados por categoria (para o gráfico de barras).
  categoryCounts: [
    { label: "1. Isolamento / RBAC", value: 0 },
    { label: "2. Permissão no navegador", value: 0 },
    { label: "3. IDOR", value: 0 },
    { label: "4. Chaves expostas", value: 2 },
    { label: "5. XSS", value: 0 },
    { label: "Outros (hardening)", value: 3 },
  ],

  strengths: [
    {
      title: "RBAC aplicado no servidor em 100% das rotas sensíveis",
      evidence:
        "cmd/mimic/main.go:213-272 — todas as 29 rotas de mutação/gestão (nodes, users, credentials, " +
        "routines, sftp, alerts, export, trigger de backup) passam por middleware.RequirePermission(...) " +
        "ou middleware.RequireAdmin() antes do handler. Nenhuma rota de escrita foi encontrada sem gate.",
    },
    {
      title: "Nenhuma inconsistência entre UI condicional e backend",
      evidence:
        "templates/node_list.html:11, templates/partials/node_table_body.html:85,145,153 escondem " +
        "'New Node/Edit/Delete/Manual Backup' para papéis sem permissão; as rotas correspondentes " +
        "(/nodes/new, /nodes/:id/edit, /nodes/:id/delete, /nodes/:id/trigger) exigem exatamente " +
        "access.ManageNodes ou access.RunBackups no servidor (cmd/mimic/main.go:214-225,283-296).",
      },
    {
      title: "Nenhum IDOR encontrado nas rotas CRUD",
      evidence:
        "Todos os handlers com :id (NodeDetails, EditNode/SaveNode/DeleteNode, EditUser/SaveUser/DeleteUser, " +
        "EditCredential/SaveCredential/DeleteCredential, EditRoutine/SaveRoutine/DeleteRoutine, " +
        "EditAlertRule/SaveAlertRule/DeleteAlertRule) buscam o recurso por ID e retornam 404 se ausente; " +
        "como o app é single-tenant e RBAC-gated, não há escopo de 'dono' a violar. SaveProfile " +
        "(internal/handlers/forms.go:1428) deriva o ID exclusivamente de c.Locals(\"user_id\") da sessão, " +
        "nunca de parâmetro externo — impossível editar o perfil de outro usuário por esse endpoint.",
    },
    {
      title: "Proteções de conta e sessão bem implementadas",
      evidence:
        "internal/handlers/forms.go:762-776,826-837 impedem auto-rebaixamento e remoção do último " +
        "administrador; internal/handlers/auth.go:21-55 usa hash bcrypt dummy para tempo constante " +
        "(anti-enumeração de usuário); sess.Regenerate() é chamado em todo login bem-sucedido " +
        "(auth.go:61, setup.go:188) prevenindo session fixation.",
    },
    {
      title: "Nenhum segredo hardcoded no código-fonte rastreado ou no histórico git",
      evidence:
        "git grep e git log --all -p não encontraram chaves de API, senhas literais ou blocos de chave " +
        "privada em nenhum arquivo rastreado. .env (com segredos reais gerados localmente) está listado " +
        "em .gitignore:2-3 e nunca foi commitado (git log --all --oneline -- .env retornou vazio). " +
        "docker-compose.yml:11,13,34 usa a sintaxe ${VAR:?mensagem}, que interrompe a subida do container " +
        "se POSTGRES_PASSWORD/SECRET_KEY não forem definidos — não há fallback inseguro.",
    },
    {
      title: "SECRET_KEY validado no startup, nunca aceito com menos de 32 caracteres",
      evidence:
        "pkg/crypto/crypto.go:32-39 rejeita SECRET_KEY com menos de 32 caracteres; " +
        "cmd/mimic/main.go:54-56 chama appcrypto.ValidateSecretKey() antes de qualquer rota subir, " +
        "com log.Fatalf em caso de falha.",
    },
    {
      title: "Nenhuma XSS encontrada — escaping consistente",
      evidence:
        "Nenhuma ocorrência de template.HTML, template.JS ou funções 'safe'/'noescape' em todo o " +
        "repositório (grep). A única construção manual de HTML fora do template engine " +
        "(internal/handlers/handlers.go:688, mensagem de erro do explorador SFTP) usa html.EscapeString " +
        "antes de interpolar. Exportação CSV usa csvSafe() (internal/handlers/handlers.go:241-251) para " +
        "neutralizar injeção de fórmula em Excel/Sheets nos campos controlados pelo usuário.",
    },
    {
      title: "SSRF parcialmente mitigado em webhooks de alerta",
      evidence:
        "internal/services/alert/alert.go:88-120 resolve o host do webhook e bloqueia IPs loopback, " +
        "privados, link-local e não especificados antes de qualquer disparo (ValidateWebhookURL é chamado " +
        "em SaveAlertRule, TestAlertRule e novamente dentro de SendWebhook). Ver achado F1 para o gap " +
        "residual de DNS rebinding.",
    },
    {
      title: "CSRF mitigado por checagem de Origin/Referer com fail-closed",
      evidence:
        "internal/middleware/security.go:14-51 rejeita qualquer requisição de mutação (não GET/HEAD/OPTIONS) " +
        "sem Origin nem Referer, e rejeita Origin/Referer que não batam com o Host — coberto por teste " +
        "(internal/middleware/security_test.go). Cookie de sessão usa SameSite=Strict e HttpOnly " +
        "(cmd/mimic/main.go:108-113).",
    },
    {
      title: "Comandos SSH por vendor são strings estáticas, sem interpolação de dados do usuário",
      evidence:
        "internal/services/ssh/vendors/{cisco,mikrotik,huawei,juniper}.go retornam comandos fixos " +
        "(\"show running-config\", \"/export\", etc.); internal/services/ssh/service.go:223-237 apenas " +
        "concatena essas strings estáticas com '; ', sem incluir Node.Name/IP/Username do usuário no " +
        "comando remoto — não há injeção de comando no dispositivo de rede.",
    },
    {
      title: "Upload de avatar restrito e sem travessia de diretório",
      evidence:
        "internal/handlers/forms.go:1336-1404 valida tamanho (2 MB), sniffa o Content-Type real " +
        "(image/jpeg ou image/png apenas) e as dimensões da imagem antes de salvar; o nome do arquivo é " +
        "gerado no servidor (user-<id>-<timestamp>.ext), nunca a partir de input do usuário, e a remoção " +
        "usa filepath.Base() sobre o valor armazenado — sem risco de path traversal.",
    },
  ],

  findings: [
    {
      id: "F1",
      severity: "baixa",
      severityLabel: "Baixa",
      category: "Outros (SSRF)",
      file: "internal/services/alert/alert.go",
      lines: "88-159",
      title: "SSRF via DNS rebinding (TOCTOU) na validação de webhooks de alerta",
      description:
        "ValidateWebhookURL/isBlockedWebhookHost resolvem o hostname do webhook e bloqueiam IPs " +
        "privados/loopback/link-local no momento da validação (linhas 88-120). Porém SendWebhook " +
        "(linhas 130-159) reutiliza um http.Client padrão que refaz a resolução DNS no momento da " +
        "conexão TCP, sem fixar (pin) o IP validado. Um atacante que já possua a permissão " +
        "ManageSystem (necessária para criar/testar Alert Rules) pode registrar um domínio com TTL " +
        "curto que resolve para um IP público na validação e para 127.0.0.1/rede interna " +
        "milissegundos depois, no momento real da requisição HTTP — contornando o bloqueio de SSRF.",
      snippet:
        "func SendWebhook(url string, message string) error {\n" +
        "\tif err := ValidateWebhookURL(url); err != nil {\n" +
        "\t\treturn err\n" +
        "\t}\n" +
        "\t...\n" +
        "\tclient := &http.Client{Timeout: 10 * time.Second}\n" +
        "\tresp, err := client.Do(req) // nova resolução DNS aqui, sem IP pinning",
      why:
        "Técnica clássica de bypass de SSRF (DNS rebinding): validação e uso ocorrem em dois " +
        "momentos distintos, cada um com sua própria resolução de nome.",
      exploitability:
        "Pré-condição: exige um usuário já autenticado com papel Administrator (única role com " +
        "permissão access.ManageSystem — ver internal/access/access.go:31-47), controle de um domínio " +
        "próprio com TTL de DNS baixo, e uma rede interna alcançável a partir do host da aplicação. " +
        "Como esse ator já possui controle total do sistema, o impacto adicional é limitado a " +
        "reconhecimento/pivô de rede interna a partir do servidor da aplicação.",
      recommendation:
        "Resolver o host uma única vez, validar o IP resultante e reutilizá-lo na conexão real " +
        "(ex.: net.Dialer com DialContext customizado que conecta diretamente ao IP validado, mantendo " +
        "o header Host original), em vez de validar uma URL e deixar o net/http padrão re-resolver o DNS.",
    },
    {
      id: "F2",
      severity: "baixa",
      severityLabel: "Baixa",
      category: "Outros (Config)",
      file: "cmd/mimic/main.go / .env.example",
      lines: "main.go:108-113; .env.example:8",
      title: "Cookie de sessão sem a flag Secure por padrão",
      description:
        "O Secure flag do cookie de sessão só é ativado se a variável de ambiente COOKIE_SECURE " +
        "estiver explicitamente definida como 'true'. O .env.example traz COOKIE_SECURE=false como " +
        "valor padrão sugerido, e o docker-compose.yml usa ${COOKIE_SECURE:-false} como fallback. " +
        "Uma instalação seguindo a documentação sem alterar esse valor roda com cookies de sessão " +
        "transmissíveis em texto claro caso a aplicação seja exposta via HTTP (ex.: atrás de um " +
        "reverse proxy mal configurado, ou em rede interna sem TLS).",
      snippet:
        "store := session.New(session.Config{\n" +
        "\tExpiration:     24 * time.Hour,\n" +
        "\tCookieHTTPOnly: true,\n" +
        "\tCookieSameSite: \"Strict\",\n" +
        "\tCookieSecure:   strings.EqualFold(os.Getenv(\"COOKIE_SECURE\"), \"true\"),\n" +
        "})",
      why:
        "Sem o atributo Secure, o navegador pode enviar o cookie de sessão em uma conexão HTTP não " +
        "criptografada caso exista qualquer caminho de rede sem TLS até a aplicação, permitindo " +
        "captura de sessão por um atacante na mesma rede (ex.: Wi-Fi compartilhado, proxy interno).",
      exploitability:
        "Só é explorável se a instância for servida via HTTP (sem TLS) em algum ponto do caminho de " +
        "rede do cliente — não afeta instalações atrás de TLS/reverse proxy com HTTPS ponta a ponta. " +
        "O próprio README recomenda 'Enable this when the application is served through HTTPS', mas " +
        "o valor padrão do exemplo é 'false'.",
      recommendation:
        "Inverter o padrão para 'true' e documentar a exceção explícita para ambientes de " +
        "desenvolvimento local sem TLS, ou detectar automaticamente X-Forwarded-Proto quando atrás de " +
        "um proxy confiável.",
    },
    {
      id: "F3",
      severity: "informativa",
      severityLabel: "Informativa",
      category: "4. Chaves expostas",
      file: "TUTORIAL.md",
      lines: "63-66, 83",
      title: "Exemplo de senha literal fraca na documentação de instalação bare-metal",
      description:
        "O tutorial de instalação manual mostra a criação do usuário PostgreSQL com a senha literal " +
        "'password' e a mesma string na DATABASE_URL do serviço systemd de exemplo. Há um aviso " +
        "('> Warning: Change `password` to a secure password.') logo acima, mas o valor em si não é " +
        "um placeholder obviamente inválido — instaladores apressados podem copiar e colar sem trocar.",
      snippet:
        "> **Warning:** Change `password` to a secure password.\n" +
        "```sql\n" +
        "CREATE USER mimic WITH ENCRYPTED PASSWORD 'password';\n" +
        "```\n" +
        "...\n" +
        'Environment="DATABASE_URL=postgres://mimic:password@localhost:5432/mimic_db?sslmode=disable"',
      why:
        "Não é uma vulnerabilidade de código, mas um risco operacional: se o operador ignorar o aviso, " +
        "o banco fica protegido por uma senha trivialmente adivinhável.",
      exploitability:
        "Requer que o operador siga o tutorial sem seguir a instrução de troca, E que o PostgreSQL " +
        "esteja acessível pela rede (por padrão TUTORIAL.md configura o serviço para localhost).",
      recommendation:
        "Substituir 'password' por um placeholder que falha de forma óbvia se não for trocado, ex. " +
        "'<TROQUE_ESTA_SENHA>', em vez de uma senha que 'funciona' se deixada como está.",
    },
    {
      id: "F4",
      severity: "informativa",
      severityLabel: "Informativa",
      category: "4. Chaves expostas",
      file: "pkg/crypto/crypto.go",
      lines: "32-61",
      title: "Fallback silencioso: chave de criptografia gerada e persistida em disco se SECRET_KEY não for definida",
      description:
        "Quando SECRET_KEY não está no ambiente, loadSecretKey() tenta ler .mimic_secret e, se " +
        "ausente, gera 24 bytes aleatórios, codifica em base64 e grava em .mimic_secret com permissão " +
        "0600. Isso evita uma chave hardcoded (positivo), mas é uma degradação silenciosa: a aplicação " +
        "sobe normalmente sem avisar que está operando com uma chave não gerenciada pelo operador. " +
        "Em Docker (docker-compose.yml) isso é mitigado porque SECRET_KEY é obrigatória via " +
        "${SECRET_KEY:?...}, mas uma instalação bare-metal (TUTORIAL.md) sem exportar SECRET_KEY " +
        "cairia nesse fallback sem qualquer log de alerta.",
      snippet:
        "secretFile := \".mimic_secret\"\n" +
        "data, err := os.ReadFile(secretFile)\n" +
        "if err == nil { ... }\n" +
        "// Generate new key\n" +
        "newKey := make([]byte, 24)\n" +
        "...\n" +
        "os.WriteFile(secretFile, []byte(b64Key), 0600)",
      why:
        "Se o arquivo .mimic_secret for perdido (ex.: reinstalação, migração de host sem copiar o " +
        "arquivo), todas as credenciais SSH/SFTP/Webhook já criptografadas no banco tornam-se " +
        "permanentemente indecifráveis — mais um risco de continuidade operacional do que de " +
        "confidencialidade direta, mas vale nota de segurança porque mascara uma configuração ausente.",
      exploitability:
        "Não é explorável remotamente; é um problema de gestão de configuração/chaves em instalações " +
        "que não seguem o Docker Compose (que já força a variável).",
      recommendation:
        "Emitir um log.Printf de aviso claro quando o fallback for usado ('SECRET_KEY not set, " +
        "generated and persisted a new key at .mimic_secret — back this file up'), e considerar " +
        "recusar a subida em modo produção (ex. quando um APP_ENV=production estiver definido).",
    },
    {
      id: "F5",
      severity: "informativa",
      severityLabel: "Informativa",
      category: "Outros (Auth)",
      file: "cmd/mimic/main.go",
      lines: "194-203",
      title: "Rate limiting de login aplicado apenas por IP, sem bloqueio por conta",
      description:
        "O limiter de /login permite 10 tentativas por minuto por IP (chave padrão do middleware " +
        "limiter do Fiber é c.IP()). Não existe bloqueio complementar por username após N falhas. Um " +
        "atacante distribuído (múltiplos IPs/proxies) pode tentar senhas contra uma única conta sem " +
        "esbarrar nesse limite, já que cada IP tem sua própria cota.",
      snippet:
        "loginLimiter := limiter.New(limiter.Config{\n" +
        "\tMax:        10,\n" +
        "\tExpiration: time.Minute,\n" +
        "\t...\n" +
        "})\n" +
        'app.Post("/login", loginLimiter, authHandler.PostLogin)',
      why:
        "Rate limiting só por IP não impede brute force distribuído contra uma conta específica; " +
        "bcrypt (DefaultCost) e a checagem de tempo constante (auth.go) já elevam bastante o custo por " +
        "tentativa, mas o limite de taxa é a defesa que faltaria numa distribuição de origem.",
      exploitability:
        "Requer um atacante com acesso a múltiplos endereços IP (botnet, proxies rotativos) e um " +
        "username válido conhecido ou adivinhável.",
      recommendation:
        "Adicionar um contador de falhas por username (ex. Redis/DB) com bloqueio temporário " +
        "progressivo após N tentativas, complementando o limite por IP já existente.",
    },
  ],

  recommendations: [
    {
      priority: "P1",
      text:
        "Corrigir o SSRF por DNS rebinding em internal/services/alert/alert.go: resolver o host uma " +
        "única vez, validar o IP e reutilizá-lo na conexão HTTP real (Dialer customizado), em vez de " +
        "validar a URL e deixar o net/http padrão re-resolver o DNS no momento da requisição.",
    },
    {
      priority: "P2",
      text:
        "Inverter o padrão de COOKIE_SECURE para 'true' em .env.example e docker-compose.yml, com " +
        "instrução explícita de exceção apenas para desenvolvimento local sem TLS.",
    },
    {
      priority: "P3",
      text:
        "Higienizar exemplos de credenciais na documentação (TUTORIAL.md) trocando 'password' por um " +
        "placeholder que falha visivelmente se não for substituído, e emitir um log de aviso quando " +
        "pkg/crypto usar o fallback de geração automática de SECRET_KEY em disco.",
    },
    {
      priority: "P4",
      text:
        "Reforçar o rate limiting de /login com um contador de falhas por username (além do limite " +
        "por IP já existente), para mitigar brute force distribuído contra uma conta específica.",
    },
  ],
};
