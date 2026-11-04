"use client";
import { useEffect, useState } from "react";

// 只在倒计时内部使用整数纳秒；API 与全站行情时间仍为 Unix 毫秒。
export function countdownNanoseconds(
  opensAt: number,
  wallTime: number,
  elapsedMilliseconds = 0,
) {
  const remaining =
    BigInt(opensAt - wallTime) * 1_000_000n -
    BigInt(Math.round(elapsedMilliseconds * 1_000_000));
  return remaining > 0n ? remaining : 0n;
}

export function useCountdown(opensAt: number, now: number) {
  const [remaining, setRemaining] = useState(() =>
    countdownNanoseconds(opensAt, now),
  );
  useEffect(() => {
    const wallTime = Date.now(),
      monotonicTime = performance.now();
    let frame: number | undefined;
    const update = () => {
      const value = countdownNanoseconds(
        opensAt,
        wallTime,
        performance.now() - monotonicTime,
      );
      setRemaining(value);
      if (value > 0n) frame = requestAnimationFrame(update);
    };
    update();
    return () => {
      if (frame !== undefined) cancelAnimationFrame(frame);
    };
    // 每次全站秒钟更新时重新对齐墙上时钟；逐帧状态只影响本组件。
  }, [opensAt, now]);
  return remaining;
}
