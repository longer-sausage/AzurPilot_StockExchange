import { test, expect, type Page } from "@playwright/test";
import defaults from "../../config/defaults.json" with { type: "json" };
import meow from "../../config/presets/meow.json" with { type: "json" };
import type { Market, Settings } from "../src/core/contracts";
import { marketOpening, openingWindow } from "../src/core/marketOpening";
import {
  countdownNanoseconds,
  simulateCountdown,
} from "../src/core/useCountdown";

const opensAt = Date.parse("2026-11-05T00:00:00+08:00"),
  noon = opensAt - 12 * 3600000;
function fixtures() {
  return {
    settings: {
      ...defaults,
      activeMarket: meow,
      marketPresets: [meow],
    } as Settings,
    market: {
      version: 1,
      stocks: [],
      serverTime: noon,
      open: false,
      reason: "本月赛事尚未开始",
      season: {
        id: "2026-11",
        startsAt: opensAt,
        endsAt: Date.parse("2026-11-27T00:00:00+08:00"),
        settled: false,
      },
    } satisfies Market,
  };
}

test("开市窗口与暂停、结算、截止和已开放状态一致", () => {
  const { market, settings } = fixtures();
  expect(marketOpening(market, settings, noon)).toBe(opensAt);
  expect(marketOpening(market, settings, opensAt - openingWindow)).toBe(
    opensAt,
  );
  expect(
    marketOpening(market, settings, opensAt - openingWindow - 120000),
  ).toBeUndefined();
  expect(marketOpening(undefined, settings, noon)).toBeUndefined();
  expect(marketOpening(market, undefined, noon)).toBeUndefined();
  expect(
    marketOpening(market, { ...settings, paused: true }, noon),
  ).toBeUndefined();
  expect(
    marketOpening({ ...market, open: true }, settings, noon),
  ).toBeUndefined();
  expect(
    marketOpening(
      { ...market, season: { ...market.season, settled: true } },
      settings,
      noon,
    ),
  ).toBeUndefined();
  expect(marketOpening(market, settings, market.season.endsAt)).toBeUndefined();
});

test("分段交易、周末、假日和市场时区决定真正开市时间", () => {
  const { market, settings } = fixtures();
  market.season.startsAt = Date.parse("2026-11-01T00:00:00+08:00");
  settings.activeMarket = {
    ...meow,
    weekdaysOnly: true,
    sessions: ["09:30-11:30", "13:00-15:00"],
  };
  const at = (time: string) =>
    marketOpening(market, settings, Date.parse(time));
  expect(at("2026-11-04T12:00:00+08:00")).toBe(
    Date.parse("2026-11-04T13:00:00+08:00"),
  );
  expect(at("2026-11-06T16:00:00+08:00")).toBeUndefined();
  expect(at("2026-11-08T18:00:00+08:00")).toBe(
    Date.parse("2026-11-09T09:30:00+08:00"),
  );
  settings.activeMarket.holidays = ["2026-11-09"];
  expect(at("2026-11-08T18:00:00+08:00")).toBeUndefined();
  settings.activeMarket = {
    ...settings.activeMarket,
    holidays: [],
    timezone: "America/New_York",
  };
  // 纽约 11 月 1 日退出夏令时；周一 09:30 对应 UTC 14:30。
  expect(at("2026-11-01T16:00:00Z")).toBe(Date.parse("2026-11-02T14:30:00Z"));
  market.season.endsAt = Date.parse("2026-11-04T13:00:00+08:00");
  settings.activeMarket = {
    ...settings.activeMarket,
    timezone: "Asia/Shanghai",
  };
  expect(at("2026-11-04T12:00:00+08:00")).toBeUndefined();
});

test("纳秒换算保持九位余数、24 小时整数精度及到零边界", () => {
  expect(countdownNanoseconds(opensAt, opensAt - 1000, 123.456789)).toBe(
    876543211n,
  );
  expect(countdownNanoseconds(opensAt, opensAt - 1, 0.999999)).toBe(1n);
  expect(countdownNanoseconds(opensAt, opensAt - 1, 1)).toBe(0n);
  expect(countdownNanoseconds(opensAt, opensAt - 1, 1000)).toBe(0n);
  expect(countdownNanoseconds(opensAt, opensAt + 1)).toBe(0n);
  expect(countdownNanoseconds(opensAt, opensAt - openingWindow, 0.000001)).toBe(
    86_399_999_999_999n,
  );
});

test("模拟尾数保留实际时间高位，末五位变化且到零归零", () => {
  const values = Array.from({ length: 16 }, (_, frame) =>
    simulateCountdown(285600000n, frame),
  );
  for (const value of values) {
    expect(value / 100_000n).toBe(2856n);
    expect(value % 100_000n).toBeGreaterThan(0n);
  }
  for (let digit = 0; digit < 5; digit++)
    expect(
      new Set(values.map((value) => String(value).slice(-5)[digit])).size,
    ).toBeGreaterThan(1);
  expect(simulateCountdown(1n, 0)).toBeGreaterThan(0n);
  expect(simulateCountdown(999999999n, 0)).toBeLessThan(1_000_000_000n);
  expect(simulateCountdown(0n, 0)).toBe(0n);
});

test.beforeEach(async ({ request }) => {
  expect((await request.post("/__demo/reset")).status()).toBe(204);
});

async function mockOpening(page: Page, start = noon, liveClock = false) {
  const state = { open: false, paused: false, requests: 0, fail: false };
  await page.clock.setFixedTime(start);
  if (!liveClock)
    await page.addInitScript(() => {
      // Date 已由 Playwright 固定；让单调时钟同步受控，消除真实逐帧走时。
      const wallTime = Date.now(),
        monotonicTime = performance.now();
      Object.defineProperty(performance, "now", {
        value: () => monotonicTime + (Date.now() - wallTime),
      });
    });
  await page.route("**/api/v2/public/market", async (route) => {
    state.requests++;
    if (state.fail)
      return route.fulfill({
        status: 503,
        json: { error: { message: "隔离测试：开市状态暂不可用" } },
      });
    const response = await route.fetch(),
      data = await response.json();
    await route.fulfill({
      response,
      json: {
        ...data,
        open: state.open,
        reason: state.paused ? "管理员暂停交易" : "本月赛事尚未开始",
        season: fixtures().market.season,
      },
    });
  });
  await page.route("**/api/v2/public/rules", async (route) => {
    const response = await route.fetch(),
      data = await response.json();
    await route.fulfill({ response, json: { ...data, paused: state.paused } });
  });
  return state;
}

test("24 小时外保留原页面，进入窗口后只显示倒计时且每秒递减", async ({
  page,
}) => {
  await mockOpening(page, opensAt - openingWindow - 1000);
  await page.goto("/");
  await expect(page.getByRole("table", { name: "证券行情" })).toBeVisible();
  await page.clock.setFixedTime(opensAt - openingWindow);
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    /^距离开市还有 24:00:00:0000\d{5}$/,
  );
  await expect(page.getByRole("tab")).toHaveCount(0);
  await expect(page.getByLabel("市场状态", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("table", { name: "证券行情" })).toHaveCount(0);
  await page.clock.setFixedTime(opensAt - openingWindow + 1000);
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    /^距离开市还有 23:59:59:0000\d{5}$/,
  );
  await expect(
    page.getByText("2026/11/5 00:00:00", { exact: true }),
  ).toBeVisible();
});

test("倒计时阻止旧视图和刷新绕过，管理入口正常可用", async ({ page }) => {
  await mockOpening(page);
  let chartRequests = 0;
  page.on("request", (request) => {
    if (/\/stocks\/\d+\/chart/.test(request.url())) chartRequests++;
  });
  for (const path of [
    "security?stock=1",
    "portfolio",
    "orders",
    "seasons",
    "guide",
  ]) {
    await page.goto(`/#/${path}`);
    await expect(
      page.getByRole("heading", { name: "开市倒计时", exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("tab")).toHaveCount(0);
  }
  await page.reload();
  await expect(page.getByRole("timer")).toBeVisible();
  expect(chartRequests).toBe(0);
  await page.getByRole("link", { name: "管理", exact: true }).click();
  await expect(page.getByLabel("管理员密码")).toBeVisible();
  await expect(page.getByRole("timer")).toHaveCount(0);
  await page.getByRole("link", { name: "返回交易站", exact: true }).click();
  await expect(page.getByRole("timer")).toBeVisible();
});

test("倒计时期间可以登录、查看绑定并退出", async ({ page }) => {
  await mockOpening(page);
  await page.goto("/");
  await page.getByRole("button", { name: "登录交易账户", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "登录交易账户",
    exact: true,
  });
  await dialog.getByLabel("账号", { exact: true }).fill("预览玩家");
  await dialog
    .getByLabel("密码", { exact: true })
    .fill("Demo-only-Password!87");
  await dialog.getByRole("button", { name: "登录", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("timer")).toBeVisible();
  await page.getByRole("button", { name: "预览玩家", exact: true }).click();
  const binding = page.getByRole("dialog", {
    name: "关联你的实例股票",
    exact: true,
  });
  await expect(binding.getByText(/已成功绑定股票/)).toBeVisible();
  await binding.getByRole("button", { name: "完成", exact: true }).click();
  await page.getByRole("button", { name: "退出登录", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "登录交易账户", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("timer")).toBeVisible();
});

test("首次改密与未绑定引导在倒计时期间仍完整显示", async ({ page }) => {
  await mockOpening(page);
  await page.route(/\/api\/v2\/auth\/(login|me)$/, async (route) => {
    const response = await route.fetch(),
      data = await response.json();
    if (data.account) data.account.boundStockId = 0;
    await route.fulfill({ response, json: data });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "登录交易账户", exact: true }).click();
  const login = page.getByRole("dialog", { name: "登录交易账户", exact: true });
  await login.getByLabel("账号", { exact: true }).fill("首次登录演示");
  await login.getByLabel("密码", { exact: true }).fill("Demo-only-Password!87");
  await login.getByRole("button", { name: "登录", exact: true }).click();
  const password = page.getByRole("dialog", {
    name: "设置你的交易密码",
    exact: true,
  });
  await password
    .getByLabel("当前临时密码", { exact: true })
    .fill("Demo-only-Password!87");
  await password
    .getByLabel("新密码", { exact: true })
    .fill("Isolated-New-Password!87");
  await password
    .getByLabel("再次输入新密码", { exact: true })
    .fill("Isolated-New-Password!87");
  await password
    .getByRole("button", { name: "设置密码并继续", exact: true })
    .click();
  await expect(password).not.toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "关联你的实例股票", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "检查绑定状态", exact: true }),
  ).toBeVisible();
});

test("开市信息加载期间不闪现行情或交易导航", async ({ page }) => {
  await mockOpening(page);
  let releaseRules!: () => void;
  const ready = new Promise<void>((resolve) => {
    releaseRules = resolve;
  });
  await page.route("**/api/v2/public/rules", async (route) => {
    await ready;
    await route.fallback();
  });
  try {
    await page.goto("/");
    await expect(
      page.getByRole("status").filter({ hasText: "正在同步开市时间" }),
    ).toBeVisible();
    await expect(page.getByRole("tab")).toHaveCount(0);
    await expect(page.getByRole("table", { name: "证券行情" })).toHaveCount(0);
  } finally {
    releaseRules();
  }
  await expect(page.getByRole("timer")).toBeVisible();
});

test("到点自动请求后端并恢复行情，无需手动刷新", async ({ page }) => {
  const state = await mockOpening(page, opensAt - 2000);
  await page.goto("/");
  await expect(page.getByRole("timer")).toBeVisible();
  await expect.poll(() => state.requests).toBe(2);
  state.open = true;
  await page.clock.setFixedTime(opensAt);
  await expect(page.getByRole("table", { name: "证券行情" })).toBeVisible();
  await expect(page.getByRole("timer")).toHaveCount(0);
  expect(state.requests).toBeGreaterThan(2);
});

test("到点主动刷新，失败保留零值倒计时，后端确认后自动恢复原视图", async ({
  page,
}) => {
  const state = await mockOpening(page, opensAt - 2000);
  await page.goto("/#/guide");
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    /^距离开市还有 00:00:02:0000\d{5}$/,
  );
  await expect.poll(() => state.requests).toBeGreaterThan(0);
  // 等初次 SSE 合并刷新完成，再隔离到点的请求。
  await expect
    .poll(async () => {
      const before = state.requests;
      await new Promise((resolve) => setTimeout(resolve, 300));
      return state.requests === before;
    })
    .toBe(true);
  const before = state.requests;
  state.fail = true;
  await page.clock.setFixedTime(opensAt);
  await expect.poll(() => state.requests).toBeGreaterThan(before);
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    "距离开市还有 00:00:00:000000000",
  );
  await expect(
    page.getByText("隔离测试：开市状态暂不可用", { exact: true }),
  ).toBeVisible();
  state.fail = false;
  state.open = true;
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(page.getByRole("timer")).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "交易指南", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: "证券市场", exact: true }),
  ).toBeVisible();
});

test("暂停交易不会显示误导倒计时，恢复配置后可重新进入", async ({ page }) => {
  const state = await mockOpening(page);
  state.paused = true;
  await page.goto("/");
  await expect(page.getByRole("table", { name: "证券行情" })).toBeVisible();
  await expect(page.getByRole("timer")).toHaveCount(0);
  state.paused = false;
  await page.getByRole("button", { name: "刷新数据", exact: true }).click();
  await expect(page.getByRole("timer")).toBeVisible();
});

test("桌面与手机明暗主题倒计时无横向溢出", async ({ page }) => {
  await mockOpening(page);
  await page.goto("/");
  await expect(page.getByRole("timer")).toBeVisible();
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const theme of ["light", "dark"]) {
      if ((await page.locator("html").getAttribute("data-mode")) !== theme)
        await page
          .getByRole("button", { name: "切换暗色主题", exact: true })
          .or(page.getByRole("button", { name: "切换亮色主题", exact: true }))
          .click();
      await expect(page.locator("html")).toHaveAttribute("data-mode", theme);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await expect(page.getByRole("timer")).toBeInViewport();
      await page.screenshot({
        path: `test-results/opening-countdown-${width}-${theme}.png`,
        fullPage: true,
      });
    }
  }
});

test("最后一毫秒继续显示纳秒，归零前不提前确认开市", async ({ page }) => {
  const state = await mockOpening(page, opensAt - 1);
  await page.goto("/");
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    /^距离开市还有 00:00:00:0010\d{5}$/,
  );
  const confirming = page.getByText(
    "正在确认开市状态，确认后将自动进入交易站。",
    { exact: true },
  );
  await expect(confirming).toHaveCount(0);
  await expect.poll(() => state.requests).toBe(2);
  await page.clock.setFixedTime(opensAt);
  await expect(page.getByRole("timer")).toHaveAccessibleName(
    "距离开市还有 00:00:00:000000000",
  );
  await expect(confirming).toBeVisible();
  await expect.poll(() => state.requests).toBeGreaterThan(2);
});

test("模拟尾数逐帧变化，单行无单位且不重复请求后端", async ({ page }) => {
  const state = await mockOpening(page, noon, true);
  await page.goto("/");
  const timer = page.getByRole("timer");
  await expect(timer).toHaveText(/^\d{2}:\d{2}:\d{2}:\d{9}$/);
  await expect(timer).not.toContainText(/小时|分钟|纳秒|秒/);
  await expect(timer).toHaveAttribute("aria-live", "off");
  await expect.poll(() => state.requests).toBe(2);
  const samples = await timer.evaluate(
    (element) =>
      new Promise<string[]>((resolve) => {
        const values: string[] = [];
        const sample = () => {
          values.push(element.textContent!.slice(-5));
          if (values.length === 12) resolve(values);
          else requestAnimationFrame(sample);
        };
        requestAnimationFrame(sample);
      }),
  );
  for (let digit = 0; digit < 5; digit++)
    expect(new Set(samples.map((value) => value[digit])).size).toBeGreaterThan(
      1,
    );
  await expect(
    page.getByText("正在确认开市状态，确认后将自动进入交易站。", {
      exact: true,
    }),
  ).toHaveCount(0);
  expect(state.requests).toBe(2);
  await page.getByRole("link", { name: "管理", exact: true }).click();
  await expect(page.getByLabel("管理员密码")).toBeVisible();
  await expect(timer).toHaveCount(0);
});
