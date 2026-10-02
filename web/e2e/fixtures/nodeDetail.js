import { LOGS, NODES, PROTOCOLS, TEMPLATES, envelope } from './nodeData.js'

const SECTION = {
  overview: '', overviewChild: '', protocols: 'protocols', protocolEdit: 'protocols', protocolNew: 'protocols', protocolVisual: 'protocols',
  protocolsEmpty: 'protocols', credentials: 'credentials', credentialsShown: 'credentials', deploy: 'deploy', deployHelper: 'deploy',
  logs: 'logs', logsEmpty: 'logs', logsError: 'logs', danger: 'danger', disableConfirm: 'danger', deleteConfirm: 'danger', edit: '',
  notFound: '', error: '', loading: ''
}

export default {
  edition: 'community',
  pathFor: (scenario) => {
    const id = scenario === 'overviewChild' ? 110 : scenario === 'notFound' ? 999 : 108
    const section = SECTION[scenario]
    return `/admin/nodes/${id}${section ? `?section=${section}` : ''}`
  },
  api(path, { scenario, method }) {
    let m = path.match(/^\/api\/v2\/admin\/nodes\/(\d+)$/)
    if (m && method === 'GET') {
      if (scenario === 'notFound') return { __status: 404, body: { message: '节点不存在' } }
      if (scenario === 'error') return { __status: 502, body: { message: '上游服务没有响应' } }
      if (scenario === 'loading') return { __delay: 60000, body: envelope(NODES[7]) }
      const node = NODES.find(n => n.id === Number(m[1]))
      return envelope({ ...node, status: node.status === 1 ? 2 : node.status })
    }
    if (path === '/api/v2/admin/nodes') return envelope({ list: NODES, total: NODES.length })
    m = path.match(/^\/api\/v2\/admin\/nodes\/(\d+)\/protocols$/)
    if (m) return envelope(scenario === 'protocolsEmpty' ? [] : PROTOCOLS)
    if (path === '/api/v2/admin/protocol-templates') return envelope(TEMPLATES)
    if (/\/logs$/.test(path)) {
      if (scenario === 'logsEmpty') return envelope({ list: [], total: 0 })
      if (scenario === 'logsError') return { __status: 500, body: { message: 'log store unavailable' } }
      return envelope({ list: LOGS, total: 86 })
    }
    if (/\/credentials$/.test(path)) return envelope({ node_id: 108, api_key: 'nk_4b1f0c9e2d7a6b5c8e3f1a0d9c7b6e5a', secret: 'never-shown' })
    if (path === '/api/v2/admin/auth-keys') return envelope([{ id: 3, name: 'Panel key', key: '********', used: 12 }])
    return undefined
  },
  viewportOnly: ['protocolEdit', 'protocolNew', 'protocolVisual', 'deployHelper', 'disableConfirm', 'deleteConfirm', 'edit'],
  scenarios: {
    overview: async () => {},
    overviewChild: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    error: async () => {},
    notFound: async () => {},
    protocols: async () => {},
    protocolsEmpty: async () => {},
    protocolEdit: async (page, { width }) => {
      if (width < 640) await page.getByRole('button', { name: 'VLESS', exact: true }).click()
      else await page.getByRole('row', { name: /VLESS/ }).first().click()
    },
    protocolNew: async (page) => {
      await page.getByTestId('add-protocol').click()
    },
    protocolVisual: async (page, { width }) => {
      if (width < 640) await page.getByRole('button', { name: 'WIREGUARD', exact: true }).click()
      else await page.getByRole('row', { name: /WIREGUARD/ }).first().click()
      await page.getByRole('dialog').getByRole('button', { name: /^表单$|^Form$/ }).click()
    },
    credentials: async () => {},
    credentialsShown: async (page) => {
      await page.getByTestId('reveal-api-key').click()
    },
    deploy: async () => {},
    deployHelper: async (page) => {
      await page.getByTestId('open-deploy-helper').click()
    },
    logs: async () => {},
    logsEmpty: async () => {},
    logsError: async () => {},
    danger: async () => {},
    disableConfirm: async (page) => {
      await page.getByTestId('disable-node').click()
    },
    deleteConfirm: async (page) => {
      await page.getByTestId('delete-node').click()
      await page.getByRole('alertdialog').getByRole('textbox').fill('tw-0')
    },
    edit: async (page) => {
      await page.getByTestId('edit-node').click()
    }
  }
}
