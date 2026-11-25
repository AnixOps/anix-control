// Helpers of the 组网 page (internal/meshregion): the members table rows
// and the summary of a node's last mesh status
// (docs/design/mesh-regions/agent-contract.md).

// A status older than this means the node's control link is lost: the
// Agent reports every minute.
export const STATUS_STALE_MS = 3 * 60 * 1000

// memberRows answers one row per node (its home membership) with its home
// region and, for a boundary node, its addresses in the other regions;
// keyWarnings are the warnings of all its memberships, once each.
export function memberRows(regions) {
  const byNode = new Map()
  for (const region of regions) {
    for (const member of region.members || []) {
      const entry = byNode.get(member.node_ref) || { others: [], keyWarnings: [] }
      if (member.home) Object.assign(entry, member, { region, others: entry.others, keyWarnings: entry.keyWarnings })
      else entry.others.push({ region, address: member.address })
      for (const code of Array.isArray(member.warnings) ? member.warnings : []) {
        if (!entry.keyWarnings.includes(code)) entry.keyWarnings.push(code)
      }
      byNode.set(member.node_ref, entry)
    }
  }
  return [...byNode.values()].filter(row => row.region)
}

// statusSummary answers { state, alerts } of a member: state is none,
// lost, running, pending (a network the kernel holds until the admission
// list names its key, failure native_keys_pending) or stopped; alerts the
// codes of every network's alerts and of its failure (such as
// peer_not_native).
export function statusSummary(member, now = Date.now()) {
  if (!member?.status) return { state: 'none', alerts: [] }
  const networks = Array.isArray(member.status.networks) ? member.status.networks : []
  const codes = network => [...(Array.isArray(network?.alerts) ? network.alerts : []).map(alert => alert?.code), network?.failure?.code]
  const alerts = [...new Set(networks.flatMap(codes).filter(code => typeof code === 'string' && code))]
  const received = Date.parse(member.status_received_at || '')
  if (Number.isFinite(received) && now - received > STATUS_STALE_MS) return { state: 'lost', alerts }
  const running = networks.length > 0 && networks.every(network => network?.running)
  if (running) return { state: 'running', alerts }
  const held = network => network?.failure?.code === 'native_keys_pending' ||
    (Array.isArray(network?.alerts) && network.alerts.some(alert => alert?.code === 'native_keys_pending'))
  const pending = networks.length > 0 && networks.every(network => network?.running || held(network))
  return { state: pending ? 'pending' : 'stopped', alerts }
}

// deviceStatus answers { state, alerts } of an enrolled device without an
// Agent: its last report's summary, or lost when it has not reported and
// has not been seen for longer than STATUS_STALE_MS.
export function deviceStatus(device, now = Date.now()) {
  if (device?.status) return statusSummary(device, now)
  const seen = Date.parse(device?.last_seen_at || '')
  if (!Number.isFinite(seen) || now - seen > STATUS_STALE_MS) return { state: 'lost', alerts: [] }
  return { state: 'none', alerts: [] }
}

// inSharedAddressSpace tells whether cidr, an IPv4 range, overlaps
// 100.64.0.0/10, which other overlay software on the same machine often
// takes over whole (spec section 8.3.1).
export function inSharedAddressSpace(cidr) {
  const match = /^\s*(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})\s*$/.exec(String(cidr || ''))
  if (!match) return false
  const octets = match.slice(1, 5).map(Number)
  const bits = Number(match[5])
  if (octets.some(octet => octet > 255) || bits > 32) return false
  const address = octets.reduce((value, octet) => value * 256 + octet, 0)
  const size = 2 ** (32 - bits)
  const start = address - (address % size)
  const sharedStart = 100 * 2 ** 24 + 64 * 2 ** 16
  const sharedEnd = sharedStart + 2 ** 22
  return start < sharedEnd && sharedStart < start + size
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

// enrollmentState is used, expired or valid.
export function enrollmentState(enrollment, now = Date.now()) {
  if (enrollment.used_at) return 'used'
  return Date.parse(enrollment.expires_at) <= now ? 'expired' : 'valid'
}
