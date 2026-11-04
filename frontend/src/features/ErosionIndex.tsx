"use client";
// 层级：指数点位 > 涨跌 > 名称；只展示行情本身，不添加解释或标识。
// 沿用 Kumo 主题、现有涨跌色及 12/32px 字号，移动端指数单独占一行。
import { useMemo } from "react";
import type { Stock } from "../core/contracts";
import { calculateErosionIndex, formatIndexNumber } from "../core/erosionIndex";
import styles from "./ErosionIndex.module.css";

export function ErosionIndex({
  stocks,
  now,
  quoteTTLSeconds,
}: {
  stocks: readonly Stock[];
  now: number;
  quoteTTLSeconds: number;
}) {
  const index = useMemo(
    () => calculateErosionIndex(stocks, now, quoteTTLSeconds),
    [stocks, now, quoteTTLSeconds],
  );
  const direction =
    !index || index.changeHundredths === 0n
      ? styles.flat
      : index.changeHundredths > 0n
        ? "gain"
        : "loss";
  return (
    <div className={styles.index} role="group" aria-label="侵蚀指数">
      <dt>侵蚀指数</dt>
      <dd className={`${styles.value} ${direction}`}>
        {index ? formatIndexNumber(index.pointsHundredths) : "—"}
      </dd>
      <dd className={`${styles.change} ${direction}`}>
        {index
          ? `${formatIndexNumber(index.changeHundredths, true)} (${formatIndexNumber(index.changeBasisPoints, true)}%)`
          : "—"}
      </dd>
    </div>
  );
}
