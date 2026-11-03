// 仅本地开发夹具。独立于 Next 页面及正式 Go 服务，不连接数据库或线上 API。
import { createServer } from "node:http";
import { makeDemo } from "../ui-tests/demo-data.mjs";

import { serveStatic } from "./static-files.mjs";
const port = Number(process.env.EXCHANGE_FRONTEND_PORT ?? 5178);
let demo = makeDemo();
const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url, "http://127.0.0.1");
    if (url.pathname === "/__demo/reset" && req.method === "POST") {
      demo.close();
      demo = makeDemo();
      res.writeHead(204);
      return res.end();
    }
    if (url.pathname.startsWith("/api/v2/"))
      return await demo.handle(req, res, url);
    if (url.pathname === "/healthz") {
      res.setHeader("Content-Type", "application/json");
      return res.end('{"ok":true,"demo":true}');
    }
    await serveStatic(req, res);
  } catch (error) {
    console.error(error.message);
    res.writeHead(500);
    res.end("本地预览错误");
  }
});
server.listen(port, "127.0.0.1", () =>
  console.log(
    `假数据预览 http://127.0.0.1:${port}；玩家 预览玩家 / Demo-only-Password!87；管理员 Demo-only-Admin!87`,
  ),
);
function close() {
  demo.close();
  server.close(() => process.exit(0));
  server.closeAllConnections();
}
process.on("SIGINT", close);
process.on("SIGTERM", close);
