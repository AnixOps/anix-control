// Node entries (入口) as the sheet edits them. A NAT entry's port map is
// written one range per line, public ports first, as providers list them:
// "21000-21099 -> 30000-30099" or "20443 -> 443" (the API keeps
// { from, to, external }: protocol ports from..to reached at external..).

const RANGE = /^(\d{1,5})(?:\s*-\s*(\d{1,5}))?$/

function parseRange(text) {
  const match = RANGE.exec(text.trim())
  if (!match) return null
  const start = Number(match[1])
  const end = match[2] === undefined ? start : Number(match[2])
  if (start < 1 || end > 65535 || start > end) return null
  return { start, end }
}

// parsePortMap answers { ranges, error }: error is the 1-based line that
// does not parse, or whose two sides differ in length, or null.
export function parsePortMap(text) {
  const ranges = []
  const lines = String(text || '').split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    const sides = line.split(/\s*(?:->|=>|→|=)\s*/)
    if (sides.length !== 2) return { ranges, error: i + 1 }
    const external = parseRange(sides[0])
    const internal = parseRange(sides[1])
    if (!external || !internal || external.end - external.start !== internal.end - internal.start) {
      return { ranges, error: i + 1 }
    }
    ranges.push({ from: internal.start, to: internal.end, external: external.start })
  }
  return { ranges, error: null }
}

export function formatPortMap(ranges) {
  return (ranges || []).map(r => {
    const span = r.to - r.from
    const external = span ? `${r.external}-${r.external + span}` : `${r.external}`
    const internal = span ? `${r.from}-${r.to}` : `${r.from}`
    return `${external} -> ${internal}`
  }).join('\n')
}

let nextKey = 0

export function entryForm(entry = null) {
  return {
    key: ++nextKey,
    label: entry?.label || '',
    kind: entry?.kind || 'direct',
    host: entry?.host || '',
    portMap: formatPortMap(entry?.port_map),
    // The Panabit iWAN line of an iwan entry, by id.
    lineId: entry?.iwan_line_id ?? '',
    enabled: entry ? entry.enabled !== false : true
  }
}

// validateEntries answers the errors by form key: { label?, host?, portMap?, line? }.
export function validateEntries(forms, messages) {
  const errors = {}
  const labels = new Set()
  for (const form of forms) {
    const found = {}
    const label = form.label.trim()
    if (!label) found.label = messages.required
    else if (label.includes('|')) found.label = messages.noPipe
    else if (labels.has(label)) found.label = messages.duplicate
    labels.add(label)
    if (!form.host.trim()) found.host = messages.required
    if (form.kind === 'nat') {
      const { ranges, error } = parsePortMap(form.portMap)
      if (error) found.portMap = messages.badLine(error)
      else if (!ranges.length) found.portMap = messages.required
    }
    if (form.kind === 'iwan' && form.lineId === '') found.line = messages.required
    if (Object.keys(found).length) errors[form.key] = found
  }
  return errors
}

export function entriesRequest(forms) {
  return forms.map(form => {
    const entry = {
      label: form.label.trim(),
      kind: form.kind,
      host: form.host.trim(),
      port_map: form.kind === 'nat' ? parsePortMap(form.portMap).ranges : [],
      enabled: form.enabled
    }
    if (form.kind === 'iwan') entry.iwan_line_id = Number(form.lineId)
    return entry
  })
}

// lineForm is a Panabit iWAN line as its form edits it; the password is
// never loaded, and left empty on an edit it stays as it is.
export function lineForm(line = null) {
  return {
    id: line?.id ?? null,
    name: line?.name || '',
    host: line?.host || '',
    port: line?.port ?? null,
    username: line?.username || '',
    password: '',
    mtu: line?.mtu || null,
    encrypt: Boolean(line?.encrypt)
  }
}

// validateLine answers the errors by field: { name?, host?, port?,
// username?, password?, mtu? }.
export function validateLine(form, messages) {
  const errors = {}
  const name = form.name.trim()
  if (!name) errors.name = messages.required
  else if (name.includes('|')) errors.name = messages.noPipe
  if (!form.host.trim()) errors.host = messages.required
  if (!Number.isInteger(form.port) || form.port < 1 || form.port > 65535) errors.port = messages.port
  if (!form.username.trim()) errors.username = messages.required
  if (form.id === null && !form.password) errors.password = messages.required
  if (form.mtu !== null && form.mtu !== 0 && (form.mtu < 576 || form.mtu > 1600)) errors.mtu = messages.mtu
  return errors
}

export function lineRequest(form) {
  return {
    name: form.name.trim(),
    host: form.host.trim(),
    port: form.port,
    username: form.username.trim(),
    password: form.password,
    mtu: form.mtu || 0,
    encrypt: form.encrypt
  }
}

// portMapSummary is a short line for the table: "21000-21099 → 30000-30099".
export function portMapSummary(ranges) {
  return formatPortMap(ranges).split('\n').filter(Boolean).map(line => line.replace(' -> ', ' → ')).join(', ')
}
