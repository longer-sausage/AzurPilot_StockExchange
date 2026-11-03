"use client";
// 行内价格为主要内容，真实分钟走势为辅助内容；沿用 Kumo Chart 和主题色板。
// 只绘制服务端已有采样，不追加报价端点、不平滑、不补造空档。
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Button,
  Chart,
  ChartPalette,
  type KumoChartOption,
} from "@cloudflare/kumo";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { QuoteChart, Stock } from "../core/contracts";
import { useExchange } from "../core/Exchange";
import { date, money } from "../core/format";
import { request } from "../core/http";

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer]);
const cache = new Map<number, { key: string; data: QuoteChart }>();
const pending = new Map<string, Promise<QuoteChart>>();
function load(id: number, key: string, from: number) {
  if (cache.get(id)?.key === key) return Promise.resolve(cache.get(id)!.data);
  const existing = pending.get(key);
  if (existing) return existing;
  const task = request<QuoteChart>(
    `/public/stocks/${id}/chart?from=${from}&to=${from + 86400000 - 1}&interval=1`,
  )
    .then((data) => {
      cache.set(id, { key, data });
      return data;
    })
    .finally(() => pending.delete(key));
  pending.set(key, task);
  return task;
}
export function StockSparkline({ stock }: { stock: Stock }) {
  const { theme } = useExchange();
  const host = useRef<HTMLDivElement>(null);
  const from =
    Math.floor((stock.observedAt + 28800000) / 86400000) * 86400000 - 28800000;
  const key = `${stock.id}:${from}:${stock.historyVersion}:${stock.observedAt}`;
  const [visible, setVisible] = useState(false),
    [result, setResult] = useState(() => cache.get(stock.id)),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        setVisible(entry.isIntersecting);
      },
      { rootMargin: "160px" },
    );
    if (host.current) observer.observe(host.current);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!visible || stock.observedAt <= 0) return;
    let active = true;
    setError("");
    void load(stock.id, key, from)
      .then((data) => {
        if (active) setResult({ key, data });
      })
      .catch((e: Error) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [visible, stock.id, stock.observedAt, from, key, retry]);
  // 同日新版本读取期间保留已有真实曲线，避免每次行情通知都闪成加载文字。
  const points = result?.key.startsWith(`${stock.id}:${from}:`)
    ? result.data.points
    : undefined;
  const options = useMemo<KumoChartOption>(() => {
    const isolated = new Set(
      points
        ?.filter(
          (p, i) =>
            (i === 0 || p.t - points[i - 1].t > 60000) &&
            (i === points.length - 1 || points[i + 1].t - p.t > 60000),
        )
        .map((p) => p.t),
    );
    return {
      animation: false,
      grid: { left: 2, right: 2, top: 4, bottom: 4 },
      xAxis: { type: "time", show: false },
      yAxis: { type: "value", scale: true, show: false },
      tooltip: {
        trigger: "axis",
        renderMode: "html",
        appendTo: "body",
        formatter: (params: unknown) => {
          const value = (params as { value: [number, number | null] }[])[0]
            ?.value;
          return value && value[1] !== null
            ? `${date(value[0])}<br/>${money(value[1])}`
            : "";
        },
      },
      series: [
        {
          type: "line",
          smooth: false,
          showSymbol: true,
          symbolSize: (value: unknown) =>
            isolated.has((value as [number, number])[0]) ? 4 : 0,
          lineStyle: {
            width: 1.5,
            color: ChartPalette.categorical(0, theme === "dark"),
          },
          itemStyle: { color: ChartPalette.categorical(0, theme === "dark") },
          // null 仅用于断开缺失分钟，不代表报价，不跨空档连出虚假的直线。
          data:
            points?.flatMap((p, i) =>
              i > 0 && p.t - points[i - 1].t > 60000
                ? [
                    [points[i - 1].t + 60000, null],
                    [p.t, p.c],
                  ]
                : [[p.t, p.c]],
            ) ?? [],
        },
      ],
    };
  }, [points, theme]);
  return (
    <div
      ref={host}
      className="w-32 h-10"
      aria-busy={result?.key !== key && !error && stock.observedAt > 0}
    >
      {error ? (
        <Button
          variant="ghost"
          size="sm"
          title={error}
          onClick={(e) => {
            e.stopPropagation();
            setRetry((v) => v + 1);
          }}
        >
          重试走势
        </Button>
      ) : stock.observedAt <= 0 || points?.length === 0 ? (
        <span className="caption">暂无行情</span>
      ) : points ? (
        <div
          role="img"
          aria-label={`${stock.name} ${date(from).split(" ")[0]} 分钟报价走势，共 ${points.length} 个采样`}
        >
          <Chart
            echarts={echarts}
            options={options}
            height={40}
            isDarkMode={theme === "dark"}
          />
        </div>
      ) : (
        <span className="caption">读取走势…</span>
      )}
    </div>
  );
}
