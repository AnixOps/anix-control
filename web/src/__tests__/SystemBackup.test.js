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

  it('loads backup config from legacy and panel envelope payloads', async () => {
    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.backupConfig.interval).toBe(12)
    expect(wrapper.vm.backupConfig.keep_count).toBe(6)
    expect(wrapper.vm.backupConfig.storage_type).toBe('s3')

    adminApi.getBackupConfig.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        enabled: false,
        interval: 18,
        keep_count: 9,
        storage_type: 'local',
        storage_path: 'daily-backups'
      },
      ts: 1783526400000
    })

    await wrapper.vm.fetchBackupConfig()
    await flushPromises()

    expect(wrapper.vm.backupConfig.enabled).toBe(false)
    expect(wrapper.vm.backupConfig.interval).toBe(18)
    expect(wrapper.vm.backupConfig.keep_count).toBe(9)
    expect(wrapper.vm.backupConfig.storage_type).toBe('local')
    expect(wrapper.vm.backupConfig.storage_path).toBe('daily-backups')

    wrapper.unmount()
  })

  it('loads backup records from legacy and panel envelope payloads', async () => {
    adminApi.getBackups
      .mockResolvedValueOnce({
        data: {
          list: [
            {
              id: 1,
              filename: 'legacy-backup.zip',
              status: 'completed'
            }
          ]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [
            {
              id: 2,
              filename: 'panel-backup.zip',
              status: 'completed'
            }
          ],
          total: 1
        },
        ts: 1783526400000
      })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.backups).toHaveLength(1)
    expect(wrapper.vm.backups[0].filename).toBe('legacy-backup.zip')

    await wrapper.vm.fetchBackups()
    await flushPromises()

    expect(wrapper.vm.backups).toHaveLength(1)
    expect(wrapper.vm.backups[0]).toMatchObject({
      id: 2,
      filename: 'panel-backup.zip',
      status: 'completed'
    })

    wrapper.unmount()
  })

  it('loads subscription settings from legacy and panel envelope payloads', async () => {
    adminApi.getSubscriptionSettings
      .mockResolvedValueOnce({
        data: {
          subscribe_path: '/sub',
          subscribe_domains: ['legacy.example.com', 'backup.example.com']
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          subscribe_path: '/x',
          subscribe_domains: ['panel.example.com']
        },
        ts: 1783526400000
      })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.subscriptionPath).toBe('/sub')
    expect(wrapper.vm.subscriptionDomainsText).toBe('legacy.example.com\nbackup.example.com')

    await wrapper.vm.loadSubscriptionDomainSettings()
    await flushPromises()

    expect(wrapper.vm.subscriptionPath).toBe('/x')
    expect(wrapper.vm.subscriptionDomainsText).toBe('panel.example.com')

    wrapper.unmount()
  })

  it('renders backup stats from legacy and panel envelope payloads', async () => {
    adminApi.getBackupStats
      .mockResolvedValueOnce({
        data: {
          last_backup: null,
          total_count: 2,
          total_size: 2048
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          last_backup: null,
          total_count: 3,
          total_size: 4096
        },
        ts: 1783526400000
      })

    const wrapper = mountSystem()
    await flushPromises()

    let statValues = wrapper.findAll('.backup-stats .stat-value').map(node => node.text())
    expect(statValues).toEqual(['2', '2.00 KB', '-'])

    await wrapper.vm.fetchBackupStats()
    await flushPromises()

    statValues = wrapper.findAll('.backup-stats .stat-value').map(node => node.text())
    expect(statValues).toEqual(['3', '4.00 KB', '-'])

    wrapper.unmount()
  })

  it('logs backup panel envelope read failures instead of accepting empty payloads', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getBackupConfig.mockResolvedValueOnce({
      code: -1,
      msg: 'backup config rejected',
      data: null,
      ts: 1783526400000
    })
    adminApi.getBackups.mockResolvedValueOnce({
      code: -1,
      msg: 'backup list rejected',
      data: null,
      ts: 1783526400000
    })
    adminApi.getBackupStats.mockResolvedValueOnce({
      code: -1,
      msg: 'backup stats rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    const loggedErrors = consoleError.mock.calls.map(call => call[1]).filter(err => err instanceof Error)
    expect(loggedErrors.map(err => err.message)).toEqual(expect.arrayContaining([
      'backup config rejected',
      'backup list rejected',
      'backup stats rejected'
    ]))
    expect(wrapper.vm.backups).toEqual([])
    expect(wrapper.vm.backupStats).toEqual({})

    consoleError.mockRestore()
    wrapper.unmount()
  })

  it('does not report backup config save success when enveloped response fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    window.alert.mockClear()
    adminApi.updateBackupConfig.mockResolvedValueOnce({
      code: -1,
      msg: 'backup config save rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.saveBackupConfig()
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('backup config save rejected'))
    expect(window.alert).not.toHaveBeenCalledWith(expect.stringContaining('Saved successfully'))

    wrapper.unmount()
  })

  it('does not refresh backups when enveloped create response fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    window.alert.mockClear()
    adminApi.getBackups.mockClear()
    adminApi.getBackupStats.mockClear()
    adminApi.createBackup.mockResolvedValueOnce({
      code: -1,
      msg: 'backup create rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.createBackupRequest()
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('backup create rejected'))
    expect(window.alert).not.toHaveBeenCalledWith(expect.stringContaining('Backup started'))
    expect(adminApi.getBackups).not.toHaveBeenCalled()
    expect(adminApi.getBackupStats).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not refresh backups when enveloped delete response fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    window.alert.mockClear()
    adminApi.getBackups.mockClear()
    adminApi.getBackupStats.mockClear()
    adminApi.deleteBackup.mockResolvedValueOnce({
      code: -1,
      msg: 'backup delete rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.deleteBackupRequest({ id: 1, filename: 'rejected.zip' })
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('backup delete rejected'))
    expect(adminApi.getBackups).not.toHaveBeenCalled()
    expect(adminApi.getBackupStats).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not report restore success when enveloped response fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    window.alert.mockClear()
    adminApi.restoreBackup.mockResolvedValueOnce({
      code: -1,
      msg: 'backup restore rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.restoreBackupRequest({ id: 1, filename: 'rejected.zip' })
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('backup restore rejected'))
    expect(window.alert).not.toHaveBeenCalledWith(expect.stringContaining('Restore completed'))

    wrapper.unmount()
  })

  it('loads load balancers from legacy and panel envelope payloads', async () => {
    adminApi.getLoadBalancers
      .mockResolvedValueOnce({
        data: {
          list: [
            {
              id: 1,
              name: 'Legacy LB',
              group_id: 1,
              strategy: 'round-robin',
              health_check: true,
              enabled: true,
              weights: {}
            }
          ]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [
            {
              id: 2,
              name: 'Panel LB',
              group_id: 2,
              strategy: 'least-load',
              health_check: false,
              enabled: true,
              weights: { '10': 3 }
            }
          ],
          total: 1
        },
        ts: 1783526400000
      })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.balancers).toHaveLength(1)
    expect(wrapper.vm.balancers[0].name).toBe('Legacy LB')

    await wrapper.vm.fetchBalancers()
    await flushPromises()

    expect(wrapper.vm.balancers).toHaveLength(1)
    expect(wrapper.vm.balancers[0]).toMatchObject({
      id: 2,
      name: 'Panel LB',
      strategy: 'least-load'
    })

    wrapper.unmount()
  })

  it('logs load balancer panel envelope list failures instead of accepting empty lists', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getLoadBalancers.mockResolvedValueOnce({
      code: -1,
      msg: 'balancer list rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.balancers).toEqual([])
    expect(consoleError).toHaveBeenCalled()
    expect(consoleError.mock.calls[0][1]).toBeInstanceOf(Error)
    expect(consoleError.mock.calls[0][1].message).toBe('balancer list rejected')

    consoleError.mockRestore()
    wrapper.unmount()
  })

  it('keeps load balancer modal open when enveloped save fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.getLoadBalancers.mockClear()
    window.alert.mockClear()
    adminApi.createLoadBalancer.mockResolvedValueOnce({
      code: -1,
      msg: 'balancer save rejected',
      data: null,
      ts: 1783526400000
    })

    wrapper.vm.openBalancerModal()
    Object.assign(wrapper.vm.balancerForm, {
      name: 'Rejected LB',
      group_id: 0,
      strategy: 'round-robin',
      health_check: true,
      check_interval: 60,
      weights_json: ''
    })

    await wrapper.vm.saveBalancer()
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('balancer save rejected'))
    expect(wrapper.vm.showBalancerModal).toBe(true)
    expect(adminApi.getLoadBalancers).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not report health-check success when enveloped response fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.getLoadBalancers.mockClear()
    window.alert.mockClear()
    adminApi.runHealthCheck.mockResolvedValueOnce({
      code: -1,
      msg: 'health check rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.runHealthCheckRequest({ id: 1, name: 'Rejected LB' })
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('health check rejected'))
    expect(window.alert).not.toHaveBeenCalledWith(expect.stringContaining('Health check completed'))
    expect(adminApi.getLoadBalancers).not.toHaveBeenCalled()

    wrapper.unmount()
  })
})
