import type { Stock } from "./contracts";
export const money = (value = 0) =>
  new Intl.NumberFormat("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value / 100);
export const quantity = (value = 0) =>
  new Intl.NumberFormat("zh-CN").format(value);
export const percent = (ppm = 0) => `${(ppm / 10000).toFixed(2)}%`;
export const date = (ms: number) =>
  ms
    ? new Date(ms).toLocaleString("zh-CN", {
        timeZone: "Asia/Shanghai",
        hour12: false,
      })
    : "—";
export const day = () =>
  new Date().toLocaleDateString("sv-SE", { timeZone: "Asia/Shanghai" });
export const delta = (stock: Stock) =>
  stock.openDay === day() && stock.open > 0
    ? ((stock.price - stock.open) / stock.open) * 100
    : undefined;
export const move = (value: number | undefined) =>
  value === undefined ? "—" : `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
export const stockState = (s: Stock, ttl: number, now = Date.now()) =>
  s.deletedAt
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
export function cents(text: string, signed = false) {
  if (!(signed ? /^[+-]?\d+(?:\.\d{1,2})?$/ : /^\d+(?:\.\d{1,2})?$/).test(text))
    throw Error("金额最多保留两位小数");
  const negative = text.startsWith("-"),
    [whole, fraction = ""] = text.replace(/^[+-]/, "").split(".");
  const result =
    (Number(whole) * 100 + Number(fraction.padEnd(2, "0"))) *
    (negative ? -1 : 1);
  if (!Number.isSafeInteger(result) || (!signed && result <= 0))
    throw Error("金额超出有效范围");
  return result;
}
export const sides = {
  buy: "买入",
  sell: "卖出",
  short: "卖空",
  cover: "回补",
};
export const kinds: Record<string, string> = {
  limit: "限价",
  market: "市价",
  stop: "止损触发市价",
};
export const statuses: Record<string, string> = {
  pending: "待成交",
  partial: "部分成交",
  filled: "已成交",
  cancelled: "已撤销",
  rejected: "已拒绝",
  expired: "已过期",
};
