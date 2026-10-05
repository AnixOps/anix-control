// Mocked-API screens for the accessibility (e2e/a11y.spec.js) and visual
// regression (e2e/visual/) specs. Each screen is a fixture under
// e2e/fixtures/ ({ path | pathFor, api(path, ctx), edition, user, setup })
// plus the scenario it answers with ('empty', 'error', ...). The API answers
// come from the fixture first, then COMMON, then an empty success envelope.
import agent from '../fixtures/agent.js'
import coupons from '../fixtures/coupons.js'
import dashboard from '../fixtures/dashboard.js'
import deployments from '../fixtures/deployments.js'
import forwardV4 from '../fixtures/forwardV4.js'
import inviteCodes from '../fixtures/inviteCodes.js'
import monitor from '../fixtures/monitor.js'
import nodeDetail from '../fixtures/nodeDetail.js'
import nodes from '../fixtures/nodes.js'
import notifications from '../fixtures/notifications.js'
import orders from '../fixtures/orders.js'
import plugins from '../fixtures/plugins.js'
import security from '../fixtures/security.js'
import subscriptions from '../fixtures/subscriptions.js'
import system from '../fixtures/system.js'
import tickets from '../fixtures/tickets.js'
import user from '../fixtures/user.js'
import users from '../fixtures/users.js'
import { FIXED_NOW_MS } from '../fixtures/clock.js'

export { FIXED_NOW_MS }

// name → { fixture, scenario }
export const SCREENS = {
  login: { fixture: user, scenario: 'login' },
  'user-home': { fixture: user, scenario: 'home' },
  'user-subscribe': { fixture: user, scenario: 'subscribe' },
  'user-help': { fixture: user, scenario: 'help' },
  'user-tickets': { fixture: user, scenario: 'tickets' },
  'user-account': { fixture: user, scenario: 'account' },
  'admin-dashboard': { fixture: dashboard, scenario: 'default' },
  'admin-dashboard-error': { fixture: dashboard, scenario: 'error' },
  'admin-users': { fixture: users, scenario: 'list' },
  'admin-users-empty': { fixture: users, scenario: 'empty' },
  'admin-orders': { fixture: orders, scenario: 'list' },
  'admin-tickets': { fixture: tickets, scenario: 'list' },
  'admin-coupons': { fixture: coupons, scenario: 'list' },
  'admin-invite-codes': { fixture: inviteCodes, scenario: 'list' },
  'admin-agents': { fixture: agent, scenario: 'list' },
  'admin-plugins': { fixture: plugins, scenario: 'list' },
  'admin-nodes': { fixture: nodes, scenario: 'list' },
  'admin-node-detail': { fixture: nodeDetail, scenario: 'overview' },
  'admin-node-services': { fixture: nodeDetail, scenario: 'services' },
  // The node's traffic over time, and rotating its Agent credentials.
  'admin-node-traffic': { fixture: nodeDetail, scenario: 'traffic' },
  'admin-node-traffic-empty': { fixture: nodeDetail, scenario: 'trafficEmpty' },
  'admin-node-traffic-error': { fixture: nodeDetail, scenario: 'trafficError' },
  'admin-node-credentials': { fixture: nodeDetail, scenario: 'credentials' },
  'admin-node-rotate': { fixture: nodeDetail, scenario: 'rotate' },
  'admin-node-rotated': { fixture: nodeDetail, scenario: 'rotated' },
  // The v4.2 forwarding pages (F5b).
  'admin-forward-overview': { fixture: forwardV4, scenario: 'overview' },
  'admin-forward-routes': { fixture: forwardV4, scenario: 'routes' },
  'admin-forward-routes-bulk': { fixture: forwardV4, scenario: 'routes-bulk' },
  'admin-forward-routes-empty': { fixture: forwardV4, scenario: 'routes-empty' },
  'admin-forward-editor': { fixture: forwardV4, scenario: 'editor' },
  'admin-forward-editor-blank': { fixture: forwardV4, scenario: 'editor-blank' },
  'admin-forward-editor-preview': { fixture: forwardV4, scenario: 'editor-preview' },
  'admin-forward-route': { fixture: forwardV4, scenario: 'route' },
  'admin-forward-diagnose': { fixture: forwardV4, scenario: 'diagnose' },
  'admin-forward-nodes': { fixture: forwardV4, scenario: 'nodes' },
  'admin-forward-node': { fixture: forwardV4, scenario: 'node' },
  // Entry HA through DNS (L2).
  'admin-forward-dns': { fixture: forwardV4, scenario: 'dns' },
  'admin-forward-route-ha': { fixture: forwardV4, scenario: 'route-ha' },
  'admin-forward-editor-ha': { fixture: forwardV4, scenario: 'editor-ha' },
  'admin-forward-no-capability': { fixture: forwardV4, scenario: 'no-capability' },
  'admin-subscriptions': { fixture: subscriptions, scenario: 'list' },
  'admin-subscription-members': { fixture: subscriptions, scenario: 'members' },
  'admin-system': { fixture: system, scenario: 'general' },
  'admin-security': { fixture: security, scenario: 'mfa' },
  'admin-access-groups': { fixture: security, scenario: 'groups' },
  'admin-notifications': { fixture: notifications, scenario: 'email' },
  'admin-monitor': { fixture: monitor, scenario: 'live' },
  'admin-monitor-node-traffic': { fixture: monitor, scenario: 'traffic' },
  'admin-deployments': { fixture: deployments, scenario: 'topologies' }
}

const COMMON = {
  '/api/v4/public/config': fixture => ({ edition: fixture.edition || 'commercial', registration: { enabled: true } }),
  '/api/v2/user/profile': () => ({ code: 0, data: { id: 1, email: 'admin@example.com', is_admin: true } }),
  '/api/v2/admin/system/info': () => ({ code: 0, data: { version: '4.1.0' } }),
  '/api/v3/extensions': () => [],
  '/api/v2/admin/notifications/unread': () => ({ code: 0, data: { count: 0 } })
}

const ADMIN = { id: 1, email: 'admin@example.com', is_admin: true }
const MEMBER = { id: 7, email: 'lin.xiao@example.com', is_admin: false, token: '7f3k9q2m8x4v2c6b' }

// Opens a screen with its API mocked. `clock` fixes Date.now() (timers keep
// running) so relative times and charts are the same on every run.
export async function openScreen(page, name, { theme = 'light', locale = 'en', clock = false } = {}) {
  const screen = SCREENS[name]
  if (!screen) throw new Error(`unknown screen ${name}`)
  const { fixture, scenario } = screen
  const signedOut = Boolean(fixture.signedOut?.(scenario))
  await page.addInitScript(({ theme, locale, signedOut, profile }) => {
    localStorage.setItem('app.locale', locale)
    localStorage.setItem('v2board-theme', theme)
    if (signedOut) return
    localStorage.setItem('token', 'e2e-screen-token')
    localStorage.setItem('userInfo', JSON.stringify(profile))
  }, { theme, locale, signedOut, profile: fixture.user ? MEMBER : ADMIN })
  if (clock) await page.clock.setFixedTime(FIXED_NOW_MS)
  await page.route(url => url.pathname.startsWith('/api/') || url.pathname.startsWith('/s/'), async route => {
    const request = route.request()
    const url = new URL(request.url())
    let body = null
    try { body = request.postDataJSON() } catch { body = null }
    // `now` is what the page's clock reads: the fixed time, or the real one.
    const ctx = { method: request.method(), query: Object.fromEntries(url.searchParams), body, scenario, now: clock ? FIXED_NOW_MS : Date.now() }
    let answer = fixture.api ? await fixture.api(url.pathname, ctx) : undefined
    if (answer === undefined && COMMON[url.pathname]) answer = COMMON[url.pathname](fixture)
    if (answer?.__status) return route.fulfill({ status: answer.__status, contentType: 'application/json', body: JSON.stringify(answer.body || {}) })
    if (answer?.__text) return route.fulfill({ status: 200, contentType: 'text/plain', body: answer.__text })
    if (answer === undefined) answer = { code: 0, data: null }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(answer) })
  })
  if (fixture.setup) await fixture.setup(page, { scenario, theme })
  await page.goto(fixture.pathFor ? fixture.pathFor(scenario) : fixture.path, { waitUntil: 'networkidle' })
  // Done loading: the main landmark is there and no skeleton is left.
  await page.locator('main').first().waitFor()
  await page.waitForFunction(() => !document.querySelector('.ui-skeleton'), null, { timeout: 10_000 })
  // A scenario that needs interaction after the load (a selection, a form).
  if (fixture.after) await fixture.after(page, { scenario })
  return screen
}
