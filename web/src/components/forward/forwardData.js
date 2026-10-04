import { getNode, listNodes, listRoutes, observabilityTargets, trafficStats } from '@/api/forwardV4'
import { entryTraffic, num, routeStatus, statusContext, trafficWindow } from './routeModel'

// GET /stats refreshes at most every 60 s (D5), whatever the page polls.
const STATS_MAX_AGE_MS = 60_000
// Hop errors are read from the latest reports of at most this many nodes.
const MAX_ERROR_NODES = 20

let statsCache = null

export function resetForwardDataCache() {
  statsCache = null
}

async function cachedStats(window, now) {
  if (statsCache && statsCache.since === window.since && now - statsCache.at < STATS_MAX_AGE_MS) return statsCache.value
  const value = await trafficStats(window)
  statsCache = { at: now, since: window.since, value }
  return value
}

// loadForwardSnapshot reads what the overview and the route list show in
// one go: the routes, the nodes, one GET /stats for the last 24 hours
// (D6), the targets' health and the hop errors of the nodes that report
// some. Parts that fail leave their field empty and set its error.
export async function loadForwardSnapshot({ now = Date.now() } = {}) {
  const window = trafficWindow(now, 24)
  const [routesAnswer, nodesAnswer, statsAnswer, targetsAnswer] = await Promise.all([
    listRoutes(),
    listNodes(),
    cachedStats(window, now).catch(error => ({ error })),
    observabilityTargets().catch(error => ({ error }))
  ])
  const nodes = nodesAnswer.nodes
  const errorNodes = nodes.filter(node => num(node.hop_errors) > 0).slice(0, MAX_ERROR_NODES)
  const details = await Promise.all(errorNodes.map(node => getNode(node.node_ref).catch(() => null)))
  const hopErrors = details.flatMap(detail => (detail?.report?.errors || []).map(error => ({
    ...error,
    hop_index: num(error.hop_index),
    node_ref: detail?.node?.node_ref || ''
  })))
  const targets = targetsAnswer?.error ? [] : (targetsAnswer?.targets || [])
  const context = statusContext({ nodes, hopErrors, targets })
  const traffic = statsAnswer?.error ? new Map() : entryTraffic(statsAnswer?.totals || [])
  const routes = routesAnswer.routes.map(item => ({ ...item, status: routeStatus(item, context) }))
  return {
    routes,
    nodes,
    hopErrors,
    targets,
    context,
    traffic,
    window,
    series: statsAnswer?.error ? [] : (statsAnswer?.series || []),
    statsTruncated: Boolean(statsAnswer?.truncated),
    statsError: statsAnswer?.error || null,
    routesTruncated: routesAnswer.truncated,
    canDeleteRoutes: routesAnswer.canDelete,
    canDeleteNodes: nodesAnswer.canDelete
  }
}
