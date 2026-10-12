import { ref } from 'vue'
import { getNodesServicesSummary } from '@/api/machineTelemetry'

// The systemd services summary of the nodes in view, for the node list's
// 服务状态 column: one call to the machine-telemetry package after the list
// has loaded (it never holds the list up). The package host answers it, so
// it is there only with the plugin host running and machine-telemetry
// installed; without them, or without the package's API permission
// (machine-telemetry.api, which the kernel gateway checks), the call fails,
// the column shows a dash and the page works as before.
//
// `status` is 'idle', 'loading', 'ready' or 'unavailable'; `entries` maps a
// node id to its summary. A late answer to an earlier request (another page)
// is dropped.
export function useNodeServiceStatus() {
  const entries = ref(new Map())
  const status = ref('idle')
  let sequence = 0

  async function load(ids) {
    const mine = ++sequence
    const nodeIDs = (Array.isArray(ids) ? ids : []).filter(id => id !== undefined && id !== null && id !== '')
    if (!nodeIDs.length) {
      entries.value = new Map()
      status.value = 'idle'
      return
    }
    status.value = 'loading'
    try {
      const nodes = await getNodesServicesSummary(nodeIDs)
      if (mine !== sequence) return
      entries.value = new Map(nodes.filter(node => node && node.node_id !== undefined).map(node => [Number(node.node_id), node]))
      status.value = 'ready'
    } catch {
      // No plugin host, the package not installed, or no permission: the
      // request logged its error, the column shows a dash.
      if (mine !== sequence) return
      entries.value = new Map()
      status.value = 'unavailable'
    }
  }

  function reset() {
    sequence += 1
    entries.value = new Map()
    status.value = 'idle'
  }

  return { entries, status, load, reset }
}

// The cell's state for one node's summary: what the 服务状态 column shows.
//   none        no data (not enabled, unavailable, still loading): a dash
//   waiting     enabled, no report yet
//   unsupported the node cannot report (no systemd or cgroup v2)
//   stale       the latest report is older than 25 minutes
//   failed      at least one unit failed
//   partial     no failure, but not every unit is running
//   empty       no unit matches the node's rules
//   healthy     every unit is running
export function serviceStatusState(entry) {
  if (!entry || !entry.enabled) return 'none'
  if (!entry.reported) return 'waiting'
  if (!entry.supported) return 'unsupported'
  if (entry.stale) return 'stale'
  const summary = entry.summary || {}
  const total = Number(summary.total || 0)
  if (Number(summary.failed || 0) > 0) return 'failed'
  if (total === 0) return 'empty'
  if (Number(summary.active || 0) < total) return 'partial'
  return 'healthy'
}
