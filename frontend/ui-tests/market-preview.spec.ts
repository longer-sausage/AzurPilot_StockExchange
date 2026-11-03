import { test, expect } from "@playwright/test";

test("点击行情行普通单元格进入详情，自选仍独立操作", async ({ page }) => {
  await page.goto("/");
  const row = page.getByRole("row").filter({
    has: page.getByRole("link", { name: "海风研究所", exact: true }),
  });
  await row.getByText("可交易", { exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "海风研究所", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/:\d+\/$/);
  await page.goBack();
  await row.getByRole("button", { name: "添加自选 海风研究所" }).click();
  await expect(
    page.getByRole("dialog", { name: "登录交易账户" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("main")).toHaveAttribute(
    "aria-label",
    "证券市场",
  );
  await row
    .getByRole("link", { name: "海风研究所", exact: true })
    .press("Enter");
  await expect(
    page.getByRole("heading", { name: "海风研究所", exact: true }),
  ).toBeVisible();
});

test("走势使用历史报价，端点相同但中段变化也会刷新曲线并复用缓存", async ({
  page,
}) => {
  let revision = 0,
    requests = 0,
    conditional = "";
  await page.route("**/api/v2/public/market", async (route) => {
    const response = await route.fetch(),
      data = await response.json();
    data.stocks = data.stocks.map(
      (s: { id: number; historyVersion: number }) =>
        s.id === 1 ? { ...s, historyVersion: 1 + revision } : s,
    );
    await route.fulfill({ response, json: data });
  });
  await page.route("**/api/v2/public/stocks/1/chart?*", async (route) => {
    requests++;
    conditional = route.request().headers()["if-none-match"] ?? "";
    const url = new URL(route.request().url()),
      from = Number(url.searchParams.get("from"));
    expect(url.searchParams.get("interval")).toBe("1");
    expect(Number(url.searchParams.get("to")) - from).toBe(86400000 - 1);
    const values = revision
      ? [100, 160, 80, 120, 140]
      : [100, 120, 80, 160, 140];
    await route.fulfill({
      json: {
        stockId: 1,
        interval: 1,
        points: values.map((c, i) => ({
          t: from + 34200000 + i * 60000,
          o: c * 100,
          h: c * 100,
          l: c * 100,
          c: c * 100,
          n: 1,
        })),
      },
      headers: { ETag: `"wave-${revision}"` },
    });
  });
  await page.goto("/");
  const figure = page.getByRole("img", {
      name: /海风研究所.*分钟报价走势，共 5 个采样/,
    }),
    canvas = figure.locator("canvas");
  await expect(canvas).toBeVisible();
  const first = await canvas.screenshot();
  revision = 1;
  await page.evaluate(() => window.dispatchEvent(new Event("online")));
  await expect.poll(() => requests).toBe(2);
  await expect
    .poll(async () => first.equals(await canvas.screenshot()))
    .toBe(false);
  expect(conditional).toBe('"wave-0"');
  await page.getByRole("tab", { name: "交易指南", exact: true }).click();
  await page.getByRole("tab", { name: "证券市场", exact: true }).click();
  await expect(canvas).toBeVisible();
  expect(requests).toBe(2);
  await page.screenshot({
    path: "test-results/market-real-wave-1280.png",
    fullPage: true,
  });
});

test("真实上报间隔为几分钟时仍显示折线，不变成一串圆点", async ({ page }) => {
  await page.route("**/api/v2/public/stocks/1/chart?*", async (route) => {
    const from = Number(
      new URL(route.request().url()).searchParams.get("from"),
    );
    await route.fulfill({
      json: {
        stockId: 1,
        interval: 1,
        points: [
          { t: from + 34200000, o: 10000, h: 10000, l: 10000, c: 10000, n: 1 },
          { t: from + 34380000, o: 12000, h: 12000, l: 12000, c: 12000, n: 1 },
        ],
      },
    });
  });
  await page.goto("/");
  const canvas = page
    .getByRole("img", {
      name: /海风研究所.*共 2 个采样/,
    })
    .locator("canvas");
  await expect(canvas).toBeVisible();
  await expect
    .poll(() =>
      canvas.evaluate((el: HTMLCanvasElement) => {
        const { data } = el
          .getContext("2d")!
          .getImageData(0, 0, el.width, el.height);
        let colored = 0;
        for (let y = 0; y < el.height; y++)
          for (let x = Math.floor(el.width / 3); x < (el.width * 2) / 3; x++) {
            const i = (y * el.width + x) * 4;
            if (
              data[i + 3] > 0 &&
              Math.max(data[i], data[i + 1], data[i + 2]) -
                Math.min(data[i], data[i + 1], data[i + 2]) >
                50
            )
              colored++;
          }
        return colored;
      }),
    )
    .toBeGreaterThan(30);
});

test("无历史、单点、长缺口和请求失败不补造走势，重试不会进入详情", async ({
  page,
}) => {
  let failed = true;
  await page.route("**/api/v2/public/stocks/*/chart?*", async (route) => {
    const url = new URL(route.request().url()),
      id = Number(url.pathname.split("/").at(-2));
    if (id > 4) return route.continue();
    if (id === 3 && failed) {
      failed = false;
      return route.fulfill({
        status: 503,
        json: {
          error: { code: "UNAVAILABLE", message: "隔离测试：行情暂时不可用" },
        },
      });
    }
    const t = Number(url.searchParams.get("from")) + 34200000;
    await route.fulfill({
      json: {
        stockId: id,
        interval: 1,
        points:
          id === 1
            ? []
            : id === 4
              ? [
                  { t, o: 10000, h: 10000, l: 10000, c: 10000, n: 1 },
                  {
                    t: t + 20 * 60000,
                    o: 20000,
                    h: 20000,
                    l: 20000,
                    c: 20000,
                    n: 1,
                  },
                ]
              : [{ t, o: 10000, h: 10000, l: 10000, c: 10000, n: 1 }],
      },
    });
  });
  await page.goto("/");
  const row = (name: string) =>
    page
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name, exact: true }) });
  await expect(
    row("海风研究所").getByText("暂无行情", { exact: true }),
  ).toBeVisible();
  await expect(row("海风研究所").locator("canvas")).toHaveCount(0);
  await expect(
    row("星港远航")
      .getByRole("img", { name: /共 1 个采样/ })
      .locator("canvas"),
  ).toBeVisible();
  await row("明石工坊").getByRole("button", { name: "重试走势" }).click();
  await expect(
    row("明石工坊")
      .getByRole("img", { name: /共 1 个采样/ })
      .locator("canvas"),
  ).toBeVisible();
  await expect(page.getByRole("main")).toHaveAttribute(
    "aria-label",
    "证券市场",
  );
  const sparse = row("极光舰队")
    .getByRole("img", { name: /共 2 个采样/ })
    .locator("canvas");
  await expect(sparse).toBeVisible();
  const samples = () =>
    sparse.evaluate((el: HTMLCanvasElement) => {
      const { data } = el
        .getContext("2d")!
        .getImageData(0, 0, el.width, el.height);
      let center = 0,
        edges = 0;
      for (let y = 0; y < el.height; y++)
        for (let x = 0; x < el.width; x++) {
          const i = (y * el.width + x) * 4;
          if (
            data[i + 3] > 0 &&
            Math.max(data[i], data[i + 1], data[i + 2]) -
              Math.min(data[i], data[i + 1], data[i + 2]) >
              50
          ) {
            if (x > el.width / 3 && x < (el.width * 2) / 3) center++;
            else edges++;
          }
        }
      return { center, edges };
    });
  await expect.poll(async () => (await samples()).edges).toBeGreaterThan(0);
  expect((await samples()).center).toBe(0);
});

test("手机初始视口可见最新价与波动，滚动按需加载每支证券", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 1000 });
  await page.goto("/");
  const rows = page
    .getByRole("table", { name: "证券行情" })
    .locator("tbody tr");
  await expect(rows).toHaveCount(8);
  const first = rows.first().getByRole("img", { name: /分钟报价走势/ });
  await expect(first.locator("canvas")).toBeVisible();
  const box = await first.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(375);
  for (const row of await rows.all()) {
    await row.scrollIntoViewIfNeeded();
    await expect(
      row.getByRole("img", { name: /分钟报价走势/ }).locator("canvas"),
    ).toBeVisible();
  }
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: "test-results/market-real-wave-375.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1280, height: 1000 });
  await page
    .getByRole("table", { name: "证券行情" })
    .screenshot({ path: "test-results/market-wave-table.png" });
});
