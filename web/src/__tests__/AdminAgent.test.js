import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Agent from '@/views/admin/Agent.vue'
import { setLocale } from '@/i18n'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  createAgentTask: vi.fn(),
  executeAgentCommand: vi.fn(),
  getAgents: vi.fn(),
  listAgentDiagnosticTasks: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Agent', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getAgents.mockResolvedValue({ data: { agents: [] } })
    adminApi.listAgentDiagnosticTasks.mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders agents from legacy, panel envelope, and nested payloads', async () => {
    adminApi.getAgents
      .mockResolvedValueOnce({
        data: {
          agents: [{ node_id: 1, version: 'legacy', online: true, capabilities: ['diagnostic'] }]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          agents: [{ node_id: 2, version: 'panel', online: false, capabilities: [] }]
        },
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: {
            agents: [{ node_id: 3, version: 'nested', online: true, capabilities: ['logs'] }]
          }
        }
      })

    const wrapper = mount(Agent)
    await flushPromises()

    expect(wrapper.vm.agents[0].node_id).toBe(1)
    expect(wrapper.vm.agents[0].version).toBe('legacy')

    await wrapper.vm.fetchAgents()
    await flushPromises()

    expect(wrapper.vm.agents[0].node_id).toBe(2)
    expect(wrapper.vm.agents[0].version).toBe('panel')

    await wrapper.vm.fetchAgents()
    await flushPromises()

    expect(wrapper.vm.agents[0].node_id).toBe(3)
    expect(wrapper.vm.agents[0].version).toBe('nested')

    wrapper.unmount()
  })

  it('renders diagnostic task history from legacy, panel envelope, and nested payloads', async () => {
    adminApi.listAgentDiagnosticTasks
      .mockResolvedValueOnce({
        data: [{ task_id: 'legacy-task', node_id: 1, action: 'service_status' }]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [{ task_id: 'panel-task', node_id: 2, action: 'log_tail' }],
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: [{ task_id: 'nested-task', node_id: 3, action: 'service_restart' }]
        }
      })

    const wrapper = mount(Agent)
    await flushPromises()

    expect(wrapper.vm.taskHistory[0].task_id).toBe('legacy-task')

    await wrapper.vm.fetchTaskHistory()
    await flushPromises()

    expect(wrapper.vm.taskHistory[0].task_id).toBe('panel-task')

    await wrapper.vm.fetchTaskHistory()
    await flushPromises()

    expect(wrapper.vm.taskHistory[0].task_id).toBe('nested-task')

    wrapper.unmount()
  })

  it('reads enveloped execute responses for terminal output', async () => {
    adminApi.executeAgentCommand.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: { success: true, output: 'service is running' },
      ts: 1783526400000
    })

    const wrapper = mount(Agent)
    await flushPromises()

    wrapper.vm.selectedNodeId = 1
    wrapper.vm.selectedAction = 'service_status'
    await wrapper.vm.executeCommand()
    await flushPromises()

    expect(wrapper.vm.terminalLines.some(line => line.content === 'service is running')).toBe(true)

    wrapper.unmount()
  })

  describe('task dialog and feedback', () => {
    const Harness = { components: { Agent, UiHost }, template: '<div><Agent /><UiHost /></div>' }

    async function renderPage() {
      adminApi.getAgents.mockResolvedValue({ data: { agents: [{ node_id: 4, online: true, version: '1.0', hostname: 'hk' }] } })
      render(Harness)
      await screen.findByRole('button', { name: 'Actions for Node #4' })
    }

    // Row actions are in the row's "…" menu (UI U6).
    async function rowAction(user, name) {
      const trigger = screen.getByRole('button', { name: 'Actions for Node #4' })
      await user.click(trigger)
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name }))
      return trigger
    }

    it('sends a task from a dialog with inline validation and errors', async () => {
      const user = userEvent.setup()
      adminApi.createAgentTask.mockRejectedValueOnce(new Error('agent offline')).mockResolvedValueOnce({ code: 0 })
      await renderPage()
      await rowAction(user, 'Send Task')
      const dialog = await screen.findByRole('dialog', { name: 'Send Task' })
      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      expect(within(dialog).getByRole('alert').textContent).toBe('Please fill in the required fields')

      await user.click(within(dialog).getByRole('combobox', { name: 'Action' }))
      await user.click(within(await screen.findByRole('listbox')).getByRole('option', { name: 'Check service status' }))
      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('agent offline')
      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(toastMessages('success')).toEqual(['Task sent'])
      expect(adminApi.createAgentTask).toHaveBeenLastCalledWith(expect.objectContaining({ node_id: 4, type: 'diagnostic' }))
    })

    it('closes the task dialog with Esc and returns focus', async () => {
      const user = userEvent.setup()
      await renderPage()
      const opener = await rowAction(user, 'Send Task')
      await screen.findByRole('dialog')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))
      expect(adminApi.createAgentTask).not.toHaveBeenCalled()
    })

    it('explains the monitor shortcut in a toast', async () => {
      await renderPage()
      await rowAction(userEvent.setup(), 'Monitoring')
      expect(toastMessages('info')).toEqual(['View monitoring data for node #4'])
    })
  })

  it('lists agents in a table with status and shows a load error with retry', async () => {
    const user = userEvent.setup()
    adminApi.getAgents
      .mockRejectedValueOnce(Object.assign(new Error('Network Error'), { response: { data: { msg: 'control restarting' } } }))
      .mockResolvedValue({ data: { agents: [{ node_id: 9, online: false, version: '2.1.0', system: { os: 'linux', arch: 'amd64' }, capabilities: ['diagnostic'] }] } })
    render(Agent)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Agents didn’t load')
    expect(alert.textContent).toContain('control restarting')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    const table = await screen.findByRole('table', { name: 'Online Agents' })
    const row = within(table).getAllByRole('row')[1]
    expect(row.textContent).toContain('Node #9')
    expect(row.textContent).toContain('Offline')
    expect(row.textContent).toContain('linux amd64')
  })

  it('switches between the agents, terminal and task history tabs', async () => {
    const user = userEvent.setup()
    adminApi.listAgentDiagnosticTasks.mockResolvedValue({ data: [{ task_id: 't-1', node_id: 4, action: 'log_tail', success: true, duration_ms: 12 }] })
    render(Agent)
    await user.click(await screen.findByRole('tab', { name: 'Task History' }))
    const table = await screen.findByRole('table', { name: 'Task History' })
    expect(table.textContent).toContain('Tail service log')
    expect(table.textContent).toContain('12 ms')
    await user.click(screen.getByRole('tab', { name: 'Remote Terminal' }))
    expect(await screen.findByRole('log', { name: 'Terminal output' })).toBeTruthy()
  })
})
