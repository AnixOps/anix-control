// What each node protocol type runs on: its transports and TLS modes
// (0 none, 1 TLS, 2 Reality). Shadowsocks has no stream transport and no TLS;
// Hysteria2 and TUIC are QUIC on UDP and always TLS; WireGuard and iWAN are
// UDP. Control checks the same rules again (service.validateProtocolTransport).

export const STREAM_TRANSPORTS = Object.freeze(['tcp', 'ws', 'grpc', 'h2', 'httpupgrade'])
// Reality runs on tcp, grpc and h2 only.
export const REALITY_TRANSPORTS = Object.freeze(['tcp', 'grpc', 'h2'])
// Transports the old editor offered and stored protocols may still carry:
// kept when loaded, never offered for a new value.
export const LEGACY_TRANSPORTS = Object.freeze(['quic'])

export const PROTOCOL_CAPABILITIES = Object.freeze({
  vless: { transports: STREAM_TRANSPORTS, tls: [0, 1, 2] },
  vmess: { transports: STREAM_TRANSPORTS, tls: [0, 1] },
  trojan: { transports: STREAM_TRANSPORTS, tls: [0, 1, 2] },
  shadowsocks: { transports: ['tcp'], tls: [0] },
  hysteria2: { transports: ['udp'], tls: [1] },
  tuic: { transports: ['udp'], tls: [1] },
  wireguard: { transports: ['udp'], tls: [0] },
  iwan: { transports: ['udp'], tls: [0] },
  // ANX-A runs its own jacket over TCP: no stream transport, no TLS of the node's.
  anx: { transports: ['tcp'], tls: [0] }
})

export function capabilitiesOf(type) {
  return PROTOCOL_CAPABILITIES[type] || PROTOCOL_CAPABILITIES.vless
}

// The transports a form may offer for type and TLS mode. A legacy transport
// the protocol already has stays listed so the form shows it.
export function transportsFor(type, tls, current) {
  const caps = capabilitiesOf(type)
  let list = Number(tls) === 2 ? caps.transports.filter(value => REALITY_TRANSPORTS.includes(value)) : [...caps.transports]
  if (caps.transports === STREAM_TRANSPORTS && Number(tls) !== 2 && LEGACY_TRANSPORTS.includes(current)) list = [...list, current]
  return list
}

// A type has a single transport (and the form shows it fixed).
export function hasFixedTransport(type) {
  return capabilitiesOf(type).transports.length === 1
}

// Moves tls and transport to what type takes, keeping them when it still
// takes them. Mutates and returns form.
export function adaptTransport(form) {
  const caps = capabilitiesOf(form.type)
  const tls = Number(form.tls)
  if (!caps.tls.includes(tls)) form.tls = caps.tls[0]
  const allowed = transportsFor(form.type, form.tls, form.transport)
  if (!allowed.includes(form.transport)) form.transport = allowed[0]
  return form
}

// The first problem of a protocol body ({ type, tls, transport }), or null:
// { field: 'transport' | 'tls', type, value }.
export function transportProblem(protocol) {
  const type = String(protocol.type || '')
  if (!PROTOCOL_CAPABILITIES[type]) return null
  const caps = PROTOCOL_CAPABILITIES[type]
  const tls = Number(protocol.tls ?? 0)
  if (!caps.tls.includes(tls)) return { field: 'tls', type, value: tls }
  const transport = String(protocol.transport || '')
  if (!transport) return null
  if (!transportsFor(type, tls, transport).includes(transport)) return { field: 'transport', type, value: transport }
  return null
}
