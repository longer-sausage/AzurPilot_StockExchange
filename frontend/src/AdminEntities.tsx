"use client";
import { Collapsible } from "@cloudflare/kumo";
import {
  Banner,
  Button,
  Checkbox,
  Dialog,
  Empty,
  Input,
  LayerCard,
  Select,
  Table,
} from "@cloudflare/kumo";
import { XIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { api, cents, datetime, money } from "./api";
import { Portfolio } from "./Portfolio";
import type { Account, Portfolio as Data, Stock } from "./types";
type Run = (action: () => Promise<void>, success: string) => Promise<void>;
export function StockAdmin({
  stocks,
  load,
  run,
  busy,
  operationError,
}: {
  operationError?: string;
  stocks: Stock[];
  load: () => Promise<void>;
  run: Run;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<Stock>(),
    [deleting, setDeleting] = useState<Stock>(),
    [confirmation, setConfirmation] = useState(""),
    [query, setQuery] = useState(""),
    [showDeleted, setShowDeleted] = useState(false);
  return (
    <LayerCard className="panel">
      <div className="panel-heading">
        <h2>股票管理</h2>
        <Input
          aria-label="搜索管理股票"
          placeholder="名称、代码、实例 UUID"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <label>
          <Checkbox
            checked={showDeleted}
            onCheckedChange={(value) => setShowDeleted(value)}
          />
          显示已删除
        </label>
      </div>
      <div className="table-scroll">
        <Table>
          <Table.Header>
            <Table.Row>
              <Table.Head>股票 / 实例</Table.Head>
              <Table.Head>状态</Table.Head>
              <Table.Head>报价 / 上传</Table.Head>
              <Table.Head>发行 / 借券</Table.Head>
              <Table.Head>操作</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {stocks
              .filter(
                (s) =>
                  (showDeleted || !s.deletedAt) &&
                  `${s.name} ${s.symbol} ${s.instanceId}`
                    .toLowerCase()
                    .includes(query.toLowerCase()),
              )
              .map((s) => (
                <Table.Row key={s.id}>
                  <Table.Cell>
                    <strong>{s.name}</strong>
                    <small>
                      {s.symbol} · {s.instanceId}
                    </small>
                  </Table.Cell>
                  <Table.Cell>
                    {s.deletedAt
                      ? "已删除"
                      : s.disabled
                        ? "停牌"
                        : s.delisted
                          ? "本月退市"
                          : s.accountId
                            ? "已绑定"
                            : "未绑定"}
                  </Table.Cell>
                  <Table.Cell>
                    {money(s.price)}
                    <small>{datetime(s.observedAt)}</small>
                  </Table.Cell>
                  <Table.Cell>
                    {s.issuedShares} / {s.borrowShares}
                  </Table.Cell>
                  <Table.Cell>
                    {!s.deletedAt && (
                      <>
                        <Button
                          disabled={busy}
                          onClick={() => setEditing({ ...s })}
                        >
                          配置 / 资料
                        </Button>
                        <Button
                          disabled={busy}
                          onClick={() =>
                            void run(
                              async () => {
                                await api(`/admin/stocks/${s.id}/status`, {
                                  disabled: !s.disabled,
                                });
                                await load();
                              },
                              s.disabled ? "已恢复交易" : "已停牌并撤销挂单",
                            )
                          }
                        >
                          {s.disabled ? "恢复" : "停牌"}
                        </Button>
                        <Button
                          variant="secondary-destructive"
                          className="danger"
                          disabled={busy}
                          onClick={() => {
                            setDeleting(s);
                            setConfirmation("");
                          }}
                        >
                          删除股票
                        </Button>
                      </>
                    )}
                  </Table.Cell>
                </Table.Row>
              ))}
          </Table.Body>
        </Table>
      </div>
      <Dialog.Root
        open={!!editing}
        onOpenChange={(open) => {
          if (!open) (busy ? undefined : () => setEditing(undefined))?.();
        }}
      >
        <Dialog size="lg" className="exchange-dialog">
          {editing && (
            <>
              <div className="dialog-heading">
                <Dialog.Title>{`股票配置 ${editing.name}`}</Dialog.Title>
                <Button
                  variant="ghost"
                  shape="square"
                  aria-label="关闭"
                  disabled={!(busy ? undefined : () => setEditing(undefined))}
                  onClick={() =>
                    (busy ? undefined : () => setEditing(undefined))?.()
                  }
                >
                  <XIcon size={20} />
                </Button>
              </div>
              <div className="dialog-body">
                <>
                  {operationError && (
                    <Banner variant="error" role="alert">
                      {operationError}
                    </Banner>
                  )}
                </>
                <form
                  className="form"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void run(async () => {
                      await api(
                        `/admin/stocks/${editing.id}`,
                        Object.fromEntries(
                          [
                            "issuedShares",
                            "hourlyVolume",
                            "borrowShares",
                            "borrowHourlyVolume",
                          ].map((k) => [k, editing[k as keyof Stock]]),
                        ),
                        "PUT",
                      );
                      setEditing(undefined);
                      await load();
                    }, "股票配置已保存");
                  }}
                >
                  {(
                    [
                      "issuedShares",
                      "hourlyVolume",
                      "borrowShares",
                      "borrowHourlyVolume",
                    ] as const
                  ).map((key, i) => (
                    <label key={key}>
                      {
                        [
                          "发行股数",
                          "普通小时额度",
                          "借券股数",
                          "融券小时额度",
                        ][i]
                      }
                      <Input
                        type="number"
                        min={1}
                        step={1}
                        value={editing[key]}
                        onChange={(e) =>
                          setEditing({
                            ...editing,
                            [key]: Number(e.target.value),
                          })
                        }
                      />
                    </label>
                  ))}
                  <Collapsible.Root>
                    <Collapsible.DefaultTrigger>
                      实例绑定与公钥资料
                    </Collapsible.DefaultTrigger>
                    <Collapsible.Panel>
                      <p className="break-all">
                        实例 UUID：{editing.instanceId}
                      </p>
                      <p className="break-all">
                        Ed25519 公钥：{editing.publicKey}
                      </p>
                      <p className="break-all">
                        绑定码：{editing.bindingCode || "已删除"}
                      </p>
                      <p>绑定交易账户：{editing.accountId || "未绑定"}</p>
                      <p>上传时间：{datetime(editing.uploadedAt)}</p>
                    </Collapsible.Panel>
                  </Collapsible.Root>
                  <Button
                    type="submit"
                    variant="primary"
                    className="primary"
                    disabled={busy}
                  >
                    保存股票配置
                  </Button>
                </form>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) (busy ? undefined : () => setDeleting(undefined))?.();
        }}
      >
        <Dialog size="lg" className="exchange-dialog">
          {deleting && (
            <>
              <div className="dialog-heading">
                <Dialog.Title>{`确认删除股票 ${deleting.name}`}</Dialog.Title>
                <Button
                  variant="ghost"
                  shape="square"
                  aria-label="关闭"
                  disabled={!(busy ? undefined : () => setDeleting(undefined))}
                  onClick={() =>
                    (busy ? undefined : () => setDeleting(undefined))?.()
                  }
                >
                  <XIcon size={20} />
                </Button>
              </div>
              <div className="dialog-body">
                {operationError && (
                  <Banner variant="error" role="alert">
                    {operationError}
                  </Banner>
                )}
                <p>
                  按最后报价结清该股票所有多空持仓、撤销相关挂单并解除交易账户绑定。原实例停止上报且不可重新注册；交易账户绑定其他股票后才可继续使用。
                </p>
                <p>账本、逐笔成交和原始历史保留用于核对。此操作不能撤销。</p>
                <label>
                  输入股票代码 {deleting.symbol}
                  <Input
                    aria-label="确认删除股票代码"
                    value={confirmation}
                    onChange={(e) => setConfirmation(e.target.value)}
                  />
                </label>
                <Button
                  variant="secondary-destructive"
                  className="danger"
                  disabled={busy || confirmation !== deleting.symbol}
                  onClick={() =>
                    void run(async () => {
                      await api(
                        `/admin/stocks/${deleting.id}`,
                        { confirmed: true },
                        "DELETE",
                      );
                      setDeleting(undefined);
                      await load();
                    }, "股票已删除，实例上报与账户绑定已撤销")
                  }
                >
                  确认删除并结清
                </Button>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
    </LayerCard>
  );
}
export function AccountAdmin({
  accounts,
  stocks,
  run,
  load,
  busy,
  operationError,
}: {
  operationError?: string;
  accounts: Account[];
  stocks: Stock[];
  run: Run;
  load: () => Promise<void>;
  busy: boolean;
}) {
  const [query, setQuery] = useState(""),
    [status, setStatus] = useState("all"),
    [page, setPage] = useState(1),
    [editing, setEditing] = useState<Account>(),
    [detail, setDetail] = useState<Data>(),
    [username, setUsername] = useState(""),
    [adjustment, setAdjustment] = useState("0"),
    [disabled, setDisabled] = useState(false),
    [confirmDisable, setConfirmDisable] = useState(false),
    [error, setError] = useState(""),
    [reset, setReset] = useState(false),
    [password, setPassword] = useState(""),
    [requestId, setRequestId] = useState("");
  const filtered = accounts.filter(
      (a) =>
        (status === "all" ||
          (status === "disabled" ? a.disabled : !a.disabled)) &&
        `${a.username} ${a.identityCode} ${stocks.find((s) => s.id === a.boundStockId)?.symbol ?? ""} ${stocks.find((s) => s.id === a.boundStockId)?.instanceId ?? ""}`
          .toLowerCase()
          .includes(query.toLowerCase()),
    ),
    pages = Math.max(1, Math.ceil(filtered.length / 20)),
    current = Math.min(page, pages);
  useEffect(() => setPage(1), [query, status]);
  async function open(a: Account) {
    setEditing(a);
    setDetail(undefined);
    setUsername(a.username);
    setAdjustment("0");
    setDisabled(!!a.disabled);
    setConfirmDisable(false);
    setError("");
    setPassword("");
    try {
      setDetail(await api<Data>(`/admin/accounts/${a.id}`));
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <LayerCard className="panel">
      <div className="panel-heading">
        <h2>
          交易账户 · 正常 {accounts.filter((a) => !a.disabled).length} / 停用{" "}
          {accounts.filter((a) => a.disabled).length}
        </h2>
        <Input
          aria-label="搜索交易账户"
          placeholder="用户名、代码、身份码、UUID"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Select<string>
          aria-label="账户状态筛选"
          value={status}
          onValueChange={(value) => setStatus(value ?? "")}
          items={[
            { value: "all", label: "全部账户" },
            { value: "active", label: "正常账户" },
            { value: "disabled", label: "停用账户" },
          ]}
        />
      </div>
      <div className="table-scroll">
        <Table>
          <Table.Header>
            <Table.Row>
              <Table.Head>账号 / 注册时间</Table.Head>
              <Table.Head>现金 / 总资产</Table.Head>
              <Table.Head>绑定股票</Table.Head>
              <Table.Head>状态</Table.Head>
              <Table.Head>操作</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {filtered.slice((current - 1) * 20, current * 20).map((a) => (
              <Table.Row key={a.id}>
                <Table.Cell>
                  {a.username}
                  <small>{datetime(a.createdAt)}</small>
                  <small>
                    持仓 {a.positionCount ?? 0} · 待成交 {a.pendingCount ?? 0}
                  </small>
                </Table.Cell>
                <Table.Cell>
                  {money(a.cash)}
                  <small>{money(a.equity)}</small>
                </Table.Cell>
                <Table.Cell>
                  {stocks.find((s) => s.id === a.boundStockId)?.name ??
                    "未绑定"}
                </Table.Cell>
                <Table.Cell>
                  {a.disabled
                    ? "已停用"
                    : a.forcePasswordChange
                      ? "待改密"
                      : !a.boundStockId
                        ? "待绑定"
                        : "正常"}
                </Table.Cell>
                <Table.Cell>
                  <Button onClick={() => void open(a)}>管理 / 详情</Button>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table>
      </div>
      <div className="pagination">
        <Button disabled={current <= 1} onClick={() => setPage(current - 1)}>
          上一页
        </Button>
        <span>
          第 {current} / {pages} 页 · {filtered.length} 个账户
        </span>
        <Button
          disabled={current >= pages}
          onClick={() => setPage(current + 1)}
        >
          下一页
        </Button>
      </div>
      {!filtered.length && (
        <Empty
          title="暂无符合条件的账户"
          description="在群内向官方机器人发送 注册 <用户名> 完成开户。"
        />
      )}
      <Dialog.Root
        open={!!editing}
        onOpenChange={(open) => {
          if (!open)
            (busy
              ? undefined
              : () => {
                  setEditing(undefined);
                  setPassword("");
                })?.();
        }}
      >
        <Dialog size="lg" className="exchange-dialog">
          {editing && (
            <>
              <div className="dialog-heading">
                <Dialog.Title>{`管理账户 ${editing.username}`}</Dialog.Title>
                <Button
                  variant="ghost"
                  shape="square"
                  aria-label="关闭"
                  disabled={
                    !(busy
                      ? undefined
                      : () => {
                          setEditing(undefined);
                          setPassword("");
                        })
                  }
                  onClick={() =>
                    (busy
                      ? undefined
                      : () => {
                          setEditing(undefined);
                          setPassword("");
                        })?.()
                  }
                >
                  <XIcon size={20} />
                </Button>
              </div>
              <div className="dialog-body">
                <>
                  {operationError && (
                    <Banner variant="error" role="alert">
                      {operationError}
                    </Banner>
                  )}
                </>
                <form
                  className="form"
                  onSubmit={(e) => {
                    e.preventDefault();
                    let value: number;
                    try {
                      const amount = adjustment.trim(),
                        raw = amount.replace(/^[+-]/, "");
                      value = /^0+(?:\.0{1,2})?$/.test(raw) ? 0 : cents(raw);
                      if (amount.startsWith("-")) value = -value;
                    } catch (e) {
                      setError((e as Error).message);
                      return;
                    }
                    void run(async () => {
                      await api(
                        `/admin/accounts/${editing.id}`,
                        {
                          username,
                          cashAdjustment: value,
                          expectedCash: editing.cash,
                          disabled,
                          confirmDisable,
                        },
                        "PUT",
                      );
                      setEditing(undefined);
                      await load();
                    }, "账户修改已保存");
                  }}
                >
                  <label>
                    用户名
                    <Input
                      required
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                    />
                  </label>
                  <label>
                    现金增量（模拟币，可为负数）
                    <Input
                      inputMode="decimal"
                      value={adjustment}
                      onChange={(e) => setAdjustment(e.target.value)}
                    />
                  </label>
                  <p className="muted">
                    当前现金 {money(editing.cash)}
                    。保留赛季起始本金；余额变化时拒绝覆盖。
                  </p>
                  <label className="check">
                    <Checkbox
                      checked={disabled}
                      onCheckedChange={(value) => setDisabled(value)}
                    />
                    停用账户
                  </label>
                  {disabled && !editing.disabled && (
                    <label className="check">
                      <Checkbox
                        checked={confirmDisable}
                        onCheckedChange={(value) => setConfirmDisable(value)}
                      />
                      确认撤销挂单、结清本人持仓并撤销会话
                    </label>
                  )}
                  {error && (
                    <Banner variant="error" role="alert">
                      {error}
                    </Banner>
                  )}
                  <Button
                    type="submit"
                    variant="primary"
                    className="primary"
                    disabled={
                      busy || (disabled && !editing.disabled && !confirmDisable)
                    }
                  >
                    保存账户修改
                  </Button>
                  <Button
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setReset(true);
                      setRequestId(crypto.randomUUID());
                    }}
                  >
                    重置该账户密码
                  </Button>
                </form>
                {password && (
                  <Banner variant="default">
                    <p>
                      随机初始密码（仅当前展示）：
                      <code className="break-all">{password}</code>
                    </p>
                    <p>已撤销旧会话，下次登录必须改密。</p>
                  </Banner>
                )}
                <Collapsible.Root>
                  <Collapsible.DefaultTrigger>
                    完整账户、持仓、交收与成交
                  </Collapsible.DefaultTrigger>
                  <Collapsible.Panel>
                    {detail && (
                      <Portfolio
                        readOnly
                        data={detail}
                        stocks={stocks}
                        onSelect={(id) => {
                          window.open(
                            `/?stock=${id}`,
                            "_blank",
                            "noopener,noreferrer",
                          );
                        }}
                        onRefresh={async () =>
                          setDetail(
                            await api<Data>(`/admin/accounts/${editing.id}`),
                          )
                        }
                        onError={setError}
                      />
                    )}
                  </Collapsible.Panel>
                </Collapsible.Root>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
      <Dialog.Root
        open={!!(reset && editing)}
        onOpenChange={(open) => {
          if (!open) (busy ? undefined : () => setReset(false))?.();
        }}
      >
        <Dialog size="lg" className="exchange-dialog">
          {reset && editing && (
            <>
              <div className="dialog-heading">
                <Dialog.Title>确认管理员重置密码</Dialog.Title>
                <Button
                  variant="ghost"
                  shape="square"
                  aria-label="关闭"
                  disabled={!(busy ? undefined : () => setReset(false))}
                  onClick={() => (busy ? undefined : () => setReset(false))?.()}
                >
                  <XIcon size={20} />
                </Button>
              </div>
              <div className="dialog-body">
                {operationError && (
                  <Banner variant="error" role="alert">
                    {operationError}
                  </Banner>
                )}
                <p>
                  旧密码与全部会话立即失效，生成随机强密码，首次登录必须修改。
                </p>
                <Button
                  variant="secondary-destructive"
                  className="danger"
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      const r = await api<{ password: string }>(
                        `/admin/accounts/${editing.id}/reset-password`,
                        { confirmed: true, requestId },
                      );
                      setPassword(r.password);
                      setReset(false);
                    }, "密码已重置")
                  }
                >
                  确认重置密码
                </Button>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
    </LayerCard>
  );
}
