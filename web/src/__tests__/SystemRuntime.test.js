import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import System from '@/views/admin/System.vue'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getBackupConfig: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  getLoadBalancers: vi.fn(),
  listForwardRuntimeJobs: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountSystem() {
  return mount(System, {
    global: {
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
      'forward.runtime_backend': { value: 'iptables_ansible' },
      'forward.runtime.iptables_ansible.config': { value: '{"inventory":"local"}' },
      'forward.runtime.nodex.base_url': { value: 'https://nodex.example' },
      'forward.runtime.nodex.token': { value: 'token' },
      'forward.runtime.nodex.timeout_seconds': { value: 15 }
    }

    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))
    adminApi.setSystemConfig.mockResolvedValue({})
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
  })

  it('saves NodeX runtime config when NodeX mode is enabled', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.setSystemConfig.mockClear()

    wrapper.vm.runtimeNodeXMode = true
    wrapper.vm.runtimeNodeXBaseUrl = 'https://nodex.internal'
    wrapper.vm.runtimeNodeXToken = 'secret-token'
    wrapper.vm.runtimeNodeXTimeout = 42
    wrapper.vm.runtimeConfigJson = '{"inventory":"node"}'
    await wrapper.vm.saveForwardRuntimeConfig()
    await flushPromises()

    const calls = adminApi.setSystemConfig.mock.calls
    const modeCall = calls.find(([key]) => key === 'forward.runtime.nodex_mode')
    const backendCall = calls.find(([key]) => key === 'forward.runtime_backend')
    const ansibleCall = calls.find(([key]) => key === 'forward.runtime.iptables_ansible.config')
    const baseUrlCall = calls.find(([key]) => key === 'forward.runtime.nodex.base_url')
    const tokenCall = calls.find(([key]) => key === 'forward.runtime.nodex.token')
    const timeoutCall = calls.find(([key]) => key === 'forward.runtime.nodex.timeout_seconds')

    expect(modeCall?.[1].value).toBe(true)
    expect(backendCall?.[1].value).toBe('gost')
    expect(ansibleCall).toBeUndefined()
    expect(baseUrlCall?.[1].value).toBe('https://nodex.internal')
    expect(tokenCall?.[1].value).toBe('secret-token')
    expect(timeoutCall?.[1].value).toBe(42)
    expect(wrapper.vm.runtimeValidationError).toBe('')
  })

  it('blocks saving when NodeX base URL is missing', async () => {
    const wrapper = mountSystem()
    await flushPromises()
    adminApi.setSystemConfig.mockClear()

    wrapper.vm.runtimeNodeXMode = true
    wrapper.vm.runtimeNodeXBaseUrl = ''
    wrapper.vm.runtimeNodeXToken = 'token'
    await wrapper.vm.saveForwardRuntimeConfig()

    expect(adminApi.setSystemConfig).not.toHaveBeenCalled()
    expect(wrapper.vm.runtimeValidationError).toBe('NodeX base URL is required in NodeX Mode')
  })
})
