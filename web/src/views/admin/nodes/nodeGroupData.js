// Node groups (5.0 node onboarding): how a group and its protocol template
// are edited in the console and sent to /api/v4/kernel/node-groups. No
// requests here. The server checks everything again (internal/nodegroup);
// these helpers only shape the form and the body.

export const BYTES_PER_GB = 1024 * 1024 * 1024

export const NODE_GROUP_KINDS = Object.freeze(['proxy', 'forward'])

// Every node a group makes is a regular node (docs/design/node-unification):
// the API stores every group as proxy, and a group without protocols makes
// relays. The console still shows such a group as a relay ("forward") group.
export function groupKind(group) {
  return Array.isArray(group?.protocol_template) && group.protocol_template.length > 0 ? 'proxy' : 'forward'
}

// The protocol types a template may name, and each one's transports and TLS
// modes (internal/nodegroup/template.go): Reality is VLESS only and runs on
// tcp, grpc and h2.
export const TEMPLATE_TYPES = Object.freeze(['vless', 'vmess', 'shadowsocks', 'wireguard'])
export const TEMPLATE_TRANSPORTS = Object.freeze({
  vless: ['tcp', 'ws', 'grpc', 'h2', 'httpupgrade'],
  vmess: ['tcp', 'ws', 'grpc', 'h2', 'httpupgrade'],
  shadowsocks: ['tcp'],
  wireguard: ['udp']
})
export const REALITY_TRANSPORTS = Object.freeze(['tcp', 'grpc', 'h2'])

// A new node of a preset group gets a random port of this range, clear of the
// well-known ports and of the ephemeral range (nodegroup.DefaultPortRangeFrom
// and DefaultPortRangeTo).
export const DEFAULT_PORT_RANGE = Object.freeze([20000, 40000])

// The site a Reality protocol presents to probes and forwards unauthenticated
// handshakes to. It must answer TLS 1.3 with HTTP/2 on port 443 and stay up.
// Keep it in step with nodegroup.DefaultRealityServerName, the default group
// the server creates; an operator edits it in the group's protocol template.
export const DEFAULT_REALITY_SERVER_NAME = 'www.microsoft.com'

export const SHADOWSOCKS_2022_METHOD = '2022-blake3-aes-128-gcm'

let entryKey = 0
function nextKey() {
  entryKey += 1
  return `protocol-${entryKey}`
}

// One protocol of a template as the editor holds it. `key` is only for v-for.
export function blankProtocol(type = 'vless') {
  const protocol = {
    key: nextKey(),
    name: '',
    type,
    portMode: 'range',
    port: 443,
    portFrom: DEFAULT_PORT_RANGE[0],
    portTo: DEFAULT_PORT_RANGE[1],
    transport: TEMPLATE_TRANSPORTS[type][0],
    tls: 'none',
    serverName: DEFAULT_REALITY_SERVER_NAME,
    dest: '',
    fingerprint: '',
    settingsText: '',
    transportSettingsText: ''
  }
  if (type === 'vless') {
    protocol.tls = 'reality'
    protocol.settingsText = JSON.stringify({ flow: 'xtls-rprx-vision' })
  } else if (type === 'shadowsocks') {
    protocol.settingsText = JSON.stringify({ method: SHADOWSOCKS_2022_METHOD })
  }
  return protocol
}

// What the inline "new group" form offers instead of a template editor.
export const PROTOCOL_PRESETS = Object.freeze({
  'vless-reality': () => [{ ...blankProtocol('vless'), name: 'VLESS Reality' }],
  'shadowsocks-2022': () => [{ ...blankProtocol('shadowsocks'), name: 'Shadowsocks 2022' }],
  'vless-reality-shadowsocks': () => [
    { ...blankProtocol('vless'), name: 'VLESS Reality' },
    { ...blankProtocol('shadowsocks'), name: 'Shadowsocks 2022' }
  ]
})
export const PRESET_KEYS = Object.freeze(Object.keys(PROTOCOL_PRESETS))
export const DEFAULT_PRESET = 'vless-reality'

function jsonText(value) {
  if (value === undefined || value === null) return ''
  return JSON.stringify(value)
}

// A template entry of the API as an editor protocol.
export function protocolFromEntry(entry) {
  const type = TEMPLATE_TYPES.includes(entry?.type) ? entry.type : 'vless'
  const range = Array.isArray(entry?.port_range) && entry.port_range.length === 2 ? entry.port_range : null
  const reality = entry?.reality || null
  return {
    key: nextKey(),
    name: entry?.name || '',
    type,
    portMode: range ? 'range' : 'fixed',
    port: Number(entry?.port) || 443,
    portFrom: range ? Number(range[0]) : DEFAULT_PORT_RANGE[0],
    portTo: range ? Number(range[1]) : DEFAULT_PORT_RANGE[1],
    transport: entry?.transport || TEMPLATE_TRANSPORTS[type][0],
    tls: entry?.tls === 'reality' ? 'reality' : 'none',
    serverName: reality?.server_name || DEFAULT_REALITY_SERVER_NAME,
    dest: reality?.dest || '',
    fingerprint: reality?.fingerprint || '',
    settingsText: jsonText(entry?.settings),
    transportSettingsText: jsonText(entry?.transport_settings)
  }
}

// What a settings field says when it is not a JSON object; '' when it is
// (or empty).
export function jsonObjectError(text) {
  const value = String(text ?? '').trim()
  if (!value) return ''
  try {
    const parsed = JSON.parse(value)
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) return 'object'
    return ''
  } catch {
    return 'json'
  }
}

// An editor protocol as a template entry of the API body. The caller checks
// the two settings fields with jsonObjectError first, so they are valid JSON
// objects or empty here.
export function entryFromProtocol(protocol) {
  const entry = { name: String(protocol.name || '').trim(), type: protocol.type, transport: protocol.transport, tls: protocol.tls }
  if (protocol.portMode === 'range') entry.port_range = [Number(protocol.portFrom), Number(protocol.portTo)]
  else entry.port = Number(protocol.port)
  if (protocol.tls === 'reality') {
    entry.reality = { server_name: String(protocol.serverName || '').trim() }
    const dest = String(protocol.dest || '').trim()
    if (dest) entry.reality.dest = dest
    const fingerprint = String(protocol.fingerprint || '').trim()
    if (fingerprint) entry.reality.fingerprint = fingerprint
  }
  const settings = String(protocol.settingsText || '').trim()
  if (settings) entry.settings = JSON.parse(settings)
  const transportSettings = String(protocol.transportSettingsText || '').trim()
  if (transportSettings) entry.transport_settings = JSON.parse(transportSettings)
  return entry
}

// What is wrong with one editor protocol, by field: '' when nothing is. The
// server validates everything again; this catches what would not even be
// valid JSON or a port before a request is sent.
export function protocolProblems(protocol) {
  const problems = { port: '', serverName: '', settings: '', transportSettings: '' }
  const inPorts = value => Number.isInteger(Number(value)) && Number(value) >= 1 && Number(value) <= 65535
  if (protocol.portMode === 'range') {
    if (!inPorts(protocol.portFrom) || !inPorts(protocol.portTo) || Number(protocol.portFrom) > Number(protocol.portTo)) problems.port = 'range'
  } else if (!inPorts(protocol.port)) {
    problems.port = 'port'
  }
  if (protocol.tls === 'reality' && !String(protocol.serverName || '').trim()) problems.serverName = 'required'
  problems.settings = jsonObjectError(protocol.settingsText)
  problems.transportSettings = jsonObjectError(protocol.transportSettingsText)
  return problems
}

export function hasProblems(problems) {
  return Object.values(problems).some(Boolean)
}

// Types, transports and TLS follow each other: a type change keeps the
// transport and TLS when the new type still takes them.
export function adaptProtocol(protocol) {
  const transports = TEMPLATE_TRANSPORTS[protocol.type] || TEMPLATE_TRANSPORTS.vless
  if (!transports.includes(protocol.transport)) protocol.transport = transports[0]
  if (protocol.type !== 'vless' && protocol.tls === 'reality') protocol.tls = 'none'
  if (protocol.tls === 'reality' && !REALITY_TRANSPORTS.includes(protocol.transport)) protocol.transport = 'tcp'
  return protocol
}

// Options of the token validity select, in days; 'never' and 'keep' are not.
export const TOKEN_EXPIRY_DAYS = Object.freeze({ '7d': 7, '30d': 30, '90d': 90, '365d': 365 })

// The form of a group (the edit sheet), or of a new one.
export function groupFormFrom(group, { now = Date.now() } = {}) {
  if (!group) {
    return {
      name: '',
      kind: 'proxy',
      rate: 1,
      tagsText: '',
      monthlyLimitGb: null,
      resetDay: 1,
      sort: 0,
      maxNodes: 0,
      requireApproval: false,
      subscriptionGroupIds: [],
      protocols: PROTOCOL_PRESETS[DEFAULT_PRESET](),
      expiry: 'never',
      currentExpiry: null
    }
  }
  const expires = group.token_expires_at ? Date.parse(group.token_expires_at) : NaN
  const hasExpiry = Number.isFinite(expires)
  let expiry = 'never'
  if (hasExpiry) expiry = group.token_expired || expires <= now ? 'expired' : 'keep'
  return {
    name: group.name || '',
    kind: groupKind(group),
    rate: Number(group.rate) || 1,
    tagsText: Array.isArray(group.tags) ? group.tags.join(', ') : '',
    monthlyLimitGb: group.monthly_limit_bytes ? group.monthly_limit_bytes / BYTES_PER_GB : null,
    resetDay: Number(group.reset_day) || 1,
    sort: Number(group.sort) || 0,
    maxNodes: Number(group.max_nodes) || 0,
    requireApproval: Boolean(group.require_approval),
    subscriptionGroupIds: Array.isArray(group.subscription_group_ids) ? [...group.subscription_group_ids] : [],
    protocols: Array.isArray(group.protocol_template) ? group.protocol_template.map(protocolFromEntry) : [],
    expiry,
    currentExpiry: hasExpiry ? group.token_expires_at : null
  }
}

function splitTags(text) {
  return [...new Set(String(text || '').split(/[,，]/).map(tag => tag.trim()).filter(Boolean))]
}

// The token's new expiry for the form's choice: null (never), the group's own
// when kept, or now plus the chosen number of days. An expired token must be
// given a new one: the API refuses a past time.
export function expiryFor(form, now = Date.now()) {
  if (form.expiry === 'never') return null
  if (form.expiry === 'keep') return form.currentExpiry
  const days = TOKEN_EXPIRY_DAYS[form.expiry]
  return days ? new Date(now + days * 86_400_000).toISOString() : null
}

// The body of POST/PUT /kernel/node-groups. A relay ("forward") group has
// no template and no subscription groups: its nodes are regular nodes
// without protocols.
export function buildGroupInput(form, { now = Date.now() } = {}) {
  const proxy = form.kind === 'proxy'
  const input = {
    name: String(form.name || '').trim(),
    kind: form.kind,
    max_nodes: Number(form.maxNodes) || 0,
    require_approval: Boolean(form.requireApproval),
    rate: Number(form.rate),
    tags: splitTags(form.tagsText),
    monthly_limit_bytes: form.monthlyLimitGb ? Math.round(Number(form.monthlyLimitGb) * BYTES_PER_GB) : 0,
    reset_day: Number(form.resetDay) || 1,
    sort: Number(form.sort) || 0,
    token_expires_at: expiryFor(form, now),
    protocol_template: proxy ? form.protocols.map(entryFromProtocol) : [],
    subscription_group_ids: proxy ? [...form.subscriptionGroupIds] : []
  }
  return input
}

// A group as an unchanged update body, for the toggles of the list: the same
// fields the API replaces, taken from the group as it was listed.
export function inputFromGroup(group, overrides = {}) {
  return {
    name: group.name,
    kind: group.kind,
    max_nodes: group.max_nodes || 0,
    require_approval: Boolean(group.require_approval),
    rate: group.rate,
    tags: Array.isArray(group.tags) ? group.tags : [],
    monthly_limit_bytes: group.monthly_limit_bytes || 0,
    reset_day: group.reset_day || 1,
    sort: group.sort || 0,
    token_expires_at: group.token_expires_at || null,
    protocol_template: Array.isArray(group.protocol_template) ? group.protocol_template : [],
    subscription_group_ids: Array.isArray(group.subscription_group_ids) ? group.subscription_group_ids : [],
    ...overrides
  }
}

// The body of the inline "new group" form of the join dialog: a name, a kind
// and (for a proxy group) one of the presets.
export function buildQuickGroupInput({ name, kind, preset, subscriptionGroupIds = [] }) {
  const proxy = kind === 'proxy'
  const protocols = proxy ? (PROTOCOL_PRESETS[preset] || PROTOCOL_PRESETS[DEFAULT_PRESET])() : []
  return {
    name: String(name || '').trim(),
    kind,
    protocol_template: protocols.map(entryFromProtocol),
    subscription_group_ids: proxy ? [...subscriptionGroupIds] : []
  }
}

// The subscription group a new proxy group joins by default: "默认" first,
// then the one a fresh panel seeds ("default"), as the server picks it for
// the default node group (nodegroup.DefaultSubscriptionGroupName).
const DEFAULT_SUBSCRIPTION_GROUP_NAMES = ['默认', 'default']
export function pickDefaultSubscriptionGroup(groups) {
  const list = Array.isArray(groups) ? groups : []
  for (const name of DEFAULT_SUBSCRIPTION_GROUP_NAMES) {
    const match = list.find(group => group?.name === name)
    if (match) return match
  }
  return null
}

// The groups of an answer of GET /kernel/node-groups.
export function readNodeGroups(answer) {
  if (Array.isArray(answer)) return answer
  return Array.isArray(answer?.groups) ? answer.groups : []
}

export function readNodeGroupNodes(answer) {
  if (Array.isArray(answer)) return answer
  return Array.isArray(answer?.nodes) ? answer.nodes : []
}

// The API's error as one line: kernel errors are { error: { code, message } }.
export function readKernelError(error, fallback = '') {
  const body = error?.response?.data
  const detail = body?.error
  return {
    status: error?.response?.status || 0,
    code: (detail && typeof detail === 'object' ? detail.code : '') || '',
    message: (detail && typeof detail === 'object' ? detail.message : (typeof detail === 'string' ? detail : '')) || body?.message || error?.message || fallback
  }
}
