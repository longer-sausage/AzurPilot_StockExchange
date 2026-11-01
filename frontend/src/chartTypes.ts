import type {Stock,TradePrint,Order} from './types'
type PublicPending=Order
export type ChartPeriod='time'|'day'|'m5'|'m10'|'m20'|'m30'|'m60'
export interface Candle {time:number;open:number;high:number;low:number;close:number;samples:number;volume:number;turnover:number;buyVolume:number;sellVolume:number}
export interface StockDetail {stock:Stock;period:ChartPeriod;month:string;day:string;displayFrom:number;bars:Candle[];trades:TradePrint[];orders:(PublicPending&{username:string})[];legacyOrders:PublicPending[];summary:{day:string;open:number;high:number;low:number;previousClose:number;volume:number;turnover:number;buyVolume:number;sellVolume:number};coverage:{count:number;firstObservedAt:number;lastObservedAt:number;reconciledAt:number}}