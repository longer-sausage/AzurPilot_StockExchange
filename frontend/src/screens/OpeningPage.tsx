"use client";
// 层级：剩余时分秒 > 开市时间 > 账户准备提示；唯一主要操作沿用页头登录。
// 复用 Kumo 主题色与 12/24/32/64px 间距，数字使用 48/96px 和等宽数字。
import { useExchange } from "../core/Exchange";
import { date } from "../core/format";
import styles from "./OpeningPage.module.css";

export function OpeningPage({ opensAt }: { opensAt: number }) {
  const x = useExchange(),
    seconds = Math.max(0, Math.ceil((opensAt - x.now) / 1000)),
    hours = Math.floor(seconds / 3600),
    minutes = Math.floor((seconds % 3600) / 60),
    remainingSeconds = seconds % 60;
  return (
    <section className={styles.opening} aria-labelledby="opening-title">
      <div>
        <p className="eyebrow">茗喵证券交易所</p>
        <h1 id="opening-title">开市倒计时</h1>
      </div>
      <div
        className={styles.timer}
        role="timer"
        aria-label={`距离开市还有 ${hours} 小时 ${minutes} 分 ${remainingSeconds} 秒`}
      >
        {[
          [hours, "小时"],
          [minutes, "分钟"],
          [remainingSeconds, "秒"],
        ].map(([value, label], index) => (
          <div className={styles.unit} key={label} aria-hidden="true">
            <span className={styles.digits}>
              {index > 0 && <span className={styles.colon}>:</span>}
              {String(value).padStart(2, "0")}
            </span>
            <span className="caption">{label}</span>
          </div>
        ))}
      </div>
      <div className={styles.details}>
        <p>
          <time dateTime={new Date(opensAt).toISOString()}>
            {date(opensAt)}
          </time>{" "}
          （北京时间）开市
        </p>
        <p className="caption">
          {seconds === 0
            ? "正在确认开市状态，确认后将自动进入交易站。"
            : x.account
              ? "你已登录，可在开市前通过账户入口确认股票绑定。开市后将自动进入交易站。"
              : "开市后将自动进入交易站。你可以先登录，完成改密与股票绑定。"}
        </p>
      </div>
    </section>
  );
}
