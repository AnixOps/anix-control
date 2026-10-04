// F5b design mockups: mocked /api/v4/forward answers (docs/forwarding/v4-api.md).
// Shapes follow the protojson of sdk/api/forward/v1 verbatim: proto field
// names, enum names, 64-bit integers as strings. Nothing here calls a backend;
// every time derives from MOCK_NOW so screenshots are stable.

export const MOCK_NOW = Date.UTC(2026, 9, 4, 6, 0, 0) // 2026-10-04 14:00 CST
const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const GB = 1024 ** 3

export const ENGINES = {
  ENGINE_NFTABLES: { short: 'nft', label: 'nftables' },
  ENGINE_GOST: { short: 'gost', label: 'gost' },
  ENGINE_ANIXOPS: { short: 'anixops', label: 'anixops' }
}

export const SECURITIES = {
  LINK_SECURITY_RAW: 'RAW',
  LINK_SECURITY_TLS: 'TLS',
  LINK_SECURITY_WSS: 'WSS',
  LINK_SECURITY_QUIC: 'QUIC',
  LINK_SECURITY_GRPC: 'gRPC',
  LINK_SECURITY_ANIXOPS: 'AnixOps'
}

export const STRATEGIES = {
  BALANCE_STRATEGY_ROUND_ROBIN: { label: '轮询', hint: '按权重轮流' },
  BALANCE_STRATEGY_RANDOM: { label: '随机', hint: '按权重随机' },
  BALANCE_STRATEGY_IP_HASH: { label: 'IP 哈希', hint: '同一客户端固定到同一上游' },
  BALANCE_STRATEGY_LEAST_CONN: { label: '最少连接', hint: '每 10 秒按连接数重新加权（近似）' },
  BALANCE_STRATEGY_FAILOVER: { label: '主备', hint: '优先级最低的健康上游' }
}

export const DIRECT_MODES = {
  DIRECT_MODE_OFF: { label: '关闭', hint: '按跳链转发' },
  DIRECT_MODE_PREFERRED: { label: '优先直连', hint: '入口优先直连目标，目标不健康时走跳链' },
  DIRECT_MODE_FORCED: { label: '强制直连', hint: '只直连目标，不能有后续跳' }
}

// H21 defaults (sdk/forward/model/defaults.go).
export const H21_DEFAULTS = {
  interval_ms: 5000,
  timeout_ms: 2000,
  failure_threshold: 3,
  open_ms: 30000
}

const ALL_STRATEGIES = Object.keys(STRATEGIES)

function nftCaps(overrides = {}) {
  return {
    engine: 'ENGINE_NFTABLES',
    version: 'nftables 1.0.9',
    available: true,
    ipv6: true,
    udp: true,
    strategies: ALL_STRATEGIES,
    link_securities: ['LINK_SECURITY_RAW'],
    bandwidth_limit: true,
    quota: true,
    max_conns: true,
    ...overrides
  }
}

function gostCaps(overrides = {}) {
  return {
    engine: 'ENGINE_GOST',
    version: 'gost 3.0.0',
    available: true,
    ipv6: true,
    udp: true,
    strategies: ALL_STRATEGIES,
    link_securities: ['LINK_SECURITY_RAW', 'LINK_SECURITY_TLS', 'LINK_SECURITY_WSS', 'LINK_SECURITY_QUIC', 'LINK_SECURITY_GRPC'],
    bandwidth_limit: true,
    quota: true,
    max_conns: true,
    ...overrides
  }
}

function node({ ref, kind = 'forward', name, host, region, engines, labels = {}, range = [30000, 39999], reserved = [22, 80, 443], addresses, desired, reported, hopErrors = 0, hops, enabled = true, transport = 'NODE_TRANSPORT_AGENT', agent = '4.2.0', reportedAgo = 0.4 * MIN, online = true }) {
  const isReported = reported !== undefined
  return {
    node_ref: ref,
    kind,
    name,
    host,
    enabled,
    in_inventory: enabled,
    negotiated: transport === 'NODE_TRANSPORT_AGENT' && isReported,
    settings: { port_range: { first: range[0], last: range[1] }, reserved_ports: reserved, addresses: addresses || [host], labels },
    info: { node_ref: ref, addresses: addresses || [host], engines, labels },
    reserved_ports: reserved,
    capabilities: { node_ref: ref, engines, kernel_version: '6.1.0-26-amd64', cgroup: 'v2', ipv6: true, agent_version: transport === 'NODE_TRANSPORT_AGENT' ? agent : '' },
    desired_generation: String(desired),
    desired_state_hash: 'c1f0a9…',
    desired_hops: hops,
    reported: isReported,
    reported_generation: isReported ? String(reported) : undefined,
    reported_state_hash: isReported ? (reported === desired ? 'c1f0a9…' : '7be2d4…') : undefined,
    applied: isReported && reported === desired && hopErrors === 0,
    hop_errors: hopErrors,
    reported_at_unix_ms: isReported ? String(MOCK_NOW - reportedAgo) : undefined,
    // Not in NodeSummary: the mockup's stand-ins for what the page joins
    // from the node list (region) and the Agent session (online, transport).
    _region: region,
    _transport: transport,
    _online: online
  }
}

export const NODES = [
  node({ ref: 'forward-41', name: 'hk-edge-01', host: '203.0.113.41', region: '香港', engines: [nftCaps(), gostCaps()], labels: { region: 'hk', link: 'iepl' }, desired: 18, reported: 18, hops: 5 }),
  node({ ref: 'forward-42', name: 'hk-edge-02', host: '203.0.113.42', region: '香港', engines: [nftCaps(), gostCaps()], labels: { region: 'hk' }, desired: 18, reported: 17, hops: 2, reportedAgo: 2.5 * MIN }),
  node({ ref: 'forward-51', name: 'sg-relay-01', host: '198.51.100.51', region: '新加坡', engines: [gostCaps()], labels: { region: 'sg' }, desired: 9, reported: 9, hopErrors: 1, hops: 2 }),
  node({ ref: 'forward-61', name: 'tyo-exit-01', host: '198.51.100.61', region: '东京', engines: [nftCaps(), gostCaps()], labels: { region: 'jp' }, desired: 12, reported: 12, hops: 3 }),
  node({ ref: 'forward-62', name: 'tyo-exit-02', host: '198.51.100.62', region: '东京', engines: [gostCaps({ version: 'gost 3.0.0-rc.10' })], labels: { region: 'jp' }, desired: 12, reported: 12, hops: 4, agent: '4.1.2' }),
  node({ ref: 'forward-71', name: 'sha-iepl-01', host: '192.0.2.71', region: '上海', engines: [nftCaps(), gostCaps({ available: false, unavailable_reason: 'gost 未安装' })], labels: { region: 'sh', link: 'iepl' }, desired: 6, reported: 6, hops: 1, range: [20000, 24999], transport: 'NODE_TRANSPORT_ANSIBLE', reportedAgo: 0.8 * MIN }),
  node({ ref: 'proxy-12', kind: 'proxy', name: 'lax-proxy-12', host: '203.0.113.112', region: '洛杉矶', engines: [gostCaps()], labels: { region: 'us' }, desired: 4, reported: 4, hopErrors: 2, hops: 1 }),
  node({ ref: 'forward-81', name: 'fra-exit-01', host: '198.51.100.81', region: '法兰克福', engines: [nftCaps(), gostCaps()], labels: { region: 'de' }, desired: 0, hops: 0, enabled: false, online: false, agent: '' })
]

export function nodeByRef(ref) {
  return NODES.find(item => item.node_ref === ref)
}

export function nodeName(ref) {
  return nodeByRef(ref)?.name || ref
}

function hop(role, engine, refs, ingress = undefined, extra = {}) {
  return { role: `HOP_ROLE_${role}`, engine, node_refs: refs, ...(ingress ? { ingress } : {}), ...extra }
}

const RAW = { security: 'LINK_SECURITY_RAW' }

function route(id, fields) {
  return {
    id,
    owner: 'admin',
    revision: '3',
    created_at_unix_ms: String(MOCK_NOW - 21 * DAY),
    updated_at_unix_ms: String(MOCK_NOW - 2 * DAY),
    policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_ROUND_ROBIN' },
    labels: {},
    ...fields
  }
}

// GET /routes answers {routes: [{route, enforced}]}.
export const ROUTES = [
  {
    route: route('01JB7Q2M8X4V2C6B9K3D5F7H1A', {
      name: 'game-hk-tyo',
      listen: { address: '0.0.0.0', port: 30443, protocol: 'L4_PROTOCOL_TCP_UDP' },
      hops: [
        hop('ENTRY', 'ENGINE_NFTABLES', ['forward-41']),
        hop('RELAY', 'ENGINE_GOST', ['forward-51'], RAW),
        hop('EXIT', 'ENGINE_GOST', ['forward-61', 'forward-62'], { security: 'LINK_SECURITY_TLS', mux: true })
      ],
      targets: [{ host: 'game-1.example.jp', port: 7777, weight: 2 }, { host: 'game-2.example.jp', port: 7777, weight: 1 }],
      policy: { next_hop: 'BALANCE_STRATEGY_FAILOVER', target: 'BALANCE_STRATEGY_ROUND_ROBIN', health: {}, circuit_breaker: {} },
      limits: { bandwidth_bps: String(500 * 1e6), max_conns: 4000 },
      labels: { team: 'game', tier: 'gold' },
      revision: '7'
    }),
    enforced: ''
  },
  {
    route: route('01JB7Q3A1Z9Y8X7W6V5U4T3S2R', {
      name: 'iepl-sha-hk',
      listen: { address: '0.0.0.0', port: 21080, protocol: 'L4_PROTOCOL_TCP' },
      hops: [
        hop('ENTRY', 'ENGINE_NFTABLES', ['forward-71']),
        hop('EXIT', 'ENGINE_NFTABLES', ['forward-41'], RAW)
      ],
      targets: [{ host: '10.20.0.15', port: 443 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_ROUND_ROBIN', direct: 'DIRECT_MODE_PREFERRED', target_policy: 'TARGET_POLICY_ALLOW_PRIVATE' },
      labels: { link: 'iepl' }
    }),
    enforced: ''
  },
  {
    route: route('01JB7Q4C5D6E7F8G9H0J1K2L3M', {
      name: 'web-edge-ha',
      listen: { address: '0.0.0.0', port: 30080, protocol: 'L4_PROTOCOL_TCP', entry_hostname: 'edge.example.net' },
      hops: [
        hop('ENTRY', 'ENGINE_GOST', ['forward-41', 'forward-42']),
        hop('EXIT', 'ENGINE_GOST', ['forward-61'], { security: 'LINK_SECURITY_WSS', path: '/ws' })
      ],
      targets: [{ host: 'origin.example.com', port: 443 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_IP_HASH' },
      limits: { bandwidth_bps: String(100 * 1e6) },
      labels: { team: 'web' }
    }),
    enforced: ''
  },
  {
    route: route('01JB7Q5N6P7Q8R9S0T1U2V3W4X', {
      name: 'sg-direct-udp',
      listen: { address: '0.0.0.0', port: 31000, protocol: 'L4_PROTOCOL_UDP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['forward-51'])],
      targets: [{ host: '198.51.100.200', port: 51820 }],
      labels: { team: 'ops' }
    }),
    enforced: ''
  },
  {
    route: route('01JB7Q6Y7Z8A9B0C1D2E3F4G5H', {
      name: 'reseller-a',
      listen: { address: '0.0.0.0', port: 30500, protocol: 'L4_PROTOCOL_TCP' },
      hops: [
        hop('ENTRY', 'ENGINE_GOST', ['forward-42']),
        hop('EXIT', 'ENGINE_GOST', ['forward-62'], { security: 'LINK_SECURITY_TLS' })
      ],
      targets: [{ host: 'svc.reseller-a.example', port: 8443 }],
      limits: { quota_bytes: String(500 * GB), bandwidth_bps: String(200 * 1e6) },
      labels: { customer: 'reseller-a' }
    }),
    enforced: 'quota'
  },
  {
    route: route('01JB7Q7J8K9L0M1N2P3Q4R5S6T', {
      name: 'promo-oct',
      listen: { address: '0.0.0.0', port: 30600, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['forward-41']), hop('EXIT', 'ENGINE_GOST', ['forward-62'], { security: 'LINK_SECURITY_QUIC' })],
      targets: [{ host: 'promo.example.com', port: 443 }],
      limits: { expires_at_unix_ms: String(MOCK_NOW - 6 * HOUR) },
      labels: { campaign: 'promo' }
    }),
    enforced: 'expired'
  },
  {
    route: route('01JB7Q8U9V0W1X2Y3Z4A5B6C7D', {
      name: 'trial-user-23',
      listen: { address: '0.0.0.0', port: 30700, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_NFTABLES', ['forward-41']), hop('EXIT', 'ENGINE_NFTABLES', ['forward-61'], RAW)],
      targets: [{ host: 'trial.example.org', port: 22 }],
      paused: true,
      labels: { customer: 'trial' }
    }),
    enforced: ''
  },
  {
    route: route('01JB7Q9E0F1G2H3J4K5L6M7N8P', {
      name: 'us-media',
      listen: { address: '0.0.0.0', port: 32000, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['proxy-12']), hop('EXIT', 'ENGINE_GOST', ['forward-62'], { security: 'LINK_SECURITY_GRPC', mux: true })],
      targets: [{ host: 'media.example.com', port: 443 }, { host: 'media-b.example.com', port: 443, priority: 1 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_FAILOVER' },
      labels: { team: 'media' }
    }),
    enforced: ''
  }
]

export function routeById(id) {
  return ROUTES.find(item => item.route.id === id)
}

// GET /routes/{id}/health → {health: [UpstreamHealth]} per route.
function upstream(routeId, hopIndex, address, port, state, extra = {}) {
  return {
    route_id: routeId,
    hop_index: hopIndex,
    address,
    port,
    state,
    consecutive_failures: 0,
    rtt_us: 18_000,
    checked_at_unix_ms: String(MOCK_NOW - 4_000),
    ...extra
  }
}

export const HEALTH = {
  '01JB7Q2M8X4V2C6B9K3D5F7H1A': [
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 0, '198.51.100.51', 30012, 'HEALTH_STATE_HEALTHY', { rtt_us: 31_200, _node: 'forward-41' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 1, '198.51.100.61', 30020, 'HEALTH_STATE_HEALTHY', { rtt_us: 66_800, _node: 'forward-51' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 1, '198.51.100.62', 30020, 'HEALTH_STATE_CIRCUIT_OPEN', { rtt_us: 0, consecutive_failures: 3, circuit_open_until_unix_ms: String(MOCK_NOW + 18_000), _node: 'forward-51' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 2, 'game-1.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 2_100, _node: 'forward-61' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 2, 'game-2.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 2_400, _node: 'forward-61' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 2, 'game-1.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 1_900, _node: 'forward-62' }),
    upstream('01JB7Q2M8X4V2C6B9K3D5F7H1A', 2, 'game-2.example.jp', 7777, 'HEALTH_STATE_UNHEALTHY', { rtt_us: 0, consecutive_failures: 1, _node: 'forward-62' })
  ],
  '01JB7Q9E0F1G2H3J4K5L6M7N8P': [
    upstream('01JB7Q9E0F1G2H3J4K5L6M7N8P', 0, '198.51.100.62', 30031, 'HEALTH_STATE_UNHEALTHY', { rtt_us: 0, consecutive_failures: 2, _node: 'proxy-12' })
  ]
}

// Latest NodeForwardReport.errors per node (GET /nodes/{ref}).
export const HOP_ERRORS = {
  'forward-51': [{ route_id: '01JB7Q5N6P7Q8R9S0T1U2V3W4X', hop_index: 0, message: 'listen udp 0.0.0.0:31000: bind: address already in use' }],
  'proxy-12': [
    { route_id: '01JB7Q9E0F1G2H3J4K5L6M7N8P', hop_index: 0, message: 'dial grpc 198.51.100.62:30031: tls: certificate pinned identity mismatch' },
    { route_id: '01JB7Q9E0F1G2H3J4K5L6M7N8P', hop_index: 0, message: 'gost service restarted 3 times in 5m' }
  ]
}

// Route status for the list badge: paused, enforced, then health.
export function routeStatus({ route, enforced }) {
  if (enforced === 'quota') return { key: 'quota', label: '已强制暂停 · 配额用尽', tone: 'warning' }
  if (enforced === 'expired') return { key: 'expired', label: '已强制暂停 · 已到期', tone: 'warning' }
  if (route.paused) return { key: 'paused', label: '已暂停', tone: 'neutral' }
  const nodes = route.hops.flatMap(item => item.node_refs).map(nodeByRef)
  if (Object.values(HOP_ERRORS).flat().some(item => item.route_id === route.id)) return { key: 'error', label: '跳错误', tone: 'danger' }
  const health = HEALTH[route.id] || []
  if (health.some(item => item.state !== 'HEALTH_STATE_HEALTHY')) return { key: 'degraded', label: '降级', tone: 'warning' }
  if (nodes.some(item => item && item.reported_generation !== item.desired_generation)) return { key: 'syncing', label: '同步中', tone: 'info' }
  return { key: 'healthy', label: '正常', tone: 'success' }
}

// Hourly buckets (TrafficBucket summed over hops 0 only) for the last 24 h.
function wave(seed, scale) {
  return Array.from({ length: 24 }, (_, index) => {
    const hour = (index + 14) % 24 // MOCK_NOW is 14:00 local
    const daily = Math.sin(((hour - 6) / 24) * Math.PI * 2) * 0.45 + 0.6
    const jitter = ((seed * 7 + index * 13) % 11) / 40
    return Math.max(0, Math.round((daily + jitter) * scale))
  })
}

const TRAFFIC_SCALE = {
  '01JB7Q2M8X4V2C6B9K3D5F7H1A': 9.2,
  '01JB7Q3A1Z9Y8X7W6V5U4T3S2R': 14.5,
  '01JB7Q4C5D6E7F8G9H0J1K2L3M': 5.8,
  '01JB7Q5N6P7Q8R9S0T1U2V3W4X': 1.1,
  '01JB7Q6Y7Z8A9B0C1D2E3F4G5H': 0,
  '01JB7Q7J8K9L0M1N2P3Q4R5S6T': 0,
  '01JB7Q8U9V0W1X2Y3Z4A5B6C7D': 0,
  '01JB7Q9E0F1G2H3J4K5L6M7N8P': 2.6
}

export const HOURS = Array.from({ length: 24 }, (_, index) => MOCK_NOW - (23 - index) * HOUR)

export function routeSeries(id) {
  const scale = (TRAFFIC_SCALE[id] ?? 0) * 0.04 * GB
  const seed = id.charCodeAt(6)
  const down = wave(seed, scale)
  const up = wave(seed + 3, scale * 0.18)
  return HOURS.map((hour, index) => ({
    hour_start_unix_ms: String(hour),
    up_bytes: String(up[index]),
    down_bytes: String(down[index]),
    new_conns: String(Math.round(down[index] / (6 * 1024 ** 2)))
  }))
}

export function routeTraffic24h(id) {
  return routeSeries(id).reduce((sum, bucket) => ({
    up: sum.up + Number(bucket.up_bytes),
    down: sum.down + Number(bucket.down_bytes)
  }), { up: 0, down: 0 })
}

export function totalSeries() {
  const all = ROUTES.map(item => routeSeries(item.route.id))
  return HOURS.map((hour, index) => ({
    hour_start_unix_ms: String(hour),
    up_bytes: String(all.reduce((sum, series) => sum + Number(series[index].up_bytes), 0)),
    down_bytes: String(all.reduce((sum, series) => sum + Number(series[index].down_bytes), 0))
  }))
}

// Quota used, all time (Counters of the entry hop): reseller-a is at 100 %.
export const QUOTA_USED = {
  '01JB7Q6Y7Z8A9B0C1D2E3F4G5H': 500 * GB
}

// Desired NodeHop rows of one node (GET /nodes/{ref} state.hops), derived
// from the routes so the node page and the route page agree.
export function hopsOnNode(ref) {
  const out = []
  let mark = 1
  for (const { route } of ROUTES) {
    route.hops.forEach((item, index) => {
      if (!item.node_refs.includes(ref)) return
      out.push({
        route_id: route.id,
        route_name: route.name,
        hop_index: index,
        role: item.role,
        engine: item.engine,
        port: index === 0 ? route.listen.port : 30000 + ((route.id.charCodeAt(7) * 7 + index) % 90),
        mark: mark++,
        paused: Boolean(route.paused)
      })
    })
  }
  return out
}

export function routesUsingNode(ref) {
  return ROUTES.filter(({ route }) => route.hops.some(item => item.node_refs.includes(ref)))
}

export const LABEL_OPTIONS = Array.from(new Set(ROUTES.flatMap(({ route }) => Object.entries(route.labels || {}).map(([key, value]) => `${key}=${value}`)))).sort()
