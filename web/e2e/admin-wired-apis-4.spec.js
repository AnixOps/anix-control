import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { openScreen } from './support/screens.js'
import { TELEGRAM_TEST_PATH } from './fixtures/notifications.js'

// Notifications → Telegram → Test message (docs/reference/telegram-test-endpoint.md)
// and the kernel's alerts in the dashboard's 需要处理
// (docs/reference/kernel-alerts.md), against the mocked routes
// (e2e/fixtures/notifications.js, e2e/fixtures/dashboard.js).
//
// The Telegram flows run on the real clock: the button's countdown is a timer,
// and nothing here needs the screens' fixed one.
test.use({ timezoneId: 'UTC', locale: 'en-US' })

function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return
    calls.push({ method: request.method(), path: url.pathname, query: Object.fromEntries(url.searchParams), headers: request.headers(), body: request.postData() })
  })
  return { calls, to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method) }
}

async function axeFindings(page, include) {
  const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
  if (include) builder.include(include)
  const results = await builder.analyze()
  return results.violations.map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
}

test.describe('the Telegram test message', () => {
  test('sends {} to the kernel route and shows a delivered message as a success', async ({ page }) => {
    const requests = recordRequests(page)
    const logged = []
    page.on('console', message => logged.push(message.text()))
    await openScreen(page, 'admin-telegram')
    const section = page.getByRole('heading', { name: 'Test message' })
    await expect(section).toBeVisible()
    await expect(page.getByTestId('telegram-test-result')).toHaveCount(0)

    await page.getByRole('button', { name: 'Send test message' }).click()
    const result = page.getByTestId('telegram-test-result')
    await expect(result).toHaveAttribute('data-tone', 'success')
    await expect(result).toContainText('Delivered')
    await expect(result).toContainText('Telegram accepted the test message.')

    const calls = requests.to(TELEGRAM_TEST_PATH, 'POST')
    expect(calls).toHaveLength(1)
    expect(JSON.parse(calls[0].body)).toEqual({})
    expect(calls[0].headers.authorization).toBe('Bearer e2e-screen-token')
    // The page never reads or sends the bot token for this: the saved one is the server's.
    expect(calls[0].body).not.toContain('123456:ABC-DEF')
    expect(logged.join('\n')).not.toContain('123456:ABC-DEF')
  })

  test('shows a blocked bot as a warning and an unreachable Telegram as a danger, with the reason', async ({ page }) => {
    await openScreen(page, 'admin-telegram-test-blocked')
    const blocked = page.getByTestId('telegram-test-result')
    await expect(blocked).toHaveAttribute('data-tone', 'warning')
    await expect(blocked).toContainText('Bot blocked')
    await expect(blocked).toContainText('the chat blocked the bot or never started it')
    await expect(blocked).toContainText('press Start')

    await openScreen(page, 'admin-telegram-test-network')
    const network = page.getByTestId('telegram-test-result')
    await expect(network).toHaveAttribute('data-tone', 'danger')
    await expect(network).toContainText('Telegram not reachable')
    await expect(network).toContainText('api.telegram.org did not resolve')
  })

  test('tells an administrator without a linked Telegram account to link it first (409)', async ({ page }) => {
    await openScreen(page, 'admin-telegram-test-not-bound')
    const result = page.getByTestId('telegram-test-result')
    await expect(result).toHaveAttribute('data-tone', 'warning')
    await expect(result).toContainText('Telegram not linked')
    await expect(result).toContainText('/bind')
    await expect(page.getByRole('button', { name: 'Send test message' })).toBeEnabled()
  })

  test('honours Retry-After on 429: the button is disabled and counts down', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-telegram-test-limited')
    const button = page.getByRole('button', { name: 'Send test message' })
    await expect(page.getByTestId('telegram-test-result')).toContainText('Too many tests')
    await expect(button).toBeDisabled()
    const wait = page.getByTestId('telegram-test-wait')
    await expect(wait).toContainText(/^Try again in (42|41|40) s$/)
    const first = Number((await wait.innerText()).match(/\d+/)[0])
    await expect.poll(async () => Number((await wait.innerText()).match(/\d+/)[0]), { timeout: 5_000 }).toBeLessThan(first)
    await button.click({ force: true })
    expect(requests.to(TELEGRAM_TEST_PATH, 'POST')).toHaveLength(1)
  })

  test('disables the button while the request is in flight', async ({ page }) => {
    await openScreen(page, 'admin-telegram')
    let release
    const gate = new Promise((resolve) => { release = resolve })
    await page.route(url => url.pathname === TELEGRAM_TEST_PATH, async (route) => {
      await gate
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { class: 'ok', ok: true, message: 'Telegram accepted the test message.', target: 'self' } }) })
    })
    const button = page.getByRole('button', { name: 'Send test message' })
    await button.click()
    await expect(button).toHaveAttribute('aria-busy', 'true')
    release()
    await expect(page.getByTestId('telegram-test-result')).toHaveAttribute('data-tone', 'success')
    await expect(button).not.toHaveAttribute('aria-busy', 'true')
  })
})

test.describe('kernel alerts in the dashboard', () => {
  test('merges them with the browser-built alerts: critical first, text from kind and detail, node links, summary badge', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-dashboard', { clock: true })
    expect(requests.to('/api/v4/kernel/alerts')).toHaveLength(1)
    expect(requests.to('/api/v4/kernel/alerts')[0].query).toEqual({ status: 'active' })

    const card = page.locator('[data-dashboard-alerts]')
    await expect(card.locator('[data-alerts-summary]')).toHaveText('4 active, 1 critical')
    const items = card.locator('[data-alert]')
    // Danger first: the offline nodes (browser-built) and the critical CA.
    const order = await items.evaluateAll(list => list.map(item => item.getAttribute('data-alert')))
    expect(order.indexOf('kernel-31')).toBeLessThan(order.indexOf('tickets'))
    expect(order.indexOf('kernel-31')).toBeLessThan(order.indexOf('kernel-32'))
    expect(order.indexOf('tickets')).toBeGreaterThan(order.findIndex(key => key.startsWith('node-')))

    await expect(card.locator('[data-alert="kernel-31"]')).toContainText('The current forward link CA is about to end')
    await expect(card.locator('[data-alert="kernel-31"]')).toContainText('No next CA is staged: rotate the CA now.')
    await expect(card.locator('[data-alert="kernel-32"]')).toContainText('The Agent certificate of hk-01 is about to expire')
    await expect(card.locator('[data-alert="kernel-32"]')).toContainText('Ends 2026-10-03 04:00')
    await expect(card.locator('[data-alert="kernel-33"]')).toContainText('The forward link certificate of edge-sg-1 is about to expire')
    await expect(card.locator('[data-alert="kernel-34"]')).toContainText('The credential split of v2_node has stalled in dual_write')
    // Node subjects link to the node; others are plain text.
    await expect(card.locator('[data-alert="kernel-32"] a')).toHaveAttribute('href', '/admin/nodes/1')
    await expect(card.locator('[data-alert="kernel-33"] a')).toHaveAttribute('href', '/admin/forward/inventory/forward-4')
    await expect(card.locator('[data-alert="kernel-31"] a')).toHaveCount(0)
    // The browser-built ones still work.
    await expect(card.locator('[data-alert="node-9"]')).toContainText('sg-edge-03 is offline')
    await expect(card.locator('[data-alert="tickets"]')).toContainText('2 tickets waiting for a reply')
  })

  test('shows the resolved history from the toggle and goes back', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-dashboard', { clock: true })
    const card = page.locator('[data-dashboard-alerts]')
    const toggle = card.getByRole('group', { name: 'Alert status' })
    await expect(toggle.getByRole('button', { name: 'Active' })).toHaveAttribute('aria-pressed', 'true')
    await toggle.getByRole('button', { name: 'Resolved' }).click()
    await expect(toggle.getByRole('button', { name: 'Resolved' })).toHaveAttribute('aria-pressed', 'true')
    await expect.poll(() => requests.to('/api/v4/kernel/alerts').at(-1).query).toEqual({ status: 'resolved', limit: '30' })
    await expect(card.locator('[data-alert="kernel-21"]')).toContainText('The certificate of module identity-platform is about to expire')
    await expect(card.locator('[data-alert="kernel-21"]')).toContainText('Resolved 2026-10-02 05:00')
    await expect(card.locator('[data-alert="kernel-20"]')).toContainText('The identity import has stalled')
    await expect(card.locator('[data-alert="tickets"]')).toHaveCount(0)
    await toggle.getByRole('button', { name: 'Active' }).click()
    await expect(card.locator('[data-alert="tickets"]')).toBeVisible()
  })

  test('keeps the dashboard working when the alerts fail, and loads them again on Try again', async ({ page }) => {
    await openScreen(page, 'admin-dashboard-alerts-failed', { clock: true })
    const card = page.locator('[data-dashboard-alerts]')
    await expect(card.locator('[data-alert="alerts-failed"]')).toContainText('Couldn’t load certificate and rollout alerts')
    await expect(card.locator('[data-alert="node-9"]')).toBeVisible()
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.locator('[data-metric="users"]')).toContainText('1,204')
    await expect(page.getByRole('button', { name: 'Refresh' })).toBeEnabled()
    // The server answers this time.
    await page.route(url => url.pathname === '/api/v4/kernel/alerts', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { alerts: [], summary: { active: 0, critical: 0, warning: 0 } } }) }))
    await card.locator('[data-alert="alerts-failed"]').getByRole('button', { name: 'Try again' }).click()
    await expect(card.locator('[data-alert="alerts-failed"]')).toHaveCount(0)
  })
})

// Every state of both areas, light and dark, desktop and phone: no horizontal
// scroll and no axe finding of any impact.
const SWEEPS = [
  ['admin-telegram', {}],
  ['admin-telegram-test-ok', {}],
  ['admin-telegram-test-blocked', {}],
  ['admin-telegram-test-network', {}],
  ['admin-telegram-test-not-bound', {}],
  ['admin-telegram-test-limited', {}],
  ['admin-dashboard', { clock: true }],
  ['admin-dashboard-alerts-resolved', { clock: true }],
  ['admin-dashboard-alerts-none', { clock: true }],
  ['admin-dashboard-alerts-failed', { clock: true }]
]
for (const [name, options] of SWEEPS) {
  for (const [theme, width, height] of [['light', 1440, 900], ['dark', 1440, 900], ['light', 390, 844], ['dark', 390, 844]]) {
    test(`${name} has no axe finding and no horizontal scroll (${theme}, ${width})`, async ({ page }) => {
      await page.setViewportSize({ width, height })
      await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' })
      await openScreen(page, name, { theme, ...options })
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
      expect(overflow, 'horizontal scroll').toBeLessThanOrEqual(0)
      await page.mouse.move(0, 0)
      await page.evaluate(() => Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {}))))
      const findings = await axeFindings(page)
      expect(findings, findings.join('\n')).toEqual([])
    })
  }
}
