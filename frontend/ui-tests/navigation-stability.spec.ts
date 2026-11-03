import { test, expect } from "@playwright/test";
// Playwright 的默认 headless 参数隐藏滚动条，会漏掉真实桌面的宽度跳变。
test.use({ launchOptions: { ignoreDefaultArgs: ["--hide-scrollbars"] } });

for (const width of [1280, 1440, 375]) {
  test(`导航切换时布局保持稳定 ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/");
    await expect(page.getByRole("table", { name: "证券行情" })).toBeVisible();
    for (const name of [
      "我的持仓",
      "委托记录",
      "月赛排名",
      "交易指南",
      "证券市场",
    ]) {
      const frames = page.evaluate(
        () =>
          new Promise<
            {
              x: number;
              y: number;
              width: number;
              gutter: number;
              scroll: number;
            }[]
          >((resolve) => {
            const samples: {
              x: number;
              y: number;
              width: number;
              gutter: number;
              scroll: number;
            }[] = [];
            const read = () => {
              const box = document
                .querySelector(".navigation")!
                .getBoundingClientRect();
              samples.push({
                x: box.x,
                y: box.y,
                width: box.width,
                gutter: innerWidth - document.documentElement.clientWidth,
                scroll: scrollY,
              });
              if (samples.length < 30) requestAnimationFrame(read);
              else resolve(samples);
            };
            requestAnimationFrame(read);
          }),
      );
      await page
        .locator(".navigation")
        .getByRole("tab", { name, exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name, exact: true }),
      ).toBeVisible();
      const samples = await frames;
      for (const key of ["x", "y", "width", "scroll"] as const) {
        const values = samples.map((s) => s[key]);
        expect(
          Math.max(...values) - Math.min(...values),
          `${name} ${key}: ${JSON.stringify([...new Set(values)])}`,
        ).toBeLessThanOrEqual(0.5);
      }
    }
    await page
      .locator(".site-header")
      .screenshot({ path: `test-results/navigation-stable-${width}.png` });
  });
}
