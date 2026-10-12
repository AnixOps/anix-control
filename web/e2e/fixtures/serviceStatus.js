import { FIXED_NOW_MS } from './clock.js'

// The machine-telemetry services summary of the nodes in view (GET
// /api/v3/plugins/machine-telemetry/nodes/services?ids=101,102,...): one
// entry per node asked for, for the node list's Services column. Node ids are
// those of nodeData.js (101 to 112); a node with no entry here has
// collection off.
const iso = offset => new Date(FIXED_NOW_MS + offset).toISOString()
const summary = (total, active, failed) => ({ total, active, failed, inactive: total - active - failed })
const reported = (counts, extra = {}) => ({
  enabled: true, reported: true, supported: true, unsupported_reason: '', stale: false,
  observed_at: iso(-60_000), summary: counts, ...extra
})

const BY_ID = {
  101: reported(summary(4, 4, 0)),
  102: reported(summary(3, 2, 1)),
  103: reported(summary(5, 4, 0)),
  104: { enabled: true, reported: false, supported: false, unsupported_reason: '', stale: false, observed_at: null, summary: summary(0, 0, 0) },
  105: reported(summary(3, 3, 0), { stale: true, observed_at: iso(-3_600_000) }),
  106: reported(summary(0, 0, 0), { supported: false, unsupported_reason: 'cgroup v1' }),
  108: reported(summary(2, 2, 0)),
  110: reported(summary(6, 6, 0))
}

const off = { enabled: false, reported: false, supported: false, unsupported_reason: '', stale: false, observed_at: null, summary: summary(0, 0, 0) }

export function serviceStatusAnswer(query) {
  const ids = String(query.ids || '').split(',').map(Number).filter(Number.isSafeInteger)
  return { data: { nodes: ids.map(id => ({ node_id: id, ...(BY_ID[id] || off) })) } }
}
