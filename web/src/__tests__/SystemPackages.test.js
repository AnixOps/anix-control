// 系统设置 → 内置包: the status of the built-in packages (it replaced 插件中心).
// It lists the Control installations with their version, state and last
// error, restarts a host after a confirmation, and edits the settings of a
// package that declares them. Nothing imports, installs, upgrades, rolls back
// or disables a package.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SettingsPackages from '@/views/admin/system/SettingsPackages.vue'
import { answerConfirms, inBody, toastMessages } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({
  getKernelPlugins: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelInstallationConfig: vi.fn(),
  updateKernelInstallationConfig: vi.fn(),
  restartKernelInstallation: vi.fn()
}))
const runtime = vi.hoisted(() => ({
  refreshAdminExtensions: vi.fn(),
  adminExtensionErrors: []
}))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/extensions/runtime', () => runtime)

enableAutoUnmount(afterEach)

const PLUGINS = [
  { id: 'identity-platform', name: 'Identity Platform', publisher: 'AnixOps', official: true },
  { id: 'machine-telemetry', name: 'Machine Telemetry', publisher: 'AnixOps', official: true },
  { id: 'knowledge', name: 'Knowledge', publisher: 'AnixOps', official: true }
]
const SCHEMA = { type: 'object', properties: { interval_seconds: { type: 'integer', title: 'Interval' } } }
const RELEASES = [
  { id: 1, plugin_id: 'identity-platform', version: '4.2.1', manifest: JSON.stringify({ id: 'identity-platform', version: '4.2.1', targets: ['control'] }) },
  { id: 2, plugin_id: 'machine-telemetry', version: '4.2.1', manifest: JSON.stringify({ id: 'machine-telemetry', version: '4.2.1', targets: ['control'], config_schema: SCHEMA }) },
  { id: 3, plugin_id: 'knowledge', version: '4.3.0', manifest: JSON.stringify({ id: 'knowledge', version: '4.3.0', targets: ['control'] }) }
]
const INSTALLATIONS = [
  { id: 11, plugin_id: 'identity-platform', target: 'control', desired_version: '4.2.1', observed_version: '4.2.1', state: 'healthy', health: 'healthy', enabled: true },
  { id: 12, plugin_id: 'machine-telemetry', target: 'control', desired_version: '4.2.1', observed_version: '4.2.1', state: 'failed', health: 'unhealthy', enabled: true, has_error: true, last_error: 'plugin installation failed' },
  { id: 13, plugin_id: 'knowledge', target: 'control', desired_version: '4.3.0', observed_version: '4.2.1', state: 'pending', health: 'pending', enabled: true },
  // An Agent installation an earlier release left is not a built-in package.
  { id: 14, plugin_id: 'machine-telemetry', target: 'agent', desired_version: '4.0.0', state: 'healthy', health: 'healthy', enabled: true }
]

async function mountSection() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }] })
  await router.push('/admin/system/packages')
  await router.isReady()
  const wrapper = mount(SettingsPackages, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('built-in packages section', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    runtime.adminExtensionErrors.length = 0
    kernelApi.getKernelPlugins.mockResolvedValue(PLUGINS)
    kernelApi.getKernelPluginReleases.mockResolvedValue(RELEASES)
    kernelApi.getKernelInstallations.mockResolvedValue(INSTALLATIONS)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('lists the Control installations with their version, state and last error', async () => {
    const wrapper = await mountSection()
    const rows = wrapper.findAll('[data-package]')
    expect(rows.map(row => row.attributes('data-package'))).toEqual(['identity-platform', 'knowledge', 'machine-telemetry'])
    const identity = wrapper.get('[data-package="identity-platform"]')
    expect(identity.text()).toContain('Identity Platform')
    expect(identity.text()).toContain('Version 4.2.1')
    expect(identity.text()).toContain('Running')
    const telemetry = wrapper.get('[data-package="machine-telemetry"]')
    expect(telemetry.text()).toContain('Failed')
    expect(telemetry.text()).toContain('plugin installation failed')
    const knowledge = wrapper.get('[data-package="knowledge"]')
    expect(knowledge.text()).toContain('Version 4.2.1, starting 4.3.0')
    expect(telemetry.text()).toContain('Version 4.2.1')
    expect(knowledge.text()).toContain('Starting')
    // Only a package that declares settings can be configured.
    expect(telemetry.find('[data-action="configure"]').exists()).toBe(true)
    expect(identity.find('[data-action="configure"]').exists()).toBe(false)
    // No lifecycle control is left.
    for (const action of ['install', 'upgrade', 'rollback', 'enable', 'disable']) {
      expect(wrapper.find(`[data-action="${action}"]`).exists(), action).toBe(false)
    }
    expect(wrapper.text()).not.toContain('Import')
  })

  it('restarts a host after a confirmation and refreshes', async () => {
    const confirms = answerConfirms(true)
    kernelApi.restartKernelInstallation.mockResolvedValue({ operation: { id: 'op-1' } })
    const wrapper = await mountSection()
    await wrapper.get('[data-package="identity-platform"] [data-action="restart"]').trigger('click')
    await flushPromises()
    expect(confirms.last()).toMatchObject({ title: 'Restart Identity Platform?' })
    expect(kernelApi.restartKernelInstallation).toHaveBeenCalledWith(11, expect.stringMatching(/^webui:11:restart:/))
    expect(toastMessages('success')).toContain('Identity Platform is restarting')
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(2)
  })

  it('does nothing when the restart is cancelled', async () => {
    answerConfirms(false)
    const wrapper = await mountSection()
    await wrapper.get('[data-package="identity-platform"] [data-action="restart"]').trigger('click')
    await flushPromises()
    expect(kernelApi.restartKernelInstallation).not.toHaveBeenCalled()
  })

  it('keeps refreshing while a package is starting', async () => {
    await mountSection()
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(1)
    kernelApi.getKernelInstallations.mockResolvedValue(INSTALLATIONS.map(item => item.id === 13 ? { ...item, observed_version: '4.3.0', state: 'healthy', health: 'healthy' } : item))
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(10000)
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(2)
  })

  it('edits the settings a package declares, at the revision it read', async () => {
    kernelApi.getKernelInstallationConfig.mockResolvedValue({ config: '{"interval_seconds":30}', revision: 4 })
    kernelApi.updateKernelInstallationConfig.mockResolvedValue({ revision: 5 })
    const wrapper = await mountSection()
    await wrapper.get('[data-package="machine-telemetry"] [data-action="configure"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelInstallationConfig).toHaveBeenCalledWith(12)
    expect(inBody('[data-testid="package-config-dialog"]').exists()).toBe(true)
    await inBody('[data-testid="save-package-config"]').trigger('click')
    await flushPromises()
    expect(kernelApi.updateKernelInstallationConfig).toHaveBeenCalledWith(12, { interval_seconds: 30 }, 4)
    expect(runtime.refreshAdminExtensions).toHaveBeenCalled()
  })

  it('says when the build carries no package and when the list fails', async () => {
    kernelApi.getKernelInstallations.mockResolvedValue([])
    let wrapper = await mountSection()
    expect(wrapper.text()).toContain('No built-in packages')
    wrapper.unmount()

    kernelApi.getKernelInstallations.mockRejectedValue(new Error('kernel unavailable'))
    wrapper = await mountSection()
    expect(wrapper.text()).toContain('The built-in packages didn’t load')
  })
})
