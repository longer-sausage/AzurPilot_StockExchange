"use client";
import { useRoute } from "../core/router";
import { useState } from "react";
import {
  Banner,
  Button,
  ClipboardText,
  Empty,
  LayerCard,
  LinkButton,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import { useExchange } from "../core/Exchange";
import { money, date, percent, quantity } from "../core/format";
import { AccountRequired } from "../features/AccountAccess";
import { FeeDetails, TradeRecords } from "../features/Records";
import type { Ledger, Stock } from "../core/contracts";
export function AccountBook({
  data,
  stocks,
}: {
  data: Ledger;
  stocks: Stock[];
}) {
  const route = useRoute();
  const [tab, setTab] = useState("positions"),
    a = data.account,
    m = a.metrics ?? {};
  return (
    <>
      <dl className="overview">
        {[
          ["总资产", money(a.equity)],
          ["可用现金", money(a.availableCash)],
          ["本赛季收益", percent(a.returnPPM)],
          ["持仓浮盈亏", money(m.floatingPnL)],
        ].map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <Tabs
        variant="underline"
        value={tab}
        onValueChange={setTab}
        tabs={[
          { value: "positions", label: "当前持仓" },
          { value: "details", label: "资产与交收" },
          { value: "trades", label: "逐笔成交" },
        ]}
      />
      {tab === "positions" && (
        <LayerCard className="data-card">
          {data.positions.length ? (
            <div className="table-region">
              <Table className="min-w-[1000px]">
                <Table.Header>
                  <Table.Row>
                    {[
                      "证券",
                      "持有 / 可卖",
                      "多头成本 / 均价",
                      "市值 / 多头浮盈亏",
                      "空头股数 / 负债",
                      "空头均价 / 浮盈亏",
                      "融资 / 保证金",
                    ].map((t) => (
                      <Table.Head key={t}>{t}</Table.Head>
                    ))}
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {data.positions.map((p) => {
                    const s = stocks.find((s) => s.id === p.stockId) ?? p.stock,
                      price = s?.price ?? 0;
                    return (
                      <Table.Row key={p.stockId}>
                        <Table.Cell>
                          <LinkButton
                            variant="ghost"
                            href="/"
                            onClick={(e) => {
                              e.preventDefault();
                              route.navigate(`/security?stock=${p.stockId}`);
                            }}
                            className="-ml-3"
                          >
                            {s?.name ?? p.stockId}
                          </LinkButton>
                          <p className="caption">最新 {money(price)}</p>
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {quantity(p.quantity)} /{" "}
                          {quantity(p.sellable ?? p.quantity)}
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {money(p.cost)}
                          <p className="caption">
                            {p.quantity ? money(p.cost / p.quantity) : "—"}
                          </p>
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {money(p.quantity * price)}
                          <p
                            className={
                              p.quantity * price - p.cost >= 0 ? "gain" : "loss"
                            }
                          >
                            {money(p.quantity * price - p.cost)}
                          </p>
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {quantity(p.shortQuantity)}
                          <p className="caption">
                            {money(p.shortQuantity * price)}
                          </p>
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {p.shortQuantity
                            ? money(p.shortCost / p.shortQuantity)
                            : "—"}
                          <p
                            className={
                              p.shortCost - p.shortQuantity * price >= 0
                                ? "gain"
                                : "loss"
                            }
                          >
                            {money(p.shortCost - p.shortQuantity * price)}
                          </p>
                        </Table.Cell>
                        <Table.Cell className="numeric">
                          {money(p.loan)}
                          <p className="caption">{money(p.collateral)}</p>
                        </Table.Cell>
                      </Table.Row>
                    );
                  })}
                </Table.Body>
              </Table>
            </div>
          ) : (
            <Empty
              title="还没有持仓"
              description="从证券市场选择一支股票，完成你的第一笔模拟交易。"
              contents={
                <LinkButton
                  href="/"
                  onClick={(e) => {
                    e.preventDefault();
                    route.navigate("/");
                  }}
                  variant="primary"
                >
                  浏览证券市场
                </LinkButton>
              }
            />
          )}
        </LayerCard>
      )}
      {tab === "trades" && (
        <LayerCard className="data-card">
          <TradeRecords trades={data.trades ?? []} />
        </LayerCard>
      )}
      {tab === "details" && (
        <div className="two-columns">
          <LayerCard className="card-body">
            <h2>资产明细</h2>
            {a.legacyMetrics && (
              <Banner
                variant="alert"
                description={`历史统计不完整。已实现盈亏从 ${date(a.metricsSince)} 起累计；旧借息和融资费用不能还原。`}
              />
            )}
            <dl className="definition-list">
              {Object.entries({
                cash: "现金余额",
                initialCash: "赛季起始本金",
                realized: "已实现盈亏",
                reserved: "委托冻结",
              }).map(([k, label]) => (
                <div key={k}>
                  <dt>{label}</dt>
                  <dd>{money(Number(a[k as keyof typeof a]))}</dd>
                </div>
              ))}
              {Object.entries({
                financingDebt: "融资借款",
                shortLiability: "空头负债",
                initialMargin: "初始保证金要求",
                maintenanceMargin: "维持保证金要求",
                collateral: "空头保证金",
                shortProceeds: "冻结卖空所得",
                unsettled: "待交收现金",
                financingAccrued: "未落账融资利息估算",
                borrowAccrued: "未落账借券费估算",
              }).map(([k, label]) => (
                <div key={k}>
                  <dt>{label}</dt>
                  <dd>{money(m[k])}</dd>
                </div>
              ))}
            </dl>
            <h3>身份识别码</h3>
            <ClipboardText text={a.identityCode} />
          </LayerCard>
          <div className="stack">
            <LayerCard className="card-body">
              <h2>本赛季费用</h2>
              <FeeDetails fees={a.fees} />
            </LayerCard>
            <LayerCard className="card-body">
              <h2>现金交收</h2>
              {a.settlements?.length ? (
                <dl className="definition-list">
                  {a.settlements.map((s, i) => (
                    <div key={i}>
                      <dt>{date(s.availableAt)} 可用</dt>
                      <dd>{money(s.amount)}</dd>
                    </div>
                  ))}
                </dl>
              ) : (
                <p className="muted">没有待交收现金。</p>
              )}
            </LayerCard>
          </div>
        </div>
      )}
    </>
  );
}
export function PortfolioPage() {
  const x = useExchange();
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">投资组合</p>
          <h1>我的资产</h1>
          <p>查看持仓、资金占用、损益与每一笔交收。</p>
        </div>
        {x.account && (
          <Button onClick={() => x.setBindingOpen(true)}>股票绑定</Button>
        )}
      </div>
      {x.ledger ? (
        <AccountBook data={x.ledger} stocks={x.market?.stocks ?? []} />
      ) : (
        <AccountRequired />
      )}
    </>
  );
}
