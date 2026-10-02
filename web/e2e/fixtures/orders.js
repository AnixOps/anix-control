const EMAILS = ['lin.xiao', 'wang.fang', 'chen.jie', 'zhao.lei', 'sun.li', 'zhou.min', 'wu.hao', 'zheng.yu', 'feng.yi', 'he.ming']
const PERIODS = ['month', 'quarter', 'year', 'half_year']
const ORDERS = EMAILS.map((name, index) => ({
  id: 500 + index,
  trade_no: `2026100${index}1824${String(index * 7).padStart(4, '0')}`,
  user: { id: index + 1, email: `${name}@example.com` },
  plan: index % 4 === 3 ? null : { id: 1, name: ['标准 200G', '高级 1T', '基础 50G'][index % 3] },
  period: PERIODS[index % 4],
  type: (index % 3) + 1,
  total_amount: [1990, 5400, 19900, 990][index % 4],
  status: [0, 1, 3, 2, 3][index % 5],
  paid_at: index % 5 === 0 || index % 5 === 3 ? null : 1790000000 + index * 3600,
  callback_no: index % 5 === 1 ? `CB-2026-${index}` : '',
  created_at: new Date(Date.UTC(2026, 8, 20 + index, 8, 12)).toISOString()
}))
export default {
  edition: 'commercial',
  path: '/admin/orders',
  viewportOnly: ['sheet', 'menu'],
  api(path, { query, scenario }) {
    if (path === '/api/v2/admin/orders/stats') return { code: 0, data: { total_orders: 1284, pending_orders: 17, total_revenue: 8452300, today_revenue: 59700 } }
    if (path === '/api/v2/admin/orders') {
      if (scenario === 'empty') return { code: 0, data: { list: [], total: 0 } }
      if (scenario === 'error') return { __status: 502, body: { msg: '上游服务没有响应' } }
      const list = query.status ? ORDERS.filter(o => String(o.status) === query.status) : ORDERS
      return { code: 0, data: { list, total: query.status ? list.length : 1284 } }
    }
    return undefined
  },
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    menu: async (page) => { await page.getByRole('button', { name: /的操作$/ }).first().click() },
    sheet: async (page, { width }) => {
      if (width < 640) await page.getByRole('button', { name: ORDERS[0].trade_no, exact: true }).click()
      else await page.getByRole('row', { name: new RegExp(ORDERS[0].trade_no) }).click()
    }
  }
}
