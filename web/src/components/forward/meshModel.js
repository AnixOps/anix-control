// Helpers of the 组网 page (internal/meshregion): the members table rows
// and the summary of a node's last mesh status
// (docs/design/mesh-regions/agent-contract.md).

// A status older than this means the node's control link is lost: the
// Agent reports every minute.
export const STATUS_STALE_MS = 3 * 60 * 1000

// memberRows answers one row per node (its home membership) with its home
// region and, for a boundary node, its addresses in the other regions.
export function memberRows(regions) {
  const byNode = new Map()
  for (const region of regions) {
    for (const member of region.members || []) {
      const entry = byNode.get(member.node_ref) || { others: [] }
      if (member.home) Object.assign(entry, member, { region })
      else entry.others.push({ region, address: member.address })
      byNode.set(member.node_ref, entry)
    }
  }
  return [...byNode.values()].filter(row => row.region)
}

// statusSummary answers { state, alerts } of a member: state is none,
// lost, running or stopped; alerts the codes of every network's alerts.
export function statusSummary(member, now = Date.now()) {
  if (!member?.status) return { state: 'none', alerts: [] }
  const networks = Array.isArray(member.status.networks) ? member.status.networks : []
  const alerts = [...new Set(networks.flatMap(network => (network.alerts || []).map(alert => alert.code).filter(Boolean)))]
  const received = Date.parse(member.status_received_at || '')
  if (Number.isFinite(received) && now - received > STATUS_STALE_MS) return { state: 'lost', alerts }
  const running = networks.length > 0 && networks.every(network => network.running)
  return { state: running ? 'running' : 'stopped', alerts }
}

// boundaryCount is the number of boundary nodes in a region.
export function boundaryCount(region) {
  return (region.members || []).filter(member => !member.home || member.boundary).length
}

export function meshErrorMessage(error) {
  const answer = error?.response?.data
  return answer?.error?.message || answer?.message || error?.message || String(error)
}

export function isFeatureDisabled(error) {
  return error?.response?.status === 404 && error?.response?.data?.error?.code === 'feature_disabled'
}

// deviceRows answers one row per enrolled device with its region.
export function deviceRows(regions) {
  return regions.flatMap(region => (region.devices || []).map(device => ({ ...device, region })))
}

// enrollmentState is used, expired or valid.
export function enrollmentState(enrollment, now = Date.now()) {
  if (enrollment.used_at) return 'used'
  return Date.parse(enrollment.expires_at) <= now ? 'expired' : 'valid'
}
