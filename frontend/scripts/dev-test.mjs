import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { createServer, request as proxyRequest } from "node:http";
import { serveStatic } from "./static-files.mjs";
const root = fileURLToPath(new URL("../../", import.meta.url));
const database = process.env.EXCHANGE_E2E_DATABASE_URL;
if (
  !database ||
  !/^postgres(?:ql)?:\/\/[^@]+@(127\.0\.0\.1|localhost):\d+\/[^?]+/.test(
    database,
  )
)
  throw new Error(
    "请提供仅指向回环隔离数据库的 EXCHANGE_E2E_DATABASE_URL；测试不得使用生产数据库",
  );
const apiPort = Number(process.env.EXCHANGE_API_PORT ?? 18889),
  frontendPort = Number(process.env.EXCHANGE_FRONTEND_PORT ?? 5178);
if (
  !Number.isInteger(apiPort) ||
  !Number.isInteger(frontendPort) ||
  apiPort < 1024 ||
  apiPort > 65535 ||
  frontendPort < 1024 ||
  frontendPort > 65535
)
  throw new Error("隔离测试端口无效");
const apiURL = `http://127.0.0.1:${apiPort}`;
const child = spawn("go", ["run", "./cmd/exchange"], {
  cwd: root,
  stdio: "inherit",
  env: {
    ...process.env,
    DATABASE_URL: database,
    LISTEN_ADDR: `127.0.0.1:${apiPort}`,
    ADMIN_PASSWORD: "isolated-admin-password",
    BOT_API_TOKEN: "isolated-bot-token-for-tests-".repeat(3),
    SESSION_SECRET: "isolated-session-secret-for-tests-".repeat(3),
    COOKIE_SECURE: "false",
    ALLOWED_ORIGINS: `http://127.0.0.1:${frontendPort}`,
    DEFAULTS_FILE: "config/defaults.json",
  },
});
let server,
  closing = false;
async function close() {
  if (closing) return;
  closing = true;
  await server?.close();
  if (process.platform === "win32" && child.pid)
    spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
      stdio: "ignore",
    });
  else child.kill("SIGTERM");
  setTimeout(() => process.exit(0), 250);
}
process.on("SIGINT", close);
process.on("SIGTERM", close);
child.on("error", (e) => {
  console.error(e.message);
  process.exit(1);
});
child.on("exit", (code) => {
  if (!closing) {
    console.error(`隔离后端退出 (${code})`);
    process.exit(code ?? 1);
  }
});
for (let i = 0; i < 300; i++) {
  try {
    if ((await fetch(`${apiURL}/healthz`)).ok) break;
  } catch {}
  if (i === 299) throw new Error("隔离后端启动超时");
  await new Promise((r) => setTimeout(r, 200));
}
server = createServer((req, res) => {
  if (req.url.startsWith("/api/") || req.url.startsWith("/healthz")) {
    const proxy = proxyRequest(
      new URL(req.url, apiURL),
      { method: req.method, headers: req.headers },
      (response) => {
        res.writeHead(response.statusCode, response.headers);
        response.pipe(res);
      },
    );
    proxy.on("error", () => {
      if (!res.headersSent) res.writeHead(502);
      res.end();
    });
    req.pipe(proxy);
    res.on("close", () => proxy.destroy());
    return;
  }
  void serveStatic(req, res).catch((error) => {
    console.error(error.message);
    res.writeHead(500);
    res.end();
  });
});
server.listen(frontendPort, "127.0.0.1", () =>
  console.log(`当前构建产物 + 隔离 Go 后端：http://127.0.0.1:${frontendPort}`),
);
