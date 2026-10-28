import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SettingsBackup from '@/views/admin/system/SettingsBackup.vue'
import SettingsBalancer from '@/views/admin/system/SettingsBalancer.vue'
import SettingsGeneral from '@/views/admin/system/SettingsGeneral.vue'
import { toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getLoadBalancers: vi.fn(),
  createLoadBalancer: vi.fn(),
  updateLoadBalancer: vi.fn(),
  deleteLoadBalancer: vi.fn(),
  runHealthCheck: vi.fn(),
}))

const kernelApi = vi.hoisted(() => ({
  getPanelBackups: vi.fn(),
  createPanelBackup: vi.fn(),
  downloadPanelBackup: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/api/kernel', () => kernelApi)

function mountSection(component = SettingsBackup) {
  return mount(component, {
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

const HOUR = 3_600_000

// GET /api/v5/backups as getPanelBackups answers it (the data of the
// server's answer).
function backupsAnswer({ stale = false, supported = true, backups } = {}) {
  const now = Date.now()
  const list = backups ?? [
    { id: 3, file: 'anix-control-20261013T034107Z-daily.tar', type: 'daily', size: 2_097_152, created_at: new Date(now - 2 * HOUR).toISOString(), verify_status: 'passed', verified_at: new Date(now - 2 * HOUR + 60_000).toISOString(), verify_error: '' },
    { id: 2, file: 'anix-control-20261012T034107Z-manual.tar', type: 'manual', size: 2_000_000, created_at: new Date(now - 26 * HOUR).toISOString(), verify_status: 'failed', verify_error: 'integrity: PRAGMA integrity_check: database disk image is malformed' },
    { id: 1, file: 'anix-control-20261011T034107Z-pre-migration.tar', type: 'pre-migration', size: 1_900_000, created_at: new Date(now - 50 * HOUR).toISOString(), verify_status: 'pending', verify_error: '' }
  ]
  return {
    supported,
    reason: supported ? '' : 'backups need SQLite: this Control runs on PostgreSQL',
    backups: supported ? list : [],
    summary: {
      last_verified: supported ? list.find(backup => backup.verify_status === 'passed') || null : null,
      stale,
      stale_after_hours: 36,
      next_daily_at: new Date(now + 10 * HOUR).toISOString(),
      directory: '/var/lib/anix-control/backups'
    },
    retention: { daily: 7, weekly: 4, pre_migration_days: 30, manual: 3, verify: 1 }
  }
}

describe('System settings: backups, subscription domains and load balancers', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getSystemAuditLogs.mockResolvedValue({ data: { data: { list: [], total: 0, page: 1, page_size: 20 } } })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.createLoadBalancer.mockResolvedValue({})
    adminApi.updateLoadBalancer.mockResolvedValue({})
    adminApi.deleteLoadBalancer.mockResolvedValue({})
    adminApi.runHealthCheck.mockResolvedValue({})
    kernelApi.getPanelBackups.mockResolvedValue(backupsAnswer())
    kernelApi.createPanelBackup.mockResolvedValue({ backup: { id: 4, file: 'anix-control-20261013T120000Z-manual.tar', verify_status: 'pending' } })
  })

  it('lists the bundles with their verification and the last verified backup', async () => {
    const wrapper = mountSection(SettingsBackup)
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('anix-control-20261013T034107Z-daily.tar')
    expect(text).toContain('Before a migration')
    expect(wrapper.find('[data-verify="passed"]').text()).toBe('Verified')
    expect(wrapper.find('[data-verify="failed"]').text()).toBe('Verification failed')
    expect(wrapper.find('[data-verify="failed"]').attributes('title')).toContain('integrity_check')
    expect(wrapper.find('[data-verify="pending"]').text()).toBe('Not verified yet')
    const lastVerified = wrapper.find('[data-test="backup-last-verified"]')
    expect(lastVerified.text()).toContain('2 hours ago')
    expect(wrapper.find('[data-test="backup-freshness"]').text()).toBe('Up to date')
    expect(text).toContain('/var/lib/anix-control/backups')
    expect(text).toContain('the newest 7 daily backups')
    // No restore in the console: the commands for the server.
    const commands = wrapper.find('[data-test="backup-restore-commands"]').text()
    expect(commands).toContain('sudo systemctl stop anix-control')
    expect(commands).toContain('sudo anix-control backup restore /var/lib/anix-control/backups/anix-control-20261013T034107Z-daily.tar --replace')
    expect(text).not.toMatch(/Restore…/)

    wrapper.unmount()
  })

  it('warns when no verified backup is recent enough, and when there is none', async () => {
    kernelApi.getPanelBackups.mockResolvedValueOnce(backupsAnswer({ stale: true }))
    let wrapper = mountSection(SettingsBackup)
    await flushPromises()
    expect(wrapper.find('[data-test="backup-freshness"]').text()).toBe('Older than 36 hours')
    wrapper.unmount()

    kernelApi.getPanelBackups.mockResolvedValueOnce(backupsAnswer({ stale: true, backups: [] }))
    wrapper = mountSection(SettingsBackup)
    await flushPromises()
    expect(wrapper.find('[data-test="backup-last-verified"]').text()).toContain('None yet')
    expect(wrapper.text()).toContain('No backups yet')
    wrapper.unmount()
  })

  it('says why there are no backups on PostgreSQL and offers no backup', async () => {
    kernelApi.getPanelBackups.mockResolvedValueOnce(backupsAnswer({ supported: false }))
    const wrapper = mountSection(SettingsBackup)
    await flushPromises()
    expect(wrapper.find('[data-test="backup-unsupported"]').text()).toContain('Backups need SQLite')
    expect(wrapper.find('[data-test="backup-now"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('backs up now and reloads the list; a running backup is said so', async () => {
    const wrapper = mountSection(SettingsBackup)
    await flushPromises()
    kernelApi.getPanelBackups.mockClear()

    await wrapper.find('[data-test="backup-now"]').trigger('click')
    await flushPromises()
    expect(kernelApi.createPanelBackup).toHaveBeenCalledTimes(1)
    expect(toastMessages('success')).toEqual([expect.stringContaining('anix-control-20261013T120000Z-manual.tar')])
    expect(kernelApi.getPanelBackups).toHaveBeenCalledTimes(1)

    kernelApi.createPanelBackup.mockRejectedValueOnce({ response: { status: 409, data: { error: { code: 'backup_in_progress', message: 'another backup or verification is running' } } } })
    await wrapper.find('[data-test="backup-now"]').trigger('click')
    await flushPromises()
    expect(toastMessages('error')).toEqual([expect.stringContaining('already running')])

    wrapper.unmount()
  })

  it('shows a load failure as an error state', async () => {
    kernelApi.getPanelBackups.mockRejectedValueOnce({ response: { status: 500, data: { error: { code: 'backups_unreadable', message: 'permission denied' } } } })
    const wrapper = mountSection(SettingsBackup)
    await flushPromises()
    expect(wrapper.text()).toContain('Couldn’t load the backups')
    expect(wrapper.text()).toContain('permission denied')
    wrapper.unmount()
  })

  it('downloads a bundle, and tells an administrator who is not super why not', async () => {
    const wrapper = mountSection(SettingsBackup)
    await flushPromises()
    const createObjectURL = vi.fn(() => 'blob:bundle')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    kernelApi.downloadPanelBackup.mockResolvedValueOnce({ blob: new Blob(['tar']), filename: 'anix-control-20261013T034107Z-daily.tar' })

    await wrapper.vm.download({ id: 3, file: 'anix-control-20261013T034107Z-daily.tar' })
    expect(kernelApi.downloadPanelBackup).toHaveBeenCalledWith(3)
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:bundle')

    kernelApi.downloadPanelBackup.mockRejectedValueOnce({ response: { status: 403, data: { error: { code: 'super_admin_required' } } } })
    await wrapper.vm.download({ id: 3, file: 'anix-control-20261013T034107Z-daily.tar' })
    expect(toastMessages('error')).toEqual([expect.stringContaining('Only a super administrator can download a backup')])
    vi.unstubAllGlobals()
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

    const wrapper = mountSection(SettingsGeneral)
    await flushPromises()

    expect(wrapper.vm.subscriptionPath).toBe('/sub')
    expect(wrapper.vm.subscriptionDomainsText).toBe('legacy.example.com\nbackup.example.com')

    await wrapper.vm.loadSubscriptionDomainSettings()
    await flushPromises()

    expect(wrapper.vm.subscriptionPath).toBe('/x')
    expect(wrapper.vm.subscriptionDomainsText).toBe('panel.example.com')

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

    const wrapper = mountSection(SettingsBalancer)
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

  it('shows a load balancer list failure as the table error state', async () => {
    adminApi.getLoadBalancers.mockResolvedValueOnce({
      code: -1,
      msg: 'balancer list rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mountSection(SettingsBalancer)
    await flushPromises()

    expect(wrapper.vm.balancers).toEqual([])
    expect(wrapper.vm.balancersError).toBe('balancer list rejected')
    expect(wrapper.text()).toContain('balancer list rejected')

    wrapper.unmount()
  })

  it('keeps load balancer modal open when enveloped save fails', async () => {
    const wrapper = mountSection(SettingsBalancer)
    await flushPromises()
    adminApi.getLoadBalancers.mockClear()
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

    expect(wrapper.vm.balancerError).toContain('balancer save rejected')
    expect(toastMessages()).toEqual([])
    expect(wrapper.vm.showBalancerModal).toBe(true)
    expect(adminApi.getLoadBalancers).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not report health-check success when enveloped response fails', async () => {
    const wrapper = mountSection(SettingsBalancer)
    await flushPromises()
    adminApi.getLoadBalancers.mockClear()
    adminApi.runHealthCheck.mockResolvedValueOnce({
      code: -1,
      msg: 'health check rejected',
      data: null,
      ts: 1783526400000
    })

    await wrapper.vm.runHealthCheckRequest({ id: 1, name: 'Rejected LB' })
    await flushPromises()

    expect(toastMessages('error')).toEqual([expect.stringContaining('health check rejected')])
    expect(toastMessages('success')).toEqual([])
    expect(adminApi.getLoadBalancers).not.toHaveBeenCalled()

    wrapper.unmount()
  })
})
