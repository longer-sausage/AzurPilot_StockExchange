import {useEffect,useRef,useState} from 'react'
import type {Meta} from './types'
import {loadRecaptcha,RECAPTCHA_SITE_KEY,RECAPTCHA_TEST_SITE_KEY,type Recaptcha} from './recaptcha'

export function Captcha({meta,onToken,reset}:{meta:Meta;onToken:(token:string)=>void;reset:number}){
  const ref=useRef<HTMLDivElement>(null),callback=useRef(onToken)
  callback.current=onToken
  const [error,setError]=useState(''),[retry,setRetry]=useState(0)
  useEffect(()=>{
    callback.current('');setError('')
    const host=document.createElement('div')
    ref.current!.append(host)
    let stopped=false,id:number|undefined,api:Recaptcha|undefined
    void loadRecaptcha().then(loaded=>{
      if(stopped)return
      api=loaded
      id=api.render(host,{sitekey:meta.mock?RECAPTCHA_TEST_SITE_KEY:RECAPTCHA_SITE_KEY,theme:'dark',
        callback:(token:string)=>{if(!stopped){callback.current(token);setError('')}},
        'expired-callback':()=>{if(!stopped){callback.current('');setError('验证码已过期，请重新验证')}},
        'error-callback':()=>{if(!stopped){callback.current('');setError('验证码失败，请重试')}}})
    }).catch((error:Error)=>{if(!stopped){callback.current('');setError(error.message)}})
    return ()=>{
      stopped=true
      // 先 reset 再移除 DOM，且 0 也是有效组件 ID；旧回调不能恢复已清空的 token。
      if(id!==undefined)api?.reset(id)
      host.remove()
    }
  },[meta.mock,reset,retry])
  return <div className="captcha"><div className="captcha-widget" ref={ref}/>{meta.mock&&<small className="muted">Google reCAPTCHA 官方测试验证码</small>}{error&&<div className="inline-error">{error} <button type="button" onClick={()=>setRetry(value=>value+1)}>重试</button></div>}</div>
}
