// Gera relatorio-auditoria-seguranca.html a partir de report-data.js.
// Uso: node generate-report.js
// Depois: node render-pdf.js  (imprime o HTML em PDF via Chrome/Edge headless)

const fs = require("fs");
const path = require("path");
const data = require("./report-data.js");

function esc(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function nl2br(s) {
  return esc(s).replace(/\n/g, "<br/>");
}

const severityBg = {
  critica: "#B91C1C",
  alta: "#EA580C",
  media: "#D97706",
  baixa: "#2563EB",
  informativa: "#64748B",
};

// ---- Gráfico de rosca (donut) via conic-gradient, calculado em JS ----
function buildDonut(counts) {
  const total = counts.reduce((a, c) => a + c.value, 0) || 1;
  let acc = 0;
  const stops = [];
  counts.forEach((c) => {
    const start = (acc / total) * 360;
    acc += c.value;
    const end = (acc / total) * 360;
    if (c.value > 0) {
      stops.push(`${c.color} ${start.toFixed(2)}deg ${end.toFixed(2)}deg`);
    }
  });
  const gradient = stops.length
    ? `conic-gradient(${stops.join(", ")})`
    : "conic-gradient(#e2e8f0 0deg 360deg)";
  const legend = counts
    .map(
      (c) =>
        `<div class="legend-row"><span class="legend-dot" style="background:${c.color}"></span>` +
        `<span class="legend-label">${esc(c.label)}</span><span class="legend-value">${c.value}</span></div>`
    )
    .join("");
  return `
  <div class="chart-card">
    <h3>Achados por severidade</h3>
    <div class="donut-wrap">
      <div class="donut" style="background:${gradient}">
        <div class="donut-hole"><strong>${total}</strong><span>achados</span></div>
      </div>
      <div class="legend">${legend}</div>
    </div>
  </div>`;
}

// ---- Gráfico de barras horizontal (achados por categoria) ----
function buildBars(counts) {
  const max = Math.max(...counts.map((c) => c.value), 1);
  const rows = counts
    .map((c) => {
      const pct = Math.max((c.value / max) * 100, c.value > 0 ? 4 : 0);
      const color = c.value > 0 ? "#334155" : "#cbd5e1";
      return `
      <div class="bar-row">
        <div class="bar-label">${esc(c.label)}</div>
        <div class="bar-track"><div class="bar-fill" style="width:${pct}%;background:${color}"></div></div>
        <div class="bar-value">${c.value}</div>
      </div>`;
    })
    .join("");
  return `
  <div class="chart-card">
    <h3>Achados por categoria</h3>
    <div class="bars">${rows}</div>
  </div>`;
}

function chip(sev, label) {
  const bg = severityBg[sev] || "#64748B";
  return `<span class="chip" style="background:${bg}">${esc(label)}</span>`;
}

function buildFindingCard(f) {
  return `
  <div class="finding-card" id="finding-${f.id}">
    <div class="finding-head">
      ${chip(f.severity, f.severityLabel)}
      <span class="finding-id">${f.id}</span>
      <span class="finding-title">${esc(f.title)}</span>
    </div>
    <div class="finding-meta">
      <span><strong>Arquivo:</strong> <code>${esc(f.file)}</code></span>
      <span><strong>Linhas:</strong> <code>${esc(f.lines)}</code></span>
      <span><strong>Categoria:</strong> ${esc(f.category)}</span>
    </div>
    <p>${esc(f.description)}</p>
    <pre class="code-block">${esc(f.snippet)}</pre>
    <div class="finding-grid">
      <div><h4>Por que é explorável</h4><p>${esc(f.why)}</p></div>
      <div><h4>Condições de explorabilidade</h4><p>${esc(f.exploitability)}</p></div>
      <div><h4>Recomendação</h4><p>${esc(f.recommendation)}</p></div>
    </div>
  </div>`;
}

function buildFindingsTableRow(f) {
  return `
  <tr>
    <td>${chip(f.severity, f.severityLabel)}</td>
    <td><code>${esc(f.file)}:${esc(f.lines.split(",")[0].split(";")[0])}</code></td>
    <td>${esc(f.title)}</td>
  </tr>`;
}

function buildStrengths(items) {
  return items
    .map(
      (s) => `
    <div class="strength-card">
      <div class="strength-title"><span class="strength-dot"></span>${esc(s.title)}</div>
      <p>${esc(s.evidence)}</p>
    </div>`
    )
    .join("");
}

function buildStack(rows) {
  return rows
    .map(([k, v]) => `<tr><td>${esc(k)}</td><td>${esc(v)}</td></tr>`)
    .join("");
}

function buildMethodology(rows) {
  return rows
    .map(
      ([k, v]) => `
    <div class="method-row">
      <div class="method-key">${esc(k)}</div>
      <div class="method-val">${esc(v)}</div>
    </div>`
    )
    .join("");
}

function buildRecommendations(items) {
  return items
    .map(
      (r) => `
    <div class="reco-row">
      <div class="reco-priority">${esc(r.priority)}</div>
      <div class="reco-text">${esc(r.text)}</div>
    </div>`
    )
    .join("");
}

// ---- Seção final: issues do GitHub prontas para copiar/colar ----
function issueBody({ title, labels, summary, evidenceBlocks, impact, fix, acceptance }) {
  const ev = evidenceBlocks
    .map((e) => `**${e.file}:${e.lines}**\n\`\`\`\n${e.snippet}\n\`\`\``)
    .join("\n\n");
  const accList = acceptance.map((a) => `- [ ] ${a}`).join("\n");
  return `# ${title}

**Labels sugeridas:** ${labels.join(", ")}

## Descrição do problema

${summary}

## Evidência

${ev}

## Impacto

${impact}

## Sugestão de correção

${fix}

## Critérios de aceite

${accList}`;
}

const issues = [
  issueBody({
    title: "[Segurança] SSRF via DNS rebinding na validação de webhooks de alerta",
    labels: ["security", "baixa"],
    summary:
      "ValidateWebhookURL valida o host do webhook e bloqueia IPs privados/loopback no momento do " +
      "salvamento/teste da regra, mas SendWebhook deixa o net/http padrão re-resolver o DNS no " +
      "momento da conexão real, sem fixar o IP validado. Um domínio com TTL curto pode resolver " +
      "para um IP público na validação e para um alvo interno (127.0.0.1, rede privada) no momento " +
      "do disparo real, contornando o bloqueio de SSRF. Requer papel Administrator (permissão " +
      "ManageSystem) para cadastrar a regra de alerta.",
    evidenceBlocks: [
      {
        file: "internal/services/alert/alert.go",
        lines: "130-159",
        snippet:
          "func SendWebhook(url string, message string) error {\n\tif err := ValidateWebhookURL(url); err != nil {\n\t\treturn err\n\t}\n\t...\n\tclient := &http.Client{Timeout: 10 * time.Second}\n\tresp, err := client.Do(req)",
      },
    ],
    impact:
      "Um Administrator malicioso ou uma conta de Administrator comprometida pode usar o servidor da " +
      "aplicação como pivô para escanear/alcançar serviços internos que não deveriam ser expostos, " +
      "contornando o bloqueio de SSRF já existente.",
    fix:
      "Resolver o hostname uma única vez, validar o(s) IP(s) resultante(s) e reutilizá-los na conexão " +
      "TCP real (por exemplo, um http.Transport com DialContext customizado que conecta diretamente " +
      "ao IP validado, preservando o header Host original), em vez de validar a URL textual e deixar " +
      "o cliente HTTP padrão resolver o DNS de novo no momento do request.",
    acceptance: [
      "SendWebhook (e qualquer outro chamador de rede a partir de URL fornecida pelo usuário) conecta " +
        "ao IP resolvido e validado, não a um novo lookup de DNS",
      "Teste automatizado cobrindo um cenário de DNS rebinding (mock de resolver retornando IPs " +
        "diferentes em validação vs. conexão) confirma o bloqueio",
      "Comportamento de bloqueio para IPs privados/loopback/link-local continua funcionando (regressão)",
    ],
  }),
  issueBody({
    title: "[Segurança] Cookie de sessão sem flag Secure por padrão",
    labels: ["security", "baixa"],
    summary:
      "CookieSecure só é habilitado se COOKIE_SECURE=true for definido explicitamente. " +
      ".env.example e docker-compose.yml usam 'false' como padrão sugerido, então uma instalação " +
      "seguindo a documentação sem alterar esse valor roda com cookies de sessão sem o atributo " +
      "Secure, que poderiam ser transmitidos em texto claro se a aplicação for exposta via HTTP em " +
      "algum trecho do caminho de rede.",
    evidenceBlocks: [
      {
        file: "cmd/mimic/main.go",
        lines: "108-113",
        snippet:
          'store := session.New(session.Config{\n\tExpiration:     24 * time.Hour,\n\tCookieHTTPOnly: true,\n\tCookieSameSite: "Strict",\n\tCookieSecure:   strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),\n})',
      },
      {
        file: ".env.example",
        lines: "7-8",
        snippet:
          "# Enable this when the application is served through HTTPS.\nCOOKIE_SECURE=false",
      },
    ],
    impact:
      "Em uma instalação exposta via HTTP (sem TLS ponta a ponta), o cookie de sessão pode ser " +
      "capturado por um atacante na mesma rede (ex. Wi-Fi compartilhado, proxy interno mal " +
      "configurado), permitindo sequestro de sessão.",
    fix:
      "Inverter o valor padrão sugerido para 'true' em .env.example e docker-compose.yml, deixando " +
      "clara a exceção apenas para desenvolvimento local sem TLS; opcionalmente, detectar " +
      "automaticamente X-Forwarded-Proto quando atrás de um proxy confiável.",
    acceptance: [
      ".env.example e docker-compose.yml passam a sugerir COOKIE_SECURE=true por padrão",
      "README/DOCKER.md documentam explicitamente quando é seguro definir 'false' (apenas dev local)",
    ],
  }),
  issueBody({
    title: "[Segurança] Hardening de segredos: exemplo de senha fraca na documentação e fallback silencioso de SECRET_KEY",
    labels: ["security", "informativa"],
    summary:
      "Dois achados relacionados a gestão de segredos, agrupados por serem do mesmo tema: " +
      "(1) TUTORIAL.md usa a senha literal 'password' em um exemplo de criação de usuário PostgreSQL " +
      "e na DATABASE_URL de um serviço systemd — com aviso ao lado, mas sem ser um placeholder " +
      "obviamente inválido. (2) pkg/crypto/crypto.go gera e persiste silenciosamente uma nova " +
      "SECRET_KEY em .mimic_secret quando a variável de ambiente não está definida, sem log de aviso, " +
      "o que pode mascarar uma configuração ausente em instalações fora do Docker Compose (que já " +
      "torna a variável obrigatória).",
    evidenceBlocks: [
      {
        file: "TUTORIAL.md",
        lines: "63-66, 83",
        snippet:
          "> **Warning:** Change `password` to a secure password.\nCREATE USER mimic WITH ENCRYPTED PASSWORD 'password';\n...\nEnvironment=\"DATABASE_URL=postgres://mimic:password@localhost:5432/mimic_db?sslmode=disable\"",
      },
      {
        file: "pkg/crypto/crypto.go",
        lines: "32-61",
        snippet:
          'secretFile := ".mimic_secret"\ndata, err := os.ReadFile(secretFile)\nif err == nil { ... }\n// Generate new key\nnewKey := make([]byte, 24)\n...\nos.WriteFile(secretFile, []byte(b64Key), 0600)',
      },
    ],
    impact:
      "(1) Banco de dados protegido por senha trivial se o operador ignorar o aviso. " +
      "(2) Perda do arquivo .mimic_secret (reinstalação, migração sem backup) torna credenciais SSH/" +
      "SFTP/Webhook já criptografadas permanentemente indecifráveis, sem que o operador tenha sido " +
      "avisado de que dependia desse arquivo.",
    fix:
      "Trocar 'password' por um placeholder que falha visivelmente se não for substituído (ex. " +
      "'<TROQUE_ESTA_SENHA>'). Emitir log.Printf de aviso quando o fallback de geração de SECRET_KEY " +
      "for usado, e considerar recusar a subida quando um modo de produção estiver sinalizado.",
    acceptance: [
      "TUTORIAL.md não contém mais uma senha literal 'funcional' em exemplos de configuração",
      "Aplicação loga um aviso claro no startup quando SECRET_KEY é gerada automaticamente",
      "Documentação menciona explicitamente a necessidade de backup do .mimic_secret quando esse " +
        "fallback é usado",
    ],
  }),
  issueBody({
    title: "[Segurança] Rate limiting de login apenas por IP, sem bloqueio por conta",
    labels: ["security", "informativa"],
    summary:
      "O limiter de POST /login usa a chave padrão do middleware Fiber (endereço IP do cliente), " +
      "permitindo 10 tentativas por minuto por IP. Não existe um contador complementar por username, " +
      "então um atacante com múltiplos IPs (proxies, botnet) pode tentar senhas contra uma conta " +
      "específica sem esbarrar em limite algum.",
    evidenceBlocks: [
      {
        file: "cmd/mimic/main.go",
        lines: "194-203",
        snippet:
          'loginLimiter := limiter.New(limiter.Config{\n\tMax:        10,\n\tExpiration: time.Minute,\n\tLimitReached: func(c *fiber.Ctx) error { ... },\n})\napp.Post("/login", loginLimiter, authHandler.PostLogin)',
      },
    ],
    impact:
      "Brute force distribuído contra uma conta específica não é mitigado pelo rate limit atual, " +
      "embora o custo por tentativa já seja elevado por bcrypt + comparação de tempo constante.",
    fix:
      "Adicionar um contador de falhas de login por username (Redis, tabela dedicada ou coluna em " +
      "User) com bloqueio temporário progressivo após N tentativas, complementando — não substituindo " +
      "— o rate limit por IP já existente.",
    acceptance: [
      "Login com um mesmo username falhando N vezes seguidas (mesmo de IPs diferentes) resulta em " +
        "bloqueio temporário da conta",
      "O bloqueio por conta não introduz um vetor de enumeração de usuários (mensagem de erro " +
        "permanece genérica)",
    ],
  }),
];

const issuesHtml = issues
  .map(
    (body, i) => `
  <div class="issue-block">
    <div class="issue-marker">--- ISSUE ${i + 1} ---</div>
    <pre class="issue-body">${esc(body)}</pre>
    <div class="issue-marker">--- FIM ISSUE ${i + 1} ---</div>
  </div>`
  )
  .join("\n");

const totalFindings = data.findings.length;

const css = `
  :root{
    --ink:#0f172a; --muted:#475569; --line:#e2e8f0; --bg:#ffffff; --panel:#f8fafc;
    --critica:#B91C1C; --alta:#EA580C; --media:#D97706; --baixa:#2563EB; --forte:#059669;
  }
  *{box-sizing:border-box}
  html,body{margin:0;padding:0}
  body{
    font-family:'Segoe UI',-apple-system,BlinkMacSystemFont,Arial,sans-serif;
    color:var(--ink); background:var(--bg); font-size:12.5px; line-height:1.55;
  }
  h1,h2,h3,h4{font-family:'Segoe UI',Arial,sans-serif; margin:0 0 8px 0; color:var(--ink)}
  p{margin:0 0 8px 0}
  code{font-family:'Consolas','Courier New',monospace; background:#f1f5f9; padding:1px 5px; border-radius:4px; font-size:11px}
  .page{page-break-after:always; padding:4px 2px}
  .page:last-child{page-break-after:auto}

  /* Capa */
  .cover{display:flex; flex-direction:column; justify-content:center; min-height:920px; text-align:left}
  .cover .kicker{color:var(--muted); letter-spacing:2px; text-transform:uppercase; font-size:11px; font-weight:600; margin-bottom:18px}
  .cover h1{font-size:34px; line-height:1.25; max-width:620px}
  .cover .sub{font-size:15px; color:var(--muted); margin-top:10px; max-width:560px}
  .cover .meta{margin-top:40px; display:flex; gap:40px}
  .cover .meta div strong{display:block; font-size:11px; text-transform:uppercase; color:var(--muted); letter-spacing:1px; margin-bottom:4px}
  .cover .badge-bar{display:flex; gap:8px; margin-top:34px}
  .cover .badge-bar span{padding:5px 12px; border-radius:20px; font-size:11px; font-weight:600; color:#fff}
  .cover-footer{margin-top:60px; border-top:1px solid var(--line); padding-top:16px; color:var(--muted); font-size:11px}

  .section-title{font-size:20px; border-bottom:3px solid var(--ink); padding-bottom:8px; margin-bottom:18px}
  .section-sub{color:var(--muted); font-size:12px; margin-top:-10px; margin-bottom:18px}

  table.stack-table{width:100%; border-collapse:collapse; margin-top:8px}
  table.stack-table td{padding:6px 10px; border-bottom:1px solid var(--line); font-size:12px; vertical-align:top}
  table.stack-table td:first-child{color:var(--muted); width:220px; font-weight:600}

  .method-row{display:flex; gap:14px; padding:10px 0; border-bottom:1px solid var(--line)}
  .method-key{width:230px; font-weight:700; font-size:11.5px; flex-shrink:0}
  .method-val{font-size:11.5px; color:var(--ink)}

  /* Resumo executivo / gráficos */
  .charts-row{display:flex; gap:24px; margin-top:10px}
  .chart-card{flex:1; border:1px solid var(--line); border-radius:10px; padding:16px; background:var(--panel)}
  .chart-card h3{font-size:13px; margin-bottom:14px}
  .donut-wrap{display:flex; align-items:center; gap:20px}
  .donut{width:150px; height:150px; border-radius:50%; position:relative; flex-shrink:0}
  .donut-hole{position:absolute; inset:26px; background:#fff; border-radius:50%; display:flex; flex-direction:column; align-items:center; justify-content:center; box-shadow:inset 0 0 0 1px var(--line)}
  .donut-hole strong{font-size:22px; line-height:1}
  .donut-hole span{font-size:9px; color:var(--muted); text-transform:uppercase; letter-spacing:0.5px}
  .legend{display:flex; flex-direction:column; gap:7px}
  .legend-row{display:flex; align-items:center; gap:8px; font-size:11.5px}
  .legend-dot{width:9px; height:9px; border-radius:50%; display:inline-block; flex-shrink:0}
  .legend-label{flex:1; min-width:90px}
  .legend-value{font-weight:700}
  .bars{display:flex; flex-direction:column; gap:12px; margin-top:6px}
  .bar-row{display:flex; align-items:center; gap:10px}
  .bar-label{width:160px; font-size:11px; color:var(--muted); flex-shrink:0}
  .bar-track{flex:1; background:#e2e8f0; border-radius:5px; height:14px; overflow:hidden}
  .bar-fill{height:100%; border-radius:5px}
  .bar-value{width:18px; text-align:right; font-weight:700; font-size:11.5px}

  .kpi-row{display:flex; gap:14px; margin:18px 0}
  .kpi{flex:1; border:1px solid var(--line); border-radius:10px; padding:12px 14px; text-align:center; background:#fff}
  .kpi strong{display:block; font-size:22px}
  .kpi span{font-size:10.5px; color:var(--muted); text-transform:uppercase; letter-spacing:0.5px}

  /* Pontos fortes / fracos */
  .strength-card{border:1px solid var(--line); border-left:4px solid var(--forte); border-radius:6px; padding:10px 14px; margin-bottom:10px; background:#fff}
  .strength-title{font-weight:700; font-size:12px; margin-bottom:4px; display:flex; align-items:center; gap:8px}
  .strength-dot{width:8px; height:8px; border-radius:50%; background:var(--forte); flex-shrink:0}
  .strength-card p{font-size:11px; color:var(--muted); margin:0}

  /* Tabela de achados */
  table.findings-table{width:100%; border-collapse:collapse; margin-top:10px}
  table.findings-table th{text-align:left; font-size:10.5px; text-transform:uppercase; letter-spacing:0.5px; color:var(--muted); border-bottom:2px solid var(--ink); padding:6px 8px}
  table.findings-table td{padding:8px; border-bottom:1px solid var(--line); font-size:11px; vertical-align:top}
  .chip{display:inline-block; color:#fff; font-size:10px; font-weight:700; padding:3px 9px; border-radius:20px; text-transform:uppercase; letter-spacing:0.3px}

  /* Cards de achado detalhado */
  .finding-card{border:1px solid var(--line); border-radius:10px; padding:16px; margin-bottom:16px; page-break-inside:avoid}
  .finding-head{display:flex; align-items:center; gap:10px; margin-bottom:8px}
  .finding-id{font-weight:800; color:var(--muted); font-size:11px}
  .finding-title{font-weight:700; font-size:13px}
  .finding-meta{display:flex; gap:18px; font-size:10.5px; color:var(--muted); margin-bottom:10px; flex-wrap:wrap}
  .code-block{background:#0f172a; color:#e2e8f0; padding:10px 12px; border-radius:8px; font-size:10.5px; overflow-x:auto; white-space:pre-wrap; margin:8px 0}
  .finding-grid{display:grid; grid-template-columns:1fr 1fr 1fr; gap:12px; margin-top:10px}
  .finding-grid h4{font-size:10.5px; text-transform:uppercase; letter-spacing:0.4px; color:var(--muted); margin-bottom:4px}
  .finding-grid p{font-size:10.5px; margin:0}

  /* Recomendações */
  .reco-row{display:flex; gap:14px; padding:10px 0; border-bottom:1px solid var(--line); align-items:flex-start}
  .reco-priority{width:34px; flex-shrink:0; font-weight:800; font-size:13px; color:#fff; background:var(--ink); border-radius:6px; text-align:center; padding:4px 0}
  .reco-text{font-size:11.5px; padding-top:3px}

  /* Issues GitHub */
  .issue-block{margin-bottom:22px; page-break-inside:avoid}
  .issue-marker{font-family:'Consolas','Courier New',monospace; font-size:10px; color:var(--muted); margin:6px 0}
  .issue-body{background:#f8fafc; border:1px dashed #94a3b8; border-radius:8px; padding:14px; font-family:'Consolas','Courier New',monospace; font-size:9.5px; white-space:pre-wrap; word-wrap:break-word}

  .toc{margin-top:10px}
  .toc a{display:block; padding:5px 0; border-bottom:1px dotted var(--line); font-size:11.5px; color:var(--ink); text-decoration:none}
`;

const html = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8"/>
<title>Relatório de Auditoria — ${esc(data.project.name)}</title>
<style>${css}</style>
</head>
<body>

  <!-- CAPA -->
  <section class="page cover">
    <div class="kicker">Relatório confidencial de auditoria</div>
    <h1>Relatório de Auditoria de Segurança<br/>${esc(data.project.name)}</h1>
    <p class="sub">Auditoria de código-fonte cobrindo isolamento de acesso (RBAC), autorização no
    backend, IDOR, exposição de segredos e tratamento de entradas (XSS), com mapeamento de cada
    categoria para a stack real do projeto.</p>

    <div class="meta">
      <div><strong>Data</strong>${esc(data.project.date)}</div>
      <div><strong>Versão auditada</strong>v${esc(data.project.version)}</div>
      <div><strong>Repositório</strong>${esc(data.project.repo)}</div>
    </div>

    <div class="badge-bar">
      <span style="background:${severityBg.baixa}">2 achados de severidade baixa</span>
      <span style="background:${severityBg.informativa}">3 achados informativos</span>
      <span style="background:${severityBg.critica === "#B91C1C" ? "#059669" : "#059669"}">0 críticos / 0 altos</span>
    </div>

    <div class="cover-footer">
      <strong>Escopo auditado:</strong> ${esc(data.project.scope)}
    </div>
  </section>

  <!-- STACK E METODOLOGIA -->
  <section class="page">
    <h2 class="section-title">Stack detectada e nota metodológica</h2>
    <p class="section-sub">Como cada categoria da auditoria foi mapeada para a stack real do projeto antes da varredura.</p>

    <h3>Stack técnica identificada</h3>
    <table class="stack-table">${buildStack(data.project.stack)}</table>

    <h3 style="margin-top:22px">Mapeamento categoria → mecanismo real</h3>
    <div class="method-list">${buildMethodology(data.project.methodology)}</div>
  </section>

  <!-- RESUMO EXECUTIVO -->
  <section class="page">
    <h2 class="section-title">Resumo executivo</h2>

    <div class="kpi-row">
      <div class="kpi"><strong>${totalFindings}</strong><span>Achados totais</span></div>
      <div class="kpi"><strong>0</strong><span>Críticos</span></div>
      <div class="kpi"><strong>0</strong><span>Altos</span></div>
      <div class="kpi"><strong>2</strong><span>Baixos</span></div>
      <div class="kpi"><strong>3</strong><span>Informativos</span></div>
      <div class="kpi"><strong>${data.strengths.length}</strong><span>Pontos fortes verificados</span></div>
    </div>

    <div class="charts-row">
      ${buildDonut(data.severityCounts)}
      ${buildBars(data.categoryCounts)}
    </div>

    <p style="margin-top:18px; font-size:11.5px; color:var(--muted)">
      Nenhuma falha crítica, alta ou média foi identificada. Os cinco achados reportados são de
      severidade baixa ou informativa e, em sua maioria, exigem que o ator já possua privilégio
      elevado (Administrator) ou dependam de uma escolha de configuração de deploy/documentação —
      não representam uma via de comprometimento remoto não autenticado.
    </p>
  </section>

  <!-- PONTOS FORTES -->
  <section class="page">
    <h2 class="section-title">Pontos fortes (protegido, com evidência)</h2>
    <p class="section-sub">Comportamentos corretos verificados no código, listados para demonstrar a cobertura da auditoria.</p>
    ${buildStrengths(data.strengths)}
  </section>

  <!-- TABELA DE ACHADOS -->
  <section class="page">
    <h2 class="section-title">Achados detalhados por categoria</h2>
    <table class="findings-table">
      <thead><tr><th>Severidade</th><th>Arquivo:linha</th><th>Descrição</th></tr></thead>
      <tbody>
        ${data.findings.map(buildFindingsTableRow).join("")}
      </tbody>
    </table>

    <h3 style="margin-top:26px">Detalhamento técnico</h3>
    ${data.findings.map(buildFindingCard).join("")}
  </section>

  <!-- RECOMENDAÇÕES -->
  <section class="page">
    <h2 class="section-title">Recomendações priorizadas</h2>
    ${buildRecommendations(data.recommendations)}
  </section>

  <!-- ISSUES PARA O GITHUB -->
  <section class="page">
    <h2 class="section-title">Issues para o GitHub</h2>
    <p class="section-sub">Texto completo em Markdown, pronto para copiar e colar em uma nova issue.</p>
    ${issuesHtml}
  </section>

</body>
</html>`;

const outPath = path.join(__dirname, "relatorio-auditoria-seguranca.html");
fs.writeFileSync(outPath, html, "utf8");
console.log("HTML gerado em:", outPath);
