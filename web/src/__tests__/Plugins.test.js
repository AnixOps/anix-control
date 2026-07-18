import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Plugins from '@/views/admin/Plugins.vue'

const kernelApi = vi.hoisted(() => ({
  getKernelPlugins: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelOperations: vi.fn(),
  getKernelTopologies: vi.fn(),
  getKernelDeployments: vi.fn(),
  getKernelNodeAssignments: vi.fn(),
  getKernelScopes: vi.fn(),
  getKernelInstallationConfig: vi.fn(),
  updateKernelInstallationConfig: vi.fn(),
  upsertKernelInstallation: vi.fn(),
  runKernelInstallationAction: vi.fn(),
  registerKernelPluginRelease: vi.fn(),
  uploadKernelPluginReleaseArtifact: vi.fn(),
}))

const adminApi = vi.hoisted(() => ({ getNodes: vi.fn() }))

const extensionRuntime = vi.hoisted(() => ({
  refreshAdminExtensions: vi.fn(async () => ({ errors: [] })),
}))

const router = vi.hoisted(() => ({
  addRoute: vi.fn(),
  getRoutes: vi.fn(() => []),
}))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/api/admin', () => adminApi)
vi.mock('@/extensions/runtime', () => ({
  refreshAdminExtensions: extensionRuntime.refreshAdminExtensions,
}))
vi.mock('vue-router', () => ({ useRouter: () => router }))

const MANIFEST_V1 = JSON.stringify({
  id: 'protocol-runtime',
  version: '1.0.0',
  targets: ['control', 'agent'],
  config_schema: {
    type: 'object',
    properties: {
      port: { type: 'integer', title: 'Port' },
      enabled: { type: 'boolean', title: 'Enabled' },
    },
    required: ['port'],
  },
})
const MANIFEST_V2 = JSON.stringify({
  id: 'protocol-runtime',
  version: '1.1.0',
  targets: ['control', 'agent'],
  config_schema: { type: 'object' },
})

const plugin = {
  id: 'protocol-runtime',
  name: 'Protocol Runtime',
  description: 'Signed runtime package',
  publisher: 'AnixOps',
}

const releases = [
  { id: 2, plugin_id: plugin.id, version: '1.1.0', manifest: MANIFEST_V2 },
  { id: 1, plugin_id: plugin.id, version: '1.0.0', manifest: MANIFEST_V1 },
]

const mounted = []

function deferred() {
  let resolve
  let reject
  const promise = new Promise((nextResolve, nextReject) => {
    resolve = nextResolve
    reject = nextReject
  })
  return { promise, reject, resolve }
}

function resolveCatalog(installations = []) {
  kernelApi.getKernelPlugins.mockResolvedValue([plugin])
  kernelApi.getKernelPluginReleases.mockResolvedValue(releases)
  kernelApi.getKernelInstallations.mockResolvedValue(installations)
  kernelApi.getKernelOperations.mockResolvedValue([])
  kernelApi.getKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 3, config: '{"port":443,"enabled":true}' })
  kernelApi.updateKernelInstallationConfig.mockResolvedValue({ installation_id: 1, revision: 4, config: '{"port":8443,"enabled":true}' })
  kernelApi.upsertKernelInstallation.mockResolvedValue({ id: 1 })
  kernelApi.runKernelInstallationAction.mockResolvedValue({ operation: { id: 'plugin-operation-1', state: 'pending' } })
  kernelApi.registerKernelPluginRelease.mockResolvedValue({ id: 9, plugin_id: plugin.id, version: '1.2.0' })
  kernelApi.uploadKernelPluginReleaseArtifact.mockResolvedValue({ release_id: 9 })
}

function mountPlugins() {
  const wrapper = mount(Plugins, { attachTo: document.body })
  mounted.push(wrapper)
  return wrapper
}

async function openTarget(wrapper, target) {
  const row = wrapper.get('[data-testid="plugin-row-protocol-runtime"]')
  row.element.focus()
  await row.trigger('click')
  await flushPromises()
  await wrapper.get(`[data-target="${target}"]`).trigger('click')
  return wrapper.get('[data-testid="plugin-detail-drawer"]')
}

beforeEach(() => {
  vi.resetAllMocks()
  resolveCatalog()
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
})

describe('Plugin Center', () => {
  it('loads only plugin resources, renders one grouped row, filters it, and restores row focus after close', async () => {
    resolveCatalog([
      { id: 1, plugin_id: plugin.id, target: 'control', desired_version: '1.0.0', observed_version: '1.0.0', state: 'healthy', enabled: true },
      { id: 2, plugin_id: plugin.id, target: 'agent', desired_version: '1.1.0', observed_version: '1.1.0', state: 'healthy', enabled: true },
    ])
    const wrapper = mountPlugins()
    await flushPromises()

    expect(kernelApi.getKernelPlugins).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelPluginReleases).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelTopologies).not.toHaveBeenCalled()
    expect(kernelApi.getKernelDeployments).not.toHaveBeenCalled()
    expect(kernelApi.getKernelNodeAssignments).not.toHaveBeenCalled()
    expect(kernelApi.getKernelScopes).not.toHaveBeenCalled()
    expect(adminApi.getNodes).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(1)

    await wrapper.get('[data-testid="plugin-target-filter"]').setValue('agent')
    expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(1)
    await wrapper.get('[data-testid="plugin-health-filter"]').setValue('attention')
    expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(0)
    await wrapper.get('[data-testid="plugin-health-filter"]').setValue('healthy')
    expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(1)
    await wrapper.get('[data-testid="plugin-search"]').setValue('unmatched')
    expect(wrapper.findAll('[data-testid^="plugin-row-"]')).toHaveLength(0)
    await wrapper.get('[data-testid="plugin-search"]').setValue('protocol')

    const row = wrapper.get('[data-testid="plugin-row-protocol-runtime"]')
    row.element.focus()
    await row.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="plugin-detail-drawer"]').text()).toContain('agent')
    await wrapper.get('[data-testid="plugin-detail-close"]').trigger('click')
    await nextTick()
    expect(document.activeElement).toBe(row.element)
  })

  it('installs a catalogued target and refreshes the catalog before extensions', async () => {
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="install"]').trigger('click')
    await wrapper.get('[data-action="save-installation"]').trigger('click')
    await flushPromises()

    expect(kernelApi.upsertKernelInstallation).toHaveBeenCalledWith({
      plugin_id: plugin.id,
      target: 'control',
      desired_version: '1.1.0',
      enabled: true,
    })
    expect(extensionRuntime.refreshAdminExtensions).toHaveBeenCalledWith(router)
    expect(kernelApi.getKernelInstallations.mock.invocationCallOrder.at(-1)).toBeLessThan(
      extensionRuntime.refreshAdminExtensions.mock.invocationCallOrder.at(-1)
    )
  })

  it('uses idempotent control lifecycle actions, including upgrade and rollback', async () => {
    const installation = {
      id: 1,
      plugin_id: plugin.id,
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      previous_version: '0.9.0',
      state: 'healthy',
      enabled: false,
    }
    resolveCatalog([installation])
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="enable"]').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenLastCalledWith(1, 'enable', expect.objectContaining({
      idempotencyKey: expect.stringContaining('webui:1:enable:'),
    }))

    await drawer.get('[data-action="upgrade"]').trigger('click')
    await wrapper.get('#plugin-install-version').setValue('1.1.0')
    await wrapper.get('[data-action="save-installation"]').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenLastCalledWith(1, 'update', expect.objectContaining({
      targetVersion: '1.1.0',
      idempotencyKey: expect.stringContaining('webui:1:update:'),
    }))

    await drawer.get('[data-action="rollback"]').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelInstallationAction).toHaveBeenLastCalledWith(1, 'rollback', expect.objectContaining({
      idempotencyKey: expect.stringContaining('webui:1:rollback:'),
    }))
  })

  it('upserts agent lifecycle state without dispatching a control operation', async () => {
    const installation = {
      id: 2,
      plugin_id: plugin.id,
      target: 'agent',
      desired_version: '1.1.0',
      observed_version: '1.1.0',
      previous_version: '1.0.0',
      state: 'healthy',
      enabled: true,
    }
    resolveCatalog([installation])
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'agent')
    await drawer.get('[data-action="disable"]').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelInstallation).toHaveBeenLastCalledWith({
      plugin_id: plugin.id,
      target: 'agent',
      desired_version: '1.1.0',
      enabled: false,
    })
    expect(kernelApi.runKernelInstallationAction).not.toHaveBeenCalled()

    await drawer.get('[data-action="rollback"]').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelInstallation).toHaveBeenLastCalledWith({
      plugin_id: plugin.id,
      target: 'agent',
      desired_version: '1.0.0',
      enabled: true,
    })
  })

  it('saves schema-backed configuration with its optimistic revision', async () => {
    resolveCatalog([{
      id: 1,
      plugin_id: plugin.id,
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      config_revision: 3,
      state: 'healthy',
      enabled: true,
    }])
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="configure"]').trigger('click')
    await flushPromises()
    await wrapper.get('#plugin-config-port').setValue('8443')
    await wrapper.get('[data-testid="save-plugin-config"]').trigger('click')
    await flushPromises()

    expect(kernelApi.updateKernelInstallationConfig).toHaveBeenCalledWith(1, { port: 8443, enabled: true }, 3)
    expect(extensionRuntime.refreshAdminExtensions).toHaveBeenCalledWith(router)
  })

  it('keeps the detail drawer and edited config draft open after a config conflict', async () => {
    resolveCatalog([{
      id: 1,
      plugin_id: plugin.id,
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      config_revision: 3,
      state: 'healthy',
      enabled: true,
    }])
    kernelApi.updateKernelInstallationConfig.mockRejectedValue({
      response: { data: { error: { message: 'configuration revision conflict' } } },
    })
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="configure"]').trigger('click')
    await flushPromises()
    const portInput = wrapper.get('#plugin-config-port')
    await portInput.setValue('8443')
    await wrapper.get('[data-testid="save-plugin-config"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="plugin-detail-drawer"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="plugin-config-dialog"]').text()).toContain('configuration revision conflict')
    expect(wrapper.get('#plugin-config-port').element.value).toBe('8443')
  })

  it('ignores a late configuration response after the editor is reopened for another target', async () => {
    const controlConfig = deferred()
    const agentConfig = deferred()
    resolveCatalog([
      {
        id: 1,
        plugin_id: plugin.id,
        target: 'control',
        desired_version: '1.0.0',
        observed_version: '1.0.0',
        config_revision: 3,
        state: 'healthy',
        enabled: true,
      },
      {
        id: 2,
        plugin_id: plugin.id,
        target: 'agent',
        desired_version: '1.0.0',
        observed_version: '1.0.0',
        config_revision: 7,
        state: 'healthy',
        enabled: true,
      },
    ])
    kernelApi.getKernelInstallationConfig
      .mockImplementationOnce(() => controlConfig.promise)
      .mockImplementationOnce(() => agentConfig.promise)
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="configure"]').trigger('click')
    await nextTick()
    expect(kernelApi.getKernelInstallationConfig).toHaveBeenCalledWith(1)
    await wrapper.get('[data-testid="plugin-config-dialog"]').find('button').trigger('click')
    await wrapper.get('[data-target="agent"]').trigger('click')
    await drawer.get('[data-action="configure"]').trigger('click')
    await nextTick()
    expect(kernelApi.getKernelInstallationConfig).toHaveBeenCalledWith(2)

    agentConfig.resolve({ installation_id: 2, revision: 7, config: '{"port":8443,"enabled":true}' })
    await flushPromises()
    expect(wrapper.get('#plugin-config-port').element.value).toBe('8443')

    controlConfig.resolve({ installation_id: 1, revision: 3, config: '{"port":443,"enabled":false}' })
    await flushPromises()
    expect(wrapper.get('#plugin-config-port').element.value).toBe('8443')
    await wrapper.get('[data-testid="save-plugin-config"]').trigger('click')
    await flushPromises()
    expect(kernelApi.updateKernelInstallationConfig).toHaveBeenCalledWith(2, { port: 8443, enabled: true }, 7)
  })

  it('does not report installation success or refresh extensions when catalog refresh fails', async () => {
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="install"]').trigger('click')
    kernelApi.getKernelPlugins.mockRejectedValueOnce(new Error('catalog refresh failed'))
    await wrapper.get('[data-action="save-installation"]').trigger('click')
    await flushPromises()

    expect(extensionRuntime.refreshAdminExtensions).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="plugin-installation-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="plugin-installation-dialog"]').text()).toContain('catalog refresh failed')
    expect(wrapper.find('.notice-message').exists()).toBe(false)
  })

  it('refreshes operation state after an agent mutation and polls newly active plugin work', async () => {
    vi.useFakeTimers()
    resolveCatalog([{
      id: 2,
      plugin_id: plugin.id,
      target: 'agent',
      desired_version: '1.1.0',
      observed_version: '1.1.0',
      state: 'healthy',
      enabled: true,
    }])
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ id: 'agent-op-1', kind: 'plugin.update', plugin_id: plugin.id, state: 'running' }])
      .mockResolvedValueOnce([{ id: 'agent-op-1', kind: 'plugin.update', plugin_id: plugin.id, state: 'completed' }])
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'agent')
    await drawer.get('[data-action="disable"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(3)
  })

  it('removes a tracked operation that is absent from a refresh instead of polling forever', async () => {
    vi.useFakeTimers()
    resolveCatalog([{
      id: 1,
      plugin_id: plugin.id,
      target: 'control',
      desired_version: '1.0.0',
      observed_version: '1.0.0',
      state: 'healthy',
      enabled: false,
    }])
    kernelApi.getKernelOperations.mockResolvedValue([])
    const wrapper = mountPlugins()
    await flushPromises()

    const drawer = await openTarget(wrapper, 'control')
    await drawer.get('[data-action="enable"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
  })

  it('imports a release artifact and refreshes the catalog before extensions', async () => {
    const wrapper = mountPlugins()
    await flushPromises()

    await wrapper.get('[data-testid="import-plugin-release"]').trigger('click')
    await wrapper.get('#plugin-release-manifest').setValue('{"id":"protocol-runtime","version":"1.2.0"}')
    await wrapper.get('#plugin-release-signature').setValue('signed-release')
    const artifact = {
      name: 'runtime.tar.gz',
      size: 3,
      arrayBuffer: vi.fn(async () => new Uint8Array([1, 2, 3]).buffer),
    }
    const artifactInput = wrapper.get('#plugin-release-artifact')
    Object.defineProperty(artifactInput.element, 'files', { configurable: true, value: [artifact] })
    await artifactInput.trigger('change')
    await flushPromises()
    await wrapper.get('[data-action="save-release"]').trigger('click')
    await flushPromises()

    expect(kernelApi.registerKernelPluginRelease).toHaveBeenCalledWith('{"id":"protocol-runtime","version":"1.2.0"}', 'signed-release')
    expect(kernelApi.uploadKernelPluginReleaseArtifact).toHaveBeenCalledWith(9, 'AQID')
    expect(kernelApi.getKernelPlugins.mock.invocationCallOrder.at(-1)).toBeLessThan(
      extensionRuntime.refreshAdminExtensions.mock.invocationCallOrder.at(-1)
    )
  })

  it('retries a failed artifact upload against the release that was already registered', async () => {
    kernelApi.uploadKernelPluginReleaseArtifact
      .mockRejectedValueOnce(new Error('artifact upload failed'))
      .mockResolvedValueOnce({ release_id: 9 })
    const wrapper = mountPlugins()
    await flushPromises()

    await wrapper.get('[data-testid="import-plugin-release"]').trigger('click')
    await wrapper.get('#plugin-release-manifest').setValue('{"id":"protocol-runtime","version":"1.2.0"}')
    await wrapper.get('#plugin-release-signature').setValue('signed-release')
    const artifact = {
      name: 'runtime.tar.gz',
      size: 3,
      arrayBuffer: vi.fn(async () => new Uint8Array([1, 2, 3]).buffer),
    }
    const artifactInput = wrapper.get('#plugin-release-artifact')
    Object.defineProperty(artifactInput.element, 'files', { configurable: true, value: [artifact] })
    await artifactInput.trigger('change')
    await flushPromises()

    await wrapper.get('[data-action="save-release"]').trigger('click')
    await flushPromises()
    expect(kernelApi.registerKernelPluginRelease).toHaveBeenCalledTimes(1)
    expect(kernelApi.uploadKernelPluginReleaseArtifact).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="plugin-release-import-dialog"]').text()).toContain('artifact upload failed')

    await wrapper.get('[data-action="save-release"]').trigger('click')
    await flushPromises()
    expect(kernelApi.registerKernelPluginRelease).toHaveBeenCalledTimes(1)
    expect(kernelApi.uploadKernelPluginReleaseArtifact).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="plugin-release-import-dialog"]').exists()).toBe(false)
  })

  it('polls only active plugin operations, stops at terminal state, and clears its timer on unmount', async () => {
    vi.useFakeTimers()
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'plugin.update', plugin_id: plugin.id, state: 'running' }])
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'plugin.update', plugin_id: plugin.id, state: 'completed' }])
    const wrapper = mountPlugins()
    await flushPromises()

    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(4000)
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
  })
})
