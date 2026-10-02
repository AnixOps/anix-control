// Shared helpers of the node list and the node detail page (UI U7): how the
// admin API's answers are read, what a node's status means, and the error
// text of a failed request. No requests here.

export const BYTES_PER_GB = 1024 * 1024 * 1024

// A heartbeat in the last five minutes means online (model.Node.IsOnline).
export const ONLINE_WINDOW_SECONDS = 300

export const NODE_STATUS = Object.freeze({ pending: 0, online: 1, offline: 2, disabled: 3 })
const STATUS_KEYS = ['pending', 'online', 'offline', 'disabled']

// Sections of the node page, in order; ?section= names one.
export const NODE_SECTIONS = ['overview', 'protocols', 'credentials', 'deploy', 'logs', 'danger']

// The answer's data: a panel envelope ({ code, data }), axios ({ data }),
// a nested axios envelope ({ data: { data } }) or the value itself.
export function readNodePayload(res) {
  if (!res || typeof res !== 'object') {
    return null
  }
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? null
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

export function readNodeList(res) {
  const payload = readNodePayload(res)
  if (Array.isArray(payload)) return payload
  if (payload && Array.isArray(payload.list)) return payload.list
  return []
}

export function readNodePage(res) {
  const payload = readNodePayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

export function readNodeApiError(error) {
  const response = error?.response?.data
  return response?.error || response?.message || response?.msg || error?.message || String(error)
}

export function normalizeNode(node) {
  return {
    ...node,
    address: node.address || node.host || '',
    traffic_today: node.traffic_today || (Number(node.total_upload || 0) + Number(node.total_download || 0))
  }
}

// The list endpoint already reports online/offline from the last heartbeat;
// the single-node endpoint returns the stored column, so the detail page
// applies the same rule (service.NodeService.GetNodes).
export function displayStatus(node, now = Date.now()) {
  const stored = Number(node?.status ?? 0)
  if (stored === NODE_STATUS.disabled) return stored
  const last = Number(node?.last_check_at || 0)
  if (last > 0 && Math.floor(now / 1000) - last < ONLINE_WINDOW_SECONDS) return NODE_STATUS.online
  if (stored === NODE_STATUS.online) return NODE_STATUS.offline
  return stored
}

// 'pending' | 'online' | 'offline' | 'disabled' (UiBadge status names).
export function statusName(status) {
  return STATUS_KEYS[Number(status)] || 'pending'
}

export function isOverQuota(node) {
  if (!node?.monthly_limit) return false
  return (node.monthly_upload || 0) + (node.monthly_download || 0) > node.monthly_limit
}

export function monthlyUsed(node) {
  return Number(node?.monthly_upload || 0) + Number(node?.monthly_download || 0)
}

export function splitTags(tags) {
  return String(tags || '').split(',').map(tag => tag.trim()).filter(Boolean)
}

// The body of POST /admin/nodes and PUT /admin/nodes/:id, as the node form
// has always sent it; `status` only when editing.
export function buildNodePayload(form, { editing = false } = {}) {
  const payload = {
    name: form.name,
    host: form.address,
    tags: form.tags,
    rate: Number(form.rate),
    sort: Number(form.sort),
    parent_id: form.parent_id || null,
    monthly_limit: form.monthly_limit_gb ? Math.round(Number(form.monthly_limit_gb) * BYTES_PER_GB) : null,
    monthly_reset_day: Number(form.monthly_reset_day) || 1
  }
  if (editing) payload.status = Number(form.status)
  return payload
}

export function nodeFormFrom(node) {
  if (!node) {
    return {
      name: '',
      address: '',
      tags: '',
      rate: 1.0,
      sort: 0,
      status: 0,
      parent_id: null,
      monthly_limit_gb: null,
      monthly_reset_day: 1
    }
  }
  return {
    name: node.name,
    address: node.address || node.host || '',
    tags: node.tags || '',
    rate: node.rate || 1.0,
    sort: node.sort || 0,
    status: node.status,
    parent_id: node.parent_id || null,
    monthly_limit_gb: node.monthly_limit ? node.monthly_limit / BYTES_PER_GB : null,
    monthly_reset_day: node.monthly_reset_day || 1
  }
}

// Parent candidates: every node but the one being edited and its
// descendants, so the chain cannot loop.
export function parentCandidatesFor(nodes, editing) {
  if (!editing) return nodes
  const excluded = new Set([editing.id])
  let changed = true
  while (changed) {
    changed = false
    for (const n of nodes) {
      if (n.parent_id && excluded.has(n.parent_id) && !excluded.has(n.id)) {
        excluded.add(n.id)
        changed = true
      }
    }
  }
  return nodes.filter(n => !excluded.has(n.id))
}

// Remembered list query (search, status, page), so 返回节点 lands where the
// administrator left the list.
let lastListQuery = {}
export function rememberListQuery(query) {
  lastListQuery = { ...query }
}
export function listQuery() {
  return { ...lastListQuery }
}

// Names of the nodes the list showed last, so the node page can name a
// parent without another request.
const knownNames = new Map()
export function rememberNodeNames(nodes) {
  for (const node of nodes) knownNames.set(node.id, node.name)
}
export function knownNodeName(id) {
  return knownNames.get(id) || ''
}
