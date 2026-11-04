"use client";
import { useMemo, useRef, useState } from "react";
import {
  Banner,
  Button,
  Dialog,
  Input,
  LayerCard,
  Radio,
  Select,
} from "@cloudflare/kumo";
import { XIcon } from "@phosphor-icons/react";
import { useExchange, useNotice } from "../core/Exchange";
import type { Order, Preview, Side, Stock } from "../core/contracts";
import { request } from "../core/http";
import {
  cents,
  date,
  kinds,
  money,
  percent,
  quantity,
  sides,
  stockState,
} from "../core/format";
export function TradeTicket({ stock }: { stock: Stock }) {
  const x = useExchange(),
    notice = useNotice();
  const [side, setSide] = useState<Side>("buy"),
    [kind, setKind] = useState("limit"),
    [tif, setTif] = useState("GTC"),
    [amount, setAmount] = useState("100"),
    [price, setPrice] = useState((stock.price / 100).toFixed(2)),
    [leverage, setLeverage] = useState(1),
    [preview, setPreview] = useState<Preview>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const identity = useMemo(
      () => crypto.randomUUID(),
      [stock.id, side, kind, tif, amount, price, leverage],
    ),
    submitted = useRef(false),
    rules = x.settings?.activeMarket,
    position = x.ledger?.positions.find((p) => p.stockId === stock.id),
    lot = rules?.lotSize ?? 1,
    max = Math.min(10, Math.floor(1000000 / (rules?.imrPPM ?? 500000))),
    ttl =
      rules?.id === "meow"
        ? (x.settings?.quoteTTLSeconds ?? 86400)
        : (rules?.quoteTTLSeconds ?? 86400),
    state = stockState(stock, ttl, x.now);
  const blocked = !x.connected
    ? "行情连接已断开，请等待网络重连后再交易。"
    : !x.market?.open
      ? x.market?.reason || "当前市场休市中，暂时停止交易。"
      : state !== "可交易"
        ? `这只股票当前${state}，暂时无法下单。`
        : x.account?.boundStockId === stock.id
          ? "根据市场规则，你不能交易自己绑定的专属股票。"
          : side === "short" && !stock.bound
            ? "该股票尚未绑定实例账户，暂不开放卖空。"
            : "";
  async function prepare() {
    if (!x.account) {
      x.setLoginOpen(true);
      return;
    }
    if (x.account.forcePasswordChange || !x.account.boundStockId) {
      x.setBindingOpen(true);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const count = Number(amount);
      if (!Number.isSafeInteger(count) || count <= 0 || count % lot)
        throw Error(`委托股数必须为 ${lot} 股（每手股数）的正整数倍`);
      const result = await request<Preview>("/orders/preview", {
        stock: String(stock.id),
        side,
        quantity: count,
        price: kind === "market" ? stock.price : cents(price),
        clientId: identity,
        leverage: side === "buy" || side === "short" ? leverage : 1,
        kind,
        timeInForce: tif,
      });
      submitted.current = false;
      setPreview(result);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function confirm() {
    if (!preview || submitted.current) return;
    submitted.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await request<{ order: Order }>("/orders/confirm", {
        previewId: preview.previewId,
      });
      setPreview(undefined);
      notice(
        result.order.status === "rejected"
          ? `委托提交失败：${result.order.reason ?? "请核对后重试"}`
          : result.order.status === "cancelled"
            ? `委托 #${result.order.id} 已成交 ${quantity(result.order.filled)} 股，剩余 ${quantity(result.order.remaining)} 股已撤销`
            : result.order.filled
              ? `委托 #${result.order.id} 已成功成交 ${quantity(result.order.filled)} 股，均价 ${money(result.order.averagePrice)}${result.order.remaining ? "，余量挂单" : ""}`
              : `委托 #${result.order.id} 已挂单，排队等待撮合中`,
        result.order.status === "rejected",
      );
      await x.refresh();
    } catch (e) {
      submitted.current = false;
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <LayerCard className="data-card">
        <div className="card-heading">
          <h2>交易下单</h2>
          <span className="caption">{stock.symbol}</span>
        </div>
        <form
          className="card-body"
          onSubmit={(e) => {
            e.preventDefault();
            void prepare();
          }}
        >
          <Radio.Group
            legend="交易方向"
            appearance="segmented"
            orientation="horizontal"
            value={side}
            onValueChange={(v) => setSide(v as Side)}
          >
            {Object.entries(sides).map(([value, label]) => (
              <Radio.Item key={value} value={value} label={label} />
            ))}
          </Radio.Group>
          <div className="fields">
            <Select
              label="委托类型"
              value={kind}
              onValueChange={(v) => setKind(String(v))}
              items={{ limit: "限价", market: "市价", stop: "止损触发市价" }}
            />
            <Select
              label="有效期"
              value={tif}
              disabled={kind === "market"}
              onValueChange={(v) => setTif(String(v))}
              items={
                kind === "market"
                  ? { [tif]: "立即成交，余量自动撤销" }
                  : { GTC: "本赛季有效", DAY: "当日有效" }
              }
            />
          </div>
          <Input
            label="委托股数"
            type="number"
            min={lot}
            step={lot}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
            description={`每手 ${lot} 股；当前可用持仓可卖 ${quantity(position?.sellable ?? 0)} 股，空头可回补 ${quantity(position?.shortQuantity ?? 0)} 股`}
          />
          <div className="toolbar-group">
            {[1, 5, 10].map((n) => (
              <Button
                key={n}
                variant="secondary"
                onClick={() => setAmount(String(lot * n))}
              >
                {n} 手
              </Button>
            ))}
          </div>
          <Input
            label={
              kind === "stop"
                ? "触发股价"
                : kind === "market"
                  ? "参考股价"
                  : "委托限价"
            }
            inputMode="decimal"
            value={kind === "market" ? (stock.price / 100).toFixed(2) : price}
            onChange={(e) => setPrice(e.target.value)}
            disabled={kind === "market"}
            required
            description="单位：模拟币 / 股，最多支持两位小数。"
          />
          <Button
            variant="ghost"
            onClick={() => setPrice((stock.price / 100).toFixed(2))}
          >
            填入最新成交价 {money(stock.price)}
          </Button>
          {(side === "buy" || side === "short") && (
            <Select<number>
              label="杠杆倍数"
              value={leverage}
              onValueChange={(v) => setLeverage(Number(v))}
              items={Array.from({ length: max }, (_, i) => ({
                value: i + 1,
                label: i === 0 ? "现金交易 · 1 倍" : `${i + 1} 倍杠杆`,
              }))}
            />
          )}
          <dl className="definition-list">
            <div>
              <dt>可用现金</dt>
              <dd>{money(x.account?.availableCash)}</dd>
            </div>
            <div>
              <dt>初始 / 维持保证金</dt>
              <dd>
                {percent(rules?.imrPPM)} / {percent(rules?.mmrPPM)}
              </dd>
            </div>
          </dl>
          {(leverage > 1 || side === "short" || kind === "stop") && (
            <Banner
              variant="alert"
              description="杠杆交易会同步放大收益与亏损，持仓期间会持续计收融资借款利息或借券费用；设置止损时，触发后将转为市价单撮合，遇到行情剧烈波动时成交价可能会有跳空偏差。"
            />
          )}
          {blocked && <Banner variant="secondary" description={blocked} />}
          <p className="caption">
            {kind === "market"
              ? "立即按当前有效报价与溢价成交，未成交余量自动撤销并解冻，不进入挂单。"
              : kind === "stop"
                ? "达到触发价后转为市价，未成交余量自动撤销并解冻；未触发时按有效期等待整点检查。"
                : "限价委托在每小时流动性额度内可立即撮合成交，超出部分会自动挂单并在整点撮合；实际成交受溢价、额度和限价约束。"}
            预估税费与保证金扣减以下一步预览明细为准。
          </p>
          {error && !preview && (
            <Banner variant="error" role="alert" description={error} />
          )}
          <Button
            type="submit"
            variant="primary"
            loading={busy}
            disabled={!!blocked}
            className="w-full justify-center"
          >
            {x.account ? "预览交易明细" : "登录后交易"}
          </Button>
        </form>
      </LayerCard>
      <Dialog.Root
        open={!!preview}
        onOpenChange={(open) => {
          if (!open && !busy) {
            setPreview(undefined);
            setError("");
          }
        }}
      >
        <Dialog size="lg" className="dialog-content">
          <div className="dialog-title">
            <Dialog.Title>确认交易委托</Dialog.Title>
            <Button
              shape="square"
              variant="ghost"
              aria-label="关闭交易预览"
              disabled={busy}
              onClick={() => setPreview(undefined)}
            >
              <XIcon size={20} />
            </Button>
          </div>
          {preview && (
            <>
              <Dialog.Description>
                {stock.name} · {sides[preview.side]} · {kinds[preview.kind]} ·{" "}
                {preview.leverage} 倍
              </Dialog.Description>
              <dl className="definition-list">
                {[
                  ["数量", `${quantity(preview.quantity)} 股`],
                  [
                    preview.kind === "market"
                      ? "参考股价"
                      : preview.kind === "stop"
                        ? "触发股价"
                        : "委托价格",
                    money(preview.price),
                  ],
                  [
                    "有效期",
                    preview.kind === "market"
                      ? "立即成交，余量撤销"
                      : preview.timeInForce,
                  ],
                  ["成交额", money(preview.notional)],
                  ["税率", percent(preview.taxRatePPM)],
                  [
                    "税 / 佣金",
                    `${money(preview.tax)} / ${money(preview.commission)}`,
                  ],
                  [
                    "印花税 / 征费",
                    `${money(preview.stamp)} / ${money(preview.levy)}`,
                  ],
                  [
                    "融资 / 保证金",
                    `${money(preview.financing)} / ${money(preview.collateral)}`,
                  ],
                  ["预计归还融资", money(preview.repayment)],
                  ["预计资金变动", money(preview.estimatedTotal)],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt>{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
              <p className="caption">
                此预览明细有效至 {date(preview.expiresAt)}
                。提交时系统会再次核验最新股价与账户资产；请放心，重复点击确认不会导致重复扣款或重复下单。
                {preview.kind === "market"
                  ? "未成交余量自动撤销并解冻，不进入挂单。"
                  : preview.kind === "stop"
                    ? "未触发时保留委托；触发后未成交余量自动撤销并解冻。"
                    : "未成交部分保留为挂单。"}
              </p>
              {error && (
                <Banner variant="error" role="alert" description={error} />
              )}
              <Button
                variant="primary"
                loading={busy}
                onClick={() => void confirm()}
              >
                确认提交委托
              </Button>
            </>
          )}
        </Dialog>
      </Dialog.Root>
    </>
  );
}
