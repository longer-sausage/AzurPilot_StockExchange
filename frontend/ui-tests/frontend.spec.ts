import { test, expect, type Page } from "@playwright/test";

test("证券筛选、排序和持久自选使用 Kumo Select 及原请求", async ({ page }) => {
  await login(page);
  await page.getByLabel("搜索股票").fill("星港远航");
  const watch = page.waitForResponse((r) =>
    r.url().includes("/account/watchlist/2"),
  );
  await page.getByRole("button", { name: "添加自选 星港远航" }).click();
  expect((await watch).request().method()).toBe("PUT");
  await expect(
    page.getByRole("button", { name: "移除自选 星港远航" }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "展开筛选" }).click();
  await choose(page, "涨跌方向", "上涨");
  await expect(page.locator(".stock-row")).toHaveCount(0);
  await page.getByRole("button", { name: "重置筛选与排序" }).click();
  await choose(page, "证券排序", "价格 · 降序");
  await expect(page.getByRole("combobox", { name: "证券排序" })).toContainText(
    "价格 · 降序",
  );
});

test("账户持仓、损益、委托撤销和排行榜可用", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "持仓与委托", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "当前持仓", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "账户明细", exact: true }).click();
  await expect(page.getByText("身份识别码", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /^我的委托/ }).click();
  const cancel = page.waitForResponse(
    (r) =>
      r.url().endsWith("/orders/1001") && r.request().method() === "DELETE",
  );
  await page.getByRole("button", { name: "撤单", exact: true }).click();
  expect((await cancel).ok()).toBe(true);
  await expect(
    page.getByRole("button", { name: "撤单", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "月赛排行榜", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "月赛与排行榜" }),
  ).toBeVisible();
});

test("规则保留六步教程和可搜索、可展开的名词词典", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "交易规则", exact: true }).click();
  await page.getByRole("button", { name: /名词词典/ }).click();
  await page.getByLabel("搜索交易名词").fill("MACD");
  const term = page.getByRole("button", {
    name: "MACD / DIF / DEA",
    exact: true,
  });
  await expect(term).toBeVisible();
  await term.click();
  await expect(term).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("button", { name: "新手六步" }).click();
  await page.getByRole("button", { name: "下一步", exact: true }).click();
  await expect(page.getByText("STEP 02 / 06", { exact: true })).toBeVisible();
});

test.beforeEach(async ({ request }) => {
  expect((await request.post("/__demo/reset")).status()).toBe(204);
});

async function login(page: Page, username = "预览玩家") {
  await page.goto("/");
  await page.getByRole("button", { name: "登录交易账户", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "登录交易账户" });
  await dialog.getByLabel("账号", { exact: true }).fill(username);
  await dialog
    .getByLabel("密码", { exact: true })
    .fill("Demo-only-Password!87");
  await dialog.getByRole("button", { name: "登录", exact: true }).click();
  await expect(dialog).not.toBeVisible();
}
async function admin(page: Page) {
  await page.goto("/console");
  await page.getByLabel("管理员密码").fill("Demo-only-Admin!87");
  await page.getByRole("button", { name: "登录控制台" }).click();
  await expect(page.getByRole("heading", { name: "交易所管理" })).toBeVisible();
}
async function choose(page: Page, name: string, value: string) {
  await page.getByRole("combobox", { name, exact: true }).click();
  await page.getByRole("option", { name: value, exact: true }).click();
}
test("桌面与手机展示行情、真实图表 Canvas、Kumo 控件且无横向溢出", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(page.getByRole("heading", { name: /海风研究所/ })).toBeVisible();
  await expect(page.locator(".financial-canvas canvas")).toBeVisible();
  for (const width of [1600, 1280, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    expect(
      await page
        .locator(".financial-canvas")
        .evaluate((el) => el.getBoundingClientRect().height),
    ).toBeGreaterThan(300);
    await page.screenshot({
      path: `test-results/kumo-market-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "交易规则", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "总资产阶梯税率" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("Kumo 登录弹窗键盘退出、错误提示与 HttpOnly 会话", async ({ page }) => {
  await page.goto("/");
  const trigger = page.getByRole("button", {
    name: "登录交易账户",
    exact: true,
  });
  await trigger.click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "登录交易账户" });
  await dialog.getByLabel("账号", { exact: true }).fill("预览玩家");
  await dialog.getByLabel("密码", { exact: true }).fill("wrong-password");
  await dialog.getByRole("button", { name: "登录", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("账号或密码不正确");
  await dialog
    .getByLabel("密码", { exact: true })
    .fill("Demo-only-Password!87");
  await dialog.getByRole("button", { name: "登录", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(
    (await page.context().cookies()).find((c) => c.name === "mmex_player")
      ?.httpOnly,
  ).toBe(true);
});
test("首次登录强制改密，Kumo 表单提交完成后解锁账户", async ({ page }) => {
  await login(page, "首次登录演示");
  const dialog = page.getByRole("dialog", { name: "请先设置新密码" });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("当前临时密码").fill("Demo-only-Password!87");
  await dialog
    .getByLabel("新密码", { exact: true })
    .fill("New-demo-Password!87");
  await dialog.getByLabel("再次输入新密码").fill("New-demo-Password!87");
  await dialog.getByRole("button", { name: "设置新密码并继续" }).click();
  await expect(dialog).not.toBeVisible();
});
test("选择委托类型和杠杆，预览与二次确认发送原 v2 契约", async ({ page }) => {
  await login(page);
  await page.getByLabel("委托股数").fill("10");
  await page.getByLabel("委托限价").fill("1286.40");
  await choose(page, "买入方式", "现金 / 1 倍");
  const response = page.waitForResponse((r) =>
    r.url().endsWith("/orders/preview"),
  );
  await page.getByRole("button", { name: "预览交易明细" }).click();
  const preview = await response;
  expect(preview.request().postDataJSON()).toMatchObject({
    side: "buy",
    quantity: 10,
    price: 128640,
    kind: "limit",
    timeInForce: "GTC",
    leverage: 1,
  });
  const dialog = page.getByRole("dialog", { name: "确认交易委托" });
  await expect(dialog).toBeVisible();
  const confirmation = page.waitForResponse((r) =>
    r.url().endsWith("/orders/confirm"),
  );
  await dialog.getByRole("button", { name: "确认提交委托" }).click();
  expect((await confirmation).request().postDataJSON()).toHaveProperty(
    "previewId",
  );
  await expect(dialog).not.toBeVisible();
  await expect(page.getByText(/委托 #.*成交 10 股/)).toBeVisible();
});
test("图表切换周期、全屏与退出，亮暗主题刷新后保留", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator(".financial-canvas canvas")).toBeVisible();
  await page.getByRole("button", { name: "M5", exact: true }).click();
  await page.getByRole("button", { name: "全屏走势图" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  const current = await page.locator("html").getAttribute("data-mode");
  await page
    .getByRole("button", {
      name: current === "dark" ? "切换亮色主题" : "切换暗色主题",
    })
    .click();
  const next = current === "dark" ? "light" : "dark";
  await expect(page.locator("html")).toHaveAttribute("data-mode", next);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-mode", next);
});
test("管理入口兼容 Go HTML fallback，保存交易规则沿用 PUT", async ({
  page,
}) => {
  await admin(page);
  const response = page.waitForResponse(
    (r) =>
      r.url().endsWith("/admin/settings") && r.request().method() === "PUT",
  );
  await page.getByLabel("最大溢价（%）").fill("4");
  await page.getByRole("button", { name: "保存交易规则" }).click();
  expect((await response).request().postDataJSON().premiumPPM).toBe(40000);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/kumo-console-1280.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 375, height: 1000 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: "test-results/kumo-console-375.png",
    fullPage: true,
  });
});
test("管理账户详情、改名及二次确认密码重置", async ({ page }) => {
  await admin(page);
  await page.getByRole("button", { name: "交易账户", exact: true }).click();
  await page.getByRole("button", { name: "管理 / 详情" }).first().click();
  const dialog = page.getByRole("dialog", { name: /管理账户/ });
  await dialog.getByLabel("用户名", { exact: true }).fill("改名演示");
  await dialog.getByRole("button", { name: "重置该账户密码" }).click();
  const reset = page.getByRole("dialog", { name: "确认管理员重置密码" });
  await expect(reset).toBeVisible();
  await reset.getByRole("button", { name: "确认重置密码" }).click();
  await expect(dialog.getByText("Demo-reset-Password!87")).toBeVisible();
  await dialog.getByRole("button", { name: "保存账户修改" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: "改名演示" }),
  ).toBeVisible();
});
test("股票停牌和删除需要独立管理操作与输入确认", async ({ page }) => {
  await admin(page);
  await page.getByRole("button", { name: "股票管理", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: "星港远航" });
  await row.getByRole("button", { name: "停牌", exact: true }).click();
  await expect(
    row.getByRole("button", { name: "恢复", exact: true }),
  ).toBeVisible();
  await row.getByRole("button", { name: "删除股票", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "确认删除股票 星港远航" });
  await expect(
    dialog.getByRole("button", { name: "确认删除并结清" }),
  ).toBeDisabled();
  await dialog.getByLabel("确认删除股票代码").fill("MM0002");
  await dialog.getByRole("button", { name: "确认删除并结清" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(row).toHaveCount(0);
});
