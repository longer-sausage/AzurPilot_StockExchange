"use client";
import { useExchange } from "../core/Exchange";
import { AccountRequired } from "../features/AccountAccess";
import { OrderRecords } from "../features/Records";
export function OrdersPage() {
  const { ledger } = useExchange();
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">交易活动</p>
          <h1>委托记录</h1>
          <p>查看订单成交明细、挂单排队进度与交易税费，可随时撤销尚未成交的挂单。</p>
        </div>
      </div>
      {ledger ? <OrderRecords orders={ledger.orders} /> : <AccountRequired />}
    </>
  );
}
