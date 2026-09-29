import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const kernelPluginsApi = vi.hoisted(() => ({
  list: vi.fn(),
  releases: vi.fn(),
  installations: vi.fn(),
  extensions: vi.fn(),
  upsertInstallation: vi.fn(),
  action: vi.fn(),
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  operations: vi.fn()
}))

vi.mock('@/api', () => ({ default: { post: vi.fn() }, kernelAuthApi: { login: vi.fn() }, kernelPluginsApi }))

import Plugins from '@/views/Plugins.vue'

describe('Plugins view', () => {
  let pinia

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.resetAllMocks()
    sessionStorage.setItem('kernel_token', 'test-control-token')
    kernelPluginsApi.list.mockResolvedValue([{ id: 'machine-telemetry', name: 'Machine Telemetry', official: true }])
    kernelPluginsApi.releases.mockResolvedValue([
      { id: 2, plugin_id: 'machine-telemetry', version: '2.0.0' },
      { id: 1, plugin_id: 'machine-telemetry', version: '1.0.0' }
    ])
    kernelPluginsApi.installations.mockResolvedValue([{
      id: 7,
      plugin_id: 'machine-telemetry',
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      previous_version: '',
      state: 'disabled',
      enabled: false,
      config_revision: 2
    }])
    kernelPluginsApi.extensions.mockResolvedValue([])
    kernelPluginsApi.operations.mockResolvedValue([])
    kernelPluginsApi.action.mockResolvedValue({
      installation: { id: 7, plugin_id: 'machine-telemetry', target: 'control', state: 'pending' },
      operation: { id: 'op-7', kind: 'plugin.enable', state: 'pending' },
      operation_chain: 'op-7'
    })
    kernelPluginsApi.getConfig.mockResolvedValue({ installation_id: 7, revision: 2, config: '{"enabled":true}' })
    kernelPluginsApi.updateConfig.mockResolvedValue({ installation_id: 7, revision: 3, config: '{"enabled":false}' })
  })

  it('renders signed plugin and target lifecycle state', async () => {
    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Machine Telemetry')
    expect(wrapper.text()).toContain('control')
    expect(wrapper.text()).toContain('disabled')
    expect(wrapper.text()).toContain('Install agent plugin')
  })

  it('renders health and a failure summary for a failed installation', async () => {
    kernelPluginsApi.installations.mockResolvedValueOnce([{
      id: 7,
      plugin_id: 'machine-telemetry',
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      previous_version: '',
      state: 'failed',
      enabled: true,
      has_error: true,
      last_error: 'plugin installation failed',
      config_revision: 2
    }])

    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.text()).toContain('health unhealthy')
    expect(wrapper.text()).toContain('Failure: plugin installation failed')
  })

  it('renders recent operation history with target and version', async () => {
    kernelPluginsApi.operations.mockResolvedValueOnce([
      { id: 'op-enable-7', kind: 'plugin.enable', state: 'succeeded', node_id: null, target_version: '1.0.0' },
      { id: 'op-agent-8', kind: 'plugin.update', state: 'failed', node_id: 8, target_version: '2.0.0' }
    ])

    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    const table = wrapper.get('table[aria-label="Recent plugin operations"]')
    expect(table.text()).toContain('op-enable-7')
    expect(table.text()).toContain('plugin.enable')
    expect(table.text()).toContain('control')
    expect(table.text()).toContain('agent')
    expect(table.text()).toContain('2.0.0')
    expect(table.find('[aria-live="polite"]').attributes('aria-label')).toBe('Operation state: succeeded')
  })

  it('does not offer installation for an unverified catalog entry', async () => {
    kernelPluginsApi.list.mockResolvedValueOnce([{ id: 'unverified', name: 'Unverified', official: false }])
    kernelPluginsApi.releases.mockResolvedValueOnce([{ id: 3, plugin_id: 'unverified', version: '1.0.0' }])
    kernelPluginsApi.installations.mockResolvedValueOnce([])

    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Official release required before installation')
    expect(wrapper.findAll('button').some((button) => button.text().includes('Install'))).toBe(false)
  })

  it('dispatches Control enable with an idempotency key and shows operation state', async () => {
    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    const enable = wrapper.findAll('button').find((button) => button.text() === 'Enable')
    await enable.trigger('click')
    await flushPromises()

    expect(kernelPluginsApi.action).toHaveBeenCalledWith(7, 'enable', expect.objectContaining({
      idempotencyKey: expect.stringMatching(/^control-center:enable:7:/)
    }))
    expect(wrapper.text()).toContain('op-7')
  })

  it('loads revisioned configuration in the dialog and saves it', async () => {
    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    const configure = wrapper.findAll('button').find((button) => button.text() === 'Configure')
    await configure.trigger('click')
    await flushPromises()
    expect(kernelPluginsApi.getConfig).toHaveBeenCalledWith(7)
    expect(wrapper.find('[role="dialog"]').text()).toContain('revision 2')

    await wrapper.find('[role="dialog"] textarea').setValue('{"enabled":false}')
    await wrapper.find('[role="dialog"] button.bg-primary-600').trigger('click')
    await flushPromises()
    expect(kernelPluginsApi.updateConfig).toHaveBeenCalledWith(7, { enabled: false }, 2)
  })

  it('dispatches a versioned Control update', async () => {
    const wrapper = mount(Plugins, { global: { plugins: [pinia] } })
    await flushPromises()

    const update = wrapper.findAll('button').find((button) => button.text() === 'Update to 2.0.0')
    await update.trigger('click')
    await flushPromises()

    expect(kernelPluginsApi.action).toHaveBeenCalledWith(7, 'update', expect.objectContaining({
      targetVersion: '2.0.0',
      idempotencyKey: expect.stringMatching(/^control-center:update:7:/)
    }))
  })
})
