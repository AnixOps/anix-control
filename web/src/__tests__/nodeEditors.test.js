// The node page's editors without the page (UI U7): the Agent config and
// Ansible files of useNodeDeploy, and the protocol request bodies of
// useProtocolEditor (unchanged from the pre-redesign page).
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { isLoopbackGRPCHost, useNodeDeploy } from '@/views/admin/nodes/useNodeDeploy'
import { useProtocolEditor } from '@/views/admin/nodes/useProtocolEditor'
import { buildNodePayload, displayStatus, nodeFormFrom, parentCandidatesFor } from '@/views/admin/nodes/nodeData'

vi.mock('@/api/admin', () => ({
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn(),
  getNodeCredentials: vi.fn()
}))

const KEY = 'lvbhRmhzVbSAbrw3vm0k7vYqpEu4/dF/ZqVbp2gS7uM='
const config = deploy => JSON.parse(deploy.configSnippet.value.slice(deploy.configSnippet.value.indexOf('\n') + 1)).Nodes[0]

describe('useNodeDeploy', () => {
  let deploy
  beforeEach(() => { deploy = useNodeDeploy() })

  it('keeps the Plugin Supervisor canary off until it is opted in and complete', () => {
    expect(config(deploy)).not.toHaveProperty('PluginSupervisorEnabled')
    Object.assign(deploy.deploySettings, { pluginSupervisorEnabled: true, grpcUseTLS: true })
    expect(deploy.pluginSupervisorCanaryReady.value).toBe(false)
    expect(config(deploy)).not.toHaveProperty('PluginSupervisorEnabled')

    deploy.deploySettings.pluginOfficialPublicKey = KEY
    expect(deploy.pluginSupervisorCanaryReady.value).toBe(true)
    expect(config(deploy)).toMatchObject({
      AgentControlEnabled: true,
      PluginSupervisorEnabled: true,
      PluginRoot: '/var/lib/anixops/plugins',
      PluginSocketDir: '/run/anixops/plugins',
      PluginOfficialPublicKey: KEY
    })
    expect(deploy.deployGroupVarsPreview.value).toContain('plugin_supervisor_enabled: true')
    expect(deploy.deployGroupVarsPreview.value).toContain(`plugin_official_public_key: "${KEY}"`)
  })

  it('refuses a plaintext external gRPC endpoint and an invalid trust root', () => {
    Object.assign(deploy.deploySettings, {
      panelApiHost: 'https://panel.example.test',
      grpcHost: 'control.example.test:50051',
      grpcUseTLS: false,
      pluginSupervisorEnabled: true,
      pluginOfficialPublicKey: KEY
    })
    expect(deploy.agentControlEnabled.value).toBe(false)
    expect(deploy.pluginSupervisorCanaryReady.value).toBe(false)
    expect(config(deploy)).toMatchObject({ ApiHost: 'https://panel.example.test', AgentControlEnabled: false, GRPCUseTLS: false })

    Object.assign(deploy.deploySettings, { grpcHost: '[::1]:50051', pluginOfficialPublicKey: 'not-a-public-key' })
    expect(deploy.agentControlEnabled.value).toBe(true)
    expect(deploy.pluginSupervisorCanaryError.value).toContain('Ed25519')
    deploy.deploySettings.pluginOfficialPublicKey = KEY.slice(0, -1)
    expect(deploy.pluginSupervisorCanaryReady.value).toBe(false)
  })

  it('recognises loopback gRPC hosts only', () => {
    expect(isLoopbackGRPCHost('127.0.0.1:50051')).toBe(true)
    expect(isLoopbackGRPCHost('localhost')).toBe(true)
    expect(isLoopbackGRPCHost('[::1]:50051')).toBe(true)
    expect(isLoopbackGRPCHost('10.0.0.1:50051')).toBe(false)
    expect(isLoopbackGRPCHost('http://127.0.0.1:50051')).toBe(false)
  })
})

describe('useProtocolEditor', () => {
  it('sends masked secrets back as the placeholder, which keeps the stored values', () => {
    const editor = useProtocolEditor()
    editor.startEdit({
      id: 91,
      type: 'vless',
      port: 443,
      tls: 2,
      settings: '{"flow":"xtls-rprx-vision"}',
      reality_settings: '{"private_key":"********","public_key":"reality-public","short_id":"ab"}'
    })
    const { payload } = editor.buildPayload()
    expect(payload).toMatchObject({ type: 'vless', port: 443, tls: 2, transport: 'tcp', enable: 1, show: 1 })
    expect(JSON.parse(payload.reality_settings)).toEqual({ private_key: '********', public_key: 'reality-public', short_id: 'ab' })
    expect(JSON.parse(payload.settings)).toEqual({ flow: 'xtls-rprx-vision' })
  })

  it('reports invalid JSON and builds WireGuard relay settings from the form', async () => {
    const editor = useProtocolEditor()
    editor.startCreate()
    editor.jsonEditorContent.value = 'not json'
    expect(editor.buildPayload()).toMatchObject({ error: 'invalidJson' })

    editor.protocolForm.mode = 'visual'
    await nextTick()
    editor.protocolForm.type = 'wireguard'
    await nextTick()
    Object.assign(editor.wireGuardForm, {
      cidr: '10.88.0.0/24',
      serverPrivateKey: 'server-private',
      serverPublicKey: 'server-public',
      role: 'exit',
      wssCompat: true,
      wssPath: '/wireguard',
      wssCertFile: '/etc/v2bx/relay-cert.pem',
      wssKeyFile: '/etc/v2bx/relay-key.pem',
      outboundIface: 'eth0'
    })
    await nextTick()

    const json = editor.visualToJson()
    expect(json).toMatchObject({ type: 'wireguard', port: 51820, transport: 'udp', show: 0 })
    expect(json.settings).toMatchObject({ cidr: '10.88.0.0/24', tunnel_type: 'wss', server_private_key: '', server_public_key: '' })
    expect(json.settings.relay).toMatchObject({
      mode: 'relay+wss', role: 'exit', wss_compat: true, wss_path: '/wireguard', wss_secure: false,
      wss_cert_file: '/etc/v2bx/relay-cert.pem', wss_key_file: '/etc/v2bx/relay-key.pem', outbound_iface: 'eth0'
    })

    const { payload } = editor.buildPayload()
    expect(payload).toMatchObject({ type: 'wireguard', port: 51820, transport: 'udp', tls: 0 })
    expect(JSON.parse(payload.settings).relay.role).toBe('exit')
  })

  it('loads a template into the editor and the form', () => {
    const editor = useProtocolEditor()
    editor.startCreate()
    editor.applyTemplate({ name: 'Trojan', type: 'trojan', default_port: 9443, tls: 1, transport: 'ws', settings: '{"a":1}' })
    expect(JSON.parse(editor.jsonEditorContent.value)).toMatchObject({ type: 'trojan', port: 9443, tls: 1, transport: 'ws', settings: { a: 1 } })
    expect(editor.protocolForm).toMatchObject({ type: 'trojan', port: 9443, tls: 1, transport: 'ws' })
    expect(editor.selectedTemplate.value).toBe('Trojan')
  })
})

describe('node data', () => {
  it('builds the node body the form has always sent', () => {
    const form = nodeFormFrom({ id: 3, name: 'hk', host: 'hk.example', tags: 'HK', rate: 1.5, sort: 2, status: 1, parent_id: 1, monthly_limit: 2 * 1024 ** 3, monthly_reset_day: 5 })
    expect(buildNodePayload(form, { editing: true })).toEqual({
      name: 'hk', host: 'hk.example', tags: 'HK', rate: 1.5, sort: 2, parent_id: 1, monthly_limit: 2 * 1024 ** 3, monthly_reset_day: 5, status: 1
    })
    expect(buildNodePayload(nodeFormFrom(null))).not.toHaveProperty('status')
  })

  it('leaves the node and its descendants out of the parent candidates', () => {
    const nodes = [{ id: 1 }, { id: 2, parent_id: 1 }, { id: 3, parent_id: 2 }, { id: 4 }]
    expect(parentCandidatesFor(nodes, { id: 1 }).map(n => n.id)).toEqual([4])
    expect(parentCandidatesFor(nodes, null)).toHaveLength(4)
  })

  it('derives online and offline from the last heartbeat, as the list endpoint does', () => {
    const now = 1_800_000_000_000
    const seconds = now / 1000
    expect(displayStatus({ status: 2, last_check_at: seconds - 60 }, now)).toBe(1)
    expect(displayStatus({ status: 1, last_check_at: seconds - 600 }, now)).toBe(2)
    expect(displayStatus({ status: 3, last_check_at: seconds - 60 }, now)).toBe(3)
    expect(displayStatus({ status: 0 }, now)).toBe(0)
  })
})
