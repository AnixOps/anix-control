import { createHash } from 'node:crypto'
import { test, expect } from '@playwright/test'

const dashboardStats = {
  total_users: 12,
  today_new_users: 2,
  active_users: 9,
  expired_users: 1,
  banned_users: 0,
  total_nodes: 3,
  active_nodes: 2,
  online_users: 7,
  monthly_income: 1234,
  today_income: 100,
  total_revenue: 5000,
  total_orders: 8,
  pending_orders: 1,
  paid_orders: 7,
  total_traffic_used: 1024,
  today_traffic: 128,
}

function sha256(source) {
  return createHash('sha256').update(source).digest('hex')
}

function extensionSource(pluginID, version, label) {
  return `
export const anixopsExtension = Object.freeze({
  pluginId: '${pluginID}',
  version: '${version}',
  webuiApiVersion: 'anixops.webui/v1',
  bundle: { path: 'webui/index.mjs' },
})

export default function createExtension(host) {
  return host.defineComponent({
    name: '${pluginID.replace(/[^A-Za-z0-9]/g, '')}Extension',
    setup() {
      return () => host.h('section', { class: 'page-shell', 'data-testid': 'signed-extension' }, [
        host.h('h1', '${label}'),
        host.h('p', { 'data-testid': 'bundle-version' }, '${pluginID}@${version}'),
      ])
    },
  })
}
`
}

function extensionEntry({
  pluginID,
  version = '1.0.0',
  source,
  state = 'healthy',
  route = `/admin/extensions/${pluginID}`,
  declaredSHA256,
  installationID = 1,
  permission = `${pluginID}.view`,
  label = pluginID,
}) {
  const actualSHA256 = sha256(source)
  const bundleSHA256 = declaredSHA256 || actualSHA256
  return {
    plugin_id: pluginID,
    plugin_name: label,
    publisher: 'AnixOps',
    version,
    api_version: 'v1',
    installation_id: installationID,
    state,
    bundle: {
      path: 'webui/index.mjs',
      sha256: bundleSHA256,
      url: `/api/v3/extensions/${pluginID}/${version}/webui/${bundleSHA256}/index.mjs`,
    },
    permissions: [permission],
    menus: [{
      id: `${pluginID}.main`,
      parent: 'services',
      label,
      icon: 'box',
      route,
      permission,
      order: 10,
    }],
    routes: [{
      id: `${pluginID}.main`,
      path: route,
      export: 'default',
      permission,
    }],
  }
}

async function seedAdmin(page, permissions = []) {
  await page.addInitScript(({ permissions: granted }) => {
    localStorage.setItem('token', 'browser-e2e-token')
    localStorage.setItem('app.locale', 'en')
    localStorage.setItem('userInfo', JSON.stringify({
      id: 1,
      email: 'browser-e2e@example.test',
      is_admin: true,
      permissions: granted,
    }))
  }, { permissions })
}

async function installAPIFixtures(page, catalog, bundleSources) {
  const assetRequests = []

  // The Control kernel only exposes releases after signature verification. The
  // browser gate exercises the remaining boundary: same-origin asset identity
  // and SHA-256 verification before dynamic import.
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const requestURL = new URL(route.request().url())

    if (requestURL.pathname === '/api/v3/extensions') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(catalog),
      })
      return
    }

    if (requestURL.pathname.startsWith('/api/v3/extensions/')) {
      assetRequests.push(requestURL.pathname)
      const pluginID = requestURL.pathname.split('/')[4]
      const source = bundleSources[pluginID]
      if (!source) {
        await route.fulfill({ status: 404, body: 'missing fixture bundle' })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'text/javascript',
        body: source,
      })
      return
    }

    if (requestURL.pathname === '/api/v2/admin/dashboard') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: dashboardStats }),
      })
      return
    }

    if (requestURL.pathname === '/api/v2/admin/system/info') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: { version: 'e2e', build_code: 'browser' } }),
      })
      return
    }

    if (requestURL.pathname === '/api/v2/user/profile') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: { id: 1, email: 'browser-e2e@example.test', is_admin: true } }),
      })
      return
    }

    if (requestURL.pathname.startsWith('/api/v3/')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      })
      return
    }

    await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: 'unhandled fixture request' }) })
  })

  return assetRequests
}

test('installs a signed WebUI bundle and isolates invalid plugin routes from core UI', async ({ page }) => {
  const pluginID = 'browser-fixture'
  const source = extensionSource(pluginID, '1.0.0', 'Signed Browser Extension')
  const valid = extensionEntry({
    pluginID,
    source,
    label: 'Signed Browser Extension',
  })
  const collisionSource = extensionSource('route-collision', '1.0.0', 'Should Never Render')
  const invalidCollision = extensionEntry({
    pluginID: 'route-collision',
    source: collisionSource,
    route: '/admin/dashboard',
    label: 'Should Never Render',
    installationID: 2,
  })

  const assetRequests = await installAPIFixtures(page, [valid, invalidCollision], {
    [pluginID]: source,
    'route-collision': collisionSource,
  })
  await seedAdmin(page, ['browser-fixture.view'])

  await page.goto('/admin/dashboard')
  await expect(page.locator('.page-toolbar h1')).toHaveText('Dashboard')
  await expect(page.locator('a[href="/admin/extensions/browser-fixture"]')).toBeVisible()
  await expect(page.locator('a[href="/admin/extensions/route-collision"]')).toHaveCount(0)

  await page.locator('a[href="/admin/extensions/browser-fixture"]').click()
  await expect(page).toHaveURL(/\/admin\/extensions\/browser-fixture$/)
  await expect(page.getByTestId('signed-extension')).toBeVisible()
  await expect(page.getByTestId('signed-extension').locator('h1')).toHaveText('Signed Browser Extension')
  await expect(page.getByTestId('bundle-version')).toHaveText('browser-fixture@1.0.0')

  expect(assetRequests).toEqual([
    `/api/v3/extensions/browser-fixture/1.0.0/webui/${valid.bundle.sha256}/index.mjs`,
  ])
})

test('disabled and tampered plugins fail closed without contaminating core admin UI', async ({ page }) => {
  const disabledID = 'disabled-fixture'
  const disabledSource = extensionSource(disabledID, '1.0.0', 'Disabled Extension')
  const tamperedID = 'tampered-fixture'
  const tamperedSource = extensionSource(tamperedID, '1.0.0', 'Tampered Extension')
  const tampered = extensionEntry({
    pluginID: tamperedID,
    source: tamperedSource,
    declaredSHA256: 'd'.repeat(64),
    label: 'Tampered Extension',
  })
  const disabled = extensionEntry({
    pluginID: disabledID,
    source: disabledSource,
    state: 'disabled',
    label: 'Disabled Extension',
    installationID: 3,
  })

  const assetRequests = await installAPIFixtures(page, [disabled, tampered], {
    [disabledID]: disabledSource,
    [tamperedID]: tamperedSource,
  })
  await seedAdmin(page, [])

  await page.goto('/admin/dashboard')
  await expect(page.locator('.page-toolbar h1')).toHaveText('Dashboard')
  await expect(page.locator('a[href="/admin/extensions/disabled-fixture"]')).toHaveCount(0)
  await expect(page.locator('a[href="/admin/extensions/tampered-fixture"]')).toHaveCount(0)
  await expect(page.locator('a[href="/admin/dashboard"]')).toBeVisible()

  expect(assetRequests).toEqual([
    `/api/v3/extensions/tampered-fixture/1.0.0/webui/${tampered.bundle.sha256}/index.mjs`,
  ])

  await page.goto('/admin/extensions/tampered-fixture')
  await expect(page).toHaveURL(/\/admin\/control$/)
  await expect(page.locator('.page-header h1')).toHaveText('Control Kernel')
  await expect(page.locator('a[href="/admin/dashboard"]')).toBeVisible()
})
