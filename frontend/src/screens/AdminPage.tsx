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
  if (checking) return <Empty title="正在验证管理员身份..." />;
  if (!authenticated)
    return (
      <LayerCard className="admin-login card-body">
        <p className="eyebrow">交易所管理</p>
        <h1>管理员登录</h1>
        <p className="muted">
          请输入服务器配置的管理员管理密码。管理员权限与普通交易会话相互隔离。
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
            label="记住登录状态（30 天）"
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
          <p>集中管理市场规则、交易账户与挂牌证券，所有操作均由服务端实时校验并安全落账。</p>
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
            退出管理后台
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
            {!listed.length && <Empty title="未找到符合条件的证券" />}
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
            {!people.length && <Empty title="未找到符合条件的交易账户" />}
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
                description="删除后，系统将按最后报价结清该股票的所有多空持仓、撤销未成交挂单，并解除与交易账户的绑定关系；原 AzurPilot 实例将永久停止上报且不可再次注册。历史账本记录将完整保留以供追溯。"
              />
              <Input
                label="确认股票代码"
                description={`为避免误操作，请输入股票代码「${stockDelete.symbol}」以确认删除。`}
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
                    description={`当前现金余额：${money(accountEdit.cash)}。输入正数增加、负数扣减（不会扣减赛季初始本金）；若保存时账户余额发生并发变动，系统将自动拦截以防覆盖。`}
                  />
                  <Checkbox
                    label="停用账户"
                    checked={disabled}
                    onCheckedChange={setDisabled}
                  />
                  {disabled && !accountEdit.disabled && (
                    <Checkbox
                      label="已知晓后果：撤销该账户所有活动委托、结清名下多空持仓并强制退出登录"
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
                  title="临时初始密码（仅当前显示一次，请妥善保存）"
                  description={
                    <>
                      <ClipboardText text={temporary} />
                      <p>原登录会话已全部撤销，玩家下次登录时必须修改为自己的新密码。</p>
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
            重置后原密码与所有在线登录会话将立即失效。系统会生成一个随机强密码，玩家首次登录时必须设置新密码。
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
