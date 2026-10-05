import { HOUR, NOW, hourly } from './monitorCommon.js'
import { FIXED_NOW_MS } from './clock.js'
const STATS = { total_users: 1204, today_new_users: 12, active_users: 980, expired_users: 31, banned_users: 4, total_nodes: 20, active_nodes: 18, online_users: 342, total_traffic_used: 46.3e12, today_traffic: 137.9e9, monthly_income: 1288000, today_income: 45600, total_revenue: 9876500, total_orders: 912, pending_orders: 3, paid_orders: 870, cached_at: new Date((NOW - 40) * 1000).toISOString() }
const NODES = [
  { id: 1, name: 'hk-01', status: 1, last_check_at: NOW - 20 },
  { id: 7, name: 'tokyo-02', status: 2, last_check_at: NOW - 3 * 3600 },
  { id: 9, name: 'sg-edge-03', status: 2, last_check_at: NOW - 26 * 3600 }
]
const TICKETS = [
  { id: 31, subject: '无法连接香港节点', status: 0, created_at: NOW - 5 * 3600 },
  { id: 32, subject: '订阅链接失效', status: 0, created_at: NOW - 1800 },
  { id: 33, subject: '感谢', status: 1, created_at: NOW - 600 }
]
const AUDIT = [
  { id: 501, action: 'delete', module: 'nodes', username: 'root@example.com', content: '删除节点 sg-old-01', status: 'success', created_at: NOW - 180 },
  { id: 500, action: 'update', module: 'system', username: 'ops@example.com', content: '更新订阅域名', status: 'success', created_at: NOW - 3600 },
  { id: 499, action: 'login', module: 'auth', username: 'ops@example.com', content: '管理员登录', status: 'failed', created_at: NOW - 7200 },
  { id: 498, action: 'create', module: 'users', username: 'root@example.com', content: '创建用户 alice@example.com', status: 'success', created_at: NOW - 86400 }
]

// GET /api/v4/kernel/alerts: the kernel's alerts as the server answers them
// (kinds, subjects and detail of docs/reference/kernel-alerts.md). Dates are
// offsets from `now`, what the page's clock reads.
const DAY_MS = 86_400_000
const iso = (now, offset) => new Date(now + offset).toISOString()
function kernelAlerts(now) {
  const alert = (id, kind, severity, subject_kind, subject, detail, extra = {}) => ({
    id, key: `${kind}/${subject}`, kind, severity, status: 'active', subject_kind, subject, message: `English notification text of alert ${id}`,
    detail, first_seen_at: iso(now, -2 * DAY_MS), last_seen_at: iso(now, -15 * 60_000), last_notified_at: iso(now, -6 * 3_600_000), notify_count: 1, resolved_at: null, ...extra
  })
  return {
    active: [
      alert(31, 'ca_expiring', 'critical', 'ca', 'forward_link_ca:3fa9c1', { ca: 'forward_link', cluster: 'default', not_after: iso(now, 6 * DAY_MS), expired: false, next_staged: false, window_days: 60 }, { expires_at: iso(now, 6 * DAY_MS) }),
      alert(32, 'agent_certificate_expiring', 'warning', 'node', 'proxy-1', { node: 'proxy-1', node_name: 'hk-01', not_after: iso(now, 20 * 3_600_000), expired: false, lifetime_hours: 168, window_hours: 28 }, { expires_at: iso(now, 20 * 3_600_000) }),
      alert(33, 'link_certificate_expiring', 'warning', 'node', 'forward-4', { node: 'forward-4', node_name: 'edge-sg-1', not_after: iso(now, 14 * 3_600_000), expired: false, lifetime_hours: 168, window_hours: 28 }, { expires_at: iso(now, 14 * 3_600_000) }),
      alert(34, 'node_secrets_split_stalled', 'warning', 'node_secrets', 'v2_node', { table: 'v2_node', phase: 'dual_write', since: iso(now, -5 * DAY_MS), stuck_after_hours: 72 })
    ],
    resolved: [
      alert(21, 'module_certificate_expiring', 'warning', 'module', 'identity-platform#4c1d0e7a', { package_id: 'identity-platform', not_after: iso(now, -1 * DAY_MS), expired: false }, { status: 'resolved', resolved_at: iso(now, -3 * 3_600_000) }),
      alert(20, 'identity_import_stalled', 'warning', 'identity', 'authority', { state: 'importing', since: iso(now, -9 * DAY_MS), stuck_after_hours: 72 }, { status: 'resolved', resolved_at: iso(now, -4 * DAY_MS) })
    ]
  }
}

export default {
  path: '/admin/dashboard',
  edition: 'community',
  api(path, { scenario, query, now }) {
    if (path === '/api/v4/kernel/alerts') {
      if (scenario === 'alertsFailed') return { __status: 500, body: { error: { code: 'internal', message: 'the alert store is unavailable' } } }
      const { active, resolved } = kernelAlerts(now ?? FIXED_NOW_MS)
      const none = scenario === 'empty' || scenario === 'alertsNone'
      const summary = none ? { active: 0, critical: 0, warning: 0 } : { active: active.length, critical: 1, warning: active.length - 1 }
      const alerts = query?.status === 'resolved' ? (none ? [] : resolved) : (none ? [] : active)
      return { data: { alerts, summary } }
    }
    if (path === '/api/v2/admin/dashboard') {
      if (scenario === 'error') return { __status: 502, body: { msg: '统计服务不可用' } }
      if (scenario === 'loading') return { __delay: 60000, body: { data: STATS } }
      return { data: scenario === 'empty' ? { ...STATS, total_users: 0, today_new_users: 0, active_users: 0, total_nodes: 0, active_nodes: 0, online_users: 0, today_traffic: 0, total_traffic_used: 0 } : STATS }
    }
    if (path === '/api/v2/admin/traffic/hourly') {
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: { list: [] } } }
      return { code: 0, data: { list: scenario === 'empty' ? hourly(24, 0) : hourly(24), meta: { latest_log_at: scenario === 'empty' ? 0 : HOUR } } }
    }
    if (path === '/api/v2/admin/ticket') return { code: 0, data: scenario === 'empty' ? [] : TICKETS }
    if (path === '/api/v2/admin/nodes') return { code: 0, data: { list: scenario === 'empty' ? [] : NODES, total: 3 } }
    if (path === '/api/v2/admin/system/audit-logs') return { code: 0, data: { list: scenario === 'empty' ? [] : AUDIT, total: 4 } }
    return undefined
  },
  scenarios: {
    default: async () => {},
    alertsFailed: async () => {},
    alertsNone: async () => {},
    alertsResolved: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    table: async page => { await page.getByRole('button', { name: '以表格查看' }).click() }
  },
  // alertsResolved: the history view of 需要处理.
  async after(page, { scenario }) {
    if (scenario !== 'alertsResolved') return
    await page.getByRole('group', { name: 'Alert status' }).getByRole('button', { name: 'Resolved' }).click()
    await page.locator('[data-alert^="kernel-"]').first().waitFor()
  }
}
