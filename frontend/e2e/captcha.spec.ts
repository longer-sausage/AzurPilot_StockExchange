import {expect,test} from '@playwright/test'
import {completeCaptcha} from './recaptcha'

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
