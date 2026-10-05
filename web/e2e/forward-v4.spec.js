import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'
import { expectToast } from './support/toasts.js'
import { chooseNode } from './fixtures/forwardV4.js'

// The v4.2 forwarding pages (F5b) against the mocked /api/v4/forward
// (e2e/fixtures/forwardV4.js): create → preview → save → detail →
// diagnose, the node settings, and the capability gate.

function recordWrites(page) {
  const writes = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/v4/forward/') || request.method() === 'GET') return
    let body = null
    try { body = request.postDataJSON() } catch { body = null }
    writes.push({ method: request.method(), path: url.pathname, body, key: request.headers()['idempotency-key'] || '' })
  })
  return writes
}

test('creates a route: preview, save, detail and diagnosis', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-editor-blank', { clock: true })

  await page.getByLabel('Name').first().fill('e2e-edge')
  await chooseNode(page, 0, 'hk-edge-01')
  await page.getByLabel('Target 1 host').fill('origin.example.com')
  await page.getByLabel('Target 1 port').fill('443')
  await page.getByLabel('Target 1 port').blur()

  // The preview runs 1 s after typing stops and plans the entry node.
  const preview = page.getByTestId('forward-preview')
  await expect(preview.getByText('1 nodes +1')).toBeVisible({ timeout: 10_000 })
  await expect(preview.getByText('hk-edge-01')).toBeVisible()
  const previews = writes.filter(write => write.path.endsWith('/routes/preview'))
  expect(previews.length).toBeGreaterThan(0)
  const planned = previews.at(-1).body.route
  // Only contract fields, enum names, unset fields left out.
  expect(planned).toEqual({
    name: 'e2e-edge',
    listen: { protocol: 'L4_PROTOCOL_TCP' },
    hops: [{ role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_GOST', node_refs: ['forward-41'] }],
    targets: [{ host: 'origin.example.com', port: 443 }],
    policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_ROUND_ROBIN', direct: 'DIRECT_MODE_OFF', target_policy: 'TARGET_POLICY_PUBLIC_ONLY' }
  })

  await page.getByTestId('forward-save').click()
  await expect(page).toHaveURL(/\/admin\/forward\/routes\/01JBNEWROUTE00000000000000$/)
  const create = writes.find(write => write.method === 'POST' && write.path === '/api/v4/forward/routes')
  expect(create.key).toMatch(/^fwd-/)
  expect(create.body).toEqual(planned)
  await expect(page.getByRole('heading', { level: 1, name: 'e2e-edge' })).toBeVisible()

  await page.getByTestId('forward-diagnose').click()
  const diagnosis = page.getByTestId('forward-diagnosis')
  await expect(diagnosis.getByText('Some steps failed')).toBeVisible()
  await expect(diagnosis.getByRole('heading', { name: /Control’s records/ })).toBeVisible()
  await expect(diagnosis.getByRole('heading', { name: /Node probes/ })).toBeVisible()
  await expect(diagnosis.getByRole('heading', { name: /Control probes/ })).toBeVisible()
  await expect(diagnosis.getByText('unreachable', { exact: true })).toBeVisible()
  await expect(diagnosis.getByText('Inconclusive').first()).toBeVisible()
  await expect(diagnosis.getByText('Skipped').first()).toBeVisible()
  const diagnose = writes.find(write => write.path.endsWith('/diagnose'))
  expect(diagnose.key).toMatch(/^fwd-/)
})

test('maps the preview violations to their fields', async ({ page }) => {
  await openScreen(page, 'admin-forward-editor', { clock: true })
  const summary = page.getByTestId('forward-violations')
  await expect(summary.getByText('3 problems to fix before saving')).toBeVisible()
  await expect(page.getByTestId('forward-save')).toBeDisabled()
  await summary.locator('[data-violation-field="targets[0].host"]').click()
  await expect(page.getByLabel('Target 1 host')).toBeFocused()
  // The link fix: insert a gost relay before the TLS exit.
  await expect(page.getByRole('button', { name: 'Insert a gost relay before' })).toBeVisible()
})

test('saves a node’s forwarding settings', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-node', { clock: true })
  const form = page.getByTestId('forward-node-settings')
  await form.getByLabel('Reserved ports').fill('22, 80, 443, 8443')
  await form.getByLabel('Addresses').fill('198.51.100.51\n10.0.0.51')
  await page.getByTestId('forward-node-settings-save').click()
  await expectToast(page, 'Forwarding settings saved; every route was replanned.')
  const save = writes.find(write => write.method === 'PUT')
  expect(save.path).toBe('/api/v4/forward/nodes/forward-51/settings')
  expect(save.key).toMatch(/^fwd-/)
  expect(save.body).toEqual({
    port_range: { first: 30000, last: 39999 },
    reserved_ports: [22, 80, 443, 8443],
    addresses: ['198.51.100.51', '10.0.0.51'],
    labels: { region: 'sg' }
  })
})

test('the forwarding area needs the forward package’s v4 API', async ({ page }) => {
  await openScreen(page, 'admin-forward-no-capability', { clock: true })
  await expect(page).toHaveURL(/\/admin\/plugins$/)
  await expect(page.locator('#admin-sidebar a[href="/admin/forward/overview"]')).toHaveCount(0)
})
