import {test,expect} from '@playwright/test'

test('控制台验证码登录、创建发布预设与服务首页',async({page})=>{
  await page.goto('/');await expect(page.getByText('交易终端已集成到 AzurPilot。')).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.goto('/console');await page.getByLabel('管理员密码').fill('mock-admin-password')
  await expect(page.locator('input[name="cf-turnstile-response"]')).toHaveValue('XXXX.DUMMY.TOKEN.XXXX',{timeout:20000})
  await assertCaptchaCentered(page)
  await page.setViewportSize({width:390,height:844});await assertCaptchaCentered(page);await page.screenshot({path:'test-results/console-captcha-mobile.png',fullPage:true})
  await page.setViewportSize({width:1600,height:1080})
  await page.getByRole('button',{name:'验证并登录控制台'}).click();await expect(page.getByRole('heading',{name:'交易规则与市场管理'})).toBeVisible()
  await expect(page.getByLabel('初始资金 (模拟币)',{exact:true})).toHaveValue('20000000')
  const before=await (await page.request.get('/api/market')).json()
  await page.getByLabel('新预设标识').fill(`custom-${Date.now()}`);await page.getByLabel('新预设名称').fill('新手练习');await page.getByRole('button',{name:'另存为预设'}).click()
  await page.getByLabel('佣金率 (%)').fill('0.02');await page.getByLabel('融资年利率 (%)').fill('6.25');await page.getByRole('button',{name:'更新当前预设'}).click()
  await page.getByLabel('初始资金 (模拟币)',{exact:true}).fill('25000000')
  const published=page.waitForResponse(r=>r.url().endsWith('/api/console/settings')&&r.request().method()==='PUT')
  await page.getByRole('button',{name:'发布配置'}).click();const settings=await (await published).json();expect(settings.active.financingAnnualPPM).toBe(62500);expect(settings.initialCash).toBe(2500000000)
  const meta=await (await page.request.get('/api/meta')).json(),market=await (await page.request.get('/api/market')).json()
  expect(meta.initialCash).toBe(2500000000);expect(market.initialCash).toBe(2500000000);expect(market.rankings).toEqual(before.rankings)
  await expect(page.getByRole('status')).toContainText('配置已发布');await page.screenshot({path:'test-results/console.png',fullPage:true})
  await page.getByRole('button',{name:'删除预设',exact:true}).click();await page.getByRole('button',{name:/茗喵全天候/}).click()
  await page.getByLabel('初始资金 (模拟币)',{exact:true}).fill('20000000')
  const cleaned=page.waitForResponse(r=>r.url().endsWith('/api/console/settings')&&r.request().method()==='PUT')
  await page.getByRole('button',{name:'发布配置'}).click();expect((await cleaned).ok()).toBeTruthy()
})

async function assertCaptchaCentered(page:import('@playwright/test').Page){
  const container=page.locator('.captcha-widget'),widget=container.locator(':scope > div').first()
  await expect.poll(async()=>(await widget.boundingBox())?.width??0).toBeGreaterThanOrEqual(299);await widget.scrollIntoViewIfNeeded();const a=await container.boundingBox(),b=await widget.boundingBox()
  expect(b!.width).toBeGreaterThanOrEqual(299);expect(Math.abs(a!.x+a!.width/2-b!.x-b!.width/2)).toBeLessThan(2)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy()
}
