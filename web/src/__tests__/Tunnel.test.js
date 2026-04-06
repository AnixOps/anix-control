import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Tunnel from '@/views/admin/Tunnel.vue'

const adminApi = vi.hoisted(() => ({
  createForwardTunnel: vi.fn(),
  deleteForwardTunnel: vi.fn(),
  diagnoseForwardTunnel: vi.fn(),
  getAdminForwardTunnelList: vi.fn(),
  getForwardNodes: vi.fn(),
  getSystemConfig: vi.fn(),
  updateForwardTunnel: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountTunnel() {
  return mount(Tunnel, {
    global: {
      stubs: {
        'router-link': {
          template: '<a><slot /></a>'
        }
      },
      mocks: {
        $t: (_key, fallback) => fallback || _key
      }
    }
  })
}

describe('Tunnel.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    adminApi.getAdminForwardTunnelList.mockResolvedValue({ code: 0, data: [] })
    adminApi.createForwardTunnel.mockResolvedValue({ code: 0 })
    adminApi.deleteForwardTunnel.mockResolvedValue({ code: 0 })
    adminApi.diagnoseForwardTunnel.mockResolvedValue({ code: 0, data: { results: [] } })
    adminApi.updateForwardTunnel.mockResolvedValue({ code: 0 })
    adminApi.getSystemConfig.mockImplementation((key) => {
      if (key === 'forward.runtime.nodex_mode') {
        return Promise.resolve({ data: { value: 'true' } })
      }
      if (key === 'forward.runtime_backend') {
        return Promise.resolve({ data: { value: 'gost' } })
      }
      return Promise.resolve({ data: { value: '' } })
    })
  })

  it('loads NodeX mode from system config', async () => {
    adminApi.getForwardNodes.mockResolvedValue({ data: { list: [] } })

    const wrapper = mountTunnel()
    await flushPromises()

    console.log('nodeX mode', wrapper.vm.runtimeNodeXMode)

    wrapper.vm.openCreateModal()
    await wrapper.vm.$nextTick()

    expect(adminApi.getSystemConfig).toHaveBeenCalledWith('forward.runtime.nodex_mode')
    expect(wrapper.vm.runtimeNodeXMode).toBe(true)
  })

  it('shows NodeX entry and exit selects when NodeX mode is active', async () => {
    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [
          { id: 10, name: 'Relay-a', host: 'relay-a.host', type: 'relay', status: 1 },
          { id: 20, name: 'Exit-a', host: 'exit-a.host', type: 'exit', status: 1 }
        ]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    expect(wrapper.find('[data-test="forward-entry-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="forward-exit-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="forward-execution-select"]').exists()).toBe(false)

    wrapper.vm.form.type = 2
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="forward-exit-select"]').exists()).toBe(true)
  })

  it('forces ansible mode to use execution node without entry node', async () => {
    adminApi.getSystemConfig.mockImplementation((key) => {
      if (key === 'forward.runtime.nodex_mode') {
        return Promise.resolve({ data: { value: 'false' } })
      }
      if (key === 'forward.runtime_backend') {
        return Promise.resolve({ data: { value: 'iptables_ansible' } })
      }
      return Promise.resolve({ data: { value: '' } })
    })
    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [
          { id: 10, name: 'Relay-a', host: 'relay-a.host', type: 'relay', status: 1 },
          { id: 20, name: 'Exit-a', host: 'exit-a.host', type: 'exit', status: 1 }
        ]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    wrapper.vm.openCreateModal()
    await wrapper.vm.$nextTick()

    wrapper.vm.form.name = 'Ansible Tunnel'
    wrapper.vm.form.flow = 1
    wrapper.vm.form.trafficRatio = 1
    wrapper.vm.form.tcpListenAddr = '[::]'
    wrapper.vm.form.udpListenAddr = '[::]'
    wrapper.vm.form.outNodeId = 10

    await wrapper.vm.handleSubmit()
    await flushPromises()

    expect(adminApi.createForwardTunnel).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 1,
        inNodeId: 0,
        outNodeId: 10,
        protocol: ''
      })
    )
  })

  it('shows execution select when NodeX mode is disabled', async () => {
    adminApi.getSystemConfig.mockImplementation((key) => {
      if (key === 'forward.runtime.nodex_mode') {
        return Promise.resolve({ data: { value: 'false' } })
      }
      if (key === 'forward.runtime_backend') {
        return Promise.resolve({ data: { value: 'iptables_ansible' } })
      }
      return Promise.resolve({ data: { value: '' } })
    })

    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [
          { id: 10, name: 'Relay-a', host: 'relay-a.host', type: 'relay', status: 1 }
        ]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    expect(wrapper.vm.runtimeNodeXMode).toBe(false)
    expect(wrapper.find('[data-test="forward-entry-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="forward-execution-select"]').exists()).toBe(true)
  })

  it('splits entry and exit nodes by type', async () => {
    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [
          { id: 10, name: 'Relay-a', host: 'relay-a.host', type: 'relay', status: 1 },
          { id: 20, name: 'Exit-a', host: 'exit-a.host', type: 'exit', status: 1 },
          { id: 30, name: 'Relay-b', host: 'relay-b.host', type: 'relay', status: 1 }
        ]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    expect(adminApi.getForwardNodes).toHaveBeenCalled()
    expect(wrapper.vm.relayNodeOptions).toHaveLength(2)
    expect(wrapper.vm.exitNodeOptions).toHaveLength(1)
    expect(wrapper.vm.relayNodeOptions.every(node => node.type === 'relay')).toBe(true)
    expect(wrapper.vm.exitNodeOptions.every(node => node.type === 'exit')).toBe(true)
  })

  it('rejects mismatched forward node types during validation', async () => {
    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [
          { id: 10, name: 'Relay-a', host: 'relay-a.host', type: 'relay', status: 0 },
          { id: 20, name: 'Exit-a', host: 'exit-a.host', type: 'exit', status: 1 }
        ]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    wrapper.vm.form.name = 'NodeX tunnel'
    wrapper.vm.form.type = 2
    wrapper.vm.form.inNodeId = 20
    wrapper.vm.form.outNodeId = 10
    wrapper.vm.form.flow = 1
    wrapper.vm.form.trafficRatio = 1
    wrapper.vm.form.protocol = 'tls'
    wrapper.vm.form.tcpListenAddr = '[::]'
    wrapper.vm.form.udpListenAddr = '[::]'

    await wrapper.vm.handleSubmit()

    expect(adminApi.createForwardTunnel).not.toHaveBeenCalled()
    expect(wrapper.vm.errors.inNodeId).toBe('入口节点必须是转发中继节点')
    expect(wrapper.vm.errors.outNodeId).toBe('出口节点必须是转发出口节点')
  })

  it('locks legacy ansible tunnels to execution nodes when editing', async () => {
    adminApi.getSystemConfig.mockImplementation((key) => {
      if (key === 'forward.runtime.nodex_mode') {
        return Promise.resolve({ data: { value: 'false' } })
      }
      if (key === 'forward.runtime_backend') {
        return Promise.resolve({ data: { value: 'iptables_ansible' } })
      }
      return Promise.resolve({ data: { value: '' } })
    })

    adminApi.getForwardNodes.mockResolvedValue({
      data: {
        list: [{ id: 15, name: 'Relay-legacy', host: 'legacy.host', type: 'relay', status: 1 }]
      }
    })

    const wrapper = mountTunnel()
    await flushPromises()

    const legacyTunnel = {
      id: 5,
      name: 'Legacy',
      type: 1,
      inNodeId: 15,
      outNodeId: null,
      flow: 1,
      trafficRatio: 1,
      protocol: 'tls',
      tcpListenAddr: '[::]',
      udpListenAddr: '[::]'
    }

    wrapper.vm.openEditModal(legacyTunnel)
    await wrapper.vm.$nextTick()

    expect(wrapper.vm.form.outNodeId).toBe(15)
    expect(wrapper.find('[data-test="forward-entry-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="forward-execution-select"]').exists()).toBe(true)
  })
})
