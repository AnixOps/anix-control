import { HOUR, NOW, hourly } from './monitorCommon.js'
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
export default {
  path: '/admin/dashboard',
  edition: 'community',
  api(path, { scenario }) {
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
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    table: async page => { await page.getByRole('button', { name: '以表格查看' }).click() }
  }
}
