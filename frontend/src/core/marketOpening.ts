import type { Market, Settings } from "./contracts";

export const openingWindow = 24 * 60 * 60 * 1000;
const minute = 60 * 1000;

// 只查当前赛季未来 24 小时的交易窗口；多看一分钟，让秒级时钟准确进入倒计时。
// 与 Go marketOpen / IsMarketOpen 一致：赛季、暂停、市场时区、周末、假日、分段时段。
export function marketOpening(
  market: Market | undefined,
  settings: Settings | undefined,
  minuteStart: number,
): number | undefined {
  const season = market?.season;
  if (
    !market ||
    market.open ||
    !settings ||
    settings.paused ||
    !season ||
    season.settled ||
    minuteStart >= season.endsAt
  )
    return;

  const rules = settings.activeMarket,
    sessions = rules.sessions.map((session) =>
      session.split("-").map((time) => {
        const [hours, minutes] = time.split(":").map(Number);
        return hours * 60 + minutes;
      }),
    ),
    formatter = new Intl.DateTimeFormat("en-CA", {
      timeZone: rules.timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      weekday: "short",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }),
    until = Math.min(minuteStart + openingWindow + minute, season.endsAt - 1);

  // 沿绝对时间前进，避免纽约等市场夏令时切换时误算日期或重复时段。
  for (
    let at = Math.max(minuteStart, season.startsAt);
    at <= until;
    at = (Math.floor(at / minute) + 1) * minute
  ) {
    const parts = Object.fromEntries(
        formatter.formatToParts(at).map(({ type, value }) => [type, value]),
      ),
      date = `${parts.year}-${parts.month}-${parts.day}`;
    if (
      (rules.weekdaysOnly && ["Sat", "Sun"].includes(parts.weekday)) ||
      rules.holidays.includes(date)
    )
      continue;
    const localMinute = Number(parts.hour) * 60 + Number(parts.minute);
    if (
      sessions.some(([start, end]) => localMinute >= start && localMinute < end)
    )
      return at;
  }
}
