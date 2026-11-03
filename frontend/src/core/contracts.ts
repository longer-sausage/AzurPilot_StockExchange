export type Side = "buy" | "sell" | "short" | "cover";
export type Period = "time" | "day" | "m5" | "m10" | "m20" | "m30" | "m60";
export interface Fees {
  tax: number;
  commission: number;
  stamp: number;
  levy: number;
  borrow: number;
  financing: number;
}
export interface Stock {
  id: number;
  symbol: string;
  name: string;
  price: number;
  open: number;
  openDay: string;
  observedAt: number;
  uploadedAt: number;
  historyVersion: number;
  historyCheckedAt?: number;
  tradeVersion: number;
  bound: boolean;
  disabled: boolean;
  delisted?: boolean;
  deletedAt?: number;
  issuedShares: number;
  outstanding: number;
  hourlyVolume: number;
  normalUsed: number;
  borrowShares: number;
  borrowed: number;
  borrowHourlyVolume: number;
  borrowUsed: number;
  accountId?: number;
  instanceId?: string;
  publicKey?: string;
  bindingCode?: string;
}
export interface Rules {
  id: string;
  name: string;
  description: string;
  commissionPPM: number;
  minCommission: number;
  stampPPM: number;
  stampSide: string;
  stampRound: number;
  levyPPM: number;
  borrowAnnualPPM: number;
  financingAnnualPPM: number;
  imrPPM: number;
  mmrPPM: number;
  lotSize: number;
  sellDelayDays: number;
  cashDelayDays: number;
  timezone: string;
  sessions: string[];
  weekdaysOnly: boolean;
  holidays: string[];
  quoteTTLSeconds: number;
  source: string;
}
export interface Settings {
  initialCash: number;
  issuedShares: number;
  hourlyVolume: number;
  borrowShares: number;
  borrowHourlyVolume: number;
  premiumPPM: number;
  borrowAnnualPPM: number;
  commissionPPM: number;
  minCommission: number;
  quoteTTLSeconds: number;
  paused: boolean;
  startDay: number;
  endDaysFromLast: number;
  delistThreshold: number;
  taxBrackets: { upTo: number; ratePPM: number }[];
  activeMarket: Rules;
  marketPresets: Rules[];
}
export interface Account {
  id: number;
  username: string;
  identityCode: string;
  createdAt: number;
  initialCash: number;
  cash: number;
  availableCash: number;
  reserved: number;
  equity: number;
  returnPPM: number;
  realized: number;
  boundStockId: number;
  forcePasswordChange: boolean;
  disabled?: boolean;
  watchlist: number[];
  fees: Fees;
  metrics?: Record<string, number>;
  metricsSince: number;
  legacyMetrics: boolean;
  positionCount?: number;
  pendingCount?: number;
  settlements?: { amount: number; availableAt: number }[];
}
export interface Position {
  stockId: number;
  stock?: Stock;
  quantity: number;
  sellable: number;
  cost: number;
  loan: number;
  shortQuantity: number;
  shortCost: number;
  shortProceeds: number;
  collateral: number;
}
export interface Order {
  id: number;
  stockId: number;
  stockName?: string;
  side: Side;
  kind: string;
  timeInForce: string;
  expiresAt: number;
  leverage: number;
  quantity: number;
  remaining: number;
  filled: number;
  price: number;
  averagePrice: number;
  notional: number;
  tax: number;
  commission: number;
  stamp: number;
  levy: number;
  status: string;
  reserved: number;
  createdAt: number;
  updatedAt: number;
  reason?: string;
  username?: string;
}
export interface Trade {
  id: number;
  orderId: number;
  stockId: number;
  accountId: number;
  username: string;
  side: Side;
  kind: string;
  quantity: number;
  price: number;
  notional: number;
  at: number;
  forced: boolean;
  reason?: string;
  fees: Fees;
}
export interface Ledger {
  account: Account;
  positions: Position[];
  orders: Order[];
  trades: Trade[];
}
export interface Season {
  id: string;
  startsAt: number;
  endsAt: number;
  settled: boolean;
}
export interface Ranking {
  rank: number;
  accountId: number;
  username: string;
  equity: number;
  returnPPM: number;
}
export interface Seasons {
  season: Season;
  rankings: Ranking[];
  archives: { season: Season; rankings: Ranking[]; revenue: Fees }[];
  revenue: Fees;
  lifetimeRevenue: Fees;
}
export interface Market {
  version: number;
  stocks: Stock[];
  serverTime: number;
  open: boolean;
  reason: string;
  season?: Season;
}
export interface Bar {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  samples: number;
  volume: number;
  turnover: number;
  buyVolume: number;
  sellVolume: number;
}
export interface Detail {
  stock: Stock;
  period: Period;
  month: string;
  day: string;
  displayFrom: number;
  bars: Bar[];
  trades: Trade[];
  orders: Order[];
  legacyOrders: Order[];
  summary: {
    open: number;
    high: number;
    low: number;
    previousClose: number;
    volume: number;
    turnover: number;
    buyVolume: number;
    sellVolume: number;
  };
  coverage: {
    count: number;
    firstObservedAt: number;
    lastObservedAt: number;
    reconciledAt?: number;
  };
}
export interface Preview {
  previewId: string;
  expiresAt: number;
  stock: Stock;
  side: Side;
  kind: string;
  timeInForce: string;
  quantity: number;
  price: number;
  notional: number;
  taxRatePPM: number;
  tax: number;
  commission: number;
  stamp: number;
  levy: number;
  estimatedTotal: number;
  premiumPPM: number;
  leverage: number;
  financing: number;
  collateral: number;
  repayment: number;
}
