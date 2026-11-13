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
    show: line ? line.show : true,
    ...policyForm(line)
  }
}

// BALANCES and PROXY_PROTOCOLS are the values the line API takes, with
// 'default' and 'off' for none (a select item cannot be empty).
export const BALANCES = ['default', 'round_robin', 'random', 'ip_hash', 'least_conn', 'failover']
export const PROXY_PROTOCOLS = ['off', 'v1', 'v2']
const GB = 1024 ** 3

// policyForm reads a line's policy and limits in the units the sheet
// shows (seconds, Mbps, GB, local time); empty is the default or no limit.
function policyForm(line) {
  const policy = line?.policy || {}
  const seconds = ms => (ms ? ms / 1000 : null)
  return {
    proxyProtocol: policy.proxy_protocol || 'off',
    balance: policy.balance || 'default',
    healthOff: Boolean(policy.health?.disabled),
    interval: seconds(policy.health?.interval_ms),
    timeout: seconds(policy.health?.timeout_ms),
    threshold: policy.circuit_breaker?.failure_threshold || null,
    openFor: seconds(policy.circuit_breaker?.open_ms),
    bandwidth: policy.bandwidth_bps ? policy.bandwidth_bps / 1e6 : null,
    maxConns: policy.max_conns || null,
    quota: line?.quota_bytes ? line.quota_bytes / GB : null,
    expires: localTime(line?.expires_at_unix_ms),
    hopBalance: line?.hop_balance || ''
  }
}

// hasPolicy reports a form with a policy or limit set (its advanced part
// opens).
export function hasPolicy(form) {
  return Object.keys(policyDraft(form)).some(key => key !== 'hop_balance')
}

// localTime is Unix ms as a datetime-local value in the browser's zone.
function localTime(ms) {
  if (!ms) return ''
  const date = new Date(ms)
  const pad = value => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

// policyDraft answers the policy part of a draft: only what is set.
function policyDraft(form) {
  const n = value => Math.round(Number(value) || 0)
  const ms = value => n(Number(value) * 1000)
  const prune = object => Object.fromEntries(Object.entries(object).filter(([, value]) => value))
  const health = prune({ interval_ms: ms(form.interval), timeout_ms: ms(form.timeout), disabled: form.healthOff })
  const breaker = prune({ failure_threshold: n(form.threshold), open_ms: ms(form.openFor) })
  const policy = prune({
    balance: form.protocol === FIXED && form.balance !== 'default' ? form.balance : '',
    proxy_protocol: form.proxyProtocol === 'off' ? '' : form.proxyProtocol,
    bandwidth_bps: n(Number(form.bandwidth) * 1e6),
    max_conns: n(form.maxConns),
    health: Object.keys(health).length ? health : null,
    circuit_breaker: Object.keys(breaker).length ? breaker : null
  })
  const expires = form.expires ? new Date(form.expires).getTime() : 0
  return prune({
    policy: Object.keys(policy).length ? policy : null,
    hop_balance: form.hopBalance,
    quota_bytes: n(Number(form.quota) * GB),
    expires_at_unix_ms: Number.isFinite(expires) ? expires : 0
  })
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
    ...policyDraft(form),
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
  if (form.interval && form.timeout && Number(form.timeout) >= Number(form.interval)) errors.timeout = 'timeout'
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
