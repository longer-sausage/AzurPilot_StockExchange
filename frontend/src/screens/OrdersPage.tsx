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
          <p>检查成交明细、未成交余量与费用，或撤销待成交委托。</p>
        </div>
      </div>
      {ledger ? <OrderRecords orders={ledger.orders} /> : <AccountRequired />}
    </>
  );
}
