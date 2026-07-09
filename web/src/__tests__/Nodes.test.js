import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNodeStats: vi.fn(),
  getNodeLogs: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  getNodeCredentials: vi.fn(),
  getNodeProtocols: vi.fn(),
  createNodeProtocol: vi.fn(),
  updateNodeProtocol: vi.fn(),
  deleteNodeProtocol: vi.fn(),
  getProtocolTemplates: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn(),
  deleteAuthKey: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountNodes() {
  return mount(Nodes, {
    global: {
      mocks: {
        $t: (_key, fallback) => fallback || _key
      }
    }
  })
}

describe('Nodes.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getNodes.mockResolvedValue({ data: { list: [], total: 0 } })
    adminApi.getNodeStats.mockResolvedValue({ data: { total: 0, online: 0, offline: 0, pending: 0 } })
    adminApi.getNodeLogs.mockResolvedValue({ data: { list: [], total: 0 } })
    adminApi.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    adminApi.getNodeProtocols.mockResolvedValue({ data: [] })
    adminApi.getProtocolTemplates.mockResolvedValue({ data: [] })
    adminApi.getAuthKeys.mockResolvedValue({ data: [] })
  })

  it('stops follow-up admin requests when the initial nodes load fails', async () => {
    adminApi.getNodes.mockRejectedValueOnce(new Error('401'))

    mountNodes()
    await flushPromises()

    expect(adminApi.getNodes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeStats).not.toHaveBeenCalled()
    expect(adminApi.getProtocolTemplates).not.toHaveBeenCalled()
    expect(adminApi.getAuthKeys).not.toHaveBeenCalled()
  })

  it('loads stats, templates, and auth keys after nodes load succeeds', async () => {
    mountNodes()
    await flushPromises()

    expect(adminApi.getNodes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeStats).toHaveBeenCalledTimes(1)
    expect(adminApi.getProtocolTemplates).toHaveBeenCalledTimes(1)
    expect(adminApi.getAuthKeys).toHaveBeenCalledTimes(1)
  })

  it('renders node stats from legacy and panel envelope payloads', async () => {
    adminApi.getNodeStats
      .mockResolvedValueOnce({ data: { total: 12, online: 7, offline: 4, pending: 1 } })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: { total: 21, online: 18, offline: 2, pending: 1 },
        ts: 1783526400000
      })

    const wrapper = mountNodes()
    await flushPromises()

    let metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['12', '7', '4', '1'])

    await wrapper.vm.loadStats()
    await flushPromises()

    metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['21', '18', '2', '1'])
  })

  it('loads node list resources from legacy, panel, and nested payloads', async () => {
    adminApi.getNodes.mockResolvedValueOnce({
      data: { list: [{ id: 1, name: 'Legacy Node', host: 'legacy.example' }], total: 1 }
    })
    adminApi.getProtocolTemplates.mockResolvedValueOnce({ data: [{ name: 'Legacy Template' }] })
    adminApi.getAuthKeys.mockResolvedValueOnce({ data: [{ key: 'legacy-key', used: 2 }] })

    const wrapper = mountNodes()
    await flushPromises()

    expect(wrapper.vm.nodes[0].name).toBe('Legacy Node')
    expect(wrapper.vm.nodes[0].address).toBe('legacy.example')
    expect(wrapper.vm.pagination.total).toBe(1)
    expect(wrapper.vm.protocolTemplates[0].name).toBe('Legacy Template')
    expect(wrapper.vm.authKey).toBe('legacy-key')
    expect(wrapper.vm.authKeyUsed).toBe(2)

    adminApi.getNodes.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 2, name: 'Envelope Node', host: 'env.example' }], total: 3 },
      ts: 1783526400000
    })
    await wrapper.vm.loadNodes()

    expect(wrapper.vm.nodes[0].name).toBe('Envelope Node')
    expect(wrapper.vm.pagination.total).toBe(3)

    adminApi.getProtocolTemplates.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ name: 'Envelope Template' }],
      ts: 1783526400000
    })
    await wrapper.vm.loadProtocolTemplates()
    expect(wrapper.vm.protocolTemplates[0].name).toBe('Envelope Template')

    adminApi.getAuthKeys.mockResolvedValueOnce({ data: { data: [{ key: 'nested-key', used: 4 }] } })
    await wrapper.vm.loadAuthKeysPreview()
    expect(wrapper.vm.authKey).toBe('nested-key')
    expect(wrapper.vm.authKeyUsed).toBe(4)
  })

  it('loads node protocols, logs, and deploy credentials from panel envelopes', async () => {
    adminApi.getNodes.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 9, name: 'Root Node', host: 'root.example' }], total: 1 },
      ts: 1783526400000
    })
    adminApi.getNodeProtocols.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 91, type: 'vless' }],
      ts: 1783526400000
    })
    adminApi.getNodeLogs.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 1, message: 'node started' }], total: 1 },
      ts: 1783526400000
    })
    adminApi.getNodeCredentials.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { api_key: 'node-api-key' },
      ts: 1783526400000
    })

    const wrapper = mountNodes()
    await flushPromises()

    await wrapper.vm.openProtocols(wrapper.vm.nodes[0])
    expect(wrapper.vm.protocols[0].type).toBe('vless')

    await wrapper.vm.openLogModal(wrapper.vm.nodes[0])
    expect(wrapper.vm.nodeLogs[0].message).toBe('node started')
    expect(wrapper.vm.logPagination.total).toBe(1)

    await wrapper.vm.openDeployModal()
    expect(wrapper.vm.deployRows[0].apiKey).toBe('node-api-key')
  })

  it('builds WireGuard relay JSON from the visual protocol form', async () => {
    const wrapper = mountNodes()
    await flushPromises()

    wrapper.vm.openAddProtocol()
    wrapper.vm.protocolForm.mode = 'visual'
    await wrapper.vm.$nextTick()
    wrapper.vm.protocolForm.type = 'wireguard'
    await wrapper.vm.$nextTick()

    Object.assign(wrapper.vm.wireGuardForm, {
      cidr: '10.88.0.0/24',
      serverAddress: '10.88.0.1/24',
      serverPrivateKey: 'server-private',
      serverPublicKey: 'server-public',
      role: 'exit',
      wssCompat: true,
      relayServerPort: 9443,
      tunPort: 8422,
      entryTunAddress: '172.31.88.2/24',
      exitTunAddress: '172.31.88.1/24',
      outboundIface: 'eth0',
      routingTable: 32088,
      routingPriority: 12088
    })

    const json = wrapper.vm.visualToJson()

    expect(json.type).toBe('wireguard')
    expect(json.port).toBe(51820)
    expect(json.transport).toBe('udp')
    expect(json.settings.cidr).toBe('10.88.0.0/24')
    expect(json.settings.tunnel_type).toBe('wss')
    expect(json.settings.relay.mode).toBe('relay+wss')
    expect(json.settings.relay.role).toBe('exit')
    expect(json.settings.relay.wss_compat).toBe(true)
    expect(json.settings.relay.entry_tun_address).toBe('172.31.88.2/24')
    expect(json.settings.relay.exit_tun_address).toBe('172.31.88.1/24')
    expect(json.settings.relay.outbound_iface).toBe('eth0')
  })

  it('saves WireGuard visual protocol settings through the node protocol API', async () => {
    adminApi.createNodeProtocol.mockResolvedValueOnce({ data: { id: 101 } })
    adminApi.getNodeProtocols.mockResolvedValueOnce({ data: [] })

    const wrapper = mountNodes()
    await flushPromises()

    wrapper.vm.selectedNode = { id: 77, name: 'Entry Node' }
    wrapper.vm.openAddProtocol()
    wrapper.vm.protocolForm.mode = 'visual'
    await wrapper.vm.$nextTick()
    wrapper.vm.protocolForm.type = 'wireguard'
    await wrapper.vm.$nextTick()

    Object.assign(wrapper.vm.wireGuardForm, {
      cidr: '10.77.0.0/24',
      serverAddress: '10.77.0.1/24',
      serverPrivateKey: 'server-private',
      serverPublicKey: 'server-public',
      role: 'entry',
      relayServer: 'exit.example.com',
      relayServerPort: 8443,
      wssCompat: false
    })

    await wrapper.vm.saveProtocol()
    await flushPromises()

    expect(adminApi.createNodeProtocol).toHaveBeenCalledTimes(1)
    const [nodeID, payload] = adminApi.createNodeProtocol.mock.calls[0]
    expect(nodeID).toBe(77)
    expect(payload.type).toBe('wireguard')
    expect(payload.port).toBe(51820)
    expect(payload.transport).toBe('udp')
    const settings = JSON.parse(payload.settings)
    expect(settings.tunnel_type).toBe('quic')
    expect(settings.relay.mode).toBe('relay+quic')
    expect(settings.relay.role).toBe('entry')
    expect(settings.relay.server).toBe('exit.example.com')
    expect(settings.relay.server_port).toBe(8443)
  })
})
