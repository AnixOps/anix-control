import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Control from '@/views/admin/Control.vue'

const kernelApi = vi.hoisted(() => ({
  getKernelPlugins: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelScopes: vi.fn(),
  getKernelTopologies: vi.fn(),
  getKernelOperations: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

function resolveEmptyState() {
  kernelApi.getKernelPlugins.mockResolvedValue([])
  kernelApi.getKernelInstallations.mockResolvedValue([])
  kernelApi.getKernelScopes.mockResolvedValue([])
  kernelApi.getKernelTopologies.mockResolvedValue([])
  kernelApi.getKernelOperations.mockResolvedValue([])
}

describe('Control', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    resolveEmptyState()
  })

  it('loads all control resources and renders every plugin installation target', async () => {
    kernelApi.getKernelPlugins.mockResolvedValue([
      { id: 'protocol-runtime', name: 'Protocol Runtime', publisher: 'AnixOps' }
    ])
    kernelApi.getKernelInstallations.mockResolvedValue([
      { id: 1, plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.2.0', observed_version: '1.1.0', state: 'pending' },
      { id: 2, plugin_id: 'protocol-runtime', target: 'agent', desired_version: '1.2.0', observed_version: '1.2.0', state: 'healthy' }
    ])
    kernelApi.getKernelScopes.mockResolvedValue([{ id: 'proxy', name: 'Proxy', plugin_id: 'protocol-runtime' }])
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 1, name: 'Proxy ingress', service_scope: 'proxy', active_revision_id: 3 }])
    kernelApi.getKernelOperations.mockResolvedValue([{ id: 'op-1', kind: 'plugin.update', plugin_id: 'protocol-runtime', revision: 4, state: 'completed' }])

    const wrapper = mount(Control)
    await flushPromises()

    expect(kernelApi.getKernelPlugins).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('#control-panel-plugins tbody tr')).toHaveLength(2)
    expect(wrapper.find('#control-panel-plugins').text()).toContain('control')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('agent')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('1.1.0')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('1.2.0')
    expect(wrapper.attributes('aria-busy')).toBe('false')
  })

  it('links tabs to panels and supports arrow-key navigation', async () => {
    const wrapper = mount(Control)
    await flushPromises()

    const pluginTab = wrapper.get('#control-tab-plugins')
    expect(pluginTab.attributes('aria-controls')).toBe('control-panel-plugins')
    expect(wrapper.get('#control-panel-plugins').attributes('aria-labelledby')).toBe('control-tab-plugins')

    await pluginTab.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('#control-tab-scopes').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('#control-panel-scopes').isVisible()).toBe(true)

    await wrapper.get('#control-tab-scopes').trigger('keydown', { key: 'End' })
    expect(wrapper.get('#control-tab-operations').attributes('aria-selected')).toBe('true')
  })

  it('shows backend error messages without flashing empty-state rows', async () => {
    let rejectRequest
    kernelApi.getKernelPlugins.mockReturnValue(new Promise((resolve, reject) => { rejectRequest = reject }))

    const wrapper = mount(Control)
    await nextTick()
    expect(wrapper.text()).toContain('Loading control state...')
    expect(wrapper.text()).not.toContain('No plugins')

    rejectRequest({ response: { data: { error: { message: 'kernel unavailable' } } } })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toBe('kernel unavailable')
    expect(wrapper.text()).not.toContain('No plugins')
  })
})
