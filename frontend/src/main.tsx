"use client";
import {
  Banner,
  Button,
  Empty,
  LayerCard,
  useKumoToastManager,
} from "@cloudflare/kumo";
import {
  Component,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ArrowUpRightIcon as ArrowUpRight,
  ChartBarIcon as BarChart3,
  BookOpenIcon as BookOpen,
  CatIcon as Cat,
  CaretRightIcon as ChevronRight,
  LinkIcon as Link2,
  SignInIcon as LogIn,
  SignOutIcon as LogOut,
  ArrowsClockwiseIcon as RefreshCw,
  ShieldCheckIcon as ShieldCheck,
  TrophyIcon as Trophy,
  WalletIcon as Wallet,
  ListNumbersIcon as ListOrdered,
} from "@phosphor-icons/react";
import { api, ApiError, datetime, money } from "./api";
import type {
  Account,
  Market,
  Portfolio as PortfolioData,
  Settings,
  Seasons as SeasonData,
  Stock,
} from "./types";
import { Rules } from "./Rules";
import { Binding, Login, PasswordChange } from "./Auth";
import { StockPanel, Trade } from "./Market";
import { Securities, change } from "./Securities";
import { Portfolio, OrderTable } from "./Portfolio";
import { Seasons, SeasonCard } from "./Seasons";
import { ThemeToggle } from "./theme";
class Boundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <main className="startup">
        <Cat size={40} />
        <h1>页面暂时无法显示</h1>
        <p>请重新加载，已确认的委托不会重复提交。</p>
        <Button onClick={() => location.reload()}>重新加载</Button>
      </main>
    ) : (
      this.props.children
    );
  }
}
const navigation = [
  ["market", "市场行情", BarChart3],
  ["portfolio", "持仓与委托", Wallet],
  ["orders", "我的委托", ListOrdered],
  ["seasons", "月赛排行榜", Trophy],
  ["rules", "交易规则", BookOpen],
] as const;
function App() {
  const toasts = useKumoToastManager();
  const [view, setView] = useState("market"),
    [market, setMarket] = useState<Market>(),
    [settings, setSettings] = useState<Settings>(),
    [account, setAccount] = useState<Account>(),
    [portfolio, setPortfolio] = useState<PortfolioData>(),
    [seasons, setSeasons] = useState<SeasonData>(),
    [error, setError] = useState(""),
    [login, setLogin] = useState(false),
    [binding, setBinding] = useState(false),
    [selected, setSelected] = useState(() =>
      Number(new URLSearchParams(location.search).get("stock") ?? 0),
    ),
    [connection, setConnection] = useState("连接中"),
    [notice, setNotice] = useState(""),
    [now, setNow] = useState(Date.now());
  const accountRef = useRef(account);
  accountRef.current = account;
  const refresh = useCallback(async () => {
    try {
      const [m, s, r] = await Promise.all([
        api<Market>("/public/market"),
        api<Settings>("/public/rules"),
        api<SeasonData>("/public/seasons"),
      ]);
      setMarket(m);
      setSettings(s);
      setSeasons(r);
      setError("");
      if (accountRef.current) {
        const a = (await api<{ account: Account }>("/auth/me")).account;
        setAccount(a);
        if (!a.forcePasswordChange && a.boundStockId) {
          const p = await api<PortfolioData>("/account");
          setPortfolio(p);
          setAccount(p.account);
        } else setPortfolio(undefined);
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setAccount(undefined);
        setPortfolio(undefined);
      } else setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    let active = true,
      busy = false,
      queued = false;
    const update = async () => {
      queued = true;
      if (busy) return;
      busy = true;
      try {
        do {
          queued = false;
          if (active) await refresh();
        } while (queued && active);
      } finally {
        busy = false;
      }
    };
    void api<{ account: Account }>("/auth/me")
      .then((r) => {
        if (active) {
          accountRef.current = r.account;
          setAccount(r.account);
          void update();
        }
      })
      .catch(() => {});
    void update();
    let scheduled: number | undefined;
    const schedule = () => {
      if (scheduled !== undefined) return;
      scheduled = window.setTimeout(() => {
        scheduled = undefined;
        void update();
      }, 200);
    };
    const events = new EventSource("/api/v2/public/events");
    events.onopen = () => {
      setConnection("行情已连接");
      schedule();
    };
    events.onerror = () => setConnection("正在重连");
    events.addEventListener("change", schedule);
    const visible = () => {
      if (document.visibilityState === "visible") schedule();
    };
    window.addEventListener("online", schedule);
    document.addEventListener("visibilitychange", visible);
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      active = false;
      clearInterval(timer);
      clearTimeout(scheduled);
      events.close();
      window.removeEventListener("online", schedule);
      document.removeEventListener("visibilitychange", visible);
    };
  }, [refresh]);
  useEffect(() => {
    if (notice) {
      toasts.add({ title: notice });
      setNotice("");
    }
  }, [notice, toasts]);
  useEffect(() => {
    const lost = (event: Event) => {
      if ((event as CustomEvent).detail !== "player") return;
      if (accountRef.current) setNotice("登录已失效，请重新登录");
      setAccount(undefined);
      accountRef.current = undefined;
      setPortfolio(undefined);
    };
    window.addEventListener("mmex-auth-lost", lost);
    return () => window.removeEventListener("mmex-auth-lost", lost);
  }, []);
  const stocks = market?.stocks ?? [],
    stock =
      stocks.find((s) => s.id === selected) ??
      stocks.find(
        (s) => s.id !== account?.boundStockId && !s.delisted && !s.disabled,
      ) ??
      stocks[0],
    ready = !!account && !account.forcePasswordChange && !!account.boundStockId;
  function select(id: number) {
    setSelected(id);
    setView("market");
  }
  async function logout() {
    try {
      await api("/auth/logout", {});
      setAccount(undefined);
      accountRef.current = undefined;
      setPortfolio(undefined);
      setBinding(false);
      setView("market");
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function star(s: Stock) {
    if (!account) {
      setLogin(true);
      return;
    }
    try {
      await api(
        `/account/watchlist/${s.id}`,
        { enabled: !account.watchlist?.includes(s.id) },
        "PUT",
      );
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  const myRank = seasons?.rankings.find((r) => r.accountId === account?.id);
  return (
    <div className="app-shell stock-terminal">
      <aside className="sidebar">
        <a href="/" className="brand">
          <span className="brand-mark">
            <Cat size={27} />
          </span>
          <span>
            <strong>茗喵证券交易所</strong>
            <small>Mingmiao Exchange</small>
          </span>
        </a>
        <span className="nav-label">交易空间</span>
        <nav aria-label="主导航">
          {navigation.map(([id, label, Icon]) => (
            <Button
              key={id}
              variant={view === id ? "secondary" : "ghost"}
              className={view === id ? "active" : ""}
              onClick={() => setView(id)}
            >
              <Icon size={19} />
              <span>
                {label}
                {id === "orders" &&
                portfolio?.orders.some((o) => o.status === "pending")
                  ? ` (${portfolio.orders.filter((o) => o.status === "pending").length})`
                  : ""}
              </span>
              <ChevronRight size={15} />
            </Button>
          ))}
        </nav>
        <SeasonCard season={market?.season} now={now} />
        <div className="sidebar-bottom">
          <div className="join-note">
            <span className="tiny-label">从行动力，看见价值</span>
            <p>
              在 QQ 群向官方机器人发送
              <br />
              <code>注册 &lt;用户名&gt;</code>
            </p>
            <a
              href="https://qm.qq.com/q/93NWF3S362"
              target="_blank"
              rel="noreferrer"
            >
              加入 QQ 群 1126124220 <ArrowUpRight size={14} />
            </a>
          </div>
          <a className="admin-link" href="/console">
            <ShieldCheck size={16} />
            管理控制台
          </a>
          <small>模拟证券游戏 · 不涉及真实资金</small>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="breadcrumb">
            茗交所 <span>/</span>{" "}
            <b>
              {view === "detail"
                ? "证券详情"
                : navigation.find((n) => n[0] === view)?.[1]}
            </b>
          </div>
          <div className="top-actions">
            <ThemeToggle />
            <Button
              variant="ghost"
              shape="square"
              className="icon"
              aria-label="刷新行情与账户"
              onClick={() => void refresh()}
            >
              <RefreshCw size={16} />
            </Button>
            <span
              className={`connection ${connection !== "行情已连接" ? "offline" : ""}`}
            >
              <i />
              {connection}
            </span>
            {account ? (
              <>
                <span className="avatar">{account.username[0]}</span>
                <span className="account-name">{account.username}</span>
                <Button
                  variant="ghost"
                  shape="square"
                  className="icon"
                  onClick={() => void logout()}
                  aria-label="退出登录"
                >
                  <LogOut size={16} />
                </Button>
              </>
            ) : (
              <Button
                variant="primary"
                className="primary"
                onClick={() => setLogin(true)}
              >
                <LogIn size={16} />
                登录交易账户
              </Button>
            )}
          </div>
        </header>
        <main>
          {error && (
            <Banner variant="error" className="retry" role="alert">
              {error}
              <Button onClick={() => void refresh()}>
                <RefreshCw size={14} />
                重试
              </Button>
            </Banner>
          )}
          {ready && (
            <div className="account-strip">
              <span>
                净资产 <b>{money(account.equity)}</b>
              </span>
              <span>
                可用 <b>{money(account.availableCash ?? 0)}</b>
              </span>
              <span>
                挂单冻结 <b>{money(account.reserved)}</b>
              </span>
              <span>
                浮盈亏 <b>{money(account.metrics?.floatingPnL ?? 0)}</b>
              </span>
              <span>
                排名 {myRank?.rank ?? "—"} / {seasons?.rankings.length ?? 0}
              </span>
            </div>
          )}
          {view === "market" && (
            <>
              <div className="page-heading">
                <div>
                  <div className="eyebrow">模拟证券 · 行动力市场</div>
                  <h1>
                    市场行情<span className="live-label">公开行情</span>
                  </h1>
                  <p>每一次行动，都是市场的新动向。</p>
                </div>
                <span className="date">
                  {new Date(now).toLocaleDateString("zh-CN", {
                    timeZone: "Asia/Shanghai",
                  })}
                </span>
              </div>
              <div className="ticker" aria-label="市场行情条">
                {stocks
                  .filter((s) => !s.delisted)
                  .map((s) => (
                    <Button key={s.id} onClick={() => select(s.id)}>
                      <strong>{s.name}</strong> {money(s.price)}{" "}
                      <small className={(change(s) ?? 0) >= 0 ? "up" : "down"}>
                        {change(s) === undefined
                          ? "—"
                          : `${change(s)!.toFixed(2)}%`}
                      </small>
                    </Button>
                  ))}
              </div>
              <div className="stats-grid">
                <LayerCard className="stat">
                  <span>上市股票</span>
                  <strong>{String(stocks.length)}</strong>
                  <small></small>
                </LayerCard>
                <LayerCard className="stat">
                  <span>参赛人数</span>
                  <strong>{String(seasons?.rankings.length ?? 0)}</strong>
                  <small></small>
                </LayerCard>
                <LayerCard className="stat">
                  <span>本人名次</span>
                  <strong>{String(myRank?.rank ?? "—")}</strong>
                  <small></small>
                </LayerCard>
                <LayerCard className="stat">
                  <span>市场状态</span>
                  <strong>{market?.open ? "开放交易" : "暂未开市"}</strong>
                  <small>{market?.reason}</small>
                </LayerCard>
              </div>
              <div className="market-layout">
                <Securities
                  now={now}
                  stocks={stocks}
                  account={account}
                  portfolio={portfolio}
                  settings={settings}
                  selected={stock?.id}
                  onSelect={select}
                  onStar={(s) => void star(s)}
                />
                <div className="quote-column">
                  {stock ? (
                    <StockPanel
                      stock={stock}
                      onDetail={() => setView("detail")}
                    />
                  ) : (
                    <LayerCard className="panel empty-chart">
                      <Empty
                        title="等待第一支股票"
                        description="在 AzurPilot 注册实例股票后查看行动力行情。"
                      />
                    </LayerCard>
                  )}
                </div>
                {stock && (
                  <Trade
                    stock={stock}
                    account={account}
                    portfolio={portfolio}
                    settings={settings}
                    market={market}
                    online={connection === "行情已连接"}
                    onLogin={() => setLogin(true)}
                    onDone={async (o) => {
                      setNotice(
                        o.status === "rejected"
                          ? `委托 #${o.id} 未通过：${o.reason ?? ""}`
                          : o.filled
                            ? `委托 #${o.id} 成交 ${o.filled} 股，均价 ${money(o.averagePrice)}${o.remaining ? "，余量挂单" : ""}`
                            : `委托 #${o.id} 已挂单`,
                      );
                      await refresh();
                    }}
                  />
                )}
              </div>
              {ready && portfolio && (
                <OrderTable
                  orders={portfolio.orders}
                  stocks={stocks}
                  onRefresh={refresh}
                  onSelect={select}
                  onError={setError}
                />
              )}
            </>
          )}
          {view === "detail" && stock && (
            <>
              <div className="page-heading">
                <h1>{stock.name} · 证券详情</h1>
                <Button onClick={() => setView("market")}>返回市场</Button>
              </div>
              <div className="detail-layout">
                <StockPanel stock={stock} />
                <Trade
                  stock={stock}
                  account={account}
                  portfolio={portfolio}
                  settings={settings}
                  market={market}
                  online={connection === "行情已连接"}
                  onLogin={() => setLogin(true)}
                  onDone={async () => {
                    setNotice("委托已提交");
                    await refresh();
                  }}
                />
              </div>
            </>
          )}
          {(view === "portfolio" || view === "orders") && (
            <>
              <div className="page-heading">
                <div>
                  <h1>{view === "portfolio" ? "持仓与委托" : "我的委托"}</h1>
                  <p>查看资金、损益和每一笔实际成交。</p>
                </div>
                {account && (
                  <Button onClick={() => setBinding(true)}>
                    <Link2 size={16} />
                    查看股票绑定
                  </Button>
                )}
              </div>
              {!ready ? (
                <LayerCard className="panel login-prompt">
                  <Wallet size={40} />
                  <h2>登录并绑定后查看交易账户</h2>
                  <p>
                    在群内向官方机器人发送“注册
                    &lt;用户名&gt;”，登录改密并绑定实例股票。
                  </p>
                  <Button
                    variant="primary"
                    className="primary"
                    onClick={() => setLogin(true)}
                  >
                    登录交易账户
                  </Button>
                </LayerCard>
              ) : view === "portfolio" ? (
                <Portfolio
                  data={portfolio}
                  stocks={stocks}
                  onSelect={select}
                  onRefresh={refresh}
                  onError={setError}
                />
              ) : (
                <OrderTable
                  orders={portfolio?.orders ?? []}
                  stocks={stocks}
                  onRefresh={refresh}
                  onSelect={select}
                  onError={setError}
                />
              )}
            </>
          )}
          {view === "seasons" && (
            <Seasons data={seasons} accountId={account?.id} />
          )}
          {view === "rules" && <Rules settings={settings} market={market} />}
          <footer>
            <span>© {new Date(now).getFullYear()} 茗喵证券交易所</span>
            <span>本赛季截止 {datetime(market?.season?.endsAt ?? 0)}</span>
            <a
              href="https://qm.qq.com/q/93NWF3S362"
              target="_blank"
              rel="noreferrer"
            >
              帮助与反馈
            </a>
          </footer>
        </main>
      </div>
      <Login
        open={login}
        onClose={() => setLogin(false)}
        onDone={(a) => {
          accountRef.current = a;
          setAccount(a);
          setLogin(false);
          void refresh();
        }}
      />
      <PasswordChange
        open={!!account?.forcePasswordChange}
        onLogout={() => void logout()}
        onDone={refresh}
      />
      <Binding
        open={
          !!account &&
          !account.forcePasswordChange &&
          (binding || !account.boundStockId)
        }
        account={account}
        stocks={stocks}
        onClose={account?.boundStockId ? () => setBinding(false) : undefined}
        onRefresh={() => void refresh()}
        onLogout={() => void logout()}
      />
    </div>
  );
}
export default function Exchange() {
  return (
    <Boundary>
      <App />
    </Boundary>
  );
}
