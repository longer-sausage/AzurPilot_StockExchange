"use client";
// 主要：证券名称与价格；其次：涨跌和库存；辅助：代码与来源。
// 先保留完整表格宽度，筛选、排序和自选直接使用 Kumo 原生控件。
import { useRoute } from "../core/router";
import { useMemo, useState } from "react";
import {
  Badge,
  Button,
  Empty,
  Input,
  InputGroup,
  LayerCard,
  LinkButton,
  Popover,
  Select,
  Table,
  Tabs,
} from "@cloudflare/kumo";
import {
  MagnifyingGlassIcon,
  SlidersHorizontalIcon,
  StarIcon,
  XIcon,
  ArrowUpRightIcon,
} from "@phosphor-icons/react";
import { useExchange, useNotice } from "../core/Exchange";
import { request } from "../core/http";
import { day, delta, money, move, quantity, stockState } from "../core/format";
import type { Stock } from "../core/contracts";
const initial = {
  listing: "listed",
  state: "all",
  direction: "all",
  owner: "all",
  min: "",
  max: "",
  minChange: "",
  maxChange: "",
  fresh: "0",
};
const sorts = [
  { value: "code-asc", label: "代码升序" },
  { value: "name-asc", label: "名称升序" },
  { value: "name-desc", label: "名称降序" },
  { value: "price-desc", label: "价格从高到低" },
  { value: "price-asc", label: "价格从低到高" },
  { value: "change-desc", label: "涨幅从高到低" },
  { value: "change-asc", label: "涨幅从低到高" },
  { value: "time-desc", label: "最新报价优先" },
  { value: "time-asc", label: "最早报价优先" },
  { value: "code-desc", label: "代码降序" },
];
export function MarketPage() {
  const route = useRoute();
  const x = useExchange(),
    notice = useNotice();
  const [query, setQuery] = useState(""),
    [scope, setScope] = useState("all"),
    [sort, setSort] = useState("code-asc"),
    [filters, setFilters] = useState(initial),
    [filterOpen, setFilterOpen] = useState(false);
  const stocks = x.market?.stocks ?? [],
    ttl =
      x.settings?.activeMarket.id === "meow"
        ? x.settings.quoteTTLSeconds
        : (x.settings?.activeMarket.quoteTTLSeconds ?? 86400),
    watch = x.account?.watchlist ?? [],
    positions = x.ledger?.positions ?? [],
    timeKey =
      filters.fresh !== "0" || filters.state !== "all"
        ? Math.floor(x.now / 1000)
        : day();
  const list = useMemo(
    () =>
      stocks
        .filter((s) => {
          const d = delta(s);
          return (
            `${s.name} ${s.symbol} ${s.id}`
              .toLocaleLowerCase()
              .includes(query.toLocaleLowerCase()) &&
            (scope === "all" ||
              (scope === "watch" && watch.includes(s.id)) ||
              (scope === "held" &&
                positions.some((p) => p.stockId === s.id))) &&
            (filters.listing === "all" ||
              (filters.listing === "listed" && !s.delisted) ||
              (filters.listing === "delisted" && s.delisted)) &&
            (filters.state === "all" ||
              stockState(s, ttl, x.now) === filters.state) &&
            (filters.direction === "all" ||
              (filters.direction === "up" && d !== undefined && d > 0) ||
              (filters.direction === "down" && d !== undefined && d < 0) ||
              (filters.direction === "flat" && d === 0) ||
              (filters.direction === "unknown" && d === undefined)) &&
            (filters.owner === "all" ||
              (filters.owner === "mine" && s.id === x.account?.boundStockId) ||
              (filters.owner === "others" &&
                s.id !== x.account?.boundStockId)) &&
            (!filters.min || s.price >= Number(filters.min) * 100) &&
            (!filters.max || s.price <= Number(filters.max) * 100) &&
            (!filters.minChange ||
              (d !== undefined && d >= Number(filters.minChange))) &&
            (!filters.maxChange ||
              (d !== undefined && d <= Number(filters.maxChange))) &&
            (!Number(filters.fresh) ||
              x.now - s.observedAt <= Number(filters.fresh) * 60000)
          );
        })
        .sort((a, b) => {
          const [key, dir] = sort.split("-"),
            sign = dir === "asc" ? 1 : -1;
          if (key === "name")
            return sign * a.name.localeCompare(b.name, "zh-CN");
          if (key === "change") {
            const ad = delta(a),
              bd = delta(b);
            if (ad === undefined) return bd === undefined ? a.id - b.id : 1;
            if (bd === undefined) return -1;
            return sign * (ad - bd) || a.id - b.id;
          }
          return (
            sign *
              ((key === "price"
                ? a.price
                : key === "time"
                  ? a.observedAt
                  : a.id) -
                (key === "price"
                  ? b.price
                  : key === "time"
                    ? b.observedAt
                    : b.id)) || a.id - b.id
          );
        }),
    [
      stocks,
      query,
      scope,
      sort,
      filters,
      watch,
      positions,
      x.account,
      ttl,
      timeKey,
    ],
  );
  const update = (key: keyof typeof initial, value: string) =>
    setFilters((v) => ({ ...v, [key]: value }));
  const active = Object.entries(filters).filter(
    ([key, value]) => value !== initial[key as keyof typeof initial],
  ).length;
  async function star(stock: Stock) {
    if (!x.account) {
      x.setLoginOpen(true);
      return;
    }
    try {
      await request(
        `/account/watchlist/${stock.id}`,
        { enabled: !watch.includes(stock.id) },
        "PUT",
      );
      await x.refresh();
    } catch (e) {
      notice((e as Error).message, true);
    }
  }
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Mingmiao Exchange</p>
          <h1>证券市场</h1>
          <p>浏览实例证券，关注每一次变化。在模拟市场中建立你的投资组合。</p>
        </div>
        <Badge variant="secondary">
          {x.market?.open ? "市场开放" : "暂未开市"}
        </Badge>
      </div>
      <dl className="overview">
        <div>
          <dt>上市证券</dt>
          <dd>
            {quantity(stocks.filter((s) => !s.delisted).length)}{" "}
            <small>支</small>
          </dd>
        </div>
        <div>
          <dt>本月参赛玩家</dt>
          <dd>
            {quantity(x.seasons?.rankings.length)} <small>人</small>
          </dd>
        </div>
        <div>
          <dt>今日上涨 / 下跌</dt>
          <dd>
            <span className="gain">
              {stocks.filter((s) => (delta(s) ?? 0) > 0).length}
            </span>{" "}
            /{" "}
            <span className="loss">
              {stocks.filter((s) => (delta(s) ?? 0) < 0).length}
            </span>
          </dd>
        </div>
        <div>
          <dt>当前赛季</dt>
          <dd>{x.market?.season?.id ?? "—"}</dd>
        </div>
      </dl>
      <section className="stack" aria-label="证券市场">
        <div className="toolbar">
          <Tabs
            variant="underline"
            value={scope}
            onValueChange={setScope}
            tabs={[
              { value: "all", label: `全部证券 ${stocks.length}` },
              { value: "watch", label: `我的自选 ${watch.length}` },
              { value: "held", label: "我的持仓" },
            ]}
          />
          <div className="toolbar-group">
            <div className="toolbar-search">
              <InputGroup>
                <InputGroup.Addon>
                  <MagnifyingGlassIcon size={18} />
                </InputGroup.Addon>
                <InputGroup.Input
                  aria-label="搜索证券"
                  placeholder="搜索名称、代码或编号"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </InputGroup>
            </div>
            <Popover.Root open={filterOpen} onOpenChange={setFilterOpen}>
              <Popover.Trigger render={<Button variant="secondary" />}>
                <SlidersHorizontalIcon size={18} />
                筛选{active ? ` (${active})` : ""}
              </Popover.Trigger>
              <Popover.Portal>
                <Popover.Positioner align="end" sideOffset={8}>
                  <Popover.Popup className="w-96 max-w-[calc(100vw-2rem)] p-5">
                    <div className="dialog-title">
                      <Popover.Title className="font-semibold">
                        筛选证券
                      </Popover.Title>
                      <Popover.Close
                        render={
                          <Button
                            variant="ghost"
                            shape="square"
                            aria-label="关闭筛选"
                          />
                        }
                      >
                        <XIcon size={18} />
                      </Popover.Close>
                    </div>
                    <div className="fields mt-5">
                      <Select
                        label="上市范围"
                        value={filters.listing}
                        onValueChange={(v) => update("listing", String(v))}
                        items={{
                          listed: "未退市",
                          all: "全部",
                          delisted: "已退市",
                        }}
                      />
                      <Select
                        label="报价状态"
                        value={filters.state}
                        onValueChange={(v) => update("state", String(v))}
                        items={{
                          all: "全部状态",
                          可交易: "可交易",
                          报价过期: "报价过期",
                          零报价: "零报价",
                          停牌: "停牌",
                        }}
                      />
                      <Select
                        label="涨跌方向"
                        value={filters.direction}
                        onValueChange={(v) => update("direction", String(v))}
                        items={{
                          all: "全部",
                          up: "上涨",
                          down: "下跌",
                          flat: "平盘",
                          unknown: "无开盘价",
                        }}
                      />
                      <Select
                        label="归属"
                        value={filters.owner}
                        onValueChange={(v) => update("owner", String(v))}
                        items={{
                          all: "全部",
                          mine: "我的股票",
                          others: "其他玩家",
                        }}
                      />
                      {(["min", "max", "minChange", "maxChange"] as const).map(
                        (key, i) => (
                          <Input
                            key={key}
                            label={
                              [
                                "最低价格",
                                "最高价格",
                                "最低涨跌幅 %",
                                "最高涨跌幅 %",
                              ][i]
                            }
                            type="number"
                            step="any"
                            value={filters[key]}
                            onChange={(e) => update(key, e.target.value)}
                          />
                        ),
                      )}
                      <Select
                        label="最近报价"
                        value={filters.fresh}
                        onValueChange={(v) => update("fresh", String(v))}
                        items={{
                          "0": "不限",
                          "1": "1 分钟内",
                          "5": "5 分钟内",
                          "30": "30 分钟内",
                          "1440": "24 小时内",
                        }}
                      />
                    </div>
                    <div className="inline-actions mt-5">
                      <Button
                        variant="ghost"
                        onClick={() => {
                          setFilters(initial);
                          setQuery("");
                          setSort("code-asc");
                        }}
                      >
                        重置条件
                      </Button>
                      <Button
                        variant="primary"
                        onClick={() => setFilterOpen(false)}
                      >
                        完成筛选
                      </Button>
                    </div>
                  </Popover.Popup>
                </Popover.Positioner>
              </Popover.Portal>
            </Popover.Root>
          </div>
        </div>
        <div className="toolbar">
          <p className="caption">
            {list.length} 支证券 · 涨跌幅以当日首条报价为基准
          </p>
          <Select
            aria-label="证券排序"
            value={sort}
            onValueChange={(v) => setSort(String(v))}
            items={sorts}
          />
        </div>
        <LayerCard className="data-card">
          {list.length ? (
            <div className="table-region">
              <Table className="min-w-[800px]" aria-label="证券行情">
                <Table.Header>
                  <Table.Row>
                    <Table.Head>证券名称</Table.Head>
                    <Table.Head className="text-right">最新价</Table.Head>
                    <Table.Head className="text-right">今日涨跌</Table.Head>
                    <Table.Head className="text-right">发行剩余</Table.Head>
                    <Table.Head className="text-right">本小时成交</Table.Head>
                    <Table.Head>报价状态</Table.Head>
                    <Table.Head>自选</Table.Head>
                    <Table.Head />
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {list.map((s) => (
                    <Table.Row key={s.id}>
                      <Table.Cell>
                        <div className="grid gap-1">
                          <LinkButton
                            variant="ghost"
                            href="/"
                            onClick={(e) => {
                              e.preventDefault();
                              route.navigate(`/security?stock=${s.id}`);
                            }}
                            className="-ml-3"
                          >
                            {s.name}
                          </LinkButton>
                          <span className="caption">
                            {s.symbol}
                            {s.id === x.account?.boundStockId
                              ? " · 我的股票"
                              : ""}
                          </span>
                        </div>
                      </Table.Cell>
                      <Table.Cell className="text-right numeric">
                        {money(s.price)}
                      </Table.Cell>
                      <Table.Cell
                        className={`text-right numeric ${(delta(s) ?? 0) >= 0 ? "gain" : "loss"}`}
                      >
                        {move(delta(s))}
                      </Table.Cell>
                      <Table.Cell className="text-right numeric">
                        {quantity(s.issuedShares - s.outstanding)}
                      </Table.Cell>
                      <Table.Cell className="text-right numeric">
                        {quantity(s.normalUsed)} / {quantity(s.hourlyVolume)}
                      </Table.Cell>
                      <Table.Cell>
                        <Badge variant="secondary">
                          {stockState(s, ttl, x.now)}
                        </Badge>
                      </Table.Cell>
                      <Table.Cell>
                        <Button
                          variant="ghost"
                          shape="square"
                          aria-label={`${watch.includes(s.id) ? "移除" : "添加"}自选 ${s.name}`}
                          aria-pressed={watch.includes(s.id)}
                          onClick={() => void star(s)}
                        >
                          <StarIcon
                            size={18}
                            weight={watch.includes(s.id) ? "fill" : "regular"}
                          />
                        </Button>
                      </Table.Cell>
                      <Table.Cell>
                        <LinkButton
                          variant="ghost"
                          href="/"
                          onClick={(e) => {
                            e.preventDefault();
                            route.navigate(`/security?stock=${s.id}`);
                          }}
                          aria-label={`查看 ${s.name} 行情`}
                        >
                          <ArrowUpRightIcon size={18} />
                        </LinkButton>
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table>
            </div>
          ) : (
            <Empty
              title="没有符合条件的证券"
              description="尝试其他关键词或重置筛选条件。"
              contents={
                <Button
                  onClick={() => {
                    setFilters(initial);
                    setQuery("");
                    setScope("all");
                  }}
                >
                  重置搜索与筛选
                </Button>
              }
            />
          )}
          <div className="data-note">
            行情来源于 AzurPilot
            实例行动力。有限库存与小时额度由服务器统一核验。
          </div>
        </LayerCard>
      </section>
    </>
  );
}
