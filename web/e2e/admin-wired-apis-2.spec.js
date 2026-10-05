import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'
import { FIXTURE_CREDENTIAL } from './fixtures/nodeTraffic.js'

// The admin console calls two more kernel routes (docs/guide/api-reference.md):
// a proxy node's traffic over time (GET /api/v4/kernel/nodes/:id/traffic) for
// the node page's chart and the live monitor's per-node sheet, and the
// rotation of a node's Agent credentials (POST /api/v4/kernel/agents/
// rotate-credentials). The API is mocked (e2e/fixtures); each test records
// the requests the page makes and checks what it asked for.
function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return
    calls.push({ method: request.method(), path: url.pathname, query: Object.fromEntries(url.searchParams), headers: request.headers(), body: request.postData() })
  })
  return {
    calls,
    to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method)
  }
}

const HOUR = 3_600_000

test.describe('node traffic', () => {
  test('the Traffic section draws the last 24 hours, with totals and the data as a table', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-traffic', { clock: true })
    await expect(page.getByRole('tab', { name: 'Traffic', selected: true })).toBeVisible()
    const chart = page.getByTestId('node-traffic-chart')
    await expect(chart.locator('canvas').first()).toBeVisible()

    // One call, hourly, on whole UTC hours, 24 of them.
    const calls = requests.to('/api/v4/kernel/nodes/108/traffic')
    expect(calls).toHaveLength(1)
    const { granularity, since, until } = calls[0].query
    expect(granularity).toBe('hour')
    expect(Number(since) % HOUR).toBe(0)
    expect((Number(until) - Number(since)) / HOUR).toBe(24)

    // Totals of the range, in the cards.
    await expect(page.locator('[data-summary="up"]')).toContainText(/GB/)
    await expect(page.locator('[data-summary="total"]')).toContainText(/GB/)
    // The plot is named and summarised for screen readers.
    await expect(chart.getByRole('img', { name: /Traffic of tw-01, the last 24 hours: .* uploaded and .* downloaded\. Busiest hour/ })).toBeVisible()

    // The same numbers as a table.
    await chart.getByRole('button', { name: 'View as table' }).click()
    const table = chart.getByRole('table')
    await expect(table.getByRole('row')).toHaveCount(25)
    await expect(table.getByRole('columnheader')).toHaveText(['Time', 'Upload', 'Download', 'Total'])
    await chart.getByRole('button', { name: 'View as chart' }).click()
    await expect(chart.locator('canvas').first()).toBeVisible()
  })

  test('the range switch asks for hours (24 h, 7 d, 30 d) or days (90 d, 1 y) within the route\'s limits', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-traffic', { clock: true })
    const calls = () => requests.to('/api/v4/kernel/nodes/108/traffic')

    for (const [name, buckets] of [['7 d', 168], ['30 d', 720]]) {
      await page.getByRole('button', { name, exact: true }).click()
      await expect(page.getByRole('button', { name, exact: true })).toHaveAttribute('aria-pressed', 'true')
      await expect.poll(() => calls().at(-1).query.since).toBeTruthy()
      const query = calls().at(-1).query
      expect(query.granularity).toBe('hour')
      expect((Number(query.until) - Number(query.since)) / HOUR).toBe(buckets)
    }
    for (const [name, days] of [['90 d', 90], ['1 y', 365]]) {
      await page.getByRole('button', { name, exact: true }).click()
      await expect(page.getByRole('button', { name, exact: true })).toHaveAttribute('aria-pressed', 'true')
      await expect.poll(() => calls().at(-1).query.granularity).toBe('day')
      const query = calls().at(-1).query
      // No `until`: the end of today on the host. `since` is days - 1 days back.
      expect(query).not.toHaveProperty('until')
      expect(Math.round((Date.parse('2026-10-02T08:00:00Z') - Number(query.since)) / 86_400_000)).toBe(days - 1)
    }
    // The daily chart says where its numbers come from.
    await expect(page.getByTestId('node-traffic-note')).toContainText('Daily totals')
    await expect(page.getByTestId('node-traffic-chart').getByRole('img', { name: /the last year/ })).toBeVisible()
    expect(calls()).toHaveLength(5)
  })

  test('says so when there is no traffic, and when it cannot be loaded, with Try again', async ({ page }) => {
    await openScreen(page, 'admin-node-traffic-empty', { clock: true })
    await expect(page.getByText('No traffic in this range')).toBeVisible()
    await expect(page.getByText(/Hourly history only goes back as far as the traffic log is kept/)).toBeVisible()
  })

  test('shows the error of a failed load and loads again on Try again', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-traffic-error', { clock: true })
    await expect(page.getByText('Couldn’t load the traffic')).toBeVisible()
    await expect(page.getByText('the traffic store is unavailable')).toBeVisible()
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect.poll(() => requests.to('/api/v4/kernel/nodes/108/traffic').length).toBe(2)
  })

  test('fits a phone with no sideways scrolling', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openScreen(page, 'admin-node-traffic', { clock: true })
    await expect(page.getByTestId('node-traffic-chart').locator('canvas').first()).toBeVisible()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow).toBeLessThanOrEqual(0)
  })

  test('the live monitor opens a node\'s history in a sheet, by the node\'s id', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-monitor', { clock: true })
    await expect(page.getByRole('row', { name: /hk-01/ })).toBeVisible()
    expect(requests.to('/api/v4/kernel/nodes/1/traffic')).toHaveLength(0)

    await page.getByRole('row', { name: /hk-01/ }).click()
    const sheet = page.getByRole('dialog', { name: 'Traffic of hk-01' })
    await expect(sheet).toBeVisible()
    await expect(sheet.getByTestId('node-traffic-chart').locator('canvas').first()).toBeVisible()
    expect(requests.to('/api/v4/kernel/nodes/1/traffic')).toHaveLength(1)
    await expect(sheet.getByRole('link', { name: 'Open node page' })).toHaveAttribute('href', '/admin/nodes/1?section=traffic')
    await page.keyboard.press('Escape')
    await expect(sheet).toBeHidden()

    // The same from the row menu.
    await page.getByRole('button', { name: 'Actions for tokyo-02' }).click()
    await page.getByRole('menuitem', { name: 'View traffic' }).click()
    await expect(page.getByRole('dialog', { name: 'Traffic of tokyo-02' })).toBeVisible()
    expect(requests.to('/api/v4/kernel/nodes/2/traffic')).toHaveLength(1)
  })
})

test.describe('rotate Agent credentials', () => {
  test('confirms, sends the choices, shows the credential once, and keeps it nowhere afterwards', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    const requests = recordRequests(page)
    // Real clock: the screens' fixed one freezes Date.now(), under which Vue
    // skips a mouse press on a Select trigger inside a dialog (it is a second
    // listener of the event "attached at the same time" as the first).
    await openScreen(page, 'admin-node-credentials')

    await page.getByTestId('rotate-credentials').click()
    const confirm = page.getByRole('alertdialog', { name: 'Rotate the credentials of tw-01?' })
    await expect(confirm).toBeVisible()
    await expect(confirm).toContainText('Every Agent certificate, enrollment and forward link certificate of this node is revoked')
    // Nothing is sent before the button.
    expect(requests.to('/api/v4/kernel/agents/rotate-credentials', 'POST')).toHaveLength(0)
    await expect(confirm.getByRole('button', { name: 'Cancel' })).toBeFocused()

    await confirm.getByRole('textbox', { name: 'Reason' }).fill('disk of the host was stolen')
    await confirm.getByRole('combobox', { name: 'Credential lifetime' }).click()
    await page.getByRole('option', { name: '24 hours' }).click()
    await expect(confirm.getByRole('combobox', { name: 'Credential lifetime' })).toContainText('24 hours')
    await confirm.getByRole('checkbox', { name: 'Also replace the node’s API key' }).click()
    await expect(confirm.getByTestId('rotate-api-key-notice')).toBeVisible()
    await confirm.getByRole('button', { name: 'Rotate credentials' }).click()

    const result = page.getByRole('dialog', { name: 'New credential for tw-01' })
    await expect(result).toBeVisible()
    const posts = requests.to('/api/v4/kernel/agents/rotate-credentials', 'POST')
    expect(posts).toHaveLength(1)
    expect(JSON.parse(posts[0].body)).toEqual({ node: 'proxy-108', rotate_api_key: true, ttl_seconds: 86400, reason: 'disk of the host was stolen' })

    // The credential, masked; copied on request; a warning and the expiry.
    const field = result.getByLabel('One-time credential')
    await expect(field).toHaveValue(FIXTURE_CREDENTIAL)
    await expect(field).toHaveAttribute('type', 'password')
    await expect(result.getByTestId('rotate-warning')).toContainText('Treat it like a password')
    await expect(result.getByTestId('rotate-expiry')).toContainText('Valid until')
    await expect(result.getByTestId('rotate-api-key-result')).toContainText('API key was replaced')
    await result.getByRole('button', { name: 'Copy credential' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(FIXTURE_CREDENTIAL)
    // No made-up command: the guide link is the next step.
    await expect(result).not.toContainText('--reset')
    await expect(result.getByRole('link', { name: /Rotating a node’s credentials/ })).toHaveAttribute('href', /agent-onboarding\.md#rotating-a-nodes-credentials$/)

    // Not in the URL or the browser's storage while it is shown...
    const kept = () => page.evaluate(secret => ({
      url: location.href.includes(secret),
      local: JSON.stringify({ ...localStorage }).includes(secret),
      session: JSON.stringify({ ...sessionStorage }).includes(secret)
    }), FIXTURE_CREDENTIAL)
    expect(await kept()).toEqual({ url: false, local: false, session: false })

    // ...and gone from the page once it is closed, with focus back on the button.
    await result.getByRole('button', { name: 'Done' }).click()
    await expect(result).toBeHidden()
    await expect(page.getByTestId('rotate-credentials')).toBeFocused()
    expect(await page.content()).not.toContain(FIXTURE_CREDENTIAL)
    expect(await page.evaluate(() => document.body.innerText)).not.toContain(FIXTURE_CREDENTIAL)
    expect(await kept()).toEqual({ url: false, local: false, session: false })

    // Asking again starts from a clean form.
    await page.getByTestId('rotate-credentials').click()
    await expect(page.getByRole('textbox', { name: 'Reason' })).toHaveValue('')
    await expect(page.getByRole('checkbox', { name: 'Also replace the node’s API key' })).not.toBeChecked()
  })

  test('Esc closes the result and clears the credential too', async ({ page }) => {
    await openScreen(page, 'admin-node-rotated', { clock: true })
    const result = page.getByRole('dialog', { name: 'New credential for tw-01' })
    await expect(result.getByLabel('One-time credential')).toHaveValue(FIXTURE_CREDENTIAL)
    await page.keyboard.press('Escape')
    await expect(result).toBeHidden()
    expect(await page.content()).not.toContain(FIXTURE_CREDENTIAL)
  })

  test('says so, and stops offering it, when the administrator is not a super administrator', async ({ page }) => {
    await openScreen(page, 'admin-node-credentials', { clock: true })
    await page.route('**/api/v4/kernel/agents/rotate-credentials', route => route.fulfill({
      status: 403, contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'super_admin_required', message: 'only a super administrator may rotate a node\'s Agent credentials' } })
    }))
    await page.getByTestId('rotate-credentials').click()
    const confirm = page.getByRole('alertdialog')
    await confirm.getByRole('button', { name: 'Rotate credentials' }).click()
    await expect(confirm.getByRole('alert')).toHaveText('Only super administrators can rotate credentials.')
    await confirm.getByRole('button', { name: 'Cancel' }).click()
    await expect(page.getByTestId('rotate-credentials')).toHaveCount(0)
    await expect(page.getByTestId('rotate-super-only')).toHaveText('Only super administrators can rotate credentials.')
  })

  test('a disabled node cannot be rotated: the button is off and says why', async ({ page }) => {
    await openScreen(page, 'admin-node-credentials', { clock: true })
    // kr-sel-01 (107) is the disabled node of the fixture.
    await page.goto('/admin/nodes/107?section=credentials')
    await expect(page.getByTestId('rotate-credentials')).toBeDisabled()
    await expect(page.getByText('This node is disabled. Enable it first, then rotate its credentials.')).toBeVisible()
  })

  test('is offered on a forwarding node\'s page too, with no API key option', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-forward-node', { clock: true })
    await page.getByTestId('forward-node-rotate').getByTestId('rotate-credentials').click()
    const confirm = page.getByRole('alertdialog', { name: /^Rotate the credentials of / })
    await expect(confirm).toBeVisible()
    await expect(confirm.getByRole('checkbox')).toHaveCount(0)
    await expect(requests.to('/api/v4/kernel/agents/rotate-credentials', 'POST')).toHaveLength(0)
  })
})
