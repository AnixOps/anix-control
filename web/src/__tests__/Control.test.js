import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Control from '@/views/admin/Control.vue'

const kernelApi = vi.hoisted(() => ({
  deleteKernelNodeAssignment: vi.fn(),
  getKernelPlugins: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelNodeAssignments: vi.fn(),
  getKernelScopes: vi.fn(),
  getKernelTopologies: vi.fn(),
  getKernelOperations: vi.fn(),
  getKernelInstallationConfig: vi.fn(),
  updateKernelInstallationConfig: vi.fn(),
  upsertKernelInstallation: vi.fn(),
  runKernelInstallationAction: vi.fn(),
  registerKernelPluginRelease: vi.fn(),
  uploadKernelPluginReleaseArtifact: vi.fn(),
  cancelKernelOperation: vi.fn(),
  upsertKernelNodeAssignment: vi.fn(),
}))

const adminApi = vi.hoisted(() => ({ getNodes: vi.fn() }))

const extensionRuntime = vi.hoisted(() => ({
  errors: [],
  extensions: { value: [] },
  refreshAdminExtensions: vi.fn(async () => ({ errors: [] })),
}))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/api/admin', () => adminApi)
vi.mock('@/extensions/runtime', () => ({
  adminExtensionErrors: extensionRuntime.errors,
  adminExtensions: extensionRuntime.extensions,
  refreshAdminExtensions: extensionRuntime.refreshAdminExtensions,
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ addRoute: vi.fn(), getRoutes: vi.fn(() => []) }) }))

const MANIFEST_V1 = JSON.stringify({
  id: 'protocol-runtime',
  version: '1.0.0',
  targets: ['control', 'agent'],
  config_schema: {
    type: 'object',
    properties: { port: { type: 'integer', title: 'Port' }, enabled: { type: 'boolean', title: 'Enabled' } },
    required: ['port'],
  },
})
const MANIFEST_V2 = JSON.stringify({ id: 'protocol-runtime', version: '1.1.0', targets: ['control', 'agent'], config_schema: { type: 'object' } })

function resolveEmptyState() {
  kernelApi.getKernelPlugins.mockResolvedValue([])
  kernelApi.getKernelPluginReleases.mockResolvedValue([])
  kernelApi.getKernelInstallations.mockResolvedValue([])
  kernelApi.getKernelScopes.mockResolvedValue([])
  kernelApi.getKernelTopologies.mockResolvedValue([])
  kernelApi.getKernelOperations.mockResolvedValue([])
  kernelApi.getKernelNodeAssignments.mockResolvedValue([])
  kernelApi.getKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 0, config: '{}' })
  kernelApi.updateKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 1, config: '{}' })
  kernelApi.upsertKernelInstallation.mockResolvedValue({ id: 1 })
  kernelApi.runKernelInstallationAction.mockResolvedValue({ operation: { id: 'op-action', state: 'pending' } })
  kernelApi.registerKernelPluginRelease.mockResolvedValue({ id: 9, plugin_id: 'protocol-runtime', version: '1.1.0' })
  kernelApi.uploadKernelPluginReleaseArtifact.mockResolvedValue({ release_id: 9 })
  kernelApi.cancelKernelOperation.mockResolvedValue({ id: 'op-1', state: 'cancel_requested' })
  kernelApi.upsertKernelNodeAssignment.mockResolvedValue({ id: 1 })
  kernelApi.deleteKernelNodeAssignment.mockResolvedValue(undefined)
  adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [] } })
  extensionRuntime.errors.splice(0)
  extensionRuntime.extensions.value = []
}

function resolvePluginState(installations = []) {
  kernelApi.getKernelPlugins.mockResolvedValue([{ id: 'protocol-runtime', name: 'Protocol Runtime', publisher: 'AnixOps' }])
  kernelApi.getKernelPluginReleases.mockResolvedValue([
    { id: 2, plugin_id: 'protocol-runtime', version: '1.1.0', manifest: MANIFEST_V2 },
    { id: 1, plugin_id: 'protocol-runtime', version: '1.0.0', manifest: MANIFEST_V1 },
  ])
  kernelApi.getKernelInstallations.mockResolvedValue(installations)
}

function resolveAssignmentState(assignments = []) {
  const manifest = JSON.stringify({ id: 'gost-mesh', version: '1.0.0', targets: ['agent'], config_schema: { type: 'object' } })
  kernelApi.getKernelPlugins.mockResolvedValue([{ id: 'gost-mesh', name: 'GOST Mesh', publisher: 'AnixOps' }])
  kernelApi.getKernelPluginReleases.mockResolvedValue([{ id: 3, plugin_id: 'gost-mesh', version: '1.0.0', manifest }])
  kernelApi.getKernelInstallations.mockResolvedValue([{
    id: 5,
    plugin_id: 'gost-mesh',
    target: 'agent',
    desired_version: '1.0.0',
    observed_version: '1.0.0',
    config_revision: 6,
    state: 'healthy',
    enabled: true,
  }])
  kernelApi.getKernelScopes.mockResolvedValue([{ id: 'forward', name: 'Forward', plugin_id: 'gost-mesh' }])
  kernelApi.getKernelNodeAssignments.mockResolvedValue(assignments)
  adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [{ id: 11, name: 'Shanghai entry', host: '10.0.0.11' }] } })
}

describe('Control', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    resolveEmptyState()
  })

  it('loads all control resources and renders every plugin installation target', async () => {
    resolvePluginState([
      { id: 1, plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.0.0', observed_version: '1.0.0', state: 'healthy', enabled: true },
      { id: 2, plugin_id: 'protocol-runtime', target: 'agent', desired_version: '1.1.0', observed_version: '1.1.0', state: 'healthy', enabled: true },
    ])
    kernelApi.getKernelScopes.mockResolvedValue([{ id: 'proxy', name: 'Proxy', plugin_id: 'protocol-runtime' }])
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 1, name: 'Proxy ingress', service_scope: 'proxy', active_revision_id: 3 }])
    kernelApi.getKernelOperations.mockResolvedValue([{ id: 'op-1', kind: 'plugin.update', plugin_id: 'protocol-runtime', revision: 4, state: 'completed' }])

    const wrapper = mount(Control)
    await flushPromises()

    expect(kernelApi.getKernelPlugins).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelPluginReleases).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodes).toHaveBeenCalledWith({ page: 1, page_size: 200 })
    expect(wrapper.findAll('#control-panel-plugins tbody tr')).toHaveLength(2)
    expect(wrapper.find('#control-panel-plugins').text()).toContain('control')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('agent')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('1.0.0')
    expect(wrapper.find('#control-panel-plugins').text()).toContain('1.1.0')
    expect(wrapper.attributes('aria-busy')).toBe('false')
    wrapper.unmount()
  })

  it('creates an Agent assignment with installation version and config revision defaults', async () => {
    resolveAssignmentState()
    const wrapper = mount(Control)
    await flushPromises()

    await wrapper.get('#control-tab-assignments').trigger('click')
    expect(kernelApi.getKernelNodeAssignments).toHaveBeenCalledWith(11)
    await wrapper.get('#new-assignment').trigger('click')
    expect(wrapper.get('#assignment-node').element.value).toBe('11')
    expect(wrapper.get('#assignment-plugin').element.value).toBe('gost-mesh')
    expect(wrapper.get('#assignment-scope').element.value).toBe('forward')
    expect(wrapper.get('#assignment-role').element.value).toBe('relay')
    expect(wrapper.get('#assignment-version').element.value).toBe('1.0.0')
    expect(wrapper.get('#assignment-config-revision').element.value).toBe('6')

    await wrapper.get('#assignment-rollout-group').setValue('canary-a')
    await wrapper.get('[aria-labelledby="assignment-editor-title"] .modal-footer .btn-primary').trigger('click')
    await flushPromises()

    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenCalledWith(11, {
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      enabled: true,
      rollout_group: 'canary-a',
    })
    expect(kernelApi.getKernelOperations.mock.calls.length).toBeGreaterThan(1)
    expect(extensionRuntime.refreshAdminExtensions).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('edits, toggles, and deletes an assignment without changing its identity fields', async () => {
    const assignment = {
      id: 7,
      node_id: 11,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: 'canary-a',
      enabled: false,
    }
    resolveAssignmentState([assignment])
    const confirmSpy = vi.spyOn(globalThis, 'confirm').mockReturnValue(true)
    const wrapper = mount(Control)
    await flushPromises()
    await wrapper.get('#control-tab-assignments').trigger('click')

    await wrapper.findAll('#control-panel-assignments button').find(button => button.text() === 'Edit').trigger('click')
    expect(wrapper.get('#assignment-node').attributes('disabled')).toBeDefined()
    expect(wrapper.get('#assignment-plugin').attributes('disabled')).toBeDefined()
    expect(wrapper.get('#assignment-scope').attributes('disabled')).toBeDefined()
    expect(wrapper.get('#assignment-role').attributes('disabled')).toBeDefined()
    await wrapper.get('#assignment-config-revision').setValue('8')
    await wrapper.get('#assignment-rollout-group').setValue('canary-b')
    await wrapper.get('[aria-labelledby="assignment-editor-title"] .modal-footer .btn-primary').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenLastCalledWith(11, expect.objectContaining({
      service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay', desired_config_revision: 8, rollout_group: 'canary-b', enabled: false,
    }))

    await wrapper.findAll('#control-panel-assignments button').find(button => button.text() === 'Enable').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenLastCalledWith(11, expect.objectContaining({
      service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay', desired_version: '1.0.0', desired_config_revision: 6, rollout_group: 'canary-a', enabled: true,
    }))

    await wrapper.findAll('#control-panel-assignments button').find(button => button.text() === 'Delete').trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalled()
    expect(kernelApi.deleteKernelNodeAssignment).toHaveBeenCalledWith(11, 7)
    wrapper.unmount()
    confirmSpy.mockRestore()
  })

  it('shows assignment lifecycle errors returned by the kernel', async () => {
    resolveAssignmentState()
    kernelApi.upsertKernelNodeAssignment.mockRejectedValue({ response: { data: { error: { message: 'agent package artifact is missing' } } } })
    const wrapper = mount(Control)
    await flushPromises()
    await wrapper.get('#control-tab-assignments').trigger('click')
    await wrapper.get('#new-assignment').trigger('click')
    await wrapper.get('[aria-labelledby="assignment-editor-title"] .modal-footer .btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.get('.error-message').text()).toBe('agent package artifact is missing')
    wrapper.unmount()
  })

  it('installs a catalogued target through the installation upsert endpoint', async () => {
    resolvePluginState([])
    const wrapper = mount(Control)
    await flushPromises()

    const controlRow = wrapper.findAll('#control-panel-plugins tbody tr').find(row => row.text().includes('control'))
    await controlRow.get('button').trigger('click')
    await wrapper.get('#plugin-install-version').setValue('1.1.0')
    await wrapper.get('[aria-labelledby="plugin-installation-title"] .btn-primary').trigger('click')
    await flushPromises()

    expect(kernelApi.upsertKernelInstallation).toHaveBeenCalledWith({
      plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.1.0', enabled: true,
    })
    expect(extensionRuntime.refreshAdminExtensions).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('dispatches enable, disable, upgrade, and rollback with idempotency keys', async () => {
    const installation = { id: 1, plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.0.0', observed_version: '1.0.0', previous_version: '0.9.0', state: 'healthy', enabled: false }
    resolvePluginState([installation])
    const wrapper = mount(Control)
    await flushPromises()

    await wrapper.findAll('#control-panel-plugins button').find(button => button.text() === 'Enable').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenCalledWith(1, 'enable', expect.objectContaining({ idempotencyKey: expect.stringContaining('webui:1:enable:') }))

    installation.enabled = true
    kernelApi.getKernelInstallations.mockResolvedValue([installation])
    await wrapper.find('.page-header .btn:last-child').trigger('click')
    await flushPromises()
    await wrapper.findAll('#control-panel-plugins button').find(button => button.text() === 'Disable').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenCalledWith(1, 'disable', expect.objectContaining({ idempotencyKey: expect.any(String) }))

    await wrapper.findAll('#control-panel-plugins button').find(button => button.text() === 'Upgrade').trigger('click')
    await wrapper.get('#plugin-install-version').setValue('1.1.0')
    await wrapper.get('[aria-labelledby="plugin-installation-title"] .btn-primary').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenCalledWith(1, 'update', expect.objectContaining({ targetVersion: '1.1.0' }))

    await wrapper.findAll('#control-panel-plugins button').find(button => button.text() === 'Rollback').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenCalledWith(1, 'rollback', expect.objectContaining({ idempotencyKey: expect.any(String) }))
    wrapper.unmount()
  })

  it('loads schema-backed config and saves it with optimistic revision', async () => {
    resolvePluginState([{ id: 1, plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.0.0', observed_version: '1.0.0', config_revision: 3, state: 'healthy', enabled: true }])
    kernelApi.getKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 3, config: '{"port":443,"enabled":true}' })
    kernelApi.updateKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 4, config: '{"port":8443,"enabled":true}' })
    const wrapper = mount(Control)
    await flushPromises()

    await wrapper.findAll('#control-panel-plugins button').find(button => button.text() === 'Configure').trigger('click')
    await flushPromises()
    const portInput = wrapper.get('#plugin-config-port')
    expect(portInput.element.value).toBe('443')
    await portInput.setValue('8443')
    await wrapper.get('[aria-labelledby="plugin-config-title"] .modal-footer .btn-primary').trigger('click')
    await flushPromises()

    expect(kernelApi.updateKernelInstallationConfig).toHaveBeenCalledWith(1, { port: 8443, enabled: true }, 3)
    wrapper.unmount()
  })

  it('polls active plugin operations and exposes cancellation and extension errors', async () => {
    vi.useFakeTimers()
    extensionRuntime.errors.push({ plugin_id: 'protocol-runtime', message: 'bundle digest mismatch' })
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'plugin.update', plugin_id: 'protocol-runtime', state: 'running' }])
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'plugin.update', plugin_id: 'protocol-runtime', state: 'completed' }])
    const wrapper = mount(Control)
    await flushPromises()

    expect(wrapper.text()).toContain('bundle digest mismatch')
    await wrapper.get('#control-tab-operations').trigger('click')
    await wrapper.findAll('#control-panel-operations button').find(button => button.text() === 'Cancel operation').trigger('click')
    await flushPromises()
    expect(kernelApi.cancelKernelOperation).toHaveBeenCalledWith('op-1')

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations.mock.calls.length).toBeGreaterThan(1)
    expect(extensionRuntime.refreshAdminExtensions).toHaveBeenCalled()
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('links tabs to panels and supports arrow-key navigation', async () => {
    const wrapper = mount(Control)
    await flushPromises()

    const pluginTab = wrapper.get('#control-tab-plugins')
    expect(pluginTab.attributes('aria-controls')).toBe('control-panel-plugins')
    expect(wrapper.get('#control-panel-plugins').attributes('aria-labelledby')).toBe('control-tab-plugins')
    await pluginTab.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('#control-tab-assignments').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('#control-panel-assignments').isVisible()).toBe(true)
    await wrapper.get('#control-tab-assignments').trigger('keydown', { key: 'End' })
    expect(wrapper.get('#control-tab-operations').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
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
    wrapper.unmount()
  })
})
