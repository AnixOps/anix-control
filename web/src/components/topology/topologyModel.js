// The topology page's model: devices, lines and the links between devices,
// built from what Control already serves (the node list, the forward routes
// and the egress routes). No requests here.
import { displayStatus, statusName } from '@/views/admin/nodes/nodeData'
import { nodeState } from '@/components/forward/routeModel'
import { locate } from './geo'

// A line's tone from its route status (routeModel.routeStatus).
const LINE_TONES = { healthy: 'ok', syncing: 'warn', degraded: 'warn', error: 'bad' }
// A link shows its best line: it carries traffic while one line works.
const TONE_RANK = { off: 0, bad: 1, warn: 2, ok: 3 }

// buildTopology answers { devices, lines, links }:
// - devices: { ref, id, name, host, status, place, users, roles, lines }
//   (roles: entry, relay, exit by its position in the lines, or direct with
//   none; id is the node page's, null for a forward-only host);
// - lines: { id, kind: route | egress, name, status, tone, stages, traffic }
//   (stages: the device refs of each hop, entry first);
// - links: { key, from, to, kind, tone, lines } between two devices.
export function buildTopology({ nodes = [], forwardNodes = [], routes = [], traffic = new Map(), egress = [], now = Date.now() } = {}) {
  const devices = new Map()
  const add = device => devices.set(device.ref, { ...device, place: locate(device), roles: [], lines: [] })
  for (const node of nodes) {
    add({ ref: `proxy-${node.id}`, id: node.id, name: node.name, host: node.host, tags: node.tags, status: statusName(displayStatus(node, now)), users: Number(node.online_users || 0) })
  }
  for (const node of forwardNodes) {
    if (!devices.has(node.node_ref)) add({ ref: node.node_ref, id: null, name: node.name, host: node.host, tags: node.record?.region, status: nodeState(node, now), users: 0 })
  }

  const lines = []
  for (const item of routes) {
    const stages = (item.route.hops || []).map(hop => (hop.node_refs || []).filter(ref => devices.has(ref))).filter(stage => stage.length)
    if (!stages.length) continue
    const used = traffic.get(item.route.id)
    lines.push({ id: item.route.id, kind: 'route', name: item.route.name, status: item.status, tone: LINE_TONES[item.status] || 'off', stages, traffic: used ? used.up + used.down : 0 })
  }
  for (const route of egress) {
    const stages = [`proxy-${route.source_node?.id}`, `proxy-${route.exit_node?.id}`]
    if (!stages.every(ref => devices.has(ref))) continue
    const failed = route.source_state?.state === 'failed' || route.exit_state?.state === 'failed'
    lines.push({ id: `egress-${route.id}`, kind: 'egress', name: route.name, status: route.enabled ? 'healthy' : 'paused', tone: !route.enabled ? 'off' : failed ? 'bad' : 'ok', stages: stages.map(ref => [ref]), traffic: 0 })
  }

  const links = new Map()
  for (const line of lines) {
    // A hop whose devices are all offline breaks the line.
    if (line.tone !== 'off' && line.stages.some(stage => stage.every(ref => devices.get(ref).status !== 'online'))) line.tone = 'bad'
    line.stages.forEach((stage, index) => {
      const role = line.kind === 'egress' ? 'exit' : index === 0 && line.stages.length > 1 ? 'entry' : index === line.stages.length - 1 ? 'exit' : 'relay'
      for (const ref of stage) {
        const device = devices.get(ref)
        if (!device.roles.includes(role)) device.roles.push(role)
        device.lines.push(line.id)
        for (const to of line.stages[index + 1] || []) {
          const key = `${ref}>${to}`
          const link = links.get(key) || { key, from: ref, to, kind: line.kind, tone: 'off', lines: [] }
          link.lines.push(line.id)
          if (TONE_RANK[line.tone] > TONE_RANK[link.tone]) link.tone = line.tone
          links.set(key, link)
        }
      }
    })
  }
  for (const device of devices.values()) if (!device.roles.length) device.roles.push('direct')
  return { devices: [...devices.values()], lines, links: [...links.values()] }
}

// focusOf answers what a selection lights up: { devices, links } as sets of
// refs and link keys. A device lights the lines through it, a line itself.
export function focusOf(topology, selection) {
  if (!selection) return null
  const ids = selection.type === 'line' ? [selection.id] : topology.devices.find(device => device.ref === selection.id)?.lines || []
  const lines = topology.lines.filter(line => ids.includes(line.id))
  const devices = new Set(lines.flatMap(line => line.stages.flat()))
  if (selection.type === 'device') devices.add(selection.id)
  return { devices, links: new Set(topology.links.filter(link => link.lines.some(id => ids.includes(id))).map(link => link.key)) }
}

// Sizes of the abstract view, in px.
export const CARD = { width: 176, height: 56, gapX: 72, gapY: 14, rows: 8, head: 36 }

// layoutGraph places the devices in columns from entry to exit: a device's
// column is its longest way from an entry, and the last device of every
// line stands in the last column. `width` is the room there is: the
// columns spread over it, at least CARD.gapX apart. Answers { columns:
// [{ role, count, x, width }], at: Map(ref → { x, y }), width, height }.
export function layoutGraph({ devices, links }, width = 0) {
  const chain = links.filter(link => link.kind === 'route')
  const depth = new Map(devices.map(device => [device.ref, 0]))
  // Longest path; the pass count bounds a loop between two lines.
  for (let pass = 0; pass < devices.length; pass++) {
    let moved = false
    for (const link of chain) {
      if (depth.get(link.to) <= depth.get(link.from)) {
        depth.set(link.to, depth.get(link.from) + 1)
        moved = true
      }
    }
    if (!moved) break
  }
  const last = Math.max(0, ...depth.values())
  const sources = new Set(chain.map(link => link.from))
  for (const device of devices) if (!sources.has(device.ref)) depth.set(device.ref, last)

  const order = new Map()
  const columns = []
  const at = new Map()
  const lanesOf = column => Math.max(1, Math.ceil(devices.filter(device => depth.get(device.ref) === column).length / CARD.rows))
  let cards = 0
  for (let column = 0; column <= last; column++) cards += lanesOf(column) * (CARD.width + 16) - 16
  const gap = last ? Math.max(CARD.gapX, Math.min(220, (width - cards) / last)) : 0
  let x = 0
  for (let column = 0; column <= last; column++) {
    // Under the devices that send to them, then by place and name.
    const before = ref => {
      const from = chain.filter(link => link.to === ref && order.has(link.from)).map(link => order.get(link.from))
      return from.length ? from.reduce((sum, value) => sum + value, 0) / from.length : Infinity
    }
    const members = devices.filter(device => depth.get(device.ref) === column)
      .map(device => ({ device, before: before(device.ref) }))
      .sort((a, b) => a.before - b.before || (a.device.place?.key || '~').localeCompare(b.device.place?.key || '~') || a.device.name.localeCompare(b.device.name))
    const lanes = lanesOf(column)
    const perLane = Math.ceil(members.length / lanes)
    members.forEach(({ device }, index) => {
      order.set(device.ref, index)
      at.set(device.ref, { x: x + Math.floor(index / perLane) * (CARD.width + 16), y: (index % perLane) * (CARD.height + CARD.gapY), column })
    })
    const span = lanes * CARD.width + (lanes - 1) * 16
    columns.push({ role: column === last ? 'exit' : column === 0 ? 'entry' : 'relay', count: members.length, x, width: span, height: perLane * (CARD.height + CARD.gapY) - CARD.gapY })
    x += span + gap
  }
  // Shorter columns stand in the middle of the tallest.
  const height = Math.max(0, ...columns.map(column => column.height))
  for (const spot of at.values()) spot.y += CARD.head + (height - columns[spot.column].height) / 2
  return { columns, at, width: x - gap, height: CARD.head + height }
}
