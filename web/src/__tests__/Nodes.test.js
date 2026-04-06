import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNodeStats: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
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
})
