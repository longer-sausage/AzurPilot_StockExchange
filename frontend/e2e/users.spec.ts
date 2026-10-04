import {expect,test} from '@playwright/test'
import {completeCaptcha} from './recaptcha'
import {generateKeyPairSync,randomUUID,sign} from 'node:crypto'

test('用户管理查询、详情、编辑、资金校验、密码重设和停用恢复',async({page})=>{
  test.setTimeout(90000)
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  const {publicKey,privateKey}=generateKeyPairSync('ed25519'),instanceId=randomUUID(),pub=publicKey.export({format:'der',type:'spki'}).subarray(-32).toString('base64')
  const report=()=>{const now=Math.floor(Date.now()/1000),value={instanceId,publicKey:pub,actionPoints:1234,observedAt:now,issuedAt:now};return {...value,signature:sign(null,Buffer.from(`mmex-instance-v1\n${instanceId}\n${pub}\n1234\n${now}\n${now}\n`),privateKey).toString('base64')}}
  const name=`管理测试${Date.now()}`
  const registered=await page.request.post('/api/register',{data:{username:name,password:'test-player-password',acceptedNotice:'2026-10-03',recaptchaToken:'recaptcha-test-token',report:report()}})
  expect(registered.ok()).toBeTruthy();const original=await registered.json(),identityCode=original.player.identityCode
  expect(identityCode).toMatch(/^MMEX-(?:[A-F0-9]{8}-){3}[A-F0-9]{8}$/)
  const headers={Authorization:`Bearer ${original.token}`,'X-MMEX-Instance':original.player.binding.key}
  expect((await page.request.post('/api/orders',{headers,data:{clientId:`buy_${Date.now()}`,stockId:1,side:'buy',kind:'market',tif:'GTC',quantity:10,limit:0}})).ok()).toBeTruthy()
  expect((await page.request.post('/api/orders',{headers,data:{clientId:`pending_${Date.now()}`,stockId:1,side:'buy',kind:'limit',tif:'GTC',quantity:10,limit:100}})).ok()).toBeTruthy()
  await page.goto('/console');await page.getByLabel('管理员密码').fill('mock-admin-password')
  await completeCaptcha(page)
  const adminLogin=page.waitForResponse(r=>r.url().endsWith('/api/console/login'))
  await page.getByRole('button',{name:'验证并登录控制台'}).click();const adminResponse=await adminLogin
  if(!adminResponse.ok())throw new Error((await adminResponse.json()).error.message)
  await expect(page.getByRole('heading',{name:'交易规则与市场管理'})).toBeVisible({timeout:20000})
  await page.getByRole('button',{name:'用户管理',exact:true}).click();await page.getByLabel('搜索用户').fill(identityCode)
  const row=page.locator('.user-table tbody tr').filter({hasText:name});await expect(row).toBeVisible();await expect(row).toContainText(identityCode);await expect(row).toContainText('已永久绑定')
  await row.getByRole('button',{name:`管理用户 ${name}`}).click();const detail=page.getByRole('region',{name:'用户详情'})
  await expect(detail.getByRole('heading',{name:`${name} · 用户详情`})).toBeVisible();await expect(detail.locator('.user-identity')).toContainText(identityCode);await expect(detail.locator('.user-readonly')).toContainText(instanceId)
  await detail.getByRole('button',{name:'资金与交收',exact:true}).click();await expect(detail.locator('.user-metrics').first()).toContainText('账户净值')
  await detail.getByRole('button',{name:'持仓 (1)',exact:true}).click();await expect(detail.locator('tbody')).toContainText('茗喵');await expect(detail.locator('tbody')).toContainText('10')
  await detail.getByRole('button',{name:'委托记录 (2)',exact:true}).click();await expect(detail.locator('tbody')).toContainText('已成交');await expect(detail.locator('tbody')).toContainText('待触发')
  await detail.getByRole('button',{name:'基本资料与编辑',exact:true}).click()
  await page.screenshot({path:'test-results/console-users-desktop.png',fullPage:true})
  await page.setViewportSize({width:390,height:844});await detail.scrollIntoViewIfNeeded();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();await page.screenshot({path:'test-results/console-users-mobile.png',fullPage:true});await page.setViewportSize({width:1600,height:1080})
  const before=(await (await page.request.get('/api/account',{headers})).json()).player
  await detail.getByLabel('用户名称',{exact:true}).fill('茗喵');await detail.getByLabel('用户现金调整 (模拟币)').fill('1234.56');await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('alert')).toContainText('用户名已被注册')
  expect((await (await page.request.get('/api/account',{headers})).json()).player.cash).toBe(before.cash)
  const newName=`更名测试${Date.now()}`;await detail.getByLabel('用户名称',{exact:true}).fill(newName);await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('status')).toContainText('用户信息已保存')
  const saved=(await (await page.request.get('/api/account',{headers})).json()).player;expect(saved.username).toBe(newName);expect(saved.cash).toBe(before.cash+123456);expect(saved.initialCash).toBe(before.initialCash);expect(saved.identityCode).toBe(identityCode)
  await detail.getByLabel('重设用户密码').fill('new-player-password');await detail.getByLabel('确认用户新密码').fill('wrong-password');await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('alert')).toContainText('两次输入的新密码不一致')
  await detail.getByLabel('确认用户新密码').fill('new-player-password');await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('status')).toContainText('旧会话已失效')
  expect((await page.request.get('/api/account',{headers})).status()).toBe(401)
  const login=await page.request.post('/api/login',{data:{username:newName,password:'new-player-password',recaptchaToken:'recaptcha-test-token',report:report()}});expect(login.ok()).toBeTruthy();const logged=await login.json();expect(logged.player.identityCode).toBe(identityCode)
  await detail.getByLabel('用户账户状态').selectOption('disabled');await expect(detail.getByRole('button',{name:'保存用户信息'})).toBeDisabled();await detail.getByRole('checkbox',{name:/确认停用/}).check();await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('status')).toContainText('已平仓本人持仓')
  await page.getByLabel('筛选账户状态').selectOption('disabled');await expect(page.locator('.user-table tbody tr').filter({hasText:newName})).toContainText('已停用')
  const adminToken=await page.evaluate(()=>sessionStorage.getItem('mmex.admin'))
  const disabledAccount=await (await page.request.get(`/api/console/players/${original.player.id}`,{headers:{Authorization:`Bearer ${adminToken}`}})).json();expect(Object.keys(disabledAccount.player.positions)).toHaveLength(0);expect(disabledAccount.player.orders.some((o:{status:string})=>o.status==='pending')).toBeFalsy();expect(disabledAccount.player.identityCode).toBe(identityCode)
  await detail.getByLabel('用户账户状态').selectOption('active');await detail.getByRole('button',{name:'保存用户信息'}).click();await expect(detail.getByRole('status')).toContainText('用户信息已保存')
  await page.getByLabel('筛选账户状态').selectOption('active');await expect(page.locator('.user-table tbody tr').filter({hasText:newName})).toContainText('正常')
  expect((await page.request.get('/api/account',{headers:{...headers,Authorization:`Bearer ${logged.token}`}})).status()).toBe(401)
  await page.getByLabel('搜索用户').fill('不存在的用户');await expect(page.getByText('未找到符合条件的用户',{exact:true})).toBeVisible()
  await detail.getByRole('button',{name:'关闭用户详情'}).click();await expect(detail).not.toBeVisible();expect(errors).toEqual([])
})
