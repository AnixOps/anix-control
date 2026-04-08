import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NodeX from '@/views/admin/NodeX.vue'

const adminApi = vi.hoisted(() => ({
  getNodeXRuntimeStatus: vi.fn(),
  getSystemConfig: vi.fn(),
  listForwardRuntimeJobs: vi.fn(),
  runNodeXRuntimeDoctor: vi.fn(),
  setSystemConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountNodeX() {
  return mount(NodeX, {
    global: {
      stubs: {
        'router-link': true
      }
    }
  })
}

describe('NodeX admin page', () => {
  beforeEach(() => {
    vi.resetAllMocks()

    const configMap = {
      'forward.runtime.nodex_mode': { value: true },
      'forward.runtime_backend': { value: 'gost' },
      'forward.runtime.nodex.base_url': { value: 'http://127.0.0.1:18081' },
      'forward.runtime.nodex.token': { value: 'secret-token' },
      'forward.runtime.nodex.timeout_seconds': { value: 15 }
    }

    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))
    adminApi.getNodeXRuntimeStatus.mockResolvedValue({
      data: {
        data: {
          config: {
            nodeXMode: true,
            backend: 'gost',
            baseUrl: 'http://127.0.0.1:18081',
            tokenConfigured: true,
            timeoutSeconds: 15
          },
          reachability: {
            ready: true,
            reason: 'NodeX /health responded with ok from the panel host'
          },
          runtimeReady: {
            ready: true,
            reason: 'NodeX runtime status responded and advertises gost support'
          },
          runtimeStatus: {
            version: '1.0.0',
            executePath: '/api/v2/internal/forward/runtime/execute'
          },
          summary: 'NodeX control plane is ready.'
        }
      }
    })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({
      data: {
        list: [
          {
            id: 1,
            action: 'attach',
            status: 2,
            forward_id: 11,
            tunnel_id: 22,
            node_id: 33,
            result: 'attached'
          }
        ]
      }
    })
    adminApi.runNodeXRuntimeDoctor.mockResolvedValue({
      data: {
        data: {
          summary: 'doctor ok',
          commands: {
            powerShell: ['Invoke-WebRequest http://127.0.0.1:18081/health'],
            bash: ['curl -fsSL http://127.0.0.1:18081/health'],
            upgrade: ['go run ./cmd/control-plane --version'],
            references: ['NodeX repo: https://github.com/zdwtest/NodeX']
          }
        }
      }
    })
    adminApi.setSystemConfig.mockResolvedValue({})
  })

  it('loads dedicated NodeX status and gost jobs on mount', async () => {
    const wrapper = mountNodeX()
    await flushPromises()

    expect(adminApi.getNodeXRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(adminApi.listForwardRuntimeJobs).toHaveBeenCalledWith({ backend: 'gost', limit: 10 })
    expect(wrapper.text()).toContain('Reachable')
    expect(wrapper.text()).toContain('#1 attach')
  })

  it('saves NodeX mode config from the dedicated page', async () => {
    const wrapper = mountNodeX()
    await flushPromises()
    adminApi.setSystemConfig.mockClear()

    wrapper.vm.nodeXMode = true
    wrapper.vm.nodeXBaseUrl = 'https://nodex.internal'
    wrapper.vm.nodeXToken = 'new-token'
    wrapper.vm.nodeXTimeout = 42
    await wrapper.vm.saveNodeXConfig()
    await flushPromises()

    const calls = adminApi.setSystemConfig.mock.calls
    const modeCall = calls.find(([key]) => key === 'forward.runtime.nodex_mode')
    const backendCall = calls.find(([key]) => key === 'forward.runtime_backend')
    const baseUrlCall = calls.find(([key]) => key === 'forward.runtime.nodex.base_url')
    const tokenCall = calls.find(([key]) => key === 'forward.runtime.nodex.token')
    const timeoutCall = calls.find(([key]) => key === 'forward.runtime.nodex.timeout_seconds')

    expect(modeCall?.[1].value).toBe(true)
    expect(backendCall?.[1].value).toBe('gost')
    expect(baseUrlCall?.[1].value).toBe('https://nodex.internal')
    expect(tokenCall?.[1].value).toBe('new-token')
    expect(timeoutCall?.[1].value).toBe(42)
    expect(wrapper.vm.validationError).toBe('')
  })
})
