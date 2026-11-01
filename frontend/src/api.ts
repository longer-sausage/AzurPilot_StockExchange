export class ApiError extends Error {constructor(message:string,public code:string,public status:number){super(message)}}
const cache=new Map<string,{etag:string;data:unknown}>()
export async function api<T>(path:string,body?:unknown,method?:string):Promise<T>{
  const cacheable=path.startsWith('/public/')||path==='/account'
  const cached=cacheable?cache.get(path):undefined
  const response=await fetch(`/api/v2${path}`,{method:method??(body===undefined?'GET':'POST'),credentials:'same-origin',headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...(cached?{'If-None-Match':cached.etag}:{})},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(15000)})
  if(response.status===304&&cached)return cached.data as T
  const data=await response.json()
  if(!response.ok){if(response.status===401&&!['/auth/login','/admin/login'].includes(path)){cache.delete('/account');window.dispatchEvent(new CustomEvent('mmex-auth-lost',{detail:path.startsWith('/admin/')?'admin':'player'}))}throw new ApiError(data.error?.message??'服务暂时不可用，请稍后重试',data.error?.code??'HTTP_ERROR',response.status)}
  if(['/auth/logout','/auth/login'].includes(path))cache.delete('/account')
  const etag=response.headers.get('ETag');if(etag&&cacheable){cache.set(path,{etag,data});if(cache.size>32)cache.delete(cache.keys().next().value!)}
  return data as T
}
export const money=(n:number)=>new Intl.NumberFormat('zh-CN',{minimumFractionDigits:2,maximumFractionDigits:2}).format((n??0)/100)
export const number=(n:number)=>new Intl.NumberFormat('zh-CN').format(n??0)
export const rate=(ppm:number)=>`${((ppm??0)/10000).toFixed(2)}%`
export const datetime=(t:number)=>t?new Date(t<100000000000?t*1000:t).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false}):'—'
export const sideName:Record<string,string>={buy:'买入',sell:'卖出',short:'卖空',cover:'回补'}
export const statusName:Record<string,string>={pending:'挂单中',partial:'部分成交',filled:'全部成交',cancelled:'已撤销',rejected:'未通过',expired:'已到期',open:'挂单中'}
export function cents(value:string){if(!/^\d+(\.\d{1,2})?$/.test(value))throw new Error('股价须为正数，最多两位小数');const [a,b='']=value.split('.');const n=Number(a)*100+Number(b.padEnd(2,'0'));if(!Number.isSafeInteger(n)||n<=0)throw new Error('股价超出范围');return n}
