import { test, expect } from '@playwright/test'

const node = { id: 11, name: 'Shanghai entry', host: '10.0.0.11', status: 1 }
const plugin = { id: 'gost-mesh', name: 'GOST Mesh', publisher: 'AnixOps' }
const manifest = JSON.stringify({ id: 'gost-mesh', version: '1.0.0', targets: ['agent'] })

async function seedAdmin(page) {
  await page.addInitScript(() => {
    localStorage.setItem('token', 'browser-assignment-token')
    localStorage.setItem('app.locale', 'en')
    localStorage.setItem('userInfo', JSON.stringify({ id: 1, email: 'assignment@example.test', is_admin: true }))
  })
}

async function installFixtures(page) {
  const state = {
    assignments: [],
    operations: []
  }

  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const request = route.request()
    const url = new URL(request.url())
    let body = {}
    try {
      body = request.postDataJSON() || {}
    } catch {
      // GET/DELETE requests do not carry JSON bodies.
    }

    if (url.pathname === '/api/v2/user/profile') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 1, email: 'assignment@example.test', is_admin: true } }) })
      return
    }
    if (url.pathname === '/api/v2/admin/dashboard') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { total_nodes: 1, active_nodes: 1 } }) })
      return
    }
    if (url.pathname === '/api/v2/admin/system/info') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { version: 'e2e' } }) })
      return
    }
    if (url.pathname === '/api/v2/admin/nodes') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, msg: 'ok', data: { list: [node], total: 1 } }) })
      return
    }
    if (url.pathname === '/api/v3/plugins') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([plugin]) })
      return
    }
    if (url.pathname === '/api/v3/plugin-releases') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([{ id: 3, plugin_id: plugin.id, version: '1.0.0', manifest }]) })
      return
    }
    if (url.pathname === '/api/v3/plugin-installations') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([{ id: 5, plugin_id: plugin.id, target: 'agent', desired_version: '1.0.0', observed_version: '1.0.0', config_revision: 6, state: 'healthy', enabled: true }]) })
      return
    }
    if (url.pathname === '/api/v3/service-scopes') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([{ id: 'forward', name: 'Forward', plugin_id: plugin.id }]) })
      return
    }
    if (url.pathname === '/api/v3/topologies' || url.pathname === '/api/v3/operations' || url.pathname === '/api/v3/extensions') {
      const payload = url.pathname.endsWith('/operations') ? state.operations : []
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(payload) })
      return
    }
    if (url.pathname === `/api/v3/nodes/${node.id}/assignments` && request.method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(state.assignments) })
      return
    }
    if (url.pathname === `/api/v3/nodes/${node.id}/assignments` && request.method() === 'PUT') {
      const identity = state.assignments.find(item => item.service_scope === body.service_scope && item.plugin_id === body.plugin_id && item.role === body.role)
      if (identity) Object.assign(identity, body)
      else state.assignments.push({ id: 7, node_id: node.id, ...body })
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(state.assignments.at(-1)) })
      return
    }
    if (url.pathname.startsWith(`/api/v3/nodes/${node.id}/assignments/`) && request.method() === 'DELETE') {
      const assignmentID = Number(url.pathname.split('/').at(-1))
      state.assignments = state.assignments.filter(item => item.id !== assignmentID)
      await route.fulfill({ status: 204, body: '' })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
  })
}

test('runs the assignment lifecycle in a narrow viewport without page overflow', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await seedAdmin(page)
  await installFixtures(page)
  await page.goto('/admin/deployments')
  await page.locator('[data-testid="deployment-targets"]').click()
  await expect(page.locator('#assignment-node-filter')).toHaveValue('11')
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true)

  await page.locator('[data-testid="new-assignment"]').click()
  const drawer = page.locator('[data-testid="assignment-drawer"]')
  await expect(drawer).toBeVisible()
  const drawerBox = await drawer.boundingBox()
  expect(drawerBox.width).toBeLessThanOrEqual(390)
  await drawer.locator('#assignment-rollout-group').fill('canary-mobile')
  await drawer.locator('[data-testid="save-assignment"]').click()
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toContainText('canary-mobile')

  await page.locator('[data-testid="edit-assignment-7"]').click()
  await expect(drawer.locator('#assignment-role')).toBeDisabled()
  await drawer.locator('#assignment-config-revision').fill('8')
  await drawer.locator('[data-testid="save-assignment"]').click()
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toContainText('8')

  await page.locator('[data-testid="toggle-assignment-7"]').click()
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toContainText('Disabled')
  await page.locator('[data-testid="toggle-assignment-7"]').click()
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toContainText('Enabled')
  await page.locator('[data-testid="delete-assignment-7"]').click()
  await expect(page.locator('[data-testid="deployment-target-panel"]')).toContainText('No assignments for this node')
})
