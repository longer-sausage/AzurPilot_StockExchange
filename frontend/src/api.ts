export class ApiError extends Error { constructor(message:string,public code:string,public status:number){super(message)} }
export function subscribeUpdates(listener:()=>void){const events=new EventSource('/api/events');events.addEventListener('stock',listener);return ()=>events.close()}
export async function api<T>(path:string,body?:unknown,token?:string,method?:string):Promise<T>{
  const response=await fetch(`/api${path}`,{method:method??(body===undefined?'GET':'POST'),headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...(token?{Authorization:`Bearer ${token}`}:{})},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(12000)})
  const data=await response.json();if(!response.ok)throw new ApiError(data.error?.message??'服务暂不可用',data.error?.code??'HTTP_ERROR',response.status);return data as T
}
export const money=(n:number,digits=2)=>new Intl.NumberFormat('zh-CN',{minimumFractionDigits:digits,maximumFractionDigits:digits}).format(n/100)
export const compact=(n:number)=>Math.abs(n)>=10000000000?`${(n/10000000000).toFixed(2)} 亿`:Math.abs(n)>=1000000?`${(n/1000000).toFixed(2)} 万`:money(n)
export const percent=(ppm:number)=>`${ppm>=0?'+':''}${(ppm/10000).toFixed(2)}%`
export const datetime=(t:number)=>new Date(t*1000).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})
export const feesTotal=(f:{commission:number;stamp:number;levy:number;borrow:number;financing?:number})=>f.commission+f.stamp+f.levy+f.borrow+(f.financing??0)
export const sideName:Record<string,string>={buy:'买入',sell:'卖出',short:'卖空',cover:'回补'}
export const statusName:Record<string,string>={pending:'待触发',filled:'已成交',cancelled:'已撤销',expired:'已过期',rejected:'已拒绝'}
