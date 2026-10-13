import { FIXED_NOW_MS } from './clock.js'
import { NODES, envelope } from './nodeData.js'
import { serviceStatusAnswer } from './serviceStatus.js'
import { transportsAnswer } from './transports.js'

// The node list (GET /api/v4/admin/nodes: the /api/v2 rows with approval and
// node group), the node groups behind the join command, and the answers of
// the join dialog. Node ids are those of nodeData.js (101 to 112): the first
// five joined the group "HK", the next five "JP", the rest were added by
// hand, and au-syd-01 (112) waits for approval in "HK".
const DAY = 86_400_000
const iso = offset => new Date(FIXED_NOW_MS + offset).toISOString()

const PLACEMENT = id => {
  if (id === 112) return { approval: 'pending', node_group_id: 1, node_group_name: 'HK' }
  if (id <= 105) return { approval: 'approved', node_group_id: 1, node_group_name: 'HK' }
  if (id <= 110) return { approval: 'approved', node_group_id: 2, node_group_name: 'JP' }
  return { approval: 'approved', node_group_id: null, node_group_name: '' }
}
const ROWS = NODES.map(node => ({ ...node, ...PLACEMENT(node.id) }))

const REALITY = {
  name: 'VLESS Reality', type: 'vless', port_range: [20000, 40000], transport: 'tcp', tls: 'reality',
  reality: { server_name: 'www.microsoft.com', dest: 'www.microsoft.com:443', fingerprint: 'chrome' }, settings: { flow: 'xtls-rprx-vision' }
}
const SHADOWSOCKS = {
  name: 'Shadowsocks 2022', type: 'shadowsocks', port_range: [20000, 40000], transport: 'tcp', tls: 'none',
  settings: { method: '2022-blake3-aes-128-gcm' }
}

const GROUPS = [
  {
    id: 1, name: 'HK', kind: 'proxy', max_nodes: 20, require_approval: true, rate: 1.5, tags: ['HK', 'IEPL'],
    monthly_limit_bytes: 2000 * 1024 ** 3, reset_day: 1, sort: 0, protocol_template: [REALITY, SHADOWSOCKS],
    subscription_group_ids: [1], token_reset_at: iso(-20 * DAY), token_expires_at: null, token_expired: false,
    nodes: { live: 6, pending: 1 }, created_at: iso(-40 * DAY), updated_at: iso(-3 * DAY)
  },
  {
    id: 2, name: 'JP', kind: 'proxy', max_nodes: 0, require_approval: false, rate: 1, tags: ['JP'],
    monthly_limit_bytes: 0, reset_day: 1, sort: 1, protocol_template: [REALITY],
    subscription_group_ids: [1], token_reset_at: iso(-9 * DAY), token_expires_at: iso(21 * DAY), token_expired: false,
    nodes: { live: 5, pending: 0 }, created_at: iso(-30 * DAY), updated_at: iso(-9 * DAY)
  },
  {
    id: 3, name: 'edge-relays', kind: 'forward', max_nodes: 0, require_approval: false, rate: 1, tags: [],
    monthly_limit_bytes: 0, reset_day: 1, sort: 2, protocol_template: [], subscription_group_ids: [],
    token_reset_at: iso(-60 * DAY), token_expires_at: iso(-2 * DAY), token_expired: true,
    nodes: { live: 2, pending: 0 }, created_at: iso(-70 * DAY), updated_at: iso(-60 * DAY)
  }
]

const SUBSCRIPTION_GROUPS = [
  { id: 1, name: 'default', description: '默认订阅分组', enable: 1, priority: 0 },
  { id: 2, name: 'vip', description: 'VIP', enable: 1, priority: 10 }
]

const TOKEN = 'anixjoin_8Kq2mZ0vR4sXn1fL7aJbC9dE3gHtUwYpQ6oIeNkMxVc'
const command = id => ({
  group: GROUPS.find(group => group.id === id) || GROUPS[0],
  token: TOKEN,
  token_expires_at: (GROUPS.find(group => group.id === id) || GROUPS[0]).token_expires_at,
  agent_version: 'v4.2.0',
  commands: [
    { mirror: 'control', available: true, command: `curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- -u https://panel.example.com -t ${TOKEN}` },
    { mirror: 'cn', available: false, note: 'agent_install.cn_mirror_url is not set', command: `curl -fsSL https://panel.example.com/install.sh | sudo bash -s -- -u https://panel.example.com -t ${TOKEN} --mirror cn` },
    { mirror: 'github', available: true, command: `curl -fsSL https://github.com/AnixOps/anix-control/releases/download/v4.2.0/agent-install.sh | sudo bash -s -- -u https://panel.example.com -t ${TOKEN} --mirror github` }
  ],
  uninstall_command: 'sudo anix-agent uninstall',
  script: { url: 'https://panel.example.com/install.sh', signature_url: 'https://panel.example.com/install.sh.sig', signed: true }
})

const READY = {
  ready: true,
  checks: [
    { id: 'public_url', ok: true, level: 'error', message: 'Nodes reach Control at https://panel.example.com.', fix: '' },
    { id: 'grpc', ok: true, level: 'error', message: 'Nodes dial the gRPC listener at panel.example.com:50051 over TLS.', fix: '' },
    { id: 'ca', ok: true, level: 'error', message: 'The built-in agent CA is on; join tokens are sealed with module_runtime.ca_kek.', fix: '' },
    { id: 'mtls_mode', ok: true, level: 'error', message: 'agent_control.mtls is required: nodes join and connect with their certificate.', fix: '' },
    { id: 'agent_release', ok: true, level: 'error', message: 'Agent v4.2.0 is downloaded from GitHub releases.', fix: '' },
    { id: 'script_signature', ok: true, level: 'warning', message: 'The install script\'s release signature verifies; it is served at /install.sh.sig.', fix: '' }
  ]
}
const BLOCKED = {
  ready: false,
  checks: [
    { id: 'public_url', ok: false, level: 'error', message: 'Nodes would use http://10.0.0.5:8080, which is not https: the install script and the join refuse plain http.', fix: 'Set agent_install.public_url to the https address nodes reach Control at.' },
    { id: 'grpc', ok: true, level: 'error', message: 'Nodes dial the gRPC listener at 10.0.0.5:50051 over TLS.', fix: '' },
    { id: 'ca', ok: false, level: 'error', message: 'module_runtime.ca_kek is not set: node certificates are signed by the built-in CA, and group join tokens are sealed with its key.', fix: 'Set module_runtime.ca_kek.' },
    { id: 'script_signature', ok: false, level: 'warning', message: 'The configured install script signature does not verify: no valid install script signature: the signature does not match the script', fix: 'Point agent_install.signature_file at the agent-install.sh.sig release asset.' }
  ]
}

// The machines of the group "HK": none yet while waiting, or two that joined.
const JOINED = [
  { id: 112, node: 'proxy-112', kind: 'proxy', name: 'au-syd-01', host: 'syd1.edge.example.net', approval: 'pending', online: true, last_seen: iso(-20_000), agent_version: 'v4.2.0', created_at: iso(-2 * 60_000) },
  { id: 101, node: 'proxy-101', kind: 'proxy', name: 'hk-01', host: 'hk1.edge.example.net', approval: 'approved', online: true, last_seen: iso(-45_000), agent_version: 'v4.2.0', created_at: iso(-3 * DAY) }
]

// What each scenario does once the page has loaded.
const INTERACTIONS = {
  list: async () => {},
  filtered: async () => {},
  awaiting: async () => {},
  empty: async () => {},
  error: async () => {},
  loading: async page => { await page.waitForTimeout(500) },
  menu: async (page) => {
    await page.getByRole('button', { name: /hk-01 的操作|Actions for hk-01/ }).first().click()
  },
  // The edit sheet of a node (nodes are no longer added by hand).
  form: async (page) => {
    await page.getByRole('button', { name: /hk-01 的操作|Actions for hk-01/ }).first().click()
    await page.getByRole('menuitem', { name: /^编辑$|^Edit$/ }).click()
  },
  formErrors: async (page) => {
    await page.getByRole('button', { name: /hk-01 的操作|Actions for hk-01/ }).first().click()
    await page.getByRole('menuitem', { name: /^编辑$|^Edit$/ }).click()
    await page.getByRole('dialog').getByRole('textbox', { name: /名称|Name/ }).fill('')
    await page.getByRole('dialog').getByRole('button', { name: /保存更改|Save changes/ }).click()
  },
  // 接入节点: waiting for the first machine, with two machines joined (one
  // waits for approval), and a Control that cannot hand out a command yet.
  join: async (page) => {
    await page.getByTestId('join-node').click()
    await page.getByTestId('join-command-text').waitFor()
    await page.getByTestId('join-waiting').waitFor()
  },
  joined: async (page) => {
    await page.getByTestId('join-node').click()
    await page.getByTestId('join-command-text').waitFor()
    await page.getByTestId('join-nodes').getByText(/au-syd-01 joined|au-syd-01 已接入/).waitFor()
  },
  joinBlocked: async (page) => {
    await page.getByTestId('join-node').click()
    await page.getByTestId('join-blocked').waitFor()
  },
  groups: async () => {},
  groupsEmpty: async () => {},
  groupEdit: async (page) => {
    await page.getByRole('button', { name: /^HK 的操作$|^Actions for HK$/ }).click()
    await page.getByRole('menuitem', { name: /^编辑$|^Edit$/ }).click()
    await page.getByTestId('node-group-sheet').waitFor()
  }
}

export default {
  path: '/admin/nodes',
  edition: 'community',
  pathFor: (scenario) => {
    if (scenario === 'filtered') return '/admin/nodes?status=online&q=hk'
    if (scenario === 'awaiting') return '/admin/nodes?status=awaiting'
    if (scenario.startsWith('group')) return '/admin/nodes?tab=groups'
    return '/admin/nodes'
  },
  api(path, { query, scenario, method }) {
    if (path === '/api/v4/admin/nodes') {
      if (scenario === 'empty') return { data: { list: [], total: 0 } }
      if (scenario === 'error') return { __status: 502, body: { error: { code: 'upstream', message: '上游服务没有响应' } } }
      if (scenario === 'loading') return { __delay: 60000, body: { data: { list: [], total: 0 } } }
      let list = ROWS
      if (query.status) list = list.filter(n => n.status === Number(query.status))
      if (query.approval) list = list.filter(n => n.approval === query.approval)
      if (query.search) list = list.filter(n => n.name.includes(query.search) || n.host.includes(query.search))
      // sort and order, as GET /admin/nodes takes them (whitelist of the API).
      if (query.sort) {
        const column = { id: 'id', name: 'name', host: 'host', cpu_usage: 'cpu_usage', last_check_at: 'last_check_at' }[query.sort]
        const factor = query.order === 'desc' ? -1 : 1
        if (column) list = [...list].sort((a, b) => factor * (a[column] === b[column] ? 0 : (a[column] ?? Infinity) < (b[column] ?? Infinity) ? -1 : 1))
      }
      return { data: { list, total: list.length === ROWS.length ? 47 : list.length } }
    }
    // The Agent connection, certificate and version of the nodes in view (one call per page).
    if (path === '/api/v4/kernel/agents/transports') {
      return transportsAnswer(query)
    }
    // Their systemd services summary (the machine-telemetry package, one call).
    if (path === '/api/v3/plugins/machine-telemetry/nodes/services') {
      return serviceStatusAnswer(query)
    }
    if (path === '/api/v2/admin/nodes/stats') return envelope({ total: 47, online: 38, offline: 6, pending: 3 })
    // 节点组 and the join dialog.
    if (path === '/api/v4/kernel/agents/install-readiness') return { data: scenario === 'joinBlocked' ? BLOCKED : READY }
    if (path === '/api/v4/kernel/node-groups' && method === 'GET') return { data: { groups: scenario === 'groupsEmpty' ? [] : GROUPS } }
    if (path === '/api/v4/kernel/node-groups/default') return { data: { group: GROUPS[0], created: false } }
    let m = path.match(/^\/api\/v4\/kernel\/node-groups\/(\d+)\/command$/)
    if (m) return { data: command(Number(m[1])) }
    m = path.match(/^\/api\/v4\/kernel\/node-groups\/(\d+)\/nodes$/)
    if (m) return { data: { nodes: scenario === 'joined' ? JOINED : [] } }
    if (path === '/api/v2/admin/subscription/groups') return envelope(SUBSCRIPTION_GROUPS)
    return undefined
  },
  viewportOnly: ['menu', 'form', 'formErrors', 'join', 'joined', 'joinBlocked', 'groupEdit'],
  scenarios: INTERACTIONS,
  // What a scenario does after the page has loaded (a menu, a sheet).
  async after(page, { scenario }) {
    await INTERACTIONS[scenario]?.(page)
  }
}
