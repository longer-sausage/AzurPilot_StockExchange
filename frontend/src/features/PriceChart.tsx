"use client";
// 价格走势为主要内容，工具栏与指标说明降一级；生命周期交给 Kumo Chart。
import { useMemo, useRef, useState } from "react";
import {
  Banner,
  Button,
  Chart,
  Checkbox,
  Dialog,
  Empty,
  Input,
  Popover,
  Select,
  Tabs,
  Toolbar,
  type KumoChartOption,
} from "@cloudflare/kumo";
import {
  ArrowsOutIcon,
  ArrowsInIcon,
  ArrowCounterClockwiseIcon,
  MinusIcon,
  PlusIcon,
} from "@phosphor-icons/react";
import * as echarts from "echarts/core";
import type { SeriesOption } from "echarts";
import { BarChart, CandlestickChart, LineChart } from "echarts/charts";
import {
  AxisPointerComponent,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  VisualMapComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { Detail, Period } from "../core/contracts";
import { date, money } from "../core/format";
import { useExchange } from "../core/Exchange";
import { chartColors } from "../core/chartColors";
echarts.use([
  BarChart,
  CandlestickChart,
  LineChart,
  AxisPointerComponent,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  VisualMapComponent,
  CanvasRenderer,
]);
const chartUpdates = {
  replaceMerge: ["series", "visualMap", "xAxis", "yAxis", "grid"],
};
const periods = [
  { value: "time", label: "分时" },
  { value: "day", label: "日 K" },
  { value: "m5", label: "M5" },
  { value: "m10", label: "M10" },
  { value: "m20", label: "M20" },
  { value: "m30", label: "M30" },
  { value: "m60", label: "M60" },
];
const notes: Record<string, string> = {
  MA: "MA：过去 N 个周期收盘价的算术均值。",
  EXPMA: "EXPMA：指数加权均线，近期价格权重更大。",
  BOLL: "BOLL：20 周期均值和上下两倍标准差轨道。",
  ENE: "ENE：10 周期均值的上下 11% / 9% 包络线。",
  BBI: "BBI：3、6、12、24 周期均线的均值。",
};
function mean(values: number[], n: number) {
  return values.map((_, i) =>
    i + 1 < n
      ? null
      : values.slice(i + 1 - n, i + 1).reduce((a, b) => a + b, 0) / n,
  );
}
function ema(values: (number | null)[], n: number) {
  let previous: number | null = null;
  return values.map((value, i) => {
    if (value === null) return null;
    if (previous === null) {
      const window = values.slice(Math.max(0, i - n + 1), i + 1);
      if (window.length < n || window.some((v) => v === null)) return null;
      previous = window.reduce<number>((sum, v) => sum + (v ?? 0), 0) / n;
    } else previous = (value * 2) / (n + 1) + (previous * (n - 1)) / (n + 1);
    return previous;
  });
}
export function PriceChart({
  detail,
  period,
  month,
  day,
  onPeriod,
  onMonth,
  onDay,
  error,
  loading,
}: {
  detail?: Detail;
  period: Period;
  month: string;
  day: string;
  onPeriod: (p: Period) => void;
  onMonth: (m: string) => void;
  onDay: (d: string) => void;
  error: string;
  loading: boolean;
}) {
  const { theme } = useExchange(),
    [indicators, setIndicators] = useState<string[]>([]),
    [sub, setSub] = useState("macd"),
    [full, setFull] = useState(false),
    [help, setHelp] = useState(false),
    chart = useRef<echarts.ECharts | null>(null),
    viewport = useRef({ start: 0, end: 100 });
  const options = useMemo<KumoChartOption>(() => {
    const colors = chartColors(theme);
    const baseline = (detail?.summary.open ?? 0) / 100;
    const bars = detail?.bars ?? [],
      prices = bars.map((b) => b.close / 100),
      times = bars.map((b) =>
        new Date(b.time).toLocaleString("zh-CN", {
          timeZone: "Asia/Shanghai",
          month: "2-digit",
          day: "2-digit",
          ...(period === "day"
            ? {}
            : { hour: "2-digit", minute: "2-digit", hour12: false }),
        }),
      );
    const series: SeriesOption[] = [];
    const add = (name: string, data: (number | null)[], axis = 0) =>
      series.push({
        name,
        type: "line",
        data,
        showSymbol: false,
        connectNulls: false,
        xAxisIndex: axis,
        yAxisIndex: axis,
        lineStyle: {
          width: name === "分时价格" ? 2.5 : 1.5,
          ...(name === "分时价格"
            ? {}
            : { color: colors.lines[name] ?? colors.flat }),
        },
        itemStyle: { color: colors.lines[name] ?? colors.flat },
        animation: false,
      });
    if (period === "time") {
      add("分时价格", prices);
      let volume = 0,
        turnover = 0;
      add(
        "成交均价",
        bars.map((b) => {
          volume += b.volume;
          turnover += b.turnover;
          return volume ? turnover / volume / 100 : null;
        }),
      );
    } else
      series.push({
        name: "K 线",
        type: "candlestick",
        data: bars.map((b) => [
          b.open / 100,
          b.close / 100,
          b.low / 100,
          b.high / 100,
        ]),
        itemStyle: {
          color: colors.up,
          color0: colors.down,
          borderColor: colors.up,
          borderColor0: colors.down,
          borderColorDoji: colors.flat,
        },
      });
    const last = bars.at(-1);
    if (last)
      Object.assign(series[0], {
        markLine: {
          silent: true,
          symbol: "none",
          label: { show: false },
          data: [
            ...(period === "time" && baseline > 0
              ? [
                  {
                    yAxis: baseline,
                    lineStyle: { color: colors.flat, type: "dashed", width: 1 },
                  },
                ]
              : []),
            {
              yAxis: last.close / 100,
              lineStyle: {
                color: colors.direction(last.close, detail?.summary.open),
                type: "dotted",
                width: 1,
                opacity: 0.6,
              },
              label: {
                show: true,
                position: "end",
                formatter: money(last.close),
                fontSize: 14,
                fontWeight: 600,
                color: colors.flat,
                opacity: 1,
              },
            },
          ],
        },
      });
    for (const kind of indicators) {
      if (kind === "MA")
        for (const n of [5, 10, 20]) add(`MA${n}`, mean(prices, n));
      if (kind === "EXPMA")
        for (const n of [12, 50]) add(`EXPMA${n}`, ema(prices, n));
      if (kind === "BBI") {
        const lines = [3, 6, 12, 24].map((n) => mean(prices, n));
        add(
          "BBI",
          prices.map((_, i) =>
            lines.every((l) => l[i] !== null)
              ? lines.reduce((sum, l) => sum + l[i]!, 0) / 4
              : null,
          ),
        );
      }
      if (kind === "ENE") {
        const avg = mean(prices, 10);
        add(
          "ENE 上轨",
          avg.map((v) => (v === null ? null : v * 1.11)),
        );
        add(
          "ENE 下轨",
          avg.map((v) => (v === null ? null : v * 0.91)),
        );
      }
      if (kind === "BOLL") {
        const avg = mean(prices, 20),
          sd = prices.map((_, i) =>
            avg[i] === null
              ? null
              : Math.sqrt(
                  prices
                    .slice(i - 19, i + 1)
                    .reduce((sum, v) => sum + (v - avg[i]!) ** 2, 0) / 20,
                ),
          );
        add("BOLL 中轨", avg);
        add(
          "BOLL 上轨",
          avg.map((v, i) => (v === null ? null : v + 2 * sd[i]!)),
        );
        add(
          "BOLL 下轨",
          avg.map((v, i) => (v === null ? null : v - 2 * sd[i]!)),
        );
      }
    }
    series.push({
      name: "成交量",
      type: "bar",
      xAxisIndex: 1,
      yAxisIndex: 1,
      itemStyle: { color: colors.flat },
      data: bars.map((b) => ({
        value: b.volume,
        itemStyle: { color: colors.direction(b.close, b.open) },
      })),
      barMaxWidth: 12,
    });
    const short = ema(prices, 12),
      long = ema(prices, 26),
      dif = prices.map((_, i) =>
        short[i] === null || long[i] === null ? null : short[i]! - long[i]!,
      ),
      dea = ema(dif, 9);
    if (sub === "macd") {
      add("DIF", dif, 2);
      add("DEA", dea, 2);
      series.push({
        name: "MACD",
        type: "bar",
        xAxisIndex: 2,
        yAxisIndex: 2,
        itemStyle: { color: colors.flat },
        data: dif.map((v, i) => {
          if (v === null || dea[i] === null) return null;
          const value = 2 * (v - dea[i]!);
          return { value, itemStyle: { color: colors.direction(value) } };
        }),
      });
    }
    const grids =
      sub === "macd"
        ? [
            { left: 72, right: 88, top: 36, height: "48%" },
            { left: 72, right: 88, top: "64%", height: "10%" },
            { left: 72, right: 88, top: "82%", height: "8%" },
          ]
        : [
            { left: 72, right: 88, top: 36, height: "58%" },
            { left: 72, right: 88, top: "76%", height: "12%" },
          ];
    return {
      animation: false,
      backgroundColor: "transparent",
      visualMap:
        period === "time" && baseline > 0
          ? [
              {
                type: "piecewise",
                show: false,
                seriesIndex: 0,
                dimension: 1,
                pieces: [
                  {
                    gte: Math.min(...prices, baseline) - 1,
                    lt: baseline,
                    color: colors.down,
                  },
                  { value: baseline, color: colors.flat },
                  {
                    gt: baseline,
                    lte: Math.max(...prices, baseline) + 1,
                    color: colors.up,
                  },
                ],
              },
            ]
          : [],
      legend: { type: "scroll", top: 0, textStyle: { fontSize: 11 } },
      grid: grids,
      xAxis: grids.map((_, i) => ({
        type: "category",
        gridIndex: i,
        data: times,
        boundaryGap: period !== "time",
        axisLabel: {
          show: i === grids.length - 1,
          fontSize: 10,
          hideOverlap: true,
        },
        axisTick: { show: false },
      })),
      yAxis: grids.map((_, i) => ({
        type: "value",
        gridIndex: i,
        scale: i === 0,
        splitNumber: i === 0 ? 4 : 2,
        axisLabel: {
          fontSize: i === 0 ? 12 : 10,
          ...(i === 0
            ? { formatter: (value: number) => money(Math.round(value * 100)) }
            : {}),
        },
        splitLine: { show: i === 0 },
        name: i === 1 ? "成交量" : i === 2 ? "MACD" : "价格",
      })),
      tooltip: {
        trigger: "axis",
        renderMode: "richText",
        confine: true,
        axisPointer: { type: "cross" },
      },
      axisPointer: { link: [{ xAxisIndex: "all" }] },
      dataZoom: [
        {
          type: "inside",
          xAxisIndex: grids.map((_, i) => i),
          start: Math.max(
            viewport.current.start,
            (Math.max(
              0,
              bars.findIndex((b) => b.time >= (detail?.displayFrom ?? 0)),
            ) /
              Math.max(1, bars.length - 1)) *
              100,
          ),
          end: viewport.current.end,
        },
        {
          type: "slider",
          xAxisIndex: grids.map((_, i) => i),
          height: 16,
          bottom: 0,
          start: Math.max(
            viewport.current.start,
            (Math.max(
              0,
              bars.findIndex((b) => b.time >= (detail?.displayFrom ?? 0)),
            ) /
              Math.max(1, bars.length - 1)) *
              100,
          ),
          end: viewport.current.end,
          showDataShadow: false,
        },
      ],
      series,
    };
  }, [detail, period, indicators, sub, theme]);
  function zoom(direction: "in" | "out" | "reset") {
    const current = chart.current;
    if (!current) return;
    const v = viewport.current,
      span = v.end - v.start,
      change = direction === "in" ? span * 0.2 : -span * 0.25,
      next =
        direction === "reset"
          ? { start: 0, end: 100 }
          : {
              start: Math.max(0, Math.min(v.end - 1, v.start + change)),
              end: v.end,
            };
    viewport.current = next;
    current.dispatchAction({ type: "dataZoom", ...next });
  }
  const content = (
    <div className="chart-content">
      <dl
        className="flex flex-wrap items-end gap-x-6 gap-y-3"
        aria-label="图表价格摘要"
      >
        <div>
          <dt className="caption">图中末价</dt>
          <dd className="text-2xl font-semibold tabular-nums">
            {detail?.bars.length ? money(detail.bars.at(-1)!.close) : "—"}
          </dd>
        </div>
        {(
          [
            ["开盘", detail?.summary.open],
            ["最高", detail?.summary.high],
            ["最低", detail?.summary.low],
          ] as const
        ).map(([label, value]) => (
          <div key={label}>
            <dt className="caption">{label}</dt>
            <dd className="numeric">
              {value === undefined ? "—" : money(value)}
            </dd>
          </div>
        ))}
      </dl>
      <div className="chart-toolbar">
        <Tabs
          value={period}
          onValueChange={(value) => {
            viewport.current = { start: 0, end: 100 };
            onPeriod(value as Period);
          }}
          tabs={periods}
        />
        <Button
          variant="ghost"
          shape="square"
          aria-label={full ? "退出图表全屏" : "全屏走势图"}
          onClick={() => setFull(!full)}
        >
          {full ? <ArrowsInIcon size={18} /> : <ArrowsOutIcon size={18} />}
        </Button>
      </div>
      <div className="chart-toolbar">
        <div className="toolbar-group">
          <Input
            aria-label="行情月份"
            type="month"
            value={month}
            onChange={(e) => {
              if (e.target.value) {
                viewport.current = { start: 0, end: 100 };
                onMonth(e.target.value);
              }
            }}
          />
          {period === "time" && (
            <Input
              aria-label="分时日期"
              type="date"
              value={day || detail?.day || ""}
              min={`${month}-01`}
              onChange={(e) => onDay(e.target.value)}
            />
          )}
        </div>
        <div className="toolbar-group">
          <Popover.Root>
            <Popover.Trigger render={<Button variant="secondary" />}>
              技术指标 ({indicators.length})
            </Popover.Trigger>
            <Popover.Portal>
              <Popover.Positioner sideOffset={8}>
                <Popover.Popup className="w-64 p-5">
                  <Popover.Title className="mb-4 font-semibold">
                    叠加指标
                  </Popover.Title>
                  <div className="grid gap-4">
                    {Object.keys(notes).map((name) => (
                      <Checkbox
                        key={name}
                        label={name}
                        checked={indicators.includes(name)}
                        onCheckedChange={(checked) =>
                          setIndicators((values) =>
                            checked
                              ? [...values.filter((v) => v !== name), name]
                              : values.filter((v) => v !== name),
                          )
                        }
                      />
                    ))}
                  </div>
                </Popover.Popup>
              </Popover.Positioner>
            </Popover.Portal>
          </Popover.Root>
          <Select
            aria-label="图表副图"
            value={sub}
            onValueChange={(v) => setSub(String(v))}
            items={{ macd: "成交量 + MACD", volume: "仅成交量" }}
          />
        </div>
      </div>
      {error ? (
        <Banner variant="error" description={error} />
      ) : loading ? (
        <Empty title="正在读取行情" />
      ) : !detail?.bars.length ? (
        <Empty
          title="这个时段还没有行情"
          description="选择其他月份或日期。空档不会补造报价。"
        />
      ) : (
        <div role="img" aria-label="证券价格、成交量与技术指标走势图">
          <Chart
            ref={chart}
            echarts={echarts}
            options={options}
            optionUpdateBehavior={chartUpdates}
            isDarkMode={theme === "dark"}
            height={full ? Math.max(400, innerHeight - 300) : 440}
            onEvents={{
              datazoom: () => {
                const option = chart.current?.getOption().dataZoom as
                  { start: number; end: number }[] | undefined;
                if (option?.[0])
                  viewport.current = {
                    start: option[0].start,
                    end: option[0].end,
                  };
              },
            }}
          />
        </div>
      )}
      <div className="chart-toolbar">
        <Toolbar aria-label="图表缩放">
          <Toolbar.Button
            shape="square"
            aria-label="放大走势图"
            onClick={() => zoom("in")}
          >
            <PlusIcon size={16} />
          </Toolbar.Button>
          <Toolbar.Button
            shape="square"
            aria-label="缩小走势图"
            onClick={() => zoom("out")}
          >
            <MinusIcon size={16} />
          </Toolbar.Button>
          <Toolbar.Button
            shape="square"
            aria-label="重置走势图"
            onClick={() => zoom("reset")}
          >
            <ArrowCounterClockwiseIcon size={16} />
          </Toolbar.Button>
        </Toolbar>
        <Button
          variant="ghost"
          onClick={() => setHelp((v) => !v)}
          aria-expanded={help}
        >
          指标说明
        </Button>
      </div>
      {help && (
        <div className="prose">
          {indicators.map((name) => (
            <p key={name}>{notes[name]}</p>
          ))}
          <p>
            MACD：DIF = EMA12 − EMA26，DEA = EMA9(DIF)，柱 = 2 × (DIF −
            DEA)。样本不足时不绘制指标。分时均价按累计实际成交额 /
            股数计算。指标不保证未来收益。
          </p>
        </div>
      )}
      <div className="chart-metadata">
        <span>
          {period === "time"
            ? "分时红绿相对所选日开盘价"
            : "K线与成交量：红涨绿跌"}{" "}
          · MACD 红正绿负
        </span>
        <span>
          {detail?.coverage.count ?? 0} 条月原始记录 ·{" "}
          {detail?.coverage.reconciledAt
            ? `校对 ${date(detail.coverage.reconciledAt)}`
            : "等待后台校对"}
        </span>
        <span>上海时间 · 滚轮缩放 / 拖动平移</span>
      </div>
    </div>
  );
  return (
    <>
      {!full && content}
      <Dialog.Root open={full} onOpenChange={setFull}>
        <Dialog
          size="xl"
          className="max-w-[calc(100vw-2rem)] w-[calc(100vw-2rem)] p-5"
        >
          <Dialog.Title>{detail?.stock.name} · 全屏走势图</Dialog.Title>
          {full && content}
        </Dialog>
      </Dialog.Root>
    </>
  );
}
