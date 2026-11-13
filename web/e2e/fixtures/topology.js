// 网络拓扑 (/admin/topology): sixteen devices from Shanghai to Frankfurt,
// seven forward routes between them (one through an offline exit, one with
// two entries) and an egress route. The scenario 'direct' has the devices
// and no line; 'empty' no device. The scenarios 'lines' and 'lines-empty'
// open 线路 (/admin/lines) on the same network.
import { FIXED_NOW_MS } from './clock.js'
import { EXTENSION } from './forwardV4.js'

const NOW = FIXED_NOW_MS
const SECONDS = Math.floor(NOW / 1000)
const GB = 1024 ** 3

// [name, tags, status (1 online, 2 offline, 0 pending), users]
const DEVICES = [
  ['sha-entry-01', 'CN2', 1, 0], ['sha-entry-02', '', 1, 0], ['gz-entry-01', '', 1, 0], ['hk-relay-01', 'IEPL', 1, 0],
  ['hk-01', 'HK', 1, 64], ['tyo-01', 'JP,Premium', 1, 128], ['tyo-02', 'JP', 1, 87], ['osaka-01', 'JP', 2, 0],
  ['sg-01', 'SG', 1, 41], ['sel-01', 'KR', 1, 23], ['lax-01', 'US', 1, 52], ['sjc-01', 'US', 1, 17],
  ['fra-01', 'DE', 1, 12], ['lon-01', 'UK', 1, 9], ['syd-01', 'AU', 0, 0], ['edge-864', '', 1, 3]
]
const NODES = DEVICES.map(([name, tags, status, users], index) => ({
  id: index + 1, name, host: `203.0.113.${index + 10}`, tags: tags || null, status, online_users: users,
  last_check_at: status === 1 ? SECONDS - 20 : status === 2 ? SECONDS - 7200 : null, approval: 'approved'
}))

// [name, hops (device ids of each), traffic of the last 24 hours in GB]
const LINES = [
  ['sha-tyo', [[1], [6]], 412], ['sha-hk-tyo', [[1, 2], [4], [7]], 286], ['gz-hk', [[3], [5]], 198], ['gz-hk-sg', [[3], [4], [9]], 121],
  ['sha-tyo-lax', [[2], [6], [11]], 74], ['sha-osaka', [[1], [8]], 0], ['sha-fra', [[2], [13]], 38]
]
const ROUTES = LINES.map(([name, hops], index) => ({
  route: { id: `01JBTOPO${String(index).padStart(18, '0')}`, name, hops: hops.map(ids => ({ node_refs: ids.map(id => `proxy-${id}`) })) }
}))
const FORWARD_NODES = NODES.filter(node => node.status === 1).map(node => ({
  node_ref: `proxy-${node.id}`, kind: 'proxy', name: node.name, host: node.host, enabled: true, reported: true,
  desired_generation: '4', reported_generation: '4', reported_at_unix_ms: String(NOW - 20_000)
}))
const TOTALS = LINES.map(([, , gb], index) => ({ route_id: ROUTES[index].route.id, hop_index: 0, up_bytes: String(gb * GB * 0.2), down_bytes: String(gb * GB * 0.8) }))
const EGRESS = [{
  id: 1, name: 'ai-via-jp', enabled: true, domains: ['openai.com'], relay_port: 24001,
  source_node: { id: 5, name: 'hk-01', exists: true }, exit_node: { id: 6, name: 'tyo-01', exists: true },
  source_state: { state: 'applied' }, exit_state: { state: 'applied' }
}]

// The network as lines: a line per route (the first hop of the Guangzhou
// routes forwards in the kernel), a direct line per exit device, a port
// forward to a fixed target, and one route that cannot be read as a line.
const PROTOCOLS = ['vless', 'anx', 'hysteria2', 'vless', 'trojan', 'vless', 'vless']
const ref = id => `proxy-${id}`
const NETWORK = {
  devices: NODES.map(node => ({ ref: ref(node.id), name: node.name, host: node.host, kernel: node.id === 3, ...(node.id === 15 ? { region: 'au' } : {}) })),
  uplinks: [], entries: [], transports: [], exits: [],
  lines: [
    ...LINES.map(([name, hops], index) => ({
      id: ROUTES[index].route.id, name, enabled: true, show: true, port: 443 + index * 1000, l4: ['tcp'], rate: index === 1 ? 1.5 : 1, revision: 3,
      entries: hops[0].map(ref), chain: hops.map(ids => ({ devices: ids.map(ref) })), exit: ref(hops.at(-1)[0]),
      protocol: { id: index + 1, type: PROTOCOLS[index], device: ref(hops.at(-1)[0]), port: 443 },
      // sha-hk-tyo passes the user's address on and has limits (the edit
      // sheet opens its advanced part).
      ...(index === 1 ? { policy: { proxy_protocol: 'v2', bandwidth_bps: 200_000_000 }, quota_bytes: 500 * GB } : {}),
      ...(index === 2 ? { rules: [{ name: 'ai-via-jp', domains: ['openai.com', 'anthropic.com'], exit: 'proxy-6', enabled: true }] } : {})
    })),
    ...[6, 11, 14].map(id => ({
      id: `protocol-${id + 20}`, name: `${NODES[id - 1].name} direct`, enabled: true, show: id !== 14, port: 8443, l4: ['tcp', 'udp'], rate: 1,
      entries: [ref(id)], chain: [{ devices: [ref(id)] }], exit: ref(id), protocol: { id: id + 20, type: 'hysteria2', device: ref(id), port: 8443 }
    })),
    {
      id: '01JBTOPOFIXED0000000000000', name: 'game-forward', enabled: true, show: false, port: 30443, l4: ['tcp', 'udp'], rate: 1, revision: 1,
      entries: ['proxy-2'], chain: [{ devices: ['proxy-2'] }, { devices: ['proxy-5'] }], exit: 'proxy-5', targets: [{ host: 'game.example.jp', port: 7777 }]
    }
  ],
  skipped: [{ id: '01JBTOPOSKIPPED00000000000', reason: 'hop 1 runs on ENGINE_GOST' }]
}

export default {
  edition: 'community',
  pathFor: scenario => (scenario.startsWith('lines') ? '/admin/lines' : scenario === 'graph' ? '/admin/topology?view=graph' : '/admin/topology'),
  // lines-edit opens a relayed line's edit sheet.
  async after(page, { scenario }) {
    if (scenario !== 'lines-edit') return
    await page.getByText('sha-hk-tyo', { exact: true }).first().click()
    await page.getByTestId('line-edit').click()
    await page.getByTestId('line-form-save').waitFor()
  },
  api(path, { scenario }) {
    const empty = scenario === 'empty' || scenario === 'lines-empty'
    const lines = scenario !== 'direct' && !empty
    if (path === '/api/v3/extensions') return [EXTENSION]
    if (path === '/api/v4/admin/nodes') return { data: { list: empty ? [] : NODES, total: NODES.length } }
    if (path === '/api/v4/kernel/network/lines') return { data: { snapshot: empty ? { ...NETWORK, devices: [], lines: [], skipped: [] } : NETWORK, forwards: 0, differences: [] } }
    if (path === '/api/v4/kernel/egress-routes') return { data: { routes: lines ? EGRESS : [] } }
    if (path === '/api/v4/forward/routes') return { data: { routes: lines ? ROUTES : [], can_delete: true } }
    if (path === '/api/v4/forward/nodes') return { data: { nodes: FORWARD_NODES, can_delete: true } }
    if (path === '/api/v4/forward/stats') return { data: { totals: lines ? TOTALS : [], nodes: [], series: [], truncated: false } }
    if (path === '/api/v4/forward/observability/targets') return { data: { targets: [], total: 0, truncated: false } }
    return undefined
  }
}
