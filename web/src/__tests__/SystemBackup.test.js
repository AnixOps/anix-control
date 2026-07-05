import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import System from '@/views/admin/System.vue'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getBackupConfig: vi.fn(),
  updateBackupConfig: vi.fn(),
  createBackup: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  deleteBackup: vi.fn(),
  restoreBackup: vi.fn(),
  getLoadBalancers: vi.fn(),
  createLoadBalancer: vi.fn(),
  updateLoadBalancer: vi.fn(),
  deleteLoadBalancer: vi.fn(),
  runHealthCheck: vi.fn(),
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
      },
      mocks: {
        $t: (_key, fallback) => fallback || _key
      }
    }
  })
}

describe('System backup configuration', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getSystemAuditLogs.mockResolvedValue({ data: { data: { list: [], total: 0, page: 1, page_size: 20 } } })
    adminApi.getBackupConfig.mockResolvedValue({
      data: {
        enabled: true,
        interval: 12,
        keep_count: 6,
        storage_type: 's3',
        s3_access_key_sensitive: true,
        s3_access_key_has_value: true,
        s3_secret_key_sensitive: true,
        s3_secret_key_has_value: true
      }
    })
    adminApi.updateBackupConfig.mockResolvedValue({})
    adminApi.createBackup.mockResolvedValue({})
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.deleteBackup.mockResolvedValue({})
    adminApi.restoreBackup.mockResolvedValue({})
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.createLoadBalancer.mockResolvedValue({})
    adminApi.updateLoadBalancer.mockResolvedValue({})
    adminApi.deleteLoadBalancer.mockResolvedValue({})
    adminApi.runHealthCheck.mockResolvedValue({})
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardRuntimeStatus.mockResolvedValue({ data: { data: null } })
    adminApi.runForwardRuntimeDoctor.mockResolvedValue({ data: { data: null } })
    vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.spyOn(window, 'confirm').mockImplementation(() => true)
  })

  it('sends preserve_existing_sensitive when sensitive S3 keys are left blank', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.updateBackupConfig.mockClear()

    wrapper.vm.backupConfig = {
      ...wrapper.vm.backupConfig,
      enabled: true,
      interval: 8,
      keep_count: 5,
      backup_database: true,
      backup_files: false,
      storage_type: 's3',
      s3_bucket: 'panel-backups',
      s3_region: 'us-east-1',
      s3_endpoint: '',
      s3_access_key: '',
      s3_secret_key: '',
      s3_access_key_sensitive: true,
      s3_access_key_has_value: true,
      s3_secret_key_sensitive: true,
      s3_secret_key_has_value: true
    }

    await wrapper.vm.saveBackupConfig()

    expect(adminApi.updateBackupConfig).toHaveBeenCalledTimes(1)
    const payload = adminApi.updateBackupConfig.mock.calls[0][0]
    expect(payload.storage_type).toBe('s3')
    expect(payload.s3_bucket).toBe('panel-backups')
    expect(payload.s3_access_key).toBe('')
    expect(payload.s3_secret_key).toBe('')
    expect(payload.preserve_existing_sensitive).toBe(true)
  })
})
