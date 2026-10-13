import { test, expect } from '@playwright/test'

// The real Control carries three signed built-in packages, installed by its
// start-up bootstrap: identity-platform and machine-telemetry ship no WebUI,
// nftables-forward ships one.
const extensionID = 'nftables-forward'
const extensionPath = `/admin/extensions/${extensionID}`

async function installations(page) {
  return page.evaluate(async () => {
    const token = localStorage.getItem('token')
    const response = await fetch('/api/v3/plugin-installations', { headers: { Authorization: `Bearer ${token}` } })
    const payload = await response.json()
    return Array.isArray(payload?.data) ? payload.data : []
  })
}

test('lists only packages with a WebUI, then loads and serves a signed WebUI through real Control', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))

  // Extension discovery validates and preloads the signed bundle before the
  // menu is clicked. Start observing before login so this stays a real fetch
  // assertion instead of waiting for a second, cache-dependent request.
  const bundleResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname.startsWith(`/api/v3/extensions/${extensionID}/4.0.0/webui/`) &&
      response.request().method() === 'GET' && response.status() === 200
  })
  const catalogResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v3/extensions' && response.request().method() === 'GET' && response.status() === 200
  })

  await page.goto('/login')
  await page.locator('#email').fill('live-control-webui@anixops.test')
  await page.locator('#password').fill('LiveControlWebUI!2026')
  await page.locator('form[data-auth-form="password"] button[type="submit"]').click()
  await page.waitForURL(/\/admin\/dashboard$/)

  // The packages without a WebUI are installed and healthy, but the
  // catalog leaves them out: only nftables-forward has an extension.
  const installed = await installations(page)
  for (const pluginID of ['identity-platform', 'machine-telemetry', extensionID]) {
    expect(installed).toEqual(expect.arrayContaining([
      expect.objectContaining({ plugin_id: pluginID, target: 'control', observed_version: '4.0.0', state: 'healthy', enabled: true }),
    ]))
  }
  const extensions = await (await catalogResponse).json()
  expect(extensions).toEqual(expect.objectContaining({ data: expect.any(Array) }))
  expect(extensions.data.map(item => item.plugin_id)).toEqual([extensionID])
  const extension = extensions.data[0]
  expect(extension).toEqual(expect.objectContaining({
    plugin_id: extensionID,
    version: '4.0.0',
    state: 'healthy',
    installation_id: expect.any(Number),
    bundle: expect.objectContaining({
      path: 'webui/index.mjs',
      sha256: expect.stringMatching(/^[a-f0-9]{64}$/),
      url: expect.stringMatching(new RegExp(`^/api/v3/extensions/${extensionID}/4\\.0\\.0/webui/`)),
    }),
  }))
  expect(extension.menus).toEqual([expect.objectContaining({ route: extensionPath, label: 'nftables Forward' })])
  expect(extension.routes).toEqual([expect.objectContaining({ path: extensionPath, permission: `${extensionID}.view` })])
  await expect(page.locator('a[href="/admin/extensions/machine-telemetry"]')).toHaveCount(0)
  await expect(page.locator('a[href="/admin/extensions/identity-platform"]')).toHaveCount(0)

  const menu = page.locator(`a[href="${extensionPath}"]`)
  await expect(menu).toBeVisible()
  await menu.click()
  await expect(page).toHaveURL(new RegExp(`${extensionPath}$`))

  const bundle = await bundleResponse
  expect(new URL(bundle.url()).pathname).toBe(extension.bundle.url)
  expect(bundle.headers()['cache-control']).toBe('private, no-store')
  await expect(page.locator('.nftables-forward-extension h1')).toHaveText('nftables Forward')
  expect(await bundle.text()).toContain('NftablesForwardExtension')
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
