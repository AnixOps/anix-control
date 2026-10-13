import { test, expect } from '@playwright/test'

// The first administrator's claim link (/claim#anixclaim_…), with the API
// mocked: the token travels only in the claim's body (never in a URL, a
// Referer or the address bar once read), the page signs in afterwards, and
// it says what to do when the link expired or nothing can be claimed.
const TOKEN = 'anixclaim_' + 'q7Z-_'.repeat(8) + 'Kp3'

async function installFixtures(page, options = {}) {
  const calls = { requests: [], claims: [], logins: [] }
  page.on('request', request => {
    calls.requests.push({ url: request.url(), referer: request.headers().referer || '' })
  })
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const json = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (path === '/api/v5/claim/status') return json({ data: { claimable: options.claimable ?? true } })
    if (path === '/api/v5/claim' && request.method() === 'POST') {
      calls.claims.push(request.postDataJSON())
      if (options.claim) return json(options.claim.body, options.claim.status)
      return json({ data: { user_id: 1, email: request.postDataJSON().email, next: '/login' } }, 201)
    }
    if (path === '/api/v2/login') {
      calls.logins.push(request.postDataJSON())
      return json({ code: 0, data: { token: 'claim-e2e-token', is_admin: true, user_id: 1, email: request.postDataJSON().email } })
    }
    if (path === '/api/v4/public/config') return json({ edition: 'community', registration: { enabled: false } })
    if (path === '/api/v2/user/profile') return json({ code: 0, data: { id: 1, email: 'owner@example.com', is_admin: true } })
    if (path === '/api/v3/extensions') return json([])
    return json({ code: 0, data: null })
  })
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))
  return calls
}

async function fill(page, password = 'first-owner-Pa55') {
  await page.locator('#claim-email').fill('owner@example.com')
  await page.locator('#claim-password').fill(password)
  await page.locator('#claim-confirm').fill(password)
  await page.getByRole('button', { name: 'Create administrator' }).click()
}

test('claims the panel with the link and signs in, the token only in the claim body', async ({ page }) => {
  const calls = await installFixtures(page)
  await page.goto(`/claim#${TOKEN}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Create the first administrator' })).toBeVisible()
  // Read once and taken out of the address bar.
  await expect.poll(() => page.evaluate(() => window.location.hash)).toBe('')
  expect(page.url()).not.toContain('anixclaim_')
  await expect(page.locator('body')).not.toContainText('anixclaim_')

  await fill(page)
  await expect(page).toHaveURL(/\/admin\/dashboard$/)
  expect(calls.claims).toEqual([{ token: TOKEN, email: 'owner@example.com', password: 'first-owner-Pa55' }])
  expect(calls.logins).toEqual([{ email: 'owner@example.com', password: 'first-owner-Pa55' }])
  // No request line and no Referer ever carried the token.
  for (const request of calls.requests) {
    expect(request.url, request.url).not.toContain('anixclaim_')
    expect(request.referer, request.url).not.toContain('anixclaim_')
  }
  expect(await page.evaluate(() => localStorage.getItem('token'))).toBe('claim-e2e-token')
})

test('says when the link expired, with the command for a new one', async ({ page }) => {
  await installFixtures(page, { claim: { status: 410, body: { error: { code: 'claim_expired', message: 'this claim link has expired' } } } })
  await page.goto(`/claim#${TOKEN}`)
  await fill(page)
  await expect(page.getByRole('heading', { level: 1, name: 'This link has expired' })).toBeVisible()
  await expect(page.getByText('sudo anix-control admin claim-link')).toBeVisible()
})

test('asks for at least 12 characters, as the server does', async ({ page }) => {
  const calls = await installFixtures(page, { claim: { status: 422, body: { error: { code: 'weak_password', message: 'the password must have at least 12 characters' } } } })
  await page.goto(`/claim#${TOKEN}`)
  await expect(page.getByText('At least 12 characters.')).toBeVisible()
  await fill(page, 'eleven-char')
  await expect(page.getByText('Use at least 12 characters.')).toBeVisible()
  expect(calls.claims).toEqual([])
  // The server's answer reads the same.
  await fill(page, 'twelve-chars')
  await expect(page.getByText('Use at least 12 characters.')).toBeVisible()
  expect(calls.claims).toHaveLength(1)
})

test('shows a neutral message when nothing can be claimed', async ({ page }) => {
  await installFixtures(page, { claimable: false })
  await page.goto(`/claim#${TOKEN}`)
  await expect(page.getByRole('heading', { level: 1, name: 'No claim in progress' })).toBeVisible()
  await expect(page.locator('#claim-email')).toHaveCount(0)
  await page.getByRole('button', { name: 'Go to sign in' }).click()
  await expect(page).toHaveURL(/\/login$/)
})

test('the sign-in page says there is no administrator only while that is true', async ({ page }) => {
  await installFixtures(page, { claimable: true })
  await page.goto('/login')
  await expect(page.locator('[data-auth-claim-hint]')).toContainText('sudo anix-control admin claim-link')

  const other = await page.context().newPage()
  await installFixtures(other, { claimable: false })
  await other.goto('/login')
  await expect(other.getByRole('heading', { level: 1, name: 'Sign in to AnixOps Control' })).toBeVisible()
  await expect(other.locator('[data-auth-claim-hint]')).toHaveCount(0)
})

test('opens with a session too, so a signed-in visitor is not sent away with the link', async ({ page }) => {
  await installFixtures(page)
  await page.addInitScript(() => {
    localStorage.setItem('token', 'member-token')
    localStorage.setItem('userInfo', JSON.stringify({ id: 7, email: 'member@example.com', is_admin: false }))
  })
  await page.goto(`/claim#${TOKEN}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Create the first administrator' })).toBeVisible()
  await expect(page).toHaveURL(/\/claim$/)
})
