import { test, expect } from '@playwright/test'

// Sign in with a second factor, copy the subscription link from the
// overview (plan §3: the link within two clicks of signing in), then scan
// and import from the subscription page. The API is mocked.
const GIB = 1024 ** 3
const TOKEN = '7f3k9q2m8x4v2c6b'

async function installFixtures(page, calls) {
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const json = body => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
    if (path === '/api/v4/public/config') return json({ edition: 'community', registration: { enabled: true, require_invite: false } })
    if (path === '/api/v2/login') {
      const body = request.postDataJSON()
      calls.push(body)
      if (!body.mfa_code) return json({ code: 0, data: { mfa_required: true, methods: ['totp', 'backup'], email: body.email } })
      return json({ code: 0, data: { token: 'portal-e2e-token', is_admin: false, user_id: 7, email: body.email } })
    }
    if (path === '/api/v2/user/profile') return json({ code: 0, data: { id: 7, email: 'lin.xiao@example.test', token: TOKEN, is_admin: false } })
    if (path === '/api/v2/user/subscription') {
      return json({ code: 0, data: { plan_id: 3, plan_name: 'Standard', transfer_enable: 200 * GIB, used_traffic: 71.6 * GIB, expired_at: 1795996800, subscribe_path: '/s' } })
    }
    if (path === '/api/v2/user/knowledge' || path === '/api/v2/user/ticket') return json({ code: 0, data: [] })
    return json({ code: 0, data: null })
  })
}

test('signs in with a code and gets the subscription link in two clicks', async ({ page, context, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))
  const calls = []
  await installFixtures(page, calls)

  await page.goto('/login')
  await expect(page.getByRole('heading', { level: 1, name: 'Sign in to AnixOps Control' })).toBeVisible()
  await page.locator('#email').fill('lin.xiao@example.test')
  await page.locator('#password').fill('correct-horse-battery')
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page.getByRole('heading', { level: 1, name: 'Two-factor authentication' })).toBeVisible()
  await expect(page.getByLabel('Digit 1 of 6')).toBeFocused()
  await page.keyboard.type('123456')
  await expect(page).toHaveURL(/\/user\/dashboard$/)
  expect(calls.at(-1)).toMatchObject({ mfa_code: '123456', mfa_method: 'totp' })

  const link = `${new URL(baseURL).origin}/s/${TOKEN}`
  await expect(page.getByRole('img', { name: '128.4 GB of 200 GB traffic left' })).toBeVisible()
  await page.getByRole('button', { name: 'Copy subscription link' }).click()
  await expect(page.getByText('Subscription link copied')).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(link)

  await page.getByRole('link', { name: 'Import to a client' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Subscription' })).toBeVisible()
  await expect(page.getByRole('img', { name: 'QR code of your subscription link' })).toBeVisible()
  const clash = await page.getByRole('link', { name: 'Import to Clash Verge' }).getAttribute('href')
  expect(decodeURIComponent(clash)).toContain(`${link}?type=clash`)
})
