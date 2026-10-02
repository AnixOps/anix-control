import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import System from '@/views/admin/System.vue'
import { toastMessages } from './helpers/feedback'

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

describe('System runtime configuration', () => {
  beforeEach(() => {
    vi.resetAllMocks()

    const configMap = {
      'forward.runtime.nodex_mode': { value: false },
      'forward.runtime_backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.config': { value: '{"inventory":"local"}' },
      'forward.runtime.nodex.base_url': { value: 'https://nodex.example' },
      'forward.runtime.nodex.token': { value: 'token' },
      'forward.runtime.nodex.timeout_seconds': { value: 15 }
    }

    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getSystemAuditLogs.mockResolvedValue({ data: { data: { list: [], total: 0, page: 1, page_size: 20 } } })
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardRuntimeStatus.mockResolvedValue({ data: { data: null } })
    adminApi.runForwardRuntimeDoctor.mockResolvedValue({ data: { data: null } })
  })

  it('loads NodeX mode as the active runtime without writing config', async () => {
    const configMap = {
      'forward.runtime.nodex_mode': { value: true },
      'forward.runtime_backend': { value: 'nftables_ansible' },
      'forward.runtime.nodex.base_url': { value: 'https://nodex.internal' },
      'forward.runtime.nodex.token': { value: 'secret-token' }
    }
    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.runtimeNodeXMode).toBe(true)
    expect(wrapper.vm.runtimeBackend).toBe('gost')
    expect(wrapper.vm.runtimeNodeXBaseUrl).toBe('https://nodex.internal')
    expect(wrapper.vm.runtimeNodeXToken).toBe('secret-token')
    expect(adminApi.setSystemConfig).not.toHaveBeenCalled()
  })

  it('shows a masked NodeX token as configured without putting it in the commands', async () => {
    const configMap = {
      'forward.runtime.nodex_mode': { value: true },
      'forward.runtime_backend': { value: 'gost' },
      'forward.runtime.nodex.base_url': { value: 'https://nodex.internal' },
      'forward.runtime.nodex.token': { value: '********', display_value: '********', sensitive: true, has_value: true }
    }
    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.runtimeNodeXToken).toBe('********')
    expect(wrapper.vm.runtimeOperatorToken).toBe('<FORWARD_API_TOKEN>')
    expect(wrapper.vm.runtimeDisplayedCommands.bash.join('\n')).toContain('Bearer <FORWARD_API_TOKEN>')
    expect(wrapper.vm.runtimeDisplayedCommands.bash.join('\n')).not.toContain('********')
    expect(adminApi.setSystemConfig).not.toHaveBeenCalled()
  })

  it('loads runtime and config list data from panel envelope responses', async () => {
    const configMap = {
      'forward.runtime.nodex_mode': { value: false },
      'forward.runtime_backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.config': {
        value: JSON.stringify({
          inventory: 'panel-hosts.ini',
          playbookApply: 'apply-panel.yml',
          playbookRemove: 'remove-panel.yml'
        })
      },
      'forward.runtime.nodex.base_url': { value: 'https://nodex.panel' },
      'forward.runtime.nodex.token': { value: 'panel-token' },
      'forward.runtime.nodex.timeout_seconds': { value: 33 }
    }
    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({
      code: 0,
      msg: '操作成功',
      data: configMap[key] ?? { value: '' },
      ts: 1783526400000
    }))
    adminApi.getSystemConfigs.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: {
        list: [
          {
            key: 'site.name',
            value: 'AnixOps',
            remark: 'Site name',
            sensitive: false,
            has_value: true
          }
        ],
        total: 1
      },
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.runtimeBackend).toBe('nftables_ansible')
    expect(wrapper.vm.runtimeNodeXMode).toBe(false)
    expect(wrapper.vm.runtimeNodeXBaseUrl).toBe('https://nodex.panel')
    expect(wrapper.vm.runtimeNodeXToken).toBe('panel-token')
    expect(wrapper.vm.runtimeNodeXTimeout).toBe(33)
    expect(wrapper.vm.runtimeAnsibleForm.inventory).toBe('panel-hosts.ini')
    expect(wrapper.vm.configs).toHaveLength(1)
    expect(wrapper.vm.configs[0]).toMatchObject({
      key: 'site.name',
      value: 'AnixOps',
      description: 'Site name',
      sensitive: false,
      has_value: true
    })
  })

  it('logs panel envelope config list failures instead of accepting them as empty lists', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getSystemConfigs.mockResolvedValueOnce({
      code: -1,
      msg: 'config list rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mountSystem()
    await flushPromises()

    expect(wrapper.vm.configs).toEqual([])
    expect(consoleError).toHaveBeenCalledWith(
      'Failed to load configs',
      expect.any(Error)
    )

    consoleError.mockRestore()
  })

  it('keeps the config modal open when an enveloped config save fails', async () => {
    const wrapper = mountSystem()
    await flushPromises()

    adminApi.getSystemConfigs.mockClear()
    adminApi.setSystemConfig.mockResolvedValueOnce({
      code: -1,
      msg: 'config save rejected',
      data: null,
      ts: 1783526400000
    })

    wrapper.vm.openConfigModal()
    wrapper.vm.configForm.key = 'site.name'
    wrapper.vm.configForm.value = 'AnixOps'
    await wrapper.vm.saveConfig()
    await flushPromises()

    expect(wrapper.vm.configError).toContain('config save rejected')
    expect(toastMessages()).toEqual([])
    expect(wrapper.vm.showConfigModal).toBe(true)
    expect(adminApi.getSystemConfigs).not.toHaveBeenCalled()

  })
})
