"use client";
import { useEffect, useRef, useState } from "react";

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

// 浏览器计时步长之外的尾数仅作视觉模拟，不参与开市时间或后端判断。
export function simulateCountdown(remaining: bigint, frame: number) {
  if (remaining <= 0n) return 0n;
  const tail = BigInt((Math.imul(frame + 1, 0x9e3779b1) >>> 0) % 99999) + 1n;
  return (remaining / 100_000n) * 100_000n + tail;
}

export function useCountdown(opensAt: number, now: number) {
  const [remaining, setRemaining] = useState(() =>
    simulateCountdown(countdownNanoseconds(opensAt, now), 0),
  );
  const sequence = useRef(0);
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
      setRemaining(simulateCountdown(value, sequence.current++));
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
