// The order and display names of a subscription group's node protocols
// (GET/PUT /api/v4/admin/subscription-groups/:id/protocol-layout), merged
// with the protocol rows of GET /admin/subscription/groups/:id/protocols.

// The name a subscription shows when the group sets none.
export function defaultProtocolName(protocol) {
  return `${protocol?.node?.name || `#${protocol?.node_id ?? ''}`} - ${protocol?.name || ''}`
}

// The protocol rows in the layout's order, each with its display_name; rows
// the layout does not know follow by id (as the server orders them).
export function applyProtocolLayout(protocols, layout) {
  const items = Array.isArray(layout) ? layout : []
  const place = new Map(items.map((item, index) => [Number(item.protocol_id), index]))
  const names = new Map(items.map(item => [Number(item.protocol_id), item.display_name || '']))
  return [...(Array.isArray(protocols) ? protocols : [])]
    .map(protocol => ({ ...protocol, display_name: names.get(Number(protocol.id)) || '' }))
    .sort((a, b) => {
      const pa = place.has(Number(a.id)) ? place.get(Number(a.id)) : Infinity
      const pb = place.has(Number(b.id)) ? place.get(Number(b.id)) : Infinity
      if (pa !== pb) return pa < pb ? -1 : 1
      return Number(a.id) - Number(b.id)
    })
}

// A copy of list with the item at from moved to index to.
export function moveItem(list, from, to) {
  const next = [...list]
  if (from < 0 || from >= next.length || to < 0 || to >= next.length || from === to) return next
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}

// The PUT body's protocols, in order.
export function layoutRequest(rows) {
  return rows.map(row => ({ protocol_id: Number(row.id), display_name: String(row.display_name || '').trim() }))
}

export const MAX_DISPLAY_NAME = 128
