// Imprime relatorio-auditoria-seguranca.html em PDF usando o Chrome/Edge já
// instalado na máquina, via Chrome DevTools Protocol (CDP) puro (sem
// dependências npm — usa apenas módulos nativos do Node + WebSocket global).
//
// Uso: node render-pdf.js

const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const http = require("http");

const HTML_PATH = path.join(__dirname, "relatorio-auditoria-seguranca.html");
const PDF_PATH = path.join(__dirname, "relatorio-auditoria-seguranca.pdf");
const DEBUG_PORT = 9333;

const CANDIDATES = [
  "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
  "C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
  "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
  "C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
];

function findBrowser() {
  for (const p of CANDIDATES) {
    if (fs.existsSync(p)) return p;
  }
  throw new Error("Nenhum Chrome/Edge encontrado nos caminhos padrão.");
}

function httpGetJson(url) {
  return new Promise((resolve, reject) => {
    http
      .get(url, (res) => {
        let data = "";
        res.on("data", (c) => (data += c));
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch (e) {
            reject(e);
          }
        });
      })
      .on("error", reject);
  });
}

function httpRequestJson(method, url) {
  return new Promise((resolve, reject) => {
    const req = http.request(url, { method }, (res) => {
      let data = "";
      res.on("data", (c) => (data += c));
      res.on("end", () => {
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject(new Error(`resposta não-JSON (${res.statusCode}): ${data}`));
        }
      });
    });
    req.on("error", reject);
    req.end();
  });
}

function waitForPort(port, timeoutMs) {
  const start = Date.now();
  return new Promise((resolve, reject) => {
    const tick = () => {
      httpGetJson(`http://127.0.0.1:${port}/json/version`)
        .then(resolve)
        .catch(() => {
          if (Date.now() - start > timeoutMs) reject(new Error("timeout esperando o Chrome subir"));
          else setTimeout(tick, 200);
        });
    };
    tick();
  });
}

async function main() {
  const browserPath = findBrowser();
  console.log("Usando navegador:", browserPath);

  const userDataDir = path.join(require("os").tmpdir(), "mimic-audit-pdf-profile");
  const args = [
    "--headless=new",
    "--disable-gpu",
    "--no-sandbox",
    `--remote-debugging-port=${DEBUG_PORT}`,
    `--user-data-dir=${userDataDir}`,
    "about:blank",
  ];

  const child = spawn(browserPath, args, { stdio: "ignore", detached: true });
  child.unref();

  try {
    await waitForPort(DEBUG_PORT, 15000);

    // Cria uma nova aba/target apontando para o arquivo HTML local.
    const fileUrl = "file:///" + HTML_PATH.replace(/\\/g, "/");
    const newTab = await httpRequestJson(
      "PUT",
      `http://127.0.0.1:${DEBUG_PORT}/json/new?${encodeURIComponent(fileUrl)}`
    );
    const wsUrl = newTab.webSocketDebuggerUrl;

    const ws = new WebSocket(wsUrl);
    await new Promise((resolve, reject) => {
      ws.addEventListener("open", resolve);
      ws.addEventListener("error", reject);
    });

    let nextId = 1;
    const pending = new Map();
    ws.addEventListener("message", (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.id && pending.has(msg.id)) {
        const { resolve, reject } = pending.get(msg.id);
        pending.delete(msg.id);
        if (msg.error) reject(new Error(JSON.stringify(msg.error)));
        else resolve(msg.result);
      }
    });

    function send(method, params = {}) {
      const id = nextId++;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        ws.send(JSON.stringify({ id, method, params }));
      });
    }

    await send("Page.enable");

    // Aguarda o carregamento completo da página.
    await new Promise((resolve) => {
      const handler = (ev) => {
        const msg = JSON.parse(ev.data);
        if (msg.method === "Page.loadEventFired") {
          ws.removeEventListener("message", handler);
          resolve();
        }
      };
      ws.addEventListener("message", handler);
      // fallback: se já carregou antes de registrar o listener
      setTimeout(resolve, 4000);
    });

    // pequena espera extra para fontes/layout assentarem
    await new Promise((r) => setTimeout(r, 500));

    const headerTemplate = `
      <div style="font-size:8px; width:100%; padding:0 15mm; color:#64748B; display:flex; justify-content:space-between; font-family:Arial,sans-serif;">
        <span>Relatório de Auditoria de Segurança — Mimic Backup Systems</span>
      </div>`;
    const footerTemplate = `
      <div style="font-size:8px; width:100%; padding:0 15mm; color:#64748B; display:flex; justify-content:space-between; font-family:Arial,sans-serif;">
        <span>Confidencial</span>
        <span>Página <span class="pageNumber"></span> de <span class="totalPages"></span></span>
      </div>`;

    const result = await send("Page.printToPDF", {
      landscape: false,
      printBackground: true,
      preferCSSPageSize: false,
      paperWidth: 8.27, // A4
      paperHeight: 11.69,
      marginTop: 0.9, // ~2.3cm para acomodar o header
      marginBottom: 0.9,
      marginLeft: 0.79, // 2cm
      marginRight: 0.79,
      displayHeaderFooter: true,
      headerTemplate,
      footerTemplate,
      scale: 1.0,
    });

    fs.writeFileSync(PDF_PATH, Buffer.from(result.data, "base64"));
    console.log("PDF gerado em:", PDF_PATH);

    ws.close();
  } finally {
    try {
      process.kill(child.pid);
    } catch (e) {
      /* ignore */
    }
  }
}

main().catch((err) => {
  console.error("Falha ao gerar PDF:", err);
  process.exit(1);
});
