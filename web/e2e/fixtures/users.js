const GIB = 1024 ** 3
const NAMES = ['lin.xiao', 'wang.fang', 'chen.jie', 'zhao.lei', 'sun.li', 'zhou.min', 'wu.hao', 'zheng.yu', 'feng.yi', 'he.ming', 'luo.qi', 'gao.yan', 'xu.ning', 'ma.chao']
const now = Math.floor(Date.UTC(2026, 9, 2) / 1000)
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

export default {
  path: '/admin/users',
  api(path, { query, scenario }) {
    if (path === '/api/v2/admin/users') {
      if (scenario === 'empty') return { code: 0, data: { list: [], total: 0 } }
      if (scenario === 'error') return { __status: 502, body: { msg: '上游服务没有响应' } }
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: { list: [], total: 0 } } }
      let list = USERS
      if (query.status === 'banned') list = list.filter(u => u.banned)
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
