const CODES = Array.from({ length: 14 }, (_, i) => ({
  id: 200 - i,
  code: ['a1b2c3d4', 'e5f6a7b8', 'c9d0e1f2', '7f3k9q2m', 'x4v2c6b8', 'm2n4p6q8', 'r3s5t7u9', 'h8j6k4l2', 'w1y3z5a7', 'b9d7f5g3', 'q2w4e6r8', 't1y3u5i7', 'o9p7a5s3', 'd2f4g6h8'][i],
  user_id: i % 3 === 1 ? 40 + i : null,
  status: i % 4 === 1 ? 1 : 0,
  used_by: i % 4 === 1 ? 300 + i : null,
  expired_at: i % 5 === 3 ? '2026-08-01T00:00:00Z' : (i % 2 ? '2026-12-31T00:00:00Z' : null),
  created_at: new Date(Date.UTC(2026, 8, 30 - i, 10)).toISOString()
}))
export default {
  path: '/admin/invite-codes',
  edition: 'commercial',
  api(path, { scenario, method, body }) {
    // One request revokes the selection, answered per code.
    if (path === '/api/v4/admin/invite-codes/bulk' && method === 'POST') {
      const ids = body?.ids || []
      const results = ids.map(id => ({ id, ok: true }))
      return { data: { action: body?.action, requested: ids.length, succeeded: ids.length, failed: 0, results } }
    }
    if (path === '/api/v2/admin/invite/codes' && method === 'GET') {
      if (scenario === 'empty') return { code: 0, data: { list: [], total: 0 } }
      if (scenario === 'error') return { __status: 502, body: { msg: '上游服务没有响应' } }
      return { code: 0, data: { list: CODES, total: 46 } }
    }
    if (path === '/api/v2/admin/invite/codes' && method === 'POST') return { code: 0, data: { codes: CODES.slice(0, 5).map(c => ({ id: c.id + 1000, code: c.code.split('').reverse().join('') })) } }
    return undefined
  },
  viewportOnly: ['selected', 'generate', 'generated'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    selected: async (page) => {
      const boxes = page.getByRole('checkbox', { name: /^选择 / })
      await boxes.nth(0).click()
      await boxes.nth(2).click()
    },
    generate: async (page) => { await page.locator('[data-testid="open-generate"]').click() },
    generated: async (page) => {
      await page.locator('[data-testid="open-generate"]').click()
      await page.locator('#invite-code-count').fill('5')
      await page.locator('[data-testid="generate-invite-codes"]').click()
    }
  }
}
