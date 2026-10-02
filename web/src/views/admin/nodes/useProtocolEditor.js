// State of the protocol editor (UI U7): the JSON editor, the visual form,
// the WireGuard relay form and the request body of
// POST/PUT /admin/nodes/:id/protocols[/:protocol_id], unchanged from the
// pre-redesign page. Masked secrets (********) are sent back as they are:
// the server keeps the stored value.
import { computed, reactive, ref, watch } from 'vue'

export const PROTOCOL_TYPES = ['vmess', 'vless', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'wireguard']

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

export const defaultWireGuardForm = () => ({
  cidr: '10.66.0.0/24',
  serverAddress: '10.66.0.1/24',
  serverPrivateKey: '',
  serverPublicKey: '',
  mtu: 1280,
  dns: '1.1.1.1,8.8.8.8',
  allowedIps: '0.0.0.0/0',
  tunnelType: 'quic',
  role: 'entry',
  wssCompat: false,
  wssPath: '/ws',
  wssSecure: true,
  wssServerName: '',
  wssCaFile: '',
  wssCertFile: '',
  wssKeyFile: '',
  relayServer: '',
  relayServerPort: 0,
  tunPort: 8421,
  tunName: '',
  entryTunAddress: '172.31.66.2/24',
  exitTunAddress: '172.31.66.1/24',
  outboundIface: '',
  exitNat: true,
  routingTable: 0,
  routingPriority: 0,
  networkPolicyEnabled: false,
  networkPaths: [],
  healthInterval: 10,
  healthTimeout: 3,
  failureThreshold: 3,
  recoveryThreshold: 2,
  failbackDelay: 300
})

const defaultProtocolForm = () => ({
  mode: 'json', // json | visual
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
  const tunnelType = wg.wssCompat ? 'wss' : (wg.tunnelType || 'quic')
  const relayMode = tunnelType === 'wss' ? 'relay+wss' : 'relay+quic'
  const isExit = wg.role === 'exit'
  return {
    cidr: wg.cidr || '10.66.0.0/24',
    server_address: isExit ? '' : (wg.serverAddress || '10.66.0.1/24'),
    server_private_key: isExit ? '' : (wg.serverPrivateKey || ''),
    server_public_key: isExit ? '' : (wg.serverPublicKey || ''),
    mtu: asNumber(wg.mtu, 1280),
    dns: isExit ? [] : splitList(wg.dns),
    allowed_ips: isExit ? [] : splitList(wg.allowedIps),
    tunnel_type: tunnelType,
    relay: {
      backend: 'gost',
      mode: relayMode,
      role: wg.role === 'exit' ? 'exit' : 'entry',
      wss_compat: tunnelType === 'wss',
      wss_path: wg.wssPath || '/ws',
      wss_secure: !isExit && tunnelType === 'wss',
      wss_server_name: isExit ? '' : (wg.wssServerName || ''),
      wss_ca_file: isExit ? '' : (wg.wssCaFile || ''),
      wss_cert_file: isExit ? (wg.wssCertFile || '') : '',
      wss_key_file: isExit ? (wg.wssKeyFile || '') : '',
      exit_nat: Boolean(wg.exitNat),
      entry_stats: true,
      server: wg.relayServer || '',
      server_port: asNumber(wg.relayServerPort, 0),
      tun_port: asNumber(wg.tunPort, 8421),
      tun_name: wg.tunName || '',
      entry_tun_address: wg.entryTunAddress || '172.31.66.2/24',
      exit_tun_address: wg.exitTunAddress || '172.31.66.1/24',
      outbound_iface: wg.outboundIface || '',
      routing_table: asNumber(wg.routingTable, 0),
      routing_priority: asNumber(wg.routingPriority, 0),
      ...(wg.networkPolicyEnabled && tunnelType === 'wss' ? {
        network_policy: {
          version: 1,
          strategy: 'failover',
          paths: wg.networkPaths.map(path => ({
            name: path.name || '',
            interface: path.interface || '',
            source: path.source || '',
            gateway: path.gateway || '',
            priority: asNumber(path.priority, 100),
            routing_table: asNumber(path.routing_table, 0),
            rule_priority: asNumber(path.rule_priority, 0)
          })),
          health_check: {
            interval_seconds: asNumber(wg.healthInterval, 10),
            timeout_seconds: asNumber(wg.healthTimeout, 3),
            failure_threshold: asNumber(wg.failureThreshold, 3),
            recovery_threshold: asNumber(wg.recoveryThreshold, 2),
            failback_delay_seconds: asNumber(wg.failbackDelay, 300)
          }
        }
      } : {})
    }
  }
}

export function wireGuardFormFromSettings(settings = {}) {
  const relay = settings.relay && typeof settings.relay === 'object' ? settings.relay : {}
  const networkPolicy = relay.network_policy && typeof relay.network_policy === 'object' ? relay.network_policy : {}
  const healthCheck = networkPolicy.health_check && typeof networkPolicy.health_check === 'object' ? networkPolicy.health_check : {}
  const tunnelType = relay.wss_compat || settings.tunnel_type === 'wss' || String(relay.mode || '').includes('wss') ? 'wss' : 'quic'
  return {
    cidr: settings.cidr || '10.66.0.0/24',
    serverAddress: settings.server_address || '10.66.0.1/24',
    serverPrivateKey: settings.server_private_key || '',
    serverPublicKey: settings.server_public_key || '',
    mtu: asNumber(settings.mtu, 1280),
    dns: asListText(settings.dns, '1.1.1.1,8.8.8.8'),
    allowedIps: asListText(settings.allowed_ips, '0.0.0.0/0'),
    tunnelType,
    role: relay.role === 'exit' ? 'exit' : 'entry',
    wssCompat: tunnelType === 'wss',
    wssPath: relay.wss_path || '/ws',
    wssSecure: relay.wss_secure !== false,
    wssServerName: relay.wss_server_name || '',
    wssCaFile: relay.wss_ca_file || '',
    wssCertFile: relay.wss_cert_file || '',
    wssKeyFile: relay.wss_key_file || '',
    relayServer: relay.server || '',
    relayServerPort: asNumber(relay.server_port, 0),
    tunPort: asNumber(relay.tun_port, 8421),
    tunName: relay.tun_name || '',
    entryTunAddress: relay.entry_tun_address || '172.31.66.2/24',
    exitTunAddress: relay.exit_tun_address || '172.31.66.1/24',
    outboundIface: relay.outbound_iface || '',
    exitNat: relay.exit_nat !== false,
    routingTable: asNumber(relay.routing_table, 0),
    routingPriority: asNumber(relay.routing_priority, 0),
    networkPolicyEnabled: Array.isArray(networkPolicy.paths) && networkPolicy.paths.length > 0,
    networkPaths: Array.isArray(networkPolicy.paths) ? networkPolicy.paths.map(path => ({
      name: path.name || '', interface: path.interface || '', source: path.source || '', gateway: path.gateway || '',
      priority: asNumber(path.priority, 100), routing_table: asNumber(path.routing_table, 0), rule_priority: asNumber(path.rule_priority, 0)
    })) : [],
    healthInterval: asNumber(healthCheck.interval_seconds, 10),
    healthTimeout: asNumber(healthCheck.timeout_seconds, 3),
    failureThreshold: asNumber(healthCheck.failure_threshold, 3),
    recoveryThreshold: asNumber(healthCheck.recovery_threshold, 2),
    failbackDelay: asNumber(healthCheck.failback_delay_seconds, 300)
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

export function useProtocolEditor() {
  const protocolForm = reactive(defaultProtocolForm())
  const wireGuardForm = reactive(defaultWireGuardForm())
  const jsonEditorContent = ref('')
  const jsonParseError = ref('')
  const jsonValid = computed(() => jsonParseError.value === '')
  const editing = ref(false)
  const selectedTemplate = ref('')

  const resetWireGuardForm = () => Object.assign(wireGuardForm, defaultWireGuardForm())
  const hydrateWireGuardForm = (settings) => Object.assign(wireGuardForm, wireGuardFormFromSettings(settings))

  function addWireGuardNetworkPath() {
    wireGuardForm.networkPaths.push({ name: '', interface: '', source: '', gateway: '', priority: 100 })
  }

  function removeWireGuardNetworkPath(index) {
    wireGuardForm.networkPaths.splice(index, 1)
  }

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
      if (obj.settings.relay.role === 'exit') obj.show = 0
      obj.tls_settings = {}
      obj.transport_settings = {}
      obj.reality_settings = {}
      return obj
    }
    obj.settings = parseObject(protocolForm.settings)
    obj.tls_settings = parseObject(protocolForm.tls_settings)
    obj.transport_settings = parseObject(protocolForm.transport_settings)
    obj.reality_settings = parseObject(protocolForm.reality_settings)
    return obj
  }

  function jsonToVisual(json) {
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
    if (protocolForm.type === 'wireguard') {
      let settings = json.settings || {}
      if (typeof settings === 'string') {
        try { settings = JSON.parse(settings) } catch { settings = {} }
      }
      hydrateWireGuardForm(settings)
    }
  }

  function setJson(json) {
    jsonEditorContent.value = JSON.stringify(json, null, 2)
    jsonParseError.value = ''
  }

  // Switching to JSON (when adding) writes the visual form into the editor;
  // switching to the visual form reads the editor (kept as is when invalid).
  watch(() => protocolForm.mode, (mode) => {
    if (mode === 'json' && !editing.value) {
      setJson(visualToJson())
    }
    if (mode === 'visual') {
      try {
        jsonToVisual(JSON.parse(jsonEditorContent.value))
      } catch {
        // keep the visual values when the JSON is invalid
      }
    }
  })

  watch(() => protocolForm.type, (newType, oldType) => {
    if (newType === 'wireguard' && oldType !== 'wireguard') {
      protocolForm.port = protocolForm.port === 443 ? 51820 : protocolForm.port
      protocolForm.tls = 0
      protocolForm.transport = 'udp'
      resetWireGuardForm()
    }
  })

  watch(() => wireGuardForm.wssCompat, (enabled) => {
    wireGuardForm.tunnelType = enabled ? 'wss' : 'quic'
  })

  watch(() => wireGuardForm.tunnelType, (value) => {
    wireGuardForm.wssCompat = value === 'wss'
  })

  watch(() => wireGuardForm.role, (role) => {
    if (role === 'exit') protocolForm.show = 0
  })

  function startCreate() {
    editing.value = false
    selectedTemplate.value = ''
    resetWireGuardForm()
    Object.assign(protocolForm, defaultProtocolForm())
    setJson(newProtocolJson())
  }

  function startEdit(protocol) {
    editing.value = true
    selectedTemplate.value = ''
    const json = protocolToJson(protocol)
    jsonToVisual(json)
    setJson(json)
    protocolForm.mode = 'json'
  }

  function applyTemplate(tpl) {
    selectedTemplate.value = tpl.name
    const json = templateToJson(tpl)
    if (json.type === 'wireguard') hydrateWireGuardForm(json.settings)
    setJson(json)
    jsonToVisual(json)
    protocolForm.mode = 'json'
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

  // { payload } or { error: 'invalidJson', message } / { error: 'required' }.
  function buildPayload() {
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
        transport: json.transport || 'tcp',
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
    return {
      payload: {
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
    wireGuardForm,
    jsonEditorContent,
    jsonParseError,
    jsonValid,
    editing,
    selectedTemplate,
    addWireGuardNetworkPath,
    removeWireGuardNetworkPath,
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
