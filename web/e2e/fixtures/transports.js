import { FIXED_NOW_MS } from './clock.js'

// The Agent transport inventory (GET /api/v4/kernel/agents/transports?node=
// proxy-101,proxy-102): connection type and certificate state per proxy
// node, derived from the fixed clock so the relative and absolute times are
// the same on every run. Node ids are those of nodeData.js (101 to 112).
const DAY = 86_400_000
const iso = offset => new Date(FIXED_NOW_MS + offset).toISOString()

function certificate({ issued = -2 * DAY, notAfter = 5 * DAY, renewAfter = 2.5 * DAY, revokedAt = null, reason = '', serial }) {
  return {
    serial,
    spiffe_id: `spiffe://anixops.example/agent/proxy-${serial.slice(0, 3)}`,
    issued_at: iso(issued),
    not_after: iso(notAfter),
    renew_after: iso(renewAfter),
    revoked_at: revokedAt === null ? null : iso(revokedAt),
    ...(reason ? { revoke_reason: reason } : {})
  }
}

const mtls = id => ({ state: 'valid', cert: certificate({ serial: `${id}a1` }), connection: { type: 'mtls_stream', transport: 'mtls-stream', last_seen_at: iso(-45_000) } })

// id → what the inventory knows of the node. A node with no entry is not in
// the answer (the route returns only the nodes asked for that it has).
const BY_ID = {
  101: mtls(101),
  102: { ...mtls(102), cert: certificate({ serial: '102a1', issued: -5 * DAY, notAfter: 2 * DAY, renewAfter: -1 * DAY }) },
  103: { state: 'none', cert: null, connection: { type: 'apikey_stream', transport: 'apikey-stream', last_seen_at: iso(-30_000) } },
  104: { state: 'none', cert: null, connection: { type: 'legacy', transport: 'http-legacy', last_seen_at: iso(-90_000) } },
  105: { state: 'expired', cert: certificate({ serial: '105a1', issued: -12 * DAY, notAfter: -3 * DAY, renewAfter: -7 * DAY }), connection: { type: 'offline', transport: '', last_seen_at: null } },
  106: { state: 'none', cert: null, connection: { type: 'third_party', transport: 'uniproxy', last_seen_at: iso(-120_000) } },
  107: { state: 'revoked', cert: certificate({ serial: '107a1', issued: -6 * DAY, notAfter: DAY, renewAfter: -2 * DAY, revokedAt: -2 * DAY, reason: 'node disabled' }), connection: { type: 'offline', transport: '', last_seen_at: null } },
  108: mtls(108),
  109: mtls(109),
  110: { ...mtls(110), connection: { type: 'mtls_stream', transport: 'mtls-stream', last_seen_at: iso(-20_000) } },
  111: mtls(111),
  112: { state: 'none', cert: null, connection: { type: 'offline', transport: '', last_seen_at: null } }
}

function nodeRow(id, entry) {
  const valid = entry.state === 'valid'
  return {
    node: `proxy-${id}`,
    kind: 'proxy',
    id,
    name: `node-${id}`,
    enabled: id !== 107,
    status: entry.connection.type === 'mtls_stream' ? 'mtls' : entry.connection.type === 'legacy' ? 'legacy' : entry.connection.type === 'third_party' ? 'third-party' : 'unseen',
    transport: entry.connection.transport,
    agent_version: 'v1.4.2',
    last_seen_at: entry.connection.last_seen_at,
    certificate: valid ? entry.cert : null,
    certificate_state: entry.state,
    last_certificate: entry.cert,
    connection: entry.connection,
    transports: [],
    session: null
  }
}

// The answer for `query.node` (comma separated or repeated); no filter is
// every node known here.
export function transportsAnswer(query = {}) {
  const asked = String(query.node || '').split(',').map(name => name.trim()).filter(Boolean)
  const ids = Object.keys(BY_ID).map(Number).filter(id => !asked.length || asked.includes(`proxy-${id}`))
  const nodes = ids.map(id => nodeRow(id, BY_ID[id]))
  return {
    data: {
      mode: 'preferred',
      summary: { total: nodes.length, mtls: 0, legacy: 0, third_party: 0, unseen: 0, ready_for_required: false, required_reasons: [], required_blockers: [] },
      nodes
    }
  }
}
