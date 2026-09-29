import { test, expect } from '@playwright/test'

const plugin = {
  id: 'machine-telemetry',
  name: 'Machine Telemetry',
  description: 'Signed host telemetry package',
  publisher: 'AnixOps',
}

const release = {
  id: 4,
  plugin_id: plugin.id,
  version: '4.0.0',
  manifest: JSON.stringify({ id: plugin.id, version: '4.0.0', targets: ['control'] }),
}

async function seedAdmin(page) {
  await page.addInitScript(() => {
    localStorage.setItem('token', 'plugin-center-e2e-token')
    localStorage.setItem('app.locale', 'en')
    localStorage.setItem('userInfo', JSON.stringify({
      id: 1,
      email: 'plugin-center@example.test',
      is_admin: true,
    }))
  })
}

async function installFixtures(page) {
  let operations = [
    {
      id: 'plugin-op-1',
      kind: 'plugin.enable',
      plugin_id: plugin.id,
      state: 'running',
      target: 'control',
      target_version: '4.0.0',
      operation_chain: 'dependency-op-1,plugin-op-1',
      created_at: '2026-09-28T12:00:00Z',
    },
    {
      id: 'deployment-op-1',
      kind: 'deployment.apply',
      state: 'running',
      created_at: '2026-09-28T11:00:00Z',
    },
  ]

  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const request = route.request()
    const requestURL = new URL(request.url())

    if (requestURL.pathname === '/api/v2/user/profile') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 1, email: 'plugin-center@example.test', is_admin: true } }) })
      return
    }
    if (requestURL.pathname === '/api/v2/admin/dashboard') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { total_users: 1, total_nodes: 1 } }) })
      return
    }
    if (requestURL.pathname === '/api/v2/admin/system/info') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { version: 'e2e' } }) })
      return
    }
    if (requestURL.pathname === '/api/v3/plugins') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([plugin]) })
      return
    }
    if (requestURL.pathname === '/api/v3/plugin-releases') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([release]) })
      return
    }
    if (requestURL.pathname === '/api/v3/plugin-installations') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
      return
    }
    if (requestURL.pathname === '/api/v3/extensions') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
      return
    }
    if (requestURL.pathname === '/api/v3/operations' && request.method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(operations) })
      return
    }
    if (requestURL.pathname === '/api/v3/operations/plugin-op-1/cancel' && request.method() === 'POST') {
      operations = operations.map(operation => operation.id === 'plugin-op-1'
        ? { ...operation, state: 'cancel_requested' }
        : operation)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: {
          'X-AnixOps-Operation-ID': 'plugin-op-1',
          'X-AnixOps-Operation-Chain': 'dependency-op-1,plugin-op-1',
        },
        body: JSON.stringify(operations[0]),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
  })
}

test('shows plugin operation history and cancels an active Control operation', async ({ page }) => {
  await seedAdmin(page)
  await installFixtures(page)
  await page.goto('/admin/plugins')

  const history = page.getByTestId('plugin-operation-history')
  await expect(history).toBeVisible()
  await expect(history).toContainText('Recent plugin operations')
  await expect(history.getByTestId('operation-row-plugin-op-1')).toContainText('plugin.enable')
  await expect(history.getByTestId('operation-row-plugin-op-1')).toContainText('control')
  await expect(history.getByTestId('operation-row-plugin-op-1')).toContainText('4.0.0')
  await expect(history.getByTestId('operation-chain-plugin-op-1')).toContainText('dependency-op-1,plugin-op-1')
  await expect(history.getByTestId('operation-row-deployment-op-1')).toHaveCount(0)

  await history.getByTestId('cancel-operation-plugin-op-1').click()
  await expect(page.getByText('Operation cancellation was requested', { exact: true })).toBeVisible()
  await expect(history.getByTestId('operation-row-plugin-op-1')).toContainText('cancel_requested')
})
