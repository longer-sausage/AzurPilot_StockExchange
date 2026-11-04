import { test, expect, type Page } from "@playwright/test";
import type { Stock } from "../src/core/contracts";
import {
  calculateErosionIndex,
  formatIndexNumber,
} from "../src/core/erosionIndex";

const now = Date.parse("2026-11-05T12:00:00+08:00"),
  today = "2026-11-05";
function stock(overrides: Partial<Stock> = {}): Stock {
  return {
    id: 1,
    symbol: "MM0001",
    name: "指数样本甲",
    price: 12000,
    open: 10000,
    openDay: today,
    observedAt: now,
    uploadedAt: now,
    historyVersion: 1,
    tradeVersion: 1,
    bound: true,
    disabled: false,
    issuedShares: 100,
    outstanding: 0,
    hourlyVolume: 1000,
    normalUsed: 0,
    borrowShares: 10000,
    borrowed: 0,
    borrowHourlyVolume: 1000,
    borrowUsed: 0,
    ...overrides,
  };
}
const basket = () => [
  stock(),
  stock({
    id: 2,
    symbol: "MM0002",
    name: "指数样本乙",
    price: 9000,
    issuedShares: 300,
  }),
];

test("全市场按发行股数加权，不平均涨幅，也不按持仓或借券量加权", () => {
  const result = calculateErosionIndex(basket(), now, 86400)!;
  expect(result).toEqual({
    pointsHundredths: 97500n,
    changeHundredths: -2500n,
    changeBasisPoints: -250n,
    samples: 2,
  });
  expect(formatIndexNumber(result.pointsHundredths)).toBe("975.00");
  const stocks = basket().map((s) => ({
    ...s,
    outstanding: 99,
    borrowed: 100,
    normalUsed: 1000,
    bound: false,
    disabled: true,
  }));
  expect(calculateErosionIndex(stocks, now, 86400)).toEqual(result);
});

test("零价、缺基价、过期、退市、删除和非安全整数不污染分子与分母", () => {
  const stocks = [
    stock(),
    stock({ price: 0 }),
    stock({ open: 0 }),
    stock({ openDay: "2026-11-04" }),
    stock({ observedAt: now - 61000 }),
    stock({ delisted: true }),
    stock({ deletedAt: now }),
    stock({ price: NaN }),
    stock({ issuedShares: Number.MAX_SAFE_INTEGER + 1 }),
    stock({ issuedShares: 0 }),
    stock({ observedAt: 0 }),
    stock({ price: 1.5 }),
  ];
  expect(calculateErosionIndex(stocks, now, 60)).toEqual({
    pointsHundredths: 120000n,
    changeHundredths: 20000n,
    changeBasisPoints: 2000n,
    samples: 1,
  });
  expect(calculateErosionIndex(stocks.slice(1), now, 60)).toBeUndefined();
  expect(calculateErosionIndex([], now, 60)).toBeUndefined();
});

test("市值中间乘积超过安全整数仍正确，百分比正负边界对称舍入", () => {
  const large = [
    stock({ open: 80000000, price: 96000000, issuedShares: 175000000 }),
    stock({ open: 80000000, price: 72000000, issuedShares: 525000000 }),
  ];
  expect(calculateErosionIndex(large, now, 86400)?.pointsHundredths).toBe(
    97500n,
  );
  expect(
    calculateErosionIndex([stock({ open: 20000, price: 20001 })], now, 86400)
      ?.changeBasisPoints,
  ).toBe(1n);
  expect(
    calculateErosionIndex([stock({ open: 20000, price: 19999 })], now, 86400)
      ?.changeBasisPoints,
  ).toBe(-1n);
  expect(
    calculateErosionIndex([stock({ open: 100000000, price: 1 })], now, 86400)
      ?.pointsHundredths,
  ).toBe(0n);
  expect(formatIndexNumber(100000n)).toBe("1,000.00");
  expect(formatIndexNumber(-250n, true)).toBe("-2.50");
  expect(formatIndexNumber(0n, true)).toBe("0.00");
});

test("TTL 边界与上海跨日生效，不沿用昨日开盘基准", () => {
  expect(
    calculateErosionIndex([stock({ observedAt: now - 60000 })], now, 60),
  ).toBeDefined();
  expect(
    calculateErosionIndex([stock({ observedAt: now - 60001 })], now, 60),
  ).toBeUndefined();
  expect(
    calculateErosionIndex(
      [stock({ observedAt: Date.parse("2026-11-05T23:59:59+08:00") })],
      Date.parse("2026-11-06T00:00:00+08:00"),
      86400,
    ),
  ).toBeUndefined();
  expect(
    calculateErosionIndex([stock({ price: 10000 })], now, 60)?.changeHundredths,
  ).toBe(0n);
});

test.beforeEach(async ({ request }) => {
  expect((await request.post("/__demo/reset")).status()).toBe(204);
});
async function market(page: Page) {
  const state = { stocks: basket(), ttl: 86400 };
  await page.clock.setFixedTime(now);
  await page.route("**/api/v2/public/market", async (route) => {
    const response = await route.fetch(),
      data = await response.json();
    await route.fulfill({
      response,
      json: { ...data, stocks: state.stocks, open: true },
    });
  });
  await page.route("**/api/v2/public/rules", async (route) => {
    const response = await route.fetch(),
      data = await response.json();
    await route.fulfill({
      response,
      json: { ...data, quoteTTLSeconds: state.ttl },
    });
  });
  return state;
}

test("首页显示点位与涨跌，筛选不改变指数，行情刷新重新计算", async ({
  page,
}) => {
  const state = await market(page),
    requests: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v2/"))
      requests.push(new URL(request.url()).pathname);
  });
  await page.goto("/");
  const index = page.getByRole("group", { name: "侵蚀指数", exact: true });
  await expect(index.locator("dt")).toHaveText("侵蚀指数");
  await expect(index.locator("dd").first()).toHaveText("975.00");
  await expect(index.locator("dd").last()).toHaveText("-25.00 (-2.50%)");
  await expect(index.locator("dd").first()).toHaveClass(/loss/);
  await expect(index).toHaveText("侵蚀指数975.00-25.00 (-2.50%)");
  await expect(index.getByRole("button")).toHaveCount(0);
  await page.getByLabel("搜索证券").fill("指数样本甲");
  await expect(
    page.getByRole("table", { name: "证券行情" }).locator("tbody tr"),
  ).toHaveCount(1);
  await expect(index.locator("dd").first()).toHaveText("975.00");
  state.stocks[1].price = 11000;
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(index.locator("dd").first()).toHaveText("1,125.00");
  await expect(index.locator("dd").last()).toHaveText("+125.00 (+12.50%)");
  await expect(index.locator("dd").first()).toHaveClass(/gain/);
  expect(
    requests.every((path) =>
      /^\/api\/v2\/(public\/(market|rules|seasons|events|stocks\/\d+\/chart)|auth\/me)$/.test(
        path,
      ),
    ),
  ).toBe(true);
  await page.reload();
  await expect(index.locator("dd").first()).toHaveText("1,125.00");
});

test("报价过期和上海跨日自动清空指数，无数据只显示横线", async ({ page }) => {
  const state = await market(page);
  state.ttl = 60;
  state.stocks.forEach((s) => {
    s.observedAt = now - 59000;
  });
  await page.goto("/");
  const index = page.getByRole("group", { name: "侵蚀指数", exact: true });
  await expect(index.locator("dd").first()).toHaveText("975.00");
  await page.clock.setFixedTime(now + 2000);
  await expect(index.locator("dd")).toHaveText(["—", "—"]);
  state.ttl = 86400;
  state.stocks.forEach((s) => {
    s.observedAt = now;
  });
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(index.locator("dd").first()).toHaveText("975.00");
  await page.clock.setFixedTime(Date.parse("2026-11-06T00:00:00+08:00"));
  await expect(index.locator("dd")).toHaveText(["—", "—"]);
  await expect(index).toHaveText("侵蚀指数——");
});

test("桌面与手机明暗主题下只呈现指数名称、点位和涨跌", async ({ page }) => {
  await market(page);
  await page.goto("/");
  const index = page.getByRole("group", { name: "侵蚀指数", exact: true });
  await expect(index.locator("dd").first()).toHaveText("975.00");
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const theme of ["light", "dark"]) {
      if ((await page.locator("html").getAttribute("data-mode")) !== theme)
        await page
          .getByRole("button", {
            name: theme === "dark" ? "切换暗色主题" : "切换亮色主题",
            exact: true,
          })
          .click();
      await expect(index).toHaveText("侵蚀指数975.00-25.00 (-2.50%)");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page
        .locator(".overview")
        .screenshot({
          path: `test-results/erosion-index-${width}-${theme}.png`,
        });
    }
  }
});
