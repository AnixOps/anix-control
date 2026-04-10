import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AnsibleMachines from '@/views/admin/AnsibleMachines.vue'

const adminApi = vi.hoisted(() => ({
  checkAnsibleMachine: vi.fn(),
  createAnsibleMachine: vi.fn(),
  deleteAnsibleMachine: vi.fn(),
  getAnsibleMachine: vi.fn(),
  getAnsibleMachines: vi.fn(),
  syncAnsibleMachineStats: vi.fn(),
  toggleAnsibleMachine: vi.fn(),
  updateAnsibleMachine: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountAnsibleMachines() {
  return mount(AnsibleMachines, {
    global: {
      stubs: {
        'router-link': true
      }
    }
  })
}

describe('AnsibleMachines admin page', () => {
  beforeEach(() => {
    vi.resetAllMocks()

    adminApi.getAnsibleMachines.mockResolvedValue({
      data: {
        data: {
          list: [
            {
              id: 1,
              name: 'relay-exec-01',
              host: '1.2.3.4',
              port: 22,
              enabled: true,
              status: 1
            }
          ]
        }
      }
    })
    adminApi.createAnsibleMachine.mockResolvedValue({
      data: {
        data: {
          id: 2
        }
      }
    })
    adminApi.checkAnsibleMachine.mockRejectedValue({
      response: {
        data: {
          msg: 'dial tcp timeout'
        }
      }
    })
  })

  it('closes the editor after a successful save', async () => {
    const wrapper = mountAnsibleMachines()
    await flushPromises()

    await wrapper.vm.openEditor()
    wrapper.vm.form.name = 'relay-exec-02'
    wrapper.vm.form.host = '5.6.7.8'
    wrapper.vm.form.port = '2222'
    await wrapper.vm.submitForm()
    await flushPromises()

    expect(adminApi.createAnsibleMachine).toHaveBeenCalledWith({
      name: 'relay-exec-02',
      type: 'relay',
      host: '5.6.7.8',
      port: 2222,
      weight: 1
    })
    expect(wrapper.vm.editorOpen).toBe(false)
  })

  it('surfaces action failures inline on the machine card', async () => {
    const wrapper = mountAnsibleMachines()
    await flushPromises()

    await wrapper.vm.checkMachine({ id: 1 })
    await flushPromises()

    expect(wrapper.vm.results[1].success).toBe(false)
    expect(wrapper.vm.results[1].message).toContain('dial tcp timeout')
  })
})
