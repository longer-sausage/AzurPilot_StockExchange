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
    ? "行情连接中断，恢复连接后再交易。"
    : !x.market?.open
      ? x.market?.reason || "当前市场休市。"
      : state !== "可交易"
        ? `该证券${state}。`
        : x.account?.boundStockId === stock.id
          ? "不能交易自己绑定的股票。"
          : side === "short" && !stock.bound
            ? "该股票尚未绑定账户，不能卖空。"
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
        throw Error(`数量必须为 ${lot} 股的正整数倍`);
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
          ? `委托未通过：${result.order.reason ?? ""}`
          : result.order.filled
            ? `委托 #${result.order.id} 成交 ${quantity(result.order.filled)} 股，均价 ${money(result.order.averagePrice)}`
            : `委托 #${result.order.id} 已挂单`,
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
          <h2>提交委托</h2>
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
              onValueChange={(v) => setTif(String(v))}
              items={{ GTC: "本赛季有效", DAY: "当日有效" }}
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
            description={`每手 ${lot} 股；可卖 ${quantity(position?.sellable ?? 0)}，可回补 ${quantity(position?.shortQuantity ?? 0)}`}
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
            label={kind === "stop" ? "触发股价" : "委托限价"}
            inputMode="decimal"
            value={kind === "market" ? (stock.price / 100).toFixed(2) : price}
            onChange={(e) => setPrice(e.target.value)}
            disabled={kind === "market"}
            required
            description="模拟币 / 股，最多两位小数。"
          />
          <Button
            variant="ghost"
            onClick={() => setPrice((stock.price / 100).toFixed(2))}
          >
            使用最新价 {money(stock.price)}
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
              description="杠杆放大损益；借款与借券费用持续计提。止损触发后按市价执行，可能跳空。"
            />
          )}
          {blocked && <Banner variant="secondary" description={blocked} />}
          <p className="caption">
            额度内即时成交，余量挂单并在后续整点处理。金额、税费与保证金以服务器预览为准。
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
                  ["委托价格", money(preview.price)],
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
                有效至 {date(preview.expiresAt)}
                。实际成交前会重新核验报价和资产，同一预览重复确认不会重复落账。
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
