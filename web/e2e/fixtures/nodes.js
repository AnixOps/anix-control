import { NODES, envelope } from './nodeData.js'

export default {
  path: '/admin/nodes',
  edition: 'community',
  pathFor: scenario => (scenario === 'filtered' ? '/admin/nodes?status=online&q=hk' : '/admin/nodes'),
  api(path, { query, scenario, method }) {
    if (path === '/api/v2/admin/nodes') {
      if (scenario === 'empty') return envelope({ list: [], total: 0 })
      if (scenario === 'error') return { __status: 502, body: { message: '上游服务没有响应' } }
      if (scenario === 'loading') return { __delay: 60000, body: envelope({ list: [], total: 0 }) }
      let list = NODES
      if (query.status) list = list.filter(n => n.status === Number(query.status))
      if (query.search) list = list.filter(n => n.name.includes(query.search) || n.host.includes(query.search))
      return envelope({ list, total: list.length === NODES.length ? 47 : list.length })
    }
    if (path === '/api/v2/admin/nodes/stats') return envelope({ total: 47, online: 38, offline: 6, pending: 3 })
    if (path === '/api/v2/admin/auth-keys' && method === 'POST') return envelope({ id: 4, key: 'ak_live_3f9d2c71b0e84a5f9c6d1e2a7b8c4d0f' })
    if (path === '/api/v2/admin/auth-keys') {
      if (scenario === 'authkeyFresh') return envelope([])
      return envelope([{ id: 3, name: 'Panel key 2026-09-01', key: '********', used: 12 }])
    }
    if (/\/api\/v2\/admin\/nodes\/\d+\/credentials/.test(path)) return envelope({ api_key: 'nk_4b1f0c9e2d7a6b5c8e3f1a0d9c7b6e5a' })
    return undefined
  },
  viewportOnly: ['menu', 'form', 'authkey', 'authkeyFresh', 'deploy', 'formErrors'],
  scenarios: {
    list: async () => {},
    filtered: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    menu: async (page) => {
      await page.getByRole('button', { name: /hk-01 的操作|Actions for hk-01/ }).first().click()
    },
    form: async (page) => {
      await page.getByRole('button', { name: /^添加节点$|^Add node$/ }).first().click()
    },
    formErrors: async (page) => {
      await page.getByRole('button', { name: /^添加节点$|^Add node$/ }).first().click()
      await page.getByRole('dialog').getByRole('button', { name: /^添加节点$|^Add node$/ }).click()
    },
    authkey: async (page) => {
      await page.getByTestId('open-auth-key').click()
    },
    authkeyFresh: async (page) => {
      await page.getByTestId('open-auth-key').click()
      await page.getByTestId('generate-auth-key').click()
    },
    deploy: async (page) => {
      await page.getByTestId('open-deploy').click()
    }
  }
}
