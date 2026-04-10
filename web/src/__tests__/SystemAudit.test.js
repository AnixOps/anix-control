import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import System from '@/views/admin/System.vue'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getBackupConfig: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  getLoadBalancers: vi.fn(),
  listForwardRuntimeJobs: vi.fn(),
  getForwardRuntimeStatus: vi.fn(),
  runForwardRuntimeDoctor: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountSystem() {
  return mount(System, {
    global: {
      stubs: {
        'router-link': true
      }
    }
  })
}

describe('System audit logs', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardRuntimeStatus.mockResolvedValue({ data: { data: null } })
    adminApi.runForwardRuntimeDoctor.mockResolvedValue({ data: { data: null } })
    adminApi.getSystemAuditLogs.mockResolvedValue({
      data: {
        data: {
          list: [
            {
              id: 1,
              action: 'update',
              module: 'system',
              target_type: 'config',
              username: 'admin',
              content: 'updated runtime_backend',
              ip: '127.0.0.1',
              status: 'success',
              created_at: '2026-04-11T08:00:00Z'
            }
          ],
          total: 1,
          page: 1,
          page_size: 20
        }
      }
    })
    vi.spyOn(window, 'alert').mockImplementation(() => {})
  })

  it('loads audit logs with default page and page_size', async () => {
    mountSystem()
    await flushPromises()

    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledWith({
      page: 1,
      page_size: 20
    })
  })

  it('applies action and target_type filters', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.getSystemAuditLogs.mockClear()

    wrapper.vm.auditFilters.action = 'delete'
    wrapper.vm.auditFilters.target_type = 'node'
    wrapper.vm.auditFilters.page = 3
    await wrapper.vm.applyAuditFilters()
    await flushPromises()

    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledWith({
      page: 1,
      page_size: 20,
      action: 'delete',
      target_type: 'node'
    })
  })

  it('renders audit log row content from API response', async () => {
    const wrapper = mountSystem()
    wrapper.vm.activeTab = 'audit'
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('updated runtime_backend')
    expect(text).toContain('admin')
    expect(text).toContain('config')
  })
})

