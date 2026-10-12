import { test, expect } from '@playwright/test'

// The real Control runs three signed packages: identity-platform (the
// bootstrap) and machine-telemetry ship no WebUI, nftables-forward ships one.
const extensionID = 'nftables-forward'
const extensionPath = `/admin/extensions/${extensionID}`

async function operationState(page, operationID) {
  return page.evaluate(async id => {
    const token = localStorage.getItem('token')
    const response = await fetch('/api/v3/operations', {
      headers: { Authorization: `Bearer ${token}` },
    })
    const payload = await response.json()
    return Array.isArray(payload?.data) ? payload.data.find(operation => operation.id === id)?.state ?? '' : ''
  }, operationID)
}

async function installations(page) {
  return page.evaluate(async () => {
    const token = localStorage.getItem('token')
    const response = await fetch('/api/v3/plugin-installations', { headers: { Authorization: `Bearer ${token}` } })
    const payload = await response.json()
    return Array.isArray(payload?.data) ? payload.data : []
  })
}

test('lists only packages with a WebUI, then loads, serves, and revokes a signed WebUI through real Control', async ({ page }) => {
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

  const disable = await page.evaluate(async () => {
    const token = localStorage.getItem('token')
    const response = await fetch('/api/v3/plugin-installations', {
      method: 'PUT',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        plugin_id: 'nftables-forward',
        target: 'control',
        desired_version: '4.0.0',
        enabled: false,
      }),
    })
    return {
      status: response.status,
      operationID: response.headers.get('X-AnixOps-Operation-ID'),
      body: await response.json(),
    }
  })
  expect(disable.status).toBe(200)
  expect(disable.operationID).toMatch(/^[0-9a-f-]{36}$/i)
  expect(disable.body.data).toEqual(expect.objectContaining({ enabled: false, state: 'disabled' }))
  // The Control worker uses a durable 30-second lease. Give an in-flight
  // SQLite writer one complete lease/retry window rather than turning a slow
  // terminal write into a false browser failure, while still requiring the
  // externally visible operation to succeed before checking revocation.
  await expect.poll(() => operationState(page, disable.operationID), {
    timeout: 45_000,
    intervals: [500, 1_000, 2_000],
  }).toBe('succeeded')

  const revocation = await page.evaluate(async bundleURL => {
    const token = localStorage.getItem('token')
    const catalog = await fetch('/api/v3/extensions', { headers: { Authorization: `Bearer ${token}` } })
    const asset = await fetch(bundleURL, { headers: { Authorization: `Bearer ${token}` } })
    return {
      catalogStatus: catalog.status,
      catalog: await catalog.json(),
      assetStatus: asset.status,
    }
  }, extension.bundle.url)
  expect(revocation.catalogStatus).toBe(200)
  expect(revocation.catalog.data).toEqual([])
  expect(revocation.assetStatus).toBe(404)

  // A new SPA runtime must not retain a menu or dynamically registered route
  // after the actual Control installation is disabled.
  await page.reload()
  await expect(page).toHaveURL(/\/admin\/plugins$/)
  await expect(page.locator(`a[href="${extensionPath}"]`)).toHaveCount(0)
})

test('loads the merged native plugin center against the real Control process', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))

  await page.goto('/login')
  await page.locator('#email').fill('live-control-webui@anixops.test')
  await page.locator('#password').fill('LiveControlWebUI!2026')
  await page.locator('form[data-auth-form="password"] button[type="submit"]').click()
  await page.waitForURL(/\/admin\/dashboard$/)

  await page.goto('/admin/plugins')
  await expect(page.getByTestId('plugin-row-machine-telemetry')).toBeVisible()
  await page.getByTestId('plugin-row-machine-telemetry').click()
  await expect(page.getByTestId('plugin-detail-drawer')).toContainText('Machine Telemetry')

  const history = page.getByTestId('plugin-operation-history')
  await expect(history).toBeVisible()
  await expect(history).toContainText('Recent plugin operations')
  await expect(page.getByTestId('plugin-detail-drawer')).toContainText('4.0.0')
})
