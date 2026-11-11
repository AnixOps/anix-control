// The 线路 page's model: the network as lines (GET /api/v4/kernel/network/
// lines) with what the node list and the forward routes know about their
// state. No requests here.
import { displayStatus, statusName } from '@/views/admin/nodes/nodeData'
import { locate, placeOf } from '@/components/topology/geo'

// A line's tone from its route status (routeModel.routeStatus).
const ROUTE_TONES = { healthy: 'ok', syncing: 'warn', degraded: 'warn', error: 'bad' }

// buildLines answers the lines, each {
//   id, name, kind: relay | direct | fixed, enabled, show, tone,
//   protocol, port, rate, traffic, entryHostname, targets,
//   hops: [{ role: entry | relay | exit, devices: [device] }],
//   served: the ref of the device the protocol is served at,
//   rules: [{ name, domains, exit: device, enabled }],
//   route: the forward route's id of a line that has one }
// and device is { ref, id, name, host, place, kernel, online }.
// - relay: entries, maybe relays, an exit, and a protocol served on the way;
// - direct: one device, the protocol where its users connect;
// - fixed: no protocol, the ports go to fixed targets.
// A paused line is off; a line with a hop whose devices are all offline is
// bad; otherwise a line with a route has the route's tone.
export function buildLines({ network, nodes = [], routes = [], traffic = new Map(), now = Date.now() } = {}) {
  const nodeOf = new Map(nodes.map(node => [`proxy-${node.id}`, node]))
  const devices = new Map((network?.devices || []).map((item) => {
    const node = nodeOf.get(item.ref)
    return [item.ref, {
      ref: item.ref,
      id: node?.id ?? null,
      name: item.name,
      host: item.host,
      place: placeOf(item.region) || locate({ name: item.name, tags: node?.tags }),
      kernel: Boolean(item.kernel),
      online: Boolean(node) && statusName(displayStatus(node, now)) === 'online'
    }]
  }))
  const routeOf = new Map(routes.map(item => [item.route.id, item]))

  return (network?.lines || []).map((line) => {
    const route = routeOf.get(line.id)
    const used = traffic.get(line.id)
    const chain = (line.chain || []).map(hop => hop.devices.map(ref => devices.get(ref)).filter(Boolean))
    const hops = chain.map((list, index) => ({
      role: index === chain.length - 1 ? 'exit' : index === 0 ? 'entry' : 'relay',
      devices: list
    }))
    const down = hops.some(hop => hop.devices.every(device => !device.online))
    return {
      id: line.id,
      name: line.name || line.id,
      kind: !line.protocol ? 'fixed' : hops.length > 1 ? 'relay' : 'direct',
      enabled: line.enabled,
      show: line.show,
      tone: !line.enabled ? 'off' : down ? 'bad' : route ? ROUTE_TONES[route.status] || 'off' : 'ok',
      protocol: line.protocol?.type || '',
      port: line.port,
      rate: line.rate,
      traffic: used ? used.up + used.down : 0,
      entryHostname: line.entry_hostname || '',
      targets: (line.targets || []).map(target => `${target.host}:${target.port}`),
      hops,
      served: line.protocol?.device || '',
      rules: (line.rules || []).map(rule => ({ ...rule, exit: devices.get(rule.exit) })),
      route: route ? line.id : ''
    }
  })
}

// matches reports whether a line passes a filter of the page: '' (all), a
// kind, or attention (a warning or a break).
export const matches = (line, filter) => !filter || (filter === 'attention' ? line.tone === 'warn' || line.tone === 'bad' : line.kind === filter)
