// Data helpers of the forward node pages (NodeX nodes, legacy rules,
// Ansible machines, runtime jobs). They only read the answers of the
// existing endpoints; field names and request payloads are unchanged.

// NodeX forward node calls carry ?scope=nodex (the Ansible inventory is a
// separate scope with its own endpoints).
export const NODEX_SCOPE = Object.freeze({ params: Object.freeze({ scope: 'nodex' }) })

// The /admin/forward/* answers are `{ code, msg, data }`; a non-zero code is
// a failure even with HTTP 200.
export function unwrapForwardResponse(response, failureMessage = 'Request failed') {
  if (!response) return {}
  if (typeof response.code === 'number') {
    if (response.code !== 0) throw new Error(response.msg || failureMessage)
    return response.data ?? response
  }
  if (response.data && typeof response.data === 'object' && !Array.isArray(response.data)) {
    return response.data
  }
  return response
}

// Runtime and Ansible answers may be wrapped once or twice.
export function unwrapPayload(response) {
  return response?.data?.data ?? response?.data ?? response
}

export function listOf(payload) {
  return Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
}

export function errorMessage(error, fallback, translate = value => value) {
  const message =
    error?.response?.data?.error ||
    error?.response?.data?.message ||
    error?.response?.data?.msg ||
    error?.message ||
    fallback
  return translate(message) || fallback
}

export function parsePositiveInt(value) {
  const text = String(value ?? '').trim()
  if (!text) return null
  const parsed = Number(text)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null
}

export function parseNonNegativeInt(value) {
  const text = String(value ?? '').trim()
  if (!text) return null
  const parsed = Number(text)
  return Number.isFinite(parsed) && parsed >= 0 ? Math.trunc(parsed) : null
}

export function isPort(value) {
  const port = parsePositiveInt(value)
  return Boolean(port) && port <= 65535
}

// <input type="datetime-local"> value in local time.
export function toDateTimeLocal(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = input => String(input).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function toISOStringOrNull(value) {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

export function normalizeNode(item = {}) {
  const type = String(item.type || 'relay').toLowerCase() === 'exit' ? 'exit' : 'relay'
  return {
    id: Number(item.id ?? 0) || 0,
    name: item.name || `node-${item.id ?? 'new'}`,
    type,
    host: item.host || '-',
    port: Number(item.port ?? 0) || 0,
    apiPort: Number(item.api_port ?? item.apiPort ?? 0) || '',
    apiToken: item.api_token ?? item.apiToken ?? '',
    metricsPort: Number(item.metrics_port ?? item.metricsPort ?? 0) || '',
    region: item.region || '',
    isp: item.isp || '',
    bandwidth: Number(item.bandwidth ?? 0) || 0,
    weight: Number(item.weight ?? 1) || 1,
    maxConn: Number(item.max_conn ?? item.maxConn ?? 0) || 0,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : Number(item.enabled ?? 0) === 1,
    status: Number(item.status ?? 0) || 0,
    lastCheck: item.last_check ?? item.lastCheck ?? null,
    latency: Number(item.latency ?? 0) || 0,
    currentConn: Number(item.current_conn ?? item.currentConn ?? 0) || 0,
    totalUpload: Number(item.total_upload ?? item.totalUpload ?? 0) || 0,
    totalDownload: Number(item.total_download ?? item.totalDownload ?? 0) || 0,
    uptime: Number(item.uptime ?? 0) || 0
  }
}

export function normalizeRule(item = {}) {
  const relayNodeId = Number(item.relay_node_id ?? item.relayNodeID ?? item.relayNodeId ?? 0) || 0
  const exitNodeId = Number(item.exit_node_id ?? item.exitNodeID ?? item.exitNodeId ?? 0) || 0
  return {
    id: Number(item.id ?? 0) || 0,
    name: item.name || `rule-${item.id ?? 'new'}`,
    relayNodeId,
    exitNodeId,
    relayName: item.relay_node?.name || item.relayNode?.name || (relayNodeId ? `#${relayNodeId}` : '-'),
    exitName: item.exit_node?.name || item.exitNode?.name || (exitNodeId ? `#${exitNodeId}` : '-'),
    listenPort: Number(item.listen_port ?? item.listenPort ?? 0) || 0,
    protocol: String(item.protocol || 'tcp').toLowerCase(),
    targetHost: item.target_host ?? item.targetHost ?? '',
    targetPort: Number(item.target_port ?? item.targetPort ?? 0) || 0,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : Number(item.enabled ?? 0) === 1,
    userId: Number(item.user_id ?? item.userId ?? 0) || null,
    userGroupId: Number(item.user_group_id ?? item.userGroupId ?? 0) || null,
    speedLimit: item.speed_limit ?? item.speedLimit ?? null,
    trafficLimit: item.traffic_limit ?? item.trafficLimit ?? null,
    expireTime: item.expire_time ?? item.expireTime ?? null,
    upload: Number(item.upload ?? 0) || 0,
    download: Number(item.download ?? 0) || 0,
    connections: Number(item.connections ?? 0) || 0,
    updatedAt: item.updated_at ?? item.updatedAt ?? item.created_at ?? item.createdAt ?? null,
    remark: item.remark || ''
  }
}

export function normalizeMachine(node) {
  return {
    id: Number(node?.id || 0),
    name: node?.name || '-',
    host: node?.host || '-',
    port: Number(node?.port || 0),
    region: node?.region || '',
    isp: node?.isp || '',
    weight: Number(node?.weight || 1),
    enabled: Boolean(node?.enabled ?? true),
    status: Number(node?.status ?? 0),
    currentConn: Number(node?.current_conn || node?.currentConn || 0),
    totalUpload: Number(node?.total_upload || node?.totalUpload || 0),
    totalDownload: Number(node?.total_download || node?.totalDownload || 0)
  }
}

export function normalizeRuntimeJob(job, unknownLabel = '') {
  return {
    id: job?.id,
    action: job?.action || unknownLabel,
    forwardId: job?.forwardId ?? job?.forward_id ?? null,
    tunnelId: job?.tunnelId ?? job?.tunnel_id ?? null,
    nodeId: job?.nodeId ?? job?.node_id ?? null,
    status: Number(job?.status ?? 0),
    message: String(job?.result || job?.error || '').trim(),
    completedAt: job?.completedAt ?? job?.completed_at ?? null,
    updatedAt: job?.updatedAt ?? job?.updated_at ?? null,
    createdAt: job?.createdAt ?? job?.created_at ?? null
  }
}

export function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) return null
  if (['1', 'true', 'yes', 'on'].includes(normalized)) return true
  if (['0', 'false', 'no', 'off'].includes(normalized)) return false
  return null
}
