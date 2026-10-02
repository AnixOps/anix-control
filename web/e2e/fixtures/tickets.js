const base = Math.floor(Date.UTC(2026, 9, 2, 8) / 1000)
const SUBJECTS = ['香港节点晚高峰很慢', '订阅链接导入 Clash 失败', '能否开通发票？', '流量重置日期不对', 'iOS 客户端无法连接', '想升级到高级套餐', '日本节点延迟突然变高', '账户被误封']
const TICKETS = SUBJECTS.map((subject, index) => ({
  id: 120 + index, user_id: 300 + index * 7, subject, level: index % 3, status: index % 4 === 3 ? 2 : (index % 3 === 1 ? 1 : 0),
  created_at: base - index * 5400 - 86400, updated_at: base - index * 3600
}))
export default {
  path: '/admin/tickets',
  api(path, { scenario }) {
    if (path === '/api/v2/admin/ticket') {
      if (scenario === 'empty') return { code: 0, data: [] }
      if (scenario === 'error') return { __status: 502, body: { msg: '上游服务没有响应' } }
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: [] } }
      return { code: 0, data: TICKETS }
    }
    return undefined
  },
  viewportOnly: ['conversation', 'quick'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    conversation: async (page) => {
      await page.locator('[data-ticket-id="120"]').click()
    },
    quick: async (page) => {
      await page.locator('[data-ticket-id="121"]').click()
      await page.locator('[data-quick-reply="details"]').click()
      await page.locator('[data-test="ticket-reply-submit"]').scrollIntoViewIfNeeded()
    }
  }
}
