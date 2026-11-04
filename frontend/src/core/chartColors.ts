import { ChartPalette } from "@cloudflare/kumo";

// 证券颜色统一采用红涨绿跌；均线使用固定色，不随图例顺序轮换。
export function chartColors(theme: "light" | "dark") {
  const dark = theme === "dark";
  const up = ChartPalette.semantic("Attention", dark);
  const down = ChartPalette.semantic("Success", dark);
  const flat = ChartPalette.text("primary", dark);
  const average = ChartPalette.categorical(1, dark);
  const lines: Record<string, string> = {
    成交均价: average,
    MA5: ChartPalette.categorical(0, dark),
    MA10: ChartPalette.categorical(3, dark),
    MA20: ChartPalette.categorical(4, dark),
    EXPMA12: ChartPalette.categorical(0, dark),
    EXPMA50: ChartPalette.categorical(3, dark),
    BBI: ChartPalette.categorical(2, dark),
    "ENE 上轨": ChartPalette.categorical(3, dark),
    "ENE 下轨": ChartPalette.categorical(4, dark),
    "BOLL 中轨": ChartPalette.categorical(0, dark),
    "BOLL 上轨": ChartPalette.categorical(3, dark),
    "BOLL 下轨": ChartPalette.categorical(4, dark),
    DIF: flat,
    DEA: average,
  };
  return {
    up,
    down,
    flat,
    lines,
    direction: (value: number | undefined, baseline = 0) =>
      value === undefined || value === baseline
        ? flat
        : value > baseline
          ? up
          : down,
  };
}
