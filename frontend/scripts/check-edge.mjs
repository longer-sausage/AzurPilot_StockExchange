import assert from "node:assert/strict";
import { get } from "node:https";

const base = new URL(process.argv[2] ?? "https://stock.azurpilot.com").origin;
const timeout = 15000;
const request = (path, init = {}) =>
  fetch(base + path, { ...init, signal: AbortSignal.timeout(timeout) });
const json = (response) => {
  assert.match(response.headers.get("Content-Type") ?? "", /application\/json/);
  return response.json();
};

// 仅执行公开或未认证的读取；不登录，不创建账户，不发起交易。
const home = await request("/");
assert.equal(home.status, 200);
assert.match(home.headers.get("Content-Type") ?? "", /text\/html/);
const html = await home.text();
const readAssets = (source) =>
  [
    ...new Set(
      [
        ...source.matchAll(
          /(?:src|href)="([^"?#]+\.(?:js|css))(?:[?#][^"]*)?"/g,
        ),
      ]
        .map((match) => match[1])
        .filter((path) => path.startsWith("/_next/")),
    ),
  ].sort();
const assets = readAssets(html);
assert.ok(assets.length > 0, "首页应引用 Next.js 静态资源");
await Promise.all(
  [...new Set(assets)].map(async (path) => {
    const response = await request(path);
    assert.equal(response.status, 200, path);
    assert.match(
      response.headers.get("Content-Type") ?? "",
      path.endsWith(".css") ? /text\/css/ : /javascript/,
      path,
    );
    await response.arrayBuffer();
  }),
);
// Node fetch 会把 Sec-Fetch-Mode 覆盖为 cors；用 HTTPS 客户端模拟真正的页面导航。
const consolePage = await new Promise((resolve, reject) => {
  const navigation = get(
    new URL("/console", base),
    {
      headers: { "Sec-Fetch-Mode": "navigate", Accept: "text/html" },
    },
    (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => {
        body += chunk;
      });
      response.on("end", () => resolve({ status: response.statusCode, body }));
      response.on("error", reject);
    },
  );
  navigation.on("error", reject);
  navigation.setTimeout(timeout, () =>
    navigation.destroy(Error("导航检查超时")),
  );
});
assert.equal(consolePage.status, 200);
// Cloudflare 可能注入逐请求变化的安全脚本，比较应用入口资源而非整个 HTML。
assert.deepEqual(
  readAssets(consolePage.body),
  assets,
  "旧管理入口应加载同一个 SPA",
);
console.log(
  `静态首页、SPA fallback、${new Set(assets).size} 个 JS/CSS 资源通过`,
);

const health = await request("/healthz?edge-check=1");
assert.equal(health.status, 200);
assert.match(health.headers.get("Cache-Control") ?? "", /no-store/);
assert.deepEqual(await json(health), { apiVersion: 2, ok: true });
const market = await request("/api/v2/public/market");
assert.equal(market.status, 200);
assert.match(market.headers.get("Cache-Control") ?? "", /no-cache|no-store/);
const etag = market.headers.get("ETag");
assert.ok(etag, "行情应提供 ETag");
await json(market);
const cached = await request("/api/v2/public/market", {
  headers: { "If-None-Match": etag },
});
assert.equal(cached.status, 304);
assert.equal(await cached.text(), "");
const me = await request("/api/v2/auth/me");
assert.equal(me.status, 401);
assert.equal((await json(me)).error.code, "UNAUTHORIZED");
for (const path of ["/api", "/api/v1/me", "/api/v2/not-a-real-endpoint"]) {
  const missing = await request(path);
  assert.equal(missing.status, 404, path);
  await json(missing);
}
console.log("API 回源、健康检查、ETag/304、未登录 401、未知 API 404 通过");

const sameOrigin = await request("/healthz", { headers: { Origin: base } });
assert.equal(sameOrigin.status, 200);
await json(sameOrigin);
const crossOrigin = await request("/healthz", {
  headers: { Origin: "https://untrusted.example" },
});
assert.equal(crossOrigin.status, 403);
assert.equal((await json(crossOrigin)).error.code, "ORIGIN_DENIED");
console.log("同源访问与跨站 Origin 拦截通过");

const events = await request("/api/v2/public/events");
assert.equal(events.status, 200);
assert.match(events.headers.get("Content-Type") ?? "", /text\/event-stream/);
// nginx 会消费 X-Accel-Buffering，公网响应可能没有它；直接读取首条事件验证未被缓冲。
assert.match(events.headers.get("Cache-Control") ?? "", /no-cache/);
const reader = events.body.getReader();
const decoder = new TextDecoder();
let firstEvent = "";
try {
  while (!firstEvent.includes("\n\n")) {
    const chunk = await reader.read();
    assert.equal(chunk.done, false, "SSE 不应提前关闭");
    firstEvent += decoder.decode(chunk.value, { stream: true });
  }
  assert.match(firstEvent, /event: change/);
  assert.match(firstEvent, /data:.*"version"/);
} finally {
  await reader.cancel();
}
console.log(`SSE 首条版本事件即时到达；${base} 验证通过`);
