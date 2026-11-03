import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { generateKeyPairSync, randomUUID, sign } from "node:crypto";
const bot = {
    Authorization: `Bearer ${"isolated-bot-token-for-tests-".repeat(3)}`,
  },
  strong = "Isolated-New-Password!87";
async function registerStock(request: APIRequestContext, name: string) {
  const keys = generateKeyPairSync("ed25519"),
    instanceId = randomUUID(),
    publicKey = keys.publicKey
      .export({ type: "spki", format: "der" })
      .subarray(-32)
      .toString("base64");
  const body = (
    action: string,
    username = "",
    points: { t: number; p: number }[] = [],
  ) => {
    const issuedAt = Math.floor(Date.now() / 1000),
      canonical = `mmex-v2\n${action}\n${instanceId}\n${publicKey}\n${issuedAt}\n${username}\n${JSON.stringify(points)}\n`;
    return {
      instanceId,
      publicKey,
      issuedAt,
      username,
      points,
      signature: sign(null, Buffer.from(canonical), keys.privateKey).toString(
        "base64",
      ),
    };
  };
  const created = await request.post("/api/v2/instance/register", {
    data: body("register", name),
  });
  expect(created.ok(), await created.text()).toBe(true);
  const state = await created.json();
  expect(
    (
      await request.post("/api/v2/instance/report", {
        data: body(
          "report",
          "",
          Array.from({ length: 80 }, (_, i) => ({
            t: Date.now() - (80 - i) * 60000,
            p: 1000 + i,
          })),
        ),
      })
    ).ok(),
  ).toBe(true);
  return state;
}
async function player(request: APIRequestContext) {
  const officialUserId = `e2e:${randomUUID()}`,
    username = `玩家${randomUUID().slice(0, 8)}`,
    result = await request.post("/api/v2/bot/register", {
      headers: bot,
      data: { officialUserId, username, requestId: randomUUID() },
    });
  expect(result.ok()).toBe(true);
  return { officialUserId, username, ...(await result.json()) };
}
async function login(page: Page, username: string, password: string) {
  await page.getByRole("button", { name: "登录交易账户", exact: true }).click();
  const d = page.getByRole("dialog", { name: "登录交易账户" });
  await d.getByLabel("账号", { exact: true }).fill(username);
  await d.getByLabel("密码", { exact: true }).fill(password);
  await d.getByRole("button", { name: "登录", exact: true }).click();
  await expect(d).not.toBeVisible();
}
test("真实 Go 行情、图表和固定根地址的 SPA 导航", async ({ page, request }) => {
  const name = `行情${randomUUID().slice(0, 8)}`,
    s = await registerStock(request, name);
  expect(s.stock.id).toBeGreaterThan(0);
  await page.goto("/");
  await page.getByRole("link", { name, exact: true }).click();
  await expect(page).toHaveURL(/:\d+\/$/);
  await expect(page.locator('[role="img"] canvas')).toBeVisible();
  await page.getByRole("tab", { name: "M5", exact: true }).click();
  await expect(page.locator('[role="img"] canvas')).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
});
test("真实机器人开户、强制改密、准入与股票绑定", async ({ page, request }) => {
  const a = await player(request),
    stock = await registerStock(request, `归属${randomUUID().slice(0, 8)}`);
  await page.goto("/");
  await login(page, a.username, a.password);
  const d = page.getByRole("dialog", { name: "设置你的交易密码" });
  await expect(d).toBeVisible();
  await d.getByLabel("当前临时密码").fill(a.password);
  await d.getByLabel("新密码", { exact: true }).fill(strong);
  await d.getByLabel("再次输入新密码").fill(strong);
  await d.getByRole("button", { name: "设置密码并继续" }).click();
  const binding = page.getByRole("dialog", { name: "关联你的实例股票" });
  await expect(binding).toBeVisible();
  expect((await page.request.get("/api/v2/account")).status()).toBe(400);
  expect(
    (
      await request.post("/api/v2/bot/bind", {
        headers: bot,
        data: {
          officialUserId: a.officialUserId,
          bindingCode: stock.bindingCode,
        },
      })
    ).ok(),
  ).toBe(true);
  if (await binding.isVisible())
    await binding.getByRole("button", { name: "检查绑定状态" }).click();
  await expect(binding).not.toBeVisible();
  await page.getByRole("tab", { name: "我的资产" }).click();
  await expect(page.getByRole("tab", { name: "当前持仓" })).toBeVisible();
  expect((await page.request.get("/api/v2/account")).ok()).toBe(true);
});
test("真实交易预览、重复确认和账户成交记录", async ({ page, request }) => {
  const market = await (await request.get("/api/v2/public/market")).json();
  test.skip(!market.open, "隔离库当前赛期未开放，保留后端休市规则");
  const a = await player(request),
    own = await registerStock(request, `自有${randomUUID().slice(0, 8)}`),
    target = await registerStock(request, `标的${randomUUID().slice(0, 8)}`);
  expect(
    (
      await request.post("/api/v2/auth/login", {
        data: { username: a.username, password: a.password },
      })
    ).ok(),
  ).toBe(true);
  expect(
    (
      await request.post("/api/v2/auth/password", {
        data: { currentPassword: a.password, password: strong },
      })
    ).ok(),
  ).toBe(true);
  expect(
    (
      await request.post("/api/v2/bot/bind", {
        headers: bot,
        data: {
          officialUserId: a.officialUserId,
          bindingCode: own.bindingCode,
        },
      })
    ).ok(),
  ).toBe(true);
  await page.goto(`/#/security?stock=${target.stock.id}`);
  await login(page, a.username, strong);
  await page.getByLabel("委托股数", { exact: true }).fill("1");
  await page.getByLabel("委托限价", { exact: true }).fill("2000");
  const preview = page.waitForResponse((r) =>
    r.url().endsWith("/orders/preview"),
  );
  await page.getByRole("button", { name: "预览交易明细" }).click();
  const p = await preview;
  expect(p.ok(), await p.text()).toBe(true);
  const data = await p.json();
  await page
    .getByRole("dialog", { name: "确认交易委托" })
    .getByRole("button", { name: "确认提交委托" })
    .click();
  await expect(
    page.getByRole("dialog", { name: "确认交易委托" }),
  ).not.toBeVisible();
  const first = await (await page.request.get("/api/v2/account")).json();
  expect(
    (
      await page.request.post("/api/v2/orders/confirm", {
        data: { previewId: data.previewId },
      })
    ).ok(),
  ).toBe(true);
  const second = await (await page.request.get("/api/v2/account")).json();
  expect(second.orders.length).toBe(first.orders.length);
  expect(second.account.cash).toBe(first.account.cash);
});
