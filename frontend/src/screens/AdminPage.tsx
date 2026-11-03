"use client";
// 管理按规则、账户、证券分组；危险操作使用独立 Kumo Dialog 确认。
import { useCallback, useEffect, useState } from "react";
import {
  Badge,
  Banner,
  Button,
  Checkbox,
  ClipboardText,
  Dialog,
  Empty,
  Input,
  InputGroup,
  LayerCard,
  Select,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import {
  MagnifyingGlassIcon,
  XIcon,
  ArrowClockwiseIcon,
} from "@phosphor-icons/react";
import type { Account, Ledger, Settings, Stock } from "../core/contracts";
import { request } from "../core/http";
import { cents, date, money } from "../core/format";
import { useExchange, useNotice } from "../core/Exchange";
import { SettingsEditor } from "../features/SettingsEditor";
import { AccountBook } from "./PortfolioPage";
import { OrderRecords } from "../features/Records";
export function AdminPage() {
  const x = useExchange(),
    notice = useNotice();
  const [authenticated, setAuthenticated] = useState(false),
    [checking, setChecking] = useState(true),
    [password, setPassword] = useState(""),
    [remember, setRemember] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [tab, setTab] = useState("settings"),
    [settings, setSettings] = useState<Settings>(),
    [stocks, setStocks] = useState<Stock[]>([]),
    [accounts, setAccounts] = useState<Account[]>([]),
    [query, setQuery] = useState(""),
    [accountState, setAccountState] = useState("all"),
    [showDeleted, setShowDeleted] = useState(false),
    [page, setPage] = useState(1);
  const [stockEdit, setStockEdit] = useState<Stock>(),
    [stockDelete, setStockDelete] = useState<Stock>(),
    [confirmation, setConfirmation] = useState(""),
    [accountEdit, setAccountEdit] = useState<Account>(),
    [accountDetail, setAccountDetail] = useState<Ledger>(),
    [accountView, setAccountView] = useState("edit"),
    [username, setUsername] = useState(""),
    [adjustment, setAdjustment] = useState("0"),
    [disabled, setDisabled] = useState(false),
    [confirmDisable, setConfirmDisable] = useState(false),
    [resetOpen, setResetOpen] = useState(false),
    [resetId, setResetId] = useState(""),
    [temporary, setTemporary] = useState("");
  const load = useCallback(async () => {
    const [s, a, t] = await Promise.all([
      request<Settings>("/admin/settings"),
      request<{ accounts: Account[] }>("/admin/accounts"),
      request<{ stocks: Stock[] }>("/admin/stocks"),
    ]);
    setSettings(s);
    setAccounts(a.accounts);
    setStocks(t.stocks);
  }, []);
  useEffect(() => {
    let active = true;
    void request("/admin/me")
      .then(async () => {
        if (active) {
          setAuthenticated(true);
          await load();
        }
      })
      .catch(() => {})
      .finally(() => {
        if (active) setChecking(false);
      });
    return () => {
      active = false;
    };
  }, [load]);
  useEffect(() => {
    const lost = (e: Event) => {
      if ((e as CustomEvent).detail === "admin") {
        setAuthenticated(false);
        setTemporary("");
        setAccountEdit(undefined);
        setResetOpen(false);
      }
    };
    window.addEventListener("exchange:unauthorized", lost);
    return () => window.removeEventListener("exchange:unauthorized", lost);
  }, []);
  useEffect(() => {
    if (!authenticated) return;
    const events = new EventSource("/api/v2/public/events");
    let active = true,
      running = false,
      queued = false;
    const update = async () => {
      queued = true;
      if (running) return;
      running = true;
      try {
        do {
          queued = false;
          const [a, s] = await Promise.all([
            request<{ accounts: Account[] }>("/admin/accounts"),
            request<{ stocks: Stock[] }>("/admin/stocks"),
          ]);
          if (active) {
            setAccounts(a.accounts);
            setStocks(s.stocks);
          }
        } while (active && queued);
      } catch (e) {
        if (active) setError((e as Error).message);
      } finally {
        running = false;
      }
    };
    events.addEventListener("change", () => void update());
    events.onopen = () => void update();
    return () => {
      active = false;
      events.close();
    };
  }, [authenticated]);
  async function act(operation: () => Promise<void>, message = "操作已保存") {
    setBusy(true);
    setError("");
    try {
      await operation();
      await load();
      await x.refresh();
      if (message) notice(message);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function login() {
    setBusy(true);
    setError("");
    try {
      await request("/admin/login", { password, remember });
      setPassword("");
      setAuthenticated(true);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function openAccount(a: Account) {
    setAccountEdit(a);
    setAccountView("edit");
    setUsername(a.username);
    setAdjustment("0");
    setDisabled(!!a.disabled);
    setConfirmDisable(false);
    setTemporary("");
    setAccountDetail(undefined);
    setError("");
    try {
      const result = await request<Ledger>(`/admin/accounts/${a.id}`);
      setAccountDetail(result);
      setAccountEdit(result.account);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  function closeAccount() {
    if (busy) return;
    setAccountEdit(undefined);
    setTemporary("");
    setAccountDetail(undefined);
    setResetOpen(false);
  }
  const listed = stocks.filter(
    (s) =>
      (showDeleted || !s.deletedAt) &&
      `${s.name} ${s.symbol} ${s.instanceId}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  const people = accounts.filter(
    (a) =>
      `${a.username} ${a.id} ${a.identityCode} ${stocks.find((s) => s.id === a.boundStockId)?.instanceId ?? ""}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (accountState === "all" ||
        (accountState === "active" && !a.disabled) ||
        (accountState === "disabled" && a.disabled)),
  );
  const total = Math.max(1, Math.ceil(people.length / 20)),
    current = Math.min(page, total);
  if (checking) return <Empty title="正在检查管理员会话" />;
  if (!authenticated)
    return (
      <LayerCard className="admin-login card-body">
        <p className="eyebrow">交易所管理</p>
        <h1>管理员登录</h1>
        <p className="muted">
          使用服务器配置的管理员密码。管理员与玩家会话相互独立。
        </p>
        <form
          className="form-stack"
          onSubmit={(e) => {
            e.preventDefault();
            void login();
          }}
        >
          <Input
            label="管理员密码"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          <Checkbox
            label="记住我（30 天）"
            checked={remember}
            onCheckedChange={setRemember}
          />
          {error && <Banner variant="error" role="alert" description={error} />}
          <Button type="submit" variant="primary" loading={busy}>
            登录控制台
          </Button>
        </form>
      </LayerCard>
    );
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">运营管理</p>
          <h1>交易所管理</h1>
          <p>规则、账户与证券，统一由服务器验证和落账。</p>
        </div>
        <div className="toolbar-group">
          <Button
            variant="secondary"
            onClick={() => void act(load, "数据已刷新")}
            disabled={busy}
          >
            <ArrowClockwiseIcon size={18} />
            刷新
          </Button>
          <Button
            variant="ghost"
            onClick={() =>
              void act(async () => {
                await request("/admin/logout", {});
                setAuthenticated(false);
                setTemporary("");
              }, "")
            }
          >
            退出管理
          </Button>
        </div>
      </div>
      <Tabs
        variant="underline"
        value={tab}
        onValueChange={(v) => {
          setTab(v);
          setQuery("");
          setError("");
          setPage(1);
        }}
        tabs={[
          { value: "settings", label: "市场配置" },
          { value: "accounts", label: `交易账户 ${accounts.length}` },
          {
            value: "stocks",
            label: `证券管理 ${stocks.filter((s) => !s.deletedAt).length}`,
          },
        ]}
      />
      {error && <Banner variant="error" role="alert" description={error} />}
      {tab === "settings" && settings && (
        <SettingsEditor
          value={settings}
          onChange={setSettings}
          busy={busy}
          onSave={() =>
            void act(async () => {
              setSettings(
                await request<Settings>("/admin/settings", settings, "PUT"),
              );
            }, "交易规则已保存")
          }
        />
      )}
      {tab === "stocks" && (
        <>
          <div className="toolbar">
            <div className="toolbar-search">
              <InputGroup>
                <InputGroup.Addon>
                  <MagnifyingGlassIcon size={18} />
                </InputGroup.Addon>
                <InputGroup.Input
                  aria-label="搜索管理证券"
                  placeholder="名称、代码、实例 UUID"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </InputGroup>
            </div>
            <Checkbox
              label="显示已删除证券"
              checked={showDeleted}
              onCheckedChange={setShowDeleted}
            />
          </div>
          <LayerCard className="data-card">
            <div className="table-region">
              <Table className="min-w-[940px]">
                <Table.Header>
                  <Table.Row>
                    {["证券", "状态", "报价 / 上传", "发行 / 借券", "操作"].map(
                      (label) => (
                        <Table.Head key={label}>{label}</Table.Head>
                      ),
                    )}
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {listed.map((s) => (
                    <Table.Row key={s.id}>
                      <Table.Cell>
                        <strong>{s.name}</strong>
                        <p className="caption">{s.symbol}</p>
                        <p className="caption">{s.instanceId}</p>
                      </Table.Cell>
                      <Table.Cell>
                        <Badge variant="secondary">
                          {s.deletedAt
                            ? "已删除"
                            : s.disabled
                              ? "停牌"
                              : s.delisted
                                ? "本月退市"
                                : s.accountId
                                  ? "已绑定"
                                  : "未绑定"}
                        </Badge>
                      </Table.Cell>
                      <Table.Cell className="numeric">
                        {money(s.price)}
                        <p className="caption">{date(s.observedAt)}</p>
                      </Table.Cell>
                      <Table.Cell>
                        {s.issuedShares} / {s.borrowShares}
                      </Table.Cell>
                      <Table.Cell>
                        {!s.deletedAt && (
                          <div className="toolbar-group">
                            <Button
                              disabled={busy}
                              onClick={() => {
                                setStockEdit({ ...s });
                                setError("");
                              }}
                            >
                              配置资料
                            </Button>
                            <Button
                              disabled={busy}
                              onClick={() =>
                                void act(
                                  async () => {
                                    await request(
                                      `/admin/stocks/${s.id}/status`,
                                      { disabled: !s.disabled },
                                    );
                                  },
                                  s.disabled ? "已恢复交易" : "已停牌并撤单",
                                )
                              }
                            >
                              {s.disabled ? "恢复交易" : "停牌"}
                            </Button>
                            <Button
                              disabled={busy}
                              variant="secondary-destructive"
                              onClick={() => {
                                setStockDelete(s);
                                setConfirmation("");
                                setError("");
                              }}
                            >
                              删除
                            </Button>
                          </div>
                        )}
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table>
            </div>
            {!listed.length && <Empty title="没有匹配的证券" />}
          </LayerCard>
        </>
      )}
      {tab === "accounts" && (
        <>
          <div className="toolbar">
            <div className="toolbar-search">
              <InputGroup>
                <InputGroup.Addon>
                  <MagnifyingGlassIcon size={18} />
                </InputGroup.Addon>
                <InputGroup.Input
                  aria-label="搜索交易账户"
                  placeholder="用户名、编号、身份码、UUID"
                  value={query}
                  onChange={(e) => {
                    setQuery(e.target.value);
                    setPage(1);
                  }}
                />
              </InputGroup>
            </div>
            <Select
              aria-label="账户状态"
              value={accountState}
              onValueChange={(v) => {
                setAccountState(String(v));
                setPage(1);
              }}
              items={{
                all: "全部账户",
                active: "正常账户",
                disabled: "停用账户",
              }}
            />
          </div>
          <LayerCard className="data-card">
            <div className="table-region">
              <Table className="min-w-[760px]">
                <Table.Header>
                  <Table.Row>
                    {["交易账户", "现金 / 净资产", "绑定证券", "状态", ""].map(
                      (label) => (
                        <Table.Head key={label}>{label}</Table.Head>
                      ),
                    )}
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {people.slice((current - 1) * 20, current * 20).map((a) => (
                    <Table.Row key={a.id}>
                      <Table.Cell>
                        <strong>{a.username}</strong>
                        <p className="caption">
                          #{a.id} · {date(a.createdAt)}
                        </p>
                        <p className="caption">
                          持仓 {a.positionCount ?? 0} · 待成交{" "}
                          {a.pendingCount ?? 0}
                        </p>
                      </Table.Cell>
                      <Table.Cell className="numeric">
                        {money(a.cash)}
                        <p className="caption">净资产 {money(a.equity)}</p>
                      </Table.Cell>
                      <Table.Cell>
                        {stocks.find((s) => s.id === a.boundStockId)?.name ??
                          "未绑定"}
                      </Table.Cell>
                      <Table.Cell>
                        <Badge variant="secondary">
                          {a.disabled
                            ? "已停用"
                            : a.forcePasswordChange
                              ? "待改密"
                              : !a.boundStockId
                                ? "待绑定"
                                : "正常"}
                        </Badge>
                      </Table.Cell>
                      <Table.Cell>
                        <Button onClick={() => void openAccount(a)}>
                          管理账户
                        </Button>
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table>
            </div>
            {!people.length && <Empty title="没有匹配的账户" />}
            <div className="card-heading">
              <span className="caption">
                {people.length} 个账户 · 第 {current} / {total} 页
              </span>
              <div className="toolbar-group">
                <Button
                  disabled={current <= 1}
                  onClick={() => setPage(current - 1)}
                >
                  上一页
                </Button>
                <Button
                  disabled={current >= total}
                  onClick={() => setPage(current + 1)}
                >
                  下一页
                </Button>
              </div>
            </div>
          </LayerCard>
        </>
      )}
      <Dialog.Root
        open={!!stockEdit}
        onOpenChange={(open) => {
          if (!open && !busy) setStockEdit(undefined);
        }}
      >
        <Dialog size="lg" className="dialog-content">
          <div className="dialog-title">
            <Dialog.Title>证券配置</Dialog.Title>
            <Button
              variant="ghost"
              shape="square"
              aria-label="关闭证券配置"
              disabled={busy}
              onClick={() => setStockEdit(undefined)}
            >
              <XIcon size={20} />
            </Button>
          </div>
          {stockEdit && (
            <>
              <Dialog.Description>
                {stockEdit.name} · {stockEdit.symbol}
              </Dialog.Description>
              <form
                className="form-stack"
                onSubmit={(e) => {
                  e.preventDefault();
                  void act(async () => {
                    await request(
                      `/admin/stocks/${stockEdit.id}`,
                      {
                        issuedShares: stockEdit.issuedShares,
                        hourlyVolume: stockEdit.hourlyVolume,
                        borrowShares: stockEdit.borrowShares,
                        borrowHourlyVolume: stockEdit.borrowHourlyVolume,
                      },
                      "PUT",
                    );
                    setStockEdit(undefined);
                  }, "证券配置已保存");
                }}
              >
                <div className="fields">
                  {(
                    [
                      "issuedShares",
                      "hourlyVolume",
                      "borrowShares",
                      "borrowHourlyVolume",
                    ] as const
                  ).map((key, i) => (
                    <Input
                      key={key}
                      label={
                        [
                          "发行股数",
                          "普通小时额度",
                          "借券库存",
                          "融券小时额度",
                        ][i]
                      }
                      type="number"
                      min={1}
                      step={1}
                      value={stockEdit[key]}
                      onChange={(e) =>
                        setStockEdit({
                          ...stockEdit,
                          [key]: Number(e.target.value),
                        })
                      }
                      required
                    />
                  ))}
                </div>
                {error && (
                  <Banner variant="error" role="alert" description={error} />
                )}
                <Button type="submit" variant="primary" loading={busy}>
                  保存证券配置
                </Button>
              </form>
              <dl className="definition-list">
                <div>
                  <dt>实例 UUID</dt>
                  <dd>{stockEdit.instanceId}</dd>
                </div>
                <div>
                  <dt>绑定账户</dt>
                  <dd>{stockEdit.accountId ?? "未绑定"}</dd>
                </div>
              </dl>
              <div className="stack">
                <h3>实例公钥</h3>
                <ClipboardText text={stockEdit.publicKey ?? ""} />
                <h3>绑定码</h3>
                <ClipboardText text={stockEdit.bindingCode ?? ""} />
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={!!stockDelete}
        onOpenChange={(open) => {
          if (!open && !busy) setStockDelete(undefined);
        }}
      >
        <Dialog size="lg" className="dialog-content">
          <Dialog.Title>删除证券并结清持仓</Dialog.Title>
          {stockDelete && (
            <>
              <Dialog.Description>
                {stockDelete.name} · 此操作不能撤销。
              </Dialog.Description>
              <Banner
                variant="alert"
                description="按最后报价结清多空持仓，撤销相关挂单并解除账户绑定。原实例将停止上报且不能重新注册。账本和原始历史保留。"
              />
              <Input
                label="确认股票代码"
                description={`输入 ${stockDelete.symbol} 以确认删除。`}
                value={confirmation}
                onChange={(e) => setConfirmation(e.target.value)}
              />
              {error && (
                <Banner variant="error" role="alert" description={error} />
              )}
              <div className="inline-actions">
                <Button
                  disabled={busy}
                  onClick={() => setStockDelete(undefined)}
                >
                  取消
                </Button>
                <Button
                  variant="destructive"
                  loading={busy}
                  disabled={confirmation !== stockDelete.symbol}
                  onClick={() =>
                    void act(async () => {
                      await request(
                        `/admin/stocks/${stockDelete.id}`,
                        { confirmed: true },
                        "DELETE",
                      );
                      setStockDelete(undefined);
                    }, "证券已删除，持仓已结清")
                  }
                >
                  确认删除并结清
                </Button>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={!!accountEdit}
        onOpenChange={(open) => {
          if (!open) closeAccount();
        }}
      >
        <Dialog size="xl" className="dialog-content">
          <div className="dialog-title">
            <Dialog.Title>账户管理</Dialog.Title>
            <Button
              variant="ghost"
              shape="square"
              aria-label="关闭账户管理"
              disabled={busy}
              onClick={closeAccount}
            >
              <XIcon size={20} />
            </Button>
          </div>
          {accountEdit && (
            <>
              <Dialog.Description>
                {accountEdit.username} · #{accountEdit.id}
              </Dialog.Description>
              <Tabs
                value={accountView}
                onValueChange={setAccountView}
                variant="underline"
                tabs={[
                  { value: "edit", label: "资料与状态" },
                  { value: "assets", label: "资产与持仓" },
                  { value: "orders", label: "完整委托" },
                ]}
              />
              {accountView === "edit" && (
                <form
                  className="form-stack"
                  onSubmit={(e) => {
                    e.preventDefault();
                    let delta: number;
                    try {
                      delta = cents(adjustment, true);
                    } catch (e) {
                      setError((e as Error).message);
                      return;
                    }
                    void act(async () => {
                      await request(
                        `/admin/accounts/${accountEdit.id}`,
                        {
                          username,
                          cashAdjustment: delta,
                          expectedCash: accountEdit.cash,
                          disabled,
                          confirmDisable,
                        },
                        "PUT",
                      );
                      closeAccount();
                      setAccountEdit(undefined);
                      setTemporary("");
                    }, "账户修改已保存");
                  }}
                >
                  <Input
                    label="用户名"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    required
                  />
                  <Input
                    label="现金增量（模拟币）"
                    inputMode="decimal"
                    value={adjustment}
                    onChange={(e) => setAdjustment(e.target.value)}
                    description={`当前现金 ${money(accountEdit.cash)}；允许负数，保留赛季起始本金。余额变化时拒绝覆盖。`}
                  />
                  <Checkbox
                    label="停用账户"
                    checked={disabled}
                    onCheckedChange={setDisabled}
                  />
                  {disabled && !accountEdit.disabled && (
                    <Checkbox
                      label="确认撤单、结清本人持仓并撤销会话"
                      checked={confirmDisable}
                      onCheckedChange={setConfirmDisable}
                    />
                  )}
                  <div className="toolbar-group">
                    <Button
                      type="submit"
                      variant="primary"
                      loading={busy}
                      disabled={
                        disabled && !accountEdit.disabled && !confirmDisable
                      }
                    >
                      保存账户修改
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => {
                        setResetId(crypto.randomUUID());
                        setResetOpen(true);
                      }}
                    >
                      重置密码
                    </Button>
                  </div>
                </form>
              )}
              {accountView === "assets" && accountDetail && (
                <AccountBook data={accountDetail} stocks={stocks} />
              )}{" "}
              {accountView === "orders" && accountDetail && (
                <OrderRecords orders={accountDetail.orders} readOnly />
              )}
              {temporary && (
                <Banner
                  variant="alert"
                  title="随机初始密码，仅当前显示"
                  description={
                    <>
                      <ClipboardText text={temporary} />
                      <p>旧会话已撤销，下次登录强制改密。</p>
                    </>
                  }
                />
              )}{" "}
              {error && (
                <Banner variant="error" role="alert" description={error} />
              )}
            </>
          )}
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={resetOpen}
        onOpenChange={(open) => {
          if (!busy) setResetOpen(open);
        }}
      >
        <Dialog className="dialog-content">
          <Dialog.Title>确认重置账户密码</Dialog.Title>
          <Dialog.Description>
            旧密码与全部会话立即失效，服务器生成随机强密码，首次登录必须修改。
          </Dialog.Description>
          <div className="inline-actions">
            <Button onClick={() => setResetOpen(false)} disabled={busy}>
              取消
            </Button>
            <Button
              variant="destructive"
              loading={busy}
              onClick={() =>
                accountEdit &&
                void act(async () => {
                  const result = await request<{ password: string }>(
                    `/admin/accounts/${accountEdit.id}/reset-password`,
                    { confirmed: true, requestId: resetId },
                  );
                  setTemporary(result.password);
                  setResetOpen(false);
                }, "密码已重置")
              }
            >
              确认重置密码
            </Button>
          </div>
        </Dialog>
      </Dialog.Root>
    </>
  );
}
