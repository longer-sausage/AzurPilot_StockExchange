import { test, expect, type Locator } from "@playwright/test";

// 验证图上实际绘制的涨跌色，防止图例、柱体和分时线退回默认分类色。
async function renderedColors(canvas: Locator, top: number, bottom: number) {
  return canvas.evaluate(
    (el: HTMLCanvasElement, range) => {
      const { data } = el
        .getContext("2d")!
        .getImageData(0, 0, el.width, el.height);
      let red = 0,
        green = 0;
      for (
        let y = Math.ceil(el.height * range.top);
        y < el.height * range.bottom;
        y++
      ) {
        for (let x = Math.ceil(el.width * 0.12); x < el.width * 0.95; x++) {
          const i = (y * el.width + x) * 4;
          if (!data[i + 3]) continue;
          const r = data[i],
            g = data[i + 1],
            b = data[i + 2];
          if (r > 200 && g > 40 && g < 140 && b < 140) red++;
          if (r < 80 && g > 120 && g < 200 && b < 110) green++;
        }
      }
      return { red, green };
    },
    { top, bottom },
  );
}

test("分时、K线、成交量及 MACD 在明暗主题下按真实方向显示红绿", async ({
  page,
}) => {
  await page.route(
    "**/api/v2/public/stocks/1/chart-detail?*",
    async (route) => {
      const response = await route.fetch(),
        detail = await response.json();
      const start = detail.bars[0].time;
      detail.bars = Array.from({ length: 100 }, (_, i) => {
        const close = 10000 + Math.round(Math.sin(i / 5) * 1000);
        const open = close + (i % 2 ? 100 : -100);
        return {
          time: start + i * 60000,
          open,
          close,
          high: Math.max(open, close) + 50,
          low: Math.min(open, close) - 50,
          samples: 1,
          volume: 100,
          turnover: close * 100,
          buyVolume: 50,
          sellVolume: 50,
        };
      });
      detail.summary.open = 10000;
      detail.summary.high = Math.max(
        ...detail.bars.map((b: { high: number }) => b.high),
      );
      detail.summary.low = Math.min(
        ...detail.bars.map((b: { low: number }) => b.low),
      );
      detail.displayFrom = start;
      await route.fulfill({ response, json: detail });
    },
  );
  await page.goto("/#/security?stock=1");
  const canvas = page.locator('[role="img"] canvas');
  await expect(canvas).toBeVisible();
  for (const mode of ["light", "dark"]) {
    const actual = await page.locator("html").getAttribute("data-mode");
    if (actual !== mode)
      await page
        .getByRole("button", {
          name: mode === "dark" ? "切换暗色主题" : "切换亮色主题",
        })
        .click();
    await page.getByRole("tab", { name: "分时", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "技术指标 (0)" }),
    ).toBeVisible();
    const closing = 10000 + Math.round(Math.sin(99 / 5) * 1000);
    await expect(
      page
        .getByRole("definition")
        .filter({ hasText: (closing / 100).toFixed(2) }),
    ).toBeVisible();
    for (const [top, bottom] of [
      [0.1, 0.5],
      [0.64, 0.74],
      [0.82, 0.9],
    ]) {
      await expect
        .poll(async () => (await renderedColors(canvas, top, bottom)).red)
        .toBeGreaterThan(10);
      await expect
        .poll(async () => (await renderedColors(canvas, top, bottom)).green)
        .toBeGreaterThan(10);
    }
    expect((await renderedColors(canvas, 0.1, 0.26)).green).toBe(0);
    expect((await renderedColors(canvas, 0.38, 0.5)).red).toBe(0);
    await page
      .locator(".chart-content")
      .screenshot({ path: `test-results/finance-time-${mode}.png` });
    await page.getByRole("tab", { name: "M5", exact: true }).click();
    await expect
      .poll(async () => (await renderedColors(canvas, 0.1, 0.5)).red)
      .toBeGreaterThan(50);
    await expect
      .poll(async () => (await renderedColors(canvas, 0.1, 0.5)).green)
      .toBeGreaterThan(50);
    await page
      .locator(".chart-content")
      .screenshot({ path: `test-results/finance-candles-${mode}.png` });
  }
  await page.goto("/#/security?stock=3");
  await page.getByRole("tab", { name: "分时", exact: true }).click();
  await expect(page.locator('[aria-label="图表价格摘要"]')).toContainText(
    "865.00",
  );
  await expect(canvas).toBeVisible();
  await page
    .locator(".chart-content")
    .screenshot({ path: "test-results/price-clarity-865.png" });
});
