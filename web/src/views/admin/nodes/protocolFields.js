// The fields of the node protocol form (the 3x-ui style editor): which keys of
// settings, tls_settings, reality_settings and transport_settings a protocol
// type, TLS mode and transport take, and how the form edits them. Keys the
// registry does not know stay as they are; the JSON tab edits those.
//
// One field is { section, key, kind, ... }:
//   section  one of PROTOCOL_SECTIONS (a JSON object column of the protocol)
//   key      the JSON key; `keys` lists aliases, read in order, written to the
//            first one present (else the last)
//   kind     text | password | number | select | switch | list (an array of
//            strings, edited as comma separated text)
//   options  select: the values ('' means unset); labels come from i18n
//   show     (protocol) => whether the field applies
// The same registry serves the node protocol sheet, the node group template
// and the subscription group editors; ANX adds its fields here.

export const PROTOCOL_SECTIONS = Object.freeze(['settings', 'tls_settings', 'reality_settings', 'transport_settings'])

// Groups of the form, in display order.
export const FIELD_GROUPS = Object.freeze({
  settings: 'protocol',
  transport_settings: 'transport',
  tls_settings: 'security',
  reality_settings: 'security'
})

export const SS_METHODS = Object.freeze([
  '2022-blake3-aes-128-gcm',
  '2022-blake3-aes-256-gcm',
  '2022-blake3-chacha20-poly1305',
  'aes-128-gcm',
  'aes-256-gcm',
  'chacha20-ietf-poly1305'
])
export const FINGERPRINTS = Object.freeze(['chrome', 'firefox', 'safari', 'ios', 'android', 'edge', 'random'])
export const HYSTERIA_OBFS = Object.freeze(['', 'salamander'])
export const TUIC_CONGESTION = Object.freeze(['bbr', 'cubic', 'new_reno'])
export const VLESS_FLOWS = Object.freeze(['', 'xtls-rprx-vision'])
// ANX-A (anx-node's anx inbound, A2 jacket): the cover site and the names and ALPN it answers with.
export const ANX_COVER = 'www.apple.com'
export const ANX_ALPN = Object.freeze(['h2', 'http/1.1'])
// Streams one ANX session may open: anx-node's default and its u16 limit.
export const ANX_MAX_STREAMS = 256
export const ANX_MAX_STREAMS_LIMIT = 65535

// Reality targets that work from most regions; the target probe
// (NodeRealityTargetsSection) replaces this with a measured one.
export const DEFAULT_REALITY_TARGET = 'www.apple.com'

const isTls = (p) => Number(p.tls) > 0
const isReality = (p) => Number(p.tls) === 2
const takesTransportPath = (p) => ['ws', 'h2', 'httpupgrade'].includes(p.transport) && ['vless', 'vmess', 'trojan'].includes(p.type)
const isStreamProtocol = (p) => ['vless', 'vmess', 'trojan'].includes(p.type)

export const PROTOCOL_FIELDS = Object.freeze([
  // settings --------------------------------------------------------------
  {
    section: 'settings', key: 'flow', kind: 'select', options: VLESS_FLOWS,
    show: p => p.type === 'vless' && isTls(p) && p.transport === 'tcp'
  },
  {
    section: 'settings', key: 'method', keys: ['cipher', 'method'], kind: 'select', options: SS_METHODS,
    default: 'aes-256-gcm', show: p => p.type === 'shadowsocks'
  },
  {
    section: 'settings', key: 'server_key', kind: 'password', hintKey: 'serverKey',
    show: p => p.type === 'shadowsocks' && String(readField(p, { section: 'settings', key: 'method', keys: ['cipher', 'method'] }) || '').startsWith('2022-blake3-')
  },
  { section: 'settings', key: 'up_mbps', kind: 'number', min: 0, max: 100000, show: p => p.type === 'hysteria2' },
  { section: 'settings', key: 'down_mbps', kind: 'number', min: 0, max: 100000, show: p => p.type === 'hysteria2' },
  { section: 'settings', key: 'obfs', kind: 'select', options: HYSTERIA_OBFS, show: p => p.type === 'hysteria2' },
  {
    section: 'settings', key: 'obfs-password', kind: 'password',
    show: p => p.type === 'hysteria2' && Boolean(readField(p, { section: 'settings', key: 'obfs' }))
  },
  { section: 'settings', key: 'congestion_control', kind: 'select', options: TUIC_CONGESTION, default: 'bbr', show: p => p.type === 'tuic' },
  {
    section: 'settings', key: 'server_names', kind: 'list', default: [ANX_COVER], hintKey: 'anxServerNames',
    placeholder: ANX_COVER, show: p => p.type === 'anx'
  },
  {
    section: 'settings', key: 'dest', kind: 'text', default: `${ANX_COVER}:443`, hintKey: 'anxDest',
    placeholder: `${ANX_COVER}:443`, show: p => p.type === 'anx'
  },
  { section: 'settings', key: 'alpn', kind: 'list', default: [...ANX_ALPN], placeholder: 'h2,http/1.1', show: p => p.type === 'anx' },
  // A2 no-SNI mode (anx spec §4): also accept ClientHellos without SNI, and
  // subscriptions leave the SNI out (clients dial the IP).
  { section: 'settings', key: 'allow_no_sni', kind: 'switch', hintKey: 'anxAllowNoSni', show: p => p.type === 'anx' },
  // Shaping (anx spec §6): VisionLike by default; plain only to debug.
  { section: 'settings', key: 'shape_plain', kind: 'switch', hintKey: 'anxShapePlain', show: p => p.type === 'anx' },
  {
    section: 'settings', key: 'max_streams', kind: 'number', min: 1, max: ANX_MAX_STREAMS_LIMIT, hintKey: 'anxMaxStreams',
    placeholder: String(ANX_MAX_STREAMS), show: p => p.type === 'anx'
  },

  // transport -------------------------------------------------------------
  { section: 'transport_settings', key: 'path', kind: 'text', placeholder: '/', show: takesTransportPath },
  { section: 'transport_settings', key: 'host', kind: 'text', show: takesTransportPath },
  { section: 'transport_settings', key: 'serviceName', kind: 'text', show: p => isStreamProtocol(p) && p.transport === 'grpc' },

  // security --------------------------------------------------------------
  { section: 'tls_settings', key: 'server_name', kind: 'text', show: p => isTls(p) && p.type !== 'shadowsocks' },
  { section: 'tls_settings', key: 'fingerprint', kind: 'select', options: ['', ...FINGERPRINTS], show: p => isTls(p) && isStreamProtocol(p) },
  { section: 'tls_settings', key: 'allowInsecure', kind: 'switch', show: p => Number(p.tls) === 1 },
  { section: 'reality_settings', key: 'dest', kind: 'text', placeholder: `${DEFAULT_REALITY_TARGET}:443`, show: isReality },
  { section: 'reality_settings', key: 'short_id', kind: 'text', action: 'shortId', show: isReality },
  { section: 'reality_settings', key: 'private_key', keys: ['private_key', 'privateKey'], kind: 'password', hintKey: 'privateKey', show: isReality },
  { section: 'reality_settings', key: 'public_key', keys: ['public_key', 'publicKey', 'pbk'], kind: 'text', hintKey: 'publicKey', show: isReality }
])

// The fields that apply to protocol ({ type, tls, transport, settings, ... }
// with the section objects parsed).
export function fieldsFor(protocol) {
  return PROTOCOL_FIELDS.filter(field => field.show(protocol))
}

function keysOf(field) {
  return field.keys || [field.key]
}

// The stored key of field in its section object: the first alias present,
// else the last one (`method` for Shadowsocks).
function storedKey(section, field) {
  const keys = keysOf(field)
  return keys.find(key => section && Object.prototype.hasOwnProperty.call(section, key)) || keys[keys.length - 1]
}

export function readField(protocol, field) {
  const section = protocol?.[field.section]
  const keys = keysOf(field)
  for (const key of keys) {
    if (section && section[key] !== undefined && section[key] !== null) return section[key]
  }
  return field.default ?? (field.kind === 'switch' ? false : (field.kind === 'number' ? null : (field.kind === 'list' ? [] : '')))
}

// A copy of protocol with field set to value. An empty value removes the key
// so the stored settings stay minimal; false removes a switch.
export function writeField(protocol, field, value) {
  const section = { ...(protocol[field.section] || {}) }
  const key = storedKey(section, field)
  const empty = value === '' || value === null || value === undefined || value === false || (field.kind === 'number' && Number.isNaN(value)) || (Array.isArray(value) && value.length === 0)
  // A removed field must not fall back to an alias that is still stored.
  if (empty) keysOf(field).forEach(alias => { delete section[alias] })
  else {
    keysOf(field).filter(alias => alias !== key).forEach(alias => { delete section[alias] })
    section[key] = value
  }
  return { ...protocol, [field.section]: section }
}

// What a freshly chosen protocol type starts with: settings that must have a
// value (a Shadowsocks method, a TUIC congestion control).
export function defaultSettingsFor(type) {
  const settings = {}
  for (const field of PROTOCOL_FIELDS) {
    if (field.section === 'settings' && field.default !== undefined && field.show({ type, tls: 0, transport: 'tcp' })) {
      settings[field.key] = field.default
    }
  }
  if (type === 'hysteria2') Object.assign(settings, { up_mbps: 100, down_mbps: 100 })
  return settings
}

// What switching to Reality starts with when the protocol has no target yet:
// a target, a short id and the uTLS fingerprint a Reality client needs. The
// key pair stays empty: Control generates it when the protocol is saved.
export function defaultRealityFor(protocol, randomHex = randomShortId) {
  const reality = { ...(protocol.reality_settings || {}) }
  const tls = { ...(protocol.tls_settings || {}) }
  if (!reality.dest) reality.dest = `${DEFAULT_REALITY_TARGET}:443`
  if (!reality.short_id) reality.short_id = randomHex()
  if (!tls.server_name) tls.server_name = DEFAULT_REALITY_TARGET
  if (!tls.fingerprint) tls.fingerprint = 'chrome'
  return { ...protocol, reality_settings: reality, tls_settings: tls }
}

// 8 random bytes as 16 hex digits, the length Xray and sing-box take.
export function randomShortId() {
  const bytes = new Uint8Array(8)
  globalThis.crypto.getRandomValues(bytes)
  return Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
}
