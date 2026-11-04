"use client";
import { useState } from "react";
import {
  Badge,
  Empty,
  LayerCard,
  Meter,
  Select,
  Table,
} from "@cloudflare/kumo";
import { useExchange } from "../core/Exchange";
import { date, money, percent } from "../core/format";
import { FeeDetails } from "../features/Records";
export function SeasonsPage() {
  const x = useExchange(),
    [chosen, setChosen] = useState("current"),
    data = x.seasons;
  if (!data) return <Empty title="正在读取月赛" />;
  const archive = data.archives.find((s) => s.season.id === chosen),
    ranks = archive?.rankings ?? data.rankings,
    season = archive?.season ?? data.season;
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">月赛</p>
          <h1>月赛排名</h1>
          <p>排行榜按每位参赛者的净资产从高到低排序，收益率以本赛季你的实际起始本金为基准计算。</p>
        </div>
        <Select
          label="赛季"
          value={chosen}
          onValueChange={(v) => setChosen(String(v))}
          items={[
            { value: "current", label: `当前赛季 ${data.season.id}` },
            ...data.archives.map((s) => ({
              value: s.season.id,
              label: `${s.season.id} 最终排名`,
            })),
          ]}
        />
      </div>
      <LayerCard className="card-body">
        <div className="toolbar">
          <h2>{season.id} 赛季</h2>
          <Badge variant="secondary">
            {season.settled
              ? "已结算"
              : x.now < season.startsAt
                ? "等待开赛"
                : x.now > season.endsAt
                  ? "正在结算"
                  : "进行中"}
          </Badge>
        </div>
        <Meter
          label="赛季进度"
          value={Math.min(
            100,
            Math.max(
              0,
              ((x.now - season.startsAt) / (season.endsAt - season.startsAt)) *
                100,
            ),
          )}
          max={100}
        />
        <p className="caption">
          {date(season.startsAt)} — {date(season.endsAt)}
        </p>
      </LayerCard>
      <LayerCard className="data-card">
        <div className="table-region">
          <Table className="min-w-[560px]">
            <Table.Header>
              <Table.Row>
                <Table.Head>名次</Table.Head>
                <Table.Head>玩家</Table.Head>
                <Table.Head className="text-right">净资产</Table.Head>
                <Table.Head className="text-right">收益率</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {ranks.map((r) => (
                <Table.Row
                  key={r.accountId}
                  variant={
                    r.accountId === x.account?.id ? "selected" : "default"
                  }
                >
                  <Table.Cell>{r.rank}</Table.Cell>
                  <Table.Cell>
                    {r.username}
                    {r.accountId === x.account?.id && " · 我"}
                  </Table.Cell>
                  <Table.Cell className="numeric text-right">
                    {money(r.equity)}
                  </Table.Cell>
                  <Table.Cell
                    className={`numeric text-right ${r.returnPPM >= 0 ? "gain" : "loss"}`}
                  >
                    {percent(r.returnPPM)}
                  </Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table>
        </div>
        {!ranks.length && (
          <Empty
            title="本赛季还没有排名"
            description="完成开户并成功绑定实例股票后，你的战绩就会自动计入月赛排行榜。"
          />
        )}
      </LayerCard>
      <div className="two-columns">
        <LayerCard className="card-body">
          <h2>本赛季交易所收入</h2>
          <FeeDetails fees={archive?.revenue ?? data.revenue} />
        </LayerCard>
        <LayerCard className="card-body">
          <h2>跨赛季累计收入</h2>
          <FeeDetails fees={data.lifetimeRevenue} />
        </LayerCard>
      </div>
    </>
  );
}
