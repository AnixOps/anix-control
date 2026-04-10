import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import LocalRuntime from '@/views/admin/LocalRuntime.vue'

const adminApi = vi.hoisted(() => ({
  getLocalRuntimeStatus: vi.fn(),
  getSystemConfig: vi.fn(),
  listForwardRuntimeJobs: vi.fn(),
  runLocalRuntimeDoctor: vi.fn(),
  setSystemConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountLocalRuntime() {
  return mount(LocalRuntime, {
    global: {
      stubs: {
        'router-link': true
      }
    }
  })
}

describe('Local runtime admin page', () => {
  beforeEach(() => {
    vi.resetAllMocks()

    const configMap = {
      'forward.runtime.nodex_mode': { value: false },
      'forward.runtime_backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.backend': { value: 'nftables_ansible' },
      'forward.runtime.ansible.config': {
        value: JSON.stringify({
          inventory: 'inventory.ini',
          playbookApply: 'apply-nft.yml',
          playbookRemove: 'remove-nft.yml',
          command: 'ansible-playbook',
          workingDir: 'ansible'
        })
      }
    }

    adminApi.getSystemConfig.mockImplementation((key) => Promise.resolve({ data: configMap[key] ?? { value: '' } }))
    adminApi.getLocalRuntimeStatus.mockResolvedValue({
      data: {
        data: {
          attachment: { model: 'local_nftables_ansible_stateless' },
          reachability: { ready: true, reason: 'ansible-playbook is available on the panel host' },
          runtimeReady: { ready: true, reason: 'Local ansible executor resolved inventory/playbooks and is ready to queue jobs' },
          localAnsible: {
            command: 'ansible-playbook',
            firewallDriver: 'nftables',
            commandFound: true,
            inventoryExists: true,
            applyPlaybookExists: true,
            removePlaybookExists: true,
            workingDirExists: true,
            timeoutSeconds: 120
          },
          summary: 'Local nftables/Ansible executor is ready.'
        }
      }
    })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({
      data: {
        list: [
          { id: 2, action: 'attach', status: 2, forward_id: 10, tunnel_id: 20, node_id: 30, result: 'applied' }
        ]
      }
    })
    adminApi.runLocalRuntimeDoctor.mockResolvedValue({
      data: {
        data: {
          summary: 'doctor ok',
          commands: {
            powerShell: ['Get-Command ansible-playbook'],
            bash: ['command -v ansible-playbook'],
            upgrade: ['go test ./internal/service/... -run ForwardRuntime'],
            references: ['docs/guide/forward-relay-onboarding.md']
          }
        }
      }
    })
    adminApi.setSystemConfig.mockResolvedValue({})
  })

  it('loads dedicated local status and local jobs on mount', async () => {
    const wrapper = mountLocalRuntime()
    await flushPromises()
    const heroText = wrapper.find('.hero-card').text()
    const bannerText = wrapper.find('.mode-banner').text()
    const executorHint = wrapper.find('.runtime-local-head .hint').text()

    expect(adminApi.getLocalRuntimeStatus).toHaveBeenCalledTimes(1)
    expect(adminApi.listForwardRuntimeJobs).toHaveBeenCalledWith({ backend: 'nftables_ansible', limit: 10 })
    expect(wrapper.text()).toContain('Local runtime active')
    expect(heroText).toContain('nftables / Ansible')
    expect(heroText).not.toContain('nftables_ansible')
    expect(bannerText).toContain('nftables / Ansible')
    expect(bannerText).not.toContain('nftables_ansible')
    expect(executorHint).toContain('forward.runtime_backend=nftables_ansible')
    expect(wrapper.text()).toContain('#2 attach')
  })

  it('saves and activates local runtime config from the dedicated page', async () => {
    const wrapper = mountLocalRuntime()
    await flushPromises()
    adminApi.setSystemConfig.mockClear()

    wrapper.vm.runtimeAnsibleForm.inventory = 'hosts.ini'
    wrapper.vm.runtimeAnsibleForm.playbookApply = 'apply-local.yml'
    wrapper.vm.runtimeAnsibleForm.playbookRemove = 'remove-local.yml'
    wrapper.vm.runtimeAnsibleForm.command = 'ansible-playbook'
    wrapper.vm.runtimeAnsibleForm.workingDir = 'deploy/ansible'
    await wrapper.vm.saveLocalConfig()
    await flushPromises()

    const calls = adminApi.setSystemConfig.mock.calls
    const modeCall = calls.find(([key]) => key === 'forward.runtime.nodex_mode')
    const backendCall = calls.find(([key]) => key === 'forward.runtime_backend')
    const backendPreferenceCall = calls.find(([key]) => key === 'forward.runtime.ansible.backend')
    const payloadCall = calls.find(([key]) => key === 'forward.runtime.ansible.config')

    expect(modeCall?.[1].value).toBe(false)
    expect(backendCall?.[1].value).toBe('nftables_ansible')
    expect(backendPreferenceCall?.[1].value).toBe('nftables_ansible')
    expect(payloadCall?.[1].value).toContain('"inventory":"hosts.ini"')
    expect(wrapper.vm.validationError).toBe('')
  })
})
