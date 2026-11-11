// 网络拓扑 (/admin/topology): sixteen devices from Shanghai to Frankfurt,
// seven forward routes between them (one through an offline exit, one with
// two entries) and an egress route. The scenario 'direct' has the devices
// and no line; 'empty' no device.
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

export default {
  edition: 'community',
  pathFor: scenario => (scenario === 'graph' ? '/admin/topology?view=graph' : '/admin/topology'),
  api(path, { scenario }) {
    const lines = scenario !== 'direct' && scenario !== 'empty'
    if (path === '/api/v3/extensions') return [EXTENSION]
    if (path === '/api/v4/admin/nodes') return { data: { list: scenario === 'empty' ? [] : NODES, total: NODES.length } }
    if (path === '/api/v4/kernel/egress-routes') return { data: { routes: lines ? EGRESS : [] } }
    if (path === '/api/v4/forward/routes') return { data: { routes: lines ? ROUTES : [], can_delete: true } }
    if (path === '/api/v4/forward/nodes') return { data: { nodes: FORWARD_NODES, can_delete: true } }
    if (path === '/api/v4/forward/stats') return { data: { totals: lines ? TOTALS : [], nodes: [], series: [], truncated: false } }
    if (path === '/api/v4/forward/observability/targets') return { data: { targets: [], total: 0, truncated: false } }
    return undefined
  }
}
