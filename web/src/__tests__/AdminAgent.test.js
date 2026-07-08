import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Agent from '@/views/admin/Agent.vue'
import { setLocale } from '@/i18n'

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
    vi.spyOn(window, 'alert').mockImplementation(() => {})
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
})
