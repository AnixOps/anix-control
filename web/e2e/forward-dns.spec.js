import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'
import { EDGE, chooseNode } from './fixtures/forwardV4.js'

// Entry HA through DNS (L2) against the mocked /api/v4/forward
// (e2e/fixtures/forwardV4.js): the providers page with write-only
// credentials, the route editor's binding picker and the route page's
// entry HA card.

function recordWrites(page) {
  const writes = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/v4/forward/') || request.method() === 'GET') return
    let body = null
    try { body = request.postDataJSON() } catch { body = null }
    writes.push({ method: request.method(), path: url.pathname, query: Object.fromEntries(url.searchParams), body, key: request.headers()['idempotency-key'] || '' })
  })
  return writes
}

test('adds a provider from the kind schema and keeps stored credentials', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-dns', { clock: true })
  await expect(page.getByRole('heading', { level: 1, name: 'DNS providers' })).toBeVisible()
  await expect(page.getByText('cloudflare-main')).toBeVisible()
  await expect(page.locator('body')).not.toContainText('********')

  await page.getByTestId('forward-dns-add').click()
  const sheet = page.getByRole('dialog', { name: 'Add a DNS provider' })
  await sheet.getByLabel('Name').fill('hook-b')
  await sheet.getByRole('radio', { name: /Webhook/ }).click()
  await sheet.getByLabel('Webhook URL').fill('https://hook.example.com/b')
  await sheet.getByLabel('Signing secret').fill('s3cret')
  await page.getByTestId('forward-dns-provider-save').click()
  await expect(page.getByText('Added hook-b').first()).toBeVisible()
  const create = writes.find(write => write.method === 'POST')
  expect(create.path).toBe('/api/v4/forward/dns/providers')
  expect(create.key).toMatch(/^fwd-/)
  expect(create.body).toEqual({
    provider: { name: 'hook-b', kind: 'DNS_PROVIDER_KIND_WEBHOOK', config: { url: 'https://hook.example.com/b' } },
    credentials: { secret: 's3cret' }
  })

  // Edit: the stored token shows as ******** and is not sent back.
  await page.getByRole('row', { name: /cloudflare-main/ }).getByRole('button', { name: /Actions|More/ }).click()
  await page.getByRole('menuitem', { name: 'Edit…' }).click()
  const edit = page.getByRole('dialog', { name: 'Edit cloudflare-main' })
  await expect(edit.getByLabel('API token')).toHaveValue('********')
  await edit.getByLabel('API endpoint').fill('api.cloudflare.com')
  await page.getByTestId('forward-dns-provider-save').click()
  await expect(page.getByText('Saved cloudflare-main').first()).toBeVisible()
  const update = writes.find(write => write.method === 'PUT')
  expect(update.path).toBe('/api/v4/forward/dns/providers/1')
  expect(update.body).toEqual({ provider: { name: 'cloudflare-main', kind: 'DNS_PROVIDER_KIND_CLOUDFLARE', config: { endpoint: 'api.cloudflare.com' } }, credentials: {} })
})

test('a provider write by an administrator who is not a super administrator', async ({ page }) => {
  await openScreen(page, 'admin-forward-dns', { clock: true })
  await page.route('**/api/v4/forward/dns/providers', route => (route.request().method() === 'POST'
    ? route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: { code: 'super_admin_required', message: 'super administrator required' } }) })
    : route.fallback()))
  await page.getByTestId('forward-dns-add').click()
  const sheet = page.getByRole('dialog', { name: 'Add a DNS provider' })
  await sheet.getByLabel('Name').fill('cf-2')
  await sheet.getByLabel('API token').fill('token')
  await page.getByTestId('forward-dns-provider-save').click()
  await expect(page.getByTestId('forward-dns-forbidden')).toHaveText('Only a super administrator may add, change or delete DNS providers.')
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('forward-dns-readonly')).toBeVisible()
  await expect(page.getByTestId('forward-dns-add')).toBeDisabled()
})

test('binds a new two-entry route’s hostname after creating it', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-editor-blank', { clock: true })
  await page.getByLabel('Name').first().fill('e2e-ha')
  await chooseNode(page, 0, 'hk-edge-01')
  await chooseNode(page, 0, 'hk-edge-02')
  await page.getByLabel('Entry hostname').fill('ha.example.net')
  await page.getByLabel('Target 1 host').fill('origin.example.com')
  await page.getByLabel('Target 1 port').fill('443')

  const picker = page.getByTestId('forward-dns-picker')
  await picker.getByRole('switch', { name: 'Keep this hostname on the healthy entries' }).click()
  await picker.getByRole('combobox', { name: 'DNS provider' }).click()
  await page.getByRole('option', { name: /cloudflare-main/ }).click()
  await expect(picker.getByLabel('Zone')).toHaveValue('example.net')
  await picker.getByRole('checkbox', { name: /AAAA/ }).click()
  await picker.getByLabel('TTL').fill('120')
  await picker.getByLabel('TTL').blur()
  await expect(page.getByTestId('forward-save')).toBeEnabled({ timeout: 10_000 })
  await page.getByTestId('forward-save').click()
  await expect(page).toHaveURL(/\/admin\/forward\/routes\/01JBNEWROUTE00000000000000$/)
  const binding = writes.find(write => write.path === '/api/v4/forward/dns/bindings')
  expect(binding.method).toBe('POST')
  expect(binding.key).toMatch(/^fwd-/)
  expect(binding.body).toEqual({
    route_id: '01JBNEWROUTE00000000000000', provider_id: '1', zone: 'example.net', record_name: 'ha.example.net',
    mode: 'DNS_BINDING_MODE_DDNS', record_types: ['DNS_RECORD_TYPE_A', 'DNS_RECORD_TYPE_AAAA'], ttl: 120
  })
  const order = writes.map(write => write.path).filter(path => !path.endsWith('/preview'))
  expect(order).toEqual(['/api/v4/forward/routes', '/api/v4/forward/dns/bindings'])
})

test('changes a stored binding with the whole object and shows the CNAME instruction', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-editor-ha', { clock: true })
  const picker = page.getByTestId('forward-dns-picker')
  await expect(picker.getByText('Bound to a DNS provider')).toBeVisible()
  await expect(picker.getByTestId('forward-dns-identity')).toContainText('cloudflare-main · Cloudflare')
  await expect(picker.getByRole('combobox', { name: 'DNS provider' })).toHaveCount(0)
  await picker.getByRole('switch', { name: 'Paused' }).click()
  await page.getByTestId('forward-save').click()
  await expect(page).toHaveURL(new RegExp(`/admin/forward/routes/${EDGE}$`))
  const update = writes.find(write => write.path === '/api/v4/forward/dns/bindings/1')
  expect(update.method).toBe('PUT')
  expect(update.body).toEqual({
    id: '1', route_id: EDGE, provider_id: '1', zone: 'example.net', record_name: 'edge.example.net', mode: 'DNS_BINDING_MODE_DDNS',
    record_types: ['DNS_RECORD_TYPE_A'], ttl: 60, paused: true
  })
  // Only the binding changed: the route is not rewritten.
  expect(writes.some(write => write.method === 'PUT' && write.path.startsWith('/api/v4/forward/routes/'))).toBe(false)
})

test('the route page shows entry high availability and unbinds with purge', async ({ page }) => {
  const writes = recordWrites(page)
  await openScreen(page, 'admin-forward-route-ha', { clock: true })
  const card = page.getByTestId('forward-entry-ha')
  await expect(card.getByTestId('forward-entry-ha-state')).toHaveText('OK')
  await expect(card.getByText('edge.example.net').first()).toBeVisible()
  await expect(card.locator('[data-node-ref="forward-41"]')).toContainText('Healthy')
  await expect(card.locator('[data-node-ref="forward-42"]')).toContainText('Report stale')
  await expect(card.locator('[data-node-ref="forward-42"]')).toContainText('Out of rotation')

  await card.getByTestId('forward-entry-ha-unbind').click()
  const dialog = page.getByRole('dialog', { name: 'Unbind edge.example.net?' })
  await expect(dialog.getByRole('checkbox', { name: /Also delete the records/ })).toBeChecked()
  await dialog.getByTestId('forward-entry-ha-unbind-confirm').click()
  await expect(page.getByText('Unbound edge.example.net').first()).toBeVisible()
  const remove = writes.find(write => write.method === 'DELETE')
  expect(remove.path).toBe('/api/v4/forward/dns/bindings/1')
  expect(remove.query).toEqual({ purge: 'true' })
  expect(remove.key).toMatch(/^fwd-/)
})

test('the DNS page is a section of the forwarding area', async ({ page }) => {
  await openScreen(page, 'admin-forward-routes', { clock: true })
  await page.getByRole('group', { name: 'Forwarding' }).getByRole('button', { name: 'DNS' }).click()
  await expect(page).toHaveURL(/\/admin\/forward\/dns$/)
  await expect(page.locator('#admin-sidebar a[href="/admin/forward/overview"]')).toHaveAttribute('aria-current', 'page')
})
