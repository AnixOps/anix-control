// The line sheet's form (LineSheet.vue): a line as the administrator edits
// it, and the draft the line API takes (POST/PUT /api/v5/net/lines).
// FIXED is the protocol choice of a line that forwards to fixed targets.
export const FIXED = 'fixed'

// lineForm answers the form of a line of GET /api/v4/kernel/network/lines
// (null for a new one): entries are the first hop's devices, via the
// devices of the hops between, exit the last hop's device. A line on one
// device has it as its only entry and its exit.
export function lineForm(line) {
  const chain = line?.chain || []
  return {
    name: line?.name || '',
    entries: [...(chain[0]?.devices || [])],
    via: chain.slice(1, -1).map(hop => hop.devices[0]),
    exit: chain.at(-1)?.devices[0] || '',
    protocol: line ? (line.protocol ? line.protocol.id : FIXED) : '',
    targets: (line?.targets || []).map(target => `${target.host}:${target.port}`).join('\n'),
    port: line?.port || null,
    tcp: !line || line.l4.includes('tcp'),
    udp: !line || line.l4.includes('udp'),
    enabled: line ? line.enabled : true,
    show: line ? line.show : true
  }
}

// parseTargets reads one host:port per line ([v6]:port too).
export function parseTargets(text) {
  return text.split('\n').map(item => item.trim()).filter(Boolean).map((item) => {
    const at = item.lastIndexOf(':')
    return { host: item.slice(0, at).replace(/^\[(.*)\]$/, '$1'), port: Number(item.slice(at + 1)) }
  })
}

// lineDraft answers the request body of a form.
export function lineDraft(form, revision = 0) {
  const fixed = form.protocol === FIXED
  return {
    name: form.name.trim(),
    enabled: form.enabled,
    show: !fixed && form.show,
    port: Number(form.port),
    l4: [form.tcp && 'tcp', form.udp && 'udp'].filter(Boolean),
    chain: (alone(form) ? [[form.exit]] : [form.entries, ...form.via.map(ref => [ref]), [form.exit]]).map(devices => ({ devices })),
    ...(fixed ? { targets: parseTargets(form.targets).map(target => ({ ...target, weight: 1 })) } : { protocol_id: form.protocol }),
    ...(revision ? { revision } : {})
  }
}

// alone reports a line on one device: its only entry is its exit.
const alone = form => form.entries.length === 1 && form.entries[0] === form.exit && !form.via.length

// validate answers the form's errors by field, as message keys.
export function validate(form) {
  const errors = {}
  if (!form.name.trim()) errors.name = 'required'
  if (!form.entries.length) errors.entries = 'required'
  if (!form.exit) errors.exit = 'required'
  else if (!alone(form) && (form.entries.includes(form.exit) || form.via.includes(form.exit))) errors.exit = 'reused'
  if (form.protocol === '') errors.protocol = 'required'
  if (form.protocol === FIXED && parseTargets(form.targets).some(target => !target.host || !(target.port > 0 && target.port < 65536))) errors.targets = 'target'
  if (form.protocol === FIXED && !form.targets.trim()) errors.targets = 'required'
  if (!(form.port > 0 && form.port < 65536)) errors.port = 'required'
  if (!form.tcp && !form.udp) errors.l4 = 'required'
  return errors
}

// protocolsOn answers the protocols served on a device, from the lines of
// the network (each protocol has its own line): [{ id, type, port }].
export function protocolsOn(network, device) {
  const found = new Map()
  for (const line of network?.lines || []) {
    if (line.protocol?.device === device) found.set(line.protocol.id, line.protocol)
  }
  return [...found.values()].sort((a, b) => a.id - b.id)
}
