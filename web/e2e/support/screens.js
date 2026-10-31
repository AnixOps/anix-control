// Mocked-API screens for the accessibility (e2e/a11y.spec.js) and visual
// regression (e2e/visual/) specs. Each screen is a fixture under
// e2e/fixtures/ ({ path | pathFor, api(path, ctx), edition, user, setup })
// plus the scenario it answers with ('empty', 'error', ...). The API answers
// come from the fixture first, then COMMON, then an empty success envelope.
import agentUpgrade from '../fixtures/agentUpgrade.js'
import apiTokens from '../fixtures/apiTokens.js'
import claim from '../fixtures/claim.js'
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
import packages from '../fixtures/packages.js'
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
  // The first administrator's claim link: the form, and nothing to claim.
  claim: { fixture: claim, scenario: 'form' },
  'claim-none': { fixture: claim, scenario: 'none' },
  'user-home': { fixture: user, scenario: 'home' },
  'user-subscribe': { fixture: user, scenario: 'subscribe' },
  'user-help': { fixture: user, scenario: 'help' },
  'user-tickets': { fixture: user, scenario: 'tickets' },
  'user-account': { fixture: user, scenario: 'account' },
  'admin-dashboard': { fixture: dashboard, scenario: 'default' },
  'admin-dashboard-error': { fixture: dashboard, scenario: 'error' },
  // The kernel's alerts in 需要处理: the resolved history, none, and a failed load.
  'admin-dashboard-alerts-resolved': { fixture: dashboard, scenario: 'alertsResolved' },
  'admin-dashboard-alerts-none': { fixture: dashboard, scenario: 'alertsNone' },
  'admin-dashboard-alerts-failed': { fixture: dashboard, scenario: 'alertsFailed' },
  'admin-users': { fixture: users, scenario: 'list' },
  'admin-users-empty': { fixture: users, scenario: 'empty' },
  'admin-orders': { fixture: orders, scenario: 'list' },
  'admin-tickets': { fixture: tickets, scenario: 'list' },
  'admin-coupons': { fixture: coupons, scenario: 'list' },
  'admin-invite-codes': { fixture: inviteCodes, scenario: 'list' },
  'admin-nodes': { fixture: nodes, scenario: 'list' },
  // 接入节点: the dialog while it waits for the first machine, with two
  // machines joined (one waits for approval) and on a Control that cannot hand
  // out a command yet; the node group tab and a group's edit sheet; the
  // nodes waiting for approval; and a node's 部署 section.
  'admin-nodes-awaiting': { fixture: nodes, scenario: 'awaiting' },
  'admin-node-join': { fixture: nodes, scenario: 'join' },
  'admin-node-joined': { fixture: nodes, scenario: 'joined' },
  'admin-node-join-blocked': { fixture: nodes, scenario: 'joinBlocked' },
  'admin-node-groups': { fixture: nodes, scenario: 'groups' },
  'admin-node-group-edit': { fixture: nodes, scenario: 'groupEdit' },
  'admin-node-detail': { fixture: nodeDetail, scenario: 'overview' },
  'admin-node-services': { fixture: nodeDetail, scenario: 'services' },
  'admin-node-deploy': { fixture: nodeDetail, scenario: 'deploy' },
  // The node's traffic over time, and rotating its Agent credentials.
  'admin-node-traffic': { fixture: nodeDetail, scenario: 'traffic' },
  'admin-node-traffic-empty': { fixture: nodeDetail, scenario: 'trafficEmpty' },
  'admin-node-traffic-error': { fixture: nodeDetail, scenario: 'trafficError' },
  // 诊断: the section, a result in it, and a node that is offline.
  'admin-node-diagnostics': { fixture: nodeDetail, scenario: 'diagnostics' },
  'admin-node-diagnostics-result': { fixture: nodeDetail, scenario: 'diagnosticsResult' },
  'admin-node-diagnostics-offline': { fixture: nodeDetail, scenario: 'diagnosticsOffline' },
  // 升级节点: the sheet opened with ⌘K, with no campaign and with a running one.
  'admin-agent-upgrade': { fixture: agentUpgrade, scenario: 'upgrade' },
  'admin-agent-upgrade-running': { fixture: agentUpgrade, scenario: 'upgradeRunning' },
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
  'admin-forward-editor-copy-ha': { fixture: forwardV4, scenario: 'editor-copy-ha' },
  'admin-forward-editor-preview': { fixture: forwardV4, scenario: 'editor-preview' },
  'admin-forward-route': { fixture: forwardV4, scenario: 'route' },
  'admin-forward-diagnose': { fixture: forwardV4, scenario: 'diagnose' },
  'admin-forward-node': { fixture: forwardV4, scenario: 'node' },
  // Entry HA through DNS (L2).
  'admin-forward-dns': { fixture: forwardV4, scenario: 'dns' },
  'admin-forward-route-ha': { fixture: forwardV4, scenario: 'route-ha' },
  'admin-forward-editor-ha': { fixture: forwardV4, scenario: 'editor-ha' },
  'admin-forward-no-capability': { fixture: forwardV4, scenario: 'no-capability' },
  'admin-subscriptions': { fixture: subscriptions, scenario: 'list' },
  'admin-subscription-members': { fixture: subscriptions, scenario: 'members' },
  'admin-system': { fixture: system, scenario: 'general' },
  // 系统设置 → 备份: the bundles with their verification, none yet, a stale
  // last verified backup, and a panel on PostgreSQL.
  'admin-system-backup': { fixture: system, scenario: 'backup' },
  'admin-system-backup-empty': { fixture: system, scenario: 'backupEmpty' },
  'admin-system-backup-stale': { fixture: system, scenario: 'backupStale' },
  'admin-system-backup-unsupported': { fixture: system, scenario: 'backupUnsupported' },
  // 系统设置 → 内置包: the built-in packages (it replaced 插件中心).
  'admin-system-packages': { fixture: packages, scenario: 'list' },
  'admin-security': { fixture: security, scenario: 'mfa' },
  'admin-access-groups': { fixture: security, scenario: 'groups' },
  // Security → API tokens: the list, with ended and everyone's tokens, empty
  // and failed; the create form and the dialog that shows the token once.
  'admin-api-tokens': { fixture: apiTokens, scenario: 'list' },
  'admin-api-tokens-all': { fixture: apiTokens, scenario: 'all' },
  'admin-api-tokens-empty': { fixture: apiTokens, scenario: 'empty' },
  'admin-api-tokens-error': { fixture: apiTokens, scenario: 'error' },
  'admin-api-token-create': { fixture: apiTokens, scenario: 'create' },
  'admin-api-token-created': { fixture: apiTokens, scenario: 'created' },
  'admin-notifications': { fixture: notifications, scenario: 'email' },
  // Notifications → Telegram: the page, and the test message's answers (a
  // delivered one, a blocked bot, an unreachable Telegram, no linked account).
  'admin-telegram': { fixture: notifications, scenario: 'telegram' },
  'admin-telegram-test-ok': { fixture: notifications, scenario: 'telegramTestOk' },
  'admin-telegram-test-blocked': { fixture: notifications, scenario: 'telegramTestBlocked' },
  'admin-telegram-test-network': { fixture: notifications, scenario: 'telegramTestNetwork' },
  'admin-telegram-test-not-bound': { fixture: notifications, scenario: 'telegramTestNotBound' },
  'admin-telegram-test-limited': { fixture: notifications, scenario: 'telegramTestLimited' },
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
// running) so relative times and charts are the same on every run. `scenario`
// answers with another scenario of the screen's fixture (a refusal, a state
// the sweeps do not need a screen for).
export async function openScreen(page, name, { theme = 'light', locale = 'en', clock = false, scenario: variant = '' } = {}) {
  const screen = SCREENS[name]
  if (!screen) throw new Error(`unknown screen ${name}`)
  const { fixture } = screen
  const scenario = variant || screen.scenario
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
    if (answer?.__status) return route.fulfill({ status: answer.__status, contentType: 'application/json', headers: answer.headers, body: JSON.stringify(answer.body || {}) })
    if (answer?.__text) return route.fulfill({ status: 200, contentType: 'text/plain', body: answer.__text })
    if (answer?.__file) return route.fulfill({ status: 200, contentType: answer.__file.type, headers: { 'Content-Disposition': `attachment; filename="${answer.__file.name}"` }, body: answer.__file.body })
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
