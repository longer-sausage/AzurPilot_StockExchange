"use client";
import { useRoute } from "../core/router";
import { useEffect, useState } from "react";
import {
  Badge,
  Banner,
  Empty,
  LayerCard,
  LinkButton,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import { ArrowLeftIcon } from "@phosphor-icons/react";
import { useExchange } from "../core/Exchange";
import { request } from "../core/http";
import {
  day,
  delta,
  date,
  kinds,
  money,
  move,
  quantity,
  sides,
} from "../core/format";
import type { Detail, Period } from "../core/contracts";
import { PriceChart } from "../features/PriceChart";
import { TradeTicket } from "../features/TradeTicket";
import { TradeRecords } from "../features/Records";
export function SecurityPage() {
  const route = useRoute();
  const x = useExchange(),
    id = Number(route.query.get("stock")),
    stock = x.market?.stocks.find((s) => s.id === id),
    [detail, setDetail] = useState<Detail>(),
    [period, setPeriod] = useState<Period>("time"),
    [month, setMonth] = useState(day().slice(0, 7)),
    [selectedDay, setDay] = useState(""),
    [tab, setTab] = useState("chart"),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  useEffect(() => {
    if (!stock) return;
    let active = true;
    setLoading(true);
    void request<Detail>(
      `/public/stocks/${stock.id}/chart-detail?period=${period}&month=${month}&day=${selectedDay}`,
    )
      .then((data) => {
        if (active) {
          setDetail(data);
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
    stock?.id,
    stock?.historyVersion,
    stock?.historyCheckedAt,
    stock?.tradeVersion,
    period,
    month,
    selectedDay,
  ]);
  if (!stock)
    return (
      <Empty
        title={x.market ? "这只股票暂时无法查看" : "正在加载股票行情..."}
        contents={
          <LinkButton
            href="/"
            onClick={(e) => {
              e.preventDefault();
              route.navigate("/");
            }}
          >
            返回行情市场
          </LinkButton>
        }
      />
    );
  return (
    <>
      <div>
        <LinkButton
          href="/"
          onClick={(e) => {
            e.preventDefault();
            route.navigate("/");
          }}
          variant="ghost"
          className="-ml-3"
        >
          <ArrowLeftIcon size={16} />
          返回全部股票
        </LinkButton>
      </div>
      <div className="page-heading">
        <div>
          <p className="eyebrow">{stock.symbol}</p>
          <h1>{stock.name}</h1>
          <p>最新报价 {date(stock.observedAt)}</p>
        </div>
        <div>
          <div className="price-display numeric">
            <strong>{money(stock.price)}</strong>
            <span className={(delta(stock) ?? 0) >= 0 ? "gain" : "loss"}>
              {move(delta(stock))}
            </span>
          </div>
          <p className="caption mt-2">较今日开盘价（上海时区）</p>
        </div>
      </div>
      <div className="split">
        <div className="stack">
          <LayerCard className="data-card">
            <div className="card-heading">
              <Tabs
                variant="underline"
                value={tab}
                onValueChange={setTab}
                tabs={[
                  { value: "chart", label: "价格走势" },
                  { value: "trades", label: "逐笔成交" },
                  { value: "orders", label: "待成交委托" },
                  { value: "info", label: "证券资料" },
                ]}
              />
              <Badge variant="secondary">
                {stock.bound ? "已绑定" : "独立股票"}
              </Badge>
            </div>
            {tab === "chart" && (
              <PriceChart
                detail={detail}
                period={period}
                month={month}
                day={selectedDay}
                onPeriod={(p) => {
                  setPeriod(p);
                  setDay("");
                }}
                onMonth={(m) => {
                  setMonth(m);
                  setDay("");
                }}
                onDay={setDay}
                error={error}
                loading={loading}
              />
            )}{" "}
            {tab === "trades" && (
              <>
                <TradeRecords trades={detail?.trades ?? []} />
                {!!detail?.legacyOrders.length && (
                  <Banner
                    variant="secondary"
                    description={`另包含 ${detail.legacyOrders.length} 条早期版本的成交汇总；因当时未单独留存逐笔记录，此处仅展示汇总数据。`}
                  />
                )}
              </>
            )}
            {tab === "orders" && (
              <div className="table-region">
                <Table className="min-w-[600px]">
                  <Table.Header>
                    <Table.Row>
                      {[
                        "委托 / 玩家",
                        "方向 / 类型",
                        "价格",
                        "剩余股数",
                        "有效期",
                      ].map((t) => (
                        <Table.Head key={t}>{t}</Table.Head>
                      ))}
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {detail?.orders.map((o) => (
                      <Table.Row key={o.id}>
                        <Table.Cell>
                          #{o.id} · {o.username}
                        </Table.Cell>
                        <Table.Cell>
                          {sides[o.side]} · {kinds[o.kind]}
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {money(o.price)}
                        </Table.Cell>
                        <Table.Cell>{quantity(o.remaining)}</Table.Cell>
                        <Table.Cell>
                          {o.timeInForce} ·{" "}
                          {o.expiresAt ? date(o.expiresAt) : "本赛季"}
                        </Table.Cell>
                      </Table.Row>
                    ))}
                  </Table.Body>
                </Table>
                {!detail?.orders.length && (
                  <Empty
                    title="暂无待成交委托"
                    description="当前市场上暂无正在排队等待撮合的委托挂单。"
                  />
                )}
              </div>
            )}
            {tab === "info" && (
              <div className="card-body">
                <dl className="definition-list">
                  {[
                    ["报价来源", "AzurPilot 实例总行动力"],
                    ["价格换算", "1 行动力 = 1 模拟币"],
                    ["最近上传", date(stock.uploadedAt)],
                    ["数据覆盖", `${detail?.coverage.count ?? 0} 条月原始记录`],
                    [
                      "首条 / 末条",
                      `${date(detail?.coverage.firstObservedAt ?? 0)} / ${date(detail?.coverage.lastObservedAt ?? 0)}`,
                    ],
                  ].map(([label, value]) => (
                    <div key={label}>
                      <dt>{label}</dt>
                      <dd>{value}</dd>
                    </div>
                  ))}
                </dl>
                <p className="caption">
                  数据说明：实例签名仅用于验证上报来源，不作为游戏数据真实性背书；历史行情均真实记录不作截断，无上报时段亦不会虚构报价。
                </p>
              </div>
            )}
          </LayerCard>
          <LayerCard className="card-body">
            <h2>市场流动性与价格概览</h2>
            <div className="fields">
              <dl className="definition-list">
                {[
                  [
                    "发行剩余",
                    `${quantity(stock.issuedShares - stock.outstanding)} / ${quantity(stock.issuedShares)}`,
                  ],
                  [
                    "普通小时量",
                    `${quantity(stock.normalUsed)} / ${quantity(stock.hourlyVolume)}`,
                  ],
                  [
                    "可借券",
                    `${quantity(stock.borrowShares - stock.borrowed)} / ${quantity(stock.borrowShares)}`,
                  ],
                  [
                    "融券小时量",
                    `${quantity(stock.borrowUsed)} / ${quantity(stock.borrowHourlyVolume)}`,
                  ],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
              <dl className="definition-list">
                {[
                  [
                    "所选时段开 / 高 / 低",
                    `${money(detail?.summary.open)} / ${money(detail?.summary.high)} / ${money(detail?.summary.low)}`,
                  ],
                  [
                    "上一交易日收盘",
                    detail?.summary.previousClose
                      ? money(detail.summary.previousClose)
                      : "—",
                  ],
                  ["成交股数", quantity(detail?.summary.volume)],
                  ["成交额", money(detail?.summary.turnover)],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          </LayerCard>
        </div>
        <TradeTicket key={stock.id} stock={stock} />
      </div>
    </>
  );
}
