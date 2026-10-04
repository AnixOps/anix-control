import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SettingsAudit from '@/views/admin/system/SettingsAudit.vue'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getBackupConfig: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  getLoadBalancers: vi.fn(),
  getForwardRuntimeStatus: vi.fn(),
  runForwardRuntimeDoctor: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountSystem() {
  return mount(SettingsAudit, {
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
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
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
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('updated runtime_backend')
    expect(text).toContain('admin')
    expect(text).toContain('config')
    // Known actions, modules and results show as labels.
    const cells = wrapper.findAll('tbody td').map(cell => cell.text())
    expect(cells).toContain('Update')
    expect(cells).toContain('System')
    expect(cells).toContain('Succeeded')
    expect(wrapper.find('tbody td.is-truncate').attributes('title')).toBe('admin')
    expect(wrapper.find('.audit-content').attributes('title')).toBe('updated runtime_backend')
  })

  it('shows unknown actions and modules as sent', async () => {
    adminApi.getSystemAuditLogs.mockResolvedValue({ code: 0, data: { list: [{ id: 3, action: 'rotate_key', module: 'kms', status: 'partial', content: 'x' }], total: 1 } })
    const wrapper = mountSystem()
    await flushPromises()
    const cells = wrapper.findAll('tbody td').map(cell => cell.text())
    expect(cells).toEqual(expect.arrayContaining(['rotate_key', 'kms', 'partial']))
  })

  it('renders audit log row content from panel envelope response', async () => {
    adminApi.getSystemAuditLogs.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: {
        list: [
          {
            id: 2,
            action: 'delete',
            module: 'system',
            target_type: 'backup_record',
            username: 'operator',
            content: 'deleted backup #42',
            ip: '127.0.0.2',
            status: 'success',
            created_at: '2026-04-12T08:00:00Z'
          }
        ],
        total: 1,
        page: 1,
        page_size: 20
      },
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('deleted backup #42')
    expect(text).toContain('operator')
    expect(text).toContain('backup_record')
  })

  it('shows panel envelope errors in the table error state when audit log loading fails', async () => {
    adminApi.getSystemAuditLogs.mockResolvedValue({
      code: -1,
      msg: 'database unavailable',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.auditError).toBe('database unavailable')
    expect(wrapper.text()).toContain('database unavailable')
    expect(wrapper.vm.auditLogs).toEqual([])
    expect(wrapper.vm.auditTotal).toBe(0)
  })

  it('reads the filters and page from the query and writes them back', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/system/:section?', component: { template: '<div />' } }] })
    await router.push('/admin/system/audit?action=login&page=2&size=50')
    const replace = vi.spyOn(router, 'replace')
    const wrapper = mount(SettingsAudit, { global: { plugins: [router] } })
    await flushPromises()

    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledWith({ page: 2, page_size: 50, action: 'login' })
    expect(wrapper.vm.auditFilters).toMatchObject({ action: 'login' })

    adminApi.getSystemAuditLogs.mockClear()
    wrapper.vm.auditFilters.target_type = 'node'
    await wrapper.vm.applyAuditFilters()
    expect(replace).toHaveBeenLastCalledWith({ path: '/admin/system/audit', query: { action: 'login', target: 'node' } })
    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledWith({ page: 1, page_size: 20, action: 'login', target_type: 'node' })
  })
})
