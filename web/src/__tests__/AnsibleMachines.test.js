import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { render, screen, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
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

  it('lists machines in a table with reachability and filters on the server', async () => {
    const user = userEvent.setup()
    render(AnsibleMachines, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    const table = await screen.findByRole('table', { name: 'Execution Targets' })
    const row = within(table).getAllByRole('row')[1]
    expect(row.textContent).toContain('relay-exec-01')
    expect(row.textContent).toContain('1.2.3.4:22')
    expect(row.textContent).toContain('Online')
    await user.click(screen.getByRole('button', { name: 'Offline' }))
    expect(adminApi.getAnsibleMachines).toHaveBeenLastCalledWith({ page: 1, page_size: 200, type: 'relay', status: 0 })
    await user.click(screen.getByRole('button', { name: 'Offline' }))
    expect(adminApi.getAnsibleMachines).toHaveBeenLastCalledWith({ page: 1, page_size: 200, type: 'relay' })
  })

  it('shows a load error with retry', async () => {
    const user = userEvent.setup()
    adminApi.getAnsibleMachines.mockRejectedValueOnce({ response: { data: { msg: 'inventory unreadable' } } })
    render(AnsibleMachines, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Failed to load Ansible machines')
    expect(alert.textContent).toContain('inventory unreadable')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('table')).toBeTruthy()
  })
})
