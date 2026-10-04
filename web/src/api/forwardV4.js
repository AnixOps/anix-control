// Client of the forward package's v4 API, /api/v4/forward/* (F5a,
// docs/forwarding/v4-api.md), for the forwarding pages (F5b).
//
// Conventions the API sets and this module keeps:
// - answers are {data: ...}; refusals {error: {code, message, violations}},
//   turned into a ForwardApiError here;
// - contract messages are protojson: proto field names, enum names, 64-bit
//   integers as strings, unset fields left out. A request with an unknown
//   field is refused, so the route and settings bodies are built from a
//   whitelist (routeBody, settingsBody) that sends only known fields;
// - every write carries an Idempotency-Key (the caller passes one to retry
//   the same request; otherwise a fresh one is made).
import request from '@/utils/request'

export const FORWARD_V4_BASE = '/api/v4/forward'
const PREVIEW_TIMEOUT_MS = 15_000
// The diagnosis runs up to 25 s on Control (its timeout_ms cap).
const DIAGNOSE_TIMEOUT_MS = 32_000
const READ_TIMEOUT_MS = 15_000
const WRITE_TIMEOUT_MS = 20_000
// GET /routes pages at most 1000 routes; the UI reads up to MAX_ROUTE_PAGES.
const ROUTE_PAGE_SIZE = 1000
const MAX_ROUTE_PAGES = 5

export class ForwardApiError extends Error {
  constructor({ status = 0, code = '', message = '', violations = [], canceled = false } = {}) {
    super(message || code || 'request failed')
    this.name = 'ForwardApiError'
    this.status = status
    this.code = code
    this.violations = Array.isArray(violations) ? violations : []
    this.canceled = canceled
  }
}

export function isCanceled(error) {
  return Boolean(error?.canceled)
}

// newIdempotencyKey: 1 to 128 bytes; a UUID where the browser has one.
export function newIdempotencyKey() {
  const cryptoObject = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined
  if (cryptoObject?.randomUUID) return `fwd-${cryptoObject.randomUUID()}`
  const random = Array.from({ length: 4 }, () => Math.floor(Math.random() * 0xffffffff).toString(16).padStart(8, '0')).join('')
  return `fwd-${Date.now().toString(16)}-${random}`
}

export function toApiError(error) {
  if (error instanceof ForwardApiError) return error
  if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError' || error?.name === 'AbortError') {
    return new ForwardApiError({ code: 'canceled', canceled: true })
  }
  const status = error?.response?.status || 0
  const body = error?.response?.data
  const failure = body && typeof body === 'object' ? body.error : null
  if (failure && typeof failure === 'object') {
    return new ForwardApiError({ status, code: String(failure.code || ''), message: String(failure.message || ''), violations: failure.violations })
  }
  if (status) return new ForwardApiError({ status, code: `http_${status}`, message: error?.message || '' })
  return new ForwardApiError({ code: error?.code === 'ECONNABORTED' ? 'timeout' : 'network', message: error?.message || '' })
}

async function call({ method = 'get', url, params, data, signal, timeout = READ_TIMEOUT_MS, idempotencyKey }) {
  const headers = {}
  const write = method !== 'get'
  if (write) headers['Idempotency-Key'] = idempotencyKey || newIdempotencyKey()
  try {
    const body = await request({ baseURL: FORWARD_V4_BASE, url, method, params, data, signal, timeout, headers })
    return body && typeof body === 'object' && 'data' in body ? body.data : body
  } catch (error) {
    throw toApiError(error)
  }
}

// ---------------------------------------------------------------------------
// Request bodies: only the fields the contract defines, unset ones left out.
// ---------------------------------------------------------------------------

function isUnset(value) {
  return value === undefined || value === null || value === '' || value === false || value === 0 ||
    (Array.isArray(value) && value.length === 0) ||
    (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0)
}

// pick copies the listed fields of source that are set. A field's spec is a
// kind: 'str', 'num' (32-bit), 'int64' (a decimal string), 'bool', 'enum',
// 'strs', 'nums', 'map', or a function for nested messages.
function pick(source, spec) {
  if (!source || typeof source !== 'object') return undefined
  const out = {}
  for (const [field, kind] of Object.entries(spec)) {
    const value = convert(source[field], kind)
    if (!isUnset(value)) out[field] = value
  }
  return Object.keys(out).length ? out : undefined
}

function convert(value, kind) {
  if (value === undefined || value === null) return undefined
  if (typeof kind === 'function') return kind(value)
  switch (kind) {
    case 'str':
    case 'enum':
      return typeof value === 'string' ? value.trim() : String(value)
    case 'num': {
      const number = Number(value)
      return Number.isFinite(number) && number > 0 ? Math.trunc(number) : undefined
    }
    case 'int64': {
      if (typeof value === 'bigint') return value > 0n ? value.toString() : undefined
      const text = String(value).trim()
      if (!/^\d+$/.test(text)) {
        const number = Number(value)
        return Number.isFinite(number) && number > 0 ? String(Math.trunc(number)) : undefined
      }
      return /^0+$/.test(text) ? undefined : text.replace(/^0+/, '')
    }
    case 'bool':
      return value === true
    case 'strs':
      return Array.isArray(value) ? value.map(item => String(item).trim()).filter(Boolean) : undefined
    case 'nums':
      return Array.isArray(value) ? value.map(Number).filter(number => Number.isInteger(number) && number > 0) : undefined
    case 'map': {
      if (typeof value !== 'object') return undefined
      const out = {}
      for (const [key, item] of Object.entries(value)) {
        const name = String(key).trim()
        if (name) out[name] = String(item ?? '')
      }
      return out
    }
    default:
      return undefined
  }
}

const linkBody = value => pick(value, { security: 'enum', mux: 'bool', server_name: 'str', path: 'str' })
const hopBody = value => pick(value, { role: 'enum', engine: 'enum', node_refs: 'strs', ingress: linkBody, port: 'num', dial_address: 'str' }) || {}
const targetBody = value => pick(value, { host: 'str', port: 'num', weight: 'num', priority: 'num' }) || {}
const policyBody = value => pick(value, {
  next_hop: 'enum',
  target: 'enum',
  health: health => pick(health, { interval_ms: 'num', timeout_ms: 'num', disabled: 'bool' }),
  circuit_breaker: breaker => pick(breaker, { failure_threshold: 'num', open_ms: 'num' }),
  direct: 'enum',
  target_policy: 'enum'
})
const limitsBody = value => pick(value, { bandwidth_bps: 'int64', quota_bytes: 'int64', max_conns: 'num', expires_at_unix_ms: 'int64' })

// routeBody is a Route as the API takes it (POST /routes, PUT /routes/{id},
// the route of POST /routes/preview).
export function routeBody(route) {
  return pick(route, {
    id: 'str',
    owner: 'str',
    name: 'str',
    listen: listen => pick(listen, { address: 'str', port: 'num', protocol: 'enum', entry_hostname: 'str' }),
    hops: hops => (Array.isArray(hops) ? hops.map(hopBody) : undefined),
    targets: targets => (Array.isArray(targets) ? targets.map(targetBody) : undefined),
    policy: policyBody,
    limits: limitsBody,
    labels: 'map',
    paused: 'bool',
    revision: 'int64'
  }) || {}
}

// settingsBody is a NodeSettings (PUT /nodes/{ref}/settings).
export function settingsBody(settings) {
  return pick(settings, {
    port_range: range => pick(range, { first: 'num', last: 'num' }),
    reserved_ports: 'nums',
    addresses: 'strs',
    labels: 'map'
  }) || {}
}

// nodeRecordBody is a ForwardNodeRecord (POST /nodes).
export function nodeRecordBody(record) {
  return pick(record, {
    name: 'str', host: 'str', transport: 'enum', port: 'num', region: 'str', isp: 'str', enabled: 'bool'
  }) || {}
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

// listRoutes reads every page (up to MAX_ROUTE_PAGES of 1000). The answer:
// {routes: [{route, enforced}], canDelete, truncated}. canDelete is null
// when the package predates F5b's can_delete.
export async function listRoutes({ nodeRef = '', signal } = {}) {
  const routes = []
  let token = ''
  let canDelete = null
  for (let page = 0; page < MAX_ROUTE_PAGES; page++) {
    const params = { page_size: ROUTE_PAGE_SIZE }
    if (nodeRef) params.node_ref = nodeRef
    if (token) params.page_token = token
    const answer = await call({ url: '/routes', params, signal })
    routes.push(...(answer?.routes || []))
    if (typeof answer?.can_delete === 'boolean') canDelete = answer.can_delete
    token = answer?.next_page_token || ''
    if (!token) return { routes, canDelete, truncated: false }
  }
  return { routes, canDelete, truncated: true }
}

export function getRoute(id, { signal } = {}) {
  return call({ url: `/routes/${encodeURIComponent(id)}`, signal })
}

export function createRoute(route, { idempotencyKey } = {}) {
  const body = routeBody(route)
  delete body.id
  delete body.revision
  return call({ method: 'post', url: '/routes', data: body, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function updateRoute(id, route, { idempotencyKey } = {}) {
  return call({ method: 'put', url: `/routes/${encodeURIComponent(id)}`, data: routeBody(route), idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function deleteRoute(id, { idempotencyKey } = {}) {
  return call({ method: 'delete', url: `/routes/${encodeURIComponent(id)}`, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function pauseRoute(id, { idempotencyKey } = {}) {
  return call({ method: 'post', url: `/routes/${encodeURIComponent(id)}/pause`, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function resumeRoute(id, { idempotencyKey } = {}) {
  return call({ method: 'post', url: `/routes/${encodeURIComponent(id)}/resume`, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

// previewRoute plans a route without storing it. Pass an AbortSignal to
// cancel a preview the editor no longer needs.
export function previewRoute(route, { signal } = {}) {
  const body = routeBody(route)
  delete body.revision
  return call({ method: 'post', url: '/routes/preview', data: { route: body }, signal, timeout: PREVIEW_TIMEOUT_MS })
}

export function routeStats(id, { since, until, signal } = {}) {
  return call({ url: `/routes/${encodeURIComponent(id)}/stats`, params: windowParams(since, until), signal })
}

export function routeHealth(id, { signal } = {}) {
  return call({ url: `/routes/${encodeURIComponent(id)}/health`, signal })
}

export function diagnoseRoute(id, { timeoutMs, idempotencyKey } = {}) {
  const data = timeoutMs ? { timeout_ms: timeoutMs } : undefined
  return call({ method: 'post', url: `/routes/${encodeURIComponent(id)}/diagnose`, data, idempotencyKey, timeout: DIAGNOSE_TIMEOUT_MS })
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

// listNodes answers {nodes, canDelete}; canDelete is null on a package
// that predates it.
export async function listNodes({ kind = '', signal } = {}) {
  const answer = await call({ url: '/nodes', params: kind ? { kind } : undefined, signal })
  return { nodes: answer?.nodes || [], canDelete: typeof answer?.can_delete === 'boolean' ? answer.can_delete : null }
}

export function getNode(ref, { signal } = {}) {
  return call({ url: `/nodes/${encodeURIComponent(ref)}`, signal })
}

export function createNode(record, settings, { idempotencyKey } = {}) {
  const data = { node: nodeRecordBody(record) }
  const body = settings ? settingsBody(settings) : null
  if (body && Object.keys(body).length) data.settings = body
  return call({ method: 'post', url: '/nodes', data, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function setNodeSettings(ref, settings, { idempotencyKey } = {}) {
  return call({ method: 'put', url: `/nodes/${encodeURIComponent(ref)}/settings`, data: settingsBody(settings), idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function toggleNode(ref, enabled, { idempotencyKey } = {}) {
  return call({ method: 'post', url: `/nodes/${encodeURIComponent(ref)}/toggle`, data: { enabled: Boolean(enabled) }, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

export function deleteNode(ref, { idempotencyKey } = {}) {
  return call({ method: 'delete', url: `/nodes/${encodeURIComponent(ref)}`, idempotencyKey, timeout: WRITE_TIMEOUT_MS })
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

function windowParams(since, until) {
  const params = {}
  if (since) params.since = String(Math.trunc(since))
  if (until) params.until = String(Math.trunc(until))
  return params
}

// stats: {totals, nodes, series, truncated}. totals and nodes are plain
// JSON numbers; series are protojson TrafficBuckets (64-bit as strings).
export function trafficStats({ since, until, routeId = '', nodeRef = '', signal } = {}) {
  const params = windowParams(since, until)
  if (routeId) params.route_id = routeId
  if (nodeRef) params.node_ref = nodeRef
  return call({ url: '/stats', params, signal })
}

export function observabilityTargets({ nodeRef = '', signal } = {}) {
  return call({ url: '/observability/targets', params: nodeRef ? { node_ref: nodeRef } : undefined, signal })
}
