import { readFile, stat } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
const root = path.resolve(fileURLToPath(new URL("../dist/", import.meta.url)));
const mime = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".txt": "text/plain; charset=utf-8",
  ".ico": "image/x-icon",
  ".webp": "image/webp",
};
// 与 Go 当前静态路由相同，/console 对应根 HTML fallback。
export async function serveStatic(req, res) {
  const url = new URL(req.url, "http://127.0.0.1");
  let file = path.resolve(root, "." + decodeURIComponent(url.pathname));
  if (file !== root && !file.startsWith(root + path.sep)) {
    res.writeHead(403);
    return res.end();
  }
  try {
    if (!(await stat(file)).isFile()) file = path.join(root, "index.html");
  } catch {
    if (url.pathname.startsWith("/_next/")) {
      res.writeHead(404);
      return res.end();
    }
    file = path.join(root, "index.html");
  }
  res.setHeader(
    "Content-Type",
    mime[path.extname(file)] ?? "application/octet-stream",
  );
  res.setHeader("Cache-Control", "no-store");
  res.end(await readFile(file));
}
