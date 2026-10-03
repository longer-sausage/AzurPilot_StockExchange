import {useEffect,useRef,useState} from 'react'
import type {Meta} from './types'

// 公开站点标识固定在前端；私密验证码密钥仅由 Go 服务端读取。
const TURNSTILE_SITE_KEY='0x4AAAAAAFMCQstp3hgd939a'
const TURNSTILE_TEST_SITE_KEY='1x00000000000000000000AA'
interface Turnstile {render:(el:HTMLElement,o:Record<string,unknown>)=>string;remove:(id:string)=>void;reset:(id:string)=>void}
declare global {interface Window {turnstile?:Turnstile}}
let scriptPromise:Promise<void>|undefined
function loadTurnstile(){if(window.turnstile)return Promise.resolve();if(!scriptPromise)scriptPromise=new Promise((resolve,reject)=>{const s=document.createElement('script');s.src='https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';s.async=true;s.onload=()=>resolve();s.onerror=()=>{scriptPromise=undefined;reject(new Error('验证码加载失败，请检查网络后重试'))};document.head.append(s)});return scriptPromise}
export function Captcha({meta,action,onToken,reset}:{meta:Meta;action:string;onToken:(t:string)=>void;reset:number}){
  const ref=useRef<HTMLDivElement>(null),callback=useRef(onToken);callback.current=onToken;const [error,setError]=useState('');const [retry,setRetry]=useState(0)
  useEffect(()=>{callback.current('');let stopped=false;let id:string|undefined;setError('')
    void loadTurnstile().then(()=>{if(!stopped&&ref.current&&window.turnstile)id=window.turnstile.render(ref.current,{sitekey:meta.mock?TURNSTILE_TEST_SITE_KEY:TURNSTILE_SITE_KEY,action,theme:'dark',callback:(token:string)=>callback.current(token),'expired-callback':()=>callback.current(''),'error-callback':()=>{callback.current('');setError('验证码失败，请重试')}})}).catch(e=>setError(e.message))
    return ()=>{stopped=true;if(id)window.turnstile?.remove(id)}
  },[meta.mock,action,reset,retry])
  return <div className="captcha"><div className="captcha-widget" ref={ref}/>{meta.mock&&<small className="muted">Cloudflare 官方测试验证码</small>}{error&&<div className="inline-error">{error} <button type="button" onClick={()=>setRetry(v=>v+1)}>重试</button></div>}</div>
}
