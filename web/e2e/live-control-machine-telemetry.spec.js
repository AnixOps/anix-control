import { test, expect } from '@playwright/test'

// The real Control carries three signed built-in packages, installed by its
// start-up bootstrap: identity-platform, machine-telemetry and forward.
const extensionID = 'forward'

async function adminGet(page, url) {
  return page.evaluate(async (target) => {
    const token = localStorage.getItem('token')
    const response = await fetch(target, { headers: { Authorization: `Bearer ${token}` } })
    return { status: response.status, body: await response.json().catch(() => null) }
  }, url)
}

test('opens the forwarding pages through the forward.v4 capability of real Control', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))

  // The guard reads the capabilities before it opens a forwarding page.
  const capabilitiesResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v3/capabilities' && response.request().method() === 'GET' && response.status() === 200
  })

  await page.goto('/login')
  await page.locator('#email').fill('live-control-webui@anixops.test')
  await page.locator('#password').fill('LiveControlWebUI!2026')
  await page.locator('form[data-auth-form="password"] button[type="submit"]').click()
  await page.waitForURL(/\/admin\/dashboard$/)

  const installed = await adminGet(page, '/api/v3/plugin-installations')
  for (const pluginID of ['identity-platform', 'machine-telemetry', extensionID]) {
    expect(installed.body.data).toEqual(expect.arrayContaining([
      expect.objectContaining({ plugin_id: pluginID, target: 'control', observed_version: '4.0.0', state: 'healthy', enabled: true }),
    ]))
  }

  // The healthy forward package declares its v4 API, so Control answers
  // forward.v4, and the forwarding pages open.
  expect(await (await capabilitiesResponse).json()).toEqual({ data: ['forward.v4'] })
  await page.goto('/admin/forward/overview')
  await expect(page).toHaveURL(/\/admin\/forward\/overview$/)

  // The signed WebUI extension catalog and its asset route are gone.
  const removed = await adminGet(page, `/api/v3/extensions`)
  expect(removed.status).toBe(404)
  await expect(page.locator(`a[href="/admin/extensions/${extensionID}"]`)).toHaveCount(0)
})

test('shows the built-in packages of the real Control process', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))

  await page.goto('/login')
  await page.locator('#email').fill('live-control-webui@anixops.test')
  await page.locator('#password').fill('LiveControlWebUI!2026')
  await page.locator('form[data-auth-form="password"] button[type="submit"]').click()
  await page.waitForURL(/\/admin\/dashboard$/)

  // The old plugin center leads to the built-in packages.
  await page.goto('/admin/plugins')
  await expect(page).toHaveURL(/\/admin\/system\/packages$/)
  for (const pluginID of ['identity-platform', 'machine-telemetry', extensionID]) {
    const row = page.getByTestId(`package-row-${pluginID}`)
    await expect(row).toBeVisible()
    await expect(row).toContainText('Version 4.0.0')
    await expect(row).toContainText('Running')
  }
  for (const action of ['install', 'upgrade', 'rollback', 'enable', 'disable']) {
    await expect(page.locator(`[data-action="${action}"]`)).toHaveCount(0)
  }

  // The removed import, install and route-mode routes answer 404.
  const statuses = await page.evaluate(async () => {
    const token = localStorage.getItem('token')
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
    const probes = [
      ['POST', '/api/v3/plugin-releases'],
      ['PUT', '/api/v3/plugin-installations'],
      ['GET', '/api/v4/kernel/route-modes'],
    ]
    const answers = []
    for (const [method, url] of probes) {
      const response = await fetch(url, { method, headers, body: method === 'GET' ? undefined : '{}' })
      answers.push(response.status)
    }
    return answers
  })
  expect(statuses).toEqual([404, 404, 404])
})
