import {Component,useEffect,useState,type ReactNode} from 'react'
import {createRoot} from 'react-dom/client'
import {Console} from './Console'
import {api} from './api'
import type {Meta} from './types'
import {Cat,ArrowUpRight} from 'lucide-react'
import './styles.css'

class Boundary extends Component<{children:ReactNode},{failed:boolean}>{state={failed:false};static getDerivedStateFromError(){return {failed:true}};render(){return this.state.failed?<div className="startup"><h1>终端遇到异常</h1><button onClick={()=>location.reload()}>重新加载</button></div>:this.props.children}}
function App(){
  const [meta,setMeta]=useState<Meta>(),[error,setError]=useState('')
  useEffect(()=>{void api<Meta>('/meta').then(setMeta).catch(e=>setError(e.message))},[])
  if(error)return <div className="startup"><h1>交易所暂时无法连接</h1><p>{error}</p><button className="primary" onClick={()=>location.reload()}>重新连接</button></div>
  if(!meta)return <div className="startup"><Cat size={36}/><p>正在连接交易所…</p></div>
  if(location.pathname==='/console')return <Console meta={meta}/>
  return <div className="startup"><Cat size={44}/><p className="eyebrow">MEOWMING EXCHANGE</p><h1>茗喵证券交易所</h1><p>交易终端已集成到 AzurPilot。</p><p className="muted">打开实例的运行总览，点击资源卡片调节键左边的交易所按钮。<br/>账户与所选实例永久绑定，注册和登录均需验证码。</p><a className="primary" href="/console">进入管理控制台<ArrowUpRight size={16}/></a><small className="muted">模拟证券游戏 · 不涉及真实资金 {meta.mock?'· MOCK':''}</small></div>
}
createRoot(document.getElementById('root')!).render(<Boundary><App/></Boundary>)
