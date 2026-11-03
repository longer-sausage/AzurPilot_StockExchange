"use client";
import {
  Banner,
  Button,
  Textarea,
  Checkbox,
  Input,
  LayerCard,
  Select,
  Table,
} from "@cloudflare/kumo";
import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowLeftIcon as ArrowLeft,
  CatIcon as Cat,
  SignOutIcon as LogOut,
  FloppyDiskIcon as Save,
  ShieldCheckIcon as ShieldCheck,
  ArrowsClockwiseIcon as RefreshCw,
} from "@phosphor-icons/react";
import { api, number } from "./api";
import { StockAdmin, AccountAdmin } from "./AdminEntities";
import { ThemeToggle } from "./theme";
import type { Account, MarketRules, Settings, Stock } from "./types";
const fields: {
  key: keyof Settings;
  label: string;
  hint: string;
  money?: boolean;
}[] = [
  {
    key: "initialCash",
    label: "新账户初始模拟币",
    money: true,
    hint: "仅新开户生效，已有账户余额不变",
  },
  {
    key: "issuedShares",
    label: "默认总发行股数",
    hint: "仅新股票生效，已有股票在下方分别调整",
  },
  {
    key: "hourlyVolume",
    label: "默认普通每小时成交股数",
    hint: "买入与卖出共用",
  },
  { key: "borrowShares", label: "默认融券总股数", hint: "独立于普通发行库存" },
  {
    key: "borrowHourlyVolume",
    label: "默认融券每小时成交股数",
    hint: "卖空与回补共用",
  },
  {
    key: "premiumPPM",
    label: "最大溢价（ppm）",
    hint: "50000 = 5%，随当小时额度使用率线性增加",
  },
  {
    key: "borrowAnnualPPM",
    label: "借券年费率（ppm）",
    hint: "80000 = 8%，按持仓时间结算",
  },
  { key: "commissionPPM", label: "成交佣金（ppm）", hint: "300 = 0.03%" },
  {
    key: "minCommission",
    label: "最低佣金（模拟币）",
    money: true,
    hint: "每次成交的最低佣金",
  },
  { key: "quoteTTLSeconds", label: "报价有效期（秒）", hint: "过期后暂停成交" },
];
const marketFields: {
  key: keyof MarketRules;
  label: string;
  hint: string;
  money?: boolean;
}[] = [
  {
    key: "commissionPPM",
    label: "预设佣金（ppm）",
    hint: "现实预设使用此值，新默认使用上方默认佣金",
  },
  {
    key: "minCommission",
    label: "预设最低佣金（模拟币）",
    money: true,
    hint: "保留现实市场最低收费",
  },
  { key: "stampPPM", label: "印花税（ppm）", hint: "按成交金额与征收方向计算" },
  {
    key: "stampRound",
    label: "印花税取整单位（模拟币）",
    money: true,
    hint: "0.01 为分，1 为整模拟币",
  },
  { key: "levyPPM", label: "交易征费（ppm）", hint: "每次成交附加征费" },
  {
    key: "borrowAnnualPPM",
    label: "预设借券年费（ppm）",
    hint: "现实预设使用此值，新默认使用上方默认借息",
  },
  {
    key: "financingAnnualPPM",
    label: "融资年利率（ppm）",
    hint: "按实际融资本金和时间计算",
  },
  {
    key: "imrPPM",
    label: "初始保证金（ppm）",
    hint: "500000 = 50%，1000000 = 100%",
  },
  { key: "mmrPPM", label: "维持保证金（ppm）", hint: "不得高于初始保证金" },
  { key: "lotSize", label: "整手股数", hint: "所有委托股数必须为其整数倍" },
  {
    key: "sellDelayDays",
    label: "买入后可卖延迟（交易日）",
    hint: "T+N 股票交收",
  },
  {
    key: "cashDelayDays",
    label: "卖出后现金可用延迟（交易日）",
    hint: "T+N 现金交收",
  },
  {
    key: "quoteTTLSeconds",
    label: "预设报价有效期（秒）",
    hint: "现实预设使用此值，新默认使用上方报价有效期",
  },
];
function MarketEditor({
  settings: s,
  onChange,
}: {
  settings: Settings;
  onChange: (s: Settings) => void;
}) {
  const [presetId, setPresetId] = useState(""),
    [presetName, setPresetName] = useState("");
  const m = s.activeMarket;
  if (!m) return null;
  const update = (key: keyof MarketRules, value: unknown) =>
    onChange({ ...s, activeMarket: { ...m, [key]: value } });
  return (
    <>
      <LayerCard className="panel padded rules-summary">
        <h2>市场参考预设</h2>
        <p className="muted">
          现实预设保留各自佣金、印花税、征费、交收、保证金和交易时段；新阶梯税和溢价仅用于茗喵默认。所有模式共同遵守股票库存与小时额度。
        </p>
        <div className="preset-grid">
          {s.marketPresets?.map((p) => (
            <Button
              type="button"
              key={p.id}
              variant={m.id === p.id ? "secondary" : "ghost"}
              className={m.id === p.id ? "active" : ""}
              onClick={() =>
                onChange({ ...s, activeMarket: structuredClone(p) })
              }
            >
              <strong>{p.name}</strong>
              <span>
                {p.lotSize} 股整手 · {p.timezone}
              </span>
              <small>{p.description}</small>
            </Button>
          ))}
        </div>
        <div className="preset-tools">
          <label>
            新预设 ID
            <Input
              aria-label="新预设 ID"
              value={presetId}
              onChange={(e) => setPresetId(e.target.value)}
            />
          </label>
          <label>
            新预设名称
            <Input
              aria-label="新预设名称"
              value={presetName}
              onChange={(e) => setPresetName(e.target.value)}
            />
          </label>
          <Button
            type="button"
            disabled={
              !/^[a-z][a-z0-9-]*$/.test(presetId) ||
              !presetName ||
              s.marketPresets.some((p) => p.id === presetId)
            }
            onClick={() => {
              const draft = { ...m, id: presetId, name: presetName };
              onChange({
                ...s,
                activeMarket: draft,
                marketPresets: [...s.marketPresets, draft],
              });
              setPresetId("");
              setPresetName("");
            }}
          >
            另存为新预设
          </Button>
          <Button
            type="button"
            onClick={() =>
              onChange({
                ...s,
                marketPresets: s.marketPresets.map((p) =>
                  p.id === m.id ? structuredClone(m) : p,
                ),
              })
            }
          >
            更新回已保存预设
          </Button>
          <Button
            type="button"
            disabled={s.marketPresets.length <= 1}
            onClick={() => {
              const presets = s.marketPresets.filter((p) => p.id !== m.id);
              onChange({
                ...s,
                marketPresets: presets,
                activeMarket: structuredClone(presets[0]),
              });
            }}
          >
            删除当前预设
          </Button>
        </div>
        <div className="admin-fields">
          <label>
            预设说明
            <Textarea
              value={m.description}
              onChange={(e) => update("description", e.target.value)}
            />
          </label>
          <label>
            参考来源 URL
            <Input
              type="url"
              value={m.source}
              onChange={(e) => update("source", e.target.value)}
            />
          </label>
          <label>
            当前预设名称
            <Input
              value={m.name}
              onChange={(e) => update("name", e.target.value)}
            />
          </label>
          <label>
            市场时区
            <Input
              value={m.timezone}
              onChange={(e) => update("timezone", e.target.value)}
            />
          </label>
          <label>
            交易时段
            <Input
              value={m.sessions.join(",")}
              onChange={(e) =>
                update(
                  "sessions",
                  e.target.value.split(",").map((x) => x.trim()),
                )
              }
            />
            <small>HH:MM-HH:MM，用英文逗号分隔</small>
          </label>
          <label>
            休市日期
            <Input
              value={m.holidays.join(",")}
              onChange={(e) =>
                update(
                  "holidays",
                  e.target.value
                    .split(",")
                    .map((x) => x.trim())
                    .filter(Boolean),
                )
              }
            />
            <small>YYYY-MM-DD，用英文逗号分隔</small>
          </label>
          <label>
            印花税征收方向
            <Select<string>
              value={m.stampSide}
              onValueChange={(value) => update("stampSide", value ?? "")}
              aria-label="印花税征收方向"
              items={[
                { value: "sell", label: "卖出 / 卖空" },
                { value: "both", label: "买卖双方" },
              ]}
            />
          </label>
          {marketFields.map((f) => (
            <label key={f.key}>
              {f.label.replace("（ppm）", "（%）")}
              <Input
                aria-label={f.label.replace("（ppm）", "（%）")}
                type="number"
                min={0}
                step={
                  f.money
                    ? ".01"
                    : String(f.key).endsWith("PPM")
                      ? "0.0001"
                      : "1"
                }
                value={
                  Number(m[f.key]) /
                  (f.money ? 100 : String(f.key).endsWith("PPM") ? 10000 : 1)
                }
                onChange={(e) =>
                  update(
                    f.key,
                    Math.round(
                      Number(e.target.value) *
                        (f.money
                          ? 100
                          : String(f.key).endsWith("PPM")
                            ? 10000
                            : 1),
                    ),
                  )
                }
              />
              <small>
                {String(f.key).endsWith("PPM") ? "按百分比输入" : f.hint}
              </small>
            </label>
          ))}
          <label className="check">
            <Checkbox
              checked={m.weekdaysOnly}
              onCheckedChange={(value) => update("weekdaysOnly", value)}
            />
            周末休市
          </label>
        </div>
      </LayerCard>
      <LayerCard className="panel padded rules-summary">
        <h2>月赛与退市</h2>
        <p className="muted">
          月份与赛季统一使用上海时间；已建立的本月窗口保持不变，日期变更从后续月份生效。退市和结算由后端执行。
        </p>
        <div className="admin-fields">
          <label>
            每月开赛日
            <Input
              type="number"
              min={1}
              max={20}
              value={s.startDay}
              onChange={(e) =>
                onChange({ ...s, startDay: Number(e.target.value) })
              }
            />
          </label>
          <label>
            截止距月末天数
            <Input
              type="number"
              min={0}
              max={7}
              value={s.endDaysFromLast}
              onChange={(e) =>
                onChange({ ...s, endDaysFromLast: Number(e.target.value) })
              }
            />
            <small>倒数第 5 日填 4</small>
          </label>
          <label>
            退市行动力阈值
            <Input
              type="number"
              min={0}
              max={1000000}
              value={s.delistThreshold}
              onChange={(e) =>
                onChange({ ...s, delistThreshold: Number(e.target.value) })
              }
            />
            <small>0 关闭；非历史补传的实际行情触发</small>
          </label>
        </div>
      </LayerCard>
    </>
  );
}
export function Admin() {
  const [authenticated, setAuthenticated] = useState(false),
    [loading, setLoading] = useState(true),
    [password, setPassword] = useState(""),
    [remember, setRemember] = useState(false),
    [settings, setSettings] = useState<Settings>(),
    [stocks, setStocks] = useState<Stock[]>([]),
    [accounts, setAccounts] = useState<Account[]>([]),
    [section, setSection] = useState("settings"),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  async function loadEntities() {
    const [st, a] = await Promise.all([
      api<{ stocks: Stock[] }>("/admin/stocks"),
      api<{ accounts: Account[] }>("/admin/accounts"),
    ]);
    setStocks(st.stocks);
    setAccounts(a.accounts);
  }
  async function load() {
    const [s, st, a] = await Promise.all([
      api<Settings>("/admin/settings"),
      api<{ stocks: Stock[] }>("/admin/stocks"),
      api<{ accounts: Account[] }>("/admin/accounts"),
    ]);
    setSettings(s);
    setStocks(st.stocks);
    setAccounts(a.accounts);
  }
  useEffect(() => {
    void api("/admin/me")
      .then(() => {
        setAuthenticated(true);
        return load();
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    const lost = (event: Event) => {
      if ((event as CustomEvent).detail === "admin") setAuthenticated(false);
    };
    window.addEventListener("mmex-auth-lost", lost);
    return () => window.removeEventListener("mmex-auth-lost", lost);
  }, []);
  useEffect(() => {
    if (!authenticated) return;
    let active = true,
      busy = false,
      queued = false;
    const reload = async () => {
      queued = true;
      if (busy) return;
      busy = true;
      try {
        do {
          queued = false;
          const [st, a] = await Promise.all([
            api<{ stocks: Stock[] }>("/admin/stocks"),
            api<{ accounts: Account[] }>("/admin/accounts"),
          ]);
          if (active) {
            setStocks(st.stocks);
            setAccounts(a.accounts);
          }
        } while (active && queued);
      } catch (e) {
        if (active) setError((e as Error).message);
      } finally {
        busy = false;
      }
    };
    const events = new EventSource("/api/v2/public/events");
    events.onopen = () => void reload();
    events.addEventListener("change", () => void reload());
    return () => {
      active = false;
      events.close();
    };
  }, [authenticated]);
  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/admin/login", { password, remember });
      setPassword("");
      setAuthenticated(true);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function run(action: () => Promise<void>, success: string) {
    setError("");
    setMessage("");
    setBusy(true);
    try {
      await action();
      setMessage(success);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  if (loading)
    return (
      <div className="startup">
        <ShieldCheck size={32} />
        <p>正在检查管理会话…</p>
      </div>
    );
  if (!authenticated)
    return (
      <div className="admin-login">
        <LayerCard className="panel">
          <a className="brand" href="/">
            <span className="brand-mark">
              <Cat size={26} />
            </span>
            <span>
              <strong>茗喵证券交易所</strong>
              <small>ADMINISTRATION</small>
            </span>
          </a>
          <h1>管理员登录</h1>
          <p className="muted">使用服务器配置的管理员密码。</p>
          <form className="form" onSubmit={login}>
            <label>
              管理员密码
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="current-password"
              />
            </label>
            <label className="check">
              <Checkbox
                checked={remember}
                onCheckedChange={(value) => setRemember(value)}
              />
              记住我（30 天）
            </label>
            {error && (
              <Banner variant="error" role="alert">
                {error}
              </Banner>
            )}
            <Button
              type="submit"
              variant="primary"
              className="primary wide"
              disabled={busy}
            >
              {busy ? "登录中…" : "登录控制台"}
            </Button>
            <a href="/">
              <ArrowLeft size={14} />
              返回交易所
            </a>
          </form>
        </LayerCard>
      </div>
    );
  return (
    <div className="admin-app">
      <header className="topbar">
        <a className="brand" href="/">
          <span className="brand-mark">
            <Cat size={24} />
          </span>
          <span>
            <strong>茗交所 · 管理控制台</strong>
            <small>管理空间</small>
          </span>
        </a>
        <ThemeToggle />
        <Button
          onClick={() =>
            void run(async () => {
              await api("/admin/logout", {});
              setAuthenticated(false);
            }, "")
          }
        >
          <LogOut size={16} />
          退出管理
        </Button>
      </header>
      <main>
        <div className="page-heading">
          <div>
            <div className="eyebrow">管理空间</div>
            <h1>交易所管理</h1>
            <p>调整规则、股票库存与账户状态。所有修改由服务器验证。</p>
          </div>
          <Button disabled={busy} onClick={() => void run(load, "已刷新")}>
            <RefreshCw size={15} />
            刷新
          </Button>
        </div>
        <nav className="admin-tabs" aria-label="管理功能">
          {[
            ["settings", "交易规则"],
            ["stocks", "股票管理"],
            ["accounts", "交易账户"],
          ].map(([id, label]) => (
            <Button
              variant={section === id ? "secondary" : "ghost"}
              className={section === id ? "active" : ""}
              key={id}
              onClick={() => {
                setSection(id);
                setMessage("");
                setError("");
              }}
            >
              {label}
            </Button>
          ))}
        </nav>
        {error && (
          <Banner variant="error" role="alert">
            {error}
          </Banner>
        )}
        {message && (
          <Banner variant="default" role="status">
            {message}
          </Banner>
        )}
        {section === "settings" && settings && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void run(async () => {
                setSettings(
                  await api<Settings>("/admin/settings", settings, "PUT"),
                );
              }, "交易规则已保存并生效");
            }}
          >
            <div className="settings-savebar">
              <div>
                <strong>当前草稿 · {settings.activeMarket?.name}</strong>
                <p>
                  切换市场预设后保存，会取消未成交余量并释放冻结资金；旧交易预览需重新生成。已成交部分保留。
                </p>
              </div>
              <Button
                type="submit"
                variant="primary"
                className="primary"
                disabled={busy}
              >
                <Save size={15} />
                保存交易规则
              </Button>
            </div>
            <MarketEditor settings={settings} onChange={setSettings} />
            <LayerCard className="panel padded">
              <div className="panel-heading">
                <h2>茗喵新默认与公共数量参数</h2>
                <span className="tiny-label">保存时统一生效</span>
              </div>
              <div className="admin-fields">
                {fields.map((f) => (
                  <label key={f.key}>
                    {f.label.replace("（ppm）", "（%）")}
                    <Input
                      aria-label={f.label.replace("（ppm）", "（%）")}
                      type="number"
                      required
                      min={0}
                      step={
                        f.money
                          ? ".01"
                          : String(f.key).endsWith("PPM")
                            ? "0.0001"
                            : "1"
                      }
                      value={
                        Number(settings[f.key]) /
                        (f.money
                          ? 100
                          : String(f.key).endsWith("PPM")
                            ? 10000
                            : 1)
                      }
                      onChange={(e) => {
                        const value = e.target.value;
                        setSettings({
                          ...settings,
                          [f.key]: Math.round(
                            Number(value) *
                              (f.money
                                ? 100
                                : String(f.key).endsWith("PPM")
                                  ? 10000
                                  : 1),
                          ),
                        });
                      }}
                    />
                    <small>
                      {String(f.key).endsWith("PPM") ? "按百分比输入" : f.hint}
                    </small>
                  </label>
                ))}
                <label className="check">
                  <Checkbox
                    checked={settings.paused}
                    onCheckedChange={(value) =>
                      setSettings({ ...settings, paused: value })
                    }
                  />
                  暂停市场成交
                </label>
              </div>
            </LayerCard>
            <LayerCard className="panel padded rules-summary">
              <h2>总资产税率档位</h2>
              <p className="muted">
                上限使用模拟币；最后一档上限为 0
                代表无上限。费率使用百分比输入。按成交前总资产档位对整笔成交额征税。
              </p>
              <div className="table-scroll">
                <Table className="rules-table">
                  <Table.Header>
                    <Table.Row>
                      <Table.Head>档位</Table.Head>
                      <Table.Head>总资产上限（模拟币）</Table.Head>
                      <Table.Head>税率（%）</Table.Head>
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {settings.taxBrackets.map((b, i) => (
                      <Table.Row key={i}>
                        <Table.Cell>{i + 1}</Table.Cell>
                        <Table.Cell>
                          <Input
                            aria-label={`第${i + 1}档资产上限`}
                            type="number"
                            min={0}
                            step=".01"
                            value={b.upTo / 100}
                            onChange={(e) =>
                              setSettings({
                                ...settings,
                                taxBrackets: settings.taxBrackets.map((x, j) =>
                                  j === i
                                    ? {
                                        ...x,
                                        upTo: Math.round(
                                          Number(e.target.value) * 100,
                                        ),
                                      }
                                    : x,
                                ),
                              })
                            }
                          />
                        </Table.Cell>
                        <Table.Cell>
                          <Input
                            aria-label={`第${i + 1}档税率`}
                            type="number"
                            min={0}
                            max={100}
                            step={0.0001}
                            value={b.ratePPM / 10000}
                            onChange={(e) =>
                              setSettings({
                                ...settings,
                                taxBrackets: settings.taxBrackets.map((x, j) =>
                                  j === i
                                    ? {
                                        ...x,
                                        ratePPM: Math.round(
                                          Number(e.target.value) * 10000,
                                        ),
                                      }
                                    : x,
                                ),
                              })
                            }
                          />
                        </Table.Cell>
                      </Table.Row>
                    ))}
                  </Table.Body>
                </Table>
              </div>
            </LayerCard>
          </form>
        )}
        {section === "stocks" && (
          <StockAdmin
            operationError={error}
            stocks={stocks}
            load={loadEntities}
            run={run}
            busy={busy}
          />
        )}
        {section === "accounts" && (
          <AccountAdmin
            operationError={error}
            accounts={accounts}
            stocks={stocks}
            load={loadEntities}
            run={run}
            busy={busy}
          />
        )}
        <footer>
          <span>茗喵证券交易所 · 管理控制台</span>
          <span>
            {number(accounts.length)} 个交易账户 / {number(stocks.length)}{" "}
            支股票
          </span>
          <a href="/">返回市场</a>
        </footer>
      </main>
    </div>
  );
}
