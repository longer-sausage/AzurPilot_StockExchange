"use client";
import { Button, Empty, LayerCard, Table, Banner } from "@cloudflare/kumo";
import { useState } from "react";
import {
  api,
  datetime,
  money,
  number,
  rate,
  sideName,
  statusName,
} from "./api";
import type {
  Order,
  Portfolio as Data,
  Revenue,
  Stock,
  TradePrint,
} from "./types";
export const kindName: Record<string, string> = {
  market: "市价",
  limit: "限价",
  stop: "止损触发市价",
};
export function Fees({ fees }: { fees?: Revenue }) {
  return (
    <dl className="receipt">
      <div>
        <dt>合计</dt>
        <dd>
          {money(Object.values(fees ?? {}).reduce((sum, v) => sum + v, 0))}
        </dd>
      </div>
      {Object.entries({
        tax: "资产档位税",
        commission: "佣金",
        stamp: "印花税",
        levy: "征费",
        borrow: "借券费",
        financing: "融资利息",
      }).map(([id, label]) => (
        <div key={id}>
          <dt>{label}</dt>
          <dd>{money(fees?.[id as keyof Revenue] ?? 0)}</dd>
        </div>
      ))}
    </dl>
  );
}
export function OrderTable({
  orders,
  stocks,
  onRefresh,
  onSelect,
  onError,
  readOnly = false,
}: {
  orders: Order[];
  readOnly?: boolean;
  stocks: Stock[];
  onRefresh: () => Promise<void>;
  onSelect: (id: number) => void;
  onError: (s: string) => void;
}) {
  const [scope, setScope] = useState("pending");
  const shown = orders.filter(
    (o) =>
      scope === "all" ||
      (scope === "pending" && o.status === "pending") ||
      (scope === "filled" && o.filled > 0),
  );
  return (
    <LayerCard className="panel orders-panel">
      <div className="panel-heading">
        <h2>委托与成交记录</h2>
        <div className="list-tabs">
          {[
            ["pending", "当前委托"],
            ["filled", "全部成交"],
            ["all", "全部成交与委托"],
          ].map(([v, l]) => (
            <Button
              key={v}
              variant={scope === v ? "secondary" : "ghost"}
              className={scope === v ? "active" : ""}
              onClick={() => setScope(v)}
            >
              {l}
            </Button>
          ))}
        </div>
      </div>
      <div className="table-scroll">
        <Table>
          <Table.Header>
            <Table.Row>
              <Table.Head>委托 / 股票</Table.Head>
              <Table.Head>方向 / 类型</Table.Head>
              <Table.Head>价格 / 数量</Table.Head>
              <Table.Head>实际成交</Table.Head>
              <Table.Head>交易费用</Table.Head>
              <Table.Head>状态 / 有效期</Table.Head>
              <Table.Head>操作</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {shown.map((o) => (
              <Table.Row key={o.id}>
                <Table.Cell>
                  #{o.id}
                  <Button
                    variant="ghost"
                    className="text-button"
                    onClick={() => onSelect(o.stockId)}
                  >
                    {stocks.find((s) => s.id === o.stockId)?.name ??
                      o.stockName ??
                      o.stockId}
                  </Button>
                  <small>{datetime(o.createdAt)}</small>
                </Table.Cell>
                <Table.Cell>
                  {sideName[o.side]} · {o.leverage} 倍
                  <small>{kindName[o.kind] ?? o.kind}</small>
                </Table.Cell>
                <Table.Cell>
                  {money(o.price)}
                  <small>
                    {number(o.filled)} / {number(o.quantity)} 股
                  </small>
                </Table.Cell>
                <Table.Cell>
                  {o.filled ? money(o.averagePrice) : "—"}
                  <small>{o.filled ? datetime(o.updatedAt) : "尚未成交"}</small>
                </Table.Cell>
                <Table.Cell>
                  {money(o.tax + o.commission + o.stamp + o.levy)}
                  <small>
                    税 {money(o.tax)} · 佣 {money(o.commission)}
                  </small>
                  <small>
                    印花 {money(o.stamp)} · 征费 {money(o.levy)}
                  </small>
                </Table.Cell>
                <Table.Cell>
                  {statusName[o.status] ?? o.status}
                  <small>
                    {o.timeInForce} ·{" "}
                    {o.expiresAt ? datetime(o.expiresAt) : "本赛季有效"}
                  </small>
                  {o.reason && <small>{o.reason}</small>}
                </Table.Cell>
                <Table.Cell>
                  {!readOnly && o.status === "pending" && (
                    <Button
                      onClick={async () => {
                        try {
                          await api(`/orders/${o.id}`, undefined, "DELETE");
                          await onRefresh();
                        } catch (e) {
                          onError((e as Error).message);
                        }
                      }}
                    >
                      撤单
                    </Button>
                  )}
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table>
      </div>
      {!shown.length && (
        <Empty
          title="暂无对应委托"
          description="实际成交和每次部分成交会保留完整记录。"
        />
      )}
    </LayerCard>
  );
}
export function TradeTable({ trades }: { trades: TradePrint[] }) {
  return (
    <div className="table-scroll">
      <Table>
        <Table.Header>
          <Table.Row>
            <Table.Head>成交 / 委托</Table.Head>
            <Table.Head>时间 / 玩家</Table.Head>
            <Table.Head>方向 / 方式</Table.Head>
            <Table.Head>成交价格</Table.Head>
            <Table.Head>成交数量</Table.Head>
            <Table.Head>成交额</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {trades.map((t) => (
            <Table.Row key={t.id}>
              <Table.Cell>
                #{t.id} / #{t.orderId}
              </Table.Cell>
              <Table.Cell>
                {datetime(t.at)}
                <small>{t.username}</small>
              </Table.Cell>
              <Table.Cell>
                {sideName[t.side]} · {kindName[t.kind] ?? t.kind}
                {t.forced && <small>系统平仓 · {t.reason}</small>}
              </Table.Cell>
              <Table.Cell>{money(t.price)}</Table.Cell>
              <Table.Cell>{number(t.quantity)}</Table.Cell>
              <Table.Cell>{money(t.notional)}</Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table>
      {!trades.length && <p className="empty">此范围暂无逐笔成交。</p>}
    </div>
  );
}
export function Portfolio({
  data,
  stocks,
  onSelect,
  onRefresh,
  onError,
  readOnly = false,
}: {
  data?: Data;
  readOnly?: boolean;
  stocks: Stock[];
  onSelect: (id: number) => void;
  onRefresh: () => Promise<void>;
  onError: (s: string) => void;
}) {
  const [tab, setTab] = useState("positions");
  if (!data)
    return (
      <Empty title="正在读取账户" description="等待服务器返回最新账本。" />
    );
  const a = data.account,
    m = a.metrics ?? {};
  return (
    <>
      <div className="stats-grid">
        <LayerCard className="stat">
          <span>总资产</span>
          <strong>{money(a.equity)}</strong>
          <small></small>
        </LayerCard>
        <LayerCard className="stat">
          <span>本赛季收益率</span>
          <strong>{rate(a.returnPPM)}</strong>
          <small></small>
        </LayerCard>
        <LayerCard className="stat">
          <span>可用现金</span>
          <strong>{money(a.availableCash ?? 0)}</strong>
          <small></small>
        </LayerCard>
        <LayerCard className="stat">
          <span>总浮动盈亏</span>
          <strong>{money(m.floatingPnL ?? 0)}</strong>
          <small></small>
        </LayerCard>
      </div>
      <div className="list-tabs account-tabs">
        {[
          ["positions", "当前持仓"],
          ["details", "账户明细"],
          ["trades", "逐笔成交"],
        ].map(([v, l]) => (
          <Button
            key={v}
            variant={tab === v ? "secondary" : "ghost"}
            className={tab === v ? "active" : ""}
            onClick={() => setTab(v)}
          >
            {l}
          </Button>
        ))}
      </div>
      {tab === "positions" && (
        <LayerCard className="panel">
          <div className="panel-heading">
            <h2>当前持仓</h2>
            <span>
              多头 {money(m.longPnL ?? 0)} · 空头 {money(m.shortPnL ?? 0)}
            </span>
          </div>
          <div className="table-scroll">
            <Table>
              <Table.Header>
                <Table.Row>
                  <Table.Head>证券 / 当前价格</Table.Head>
                  <Table.Head>持有 / 可卖</Table.Head>
                  <Table.Head>均价 / 成本</Table.Head>
                  <Table.Head>多头市值 / 浮盈亏</Table.Head>
                  <Table.Head>空头股数 / 负债</Table.Head>
                  <Table.Head>空头均价 / 浮盈亏</Table.Head>
                  <Table.Head>融资 / 保证金</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {data.positions.map((p) => {
                  const s = stocks.find((s) => s.id === p.stockId) ?? p.stock,
                    price = s?.price ?? 0,
                    long = p.quantity * price - p.cost,
                    short = p.shortCost - p.shortQuantity * price;
                  return (
                    <Table.Row key={p.stockId}>
                      <Table.Cell>
                        <Button
                          variant="ghost"
                          className="text-button"
                          onClick={() => onSelect(p.stockId)}
                        >
                          {s?.name ?? p.stockId}
                        </Button>
                        <small>{money(price)}</small>
                      </Table.Cell>
                      <Table.Cell>
                        {number(p.quantity)} /{" "}
                        {number(p.sellable ?? p.quantity)}
                      </Table.Cell>
                      <Table.Cell>
                        {p.quantity ? money(p.cost / p.quantity) : "—"}
                        <small>总成本 {money(p.cost)}</small>
                      </Table.Cell>
                      <Table.Cell>
                        {money(p.quantity * price)}
                        <small className={long >= 0 ? "up" : "down"}>
                          {money(long)}
                        </small>
                      </Table.Cell>
                      <Table.Cell>
                        {number(p.shortQuantity)}
                        <small>{money(p.shortQuantity * price)}</small>
                      </Table.Cell>
                      <Table.Cell>
                        {p.shortQuantity
                          ? money(p.shortCost / p.shortQuantity)
                          : "—"}
                        <small className={short >= 0 ? "up" : "down"}>
                          {money(short)}
                        </small>
                      </Table.Cell>
                      <Table.Cell>
                        {money(p.loan ?? 0)}
                        <small>保证金 {money(p.collateral)}</small>
                      </Table.Cell>
                    </Table.Row>
                  );
                })}
              </Table.Body>
            </Table>
          </div>
          {!data.positions.length && (
            <Empty
              title="暂无持仓"
              description="成交后会在这里显示市值、成本与损益。"
            />
          )}
        </LayerCard>
      )}
      {tab === "details" && (
        <LayerCard className="panel padded">
          <h2>账户明细</h2>
          {a.legacyMetrics && (
            <Banner variant="alert">
              旧数据统计不完整：已实现盈亏从 {datetime(a.metricsSince)}{" "}
              起累计，费用已补入可核对的旧订单交易费，无法还原旧借息与融资计提；存量空头按升级时卖空所得建立成本基线。
            </Banner>
          )}
          <dl className="receipt">
            <div>
              <dt>身份识别码</dt>
              <dd>
                {a.identityCode}
                <Button
                  onClick={() =>
                    void navigator.clipboard
                      .writeText(a.identityCode)
                      .catch(() => onError("复制失败，请手动复制身份码"))
                  }
                >
                  复制
                </Button>
              </dd>
            </div>
            {Object.entries({
              cash: "现金余额",
              initialCash: "赛季起始本金",
              realized: "已实现盈亏",
              reserved: "挂单冻结",
            }).map(([key, label]) => (
              <div key={key}>
                <dt>{label}</dt>
                <dd>{money(Number(a[key as keyof typeof a]))}</dd>
              </div>
            ))}
            {Object.entries({
              financingDebt: "融资借款余额",
              shortLiability: "做空负债",
              initialMargin: "当前市值初始保证金要求",
              maintenanceMargin: "维持保证金",
              collateral: "空头保证金",
              shortProceeds: "卖空所得冻结",
              unsettled: "待交收现金",
              financingAccrued: "未落账融资利息估算",
              borrowAccrued: "未落账借券费估算",
            }).map(([key, label]) => (
              <div key={key}>
                <dt>{label}</dt>
                <dd>{money(m[key] ?? 0)}</dd>
              </div>
            ))}
          </dl>
          <h3>本赛季已付费用</h3>
          <Fees fees={a.fees} />
          <h3>现金交收明细</h3>
          {a.settlements?.map((s, i) => (
            <p key={i}>
              {money(s.amount)} · {datetime(s.availableAt)} 可用
            </p>
          ))}
          <p className="muted">
            空头所得与保证金独立冻结；融资首付款已从现金支出。未落账费用为估算，以实际结算为准。
          </p>
        </LayerCard>
      )}
      {tab === "trades" && (
        <LayerCard className="panel">
          <div className="panel-heading">
            <h2>我的逐笔成交</h2>
          </div>
          <TradeTable trades={data.trades ?? []} />
        </LayerCard>
      )}
      <OrderTable
        orders={data.orders}
        stocks={stocks}
        onRefresh={onRefresh}
        onSelect={onSelect}
        onError={onError}
        readOnly={readOnly}
      />
    </>
  );
}
