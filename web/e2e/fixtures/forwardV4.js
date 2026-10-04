// The v4.2 forwarding pages (F5b) against a mocked /api/v4/forward
// (docs/forwarding/v4-api.md). Answers are protojson the way the package
// writes them: proto field names, enum names, 64-bit integers as strings,
// unset fields (zeros, false, empty) left out. Everything derives from
// FIXED_NOW_MS so screenshots are stable. The data is the H16 mockups'
// (docs/design/forward-ui), moved here when the real pages replaced them.
import { FIXED_NOW_MS } from './clock.js'

const NOW = FIXED_NOW_MS
const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const GB = 1024 ** 3
const ALL_STRATEGIES = ['BALANCE_STRATEGY_ROUND_ROBIN', 'BALANCE_STRATEGY_RANDOM', 'BALANCE_STRATEGY_IP_HASH', 'BALANCE_STRATEGY_LEAST_CONN', 'BALANCE_STRATEGY_FAILOVER']
const GOST_LINKS = ['LINK_SECURITY_RAW', 'LINK_SECURITY_TLS', 'LINK_SECURITY_WSS', 'LINK_SECURITY_QUIC', 'LINK_SECURITY_GRPC']

const nft = (extra = {}) => ({ engine: 'ENGINE_NFTABLES', version: 'nftables 1.0.9', available: true, ipv6: true, udp: true, strategies: ALL_STRATEGIES, link_securities: ['LINK_SECURITY_RAW'], bandwidth_limit: true, quota: true, max_conns: true, ...extra })
const gost = (extra = {}) => ({ engine: 'ENGINE_GOST', version: 'gost 3.0.0', available: true, ipv6: true, udp: true, strategies: ALL_STRATEGIES, link_securities: GOST_LINKS, bandwidth_limit: true, quota: true, max_conns: true, ...extra })

function node({ ref, kind = 'forward', name, host, region, engines, labels = {}, range = [30000, 39999], reserved = [22, 80, 443], desired, reported, hopErrors = 0, hops, enabled = true, ansible = false, agent = '4.2.0', ago = 0.4 * MIN }) {
  const isReported = reported !== undefined
  const id = Number(ref.split('-')[1])
  const out = {
    node_ref: ref,
    kind,
    name,
    host,
    enabled,
    in_inventory: enabled,
    negotiated: !ansible && isReported,
    settings: { port_range: { first: range[0], last: range[1] }, reserved_ports: reserved, addresses: [host], labels },
    info: { node_ref: ref, addresses: [host], port_range: { first: range[0], last: range[1] }, engines, labels },
    reserved_ports: reserved,
    capabilities: { node_ref: ref, engines, kernel_version: '6.1.0-26-amd64', cgroup: 'v2', ipv6: true, ...(ansible || !agent ? {} : { agent_version: agent }) },
    desired_generation: String(desired),
    desired_state_hash: 'c1f0a9d27e41',
    desired_hops: hops,
    hop_errors: hopErrors || undefined
  }
  if (!out.desired_hops) delete out.desired_hops
  if (!out.hop_errors) delete out.hop_errors
  if (!desired) delete out.desired_generation
  if (isReported) {
    Object.assign(out, {
      reported: true,
      reported_generation: String(reported),
      reported_state_hash: reported === desired ? 'c1f0a9d27e41' : '7be2d4a90c13',
      reported_at_unix_ms: String(NOW - ago)
    })
    if (reported === desired && !hopErrors) out.applied = true
  }
  if (kind === 'forward') {
    out.record = { id: String(id), name, host, transport: ansible ? 'NODE_TRANSPORT_ANSIBLE' : 'NODE_TRANSPORT_AGENT', region, ...(enabled ? { enabled: true } : {}) }
  }
  return out
}

export const NODES = [
  node({ ref: 'forward-41', name: 'hk-edge-01', host: '203.0.113.41', region: 'Hong Kong', engines: [nft(), gost()], labels: { region: 'hk', link: 'iepl' }, desired: 18, reported: 18, hops: 5 }),
  node({ ref: 'forward-42', name: 'hk-edge-02', host: '203.0.113.42', region: 'Hong Kong', engines: [nft(), gost()], labels: { region: 'hk' }, desired: 18, reported: 17, hops: 2, ago: 2.5 * MIN }),
  node({ ref: 'forward-51', name: 'sg-relay-01', host: '198.51.100.51', region: 'Singapore', engines: [gost()], labels: { region: 'sg' }, desired: 9, reported: 9, hopErrors: 1, hops: 2 }),
  node({ ref: 'forward-61', name: 'tyo-exit-01', host: '198.51.100.61', region: 'Tokyo', engines: [nft(), gost()], labels: { region: 'jp' }, desired: 12, reported: 12, hops: 3 }),
  node({ ref: 'forward-62', name: 'tyo-exit-02', host: '198.51.100.62', region: 'Tokyo', engines: [gost({ version: 'gost 3.0.0-rc.10' })], labels: { region: 'jp' }, desired: 12, reported: 12, hops: 4, agent: '4.1.2' }),
  node({ ref: 'forward-71', name: 'sha-iepl-01', host: '192.0.2.71', region: 'Shanghai', engines: [nft(), gost({ available: false, unavailable_reason: 'gost is not installed' })], labels: { region: 'sh', link: 'iepl' }, desired: 6, reported: 6, hops: 1, range: [20000, 24999], ansible: true, ago: 0.8 * MIN }),
  node({ ref: 'proxy-12', kind: 'proxy', name: 'lax-proxy-12', host: '203.0.113.112', region: 'Los Angeles', engines: [gost()], labels: { region: 'us' }, desired: 4, reported: 4, hopErrors: 2, hops: 1 }),
  node({ ref: 'forward-81', name: 'fra-exit-01', host: '198.51.100.81', region: 'Frankfurt', engines: [nft(), gost()], desired: 0, hops: 0, enabled: false, agent: '' })
]

const byRef = ref => NODES.find(item => item.node_ref === ref)

function hop(role, engine, refs, ingress) {
  return { role: `HOP_ROLE_${role}`, engine, node_refs: refs, ...(ingress ? { ingress } : {}) }
}
const RAW = { security: 'LINK_SECURITY_RAW' }

function route(id, fields) {
  return {
    id,
    owner: 'admin',
    revision: '3',
    created_at_unix_ms: String(NOW - 21 * DAY),
    updated_at_unix_ms: String(NOW - 2 * DAY),
    policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_ROUND_ROBIN' },
    ...fields
  }
}

export const GAME = '01JB7Q2M8X4V2C6B9K3D5F7H1A'
const SG = '01JB7Q5N6P7Q8R9S0T1U2V3W4X'
const RESELLER = '01JB7Q6Y7Z8A9B0C1D2E3F4G5H'
const MEDIA = '01JB7Q9E0F1G2H3J4K5L6M7N8P'
const TRIAL = '01JB7Q8U9V0W1X2Y3Z4A5B6C7D'

export const ROUTES = [
  {
    route: route(GAME, {
      name: 'game-hk-tyo',
      listen: { address: '0.0.0.0', port: 30443, protocol: 'L4_PROTOCOL_TCP_UDP' },
      hops: [hop('ENTRY', 'ENGINE_NFTABLES', ['forward-41']), hop('RELAY', 'ENGINE_GOST', ['forward-51'], RAW), hop('EXIT', 'ENGINE_GOST', ['forward-61', 'forward-62'], { security: 'LINK_SECURITY_TLS', mux: true })],
      targets: [{ host: 'game-1.example.jp', port: 7777, weight: 2 }, { host: 'game-2.example.jp', port: 7777, weight: 1 }],
      policy: { next_hop: 'BALANCE_STRATEGY_FAILOVER', target: 'BALANCE_STRATEGY_ROUND_ROBIN' },
      limits: { bandwidth_bps: String(500 * 1e6), max_conns: 4000 },
      labels: { team: 'game', tier: 'gold' },
      revision: '7'
    })
  },
  {
    route: route('01JB7Q3A1Z9Y8X7W6V5U4T3S2R', {
      name: 'iepl-sha-hk',
      listen: { address: '0.0.0.0', port: 21080, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_NFTABLES', ['forward-71']), hop('EXIT', 'ENGINE_NFTABLES', ['forward-41'], RAW)],
      targets: [{ host: '10.20.0.15', port: 443 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_ROUND_ROBIN', direct: 'DIRECT_MODE_PREFERRED', target_policy: 'TARGET_POLICY_ALLOW_PRIVATE' },
      labels: { link: 'iepl' }
    })
  },
  {
    route: route('01JB7Q4C5D6E7F8G9H0J1K2L3M', {
      name: 'web-edge-ha',
      listen: { address: '0.0.0.0', port: 30080, protocol: 'L4_PROTOCOL_TCP', entry_hostname: 'edge.example.net' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['forward-41', 'forward-42']), hop('EXIT', 'ENGINE_GOST', ['forward-61'], { security: 'LINK_SECURITY_WSS', path: '/ws' })],
      targets: [{ host: 'origin.example.com', port: 443 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_IP_HASH' },
      limits: { bandwidth_bps: String(100 * 1e6) },
      labels: { team: 'web' }
    })
  },
  {
    route: route(SG, {
      name: 'sg-direct-udp',
      listen: { address: '0.0.0.0', port: 31000, protocol: 'L4_PROTOCOL_UDP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['forward-51'])],
      targets: [{ host: '198.51.100.200', port: 51820 }],
      labels: { team: 'ops' }
    })
  },
  {
    route: route(RESELLER, {
      name: 'reseller-a',
      listen: { address: '0.0.0.0', port: 30500, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['forward-42']), hop('EXIT', 'ENGINE_GOST', ['forward-62'], { security: 'LINK_SECURITY_TLS' })],
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
      limits: { expires_at_unix_ms: String(NOW - 6 * HOUR) },
      labels: { campaign: 'promo' }
    }),
    enforced: 'expired'
  },
  {
    route: route(TRIAL, {
      name: 'trial-user-23',
      listen: { address: '0.0.0.0', port: 30700, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_NFTABLES', ['forward-41']), hop('EXIT', 'ENGINE_NFTABLES', ['forward-61'], RAW)],
      targets: [{ host: 'trial.example.org', port: 22 }],
      paused: true,
      labels: { customer: 'trial' }
    })
  },
  {
    route: route(MEDIA, {
      name: 'us-media',
      listen: { address: '0.0.0.0', port: 32000, protocol: 'L4_PROTOCOL_TCP' },
      hops: [hop('ENTRY', 'ENGINE_GOST', ['proxy-12']), hop('EXIT', 'ENGINE_GOST', ['forward-62'], { security: 'LINK_SECURITY_GRPC', mux: true })],
      targets: [{ host: 'media.example.com', port: 443 }, { host: 'media-b.example.com', port: 443, priority: 1 }],
      policy: { next_hop: 'BALANCE_STRATEGY_ROUND_ROBIN', target: 'BALANCE_STRATEGY_FAILOVER' },
      labels: { team: 'media' }
    })
  }
]

// Routes the e2e flow created (POST /routes), by id.
const CREATED = new Map()
const routeById = id => ROUTES.find(item => item.route.id === id) || CREATED.get(id)

// The port a node listens on for a hop (stable per route and hop).
function portOf(routeItem, index) {
  if (index === 0) return routeItem.listen.port
  return 30000 + ((routeItem.id.charCodeAt(7) * 7 + index) % 90)
}

function upstreamsFor(routeItem, index) {
  const next = routeItem.hops[index + 1]
  if (!next) return routeItem.targets.map(target => ({ address: target.host, port: target.port, ...(target.weight ? { weight: target.weight } : {}), ...(target.priority ? { priority: target.priority } : {}) }))
  return next.node_refs.map((ref, priority) => ({
    address: byRef(ref).host,
    port: portOf(routeItem, index + 1),
    ...(priority ? { priority } : {}),
    node_ref: ref,
    egress: { security: next.ingress?.security || 'LINK_SECURITY_RAW', ...(next.ingress?.mux ? { mux: true } : {}) }
  }))
}

// Upstream health per node, as the nodes' latest reports carry it.
function health(routeId, hopIndex, address, port, state, extra = {}) {
  const out = { route_id: routeId, address, port, state, rtt_us: 18_000, checked_at_unix_ms: String(NOW - 4_000), ...extra }
  if (hopIndex) out.hop_index = hopIndex
  if (!out.rtt_us) delete out.rtt_us
  return out
}

const HEALTH = {
  'forward-41': [health(GAME, 0, '198.51.100.51', portOf(routeById(GAME).route, 1), 'HEALTH_STATE_HEALTHY', { rtt_us: 31_200 })],
  'forward-51': [
    health(GAME, 1, '198.51.100.61', portOf(routeById(GAME).route, 2), 'HEALTH_STATE_HEALTHY', { rtt_us: 66_800 }),
    health(GAME, 1, '198.51.100.62', portOf(routeById(GAME).route, 2), 'HEALTH_STATE_CIRCUIT_OPEN', { rtt_us: 0, consecutive_failures: 3, circuit_open_until_unix_ms: String(NOW + 18_000) })
  ],
  'forward-61': [
    health(GAME, 2, 'game-1.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 2_100 }),
    health(GAME, 2, 'game-2.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 2_400 })
  ],
  'forward-62': [
    health(GAME, 2, 'game-1.example.jp', 7777, 'HEALTH_STATE_HEALTHY', { rtt_us: 1_900 }),
    health(GAME, 2, 'game-2.example.jp', 7777, 'HEALTH_STATE_UNHEALTHY', { rtt_us: 0, consecutive_failures: 1 })
  ],
  'proxy-12': [health(MEDIA, 0, '198.51.100.62', portOf(routeById(MEDIA).route, 1), 'HEALTH_STATE_UNHEALTHY', { rtt_us: 0, consecutive_failures: 2 })]
}

const HOP_ERRORS = {
  'forward-51': [{ route_id: SG, engine: 'ENGINE_GOST', message: 'listen udp 0.0.0.0:31000: bind: address already in use' }],
  'proxy-12': [
    { route_id: MEDIA, engine: 'ENGINE_GOST', message: 'dial grpc 198.51.100.62:30031: tls: certificate pinned identity mismatch' },
    { route_id: MEDIA, engine: 'ENGINE_GOST', message: 'gost service restarted 3 times in 5m' }
  ]
}

function nodeView(ref) {
  const summary = byRef(ref)
  if (!summary) return null
  let mark = 1
  const hops = []
  for (const { route: item } of ROUTES) {
    item.hops.forEach((entry, index) => {
      if (!entry.node_refs.includes(ref)) return
      const out = {
        route_id: item.id,
        role: entry.role,
        engine: entry.engine,
        listen: { address: index === 0 ? '0.0.0.0' : summary.host, port: portOf(item, index), protocol: item.listen.protocol },
        upstreams: upstreamsFor(item, index),
        mark: mark++
      }
      if (index) out.hop_index = index
      if (entry.ingress && index) out.ingress = entry.ingress
      if (item.paused) out.paused = true
      hops.push(out)
    })
  }
  const state = summary.desired_generation ? { node_ref: ref, generation: summary.desired_generation, state_hash: summary.desired_state_hash, hops } : null
  const report = summary.reported
    ? { node_ref: ref, generation: summary.reported_generation, state_hash: summary.reported_state_hash, ...(summary.applied ? { applied: true } : {}), errors: HOP_ERRORS[ref] || [], health: HEALTH[ref] || [], observed_at_unix_ms: summary.reported_at_unix_ms }
    : null
  return { node: summary, state, report }
}

// Hourly traffic: a daily wave per route, metered at every hop (the UI
// counts hop 0 only).
const SCALE = { [GAME]: 9.2, '01JB7Q3A1Z9Y8X7W6V5U4T3S2R': 14.5, '01JB7Q4C5D6E7F8G9H0J1K2L3M': 5.8, [SG]: 1.1, [MEDIA]: 2.6 }
function wave(seed, scale, count) {
  return Array.from({ length: count }, (_, index) => {
    const hour = (index + 8) % 24
    const daily = Math.sin(((hour - 6) / 24) * Math.PI * 2) * 0.45 + 0.6
    const jitter = ((seed * 7 + index * 13) % 11) / 40
    return Math.max(0, Math.round((daily + jitter) * scale))
  })
}

function buckets({ since, until, routeId = '' }) {
  const first = Math.floor(Number(since) / HOUR) * HOUR
  const count = Math.max(0, Math.round((Number(until) - first) / HOUR))
  const out = []
  for (const { route: item } of ROUTES) {
    if (routeId && item.id !== routeId) continue
    const scale = (SCALE[item.id] || 0) * 0.04 * GB
    if (!scale) continue
    const seed = item.id.charCodeAt(6)
    const down = wave(seed, scale, count)
    const up = wave(seed + 3, scale * 0.18, count)
    item.hops.forEach((entry, index) => {
      const ref = entry.node_refs[0]
      for (let i = 0; i < count; i++) {
        if (!down[i] && !up[i]) continue
        const bucket = { route_id: item.id, node_ref: ref, hour_start_unix_ms: String(first + i * HOUR), up_bytes: String(up[i]), down_bytes: String(down[i]), new_conns: String(Math.round(down[i] / (6 * 1024 ** 2))) }
        if (index) bucket.hop_index = index
        out.push(bucket)
      }
    })
  }
  return out
}

function stats(query) {
  const until = Number(query.until || Math.floor(NOW / HOUR) * HOUR + HOUR)
  const since = Number(query.since || until - DAY)
  const series = buckets({ since, until, routeId: query.route_id || '' })
  const totals = new Map()
  for (const bucket of series) {
    const key = `${bucket.route_id}/${bucket.hop_index || 0}/${bucket.node_ref}`
    const sum = totals.get(key) || { route_id: bucket.route_id, hop_index: bucket.hop_index || 0, node_ref: bucket.node_ref, up_bytes: 0, down_bytes: 0, up_packets: 0, down_packets: 0, new_conns: 0 }
    sum.up_bytes += Number(bucket.up_bytes)
    sum.down_bytes += Number(bucket.down_bytes)
    sum.new_conns += Number(bucket.new_conns)
    totals.set(key, sum)
  }
  return { totals: [...totals.values()], nodes: [], series, truncated: false }
}

function targets() {
  const out = []
  for (const { route: item } of ROUTES) {
    const last = item.hops.length - 1
    for (const target of item.targets) {
      const seen = item.hops[last].node_refs.flatMap(ref => (HEALTH[ref] || []).filter(entry => entry.route_id === item.id && (entry.hop_index || 0) === last && entry.address === target.host))
      let state = 'HEALTH_STATE_UNSPECIFIED'
      if (seen.length) state = seen.some(entry => entry.state === 'HEALTH_STATE_UNHEALTHY') ? 'HEALTH_STATE_UNHEALTHY' : 'HEALTH_STATE_HEALTHY'
      out.push({ route_id: item.id, route_name: item.name, paused: Boolean(item.paused), key: `${target.host}:${target.port}`, host: target.host, port: target.port, weight: target.weight || 0, priority: target.priority || 0, state, healthy: seen.filter(entry => entry.state === 'HEALTH_STATE_HEALTHY').length, reports: seen.length, rtt_us: 0, checked_at_unix_ms: NOW - 4000 })
    }
  }
  return { targets: out, total: out.length, truncated: false }
}

// ---------------------------------------------------------------------------
// POST /routes/preview: a few of sdk/forward/validate's rules, then a plan.
// ---------------------------------------------------------------------------

const caps = (ref, engine) => byRef(ref)?.info.engines.find(item => item.engine === engine)
const isPrivate = host => /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.)/.test(host || '')

export function validate(item) {
  const out = []
  const add = (field, code, message) => out.push({ field, message, code })
  if (!item.name) add('name', 'required', 'name is required')
  const port = item.listen?.port || 0;
  (item.hops || []).forEach((entry, index) => {
    const path = `hops[${index}]`
    for (const ref of entry.node_refs || []) {
      const summary = byRef(ref)
      if (!summary || !summary.enabled) { add(`${path}.node_refs`, 'unknown_node', `node ${ref} is not in the inventory`); continue }
      const engine = caps(ref, entry.engine)
      if (!engine) add(`${path}.engine`, 'engine_not_advertised', `${ref} does not advertise ${entry.engine}`)
      else if (!engine.available) add(`${path}.engine`, 'engine_unavailable', `${ref}: ${engine.unavailable_reason}`)
      if (index === 0 && port) {
        const range = summary.settings.port_range
        if (summary.reserved_ports.includes(port)) add('listen.port', 'port_reserved', `port ${port} is reserved on ${ref}`)
        else if (port < range.first || port > range.last) add('listen.port', 'port_out_of_range', `port ${port} is outside ${range.first}-${range.last} on ${ref}`)
      }
    }
    if (index > 0) {
      const security = entry.ingress?.security || 'LINK_SECURITY_RAW'
      const previous = item.hops[index - 1]
      const originates = (previous.node_refs || []).every(ref => caps(ref, previous.engine)?.link_securities.includes(security))
      const terminates = (entry.node_refs || []).every(ref => caps(ref, entry.engine)?.link_securities.includes(security))
      if (!originates || !terminates) add(`${path}.ingress.security`, 'link_unsupported', `${previous.engine} cannot originate ${security}`)
    }
  })
  ;(item.targets || []).forEach((target, index) => {
    if (isPrivate(target.host) && item.policy?.target_policy !== 'TARGET_POLICY_ALLOW_PRIVATE') add(`targets[${index}].host`, 'target_not_allowed', `${target.host} is private and the target policy is PUBLIC_ONLY`)
  })
  return out
}

function preview(body) {
  const item = body?.route || {}
  const violations = validate(item)
  const warnings = []
  if (violations.length) return { states: [], allocations: [], violations, warnings }
  const id = item.id || ''
  const states = new Map()
  const allocations = []
  const planned = { ...item, id, listen: { ...item.listen, port: item.listen?.port || 30017 } }
  item.hops.forEach((entry, index) => {
    entry.node_refs.forEach((ref, position) => {
      const summary = byRef(ref)
      const port = index === 0 ? planned.listen.port : (entry.port || portOf(planned.id ? planned : { ...planned, id: 'NEWROUTE0' }, index))
      const mark = 40 + index * 3 + position
      allocations.push({ ...(id ? { route_id: id } : {}), ...(index ? { hop_index: index } : {}), node_ref: ref, port, mark })
      const state = states.get(ref) || { node_ref: ref, generation: String(Number(summary.desired_generation || 0) + 1), state_hash: 'f00dfeed0001', hops: [] }
      const nodeHop = { ...(id ? { route_id: id } : {}), role: entry.role, engine: entry.engine, listen: { address: index === 0 ? (item.listen?.address || '0.0.0.0') : summary.host, port }, upstreams: upstreamsFor({ ...planned, id: planned.id || 'NEWROUTE0' }, index), mark }
      if (index) nodeHop.hop_index = index
      if (index && entry.ingress) nodeHop.ingress = entry.ingress
      state.hops.push(nodeHop)
      states.set(ref, state)
    })
  })
  return { states: [...states.values()], allocations, violations: [], warnings }
}

// ---------------------------------------------------------------------------
// POST /routes/{id}/diagnose
// ---------------------------------------------------------------------------

const step = (fields, result) => ({ ...fields, result: { observed_at_unix_ms: String(NOW - 1000), ...result } })
export const DIAGNOSIS = {
  ok: false,
  route_id: GAME,
  started_at_unix_ms: String(NOW - 2_400),
  finished_at_unix_ms: String(NOW - 1_150),
  steps: [
    step({ node_ref: 'forward-41', kind: 'PROBE_KIND_CONFIG', vantage: 'DIAGNOSE_VANTAGE_CONTROL' }, { ok: true, status: 'PROBE_STATUS_OK', code: 'healthy', message: 'generation 18 applied' }),
    step({ node_ref: 'forward-51', hop_index: 1, kind: 'PROBE_KIND_HEALTH', vantage: 'DIAGNOSE_VANTAGE_CONTROL', target: '198.51.100.62:30020' }, { status: 'PROBE_STATUS_FAILED', code: 'circuit_open', message: '3 consecutive failures; next probe in 18 s' }),
    step({ node_ref: 'forward-41', kind: 'PROBE_KIND_LISTEN', vantage: 'DIAGNOSE_VANTAGE_NODE', target: '0.0.0.0:30443', protocol: 'L4_PROTOCOL_TCP' }, { ok: true, status: 'PROBE_STATUS_OK', code: 'reachable', message: 'listening', probe_id: 'fwdiag-1' }),
    step({ node_ref: 'forward-41', kind: 'PROBE_KIND_TCP_CONNECT', vantage: 'DIAGNOSE_VANTAGE_NODE', target: '198.51.100.51:30020', protocol: 'L4_PROTOCOL_TCP' }, { ok: true, status: 'PROBE_STATUS_OK', code: 'reachable', rtt_us: 31_200, probe_id: 'fwdiag-2' }),
    step({ node_ref: 'forward-62', hop_index: 2, kind: 'PROBE_KIND_DELIVERY', vantage: 'DIAGNOSE_VANTAGE_NODE', target: 'game-2.example.jp:7777', protocol: 'L4_PROTOCOL_TCP' }, { status: 'PROBE_STATUS_FAILED', code: 'unreachable', message: 'connect: connection refused', probe_id: 'fwdiag-3' }),
    step({ node_ref: 'forward-41', kind: 'PROBE_KIND_UDP_EXCHANGE', vantage: 'DIAGNOSE_VANTAGE_NODE', target: '198.51.100.51:30020', protocol: 'L4_PROTOCOL_UDP' }, { status: 'PROBE_STATUS_INCONCLUSIVE', code: 'no_reply', message: 'no UDP reply in 2 s' }),
    step({ kind: 'PROBE_KIND_TCP_CONNECT', vantage: 'DIAGNOSE_VANTAGE_CONTROL', target: '203.0.113.41:30443', protocol: 'L4_PROTOCOL_TCP' }, { ok: true, status: 'PROBE_STATUS_OK', code: 'reachable', rtt_us: 12_400 }),
    step({ kind: 'PROBE_KIND_UDP_EXCHANGE', vantage: 'DIAGNOSE_VANTAGE_CONTROL', target: '203.0.113.41:30443', protocol: 'L4_PROTOCOL_UDP' }, { status: 'PROBE_STATUS_SKIPPED', code: 'control_udp_not_probed', message: 'Control does not probe UDP' })
  ],
  nodes: [
    { node_ref: 'forward-41', connected: true, node_vantage: true },
    { node_ref: 'forward-51', connected: true, node_vantage: true },
    { node_ref: 'forward-61', connected: true, node_vantage: true },
    { node_ref: 'forward-62', connected: true, note: 'Agent 4.1.2 does not advertise diag.v1' }
  ]
}

// ---------------------------------------------------------------------------
// The fixture
// ---------------------------------------------------------------------------

const EXTENSION = {
  plugin_id: 'forward', plugin_name: 'Forward', publisher: 'AnixOps', version: '4.2.0', api_version: 'v2', installation_id: 3, state: 'healthy',
  bundle: { path: 'webui/index.mjs', sha256: 'a'.repeat(64), url: '' }, permissions: ['forward.view'], menus: [], routes: [], config_schema: {},
  control_routes: ['/api/v4/plugins/forward/*']
}

// A brand-new route as an operator might first write it: an nftables
// entry straight into a TLS exit, a reserved entry port and a private
// target under the public-only policy (three violations).
export const DRAFT_WITH_PROBLEMS = {
  name: 'game-sha-tyo',
  listen: { port: 443 },
  hops: [{ engine: 'ENGINE_NFTABLES', node_refs: ['forward-71'] }, { engine: 'ENGINE_GOST', node_refs: ['forward-61', 'forward-62'], ingress: 'LINK_SECURITY_TLS' }],
  target: { host: '10.0.3.8', port: 7777 }
}

const PATHS = {
  overview: '/admin/forward/overview',
  routes: '/admin/forward/routes',
  'routes-bulk': '/admin/forward/routes',
  'routes-empty': '/admin/forward/routes',
  editor: '/admin/forward/routes/new',
  'editor-blank': '/admin/forward/routes/new',
  'editor-preview': `/admin/forward/routes/${GAME}/edit`,
  route: `/admin/forward/routes/${GAME}`,
  diagnose: `/admin/forward/routes/${GAME}?diagnose=1`,
  nodes: '/admin/forward/inventory',
  node: '/admin/forward/inventory/forward-51',
  'no-capability': '/admin/forward/overview'
}

export function forwardApi(path, { method, query = {}, body, scenario }) {
  if (path === '/api/v3/extensions') return scenario === 'no-capability' ? [] : [EXTENSION]
  if (!path.startsWith('/api/v4/forward/')) return undefined
  const rest = path.slice('/api/v4/forward'.length)
  const data = value => ({ data: value })
  const empty = scenario === 'routes-empty'
  if (rest === '/routes' && method === 'GET') {
    const routes = empty ? [] : ROUTES.filter(item => !query.node_ref || item.route.hops.some(entry => entry.node_refs.includes(query.node_ref)))
    return data({ routes, can_delete: true })
  }
  if (rest === '/routes' && method === 'POST') {
    const created = { ...body, id: '01JBNEWROUTE00000000000000', owner: 'admin', revision: '1', created_at_unix_ms: String(NOW), updated_at_unix_ms: String(NOW) }
    CREATED.set(created.id, { route: created })
    return { __status: 201, body: data({ route: created }) }
  }
  if (rest === '/routes/preview' && method === 'POST') return data(preview(body))
  if (rest === '/nodes' && method === 'GET') return data({ nodes: (query.kind ? NODES.filter(item => item.kind === query.kind) : NODES), can_delete: true })
  if (rest === '/stats') return data(empty ? { totals: [], nodes: [], series: [], truncated: false } : stats(query))
  if (rest === '/observability/targets') return data(empty ? { targets: [], total: 0, truncated: false } : targets())
  let match = rest.match(/^\/routes\/([^/]+)(?:\/(\w+))?$/)
  if (match) {
    const [, id, action] = match
    const item = routeById(id)
    if (!item) return { __status: 404, body: { error: { code: 'not_found', message: 'forward route not found' } } }
    if (!action && method === 'GET') return data(item)
    if (!action && method === 'PUT') return data({ route: { ...body, revision: String(Number(body.revision || 0) + 1) } })
    if (!action && method === 'DELETE') return data({ deleted: id })
    if (action === 'pause' || action === 'resume') return data({ ...item, route: { ...item.route, paused: action === 'pause' } })
    if (action === 'stats') {
      const answer = stats({ ...query, route_id: id })
      return data({ counters: [{ route_id: id, node_ref: item.route.hops[0].node_refs[0], up_bytes: '0', down_bytes: '0', active_conns: 1284, total_conns: '98213' }], series: answer.series, truncated: false })
    }
    if (action === 'health') return data({ health: Object.values(HEALTH).flat().filter(entry => entry.route_id === id) })
    if (action === 'diagnose') return data(DIAGNOSIS)
  }
  match = rest.match(/^\/nodes\/([^/]+)(?:\/(\w+))?$/)
  if (match) {
    const [, ref, action] = match
    const view = nodeView(ref)
    if (!view) return { __status: 404, body: { error: { code: 'not_found', message: 'forward node not found' } } }
    if (!action && method === 'GET') return data(view)
    if (action === 'settings' && method === 'PUT') {
      return data({ node: { ...view.node, settings: { ...view.node.settings, ...body } }, violations: [] })
    }
    if (action === 'toggle') return data({ node: { ...view.node, enabled: Boolean(body?.enabled) }, violations: [] })
  }
  return { __status: 404, body: { error: { code: 'not_found', message: 'route not found' } } }
}

export default {
  edition: 'community',
  pathFor: scenario => PATHS[scenario] || PATHS.routes,
  api: forwardApi,
  // After the page loaded: the scenario's interactions.
  async after(page, { scenario }) {
    if (scenario === 'routes-bulk') {
      await page.getByRole('checkbox', { name: /trial-user-23/ }).first().check()
      await page.getByRole('checkbox', { name: /sg-direct-udp/ }).first().check()
    }
    if (scenario === 'editor') {
      await fillDraftWithProblems(page)
      await page.getByTestId('forward-violations').waitFor()
    }
    if (scenario === 'editor-preview') {
      await page.getByTestId('forward-preview').getByText(/nodes \+1/).waitFor()
    }
    if (scenario === 'diagnose') {
      await page.getByTestId('forward-diagnosis').getByText(/Some steps failed|有步骤失败/).waitFor()
    }
    // Full-page screenshots start at the top, with no focus ring.
    await page.evaluate(() => { document.activeElement?.blur?.(); window.scrollTo(0, 0) })
  }
}

// fillDraftWithProblems types DRAFT_WITH_PROBLEMS into the new-route editor.
export async function fillDraftWithProblems(page) {
  const draft = DRAFT_WITH_PROBLEMS
  await page.getByLabel('Name').first().fill(draft.name)
  await page.getByRole('group', { name: 'Port mode' }).first().getByText('Fixed').click()
  await page.getByLabel('Entry port').fill(String(draft.listen.port))
  // The exit's TLS link first: once the entry is nftables the editor offers
  // only the links it can originate.
  await page.getByTestId('forward-add-hop').click()
  await chooseNode(page, 1, 'tyo-exit-01')
  await chooseNode(page, 1, 'tyo-exit-02')
  await chooseOption(page, page.locator('[data-hop-index="1"]').getByLabel('Ingress link'), 'TLS')
  await chooseNode(page, 0, 'sha-iepl-01')
  await chooseOption(page, page.locator('[data-hop-index="0"]').getByLabel('Engine'), 'nftables')
  await page.getByLabel('Target 1 host').fill(draft.target.host)
  await page.getByLabel('Target 1 port').fill(String(draft.target.port))
  await page.getByLabel('Target 1 port').blur()
}

export async function chooseNode(page, hopIndex, name) {
  const box = page.locator(`[data-hop-index="${hopIndex}"]`).getByRole('combobox', { name: /Add a node to hop/ })
  await box.click()
  await box.fill(name)
  await page.getByRole('option', { name: new RegExp(name) }).first().click()
}

export async function chooseOption(page, trigger, name) {
  await trigger.click()
  await page.getByRole('option', { name: new RegExp(`^${name}`) }).first().click()
}
