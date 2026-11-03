"use client";
import { LayerCard, Select, Table, Meter } from "@cloudflare/kumo";
import { useState } from "react";
import { datetime, money, rate } from "./api";
import { Fees } from "./Portfolio";
import type { Seasons as Data, Season } from "./types";
export function SeasonCard({
  season,
  now = Date.now(),
}: {
  season?: Season;
  now?: number;
}) {
  if (!season) return null;
  const progress = Math.max(
    0,
    Math.min(
      100,
      ((now - season.startsAt) / (season.endsAt - season.startsAt)) * 100,
    ),
  );
  return (
    <section className="season-card">
      <strong>{season.id} 月赛</strong>
      <p>
        {season.settled
          ? "已结算"
          : now < season.startsAt
            ? "等待开赛"
            : now >= season.endsAt
              ? "正在结算"
              : `距离截止 ${Math.max(0, Math.ceil((season.endsAt - now) / 86400000))} 天`}
      </p>
      <Meter label="月赛进度" value={progress} max={100} />
      <small>
        开赛 {datetime(season.startsAt)}
        <br />
        截止 {datetime(season.endsAt)}
      </small>
    </section>
  );
}
export function Seasons({
  data,
  accountId,
}: {
  data?: Data;
  accountId?: number;
}) {
  const [selected, setSelected] = useState("current");
  if (!data) return <p>正在读取赛季…</p>;
  const archive = data.archives.find((a) => a.season.id === selected),
    ranks = archive?.rankings ?? data.rankings,
    season = archive?.season ?? data.season;
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>月赛与排行榜</h1>
          <p>按最新净资产排名，收益率使用每位玩家实际起始本金。</p>
        </div>
        <label>
          赛季
          <Select<string>
            aria-label="赛季选择"
            value={selected}
            onValueChange={(value) => setSelected(value ?? "")}
            items={[
              { value: "current", label: <>当前 {data.season.id}</> },
              ...data.archives.map((a) => ({
                value: a.season.id,
                label: <>{a.season.id} 最终排名</>,
              })),
            ]}
          />
        </label>
      </div>
      <SeasonCard season={season} />
      <div className="rank-podium">
        {ranks.slice(0, 3).map((r) => (
          <LayerCard className="panel padded" key={r.accountId}>
            <span>第 {r.rank} 名</span>
            <h2>
              {r.username}
              {r.accountId === accountId ? " · 我" : ""}
            </h2>
            <strong>{money(r.equity)}</strong>
            <p className={r.returnPPM >= 0 ? "up" : "down"}>
              {rate(r.returnPPM)}
            </p>
          </LayerCard>
        ))}
      </div>
      <LayerCard className="panel">
        <div className="panel-heading">
          <h2>全部参赛玩家 · {ranks.length} 人</h2>
        </div>
        <div className="table-scroll">
          <Table>
            <Table.Header>
              <Table.Row>
                <Table.Head>名次</Table.Head>
                <Table.Head>玩家</Table.Head>
                <Table.Head>总资产</Table.Head>
                <Table.Head>收益率</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {ranks.map((r) => (
                <Table.Row
                  className={r.accountId === accountId ? "my-rank" : ""}
                  key={r.accountId}
                >
                  <Table.Cell>{r.rank}</Table.Cell>
                  <Table.Cell>
                    {r.username}
                    {r.accountId === accountId && " · 我"}
                  </Table.Cell>
                  <Table.Cell>{money(r.equity)}</Table.Cell>
                  <Table.Cell className={r.returnPPM >= 0 ? "up" : "down"}>
                    {rate(r.returnPPM)}
                  </Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table>
        </div>
      </LayerCard>
      <div className="revenue-grid">
        <LayerCard className="panel padded">
          <h2>本赛季交易所收入</h2>
          <Fees fees={archive?.revenue ?? data.revenue} />
        </LayerCard>
        <LayerCard className="panel padded">
          <h2>跨赛季累计收入</h2>
          <Fees fees={data.lifetimeRevenue} />
        </LayerCard>
      </div>
    </>
  );
}
