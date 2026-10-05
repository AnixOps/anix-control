import { FIXED_NOW_MS } from './clock.js'

const GIB = 1024 ** 3
const NAMES = ['lin.xiao', 'wang.fang', 'chen.jie', 'zhao.lei', 'sun.li', 'zhou.min', 'wu.hao', 'zheng.yu', 'feng.yi', 'he.ming', 'luo.qi', 'gao.yan', 'xu.ning', 'ma.chao']
const now = Math.floor(Date.UTC(2026, 9, 2) / 1000)
const nowSeconds = Math.floor(FIXED_NOW_MS / 1000)
const USERS = NAMES.map((name, index) => ({
  id: 100 + index,
  email: `${name}@example.com`,
  banned: index % 6 === 4 ? 1 : 0,
  is_admin: index === 0 ? 1 : 0,
  u: ((index * 37) % 120) * GIB,
  d: ((index * 11) % 80) * GIB,
  transfer_enable: index % 5 === 3 ? 0 : 200 * GIB,
  expired_at: index % 4 === 2 ? now - 86400 * 3 : now + 86400 * (30 + index * 7),
  plan: index % 3 === 0 ? { id: 1, name: '标准 200G' } : (index % 3 === 1 ? { id: 2, name: '高级 1T' } : null),
  speed_limit: index % 2 ? 100 : 0,
  device_limit: index % 3 ? 3 : 0,
  flowResetTime: 1,
  created_at: new Date(Date.UTC(2026, 2, 1 + index)).toISOString()
}))

// GET /api/v4/admin/users/activity: when each user was last seen on a node, in
// seconds, null for a user never seen. Fixed against the mocked clock.
const SEEN = [120, 7 * 60, 3 * 3600, 2 * 86400, null, 45 * 60, 5 * 86400, null, 20, 12 * 3600, 26 * 86400, 600, null, 4 * 3600]
const lastOnline = id => {
  const ago = SEEN[(id - 100) % SEEN.length]
  return ago === null ? null : nowSeconds - ago
}

// The columns GET /admin/users sorts by (sort and order), over the fixture rows.
const SORT = {
  id: user => user.id,
  email: user => user.email,
  traffic: user => user.u + user.d,
  transfer_enable: user => user.transfer_enable,
  expired_at: user => user.expired_at,
  created_at: user => user.created_at
}

export default {
  path: '/admin/users',
  api(path, { query, scenario, method, body }) {
    // One request for a bulk action, answered per user (a user that does not
    // exist is not_found; banning yourself is forbidden_self).
    if (path === '/api/v4/admin/users/bulk' && method === 'POST') {
      const ids = body?.ids || []
      const results = ids.map(id => (id === 999
        ? { id, ok: false, error: { code: 'not_found', message: 'user not found' } }
        : { id, ok: true }))
      return { data: { action: body?.action, requested: ids.length, succeeded: results.filter(item => item.ok).length, failed: results.filter(item => !item.ok).length, results } }
    }
    if (path === '/api/v4/admin/users/activity') {
      const ids = String(query.ids || '').split(',').map(Number).filter(Boolean)
      return { data: { users: ids.map(id => ({ user_id: id, last_online_at: lastOnline(id) })) } }
    }
    if (path === '/api/v2/admin/users') {
      if (scenario === 'empty') return { code: 0, data: { list: [], total: 0 } }
      if (scenario === 'error') return { __status: 502, body: { msg: '上游服务没有响应' } }
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: { list: [], total: 0 } } }
      let list = USERS
      if (query.status === 'banned') list = list.filter(u => u.banned)
      if (SORT[query.sort]) {
        const key = SORT[query.sort]
        const factor = query.order === 'desc' ? -1 : 1
        list = [...list].sort((a, b) => factor * (key(a) === key(b) ? 0 : key(a) < key(b) ? -1 : 1))
      }
      return { code: 0, data: { list: list.slice(0, 20), total: list.length === USERS.length ? 236 : list.length } }
    }
    if (path === '/api/v2/admin/users/stats') return { code: 0, data: { total_users: 236, active_users: 198, expired_users: 27, banned_users: 11 } }
    if (path === '/api/v2/admin/subscription-groups') return { code: 0, data: [{ id: 1, name: '默认分组' }] }
    if (path.startsWith('/api/v2/admin/users/')) return { code: 0, data: USERS[0] }
    return undefined
  },
  viewportOnly: ['selected', 'sheet', 'menu'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    selected: async (page) => {
      const boxes = page.getByRole('checkbox', { name: /^选择 / })
      await boxes.nth(1).click()
      await boxes.nth(2).click()
      await boxes.nth(4).click()
    },
    menu: async (page) => {
      await page.getByRole('button', { name: /lin.xiao@example.com 的操作/ }).first().click()
    },
    sheet: async (page, { width }) => {
      if (width < 640) await page.getByRole('button', { name: 'wang.fang@example.com', exact: true }).click()
      else await page.getByRole('row', { name: /wang.fang/ }).click()
    }
  }
}
