import { test, expect } from '@playwright/test'

const extensionPath = '/admin/extensions/machine-telemetry'

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

test('loads, serves, and revokes the official signed machine-telemetry WebUI through real Control', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('app.locale', 'en'))

  // Extension discovery validates and preloads the signed bundle before the
  // menu is clicked. Start observing before login so this stays a real fetch
  // assertion instead of waiting for a second, cache-dependent request.
  const bundleResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname.startsWith('/api/v3/extensions/machine-telemetry/1.1.0/webui/') &&
      response.request().method() === 'GET' && response.status() === 200
  })
  const catalogResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v3/extensions' && response.request().method() === 'GET' && response.status() === 200
  })

  await page.goto('/login')
  await page.locator('#email').fill('live-control-webui@anixops.test')
  await page.locator('#password').fill('LiveControlWebUI!2026')
  await page.locator('form.login-form button[type="submit"]').click()
  await page.waitForURL(/\/admin\/dashboard$/)

  const extensions = await (await catalogResponse).json()
  expect(extensions).toEqual(expect.objectContaining({ data: expect.any(Array) }))
  expect(extensions.data).toHaveLength(1)
  const [extension] = extensions.data
  expect(extension).toEqual(expect.objectContaining({
    plugin_id: 'machine-telemetry',
    version: '1.1.0',
    state: 'healthy',
    installation_id: expect.any(Number),
    bundle: expect.objectContaining({
      path: 'webui/index.mjs',
      sha256: expect.stringMatching(/^[a-f0-9]{64}$/),
      url: expect.stringMatching(/^\/api\/v3\/extensions\/machine-telemetry\/1\.1\.0\/webui\//),
    }),
  }))
  expect(extension.menus).toEqual([expect.objectContaining({ route: extensionPath, label: 'Machine Telemetry' })])
  expect(extension.routes).toEqual([expect.objectContaining({ path: extensionPath, permission: 'machine-telemetry.view' })])

  const menu = page.locator(`a[href="${extensionPath}"]`)
  await expect(menu).toBeVisible()
  await menu.click()
  await expect(page).toHaveURL(new RegExp(`${extensionPath}$`))

  const bundle = await bundleResponse
  expect(new URL(bundle.url()).pathname).toBe(extension.bundle.url)
  expect(bundle.headers()['cache-control']).toBe('private, no-store')
  await expect(page.locator('.machine-telemetry-extension h1')).toHaveText('Machine Telemetry')
  expect(await bundle.text()).toContain('MachineTelemetryExtension')

  const disable = await page.evaluate(async installationID => {
    const token = localStorage.getItem('token')
    const response = await fetch('/api/v3/plugin-installations', {
      method: 'PUT',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        plugin_id: 'machine-telemetry',
        target: 'control',
        desired_version: '1.1.0',
        enabled: false,
      }),
    })
    return {
      status: response.status,
      operationID: response.headers.get('X-AnixOps-Operation-ID'),
      body: await response.json(),
    }
  }, extension.installation_id)
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
