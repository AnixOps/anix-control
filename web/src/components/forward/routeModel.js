// The forwarding pages' model (F5b): enum metadata, the route status, the
// editor draft and its conversion to the API's Route, violation lookup and
// the traffic joins. Pure functions; the views and their tests share them.
// Shapes: docs/forwarding/v4-api.md (protojson: enum names, 64-bit integers
// as strings, unset fields left out, so a missing hop_index is 0).

export const ENGINES = Object.freeze({
  ENGINE_NFTABLES: { short: 'nft', label: 'nftables' },
  ENGINE_GOST: { short: 'gost', label: 'gost' },
  ENGINE_ANIXOPS: { short: 'anixops', label: 'anixops' }
})

// Engines the editor offers; anixops only with the experimental flag (D12).
export const BASE_ENGINES = Object.freeze(['ENGINE_NFTABLES', 'ENGINE_GOST'])

export const SECURITIES = Object.freeze({
  LINK_SECURITY_RAW: 'RAW',
  LINK_SECURITY_TLS: 'TLS',
  LINK_SECURITY_WSS: 'WSS',
  LINK_SECURITY_QUIC: 'QUIC',
  LINK_SECURITY_GRPC: 'gRPC',
  LINK_SECURITY_ANIXOPS: 'AnixOps'
})

export const STRATEGIES = Object.freeze([
  'BALANCE_STRATEGY_ROUND_ROBIN',
  'BALANCE_STRATEGY_RANDOM',
  'BALANCE_STRATEGY_IP_HASH',
  'BALANCE_STRATEGY_LEAST_CONN',
  'BALANCE_STRATEGY_FAILOVER'
])

export const DIRECT_MODES = Object.freeze(['DIRECT_MODE_OFF', 'DIRECT_MODE_PREFERRED', 'DIRECT_MODE_FORCED'])
export const TARGET_POLICIES = Object.freeze(['TARGET_POLICY_PUBLIC_ONLY', 'TARGET_POLICY_ALLOW_PRIVATE'])
export const PROTOCOLS = Object.freeze(['L4_PROTOCOL_TCP', 'L4_PROTOCOL_UDP', 'L4_PROTOCOL_TCP_UDP'])

// H21 defaults (sdk/forward/model/defaults.go).
export const H21_DEFAULTS = Object.freeze({ interval_ms: 5000, timeout_ms: 2000, failure_threshold: 3, open_ms: 30000 })

export const GB = 1024 ** 3
export const HOUR_MS = 3_600_000

// Status keys in badge order (worst first; docs/design/forward-ui README).
export const STATUS_ORDER = Object.freeze(['quota', 'expired', 'paused', 'error', 'degraded', 'syncing', 'healthy'])
export const STATUS_TONES = Object.freeze({
  quota: 'warning', expired: 'warning', paused: 'neutral', error: 'danger', degraded: 'warning', syncing: 'info', healthy: 'success'
})

export function roleOf(index, total) {
  if (index === 0) return 'HOP_ROLE_ENTRY'
  return index === total - 1 ? 'HOP_ROLE_EXIT' : 'HOP_ROLE_RELAY'
}

export function securityOf(hop) {
  return hop?.ingress?.security || 'LINK_SECURITY_RAW'
}

// linkLabel names a hop's ingress link: "TLS", "TLS·mux".
export function linkLabel(hop, mux = '·mux') {
  const security = SECURITIES[securityOf(hop)] || securityOf(hop)
  return hop?.ingress?.mux ? `${security}${mux}` : security
}

export function isEnforced(enforced) {
  return enforced === 'quota' || enforced === 'expired'
}

export function num(value) {
  const number = Number(value ?? 0)
  return Number.isFinite(number) ? number : 0
}

// ---------------------------------------------------------------------------
// Route status
// ---------------------------------------------------------------------------

// statusContext joins what the list knows about every route from one load:
// - hopErrorRoutes: route ids with a HopError in a node's latest report;
// - degradedRoutes: route ids with a target or upstream that is unhealthy or
//   circuit-open;
// - laggingNodes: node refs whose applied generation is behind.
export function statusContext({ nodes = [], hopErrors = [], targets = [], health = [] } = {}) {
  const hopErrorRoutes = new Set(hopErrors.map(error => error.route_id).filter(Boolean))
  const degradedRoutes = new Set()
  for (const target of targets) {
    if (target.state === 'HEALTH_STATE_UNHEALTHY' || target.state === 'HEALTH_STATE_CIRCUIT_OPEN') degradedRoutes.add(target.route_id)
  }
  for (const upstream of health) {
    if (upstream.state === 'HEALTH_STATE_UNHEALTHY' || upstream.state === 'HEALTH_STATE_CIRCUIT_OPEN') degradedRoutes.add(upstream.route_id)
  }
  const laggingNodes = new Set(nodes.filter(nodeLagging).map(node => node.node_ref))
  return { hopErrorRoutes, degradedRoutes, laggingNodes }
}

export function nodeLag(node) {
  if (!node?.reported) return 0
  return Math.max(0, num(node.desired_generation) - num(node.reported_generation))
}

// A node counts as online while its latest report is at most 3 minutes old
// (nodes report at least every minute).
export const ONLINE_WINDOW_MS = 3 * 60_000

export function nodeOnline(node, now = Date.now()) {
  return node?.enabled !== false && Boolean(node?.reported) && now - num(node.reported_at_unix_ms) <= ONLINE_WINDOW_MS
}

// nodeState is the node's status word key: disabled, online or offline.
export function nodeState(node, now = Date.now()) {
  if (node?.enabled === false) return 'disabled'
  return nodeOnline(node, now) ? 'online' : 'offline'
}

export function nodeLagging(node) {
  return num(node?.desired_generation) > 0 && (!node?.reported || nodeLag(node) > 0)
}

// routeStatus is the one badge: enforced (Control paused it) before paused
// (an operator did) before health (D8: enforced wins over paused).
export function routeStatus({ route, enforced } = {}, context = {}) {
  if (enforced === 'quota') return 'quota'
  if (enforced === 'expired') return 'expired'
  if (route?.paused) return 'paused'
  const id = route?.id
  if (id && context.hopErrorRoutes?.has(id)) return 'error'
  if (id && context.degradedRoutes?.has(id)) return 'degraded'
  const refs = (route?.hops || []).flatMap(hop => hop.node_refs || [])
  if (refs.some(ref => context.laggingNodes?.has(ref))) return 'syncing'
  return 'healthy'
}

// ---------------------------------------------------------------------------
// Traffic
// ---------------------------------------------------------------------------

// entryTraffic sums the entry hop (hop 0) per route from GET /stats totals:
// the raw metered bytes, counted once per route (D6).
export function entryTraffic(totals = []) {
  const out = new Map()
  for (const total of totals) {
    if (num(total.hop_index) !== 0) continue
    const sum = out.get(total.route_id) || { up: 0, down: 0 }
    sum.up += num(total.up_bytes)
    sum.down += num(total.down_bytes)
    out.set(total.route_id, sum)
  }
  return out
}

// hourlySeries sums the entry hop's buckets per hour over [since, until):
// [{hour, up, down}], one per hour with zeros where nothing was metered.
export function hourlySeries(buckets = [], { since, until, routeId = '' } = {}) {
  const byHour = new Map()
  for (const bucket of buckets) {
    if (num(bucket.hop_index) !== 0) continue
    if (routeId && bucket.route_id !== routeId) continue
    const hour = num(bucket.hour_start_unix_ms)
    const sum = byHour.get(hour) || { up: 0, down: 0 }
    sum.up += num(bucket.up_bytes)
    sum.down += num(bucket.down_bytes)
    byHour.set(hour, sum)
  }
  const out = []
  const first = Math.floor(since / HOUR_MS) * HOUR_MS
  for (let hour = first; hour < until; hour += HOUR_MS) {
    const sum = byHour.get(hour) || { up: 0, down: 0 }
    out.push({ hour, up: sum.up, down: sum.down })
  }
  return out
}

// trafficWindow: the last `hours` hours up to the end of the current hour.
export function trafficWindow(now, hours = 24) {
  const until = Math.floor(now / HOUR_MS) * HOUR_MS + HOUR_MS
  return { since: until - hours * HOUR_MS, until }
}

// ---------------------------------------------------------------------------
// Violations
// ---------------------------------------------------------------------------

// fieldId is the DOM id of a violation's field ("hops[1].ingress.security"
// → "fwd-f-hops-1-ingress-security").
export function fieldId(field) {
  return `fwd-f-${String(field || 'route').replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '')}`
}

export function violationsFor(violations, field) {
  return (violations || []).filter(item => item.field === field)
}

// fieldHasViolation reports a violation on field or below it.
export function underField(violations, prefix) {
  return (violations || []).some(item => item.field === prefix || String(item.field).startsWith(`${prefix}.`) || String(item.field).startsWith(`${prefix}[`))
}

// ---------------------------------------------------------------------------
// The editor draft
// ---------------------------------------------------------------------------

let keySeq = 0
function nextKey() {
  keySeq += 1
  return `k${keySeq}`
}

export function blankHop(index, total, engine = 'ENGINE_GOST') {
  const hop = { _key: nextKey(), role: roleOf(index, total), engine, node_refs: [], port: 0, portMode: 'auto', dial_address: '' }
  if (index > 0) hop.ingress = { security: 'LINK_SECURITY_RAW', mux: false, server_name: '', path: '' }
  return hop
}

export function blankTarget() {
  return { _key: nextKey(), host: '', port: null, weight: null, priority: null }
}

function pad(value) {
  return String(value).padStart(2, '0')
}

// toLocalInput turns Unix ms into a datetime-local value in the browser's
// time zone ("2026-10-04T14:00").
export function toLocalInput(ms) {
  const value = num(ms)
  if (!value) return ''
  const date = new Date(value)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function fromLocalInput(text) {
  if (!text) return 0
  const ms = new Date(text).getTime()
  return Number.isFinite(ms) ? ms : 0
}

// routeToDraft makes the editor's draft from a stored Route (or nothing for
// a new route). UI-only keys start with _ or are named *Mode.
export function routeToDraft(route = null) {
  const source = route ? JSON.parse(JSON.stringify(route)) : null
  const hops = (source?.hops?.length ? source.hops : [{}]).map((hop, index, list) => ({
    ...blankHop(index, list.length, hop.engine || 'ENGINE_GOST'),
    ...hop,
    _key: nextKey(),
    role: roleOf(index, list.length),
    node_refs: [...(hop.node_refs || [])],
    port: num(hop.port),
    portMode: num(hop.port) ? 'explicit' : 'auto',
    dial_address: hop.dial_address || '',
    ...(index > 0 ? { ingress: { security: 'LINK_SECURITY_RAW', mux: false, server_name: '', path: '', ...(hop.ingress || {}) } } : {})
  }))
  if (hops[0]) delete hops[0].ingress
  const limits = source?.limits || {}
  return {
    id: source?.id || '',
    owner: source?.owner || '',
    revision: source?.revision || '',
    paused: Boolean(source?.paused),
    name: source?.name || '',
    labels: Object.entries(source?.labels || {}).map(([key, value]) => ({ _key: nextKey(), key, value })),
    listen: {
      address: source?.listen?.address || '',
      port: num(source?.listen?.port),
      portMode: num(source?.listen?.port) ? 'explicit' : 'auto',
      protocol: source?.listen?.protocol || 'L4_PROTOCOL_TCP',
      entry_hostname: source?.listen?.entry_hostname || ''
    },
    hops,
    targets: (source?.targets?.length ? source.targets : [{}]).map(target => ({
      ...blankTarget(),
      host: target.host || '',
      port: num(target.port) || null,
      weight: num(target.weight) || null,
      priority: num(target.priority) || null
    })),
    policy: {
      next_hop: source?.policy?.next_hop || 'BALANCE_STRATEGY_ROUND_ROBIN',
      target: source?.policy?.target || 'BALANCE_STRATEGY_ROUND_ROBIN',
      direct: source?.policy?.direct || 'DIRECT_MODE_OFF',
      target_policy: source?.policy?.target_policy || 'TARGET_POLICY_PUBLIC_ONLY',
      health: {
        interval_ms: num(source?.policy?.health?.interval_ms) || null,
        timeout_ms: num(source?.policy?.health?.timeout_ms) || null,
        disabled: Boolean(source?.policy?.health?.disabled)
      },
      circuit_breaker: {
        failure_threshold: num(source?.policy?.circuit_breaker?.failure_threshold) || null,
        open_ms: num(source?.policy?.circuit_breaker?.open_ms) || null
      }
    },
    limits: {
      bandwidthMbps: num(limits.bandwidth_bps) ? num(limits.bandwidth_bps) / 1e6 : null,
      quotaGB: num(limits.quota_bytes) ? num(limits.quota_bytes) / GB : null,
      max_conns: num(limits.max_conns) || null,
      expires: toLocalInput(limits.expires_at_unix_ms)
    }
  }
}

// draftToRoute is the Route the draft stands for, in API field names. The
// API client's routeBody drops what is unset.
export function draftToRoute(draft) {
  const hops = draft.hops.map((hop, index) => {
    const out = {
      role: roleOf(index, draft.hops.length),
      engine: hop.engine,
      node_refs: [...hop.node_refs],
      port: index > 0 && hop.portMode === 'explicit' ? num(hop.port) : 0,
      dial_address: index > 0 ? (hop.dial_address || '').trim() : ''
    }
    if (index > 0 && hop.ingress) {
      const raw = securityOf(hop) === 'LINK_SECURITY_RAW'
      out.ingress = {
        security: securityOf(hop),
        mux: raw ? false : Boolean(hop.ingress.mux),
        server_name: raw ? '' : (hop.ingress.server_name || ''),
        path: securityOf(hop) === 'LINK_SECURITY_WSS' ? (hop.ingress.path || '') : ''
      }
    }
    return out
  })
  const labels = {}
  for (const label of draft.labels) {
    const key = String(label.key || '').trim()
    if (key) labels[key] = String(label.value ?? '').trim()
  }
  const bandwidth = num(draft.limits.bandwidthMbps)
  const quota = num(draft.limits.quotaGB)
  return {
    id: draft.id,
    owner: draft.owner,
    revision: draft.revision,
    paused: draft.paused,
    name: (draft.name || '').trim(),
    listen: {
      address: (draft.listen.address || '').trim(),
      port: draft.listen.portMode === 'explicit' ? num(draft.listen.port) : 0,
      protocol: draft.listen.protocol,
      entry_hostname: draft.hops[0]?.node_refs.length > 1 ? (draft.listen.entry_hostname || '').trim() : ''
    },
    hops,
    targets: draft.targets.map(target => ({ host: (target.host || '').trim(), port: num(target.port), weight: num(target.weight), priority: num(target.priority) })),
    policy: {
      next_hop: draft.policy.next_hop,
      target: draft.policy.target,
      direct: draft.policy.direct,
      target_policy: draft.policy.target_policy,
      health: {
        interval_ms: num(draft.policy.health.interval_ms),
        timeout_ms: num(draft.policy.health.timeout_ms),
        disabled: Boolean(draft.policy.health.disabled)
      },
      circuit_breaker: {
        failure_threshold: num(draft.policy.circuit_breaker.failure_threshold),
        open_ms: num(draft.policy.circuit_breaker.open_ms)
      }
    },
    limits: {
      bandwidth_bps: bandwidth > 0 ? String(Math.round(bandwidth * 1e6)) : '',
      quota_bytes: quota > 0 ? String(Math.round(quota * GB)) : '',
      max_conns: num(draft.limits.max_conns),
      expires_at_unix_ms: fromLocalInput(draft.limits.expires) ? String(fromLocalInput(draft.limits.expires)) : ''
    },
    labels
  }
}

// missingRequired lists what the preview needs before it is worth asking
// (D4: never preview while a required field is empty).
export function missingRequired(draft) {
  const out = []
  if (!String(draft.name || '').trim()) out.push('name')
  if (!draft.hops.length) out.push('hops')
  draft.hops.forEach((hop, index) => {
    if (!hop.node_refs.length) out.push(`hops[${index}].node_refs`)
  })
  if (!draft.targets.length) out.push('targets')
  draft.targets.forEach((target, index) => {
    if (!String(target.host || '').trim()) out.push(`targets[${index}].host`)
    if (!num(target.port)) out.push(`targets[${index}].port`)
  })
  return out
}

// syncRoles sets each hop's role and ingress from its position.
export function syncRoles(hops) {
  hops.forEach((hop, index) => {
    hop.role = roleOf(index, hops.length)
    if (index === 0) {
      delete hop.ingress
    } else if (!hop.ingress) {
      hop.ingress = { security: 'LINK_SECURITY_RAW', mux: false, server_name: '', path: '' }
    }
  })
  return hops
}

// ---------------------------------------------------------------------------
// Conflicts (D10)
// ---------------------------------------------------------------------------

function flatten(value, prefix = '', out = {}) {
  if (Array.isArray(value)) {
    if (!value.length) out[prefix] = '[]'
    value.forEach((item, index) => flatten(item, `${prefix}[${index}]`, out))
  } else if (value && typeof value === 'object') {
    const keys = Object.keys(value).sort()
    if (!keys.length && prefix) out[prefix] = '{}'
    for (const key of keys) flatten(value[key], prefix ? `${prefix}.${key}` : key, out)
  } else if (value !== undefined && value !== null && value !== '' && value !== false && value !== 0) {
    out[prefix] = String(value)
  }
  return out
}

const CONFLICT_IGNORED = /^(revision|updated_at_unix_ms|created_at_unix_ms|id|owner)$/

// routeDiff compares three versions of a route body: what the editor
// loaded (base), what is stored now (theirs) and the form (mine). It lists
// each field someone else changed, and whether the form changed it too.
export function routeDiff(base, theirs, mine) {
  const a = flatten(base)
  const b = flatten(theirs)
  const c = flatten(mine)
  const fields = new Set([...Object.keys(a), ...Object.keys(b)])
  const out = []
  for (const field of [...fields].sort()) {
    if (CONFLICT_IGNORED.test(field)) continue
    if (a[field] === b[field]) continue
    out.push({ field, before: a[field] ?? '', theirs: b[field] ?? '', mine: c[field] ?? '', overlap: c[field] !== a[field] && c[field] !== b[field] })
  }
  return out
}

// ---------------------------------------------------------------------------
// Bulk (D9)
// ---------------------------------------------------------------------------

// runBulk calls task(item) for each item, at most `concurrency` at a time,
// and answers {done, failed: [{item, error}]} in the items' order.
export async function runBulk(items, task, { concurrency = 4 } = {}) {
  const results = new Array(items.length)
  let next = 0
  async function worker() {
    while (next < items.length) {
      const index = next++
      try {
        await task(items[index], index)
        results[index] = { ok: true }
      } catch (error) {
        results[index] = { ok: false, error }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, items.length) }, worker))
  return {
    done: items.filter((_, index) => results[index].ok),
    failed: items.map((item, index) => ({ item, error: results[index].error, ok: results[index].ok })).filter(entry => !entry.ok).map(({ item, error }) => ({ item, error }))
  }
}

// ---------------------------------------------------------------------------
// Diagnosis (F3c)
// ---------------------------------------------------------------------------

export const PROBE_STAGES = Object.freeze([
  { key: 'control', match: step => step.vantage !== 'DIAGNOSE_VANTAGE_NODE' && ['PROBE_KIND_CONFIG', 'PROBE_KIND_HEALTH'].includes(step.kind) },
  { key: 'node', match: step => step.vantage === 'DIAGNOSE_VANTAGE_NODE' },
  { key: 'controlProbe', match: step => step.vantage !== 'DIAGNOSE_VANTAGE_NODE' && !['PROBE_KIND_CONFIG', 'PROBE_KIND_HEALTH'].includes(step.kind) }
])

export const PROBE_TONES = Object.freeze({
  PROBE_STATUS_OK: 'success',
  PROBE_STATUS_FAILED: 'danger',
  PROBE_STATUS_INCONCLUSIVE: 'warning',
  PROBE_STATUS_SKIPPED: 'neutral'
})

// probeStatus reads a step's verdict; old answers had only ok.
export function probeStatus(step) {
  const status = step?.result?.status
  if (status && status !== 'PROBE_STATUS_UNSPECIFIED') return status
  return step?.result?.ok ? 'PROBE_STATUS_OK' : 'PROBE_STATUS_FAILED'
}

// diagnosisStages groups the steps: Control's records, node probes, then
// Control's own probes.
export function diagnosisStages(result) {
  const steps = result?.steps || []
  return PROBE_STAGES.map(stage => ({ key: stage.key, steps: steps.filter(stage.match) }))
}
