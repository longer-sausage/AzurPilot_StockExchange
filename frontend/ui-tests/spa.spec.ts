import { test, expect, type Page } from "@playwright/test";
test.beforeEach(async ({ request }) => {
  expect((await request.post("/__demo/reset")).status()).toBe(204);
});
async function login(page: Page, username = "预览玩家") {
  await page.getByRole("button", { name: "登录交易账户", exact: true }).click();
  const d = page.getByRole("dialog", { name: "登录交易账户" });
  await d.getByLabel("账号", { exact: true }).fill(username);
  await d.getByLabel("密码", { exact: true }).fill("Demo-only-Password!87");
  await d.getByRole("button", { name: "登录", exact: true }).click();
  await expect(d).not.toBeVisible();
}
async function admin(page: Page) {
  await page.goto("/");
  await page.getByRole("link", { name: "管理", exact: true }).click();
  await expect(page).toHaveURL(/:\d+\/$/);
  await page.getByLabel("管理员密码").fill("Demo-only-Admin!87");
  await page.getByRole("button", { name: "登录控制台" }).click();
  await expect(
    page.getByRole("heading", { name: "交易所管理", exact: true }),
  ).toBeVisible();
}
async function choose(page: Page, label: string, value: string) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page.getByRole("option", { name: value, exact: true }).click();
}
test("SPA 地址始终为根路径，只加载一个文档且后退刷新可用", async ({ page }) => {
  let documents = 0;
  page.on("request", (r) => {
    if (r.resourceType() === "document") documents++;
  });
  await page.goto("/");
  await page.getByRole("link", { name: "海风研究所", exact: true }).click();
  await expect(page).toHaveURL(/:\d+\/$/);
  await expect(page.locator('[role="img"] canvas')).toBeVisible();
  await page.getByRole("tab", { name: "交易指南" }).click();
  await expect(page).toHaveURL(/:\d+\/$/);
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "海风研究所", exact: true }),
  ).toBeVisible();
  await page.goForward();
  await expect(
    page.getByRole("heading", { name: "交易指南", exact: true }),
  ).toBeVisible();
  await page.goBack();
  expect(documents).toBe(1);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "海风研究所", exact: true }),
  ).toBeVisible();
  const guide = page.getByRole("tab", { name: "交易指南", exact: true });
  await guide.click();
  await guide.press("ArrowLeft");
  const seasons = page.getByRole("tab", { name: "月赛排名", exact: true });
  await expect(seasons).toBeFocused();
  await seasons.press("Enter");
  await expect(
    page.getByRole("heading", { name: "月赛排名", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/:\d+\/$/);
});
test("市场表格、筛选、自选与排序", async ({ page }) => {
  await page.goto("/");
  await login(page);
  const table = page.getByRole("table", { name: "证券行情" });
  await expect(table.locator("thead button")).toHaveCount(0);
  await page.getByLabel("搜索证券").fill("星港远航");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  const watch = page.waitForResponse((r) =>
    r.url().endsWith("/account/watchlist/2"),
  );
  await page.getByRole("button", { name: "添加自选 星港远航" }).click();
  expect((await watch).request().method()).toBe("PUT");
  await expect(
    page.getByRole("button", { name: "移除自选 星港远航" }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "筛选", exact: true }).click();
  await choose(page, "涨跌方向", "上涨");
  await page.getByRole("button", { name: "完成筛选" }).click();
  await expect(
    page.getByText("没有符合条件的证券", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "重置搜索与筛选" }).click();
  await choose(page, "证券排序", "价格从高到低");
  await expect(table.locator("tbody tr").first()).toContainText("星港远航");
});
test("登录错误、键盘关闭与 HttpOnly Cookie", async ({ page }) => {
  await page.goto("/");
  const trigger = page.getByRole("button", {
    name: "登录交易账户",
    exact: true,
  });
  await trigger.click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await trigger.click();
  const d = page.getByRole("dialog", { name: "登录交易账户" });
  await d.getByLabel("账号", { exact: true }).fill("预览玩家");
  await d.getByLabel("密码", { exact: true }).fill("wrong");
  await d.getByRole("button", { name: "登录", exact: true }).click();
  await expect(d.getByRole("alert")).toContainText("账号或密码不正确");
  await d.getByLabel("密码", { exact: true }).fill("Demo-only-Password!87");
  await d.getByRole("button", { name: "登录", exact: true }).click();
  await expect(d).not.toBeVisible();
  expect(
    (await page.context().cookies()).find((c) => c.name === "mmex_player")
      ?.httpOnly,
  ).toBe(true);
});
test("强制改密不能跳过", async ({ page }) => {
  await page.goto("/");
  await login(page, "首次登录演示");
  const d = page.getByRole("dialog", { name: "设置你的交易密码" });
  await expect(d).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(d).toBeVisible();
  await d.getByLabel("当前临时密码").fill("Demo-only-Password!87");
  await d.getByLabel("新密码", { exact: true }).fill("New-demo-Password!87");
  await d.getByLabel("再次输入新密码").fill("New-demo-Password!87");
  await d.getByRole("button", { name: "设置密码并继续" }).click();
  await expect(d).not.toBeVisible();
});
test("详情交易预览、确认与同预览重试", async ({ page }) => {
  await page.goto("/#/security?stock=1");
  await login(page);
  await page.getByLabel("委托股数", { exact: true }).fill("10");
  await page.getByLabel("委托限价", { exact: true }).fill("1286.40");
  const response = page.waitForResponse((r) =>
    r.url().endsWith("/orders/preview"),
  );
  await page.getByRole("button", { name: "预览交易明细" }).click();
  const preview = await response;
  expect(preview.request().postDataJSON()).toMatchObject({
    stock: "1",
    quantity: 10,
    price: 128640,
    side: "buy",
    kind: "limit",
    timeInForce: "GTC",
    leverage: 1,
  });
  const result = await preview.json(),
    d = page.getByRole("dialog", { name: "确认交易委托" });
  await expect(d).toBeVisible();
  await d.getByRole("button", { name: "确认提交委托" }).click();
  await expect(d).not.toBeVisible();
  await expect(page.getByText(/委托 #.*成交 10 股/)).toBeVisible();
  expect(
    (
      await page.request.post("/api/v2/orders/confirm", {
        data: { previewId: result.previewId },
      })
    ).ok(),
  ).toBe(true);
});
test("图表、分钟K、指标、全屏与主题", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/#/security?stock=1");
  await expect(page.locator('[role="img"] canvas')).toBeVisible();
  await page.getByRole("tab", { name: "M5", exact: true }).click();
  await page.getByRole("button", { name: "技术指标 (0)" }).click();
  await page.getByRole("checkbox", { name: "BOLL", exact: true }).check();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "全屏走势图" }).click();
  await expect(page.getByRole("dialog", { name: /全屏走势图/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  const initial = await page.locator("html").getAttribute("data-mode");
  await page
    .getByRole("button", {
      name: initial === "light" ? "切换暗色主题" : "切换亮色主题",
    })
    .click();
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute(
    "data-mode",
    initial === "light" ? "dark" : "light",
  );
  expect(errors).toEqual([]);
});
test("资产、费用、撤单和月赛", async ({ page }) => {
  await page.goto("/");
  await login(page);
  await page
    .locator(".navigation")
    .getByRole("tab", { name: "我的持仓" })
    .click();
  await expect(page.getByRole("tab", { name: "当前持仓" })).toBeVisible();
  await page.getByRole("tab", { name: "资产与交收" }).click();
  await expect(page.getByRole("heading", { name: "身份识别码" })).toBeVisible();
  await page.getByRole("tab", { name: "委托记录", exact: true }).click();
  const cancel = page.waitForResponse(
    (r) =>
      r.url().endsWith("/orders/1001") && r.request().method() === "DELETE",
  );
  await page.getByRole("button", { name: "撤单" }).click();
  expect((await cancel).ok()).toBe(true);
  await expect(page.getByRole("button", { name: "撤单" })).toHaveCount(0);
  await page.getByRole("tab", { name: "月赛排名" }).click();
  await expect(page.getByRole("table")).toContainText("星港观察员");
});
test("六步教程、37项词典和实时交易规则", async ({ page }) => {
  await page.goto("/#/guide");
  await expect(
    page.getByRole("heading", { name: "注册交易账户" }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "名词词典 37" }).click();
  await page.getByLabel("搜索交易名词").fill("MACD");
  const term = page.getByRole("button", { name: "MACD / DIF / DEA" });
  await term.click();
  await expect(term).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("tab", { name: "当前交易规则" }).click();
  await expect(
    page.getByRole("heading", { name: "总资产阶梯税率" }),
  ).toBeVisible();
});
test("管理设置分组编辑并保持 ppm", async ({ page }) => {
  await admin(page);
  await page.getByRole("tab", { name: "费用与税率" }).click();
  await page.getByLabel("最大溢价（%）", { exact: true }).fill("4");
  const saved = page.waitForResponse(
    (r) =>
      r.url().endsWith("/admin/settings") && r.request().method() === "PUT",
  );
  await page.getByRole("button", { name: "保存交易规则" }).click();
  expect((await saved).request().postDataJSON().premiumPPM).toBe(40000);
});
test("账户改名、并发字段和密码二次确认", async ({ page }) => {
  await admin(page);
  await page.getByRole("tab", { name: /交易账户/ }).click();
  await page.getByRole("button", { name: "管理账户" }).click();
  const d = page.getByRole("dialog", { name: "账户管理", exact: true });
  await d.getByLabel("用户名", { exact: true }).fill("改名演示");
  await d.getByRole("button", { name: "重置密码", exact: true }).click();
  const reset = page.getByRole("dialog", { name: "确认重置账户密码" });
  await reset.getByRole("button", { name: "确认重置密码" }).click();
  await expect(
    d.getByText("Demo-reset-Password!87", { exact: true }),
  ).toBeVisible();
  const saved = page.waitForResponse(
    (r) =>
      /\/admin\/accounts\/\d+$/.test(r.url()) && r.request().method() === "PUT",
  );
  await d.getByRole("button", { name: "保存账户修改" }).click();
  expect((await saved).request().postDataJSON()).toMatchObject({
    username: "改名演示",
    cashAdjustment: 0,
  });
  await expect(d).not.toBeVisible();
  await expect(page.getByRole("table")).toContainText("改名演示");
});
test("证券停牌与删除输入确认", async ({ page }) => {
  await admin(page);
  await page.getByRole("tab", { name: /证券管理/ }).click();
  const row = page.getByRole("row").filter({ hasText: "星港远航" });
  await row.getByRole("button", { name: "停牌", exact: true }).click();
  await expect(row.getByRole("button", { name: "恢复交易" })).toBeVisible();
  await row.getByRole("button", { name: "删除", exact: true }).click();
  const d = page.getByRole("dialog", { name: "删除证券并结清持仓" });
  await expect(
    d.getByRole("button", { name: "确认删除并结清" }),
  ).toBeDisabled();
  await d.getByLabel("确认股票代码").fill("MM0002");
  await d.getByRole("button", { name: "确认删除并结清" }).click();
  await expect(d).not.toBeVisible();
  await expect(row).toHaveCount(0);
});
test("各页面桌面和手机无文档横向溢出", async ({ page }) => {
  await page.goto("/");
  await login(page);
  await admin(page);
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const [name, legacyView, title] of [
      ["market", "/", "证券市场"],
      ["security", "/security?stock=1", "海风研究所"],
      ["portfolio", "/portfolio", "我的持仓"],
      ["orders", "/orders", "委托记录"],
      ["seasons", "/seasons", "月赛排名"],
      ["guide", "/guide", "交易指南"],
      ["admin", "/console", "交易所管理"],
    ]) {
      await page.goto("/#" + legacyView);
      await expect(
        page.getByRole("heading", { name: title, exact: true }),
      ).toBeVisible();
      await expect(page).toHaveURL(/:\d+\/$/);
      if (name !== "admin") {
        await expect
          .poll(() =>
            page
              .getByRole("tablist")
              .first()
              .evaluate((list) => {
                const selected = list.querySelector('[aria-selected="true"]');
                const indicator = list.lastElementChild;
                if (!selected || !indicator) return false;
                const a = selected.getBoundingClientRect(),
                  b = indicator.getBoundingClientRect();
                return (
                  Math.abs(a.x - b.x) < 1 && Math.abs(a.width - b.width) < 1
                );
              }),
          )
          .toBe(true);
      }
      if (name === "market")
        await expect(
          page.getByRole("table", { name: "证券行情" }).locator("tbody tr"),
        ).toHaveCount(8);
      if (name === "security")
        await expect(page.locator('[role="img"] canvas')).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: `test-results/spa-${name}-${width}.png`,
        fullPage: true,
        animations: "disabled",
      });
    }
  }
});
