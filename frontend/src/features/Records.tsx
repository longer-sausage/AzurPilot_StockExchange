"use client";
import { useRoute } from "../core/router";
import { useState } from "react";
import {
  Badge,
  Button,
  Empty,
  LayerCard,
  LinkButton,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import type { Fees, Order, Trade } from "../core/contracts";
import { date, kinds, money, quantity, sides, statuses } from "../core/format";
import { request } from "../core/http";
import { useExchange, useNotice } from "../core/Exchange";
export function OrderRecords({
  orders,
  readOnly = false,
}: {
  orders: Order[];
  readOnly?: boolean;
}) {
  const route = useRoute();
  const x = useExchange(),
    notice = useNotice();
  const [scope, setScope] = useState("pending"),
    [busy, setBusy] = useState<number>();
  const list = orders.filter(
    (o) =>
      scope === "all" ||
      (scope === "pending" && o.status === "pending") ||
      (scope === "filled" && o.filled > 0),
  );
  async function cancel(id: number) {
    setBusy(id);
    try {
      await request(`/orders/${id}`, undefined, "DELETE");
      await x.refresh();
      notice("委托已撤销");
    } catch (e) {
      notice((e as Error).message, true);
    } finally {
      setBusy(undefined);
    }
  }
  return (
    <LayerCard className="data-card">
      <div className="card-heading">
        <h2>委托与成交</h2>
        <Tabs
          variant="underline"
          value={scope}
          onValueChange={setScope}
          tabs={[
            { value: "pending", label: "待成交" },
            { value: "filled", label: "已成交" },
            { value: "all", label: "全部记录" },
          ]}
        />
      </div>
      {list.length ? (
        <div className="table-region">
          <Table className="min-w-[880px]">
            <Table.Header>
              <Table.Row>
                {[
                  "委托 / 证券",
                  "方向 / 类型",
                  "限价 / 数量",
                  "实际成交",
                  "费用",
                  "状态 / 有效期",
                  "",
                ].map((label) => (
                  <Table.Head key={label}>{label}</Table.Head>
                ))}
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {list.map((o) => (
                <Table.Row key={o.id}>
                  <Table.Cell>
                    <div className="grid gap-1">
                      <LinkButton
                        href="/"
                        onClick={(e) => {
                          e.preventDefault();
                          route.navigate(`/security?stock=${o.stockId}`);
                        }}
                        variant="ghost"
                        className="-ml-3"
                      >
                        {x.market?.stocks.find((s) => s.id === o.stockId)
                          ?.name ??
                          o.stockName ??
                          `证券 ${o.stockId}`}
                      </LinkButton>
                      <span className="caption">
                        #{o.id} · {date(o.createdAt)}
                      </span>
                    </div>
                  </Table.Cell>
                  <Table.Cell>
                    {sides[o.side]} · {kinds[o.kind] ?? o.kind}
                    <p className="caption">{o.leverage} 倍杠杆</p>
                  </Table.Cell>
                  <Table.Cell className="numeric">
                    {money(o.price)}
                    <p className="caption">{quantity(o.quantity)} 股</p>
                  </Table.Cell>
                  <Table.Cell className="numeric">
                    {quantity(o.filled)} 股
                    <p className="caption">
                      均价 {o.filled ? money(o.averagePrice) : "—"}
                    </p>
                  </Table.Cell>
                  <Table.Cell className="numeric">
                    {money(o.tax + o.commission + o.stamp + o.levy)}
                    <p className="caption">
                      税 {money(o.tax)} · 佣 {money(o.commission)}
                    </p>
                    <p className="caption">
                      印花 {money(o.stamp)} · 征费 {money(o.levy)}
                    </p>
                  </Table.Cell>
                  <Table.Cell>
                    <Badge variant="secondary">
                      {statuses[o.status] ?? o.status}
                    </Badge>
                    <p className="caption">
                      {o.timeInForce} ·{" "}
                      {o.expiresAt ? date(o.expiresAt) : "赛季内有效"}
                    </p>
                    {o.reason && <p className="caption">{o.reason}</p>}
                  </Table.Cell>
                  <Table.Cell>
                    {!readOnly && o.status === "pending" && (
                      <Button
                        loading={busy === o.id}
                        onClick={() => void cancel(o.id)}
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
      ) : (
        <Empty
          title="暂无对应委托"
          description="委托提交后，成交与未成交余量会保留在这里。"
        />
      )}
    </LayerCard>
  );
}
export function TradeRecords({ trades }: { trades: Trade[] }) {
  return trades.length ? (
    <div className="table-region">
      <Table className="min-w-[720px]">
        <Table.Header>
          <Table.Row>
            {[
              "成交 / 委托",
              "时间 / 玩家",
              "方向",
              "价格",
              "股数",
              "成交额",
            ].map((t) => (
              <Table.Head key={t}>{t}</Table.Head>
            ))}
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {trades.map((t) => (
            <Table.Row key={t.id}>
              <Table.Cell>
                #{t.id} / #{t.orderId}
              </Table.Cell>
              <Table.Cell>
                <div>{date(t.at)}</div>
                <span className="caption">{t.username}</span>
              </Table.Cell>
              <Table.Cell>
                {sides[t.side]} · {kinds[t.kind] ?? t.kind}
                {t.forced && <p className="caption">系统平仓：{t.reason}</p>}
              </Table.Cell>
              <Table.Cell className="numeric">{money(t.price)}</Table.Cell>
              <Table.Cell className="numeric">
                {quantity(t.quantity)}
              </Table.Cell>
              <Table.Cell className="numeric">{money(t.notional)}</Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table>
    </div>
  ) : (
    <Empty title="暂无逐笔成交" description="这里仅展示实际发生的成交。" />
  );
}
export function FeeDetails({ fees }: { fees?: Fees }) {
  return (
    <dl className="definition-list">
      {Object.entries({
        tax: "资产档位税",
        commission: "佣金",
        stamp: "印花税",
        levy: "交易征费",
        borrow: "借券费用",
        financing: "融资利息",
      }).map(([key, label]) => (
        <div key={key}>
          <dt>{label}</dt>
          <dd>{money(fees?.[key as keyof Fees] ?? 0)}</dd>
        </div>
      ))}
      <div>
        <dt>合计</dt>
        <dd>
          {money(Object.values(fees ?? {}).reduce((sum, v) => sum + v, 0))}
        </dd>
      </div>
    </dl>
  );
}
