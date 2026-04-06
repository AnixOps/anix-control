import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Tunnel from '@/views/admin/Tunnel.vue'

const adminApi = vi.hoisted(() => ({
  createForwardTunnel: vi.fn(),
  deleteForwardTunnel: vi.fn(),
  diagnoseForwardTunnel: vi.fn(),
  getAdminForwardTunnelList: vi.fn(),
  getForwardNodes: vi.fn(),
  updateForwardTunnel: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountTunnel() {
  return mount(Tunnel, {
    global: {
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
})
