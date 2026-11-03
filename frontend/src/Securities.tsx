"use client";
import { TrendUpIcon, TrendDownIcon } from "@phosphor-icons/react";
import { Badge, Button, Input, LayerCard, Select } from "@cloudflare/kumo";
import { useMemo, useState } from "react";
import {
  MagnifyingGlassIcon as Search,
  StarIcon as Star,
  SlidersHorizontalIcon as SlidersHorizontal,
} from "@phosphor-icons/react";
import { money } from "./api";
import type { Account, Portfolio, Settings, Stock } from "./types";
const dateFormatter = new Intl.DateTimeFormat("sv-SE", {
  timeZone: "Asia/Shanghai",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});
export const today = () => dateFormatter.format(Date.now());
export const change = (s: Stock) =>
  s.openDay === today() && s.open > 0
    ? ((s.price - s.open) / s.open) * 100
    : undefined;
export function quoteState(s: Stock, ttl: number, now = Date.now()) {
  return s.deletedAt
    ? "已删除"
    : s.delisted
      ? "本月退市"
      : s.disabled
        ? "停牌"
        : s.price <= 0
          ? "零报价"
          : now - s.observedAt > ttl * 1000
            ? "报价过期"
            : "可交易";
}
const defaults = {
  scope: "all",
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
export function Securities({
  stocks,
  account,
  portfolio,
  settings,
  selected,
  onSelect,
  onStar,
  now = Date.now(),
}: {
  now?: number;
  stocks: Stock[];
  account?: Account;
  portfolio?: Portfolio;
  settings?: Settings;
  selected?: number;
  onSelect: (id: number) => void;
  onStar: (s: Stock) => void;
}) {
  const [query, setQuery] = useState(""),
    [filters, setFilters] = useState(defaults),
    [expanded, setExpanded] = useState(false),
    [sort, setSort] = useState("code-asc");
  const update = (key: keyof typeof filters, value: string) =>
    setFilters((f) => ({ ...f, [key]: value }));
  const ttl =
    settings?.activeMarket.id === "meow"
      ? settings.quoteTTLSeconds
      : (settings?.activeMarket.quoteTTLSeconds ?? 86400);
  const watch = account?.watchlist ?? [],
    owned = portfolio?.positions.map((p) => p.stockId) ?? [],
    count = Object.entries(filters).filter(
      ([k, v]) => v !== defaults[k as keyof typeof defaults],
    ).length;
  const dayKey = today(),
    timeKey =
      filters.state === "all" && filters.fresh === "0"
        ? 0
        : Math.floor(now / 1000);
  const list = useMemo(
    () =>
      stocks
        .filter((s) => {
          const c = change(s),
            state = quoteState(s, ttl, now);
          return (
            `${s.name} ${s.symbol} ${s.id}`
              .toLowerCase()
              .includes(query.toLowerCase()) &&
            (filters.scope === "all" ||
              (filters.scope === "watch" ? watch : owned).includes(s.id)) &&
            (filters.listing === "all" ||
              (filters.listing === "delisted" ? !!s.delisted : !s.delisted)) &&
            (filters.state === "all" || state === filters.state) &&
            (filters.direction === "all" ||
              (filters.direction === "unknown"
                ? c === undefined
                : filters.direction === "up"
                  ? c !== undefined && c > 0
                  : filters.direction === "down"
                    ? c !== undefined && c < 0
                    : c === 0)) &&
            (filters.owner === "all" ||
              (filters.owner === "mine"
                ? s.id === account?.boundStockId
                : s.id !== account?.boundStockId)) &&
            (!filters.min || s.price >= Number(filters.min) * 100) &&
            (!filters.max || s.price <= Number(filters.max) * 100) &&
            (!filters.minChange ||
              (c !== undefined && c >= Number(filters.minChange))) &&
            (!filters.maxChange ||
              (c !== undefined && c <= Number(filters.maxChange))) &&
            (!Number(filters.fresh) ||
              now - s.observedAt <= Number(filters.fresh) * 60000)
          );
        })
        .sort((a, b) => {
          const [field, dir] = sort.split("-"),
            sign = dir === "desc" ? -1 : 1;
          if (field === "name")
            return sign * a.name.localeCompare(b.name, "zh-CN");
          if (field === "change") {
            const av = change(a),
              bv = change(b);
            if (av === undefined) return bv === undefined ? a.id - b.id : 1;
            if (bv === undefined) return -1;
            return sign * (av - bv) || a.id - b.id;
          }
          return (
            sign *
              ((field === "price"
                ? a.price
                : field === "time"
                  ? a.observedAt
                  : a.id) -
                (field === "price"
                  ? b.price
                  : field === "time"
                    ? b.observedAt
                    : b.id)) || a.id - b.id
          );
        }),
    [stocks, query, filters, sort, account, portfolio, ttl, timeKey, dayKey],
  );
  return (
    <LayerCard className="panel stock-list">
      <div className="panel-heading">
        <h2>
          证券列表 <small>{list.length}</small>
        </h2>
        <Button
          variant="ghost"
          shape="square"
          className="icon"
          aria-label="展开筛选"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          <SlidersHorizontal size={17} />
          {count > 0 && count}
        </Button>
      </div>
      <label className="search">
        <Search size={16} />
        <Input
          aria-label="搜索股票"
          placeholder="名称、代码或编号"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </label>
      <div className="list-tabs">
        {[
          ["all", "全部证券"],
          ["watch", `自选股 ${watch.length}`],
          ["positions", "持仓证券"],
        ].map(([id, label]) => (
          <Button
            key={id}
            variant={filters.scope === id ? "secondary" : "ghost"}
            className={filters.scope === id ? "active" : ""}
            onClick={() => update("scope", id)}
          >
            {label}
          </Button>
        ))}
      </div>
      {expanded && (
        <div className="stock-filters">
          <label>
            上市范围
            <Select<string>
              value={filters.listing}
              onValueChange={(value) => update("listing", value ?? "")}
              aria-label="上市范围"
              items={[
                { value: "listed", label: "未退市" },
                { value: "all", label: "全部" },
                { value: "delisted", label: "已退市" },
              ]}
            />
          </label>
          <label>
            报价状态
            <Select<string>
              value={filters.state}
              onValueChange={(value) => update("state", value ?? "")}
              aria-label="报价状态"
              items={[
                { value: "all", label: "全部状态" },
                ...["可交易", "报价过期", "零报价", "停牌"].map((s) => ({
                  value: s,
                  label: s,
                })),
              ]}
            />
          </label>
          <label>
            涨跌方向
            <Select<string>
              value={filters.direction}
              onValueChange={(value) => update("direction", value ?? "")}
              aria-label="涨跌方向"
              items={[
                ...[
                  ["all", "全部"],
                  ["up", "上涨"],
                  ["down", "下跌"],
                  ["flat", "平盘"],
                  ["unknown", "无开盘价"],
                ].map(([v, l]) => ({ value: v, label: l })),
              ]}
            />
          </label>
          <label>
            归属
            <Select<string>
              value={filters.owner}
              onValueChange={(value) => update("owner", value ?? "")}
              aria-label="归属"
              items={[
                { value: "all", label: "全部" },
                { value: "mine", label: "我的股票" },
                { value: "others", label: "其他玩家" },
              ]}
            />
          </label>
          {(["min", "max", "minChange", "maxChange"] as const).map((key, i) => (
            <label key={key}>
              {["最低价格", "最高价格", "最低涨跌幅 %", "最高涨跌幅 %"][i]}
              <Input
                aria-label={
                  ["最低价格", "最高价格", "最低涨跌幅", "最高涨跌幅"][i]
                }
                type="number"
                step="any"
                value={filters[key]}
                onChange={(e) => update(key, e.target.value)}
              />
            </label>
          ))}
          <label>
            最近报价
            <Select<string>
              value={filters.fresh}
              onValueChange={(value) => update("fresh", value ?? "")}
              aria-label="最近报价"
              items={[
                ...[
                  ["0", "不限时间"],
                  ["1", "1 分钟"],
                  ["5", "5 分钟"],
                  ["30", "30 分钟"],
                  ["1440", "24 小时"],
                ].map(([v, l]) => ({ value: v, label: l })),
              ]}
            />
          </label>
          <Button
            onClick={() => {
              setFilters(defaults);
              setQuery("");
              setSort("code-asc");
            }}
          >
            重置筛选与排序
          </Button>
        </div>
      )}
      <label className="sort-select">
        排序
        <Select<string>
          aria-label="证券排序"
          value={sort}
          onValueChange={(value) => setSort(value ?? "")}
          items={[
            ...[
              ["code", "代码"],
              ["name", "名称"],
              ["price", "价格"],
              ["change", "涨跌幅"],
              ["time", "报价时间"],
            ].flatMap(([v, l]) =>
              [
                ["asc", "升序"],
                ["desc", "降序"],
              ].map(([d, t]) => ({
                value: `${v}-${d}`,
                label: (
                  <>
                    {l} · {t}
                  </>
                ),
              })),
            ),
          ]}
        />
      </label>
      <div className="list-columns">
        <Button
          onClick={() =>
            setSort(sort === "name-asc" ? "name-desc" : "name-asc")
          }
        >
          股票 / 编号
        </Button>
        <Button
          onClick={() =>
            setSort(sort === "price-desc" ? "price-asc" : "price-desc")
          }
        >
          最新价
        </Button>
        <Button
          onClick={() =>
            setSort(sort === "change-desc" ? "change-asc" : "change-desc")
          }
        >
          涨跌幅
        </Button>
      </div>
      <div className="stock-rows">
        {list.map((s) => (
          <div
            key={s.id}
            className={`stock-row ${selected === s.id ? "selected" : ""}`}
          >
            <span>
              <Button
                variant="ghost"
                className="text-button security-name"
                onClick={() => onSelect(s.id)}
              >
                {s.name}
                {s.id === account?.boundStockId && <small>我</small>}
              </Button>
              <small>
                {s.symbol} · {quoteState(s, ttl, now)}
              </small>
              <Badge variant="secondary">
                {s.openDay !== today() || s.open <= 0 ? (
                  "—"
                ) : s.price === s.open ? (
                  "持平"
                ) : s.price > s.open ? (
                  <TrendUpIcon size={14} />
                ) : (
                  <TrendDownIcon size={14} />
                )}
              </Badge>
            </span>
            <Button
              variant="ghost"
              className="text-button"
              onClick={() => onSelect(s.id)}
            >
              {money(s.price)}
            </Button>
            <span className={(change(s) ?? 0) >= 0 ? "up" : "down"}>
              {change(s) === undefined
                ? "—"
                : `${change(s)! >= 0 ? "+" : ""}${change(s)!.toFixed(2)}%`}
              <Button
                className="star-button"
                aria-label={`${watch.includes(s.id) ? "移除" : "添加"}自选 ${s.name}`}
                aria-pressed={watch.includes(s.id)}
                onClick={() => onStar(s)}
              >
                <Star
                  size={14}
                  weight={watch.includes(s.id) ? "fill" : "regular"}
                />
              </Button>
            </span>
          </div>
        ))}
      </div>
      {!list.length && <p className="empty">没有符合筛选条件的证券。</p>}
      <div className="list-foot">涨跌幅基于上海当日首条报价</div>
    </LayerCard>
  );
}
