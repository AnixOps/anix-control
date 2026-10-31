// State of the protocol editor (UI U7): the JSON editor, the visual form,
// the WireGuard form, the iWAN form and the request body of
// POST/PUT /admin/nodes/:id/protocols[/:protocol_id], unchanged from the
// pre-redesign page. Masked secrets (********) are sent back as they are:
// the server keeps the stored value.
import { computed, reactive, ref, watch } from 'vue'
import { adaptTransport, capabilitiesOf, hasFixedTransport, transportProblem } from './protocolCapabilities'
import { PROTOCOL_SECTIONS, defaultRealityFor, defaultSettingsFor } from './protocolFields'

export const PROTOCOL_TYPES = ['vmess', 'vless', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'wireguard', 'iwan', 'anx']

export const JSON_PLACEHOLDER = `{
  "type": "vless",
  "port": 443,
  "tls": 0,
  "transport": "tcp",
  "enable": 1,
  "show": 1,
  "settings": {},
  "tls_settings": {},
  "transport_settings": {},
  "reality_settings": {}
}`

// Panabit iWAN (anixcore iwan inbound). Fixed addresses are edited as one
// `user id=address` per line.
export const defaultIwanForm = () => ({
  pool: '10.66.0.0/16',
  mtu: 1400,
  dnsUpstream: '1.1.1.1',
  allowEncrypt: true,
  allowClientToClient: false,
  fixedAddresses: ''
})

export function buildIwanSettings(form) {
  const fixed = {}
  for (const line of String(form.fixedAddresses || '').split(/\r?\n/)) {
    const [id, address] = line.split(/[=\s,]+/).filter(Boolean)
    if (id && address) fixed[id] = address
  }
  return {
    pool: form.pool,
    mtu: Number(form.mtu) || 1400,
    dns_upstream: form.dnsUpstream,
    allow_encrypt: Boolean(form.allowEncrypt),
    allow_client_to_client: Boolean(form.allowClientToClient),
    fixed_addresses: fixed
  }
}

export function iwanFormFromSettings(settings = {}) {
  const defaults = defaultIwanForm()
  const fixed = settings.fixed_addresses && typeof settings.fixed_addresses === 'object' ? settings.fixed_addresses : {}
  return {
    pool: settings.pool || defaults.pool,
    mtu: Number(settings.mtu) || defaults.mtu,
    dnsUpstream: settings.dns_upstream || defaults.dnsUpstream,
    allowEncrypt: settings.allow_encrypt ?? defaults.allowEncrypt,
    allowClientToClient: settings.allow_client_to_client ?? defaults.allowClientToClient,
    fixedAddresses: Object.entries(fixed).map(([id, address]) => `${id}=${address}`).join('\n')
  }
}

export const defaultWireGuardForm = () => ({
  cidr: '10.66.0.0/24',
  serverAddress: '10.66.0.1/24',
  serverPrivateKey: '',
  serverPublicKey: '',
  mtu: 1280,
  dns: '1.1.1.1,8.8.8.8',
  allowedIps: '0.0.0.0/0'
})

const defaultProtocolForm = () => ({
  mode: 'visual', // visual (form) | json (advanced)
  type: 'vless',
  port: 443,
  enable: 1,
  tls: 0,
  transport: 'tcp',
  settings: '{}',
  tls_settings: '{}',
  transport_settings: '{}',
  reality_settings: '{}',
  custom_config: '',
  show: 1
})

const splitList = (value) => String(value || '')
  .split(',')
  .map(item => item.trim())
  .filter(Boolean)

const asListText = (value, fallback) => {
  if (Array.isArray(value)) return value.join(',')
  if (typeof value === 'string' && value.trim()) return value
  return fallback
}

const asNumber = (value, fallback) => {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

const parseObject = (value) => {
  try { return JSON.parse(value || '{}') } catch { return {} }
}

const asJsonText = (value) => (value ? (typeof value === 'string' ? value : JSON.stringify(value, null, 2)) : '{}')

export function buildWireGuardSettings(wg) {
  // The relay (entry and exit over GOST) was removed with gost: a WireGuard
  // node terminates its users and egresses itself.
  return {
    cidr: wg.cidr || '10.66.0.0/24',
    server_address: wg.serverAddress || '10.66.0.1/24',
    server_private_key: wg.serverPrivateKey || '',
    server_public_key: wg.serverPublicKey || '',
    mtu: asNumber(wg.mtu, 1280),
    dns: splitList(wg.dns),
    allowed_ips: splitList(wg.allowedIps)
  }
}

export function wireGuardFormFromSettings(settings = {}) {
  return {
    cidr: settings.cidr || '10.66.0.0/24',
    serverAddress: settings.server_address || '10.66.0.1/24',
    serverPrivateKey: settings.server_private_key || '',
    serverPublicKey: settings.server_public_key || '',
    mtu: asNumber(settings.mtu, 1280),
    dns: asListText(settings.dns, '1.1.1.1,8.8.8.8'),
    allowedIps: asListText(settings.allowed_ips, '0.0.0.0/0')
  }
}

// A template of GET /admin/protocol-templates as editor JSON.
export function templateToJson(tpl) {
  return {
    type: tpl.type,
    port: tpl.default_port,
    tls: tpl.tls || 0,
    transport: tpl.transport || 'tcp',
    enable: 1,
    show: 1,
    settings: parseObject(tpl.settings),
    tls_settings: parseObject(tpl.tls_settings),
    transport_settings: parseObject(tpl.transport_settings),
    reality_settings: parseObject(tpl.reality_settings)
  }
}

// A stored protocol as editor JSON (settings parsed, custom_config kept).
export function protocolToJson(protocol) {
  const parse = (value) => {
    const raw = value || '{}'
    try { return typeof raw === 'string' ? JSON.parse(raw) : raw } catch { return {} }
  }
  const json = {
    type: protocol.type || 'vless',
    port: protocol.port || 443,
    tls: protocol.tls ?? 0,
    transport: protocol.transport || 'tcp',
    enable: protocol.enable ?? 1,
    show: protocol.show ?? 1,
    settings: parse(protocol.settings),
    tls_settings: parse(protocol.tls_settings),
    transport_settings: parse(protocol.transport_settings),
    reality_settings: parse(protocol.reality_settings)
  }
  if (protocol.custom_config) {
    try { json.custom_config = typeof protocol.custom_config === 'string' ? JSON.parse(protocol.custom_config) : protocol.custom_config } catch { json.custom_config = {} }
  }
  return json
}

export function newProtocolJson() {
  return {
    type: 'vless',
    port: 443,
    tls: 0,
    transport: 'tcp',
    enable: 1,
    show: 1,
    settings: {},
    tls_settings: {},
    transport_settings: {},
    reality_settings: {}
  }
}

// The transport a type with a single transport is sent with: the old editor
// stored tcp for Hysteria2 and udp for Shadowsocks, which mean nothing there.
function fixedTransport(type, transport) {
  const value = transport || 'tcp'
  if (hasFixedTransport(type) && (value === 'tcp' || value === 'udp')) return capabilitiesOf(type).transports[0]
  return value
}

export function useProtocolEditor() {
  const protocolForm = reactive(defaultProtocolForm())
  const wireGuardForm = reactive(defaultWireGuardForm())
  const iwanForm = reactive(defaultIwanForm())
  const jsonEditorContent = ref('')
  const jsonParseError = ref('')
  const jsonValid = computed(() => jsonParseError.value === '')
  const editing = ref(false)
  const selectedTemplate = ref('')
  // custom_config has no form field: it rides along from the stored protocol
  // or the JSON tab.
  let customConfig = null
  // Loading a protocol or template sets type, TLS and transport in one go;
  // the watchers below only react to the user's own changes.
  let hydrating = false
  const hydrate = (fn) => {
    hydrating = true
    try { fn() } finally { hydrating = false }
  }

  // The protocol with its section objects parsed, for ProtocolFields
  // (v-model). Writing stores the sections back as JSON text.
  const protocolModel = computed({
    get: () => ({
      type: protocolForm.type,
      tls: protocolForm.tls,
      transport: protocolForm.transport,
      ...Object.fromEntries(PROTOCOL_SECTIONS.map(section => [section, parseObject(protocolForm[section])]))
    }),
    set: (value) => {
      for (const section of PROTOCOL_SECTIONS) {
        const text = JSON.stringify(value[section] || {})
        if (text !== JSON.stringify(parseObject(protocolForm[section]))) protocolForm[section] = text
      }
    }
  })

  const resetWireGuardForm = () => Object.assign(wireGuardForm, defaultWireGuardForm())
  const hydrateWireGuardForm = (settings) => Object.assign(wireGuardForm, wireGuardFormFromSettings(settings))
  const resetIwanForm = () => Object.assign(iwanForm, defaultIwanForm())
  const hydrateIwanForm = (settings) => Object.assign(iwanForm, iwanFormFromSettings(settings))

  function visualToJson() {
    const obj = {
      type: protocolForm.type,
      port: protocolForm.port,
      tls: protocolForm.tls,
      transport: protocolForm.transport,
      enable: protocolForm.enable,
      show: protocolForm.show
    }
    if (protocolForm.type === 'wireguard') {
      obj.port = protocolForm.port || 51820
      obj.tls = 0
      obj.transport = 'udp'
      obj.settings = buildWireGuardSettings(wireGuardForm)
      obj.tls_settings = {}
      obj.transport_settings = {}
      obj.reality_settings = {}
      return obj
    }
    if (protocolForm.type === 'iwan') {
      obj.port = protocolForm.port || 4500
      obj.tls = 0
      obj.transport = 'udp'
      obj.settings = buildIwanSettings(iwanForm)
      obj.tls_settings = {}
      obj.transport_settings = {}
      obj.reality_settings = {}
      return obj
    }
    adaptTransport(obj)
    obj.settings = parseObject(protocolForm.settings)
    obj.tls_settings = parseObject(protocolForm.tls_settings)
    obj.transport_settings = parseObject(protocolForm.transport_settings)
    obj.reality_settings = parseObject(protocolForm.reality_settings)
    if (customConfig) obj.custom_config = customConfig
    return obj
  }

  function jsonToVisual(json) {
    hydrate(() => applyJsonToForm(json))
  }

  function applyJsonToForm(json) {
    customConfig = json.custom_config || null
    protocolForm.type = json.type || 'vless'
    protocolForm.port = json.port || 443
    protocolForm.tls = json.tls ?? 0
    protocolForm.transport = json.transport || 'tcp'
    protocolForm.enable = json.enable ?? 1
    protocolForm.show = json.show ?? 1
    protocolForm.settings = asJsonText(json.settings)
    protocolForm.tls_settings = asJsonText(json.tls_settings)
    protocolForm.transport_settings = asJsonText(json.transport_settings)
    protocolForm.reality_settings = asJsonText(json.reality_settings)
    adaptTransport(protocolForm)
    if (protocolForm.type === 'wireguard') {
      let settings = json.settings || {}
      if (typeof settings === 'string') {
        try { settings = JSON.parse(settings) } catch { settings = {} }
      }
      hydrateWireGuardForm(settings)
    }
    if (protocolForm.type === 'iwan') {
      let settings = json.settings || {}
      if (typeof settings === 'string') {
        try { settings = JSON.parse(settings) } catch { settings = {} }
      }
      hydrateIwanForm(settings)
    }
  }

  function setJson(json) {
    jsonEditorContent.value = JSON.stringify(json, null, 2)
    jsonParseError.value = ''
  }

  // Switching to JSON writes the form into the editor; switching to the form
  // reads the editor (kept as is when invalid). Sync, so a caller that sets
  // the mode and then the editor text is not overwritten later.
  watch(() => protocolForm.mode, (mode) => {
    if (hydrating) return
    if (mode === 'json') {
      setJson(visualToJson())
    }
    if (mode === 'visual') {
      try {
        jsonToVisual(JSON.parse(jsonEditorContent.value))
      } catch {
        // keep the visual values when the JSON is invalid
      }
    }
  }, { flush: 'sync' })

  watch(() => protocolForm.type, (newType, oldType) => {
    if (hydrating) return
    if (newType === 'wireguard' && oldType !== 'wireguard') {
      protocolForm.port = protocolForm.port === 443 ? 51820 : protocolForm.port
      protocolForm.tls = 0
      protocolForm.transport = 'udp'
      resetWireGuardForm()
    }
    if (newType === 'iwan' && oldType !== 'iwan') {
      protocolForm.port = protocolForm.port === 443 ? 4500 : protocolForm.port
      protocolForm.tls = 0
      protocolForm.transport = 'udp'
      resetIwanForm()
    }
    // Shadowsocks has no WebSocket, Hysteria2 no TCP: a new type keeps the
    // transport and TLS only when it takes them.
    adaptTransport(protocolForm)
    // The settings belong to the type: a new type starts with its own.
    protocolForm.settings = JSON.stringify(defaultSettingsFor(newType))
    seedReality()
  }, { flush: 'sync' })

  // Reality needs a target, a short id and a fingerprint before it can run.
  function seedReality() {
    if (Number(protocolForm.tls) !== 2) return
    const seeded = defaultRealityFor(protocolModel.value)
    protocolModel.value = seeded
  }

  watch(() => protocolForm.tls, () => {
    if (hydrating) return
    adaptTransport(protocolForm)
    seedReality()
  }, { flush: 'sync' })

  // The transport settings (path, host, service name) belong to the transport.
  watch(() => protocolForm.transport, () => {
    if (hydrating) return
    protocolForm.transport_settings = '{}'
  }, { flush: 'sync' })

  function startCreate() {
    editing.value = false
    selectedTemplate.value = ''
    resetWireGuardForm()
    resetIwanForm()
    customConfig = null
    hydrate(() => Object.assign(protocolForm, defaultProtocolForm()))
    setJson(newProtocolJson())
  }

  function startEdit(protocol) {
    editing.value = true
    selectedTemplate.value = ''
    const json = protocolToJson(protocol)
    jsonToVisual(json)
    setJson(json)
    hydrate(() => { protocolForm.mode = 'visual' })
  }

  function applyTemplate(tpl) {
    selectedTemplate.value = tpl.name
    const json = templateToJson(tpl)
    if (json.type === 'wireguard') hydrateWireGuardForm(json.settings)
    if (json.type === 'iwan') hydrateIwanForm(json.settings)
    setJson(json)
    jsonToVisual(json)
  }

  // The template picker loads a template into the JSON editor only.
  function loadTemplateJson(tpl) {
    setJson(templateToJson(tpl))
  }

  function validateJson() {
    try {
      JSON.parse(jsonEditorContent.value)
      jsonParseError.value = ''
    } catch (e) {
      jsonParseError.value = e.message
    }
  }

  function formatJson() {
    try {
      setJson(JSON.parse(jsonEditorContent.value))
    } catch (e) {
      jsonParseError.value = e.message
    }
  }

  // { payload }, or { error: 'invalidJson', message } / { error: 'required' } /
  // { error: 'unsupported', problem } (transportProblem).
  function buildPayload() {
    const built = buildRawPayload()
    if (built.error) return built
    const problem = transportProblem(built.payload)
    return problem ? { error: 'unsupported', problem } : built
  }

  function buildRawPayload() {
    if (protocolForm.mode === 'json') {
      let json
      try {
        json = JSON.parse(jsonEditorContent.value)
      } catch (e) {
        return { error: 'invalidJson', message: e.message }
      }
      const payload = {
        type: json.type || 'vless',
        port: json.port || 443,
        enable: json.enable ?? 1,
        tls: json.tls ?? 0,
        transport: fixedTransport(json.type || 'vless', json.transport),
        settings: JSON.stringify(json.settings || {}),
        tls_settings: JSON.stringify(json.tls_settings || {}),
        transport_settings: JSON.stringify(json.transport_settings || {}),
        reality_settings: JSON.stringify(json.reality_settings || {}),
        show: json.show ?? 1
      }
      if (json.custom_config) {
        payload.custom_config = typeof json.custom_config === 'string' ? json.custom_config : JSON.stringify(json.custom_config)
      }
      return { payload }
    }
    if (!protocolForm.type || !protocolForm.port) {
      return { error: 'required' }
    }
    const json = visualToJson()
    const custom = json.custom_config ? { custom_config: typeof json.custom_config === 'string' ? json.custom_config : JSON.stringify(json.custom_config) } : {}
    return {
      payload: {
        ...custom,
        type: json.type,
        port: json.port,
        enable: json.enable,
        tls: json.tls,
        transport: json.transport,
        settings: JSON.stringify(json.settings || {}),
        tls_settings: JSON.stringify(json.tls_settings || {}),
        transport_settings: JSON.stringify(json.transport_settings || {}),
        reality_settings: JSON.stringify(json.reality_settings || {}),
        show: protocolForm.show
      }
    }
  }

  return {
    protocolForm,
    protocolModel,
    wireGuardForm,
    iwanForm,
    jsonEditorContent,
    jsonParseError,
    jsonValid,
    editing,
    selectedTemplate,
    visualToJson,
    startCreate,
    startEdit,
    applyTemplate,
    loadTemplateJson,
    validateJson,
    formatJson,
    buildPayload
  }
}
