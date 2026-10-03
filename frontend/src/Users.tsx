import {useEffect,useRef,useState,type FormEvent} from 'react'
import {Copy,RefreshCw,Save,Search,ShieldCheck,X} from 'lucide-react'
import {api,datetime,feesTotal,money,sideName,statusName} from './api'
import type {Account,AdminPlayerList,AdminPlayerUpdate,Market} from './types'
import './users.css'

const stamp=(time:number)=>time?datetime(time):'暂无记录'
const kindName:Record<string,string>={market:'市价',limit:'限价',stop:'止损'}

export function UserManagement({token,market,onUnauthorized,onChanged}:{token:string;market?:Market;onUnauthorized:()=>void;onChanged:()=>void}){
  const [search,setSearch]=useState(''),[query,setQuery]=useState(''),[status,setStatus]=useState('all'),[page,setPage]=useState(1),[list,setList]=useState<AdminPlayerList>(),[loading,setLoading]=useState(true),[error,setError]=useState(''),[refresh,setRefresh]=useState(0),[selected,setSelected]=useState<number>()
  useEffect(()=>{const timer=setTimeout(()=>{setQuery(search.trim());setPage(1)},300);return ()=>clearTimeout(timer)},[search])
  useEffect(()=>{
    let active=true;setLoading(true);setError('')
    const params=new URLSearchParams({q:query,status,page:String(page),pageSize:'20'})
    void api<AdminPlayerList>(`/console/players?${params}`,undefined,token).then(result=>{if(active)setList(result)}).catch(e=>{if(active){setError(e.message);if(e.status===401)onUnauthorized()}}).finally(()=>{if(active)setLoading(false)})
    return ()=>{active=false}
  },[token,query,status,page,refresh])
  function changed(){setRefresh(v=>v+1);onChanged()}
  const pages=Math.max(1,Math.ceil((list?.total??0)/20)),currentPage=list?.page??page
  return <div className="user-management">
    <section className="panel padded">
      <div className="panel-title"><div><h2>用户列表</h2><p className="muted">正常 {list?.active??0} 位 · 停用 {list?.disabled??0} 位；支持身份码核实与实例查找。</p></div><button disabled={loading} onClick={()=>setRefresh(v=>v+1)}><RefreshCw size={15}/>刷新用户</button></div>
      <div className="user-filters"><label className="user-search"><Search size={17}/><input aria-label="搜索用户" placeholder="用户名 / 证券代码 / 身份识别码 / 实例 ID" value={search} onChange={e=>setSearch(e.target.value)}/></label><label>账户状态<select aria-label="筛选账户状态" value={status} onChange={e=>{setStatus(e.target.value);setPage(1)}}><option value="all">全部账户</option><option value="active">正常账户</option><option value="disabled">停用账户</option></select></label></div>
      {error&&<div className="error-banner" role="alert">{error}<button onClick={()=>setRefresh(v=>v+1)}>重试</button></div>}
      <div className="table-scroll" aria-busy={loading}><table className="user-table"><thead><tr><th>用户 / 证券代码</th><th>身份识别码</th><th>现金 / 净值</th><th>持仓 / 挂单</th><th>注册时间</th><th>账户 / 实例</th><th>管理</th></tr></thead><tbody>
        {!error&&list?.players.map(p=><tr key={p.id} className={selected===p.id?'selected-user':''}><td><strong>{p.username}</strong><small>{p.symbol} · #{p.id}</small></td><td><code className="table-identity">{p.identityCode}</code></td><td>{money(p.cash)}<small>净值 {money(p.equity)}</small></td><td>{p.positionCount} 支持仓<small>{p.pendingOrders} 笔待触发</small></td><td>{stamp(p.joinedAt)}</td><td><span className={`user-status ${p.disabled?'disabled':''}`}>{p.disabled?'已停用':'正常'}</span><small>{p.instanceId?'已永久绑定':'未绑定实例'} · {p.stale?'报价过期':'报价有效'}</small></td><td><button className="small-button" onClick={()=>setSelected(p.id)} aria-label={`管理用户 ${p.username}`}>查看 / 编辑</button></td></tr>)}
        {!error&&(!list||list.players.length===0)&&<tr><td colSpan={7} className="user-empty">{loading?'正在加载用户…':query||status!=='all'?'未找到符合条件的用户':'暂无注册用户'}</td></tr>}
      </tbody></table></div>
      <div className="user-pagination"><span className="muted">{loading?'正在加载…':`共 ${list?.total??0} 位用户 · 第 ${currentPage} / ${pages} 页`}</span><div><button disabled={loading||currentPage<=1} onClick={()=>setPage(currentPage-1)}>上一页</button><button disabled={loading||currentPage>=pages} onClick={()=>setPage(currentPage+1)}>下一页</button></div></div>
    </section>
    {selected!==undefined&&<UserDetail key={selected} id={selected} token={token} market={market} onClose={()=>setSelected(undefined)} onChanged={changed} onUnauthorized={onUnauthorized}/>}
  </div>
}

function UserDetail({id,token,market,onClose,onChanged,onUnauthorized}:{id:number;token:string;market?:Market;onClose:()=>void;onChanged:()=>void;onUnauthorized:()=>void}){
  const [account,setAccount]=useState<Account>(),[tab,setTab]=useState('profile'),[username,setUsername]=useState(''),[password,setPassword]=useState(''),[confirmation,setConfirmation]=useState(''),[adjustment,setAdjustment]=useState(''),[disabled,setDisabled]=useState(false),[acknowledged,setAcknowledged]=useState(false),[busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState(''),[message,setMessage]=useState(''),[reload,setReload]=useState(0)
  const heading=useRef<HTMLHeadingElement>(null)
  function populate(value:Account){setAccount(value);setUsername(value.player.username);setDisabled(value.player.disabled);setPassword('');setConfirmation('');setAdjustment('');setAcknowledged(false)}
  useEffect(()=>{
    let active=true;setLoading(true);setError('')
    void api<Account>(`/console/players/${id}`,undefined,token).then(result=>{if(active){populate(result);heading.current?.focus();heading.current?.scrollIntoView({block:'start',behavior:'smooth'})}}).catch(e=>{if(active){setError(e.message);if(e.status===401)onUnauthorized()}}).finally(()=>{if(active)setLoading(false)})
    return ()=>{active=false}
  },[id,token,reload])
  async function copy(){if(!account)return;try{await navigator.clipboard.writeText(account.player.identityCode);setMessage('身份识别码已复制')}catch{setError('复制失败，请选中身份识别码手动复制')}}
  async function save(event:FormEvent){
    event.preventDefault();if(!account)return;setError('');setMessage('')
    const input:AdminPlayerUpdate={}
    if(username!==account.player.username)input.username=username
    if(password){if(password!==confirmation){setError('两次输入的新密码不一致');return}input.password=password}
    if(disabled!==account.player.disabled)input.disabled=disabled
    if(adjustment.trim()){
      if(!/^[+-]?\d+(\.\d{1,2})?$/.test(adjustment.trim())){setError('现金调整请输入金额，最多两位小数');return}
      const cents=Math.round(Number(adjustment)*100)
      if(!Number.isSafeInteger(cents)||Math.abs(cents)>100000000000000){setError('现金调整金额不得超过 1 万亿模拟币');return}
      if(cents!==0){input.cashAdjustment=cents;input.expectedCash=account.player.cash}
    }
    if(input.disabled&&!acknowledged){setError('请确认停用账户的平仓及撤单操作');return}
    if(!Object.keys(input).length){setMessage('用户信息没有变更');return}
    setBusy(true)
    try{populate(await api<Account>(`/console/players/${id}`,input,token,'PUT'));setMessage(`用户信息已保存${input.password?'，旧会话已失效，用户需使用新密码重新登录':''}${input.disabled?'，已平仓本人持仓并撤销相关挂单':''}`);onChanged()}catch(e){setError((e as Error).message);if((e as {status?:number}).status===401)onUnauthorized()}finally{setBusy(false)}
  }
  const player=account?.player,positions=Object.values(player?.positions??{}),orders=[...(player?.orders??[])].sort((a,b)=>b.id-a.id)
  const now=Date.now()/1000
  const stockName=(stockId:number)=>market?.stocks.find(s=>s.id===stockId)?.username??`用户 #${stockId}`
  const stockSymbol=(stockId:number)=>market?.stocks.find(s=>s.id===stockId)?.symbol??`MM${String(stockId).padStart(6,'0')}`
  const changed=!!player&&(username!==player.username||!!password||!!adjustment.trim()||disabled!==player.disabled)
  return <section className="panel padded user-detail" aria-label="用户详情">
    <div className="panel-title"><div><p className="eyebrow">USER ACCOUNT</p><h2 ref={heading} tabIndex={-1}>{player?`${player.username} · 用户详情`:`用户 #${id} · 详情`}</h2></div><div className="user-detail-actions"><button disabled={busy||loading} onClick={()=>{setMessage('');setReload(v=>v+1)}}><RefreshCw size={15}/>重新加载详情</button><button disabled={busy} aria-label="关闭用户详情" onClick={onClose}><X size={17}/></button></div></div>
    {error&&<div className="error-banner" role="alert">{error}</div>}{message&&<div className="success-banner" role="status">{message}</div>}
    {loading?<p className="user-empty">正在加载用户详情…</p>:account&&player&&<>
      <div className="user-identity"><ShieldCheck size={23}/><div><strong>唯一身份识别码</strong><code>{player.identityCode}</code><small>仅本人和管理员可查看，用于核实账户身份；改名及赛季重置后保留。</small></div><button onClick={()=>void copy()} aria-label="复制用户身份识别码"><Copy size={15}/>复制</button></div>
      <nav className="user-detail-tabs" aria-label="用户详情分类">{[['profile','基本资料与编辑'],['assets','资金与交收'],['positions',`持仓 (${positions.length})`],['orders',`委托记录 (${orders.length})`]].map(([key,label])=><button key={key} className={tab===key?'active':''} aria-pressed={tab===key} onClick={()=>setTab(key)}>{label}</button>)}</nav>
      {tab==='profile'&&<div className="user-profile-grid"><form onSubmit={e=>void save(e)} className="user-edit-form">
        <h3>修改用户信息</h3><fieldset disabled={busy}><label>用户名<input aria-label="用户名称" required minLength={2} maxLength={20} value={username} onChange={e=>setUsername(e.target.value)}/><small>2–20 位汉字、字母、数字或下划线；修改后使用新用户名登录。</small></label>
        <div className="user-form-pair"><label>新密码<input type="password" aria-label="重设用户密码" autoComplete="new-password" value={password} onChange={e=>setPassword(e.target.value)}/><small>留空保留原密码；新密码需为 10–72 字节。</small></label><label>确认新密码<input type="password" aria-label="确认用户新密码" autoComplete="new-password" required={!!password} value={confirmation} onChange={e=>setConfirmation(e.target.value)}/></label></div>
        <label>现金调整 (模拟币)<input aria-label="用户现金调整 (模拟币)" inputMode="decimal" placeholder="如 1000.00 或 -1000.00，留空不调整" value={adjustment} onChange={e=>setAdjustment(e.target.value)}/><small>当前现金 {money(player.cash)}，可用 {money(account.available)}。正数增加、负数扣减；保留赛季本金，计入本赛季净值。</small></label>
        <label>账户状态<select aria-label="用户账户状态" value={disabled?'disabled':'active'} onChange={e=>{setDisabled(e.target.value==='disabled');setAcknowledged(false)}}><option value="active">正常</option><option value="disabled">停用</option></select></label>
        {disabled&&!player.disabled&&<label className="user-disable-confirm"><input type="checkbox" checked={acknowledged} onChange={e=>setAcknowledged(e.target.checked)}/>确认停用：使用户会话失效，强平其持仓，撤销本人挂单与其他用户针对该股票的挂单。</label>}
        <button type="submit" className="primary" disabled={!changed||disabled&&!player.disabled&&!acknowledged}><Save size={16}/>{busy?'正在保存…':'保存用户信息'}</button></fieldset>
      </form><div className="user-readonly"><h3>账户与行情</h3><dl><div><dt>内部用户 ID / 证券代码</dt><dd>#{player.id} / {stockSymbol(player.id)}</dd></div><div><dt>注册时间</dt><dd>{stamp(player.joinedAt)}</dd></div><div><dt>本赛季起始本金</dt><dd>{money(player.initialCash)} 模拟币</dd></div><div><dt>总行动力 / 当前报价</dt><dd>{player.quote.price/100} / {money(player.quote.price)}</dd></div><div><dt>行情记录时间</dt><dd>{stamp(player.quote.observedAt)}</dd></div><div><dt>最近上传时间</dt><dd>{stamp(player.quote.uploadedAt)}</dd></div><div><dt>永久绑定实例 ID</dt><dd className="user-code">{player.binding?.instanceId??'尚未绑定'}</dd></div></dl>{player.binding&&<details><summary>查看实例公钥与绑定指纹</summary><dl><div><dt>公钥</dt><dd className="user-code">{player.binding.publicKey}</dd></div><div><dt>绑定指纹</dt><dd className="user-code">{player.binding.key}</dd></div></dl></details>}<p className="muted">身份识别码用于核实账户，不作为登录凭据。实例永久绑定，凭据哈希不会显示。</p></div></div>}
      {tab==='assets'&&<>
        <div className="user-metrics">{[['现金余额',player.cash],['账户净值',account.equity],['可用资金',account.available],['冻结资金',account.frozen],['本赛季起始本金',player.initialCash],['已实现损益',player.realized],['待交收资金',account.unsettled],['空仓负债',account.shortLiability],['融资本金',account.financingDebt],['待落账融资利息',account.financingAccrued],['待落账借券费',account.borrowAccrued],['初始保证金',account.initialMargin],['维持保证金',account.maintenanceMargin],['融资多头保证金',account.longMargin]].map(([label,value])=><div key={label}><span>{label}</span><strong>{money(Number(value))}</strong></div>)}</div>
        <h3>本赛季累计费用 · {money(feesTotal(player.fees))} 模拟币</h3><div className="user-metrics">{[['佣金',player.fees.commission],['印花税',player.fees.stamp],['交易征费',player.fees.levy],['借券费',player.fees.borrow],['融资利息',player.fees.financing??0]].map(([label,value])=><div key={label}><span>{label}</span><strong>{money(Number(value))}</strong></div>)}</div>
        <h3>资金交收明细</h3><div className="table-scroll"><table><thead><tr><th>金额 (模拟币)</th><th>可用时间</th><th>状态</th></tr></thead><tbody>{(player.settlements??[]).map((s,i)=><tr key={i}><td>{money(s.amount)}</td><td>{stamp(s.availableAt)}</td><td>{s.availableAt<=now?'已交收':'等待交收'}</td></tr>)}{!player.settlements?.length&&<tr><td colSpan={3} className="user-empty">暂无待交收资金</td></tr>}</tbody></table></div>
      </>}
      {tab==='positions'&&<div className="table-scroll"><table><thead><tr><th>证券</th><th>方向</th><th>数量 (股)</th><th>可卖 (股)</th><th>持仓成本 (模拟币)</th><th>融资本金</th><th>初始保证金率</th></tr></thead><tbody>{positions.map(pos=><tr key={pos.stockId}><td>{stockName(pos.stockId)}<small>{stockSymbol(pos.stockId)}</small></td><td>{pos.quantity>0?'多头':'空头'}</td><td>{Math.abs(pos.quantity).toLocaleString()}</td><td>{pos.quantity>0?(pos.lots??[]).filter(l=>l.availableAt<=now).reduce((total,l)=>total+l.quantity,0).toLocaleString():'—'}</td><td>{money(pos.cost)}</td><td>{money(pos.loan??0)}</td><td>{((pos.marginPPM??0)/10000).toFixed(2)}%</td></tr>)}{!positions.length&&<tr><td colSpan={7} className="user-empty">当前没有持仓</td></tr>}</tbody></table></div>}
      {tab==='orders'&&<><p className="muted user-table-note">显示全部挂单及最近 100 笔已结束委托。</p><div className="table-scroll"><table><thead><tr><th>委托 / 证券</th><th>方向 / 类型</th><th>数量 / 杠杆</th><th>委托价 / 成交价</th><th>费用 / 冻结</th><th>时间</th><th>状态 / 原因</th></tr></thead><tbody>{orders.map(o=><tr key={o.id}><td>#{o.id} · {stockName(o.stockId)}<small>{stockSymbol(o.stockId)}</small></td><td>{sideName[o.side]??o.side}<small>{kindName[o.kind]??o.kind} · {o.tif}</small></td><td>{o.quantity.toLocaleString()} 股<small>{o.leverage||1} 倍</small></td><td>{o.kind==='market'?'市价':money(o.limit)}<small>成交 {o.status==='filled'?money(o.price):'—'}</small></td><td>{money(feesTotal(o.fees))}<small>冻结 {money(o.reserved)}</small></td><td>{stamp(o.createdAt)}<small>{o.executedAt?`成交 ${stamp(o.executedAt)}`:o.expiresAt?`截止 ${stamp(o.expiresAt)}`:'GTC'}</small></td><td>{statusName[o.status]??o.status}<small>{o.reason||'—'}</small></td></tr>)}{!orders.length&&<tr><td colSpan={7} className="user-empty">暂无委托记录</td></tr>}</tbody></table></div></>}
    </>}
  </section>
}
