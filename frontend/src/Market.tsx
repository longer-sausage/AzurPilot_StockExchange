"use client";
import { Collapsible } from "@cloudflare/kumo";
import {
  Banner,
  Button,
  Dialog,
  Input,
  LayerCard,
  Select,
  Table,
  Badge,
} from "@cloudflare/kumo";
import { XIcon } from "@phosphor-icons/react";
import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowsDownUpIcon as ArrowDownUp,
  CheckIcon as Check,
  CaretRightIcon as ChevronRight,
  ClockIcon as Clock3,
} from "@phosphor-icons/react";
import { api, cents, datetime, money, number, rate, sideName } from "./api";
import { FinancialChart } from "./Charts";
import { change, quoteState } from "./Securities";
import { TradeTable, kindName } from "./Portfolio";
import type { ChartPeriod, StockDetail } from "./chartTypes";
import type {
  Account,
  Order,
  Portfolio,
  Preview,
  Settings,
  Stock,
  Market,
} from "./types";
export function StockPanel({
  stock,
  onDetail,
}: {
  stock: Stock;
  onDetail?: () => void;
}) {
  const [detail, setDetail] = useState<StockDetail>(),
    [period, setPeriod] = useState<ChartPeriod>("time"),
    [month, setMonth] = useState(() =>
      new Date()
        .toLocaleDateString("sv-SE", { timeZone: "Asia/Shanghai" })
        .slice(0, 7),
    ),
    [day, setDay] = useState(""),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [tab, setTab] = useState("trades");
  useEffect(() => {
    let active = true;
    setLoading(true);
    void api<StockDetail>(
      `/public/stocks/${stock.id}/chart-detail?period=${period}&month=${month}&day=${day}`,
    )
      .then((r) => {
        if (active) {
          setDetail(r);
          setError("");
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [
    stock.id,
    stock.historyVersion,
    stock.historyCheckedAt,
    stock.tradeVersion,
    period,
    month,
    day,
  ]);
  const c = change(stock),
    summary = detail?.summary;
  return (
    <LayerCard className="panel quote-panel">
      <div className="quote-top">
        <div>
          <span className="tiny-label">{stock.symbol}</span>
          <h2>
            {stock.name}
            <Badge variant="secondary">
              {stock.delisted
                ? "本月退市"
                : stock.disabled
                  ? "停牌"
                  : stock.bound
                    ? "已绑定"
                    : "独立股票"}
            </Badge>
          </h2>
        </div>
        <span className="tiny-label">
          最新报价 {datetime(stock.observedAt)}
          {onDetail && <Button onClick={onDetail}>行情详情</Button>}
        </span>
      </div>
      <div className="quote-price">
        <strong>{money(stock.price)}</strong>
        <span className={(c ?? 0) >= 0 ? "up" : "down"}>
          {c === undefined ? "—" : `${c >= 0 ? "+" : ""}${c.toFixed(2)}%`}{" "}
          <small>较今日开盘</small>
        </span>
      </div>
      <div className="quote-metrics">
        <div>
          <span>所选时段开 / 高 / 低</span>
          <b>
            {money(summary?.open ?? 0)} / {money(summary?.high ?? 0)} /{" "}
            {money(summary?.low ?? 0)}
          </b>
        </div>
        <div>
          <span>成交量 / 额</span>
          <b>
            {number(summary?.volume ?? 0)} 股 / {money(summary?.turnover ?? 0)}
          </b>
        </div>
      </div>
      <p className="muted">
        上一交易日收盘{" "}
        {summary?.previousClose ? money(summary.previousClose) : "—"}
      </p>
      <FinancialChart
        detail={detail?.stock.id === stock.id ? detail : undefined}
        period={period}
        month={month}
        day={day}
        onPeriod={(p) => {
          setPeriod(p);
          setDay("");
        }}
        onMonth={(m) => {
          setMonth(m);
          setDay("");
        }}
        onDay={setDay}
        loading={loading}
        error={error}
      />
      <div className="quote-metrics">
        <div>
          <span>发行库存 / 剩余</span>
          <b>
            {number(stock.issuedShares)} /{" "}
            {number(stock.issuedShares - stock.outstanding)}
          </b>
        </div>
        <div>
          <span>普通小时量 / 上限</span>
          <b>
            {number(stock.normalUsed)} / {number(stock.hourlyVolume)}
          </b>
        </div>
        <div>
          <span>借券库存 / 剩余</span>
          <b>
            {number(stock.borrowShares)} /{" "}
            {number(stock.borrowShares - stock.borrowed)}
          </b>
        </div>
        <div>
          <span>融券小时量 / 上限</span>
          <b>
            {number(stock.borrowUsed)} / {number(stock.borrowHourlyVolume)}
          </b>
        </div>
        <div>
          <span>买入 / 回补成交</span>
          <b>{number(summary?.buyVolume ?? 0)} 股</b>
        </div>
        <div>
          <span>卖出 / 卖空成交</span>
          <b>{number(summary?.sellVolume ?? 0)} 股</b>
        </div>
      </div>
      <div className="flow-bar" aria-label="成交方向分布">
        <i
          style={{
            width: `${summary?.volume ? (summary.buyVolume / summary.volume) * 100 : 50}%`,
          }}
        />
      </div>
      <div className="list-tabs">
        <Button
          className={tab === "trades" ? "active" : ""}
          onClick={() => setTab("trades")}
        >
          本月逐笔成交
        </Button>
        <Button
          className={tab === "orders" ? "active" : ""}
          onClick={() => setTab("orders")}
        >
          公开待成交委托
        </Button>
        <Button
          className={tab === "source" ? "active" : ""}
          onClick={() => setTab("source")}
        >
          证券资料
        </Button>
      </div>
      {tab === "trades" && (
        <>
          <TradeTable trades={detail?.trades ?? []} />
          {!!detail?.legacyOrders?.length && (
            <Collapsible.Root>
              <Collapsible.DefaultTrigger>
                旧订单成交汇总（不能还原逐笔）
              </Collapsible.DefaultTrigger>
              <Collapsible.Panel>
                <div className="table-scroll">
                  <Table>
                    <Table.Header>
                      <Table.Row>
                        <Table.Head>委托号</Table.Head>
                        <Table.Head>均价</Table.Head>
                        <Table.Head>成交股数</Table.Head>
                        <Table.Head>订单更新时间</Table.Head>
                      </Table.Row>
                    </Table.Header>
                    <Table.Body>
                      {detail.legacyOrders.map((o) => (
                        <Table.Row key={o.id}>
                          <Table.Cell>#{o.id}</Table.Cell>
                          <Table.Cell>{money(o.averagePrice)}</Table.Cell>
                          <Table.Cell>{number(o.filled)}</Table.Cell>
                          <Table.Cell>{datetime(o.updatedAt)}</Table.Cell>
                        </Table.Row>
                      ))}
                    </Table.Body>
                  </Table>
                </div>
              </Collapsible.Panel>
            </Collapsible.Root>
          )}
        </>
      )}{" "}
      {tab === "orders" && (
        <div className="table-scroll">
          <Table>
            <Table.Header>
              <Table.Row>
                <Table.Head>委托 / 玩家</Table.Head>
                <Table.Head>方向 / 类型</Table.Head>
                <Table.Head>限价 / 触发价</Table.Head>
                <Table.Head>剩余股数</Table.Head>
                <Table.Head>有效期</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {detail?.orders?.map((o) => (
                <Table.Row key={o.id}>
                  <Table.Cell>
                    #{o.id} · {o.username}
                  </Table.Cell>
                  <Table.Cell>
                    {sideName[o.side]} · {kindName[o.kind]}
                  </Table.Cell>
                  <Table.Cell>{money(o.price)}</Table.Cell>
                  <Table.Cell>{number(o.remaining)}</Table.Cell>
                  <Table.Cell>
                    {o.timeInForce} ·{" "}
                    {o.expiresAt ? datetime(o.expiresAt) : "本赛季有效"}
                  </Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table>
          {!detail?.orders?.length && <p className="empty">暂无待成交委托。</p>}
        </div>
      )}
      {tab === "source" && (
        <dl className="receipt">
          <div>
            <dt>报价来源</dt>
            <dd>AzurPilot 实例总行动力 · 1 行动力 = 1 模拟币</dd>
          </div>
          <div>
            <dt>最近上传</dt>
            <dd>{datetime(stock.uploadedAt)}</dd>
          </div>
          <div>
            <dt>成交机制</dt>
            <dd>有限发行与借券库存、独立小时量、限价约束及整点优先级</dd>
          </div>
          <div>
            <dt>历史</dt>
            <dd>完整原始报价独立存储；空档不补造数据。</dd>
          </div>
        </dl>
      )}
    </LayerCard>
  );
}
export function Trade({
  stock,
  account,
  settings,
  onLogin,
  onDone,
  portfolio,
  online = true,
  market,
}: {
  stock: Stock;
  account?: Account;
  settings?: Settings;
  onLogin: () => void;
  onDone: (order: Order) => Promise<void>;
  portfolio?: Portfolio;
  online?: boolean;
  market?: Market;
}) {
  const [side, setSide] = useState("buy"),
    [quantity, setQuantity] = useState("100"),
    [price, setPrice] = useState(""),
    [preview, setPreview] = useState<Preview>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [clientId, setClientId] = useState(() => crypto.randomUUID()),
    [leverage, setLeverage] = useState(1),
    [kind, setKind] = useState("limit"),
    [tif, setTif] = useState("GTC");
  useEffect(() => {
    setPrice((stock.price / 100).toFixed(2));
    setPreview(undefined);
    setClientId(crypto.randomUUID());
    setError("");
  }, [stock.id]);
  async function prepare(e: FormEvent) {
    e.preventDefault();
    if (!account) {
      onLogin();
      return;
    }
    setError("");
    setBusy(true);
    try {
      const q = Number(quantity);
      if (!Number.isSafeInteger(q) || q <= 0)
        throw new Error("股数必须为正整数");
      setPreview(
        await api<Preview>("/orders/preview", {
          stock: String(stock.id),
          side,
          quantity: q,
          price: kind === "market" ? stock.price : cents(price),
          clientId,
          kind,
          timeInForce: tif,
          leverage: side === "buy" || side === "short" ? leverage : 1,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const rules = settings?.activeMarket,
    lot = rules?.lotSize ?? 1,
    maxLeverage = Math.min(10, Math.floor(1000000 / (rules?.imrPPM ?? 500000))),
    position = portfolio?.positions.find((p) => p.stockId === stock.id);
  const state = quoteState(
    stock,
    rules?.id === "meow"
      ? (settings?.quoteTTLSeconds ?? 86400)
      : (rules?.quoteTTLSeconds ?? 86400),
  );
  const blocked = !online
    ? "行情连接中断，等待重连"
    : !account
      ? ""
      : account.forcePasswordChange
        ? "请先修改初始密码"
        : !account.boundStockId
          ? "请先绑定股票"
          : account.boundStockId === stock.id
            ? "不能交易自己绑定的股票"
            : !market?.open
              ? (market?.reason ?? "市场休市")
              : state !== "可交易"
                ? state
                : "";
  const estimateQuantity = Number(quantity),
    estimatePrice =
      kind === "market" ? stock.price : Math.round(Number(price) * 100),
    amount =
      Number.isSafeInteger(estimateQuantity) &&
      Number.isSafeInteger(estimatePrice) &&
      Number.isSafeInteger(estimateQuantity * estimatePrice)
        ? Math.max(0, estimateQuantity * estimatePrice)
        : 0;
  const commissionPPM =
      rules?.id === "meow"
        ? (settings?.commissionPPM ?? 0)
        : (rules?.commissionPPM ?? 0),
    minCommission =
      rules?.id === "meow"
        ? (settings?.minCommission ?? 0)
        : (rules?.minCommission ?? 0),
    estimateCommission = amount
      ? Math.max(minCommission, Math.ceil((amount * commissionPPM) / 1000000))
      : 0,
    estimateStamp =
      amount &&
      (rules?.stampSide === "both" || side === "sell" || side === "short")
        ? Math.ceil(
            Math.ceil((amount * (rules?.stampPPM ?? 0)) / 1000000) /
              Math.max(1, rules?.stampRound ?? 1),
          ) * Math.max(1, rules?.stampRound ?? 1)
        : 0,
    estimateLevy = Math.ceil((amount * (rules?.levyPPM ?? 0)) / 1000000),
    taxRate =
      rules?.id === "meow"
        ? (settings?.taxBrackets.find(
            (b) => !b.upTo || (account?.equity ?? 0) <= b.upTo,
          )?.ratePPM ?? 0)
        : 0,
    estimateTax = Math.ceil((amount * taxRate) / 1000000),
    estimateFee =
      estimateCommission + estimateStamp + estimateLevy + estimateTax,
    estimateOwn =
      side === "buy" || side === "short"
        ? Math.ceil(
            amount * Math.max(1 / leverage, (rules?.imrPPM ?? 0) / 1000000),
          )
        : amount;
  return (
    <LayerCard className="panel ticket">
      <div className="panel-heading">
        <h2>
          <ArrowDownUp size={18} /> 交易委托
        </h2>
        <Badge variant="secondary">{kindName[kind]}</Badge>
      </div>
      <form className="form" onSubmit={prepare}>
        <div className="side-switch">
          {Object.entries(sideName).map(([id, label]) => (
            <Button
              type="button"
              key={id}
              aria-pressed={side === id}
              className={side === id ? "active" : ""}
              onClick={() => {
                setSide(id);
                setClientId(crypto.randomUUID());
              }}
            >
              {label}
            </Button>
          ))}
        </div>
        <div className="trade-stock">
          <span className="stock-avatar">{stock.name[0]}</span>
          <div>
            <strong>{stock.name}</strong>
            <small>{stock.symbol || stock.id}</small>
          </div>
        </div>
        <label>
          委托类型
          <Select<string>
            aria-label="委托类型"
            value={kind}
            onValueChange={(value) => {
              setKind(value ?? "");
              setClientId(crypto.randomUUID());
            }}
            items={[
              { value: "limit", label: "限价委托" },
              { value: "market", label: "市价委托" },
              { value: "stop", label: "止损触发市价" },
            ]}
          />
        </label>
        <label>
          有效期
          <Select<string>
            aria-label="委托有效期"
            value={tif}
            onValueChange={(value) => {
              setTif(value ?? "");
              setClientId(crypto.randomUUID());
            }}
            items={[
              { value: "GTC", label: "GTC · 本赛季有效" },
              { value: "DAY", label: "DAY · 市场次日零点到期" },
            ]}
          />
        </label>
        <label>
          委托股数
          <Input
            type="number"
            min={lot}
            step={lot}
            required
            value={quantity}
            onChange={(e) => {
              setQuantity(e.target.value);
              setClientId(crypto.randomUUID());
            }}
          />
        </label>
        <div className="list-tabs">
          {[1, 5, 10].map((n) => (
            <Button
              key={n}
              type="button"
              onClick={() => {
                setQuantity(String(lot * n));
                setClientId(crypto.randomUUID());
              }}
            >
              {n} 手
            </Button>
          ))}
        </div>
        <label>
          {kind === "stop" ? "触发股价" : "委托限价"}{" "}
          <span className="muted">模拟币 / 股</span>
          <Input
            inputMode="decimal"
            required
            disabled={kind === "market"}
            value={kind === "market" ? (stock.price / 100).toFixed(2) : price}
            onChange={(e) => {
              setPrice(e.target.value);
              setClientId(crypto.randomUUID());
            }}
          />
        </label>
        <Button
          type="button"
          variant="ghost"
          className="text-button"
          onClick={() => {
            setPrice((stock.price / 100).toFixed(2));
            setClientId(crypto.randomUUID());
          }}
        >
          使用最新价 {money(stock.price)}
        </Button>
        {(side === "buy" || side === "short") && (
          <label>
            买入方式 / 杠杆
            <Select<number>
              aria-label="买入方式"
              value={leverage}
              onValueChange={(value) => {
                setLeverage(Number(value ?? ""));
                setClientId(crypto.randomUUID());
              }}
              items={[
                ...Array.from({ length: maxLeverage }, (_, i) => i + 1).map(
                  (n) => ({
                    value: n,
                    label: n === 1 ? "现金 / 1 倍" : `${n} 倍杠杆`,
                  }),
                ),
              ]}
            />
          </label>
        )}
        <div className="ticket-estimate">
          <p>
            可用现金{" "}
            {money(
              account?.availableCash ?? portfolio?.account.availableCash ?? 0,
            )}
          </p>
          <p>
            可卖 {number(position?.sellable ?? 0)} 股 · 可回补{" "}
            {number(position?.shortQuantity ?? 0)} 股
          </p>
          <p>
            参考金额{" "}
            {money(
              Number(quantity) *
                (kind === "market" ? stock.price : Number(price) * 100),
            )}
          </p>
          <p>
            预估费用 {money(estimateFee)}（税 {money(estimateTax)} / 佣{" "}
            {money(estimateCommission)} / 印花 {money(estimateStamp)} / 征费{" "}
            {money(estimateLevy)}）
          </p>
          <p>预计自有资金占用 {money(estimateOwn + estimateFee)}</p>
          <small>
            IMR {rate(rules?.imrPPM ?? 0)} · MMR {rate(rules?.mmrPPM ?? 0)} ·
            借券年费 {rate(rules?.borrowAnnualPPM ?? 0)}
          </small>
          {(leverage > 1 || side === "short" || kind === "stop") && (
            <Banner variant="alert">
              杠杆放大损益，费用持续计提。止损跳空可能以更差价格成交，实际资金占用与税费以服务器预览为准。
            </Banner>
          )}
        </div>
        <div className="ticket-info">
          <Clock3 size={16} />
          <span>
            超额部分将挂单，后续整点按报价从高到低撮合。实际成交受溢价、额度和限价约束。
          </span>
        </div>
        {side === "short" && !stock.bound && (
          <Banner variant="alert">
            这支股票尚未绑定交易账户，暂不支持卖空。
          </Banner>
        )}
        {stock.delisted && (
          <Banner variant="alert">
            这支股票已在本月退市，等待新赛季恢复。
          </Banner>
        )}
        {blocked && <Banner variant="alert">{blocked}</Banner>}
        {error && !preview && (
          <Banner variant="error" role="alert">
            {error}
          </Banner>
        )}
        <Button
          type="submit"
          variant="primary"
          className="primary wide"
          disabled={
            busy ||
            !!blocked ||
            stock.delisted ||
            (side === "short" && !stock.bound)
          }
        >
          {busy ? "正在计算…" : account ? "预览交易明细" : "登录后交易"}
          <ChevronRight size={16} />
        </Button>
        <small className="muted">
          资金、持仓、税费及保证金以服务器最终核验为准。
        </small>
      </form>
      <Dialog.Root
        open={!!preview}
        onOpenChange={(open) => {
          if (!open)
            (busy
              ? undefined
              : () => {
                  setPreview(undefined);
                  setError("");
                })?.();
        }}
      >
        <Dialog size="lg" className="exchange-dialog">
          {preview && (
            <>
              <div className="dialog-heading">
                <Dialog.Title>确认交易委托</Dialog.Title>
                <Button
                  variant="ghost"
                  shape="square"
                  aria-label="关闭"
                  disabled={
                    !(busy
                      ? undefined
                      : () => {
                          setPreview(undefined);
                          setError("");
                        })
                  }
                  onClick={() =>
                    (busy
                      ? undefined
                      : () => {
                          setPreview(undefined);
                          setError("");
                        })?.()
                  }
                >
                  <XIcon size={20} />
                </Button>
              </div>
              <div className="dialog-body">
                <p className="muted">
                  请核对以下明细。重复确认同一委托不会重复下单。
                </p>
                <dl className="receipt">
                  <div>
                    <dt>股票 / 方向</dt>
                    <dd>
                      {stock.name} · {sideName[preview.side]}
                    </dd>
                  </div>
                  <div>
                    <dt>类型 / 有效期 / 杠杆</dt>
                    <dd>
                      {kindName[preview.kind]} · {preview.timeInForce} ·{" "}
                      {preview.leverage} 倍
                    </dd>
                  </div>
                  <div>
                    <dt>保证金 / 预计还贷</dt>
                    <dd>
                      {money(preview.collateral)} / {money(preview.repayment)}
                    </dd>
                  </div>
                  <div>
                    <dt>数量</dt>
                    <dd>{number(preview.quantity)} 股</dd>
                  </div>
                  <div>
                    <dt>限价</dt>
                    <dd>{money(preview.price)} 模拟币</dd>
                  </div>
                  <div>
                    <dt>委托金额</dt>
                    <dd>{money(preview.notional)}</dd>
                  </div>
                  <div>
                    <dt>资产档位税率</dt>
                    <dd>{rate(preview.taxRatePPM)}</dd>
                  </div>
                  <div>
                    <dt>预估税额</dt>
                    <dd>{money(preview.tax)}</dd>
                  </div>
                  {preview.commission !== undefined && (
                    <div>
                      <dt>预估佣金</dt>
                      <dd>{money(preview.commission)}</dd>
                    </div>
                  )}
                  {preview.stamp !== undefined && (
                    <div>
                      <dt>印花税</dt>
                      <dd>{money(preview.stamp)}</dd>
                    </div>
                  )}
                  {preview.levy !== undefined && (
                    <div>
                      <dt>交易征费</dt>
                      <dd>{money(preview.levy)}</dd>
                    </div>
                  )}
                  {preview.financing !== undefined && (
                    <div>
                      <dt>预估融资金额</dt>
                      <dd>{money(preview.financing)}</dd>
                    </div>
                  )}
                  <div>
                    <dt>当前溢价</dt>
                    <dd>{rate(preview.premiumPPM)}</dd>
                  </div>
                  <div className="total">
                    <dt>
                      {preview.side === "sell"
                        ? "预估到账"
                        : preview.side === "short"
                          ? "预估现金占用"
                          : "预估支出"}
                    </dt>
                    <dd>{money(preview.estimatedTotal)}</dd>
                  </div>
                </dl>
                <p className="small muted">
                  确认有效至 {datetime(preview.expiresAt)}
                  。成交时重新核验价格与资产，未成交部分保留为挂单。
                </p>
                {error && (
                  <Banner variant="error" role="alert">
                    {error}
                  </Banner>
                )}
                <Button
                  variant="primary"
                  className="primary wide"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    setError("");
                    try {
                      const r = await api<{ order: Order }>("/orders/confirm", {
                        previewId: preview.previewId,
                      });
                      setPreview(undefined);
                      setClientId(crypto.randomUUID());
                      await onDone(r.order);
                    } catch (e) {
                      setError((e as Error).message);
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  {busy ? "提交中…" : "确认提交委托"}
                  <Check size={16} />
                </Button>
              </div>
            </>
          )}
        </Dialog>
      </Dialog.Root>
    </LayerCard>
  );
}
