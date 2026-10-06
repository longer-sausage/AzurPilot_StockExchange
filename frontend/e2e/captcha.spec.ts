import {expect,test} from '@playwright/test'
import {completeCaptcha} from './recaptcha'

test('管理页面 CSP 允许 reCAPTCHA 国内资源并限制资源路径',async({page},testInfo)=>{
  const allowed='https://www.gstatic.cn/recaptcha/mmex-csp-probe.js'
  const blocked='https://www.gstatic.cn/mmex-csp-probe.js'
  for(const url of [allowed,blocked]){
    await page.route(url,route=>route.fulfill({contentType:'application/javascript',body:'/* CSP 资源加载探针，不生成验证码 token。 */'}))
  }
  // 直接访问 Go 返回的页面；Vite 开发页面不携带生产 CSP。
  const response=await page.goto(`http://127.0.0.1:${process.env.MOCK_API_PORT??8080}/console`)
  expect(response?.ok()).toBeTruthy()
  const results=await page.evaluate(async urls=>{
    const probe=(src:string)=>new Promise<boolean>(resolve=>{
      const script=document.createElement('script')
      const finish=(loaded:boolean)=>{clearTimeout(timeout);script.remove();resolve(loaded)}
      const timeout=setTimeout(()=>finish(false),5000)
      script.onload=()=>finish(true)
      script.onerror=()=>finish(false)
      script.src=src
      document.head.append(script)
    })
    return {allowed:await probe(urls.allowed),blocked:await probe(urls.blocked)}
  },{allowed,blocked})
  expect(results).toEqual({allowed:true,blocked:false})
  await expect(page.frameLocator('iframe[title="reCAPTCHA"]').getByRole('checkbox')).toBeVisible({timeout:20000})
  await page.screenshot({path:testInfo.outputPath('captcha-csp.png'),fullPage:true})
})

test('管理员认证失败后清空验证码，使用新 token 重试',async({page})=>{
  const errors:string[]=[],requests:string[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  page.on('request',request=>requests.push(request.url()))
  await page.goto('/console')
  const submit=page.getByRole('button',{name:'验证并登录控制台'})
  await page.getByLabel('管理员密码').fill('wrong-admin-password')
  await expect(submit).toBeDisabled()
  const firstToken=await completeCaptcha(page)
  const rejected=page.waitForResponse(response=>response.url().endsWith('/api/console/login'))
  await submit.click()
  expect((await rejected).status()).toBe(401)
  await expect(page.getByRole('alert')).toContainText('管理员密码错误')
  await expect(submit).toBeDisabled()
  await expect(page.locator('textarea[name="g-recaptcha-response"]')).toHaveValue('')
  await page.getByLabel('管理员密码').fill('mock-admin-password')
  const nextToken=await completeCaptcha(page)
  expect(nextToken).not.toBe(firstToken)
  const accepted=page.waitForResponse(response=>response.url().endsWith('/api/console/login'))
  await submit.click()
  const response=await accepted
  expect(response.ok()).toBeTruthy()
  expect(response.request().postDataJSON()).toEqual({password:'mock-admin-password',recaptchaToken:nextToken})
  await expect(page.getByRole('heading',{name:'交易规则与市场管理'})).toBeVisible()
  expect(requests.some(url=>/https:\/\/(?:[^/]*\.)?google\.com\//.test(url)||url.includes('challenges.cloudflare.com'))).toBeFalsy()
  expect(errors).toEqual([])
})
