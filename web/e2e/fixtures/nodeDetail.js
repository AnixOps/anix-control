import { FIXED_NOW_MS } from './clock.js'
import { LOGS, NODES, PROTOCOLS, TEMPLATES, envelope } from './nodeData.js'
import { nodeTrafficAnswer, rotateAnswer } from './nodeTraffic.js'
import { transportsAnswer } from './transports.js'

// 服务: node 108 runs machine-telemetry 4.1.0, whose release declares the
// services table, and reported it two minutes ago.
const SERVICES_RELEASE = {
  id: 41, plugin_id: 'machine-telemetry', version: '4.1.0', api_version: 'v2',
  manifest: JSON.stringify({ id: 'machine-telemetry', version: '4.1.0', capabilities: ['telemetry.read', 'telemetry.systemd.read'] })
}
const SERVICE_UNITS = [
  ['nginx.service', 'active', 'running', 3.42, 18.5, 87_031_808, 121_634_816],
  ['xray.service', 'active', 'running', 11.8, 46.25, 214_958_080, 268_435_456],
  ['anix-agent.service', 'active', 'running', 0.9, 4.1, 31_457_280, 39_845_888],
  ['ssh.service', 'active', 'running', 0.05, 1.2, 6_291_456, 9_437_184],
  ['cron.service', 'active', 'running', 0, 0.3, 2_097_152, 3_145_728],
  ['systemd-journald.service', 'active', 'running', 0.4, 2.6, 25_165_824, 41_943_040],
  ['backup-offsite.service', 'failed', 'failed', 0, 22.4, 0, 402_653_184],
  ['certbot.service', 'inactive', 'dead', 0, 7.5, 0, 58_720_256],
  ['unattended-upgrades.service', 'inactive', 'dead', 0, 0, 0, 0]
].map(([name, active, sub, cpuAvg, cpuPeak, memory, memoryPeak]) => ({
  name, active_state: active, sub_state: sub, cpu_avg_percent: cpuAvg, cpu_peak_percent: cpuPeak, memory_bytes: memory, memory_peak_bytes: memoryPeak
}))
const SERVICES = {
  node_id: 108, enabled: true, include: [], exclude: ['user-*.service'], reported: true, supported: true, unsupported_reason: '',
  stale: false, observed_at: new Date(FIXED_NOW_MS - 2 * 60_000).toISOString(), version: '4.1.0', window_seconds: 600,
  summary: { total: 9, failed: 1, active: 6, inactive: 2 },
  units: SERVICE_UNITS
}

const SECTION = {
  overview: '', overviewChild: '', protocols: 'protocols', protocolEdit: 'protocols', protocolNew: 'protocols', protocolVisual: 'protocols',
  protocolsEmpty: 'protocols', credentialsShown: 'credentials', deploy: 'deploy', deployHelper: 'deploy',
  logs: 'logs', logsEmpty: 'logs', logsError: 'logs', services: 'services', servicesDisabled: 'services', danger: 'danger', disableConfirm: 'danger', deleteConfirm: 'danger', edit: '',
  notFound: '', error: '', loading: '',
  // 流量 (the node's traffic over time) and 轮换 Agent 凭据 (in 凭据).
  traffic: 'traffic', trafficEmpty: 'traffic', trafficError: 'traffic',
  credentials: 'credentials', rotate: 'credentials', rotated: 'credentials'
}

export default {
  edition: 'community',
  pathFor: (scenario) => {
    const id = scenario === 'overviewChild' ? 110 : scenario === 'notFound' ? 999 : 108
    const section = SECTION[scenario]
    return `/admin/nodes/${id}${section ? `?section=${section}` : ''}`
  },
  api(path, { query, scenario, method, body, now }) {
    // The node's traffic over time (流量) and the credential rotation (凭据).
    let m = path.match(/^\/api\/v4\/kernel\/nodes\/(\d+)\/traffic$/)
    if (m) return nodeTrafficAnswer(m[1], query, { trafficEmpty: 'empty', trafficError: 'error' }[scenario] || 'traffic')
    if (path === '/api/v4/kernel/agents/rotate-credentials' && method === 'POST') return rotateAnswer(body, now)
    // The node's Agent connection and certificate (?node=proxy-108).
    if (path === '/api/v4/kernel/agents/transports') {
      return transportsAnswer(query)
    }
    m = path.match(/^\/api\/v2\/admin\/nodes\/(\d+)$/)
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
    if (/^\/api\/v3\/nodes\/\d+\/assignments$/.test(path) && scenario.startsWith('services')) {
      return { data: [{ id: 9, node_id: 108, service_scope: 'default', plugin_id: 'machine-telemetry', role: 'agent', desired_version: '4.1.0', enabled: true }] }
    }
    if (path === '/api/v3/plugin-releases' && scenario.startsWith('services')) return { data: [SERVICES_RELEASE] }
    if (path === '/api/v3/plugins/machine-telemetry/nodes/108/services') {
      if (scenario === 'servicesDisabled') return { data: { ...SERVICES, enabled: false, reported: false, observed_at: null, units: [], summary: { total: 0, failed: 0, active: 0, inactive: 0 } } }
      return { data: SERVICES }
    }
    if (path === '/api/v2/admin/auth-keys') return envelope([{ id: 3, name: 'Panel key', key: '********', used: 12 }])
    return undefined
  },
  // 轮换 Agent 凭据: `rotate` stops at the confirmation, `rotated` goes on to
  // the result dialog with the one-time credential.
  async after(page, { scenario }) {
    if (scenario !== 'rotate' && scenario !== 'rotated') return
    await page.getByTestId('rotate-credentials').click()
    await page.getByRole('alertdialog').waitFor()
    if (scenario === 'rotate') return
    await page.getByRole('alertdialog').getByRole('button', { name: 'Rotate credentials' }).click()
    await page.getByTestId('rotate-result').waitFor()
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
    services: async () => {},
    servicesDisabled: async () => {},
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
