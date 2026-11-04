import type { Stock } from "./contracts";

export interface ErosionIndexSnapshot {
  pointsHundredths: bigint;
  changeHundredths: bigint;
  changeBasisPoints: bigint;
  samples: number;
}

function roundRatio(numerator: bigint, denominator: bigint) {
  const magnitude = numerator < 0n ? -numerator : numerator;
  const rounded = (magnitude + denominator / 2n) / denominator;
  return numerator < 0n ? -rounded : rounded;
}

// 当日开盘为 1000 点，同一成分集合的价格按发行股数加权。
// 中间乘积与求和使用 BigInt，最后才舍入，避免市值累计超过安全整数。
export function calculateErosionIndex(
  stocks: readonly Stock[],
  now: number,
  quoteTTLSeconds: number,
): ErosionIndexSnapshot | undefined {
  const today = new Date(now).toLocaleDateString("sv-SE", {
    timeZone: "Asia/Shanghai",
  });
  let current = 0n,
    opening = 0n,
    samples = 0;
  for (const stock of stocks) {
    if (
      stock.deletedAt ||
      stock.delisted ||
      stock.openDay !== today ||
      ![stock.price, stock.open, stock.issuedShares, stock.observedAt].every(
        (value) => Number.isSafeInteger(value) && value > 0,
      ) ||
      now - stock.observedAt > quoteTTLSeconds * 1000
    )
      continue;
    const shares = BigInt(stock.issuedShares);
    current += BigInt(stock.price) * shares;
    opening += BigInt(stock.open) * shares;
    samples++;
  }
  if (opening === 0n) return;
  const pointsHundredths = roundRatio(current * 100_000n, opening);
  return {
    pointsHundredths,
    changeHundredths: pointsHundredths - 100_000n,
    changeBasisPoints: roundRatio((current - opening) * 10_000n, opening),
    samples,
  };
}

export function formatIndexNumber(value: bigint, signed = false) {
  const magnitude = value < 0n ? -value : value;
  const sign = value < 0n ? "-" : signed && value > 0n ? "+" : "";
  return `${sign}${(magnitude / 100n).toLocaleString("zh-CN")}.${String(
    magnitude % 100n,
  ).padStart(2, "0")}`;
}
