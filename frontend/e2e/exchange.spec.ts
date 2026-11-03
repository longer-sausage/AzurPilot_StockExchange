import {test,expect,type APIRequestContext,type Page} from '@playwright/test'
import {generateKeyPairSync,randomUUID,sign} from 'node:crypto'
import type {Settings} from '../src/types'

const token='isolated-bot-token-for-tests-'.repeat(3),bot={Authorization:`Bearer ${token}`}
const password='E2e-Secure-Password!87'
const adminPassword='isolated-admin-password'
let originalSettings:Settings

async function admin(request:APIRequestContext){
  const login=await request.post('/api/v2/admin/login',{data:{password:adminPassword,remember:false}})
  expect(login.ok()).toBeTruthy()
}
async function saveSettings(request:APIRequestContext,settings:Settings){
  const response=await request.put('/api/v2/admin/settings',{data:settings})
  expect(response.ok(),await response.text()).toBeTruthy()
  return await response.json() as Settings
}
async function botRegister(request:APIRequestContext,officialUserId:string,username:string){
 const requestId=randomUUID(),response=await request.post('/api/v2/bot/register',{headers:bot,data:{officialUserId,username,requestId}})
 expect(response.ok(),await response.text()).toBeTruthy()
 return {...await response.json(),requestId}
}
function instance(){
  const pair=generateKeyPairSync('ed25519')
  const publicKey=pair.publicKey.export({type:'spki',format:'der'}).subarray(-32).toString('base64'),instanceId=randomUUID()
  return {body(action:string,username='',points:{t:number;p:number}[]=[],months:{month:string;count:number;digest:string}[]=[]){
    const issuedAt=Math.floor(Date.now()/1000),canonical=`mmex-v2\n${action}\n${instanceId}\n${publicKey}\n${issuedAt}\n${username}\n${JSON.stringify(points)}\n${action==='reconcile'?`${JSON.stringify(months)}\n`:''}`
    return {instanceId,publicKey,issuedAt,username,points,...(action==='reconcile'?{months}:{}),signature:sign(null,Buffer.from(canonical),pair.privateKey).toString('base64')}
  }}
}
async function stock(request:APIRequestContext,label:string,points=[{t:Date.now(),p:1000}]){
  const reporter=instance(),name=label+randomUUID().slice(0,8)
  const registered=await request.post('/api/v2/instance/register',{data:reporter.body('register',name)})
  expect(registered.ok()).toBeTruthy()
  const state=await registered.json()
  const sent=await request.post('/api/v2/instance/report',{data:reporter.body('report','',points)})
  expect(sent.ok()).toBeTruthy()
  return {reporter,name,id:state.stock.id as number,bindingCode:state.bindingCode as string}
}
async function createAccount(request:APIRequestContext,label:string,bindOwn=true){
 const official=`e2e:${randomUUID()}`,username=label+randomUUID().slice(0,8),result=await botRegister(request,official,username)
 expect((await request.post('/api/v2/auth/login',{data:{username,password:result.password}})).ok()).toBeTruthy()
 expect((await request.post('/api/v2/auth/password',{data:{currentPassword:result.password,password}})).ok()).toBeTruthy()
 if(bindOwn){const own=await stock(request,'归属'+label);expect((await request.post('/api/v2/bot/bind',{headers:bot,data:{officialUserId:official,bindingCode:own.bindingCode}})).ok()).toBeTruthy()}
 return {official,username,id:result.accountId as number}
}
async function registerInBrowser(page:Page,request:APIRequestContext,label:string){
 const official=`e2e:${randomUUID()}`,username=label+randomUUID().slice(0,8),result=await botRegister(request,official,username)
 await page.goto('/')
 await page.getByRole('button',{name:'登录交易账户',exact:true}).click()
 const login=page.getByRole('dialog',{name:'登录交易账户'})
 await login.getByLabel('账号',{exact:true}).fill(username);await login.getByLabel('密码',{exact:true}).fill(result.password)
 await login.getByRole('button',{name:'登录',exact:true}).click()
 const forced=page.getByRole('dialog',{name:'请先设置新密码'});await expect(forced).toBeVisible()
 await forced.getByLabel('当前临时密码').fill(result.password)
 await forced.getByLabel('新密码',{exact:true}).fill(password);await forced.getByLabel('再次输入新密码').fill(password)
 await forced.getByRole('button',{name:'设置新密码并继续'}).click()
 const binding=page.getByRole('dialog',{name:'股票账户绑定'});await expect(binding).toBeVisible()
 const denied=await page.request.get('/api/v2/account');expect(denied.status()).toBe(400);expect((await denied.json()).error.code).toBe('BINDING_REQUIRED')
 const own=await stock(request,'归属'+label)
 expect((await request.post('/api/v2/bot/bind',{headers:bot,data:{officialUserId:official,bindingCode:own.bindingCode}})).ok()).toBeTruthy()
 if(await binding.isVisible())await binding.getByRole('button',{name:'检查绑定状态'}).click()
 await expect(binding).toHaveCount(0)
 return {official,username,registrationKey:result.requestId,own}
}
async function loginInBrowser(page:Page,username:string,remember=false){
  await page.getByRole('button',{name:'登录交易账户',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'登录交易账户'})
  await dialog.getByLabel('账号',{exact:true}).fill(username)
  await dialog.getByLabel('密码',{exact:true}).fill(password)
  if(remember)await dialog.getByLabel('记住我（30 天）').check()
  await dialog.getByRole('button',{name:'登录',exact:true}).click()
  await expect(dialog).toHaveCount(0)
}
async function selectStock(page:Page,name:string){
  await page.getByLabel('搜索股票').fill(name)
  await page.locator('.stock-row').filter({hasText:name}).getByRole('button',{name,exact:true}).click()
  await expect(page.locator('.quote-panel h2')).toContainText(name)
}
async function trade(page:Page,quantity:number,leverage:1|2){
  await page.getByLabel('委托股数').fill(String(quantity))
  await page.getByLabel('委托限价').fill('2000')
  await page.getByRole('combobox',{name:/买入方式/}).click()
  await page.getByRole('option',{name:leverage===1?'现金 / 1 倍':'2 倍杠杆',exact:true}).click()
  const previewResponse=page.waitForResponse(response=>response.url().endsWith('/api/v2/orders/preview')&&response.request().method()==='POST')
  await page.getByRole('button',{name:'预览交易明细'}).click()
  const previewHTTP=await previewResponse
  expect(previewHTTP.ok(),await previewHTTP.text()).toBeTruthy()
  expect(previewHTTP.request().postDataJSON().leverage).toBe(leverage)
  const preview=await previewHTTP.json()
  expect(preview.leverage).toBe(leverage)
  const dialog=page.getByRole('dialog',{name:'确认交易委托'})
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText(`${quantity} 股`,{exact:true})).toBeVisible()
  const confirmation=page.waitForResponse(response=>response.url().endsWith('/api/v2/orders/confirm')&&response.request().method()==='POST')
  await dialog.getByRole('button',{name:'确认提交委托'}).click()
  const confirmed=await confirmation
  expect(confirmed.ok(),await confirmed.text()).toBeTruthy()
  await expect(dialog).toHaveCount(0)
  return {preview,order:(await confirmed.json()).order}
}

test.beforeAll(async({request})=>{
  // 只修改隔离 E2E 数据库：当前为上海 11 月 1 日，默认 5 日开赛会阻止交易验收。
  await admin(request)
  originalSettings=await (await request.get('/api/v2/admin/settings')).json() as Settings
  expect(originalSettings.marketPresets.map(p=>p.id)).toEqual(expect.arrayContaining(['meow','a-share','hk','us']))
  await saveSettings(request,{...originalSettings,activeMarket:structuredClone(originalSettings.marketPresets.find(p=>p.id==='meow')!),startDay:1,endDaysFromLast:0,delistThreshold:0,paused:false})
})
test.afterAll(async({request})=>{
  if(originalSettings){await admin(request);await saveSettings(request,originalSettings)}
})

test('公开行情与规则无需登录，桌面和手机无横向溢出',async({page})=>{
  await page.goto('/')
  await expect(page.getByRole('heading',{name:/市场行情/})).toBeVisible()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.getByRole('button',{name:'交易规则',exact:true}).click()
  await expect(page.getByRole('heading',{name:'总资产阶梯税率'})).toBeVisible()
  await expect(page.getByText('32.00%',{exact:true})).toBeVisible()
  for(const width of [1600,900,390]){
    await page.setViewportSize({width,height:950})
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy()
    await page.screenshot({path:`test-results/rules-${width}.png`,fullPage:true})
  }
})

test('机器人直接开户、强制改密、记住我与重置撤销会话',async({page,request})=>{
  const {official,username,registrationKey}=await registerInBrowser(page,request,'指挥官')
  const repeated=await request.post('/api/v2/auth/register',{data:{registrationKey,username:'不可重复'}})
  expect(repeated.ok()).toBeFalsy()
  await page.getByRole('button',{name:'退出登录'}).click()
  await loginInBrowser(page,username,true)
  const cookie=(await page.context().cookies()).find(c=>c.name==='mmex_player')
  expect(cookie?.httpOnly).toBeTruthy()
  expect(cookie?.expires).toBeGreaterThan(Date.now()/1000+28*86400)
  const reset=await request.post('/api/v2/bot/reset-password',{headers:bot,data:{officialUserId:official,confirmed:true,requestId:randomUUID()}})
  expect(reset.ok()).toBeTruthy()
  expect((await page.request.get('/api/v2/auth/me')).status()).toBe(401)
})

test('实例独立注册、绑定、下单二次确认、幂等与库存展示',async({page,request})=>{
  const listed=await stock(request,'海风')
  const owner=await createAccount(request,'股东',false)
  expect((await request.post('/api/v2/bot/bind',{headers:bot,data:{officialUserId:owner.official,bindingCode:listed.bindingCode}})).ok()).toBeTruthy()
  await registerInBrowser(page,request,'交易者')
  await selectStock(page,listed.name)
  const {preview,order}=await trade(page,10,1)
  expect(order.filled).toBe(10)
  const again=await page.request.post('/api/v2/orders/confirm',{data:{previewId:preview.previewId}})
  expect(again.ok()).toBeTruthy()
  expect((await again.json()).order.id).toBe(order.id)
  const account=await (await page.request.get('/api/v2/account')).json()
  expect(account.positions.find((p:{stockId:number})=>p.stockId===listed.id)?.quantity).toBe(10)
  await page.getByRole('button',{name:'持仓与委托',exact:true}).click()
  await expect(page.locator('tbody').first()).toContainText(listed.name)
  await expect(page.locator('tbody').first()).toContainText('10')
  await page.screenshot({path:'test-results/portfolio.png',fullPage:true})
})

test('现金与融资买入分开落账，重新登录保留真实持仓和融资借款',async({page,request})=>{
  const listed=await stock(request,'融资行情')
  const account=await createAccount(request,'融资用户')
  await page.goto('/')
  await loginInBrowser(page,account.username)
  await selectStock(page,listed.name)
  const cash=await trade(page,10,1)
  expect(cash.order.filled).toBe(10)
  const afterCash=await (await page.request.get('/api/v2/account')).json()
  const cashPosition=afterCash.positions.find((p:{stockId:number})=>p.stockId===listed.id)
  expect(cashPosition.quantity).toBe(10)
  expect(cashPosition.loan??0).toBe(0)
  const financing=await trade(page,10,2)
  expect(financing.order.filled).toBe(10)
  const financed=await (await page.request.get('/api/v2/account')).json()
  const position=financed.positions.find((p:{stockId:number})=>p.stockId===listed.id)
  expect(position.quantity).toBe(20)
  expect(position.loan).toBeGreaterThan(0)
  expect(position.loan).toBeLessThan(financing.order.notional)
  await page.getByRole('button',{name:'退出登录'}).click()
  await loginInBrowser(page,account.username)
  await page.getByRole('button',{name:'持仓与委托',exact:true}).click()
  await expect(page.locator('tbody').first()).toContainText(listed.name)
  await expect(page.locator('tbody').first()).toContainText('20')
  const resumed=await (await page.request.get('/api/v2/account')).json()
  expect(resumed.positions.find((p:{stockId:number})=>p.stockId===listed.id)?.loan).toBeGreaterThan(0)
})

test('旧历史同毫秒修正通过 historyVersion 刷新图表，现价和报价时间保持不变',async({page,request})=>{
  const oldTime=Date.now()-90*60000,latestTime=Date.now()-2*60000
  const listed=await stock(request,'修正行情',[{t:oldTime,p:100},{t:latestTime,p:110}])
  await page.goto('/')
  await selectStock(page,listed.name)
  const chart=page.locator('.financial-canvas canvas');await expect(chart.first()).toBeVisible()
  const previousMarket=await (await request.get('/api/v2/public/market')).json()
  const before=previousMarket.stocks.find((s:{id:number})=>s.id===listed.id)
  const refreshed=page.waitForResponse(response=>response.url().includes(`/api/v2/public/stocks/${listed.id}/chart-detail?`)&&response.status()===200)
  const correction=await request.post('/api/v2/instance/report',{data:{...listed.reporter.body('report','',[{t:oldTime,p:150}]),backfill:true}})
  expect(correction.ok()).toBeTruthy()
  const correctedChart=await (await refreshed).json()
  expect(correctedChart.bars.some((p:{high:number})=>p.high===15000)).toBeTruthy()
  await expect(page.locator('.quote-metrics').first()).toContainText('150.00')
  const after=(await (await request.get('/api/v2/public/market')).json()).stocks.find((s:{id:number})=>s.id===listed.id)
  expect(after.price).toBe(before.price)
  expect(after.observedAt).toBe(before.observedAt)
  expect(after.historyVersion).toBeGreaterThan(before.historyVersion)
})

test('管理员记住我、参数保存与手机布局',async({page})=>{
  await page.goto('/console')
  await page.getByLabel('管理员密码').fill(adminPassword)
  await page.getByLabel('记住我（30 天）').check()
  await page.getByRole('button',{name:'登录控制台'}).click()
  await expect(page.getByRole('heading',{name:'交易所管理',exact:true})).toBeVisible()
  const cookie=(await page.context().cookies()).find(c=>c.name==='mmex_admin')
  expect(cookie?.expires).toBeGreaterThan(Date.now()/1000+28*86400)
  const initial=await (await page.request.get('/api/v2/admin/settings')).json() as Settings
  try{
    await page.getByLabel('最大溢价（%）',{exact:true}).fill('4')
    await page.getByRole('button',{name:'保存交易规则'}).click()
    await expect(page.getByRole('status')).toContainText('已保存')
    expect((await (await page.request.get('/api/v2/public/rules')).json()).premiumPPM).toBe(40000)
    for(const width of [1600,390]){
      await page.setViewportSize({width,height:950})
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy()
      await page.screenshot({path:`test-results/admin-${width}.png`,fullPage:true})
    }
  }finally{await saveSettings(page.request,initial)}
})

test('现实市场预设原参数完整保存，选择预设不覆盖公共数量或其他预设',async({page})=>{
  await page.goto('/console')
  await page.getByLabel('管理员密码').fill(adminPassword)
  await page.getByRole('button',{name:'登录控制台'}).click()
  await expect(page.getByRole('heading',{name:'交易所管理',exact:true})).toBeVisible()
  const initial=await (await page.request.get('/api/v2/admin/settings')).json() as Settings
  const expected={
    'a-share':{lotSize:100,stampPPM:500,sellDelayDays:1,cashDelayDays:0,timezone:'Asia/Shanghai',imrPPM:1000000,mmrPPM:500000},
    hk:{lotSize:100,stampPPM:1000,stampRound:100,levyPPM:85,cashDelayDays:2,timezone:'Asia/Shanghai',imrPPM:500000,mmrPPM:300000},
    us:{lotSize:1,commissionPPM:0,minCommission:100,cashDelayDays:1,timezone:'America/New_York',imrPPM:500000,mmrPPM:300000},
  }
  try{
    for(const id of ['a-share','hk','us'] as const){
      const preset=initial.marketPresets.find(p=>p.id===id)!
      expect(preset).toMatchObject(expected[id])
      await page.locator('.preset-grid').getByRole('button').filter({hasText:preset.name}).click()
      await expect(page.getByLabel('当前预设名称',{exact:true})).toHaveValue(preset.name)
      await expect(page.getByLabel('市场时区',{exact:true})).toHaveValue(preset.timezone)
      const saved=page.waitForResponse(response=>response.url().endsWith('/api/v2/admin/settings')&&response.request().method()==='PUT')
      await page.getByRole('button',{name:'保存交易规则'}).click()
      expect((await saved).ok()).toBeTruthy()
      await expect(page.getByRole('status')).toContainText('已保存')
      const settings=await (await page.request.get('/api/v2/admin/settings')).json() as Settings
      expect(settings.activeMarket).toEqual(preset)
      expect(settings.marketPresets).toEqual(initial.marketPresets)
      for(const field of ['issuedShares','hourlyVolume','borrowShares','borrowHourlyVolume','premiumPPM','taxBrackets','startDay','endDaysFromLast','delistThreshold'] as const)expect(settings[field]).toEqual(initial[field])
    }
  }finally{await saveSettings(page.request,initial)}
})


test('自选跨刷新保存、完整筛选排序、排行榜和账户明细',async({page,request})=>{
 const listed=await stock(request,'自选目标'),user=await registerInBrowser(page,request,'自选玩家')
 await selectStock(page,listed.name)
 await page.getByRole('button',{name:`添加自选 ${listed.name}`,exact:true}).click()
 await expect(page.getByRole('button',{name:`移除自选 ${listed.name}`,exact:true})).toHaveAttribute('aria-pressed','true')
 await page.reload();await page.getByRole('button',{name:'自选股 1',exact:true}).click()
 await expect(page.locator('.stock-row')).toHaveCount(1)
 await page.getByRole('button',{name:'展开筛选'}).click();await page.getByLabel('最低价格',{exact:true}).fill('2000')
 await expect(page.locator('.stock-row')).toHaveCount(0)
 await page.getByRole('button',{name:'重置筛选与排序'}).click();await page.getByLabel('证券排序').click();await page.getByRole('option',{name:'价格 · 降序',exact:true}).click()
 await page.getByRole('button',{name:'月赛排行榜',exact:true}).click()
 await expect(page.getByRole('heading',{name:'月赛与排行榜'})).toBeVisible()
 await expect(page.locator('tbody tr').filter({hasText:user.username})).toContainText('我')
 await page.getByRole('button',{name:'持仓与委托',exact:true}).click();await page.getByRole('button',{name:'账户明细',exact:true}).click()
 await expect(page.getByText('身份识别码',{exact:true})).toBeVisible();await expect(page.locator('.receipt').first()).toContainText('MMEX-')
 await page.getByRole('button',{name:'交易规则',exact:true}).click();await page.getByRole('button',{name:'名词词典 · 37'}).click()
 await page.getByLabel('搜索交易名词').fill('MACD');await expect(page.locator('.glossary').getByRole('button',{name:'MACD / DIF / DEA'})).toBeVisible()
})

test('图表指标、缩放全屏和亮暗主题持久化，手机行情无溢出',async({page,request})=>{
 const end=Date.now()-60000,listed=await stock(request,'图表目标',Array.from({length:120},(_,i)=>({t:end-(119-i)*60000,p:1000+i%17})))
 await page.goto('/');await selectStock(page,listed.name)
 await expect(page.locator('.financial-canvas canvas').first()).toBeVisible()
 await page.getByRole('button',{name:'M5',exact:true}).click()
 for(const indicator of ['EXPMA','BOLL','ENE','BBI']){await page.getByRole('button',{name:indicator,exact:true}).click();await expect(page.getByRole('button',{name:indicator,exact:true})).toHaveAttribute('aria-pressed','true')}
 await page.getByRole('button',{name:'指标说明',exact:true}).click();await expect(page.locator('.indicator-explanation')).toContainText('ENE')
 await page.getByRole('button',{name:'放大走势图'}).click();await expect(page.getByLabel('图表可视范围')).not.toHaveAttribute('data-start','0.000')
 await page.getByRole('button',{name:'全屏走势图'}).click();await expect(page.getByRole('dialog',{name:`${listed.name}全屏走势图`})).toBeVisible();await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
 const current=await page.locator('html').getAttribute('data-stock-theme');await page.getByRole('button',{name:current==='dark'?'切换亮色主题':'切换暗色主题'}).click();const next=current==='dark'?'light':'dark'
 await expect(page.locator('html')).toHaveAttribute('data-stock-theme',next);await page.reload();await expect(page.locator('html')).toHaveAttribute('data-stock-theme',next)
 await selectStock(page,listed.name)
 for(const width of [1600,900,390]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await page.screenshot({path:`test-results/market-restored-${next}-${width}.png`,fullPage:true})}
})

test('本月仅一根分钟K线时，指标预热的旧月蜡烛不进入可视图',async({page,request})=>{
 const month=new Date().toLocaleDateString('sv-SE',{timeZone:'Asia/Shanghai'}).slice(0,7)
 const start=Date.parse(`${month}-01T00:00:00+08:00`)
 const points=[...Array.from({length:30},(_,i)=>({t:start-(30-i)*300000,p:100+i*100})),{t:Date.now()-1000,p:2000}]
 const listed=await stock(request,'月初单根行情',points)
 await page.emulateMedia({colorScheme:'dark'})
 await page.goto('/');await selectStock(page,listed.name)
 const loaded=page.waitForResponse(response=>response.url().includes(`/api/v2/public/stocks/${listed.id}/chart-detail?period=m5`)&&response.status()===200)
 await page.getByRole('button',{name:'M5',exact:true}).click();await loaded
 await page.getByRole('button',{name:'MA',exact:true}).click()
 await expect(page.locator('.financial-chart-state')).toHaveCount(0)
 await page.screenshot({path:'test-results/single-month-candle.png',fullPage:true})
 await expect.poll(async()=>page.locator('.financial-canvas canvas').evaluate(canvas=>{
  const image=(canvas as HTMLCanvasElement).getContext('2d')!.getImageData(0,0,(canvas as HTMLCanvasElement).width,(canvas as HTMLCanvasElement).height)
  let rows=0
  for(let y=30;y<image.height*.55;y++){
   let candle=false
   for(let x=80;x<image.width-20;x++){
    const i=(y*image.width+x)*4,r=image.data[i],g=image.data[i+1],b=image.data[i+2]
    if((Math.abs(r-228)<3&&Math.abs(g-136)<3&&Math.abs(b-134)<3)||(Math.abs(r-112)<3&&Math.abs(g-187)<3&&Math.abs(b-165)<3)){candle=true;break}
   }
   if(candle)rows++
  }
  return rows||Number.MAX_SAFE_INTEGER
 }),{message:'可视价格区应只绘制本月单根平价蜡烛，不能混入旧月高低价'}).toBeLessThanOrEqual(8)
 await page.screenshot({path:'test-results/single-month-candle.png',fullPage:true})
})

test('仅月摘要校对完成也自动更新图表校对时间，不需重新上传报价',async({page,request})=>{
 const listed=await stock(request,'校对状态刷新')
 await page.goto('/');await selectStock(page,listed.name)
 await expect(page.locator('.financial-chart-foot')).toContainText('1 条月原始记录')
 await expect(page.locator('.financial-chart-state')).toHaveCount(0)
 await expect(page.locator('.financial-chart-foot')).toContainText('等待后台校对')
 const before=(await (await request.get('/api/v2/public/market')).json()).stocks.find((s:{id:number})=>s.id===listed.id)
 const manifest=await request.post('/api/v2/instance/manifest',{data:listed.reporter.body('manifest')})
 expect(manifest.ok()).toBeTruthy()
 const months=(await manifest.json()).months
 const refreshed=page.waitForResponse(response=>response.url().includes(`/api/v2/public/stocks/${listed.id}/chart-detail?`)&&response.status()===200,{timeout:5000})
 const reconciled=await request.post('/api/v2/instance/reconcile',{data:listed.reporter.body('reconcile','',[],months)})
 expect(reconciled.ok()).toBeTruthy()
 expect((await (await refreshed).json()).coverage.reconciledAt).toBeGreaterThan(0)
 await expect(page.locator('.financial-chart-foot')).not.toContainText('等待后台校对')
 const after=(await (await request.get('/api/v2/public/market')).json()).stocks.find((s:{id:number})=>s.id===listed.id)
 expect(after.historyVersion).toBe(before.historyVersion)
})

test('管理员删除股票、解除绑定、旧实例停止接入及重新绑定',async({page,request})=>{
 const listed=await stock(request,'待删除股票'),owner=await createAccount(request,'删股发行者',false)
 expect((await request.post('/api/v2/bot/bind',{headers:bot,data:{officialUserId:owner.official,bindingCode:listed.bindingCode}})).ok()).toBeTruthy()
 await page.goto('/console');await page.getByLabel('管理员密码').fill(adminPassword);await page.getByRole('button',{name:'登录控制台'}).click();await page.getByRole('button',{name:'股票管理',exact:true}).click()
 await page.getByLabel('搜索管理股票').fill(listed.name);await page.locator('tr').filter({hasText:listed.name}).getByRole('button',{name:'删除股票',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:`确认删除股票 ${listed.name}`});await expect(dialog.getByRole('button',{name:'确认删除并结清'})).toBeDisabled();await dialog.getByLabel('确认删除股票代码').fill(`MM${String(listed.id).padStart(6,'0')}`);await dialog.getByRole('button',{name:'确认删除并结清'}).click();await expect(dialog).toHaveCount(0)
 expect((await request.post('/api/v2/instance/status',{data:listed.reporter.body('status')})).status()).toBe(410)
 expect((await request.post('/api/v2/instance/report',{data:listed.reporter.body('report','',[{t:Date.now(),p:1000}])})).status()).toBe(410)
 const denied=await request.get('/api/v2/bot/portfolio',{headers:bot,params:{officialUserId:owner.official}});expect((await denied.json()).error.code).toBe('BINDING_REQUIRED')
 const replacement=await stock(request,'重新绑定股票');expect((await request.post('/api/v2/bot/bind',{headers:bot,data:{officialUserId:owner.official,bindingCode:replacement.bindingCode}})).ok()).toBeTruthy()
 expect((await request.get('/api/v2/bot/portfolio',{headers:bot,params:{officialUserId:owner.official}})).ok()).toBeTruthy()
})


test('管理账户详情、改名、余额增量与二次确认重置密码',async({page,request})=>{
 const user=await createAccount(request,'完整管理账户'),renamed='管理改名'+randomUUID().slice(0,8)
 const before=(await (await request.get('/api/v2/account')).json()).account
 await page.goto('/console');await page.getByLabel('管理员密码').fill(adminPassword);await page.getByRole('button',{name:'登录控制台'}).click();await page.getByRole('button',{name:'交易账户',exact:true}).click()
 await page.getByLabel('搜索交易账户').fill(user.username);await page.locator('tr').filter({hasText:user.username}).getByRole('button',{name:'管理 / 详情'}).click()
 let dialog=page.getByRole('dialog',{name:`管理账户 ${user.username}`});await expect(dialog).toBeVisible();await dialog.locator('summary').filter({hasText:'完整账户、持仓、交收与成交'}).click();await expect(dialog.getByRole('heading',{name:'当前持仓',exact:true})).toBeVisible()
 await dialog.getByLabel('用户名',{exact:true}).fill(renamed);await dialog.getByLabel('现金增量（模拟币，可为负数）',{exact:true}).fill('2.55');await dialog.getByRole('button',{name:'保存账户修改'}).click();await expect(dialog).toHaveCount(0)
 const edited=(await (await request.get('/api/v2/account')).json()).account;expect(edited.cash).toBe(before.cash+255);expect(edited.initialCash).toBe(before.initialCash)
 await page.getByLabel('搜索交易账户').fill(renamed);await page.locator('tr').filter({hasText:renamed}).getByRole('button',{name:'管理 / 详情'}).click();dialog=page.getByRole('dialog',{name:`管理账户 ${renamed}`})
 await dialog.getByRole('button',{name:'重置该账户密码'}).click();const confirm=page.getByRole('dialog',{name:'确认管理员重置密码'});await expect(confirm).toBeVisible();await confirm.getByRole('button',{name:'确认重置密码',exact:true}).click();await expect(confirm).toHaveCount(0)
 await expect(dialog.locator('.success code')).toBeVisible();const temporary=await dialog.locator('.success code').textContent();expect(temporary?.length).toBeGreaterThanOrEqual(12)
 expect((await request.get('/api/v2/auth/me')).status()).toBe(401)
 const login=await request.post('/api/v2/auth/login',{data:{username:renamed,password:temporary}});expect(login.ok()).toBeTruthy();expect((await login.json()).forcePasswordChange).toBe(true)
})
