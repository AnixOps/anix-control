// How a node's Agent reaches Control and the state of its Agent certificate,
// read from the transport inventory (GET /api/v4/kernel/agents/transports,
// docs/architecture/node-ops-service.md). The node list asks for the nodes
// of one page, the node page for one node; both join the answer to the v2
// node by its inventory name, `proxy-<id>` (Node, not ForwardNode).

export const CONNECTION_TYPES = Object.freeze(['mtls_stream', 'apikey_stream', 'legacy', 'third_party', 'offline'])
export const CERTIFICATE_STATES = Object.freeze(['valid', 'revoked', 'expired', 'none'])

// The inventory's name of a proxy node.
export function nodeRef(id) {
  return `proxy-${id}`
}

// The answer's nodes by their `node` name.
export function indexTransports(inventory) {
  const nodes = Array.isArray(inventory?.nodes) ? inventory.nodes : []
  return new Map(nodes.filter(node => node && node.node).map(node => [String(node.node), node]))
}

// The chip's tone. A mutual-TLS stream is the target state; an API key stream
// is an Agent that has not enrolled yet; a legacy channel is refused once
// agent_control.mtls is required; third-party node software is not an Agent
// channel at all; offline is plain, because the node's status badge already
// says so.
export function connectionTone(type) {
  switch (type) {
    case 'mtls_stream': return 'success'
    case 'apikey_stream': return 'info'
    case 'legacy': return 'warning'
    default: return 'neutral'
  }
}

export function connectionTypeOf(entry) {
  const type = entry?.connection?.type
  return CONNECTION_TYPES.includes(type) ? type : 'offline'
}

function timeOf(value) {
  const time = value ? Date.parse(value) : NaN
  return Number.isFinite(time) ? time : null
}

// The node's certificate as the screens show it. `last_certificate` is the
// newest record in any state (so a revoked or expired one still shows when it
// ended and why); `certificate_state` says which state that is. A valid
// certificate past `renew_after` means the Agent did not renew in time.
export function certificateOf(entry, now = Date.now()) {
  const state = CERTIFICATE_STATES.includes(entry?.certificate_state) ? entry.certificate_state : 'none'
  const record = entry?.last_certificate || entry?.certificate || null
  const renewAfter = record?.renew_after || null
  const overdue = state === 'valid' && timeOf(renewAfter) !== null && timeOf(renewAfter) < now
  let tone = 'neutral'
  if (state === 'valid') tone = overdue ? 'warning' : 'success'
  else if (state === 'revoked' || state === 'expired') tone = 'danger'
  return {
    state,
    overdue,
    tone,
    serial: record?.serial || '',
    issuedAt: record?.issued_at || null,
    notAfter: record?.not_after || null,
    renewAfter,
    revokedAt: record?.revoked_at || null,
    revokeReason: record?.revoke_reason || ''
  }
}
