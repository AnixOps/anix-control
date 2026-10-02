const COUPONS = [
  { id: 1, code: 'SUMMER2026', name: '夏季活动', type: 1, value: 20, use_count: 182, limit_use: 500, started_at: 1782864000, ended_at: 1788134400 },
  { id: 2, code: 'NEWUSER', name: '新用户立减', type: 2, value: 1000, use_count: 931, limit_use: -1, started_at: 1767225600, ended_at: 1798761600 },
  { id: 3, code: 'BLACKFRIDAY', name: '黑五', type: 1, value: 50, use_count: 98, limit_use: 100, started_at: 1795795200, ended_at: 1796140800 },
  { id: 4, code: 'VIP5', name: 'VIP 回馈', type: 2, value: 500, use_count: 12, limit_use: 200, started_at: 1782864000, ended_at: 1793491200 }
]
export default {
  edition: 'commercial',
  path: '/admin/coupons',
  viewportOnly: ['dialog', 'menu'],
  api(path, { scenario }) {
    if (path === '/api/v2/admin/coupon') {
      if (scenario === 'empty') return { code: 0, data: [] }
      if (scenario === 'error') return { __status: 500, body: { msg: '数据库不可用' } }
      return { code: 0, data: COUPONS }
    }
    return undefined
  },
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    menu: async (page) => { await page.getByRole('button', { name: /SUMMER2026 的操作/ }).click() },
    dialog: async (page) => {
      await page.getByRole('button', { name: '新建优惠券' }).first().click()
      await page.getByRole('dialog').getByRole('button', { name: '创建优惠券' }).click()
    }
  }
}
