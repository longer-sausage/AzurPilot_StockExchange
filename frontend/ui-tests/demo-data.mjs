import { readFileSync, readdirSync } from "node:fs";
import { randomUUID } from "node:crypto";

// 所有名称、身份、资金和密码均为本地模拟夹具，不代表线上账户。
export function makeDemo() {
  const now = () => Date.now(),
    date = () =>
      new Date().toLocaleDateString("sv-SE", { timeZone: "Asia/Shanghai" });
  const fees = {
    tax: 0,
    commission: 18000,
    stamp: 0,
    levy: 0,
    borrow: 0,
    financing: 0,
  };
  const defaults = JSON.parse(
    readFileSync(new URL("../../config/defaults.json", import.meta.url)),
  );
  const presetRoot = new URL("../../config/presets/", import.meta.url);
  const presets = readdirSync(presetRoot)
    .filter((f) => f.endsWith(".json"))
    .map((f) => JSON.parse(readFileSync(new URL(f, presetRoot))));
  let settings = {
    ...defaults,
    marketPresets: presets,
    activeMarket: presets.find((p) => p.id === "meow"),
    paused: false,
  };
  const names = [
    "海风研究所",
    "星港远航",
    "明石工坊",
    "极光舰队",
    "指挥官后勤",
    "蓝海档案",
    "白鹰航线",
    "铁血工业",
  ];
  let stocks = names.map((name, i) => ({
    id: i + 1,
    symbol: `MM${String(i + 1).padStart(4, "0")}`,
    name,
    price: [128640, 235000, 86500, 156800, 102300, 73900, 198400, 65400][i],
    open: [124000, 240000, 83000, 152000, 104000, 72000, 192000, 68000][i],
    openDay: date(),
    openAt: now() - 3600000,
    previous: 120000,
    observedAt: now(),
    uploadedAt: now(),
    historyVersion: 1,
    tradeVersion: 1,
    bound: true,
    disabled: false,
    delisted: false,
    issuedShares: 10000,
    outstanding: 1450 + i * 100,
    hourlyVolume: 1000,
    normalUsed: 120 + i * 40,
    borrowShares: 10000,
    borrowed: 80 + i * 10,
    borrowHourlyVolume: 1000,
    borrowUsed: 20,
    accountId: i + 1,
    instanceId: `demo-instance-${i + 1}`,
    publicKey: "local-demo-public-key",
    bindingCode: "DEMO-LOCAL-ONLY",
  }));
  let account = {
    id: 1,
    username: "预览玩家",
    identityCode: "DEMO-LOCAL-ONLY",
    createdAt: now() - 86400000 * 7,
    initialCash: 2000000000,
    cash: 1893220000,
    reserved: 2500000,
    availableCash: 1890720000,
    equity: 2016840000,
    returnPPM: 8420,
    realized: 420000,
    fees,
    watchlist: [1, 3],
    boundStockId: 5,
    forcePasswordChange: false,
    disabled: false,
    metricsSince: now() - 86400000,
    legacyMetrics: false,
    metrics: {
      floatingPnL: 16420000,
      longPnL: 16420000,
      shortPnL: 0,
      financingDebt: 0,
      collateral: 0,
      unsettled: 0,
    },
    settlements: [],
  };
  const season = {
    id: date().slice(0, 7),
    startsAt: new Date(date().slice(0, 7) + "-01T00:00:00+08:00").getTime(),
    endsAt: new Date(date().slice(0, 7) + "-28T23:59:59+08:00").getTime(),
    settled: false,
  };
  let orders = [
    {
      id: 1001,
      stockId: 1,
      side: "buy",
      kind: "limit",
      timeInForce: "GTC",
      expiresAt: 0,
      leverage: 1,
      quantity: 100,
      remaining: 100,
      filled: 0,
      price: 120000,
      averagePrice: 0,
      notional: 0,
      tax: 0,
      commission: 0,
      stamp: 0,
      levy: 0,
      status: "pending",
      reserved: 12000000,
      createdAt: now() - 600000,
      updatedAt: now(),
    },
  ];
  let trades = [
    {
      id: 9001,
      orderId: 1000,
      stockId: 1,
      accountId: 1,
      username: account.username,
      side: "buy",
      kind: "limit",
      quantity: 500,
      price: 124000,
      notional: 62000000,
      at: now() - 300000,
      forced: false,
      reason: "",
      fees,
    },
  ];
  const positions = [
    {
      stockId: 1,
      stock: stocks[0],
      quantity: 500,
      sellable: 500,
      cost: 62000000,
      shortQuantity: 0,
      shortCost: 0,
      shortProceeds: 0,
      collateral: 0,
      loan: 0,
    },
    {
      stockId: 3,
      stock: stocks[2],
      quantity: 700,
      sellable: 700,
      cost: 58000000,
      shortQuantity: 0,
      shortCost: 0,
      shortProceeds: 0,
      collateral: 0,
      loan: 0,
    },
  ];
  let version = 1;
  const streams = new Set(),
    previews = new Map(),
    confirmed = new Map();
  const notify = () => {
    version++;
    for (const res of streams)
      res.write(`event: change\ndata: {"version":${version}}\n\n`);
  };
  const portfolio = () => ({ account, positions, orders, trades });
  // 本地夹具的两个图表接口共用同一组模拟历史，正式前端不包含此生成器。
  const chartBars = (stock, day) => {
    const start = new Date(day + "T09:30:00+08:00").getTime();
    return Array.from({ length: 180 }, (_, i) => {
      const open = Math.round(
        stock.open +
          ((stock.price - stock.open) * i) / 180 +
          Math.sin(i * (0.08 + stock.id * 0.013) + stock.id) * 600,
      );
      return {
        time: start + i * 60000,
        open,
        close: open + Math.round(Math.sin(i * 0.4 + stock.id) * 250),
        high: open + 450,
        low: open - 450,
        samples: 3,
        volume: 20 + (i % 40),
        turnover: open * (20 + (i % 40)),
        buyVolume: 12 + (i % 22),
        sellVolume: 8 + (i % 18),
      };
    });
  };
  const json = (res, data, status = 200) => {
    res.writeHead(status, {
      "Content-Type": "application/json; charset=utf-8",
      "Cache-Control": "no-store",
    });
    res.end(JSON.stringify(data));
  };
  const error = (res, status, code, message) =>
    json(res, { error: { code, message } }, status);
  const timer = setInterval(() => {
    for (const res of streams) res.write(": demo keepalive\n\n");
  }, 20000);
  return {
    close() {
      clearInterval(timer);
      for (const res of streams) res.end();
    },
    async handle(req, res, url) {
      const route = url.pathname.slice("/api/v2".length),
        method = req.method,
        cookies = req.headers.cookie ?? "";
      let body = {};
      if (method !== "GET") {
        let raw = "";
        for await (const chunk of req) raw += chunk;
        if (raw) body = JSON.parse(raw);
      }
      const setCookie = (role, value) =>
        res.setHeader(
          "Set-Cookie",
          `mmex_${role}=${value}; Path=/api/v2; HttpOnly; SameSite=Strict${value ? "" : "; Max-Age=0"}`,
        );
      if (route === "/public/events") {
        res.writeHead(200, {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
        });
        res.write(`event: change\ndata: {"version":${version}}\n\n`);
        streams.add(res);
        req.on("close", () => streams.delete(res));
        return;
      }
      if (route === "/public/market")
        return json(res, {
          version,
          stocks: stocks.filter((s) => !s.deletedAt),
          serverTime: now(),
          open: !settings.paused,
          reason: settings.paused ? "管理员暂停市场" : "",
          season,
        });
      if (route === "/public/rules") return json(res, settings);
      if (route === "/public/seasons")
        return json(res, {
          season,
          rankings: [
            {
              rank: 1,
              accountId: 2,
              username: "星港观察员",
              equity: 2231400000,
              returnPPM: 115700,
            },
            {
              rank: 2,
              accountId: 1,
              username: account.username,
              equity: account.equity,
              returnPPM: account.returnPPM,
            },
            {
              rank: 3,
              accountId: 3,
              username: "明石采购部",
              equity: 1987000000,
              returnPPM: -6500,
            },
          ],
          archives: [],
          revenue: fees,
          lifetimeRevenue: fees,
          serverTime: now(),
        });
      const previewChart = route.match(/^\/public\/stocks\/(\d+)\/chart$/);
      if (previewChart) {
        const stock = stocks.find((s) => s.id === Number(previewChart[1]));
        if (!stock || stock.deletedAt)
          return error(res, 404, "STOCK_NOT_FOUND", "股票不存在");
        const from = Number(url.searchParams.get("from")),
          to = Number(url.searchParams.get("to"));
        return json(res, {
          stockId: stock.id,
          interval: 1,
          points: chartBars(stock, date())
            .filter((p) => p.time >= from && p.time <= to)
            .map((p) => ({
              t: p.time,
              o: p.open,
              h: p.high,
              l: p.low,
              c: p.close,
              n: p.samples,
            })),
        });
      }
      const chart = route.match(/^\/public\/stocks\/(\d+)\/chart-detail$/);
      if (chart) {
        const stock = stocks.find((s) => s.id === Number(chart[1]));
        if (!stock || stock.deletedAt)
          return error(res, 404, "STOCK_NOT_FOUND", "股票不存在");
        const period = url.searchParams.get("period") ?? "time",
          day = url.searchParams.get("day") || date(),
          month = url.searchParams.get("month") || date().slice(0, 7),
          start = new Date(day + "T09:30:00+08:00").getTime();
        const bars = chartBars(stock, day);
        return json(res, {
          stock,
          period,
          month,
          day,
          displayFrom: start,
          bars,
          trades: trades.filter((t) => t.stockId === stock.id),
          orders: orders
            .filter((o) => o.stockId === stock.id && o.status === "pending")
            .map((o) => ({ ...o, username: account.username })),
          legacyOrders: [],
          summary: {
            open: stock.open,
            high: stock.price + 1000,
            low: stock.open - 1000,
            close: stock.price,
            previousClose: stock.open,
            volume: 7200,
            turnover: 7200 * stock.price,
            buyVolume: 4500,
            sellVolume: 2700,
          },
          coverage: {
            count: 540,
            firstObservedAt: start,
            lastObservedAt: now(),
            reconciledAt: now(),
          },
        });
      }
      if (route === "/auth/login") {
        if (body.password !== "Demo-only-Password!87")
          return error(res, 401, "INVALID_PASSWORD", "账号或密码不正确");
        account = {
          ...account,
          username: body.username,
          forcePasswordChange: body.username === "首次登录演示",
        };
        setCookie("player", "demo");
        return json(res, {
          account,
          forcePasswordChange: account.forcePasswordChange,
        });
      }
      if (route === "/auth/logout") {
        setCookie("player", "");
        return json(res, { ok: true });
      }
      if (route === "/admin/login") {
        if (body.password !== "Demo-only-Admin!87")
          return error(res, 401, "INVALID_PASSWORD", "管理员密码不正确");
        setCookie("admin", "demo");
        return json(res, { ok: true });
      }
      if (route === "/admin/logout") {
        setCookie("admin", "");
        return json(res, { ok: true });
      }
      if (route.startsWith("/admin/")) {
        if (!cookies.includes("mmex_admin=demo"))
          return error(res, 401, "UNAUTHORIZED", "请先登录管理员");
        if (route === "/admin/me") return json(res, { ok: true });
        if (route === "/admin/settings") {
          if (method === "PUT") {
            settings = body;
            notify();
          }
          return json(res, settings);
        }
        if (route === "/admin/stocks") return json(res, { stocks });
        if (route === "/admin/accounts")
          return json(res, { accounts: [account] });
        if (/^\/admin\/accounts\/\d+\/reset-password$/.test(route))
          return json(res, {
            username: account.username,
            password: "Demo-reset-Password!87",
            forcePasswordChange: true,
          });
        if (/^\/admin\/accounts\/\d+$/.test(route)) {
          if (method === "PUT") {
            account = {
              ...account,
              username: body.username ?? account.username,
              cash: account.cash + (body.cashAdjustment ?? 0),
              disabled: body.disabled ?? account.disabled,
            };
            notify();
          }
          return json(res, portfolio());
        }
        const match = route.match(/^\/admin\/stocks\/(\d+)(\/status)?$/);
        if (match) {
          const stock = stocks.find((s) => s.id === Number(match[1]));
          if (method === "DELETE") stock.deletedAt = now();
          else Object.assign(stock, body);
          notify();
          return json(res, { stock });
        }
      }
      if (!cookies.includes("mmex_player=demo"))
        return error(res, 401, "UNAUTHORIZED", "请先登录交易账户");
      if (route === "/auth/me")
        return json(res, {
          account,
          forcePasswordChange: account.forcePasswordChange,
        });
      if (route === "/auth/password") {
        account.forcePasswordChange = false;
        notify();
        return json(res, { ok: true });
      }
      if (route === "/account") return json(res, portfolio());
      const watch = route.match(/^\/account\/watchlist\/(\d+)$/);
      if (watch) {
        account.watchlist = account.watchlist.filter(
          (id) => id !== Number(watch[1]),
        );
        if (body.enabled) account.watchlist.push(Number(watch[1]));
        notify();
        return json(res, { ok: true });
      }
      if (route === "/orders/preview") {
        const stock = stocks.find((s) => s.id === Number(body.stock)),
          previewId = randomUUID(),
          notional = body.quantity * body.price;
        const preview = {
          ...body,
          stock,
          previewId,
          expiresAt: now() + 300000,
          notional,
          taxRatePPM: 0,
          tax: 0,
          commission: 500,
          stamp: 0,
          levy: 0,
          estimatedTotal: notional + 500,
          premiumPPM: 0,
          financing: 0,
          collateral: 0,
          repayment: 0,
        };
        previews.set(previewId, preview);
        return json(res, preview);
      }
      if (route === "/orders/confirm") {
        if (confirmed.has(body.previewId))
          return json(res, { order: confirmed.get(body.previewId) });
        const p = previews.get(body.previewId);
        if (!p) return error(res, 400, "PREVIEW_EXPIRED", "请重新预览");
        const order = {
          ...p,
          id: 1000 + orders.length + 1,
          stockId: p.stock.id,
          filled: p.quantity,
          remaining: 0,
          status: "filled",
          averagePrice: p.price,
          reserved: 0,
          createdAt: now(),
          updatedAt: now(),
        };
        orders.push(order);
        confirmed.set(body.previewId, order);
        notify();
        return json(res, { order });
      }
      const order = route.match(/^\/orders\/(\d+)$/);
      if (order && method === "DELETE") {
        const found = orders.find((o) => o.id === Number(order[1]));
        if (found) found.status = "cancelled";
        notify();
        return json(res, { order: found });
      }
      return error(res, 404, "NOT_FOUND", "本地夹具未提供该接口");
    },
  };
}
