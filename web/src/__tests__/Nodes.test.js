import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNodeStats: vi.fn(),
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
})
