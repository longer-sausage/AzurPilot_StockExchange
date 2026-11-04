"use client";
// 倒计时使用单行 HH:MM:SS:NNNNNNNNN，不显示单位；数字保持等宽。
// 层级：倒计时 > 开市时间 > 账户提示；主要操作沿用页头登录。
import { useExchange } from "../core/Exchange";
import { date } from "../core/format";
import { useCountdown } from "../core/useCountdown";
import styles from "./OpeningPage.module.css";

export function OpeningPage({ opensAt }: { opensAt: number }) {
  const x = useExchange(),
    remaining = useCountdown(opensAt, x.now),
    seconds = Number(remaining / 1_000_000_000n),
    hours = Math.floor(seconds / 3600),
    minutes = Math.floor((seconds % 3600) / 60),
    remainingSeconds = seconds % 60,
    nanoseconds = String(remaining % 1_000_000_000n).padStart(9, "0"),
    timestamp = `${[hours, minutes, remainingSeconds]
      .map((value) => String(value).padStart(2, "0"))
      .join(":")}:${nanoseconds}`;
  return (
    <section className={styles.opening} aria-labelledby="opening-title">
      <div>
        <p className="eyebrow">茗喵证券交易所</p>
        <h1 id="opening-title">开市倒计时</h1>
      </div>
      <div
        className={styles.timer}
        role="timer"
        aria-live="off"
        aria-label={`距离开市还有 ${timestamp}`}
      >
        {timestamp}
      </div>
      <div className={styles.details}>
        <p>
          <time dateTime={new Date(opensAt).toISOString()}>
            {date(opensAt)}
          </time>{" "}
          （北京时间）开市
        </p>
        <p className="caption">
          {remaining === 0n
            ? "正在确认开市状态，确认后将自动进入交易站。"
            : x.account
              ? "你已登录，可在开市前通过账户入口确认股票绑定。开市后将自动进入交易站。"
              : "开市后将自动进入交易站。你可以先登录，完成改密与股票绑定。"}
        </p>
      </div>
    </section>
  );
}
