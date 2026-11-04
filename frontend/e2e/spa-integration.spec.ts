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
async function registerStock(
  request: APIRequestContext,
  name: string,
  points = Array.from({ length: 80 }, (_, i) => ({
    t: Date.now() - (80 - i) * 60000,
    p: 1000 + i,
  })),
) {
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
        data: body("report", "", points),
      })
    ).ok(),
  ).toBe(true);
  return {
    ...state,
    report: (points: { t: number; p: number }[]) =>
      request.post("/api/v2/instance/report", {
        data: { ...body("report", "", points), backfill: true },
      }),
  };
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
  await page.getByLabel("搜索证券", { exact: true }).fill(name);
  const row = page
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name, exact: true }) });
  await expect(
    row.getByRole("img", { name: /分钟报价走势/ }).locator("canvas"),
  ).toBeVisible();
  await row.getByText("可交易", { exact: true }).click();
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
  // 绑定成功后 SSE 会自动关闭弹窗，直接等待结果，避免点击已卸载的按钮。
  await expect(binding).not.toBeVisible();
  await page
    .locator(".navigation")
    .getByRole("tab", { name: "我的持仓" })
    .click();
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

test("真实市价委托部分成交和零成交均撤销余量，解冻且不进入挂单", async ({
  page,
  request,
}) => {
  const market = await (await request.get("/api/v2/public/market")).json();
  test.skip(!market.open, "隔离库当前赛期未开放，保留后端休市规则");
  const target = await registerStock(
      request,
      `市价${randomUUID().slice(0, 8)}`,
    ),
    a = await player(page.request),
    own = await registerStock(request, `自有${randomUUID().slice(0, 8)}`);
  expect(
    (
      await request.post("/api/v2/admin/login", {
        data: { password: "isolated-admin-password" },
      })
    ).ok(),
  ).toBe(true);
  const { stocks } = await (await request.get("/api/v2/admin/stocks")).json(),
    stock = stocks.find((s: { id: number }) => s.id === target.stock.id);
  const updated = await request.put(`/api/v2/admin/stocks/${stock.id}`, {
    data: { ...stock, hourlyVolume: 3 },
  });
  expect(updated.ok(), await updated.text()).toBe(true);
  expect(
    (
      await page.request.post("/api/v2/auth/login", {
        data: { username: a.username, password: a.password },
      })
    ).ok(),
  ).toBe(true);
  expect(
    (
      await page.request.post("/api/v2/auth/password", {
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
  await page.goto(`/?stock=${stock.id}`);
  await page.getByRole("combobox", { name: "委托类型", exact: true }).click();
  await page.getByRole("option", { name: "市价", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "有效期", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("combobox", { name: "有效期", exact: true }),
  ).toContainText("立即成交，余量自动撤销");
  await expect(page.getByLabel("参考股价", { exact: true })).toBeDisabled();
  for (const filled of [3, 0]) {
    const count = filled ? 9 : 8;
    await page.getByLabel("委托股数", { exact: true }).fill(String(count));
    const previewResponse = page.waitForResponse((r) =>
      r.url().endsWith("/orders/preview"),
    );
    await page
      .getByRole("button", { name: "预览交易明细", exact: true })
      .click();
    const preview = await (await previewResponse).json(),
      dialog = page.getByRole("dialog", { name: "确认交易委托" });
    await expect(dialog).toContainText("未成交余量自动撤销并解冻，不进入挂单");
    const confirmation = page.waitForResponse((r) =>
      r.url().endsWith("/orders/confirm"),
    );
    await dialog
      .getByRole("button", { name: "确认提交委托", exact: true })
      .click();
    const response = await confirmation;
    expect(response.ok(), await response.text()).toBe(true);
    const { order } = await response.json();
    expect(order).toMatchObject({
      kind: "market",
      status: "cancelled",
      quantity: count,
      filled,
      remaining: count - filled,
      reserved: 0,
    });
    await expect(dialog).not.toBeVisible();
    await expect(
      page.getByText(
        `委托 #${order.id} 已成交 ${filled} 股，剩余 ${count - filled} 股已撤销`,
        { exact: true },
      ),
    ).toBeVisible();
    const account = await (await page.request.get("/api/v2/account")).json();
    expect(account.account.reserved).toBe(0);
    expect(
      account.positions.find((p: { stockId: number }) => p.stockId === stock.id)
        ?.quantity,
    ).toBe(3);
    const repeated = await page.request.post("/api/v2/orders/confirm", {
      data: { previewId: preview.previewId },
    });
    expect(repeated.ok()).toBe(true);
    expect((await repeated.json()).order).toMatchObject({
      id: order.id,
      filled,
      status: "cancelled",
    });
    const after = await (await page.request.get("/api/v2/account")).json();
    expect(after.account.cash).toBe(account.account.cash);
    expect(after.orders.length).toBe(account.orders.length);
  }
  await page
    .locator(".navigation")
    .getByRole("tab", { name: "委托记录", exact: true })
    .click();
  await expect(page.getByText("暂无对应委托", { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "全部记录", exact: true }).click();
  const rows = page.getByRole("row").filter({ hasText: stock.name });
  await expect(rows).toHaveCount(2);
  for (const row of await rows.all()) {
    await expect(row).toContainText("已撤销");
    await expect(row).toContainText("立即成交 · 余量撤销");
    await expect(
      row.getByRole("button", { name: "撤单", exact: true }),
    ).toHaveCount(0);
  }
});

test("同毫秒历史修正通过 SSE 刷新图表，现价和报价时间保持不变", async ({
  page,
  request,
}) => {
  // 分时图默认显示一个上海交易日，凌晨运行时也保持两条样本位于同一天。
  const now = Date.now(),
    dayStart =
      Math.floor((now + 8 * 3600000) / 86400000) * 86400000 - 8 * 3600000,
    latestTime = Math.max(dayStart + 1, now - 2 * 60000),
    oldTime = Math.max(dayStart, latestTime - 90 * 60000),
    listed = await registerStock(request, `修正${randomUUID().slice(0, 8)}`, [
      { t: oldTime, p: 1000 },
      { t: latestTime, p: 1100 },
    ]);
  await page.goto(`/?stock=${listed.stock.id}`);
  await expect(page.locator('[role="img"] canvas')).toBeVisible();
  const before = (
    await (await request.get("/api/v2/public/market")).json()
  ).stocks.find((s: { id: number }) => s.id === listed.stock.id);
  const refreshed = page.waitForResponse(
    (r) =>
      r
        .url()
        .includes(`/api/v2/public/stocks/${listed.stock.id}/chart-detail?`) &&
      r.status() === 200,
  );
  const correction = await listed.report([{ t: oldTime, p: 1500 }]);
  expect(correction.ok(), await correction.text()).toBe(true);
  const chart = await (await refreshed).json();
  expect(chart.bars.some((bar: { high: number }) => bar.high === 150000)).toBe(
    true,
  );
  const after = (
    await (await request.get("/api/v2/public/market")).json()
  ).stocks.find((s: { id: number }) => s.id === listed.stock.id);
  expect(after.price).toBe(before.price);
  expect(after.observedAt).toBe(before.observedAt);
  expect(after.historyVersion).toBeGreaterThan(before.historyVersion);
});
