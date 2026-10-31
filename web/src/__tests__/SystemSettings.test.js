// 系统设置 shell (UI U7, plan §7.3): sections in the URL, the section list,
// phones (list, then a section with a back link) and the unsaved-changes
// guard when leaving a section with an edited form.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import System from '@/views/admin/System.vue'
import { answerConfirms } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  deleteSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getSystemInfo: vi.fn(),
  getDestinationSettings: vi.fn(),
  updateDestinationSettings: vi.fn()
}))

const kernelApi = vi.hoisted(() => ({
  getSettingsCatalog: vi.fn(),
  getPanelBackups: vi.fn(),
  createPanelBackup: vi.fn(),
  downloadPanelBackup: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/api/kernel', async (importOriginal) => ({ ...(await importOriginal()), ...kernelApi }))

enableAutoUnmount(afterEach)

function stubViewport(narrow) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: narrow,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn()
  })))
}

async function mountAt(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/system/:section?', component: System },
      { path: '/admin/users', component: { template: '<div>users</div>' } }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('System settings page', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    stubViewport(false)
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [] } })
    adminApi.getSystemAuditLogs.mockResolvedValue({ data: { data: { list: [], total: 0, page: 1, page_size: 20 } } })
    kernelApi.getSettingsCatalog.mockResolvedValue({ items: [{ key: 'traffic.destinations.retention_days', kind: 'choice', section: 'data', choices: [7, 30, 90] }] })
    kernelApi.getPanelBackups.mockResolvedValue({ supported: true, backups: [], summary: {}, retention: {} })
    adminApi.getDestinationSettings.mockResolvedValue({ enabled: false, retention_days: 30 })
    adminApi.getSystemInfo.mockResolvedValue({ code: 0, data: { version: '4.1.0', build_code: '202610020001', commit: 'abc123', build_time: '2026-10-02' } })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the sections as links and shows 通用 at /admin/system', async () => {
    const { wrapper } = await mountAt('/admin/system')
    const links = wrapper.findAll('[data-settings-section]')
    expect(links.map(link => link.attributes('data-settings-section'))).toEqual(['general', 'data', 'backup', 'audit', 'packages', 'advanced', 'about'])
    expect(links.map(link => link.attributes('href'))).toEqual([
      '/admin/system/general', '/admin/system/data', '/admin/system/backup',
      '/admin/system/audit', '/admin/system/packages', '/admin/system/advanced', '/admin/system/about'
    ])
    expect(wrapper.get('[aria-current="page"]').attributes('data-settings-section')).toBe('general')
    expect(wrapper.find('[data-settings-panel="general"]').exists()).toBe(true)
    expect(wrapper.findAll('h1')).toHaveLength(1)
  })

  it('loads only the section in the URL and switches with the path', async () => {
    const { wrapper, router } = await mountAt('/admin/system/backup')
    expect(wrapper.find('[data-settings-panel="backup"]').exists()).toBe(true)
    expect(kernelApi.getPanelBackups).toHaveBeenCalledTimes(1)
    expect(adminApi.getSystemAuditLogs).not.toHaveBeenCalled()
    expect(adminApi.getSystemConfigs).not.toHaveBeenCalled()

    await router.push('/admin/system/audit')
    await flushPromises()
    expect(wrapper.find('[data-settings-panel="audit"]').exists()).toBe(true)
    expect(wrapper.get('[aria-current="page"]').attributes('data-settings-section')).toBe('audit')
    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledTimes(1)
  })

  it('shows version and build details in 关于, like the account menu', async () => {
    const { wrapper } = await mountAt('/admin/system/about')
    const rows = wrapper.findAll('[data-about-row]').map(row => row.attributes('data-about-row'))
    expect(rows).toEqual(['product', 'version', 'backend-build', 'commit'])
    expect(wrapper.text()).toContain('v4.1.0 #202610020001')
    expect(wrapper.text()).toContain('abc123')
  })

  it('saves the access statistics switch and retention from 数据与保留', async () => {
    adminApi.updateDestinationSettings.mockResolvedValue({ enabled: true, retention_days: 30 })
    const { wrapper } = await mountAt('/admin/system/data')
    expect(wrapper.find('[data-settings-panel="data"]').exists()).toBe(true)
    expect(adminApi.getDestinationSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="settings-save-bar"]').exists()).toBe(false)

    await wrapper.get('[data-test="data-enabled"]').trigger('click')
    await flushPromises()
    const bar = wrapper.get('[data-test="settings-save-bar"]')
    await bar.get('button[type="submit"], [data-test="settings-save"]').trigger('click')
    await flushPromises()
    expect(adminApi.updateDestinationSettings).toHaveBeenCalledWith({ enabled: true, retention_days: 30 })
  })

  it('keeps the raw configuration keys in 高级, with a warning', async () => {
    const { wrapper } = await mountAt('/admin/system/advanced')
    expect(wrapper.find('[data-settings-panel="advanced"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="advanced-warning"]').exists()).toBe(true)
    expect(adminApi.getSystemConfigs).toHaveBeenCalledTimes(1)
  })

  it('sends an unknown section back to the page', async () => {
    const { router } = await mountAt('/admin/system/nope')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/system')
  })

  it('asks before leaving a section with unsaved changes', async () => {
    const { wrapper, router } = await mountAt('/admin/system/general')
    await wrapper.get('#settings-subscription-domains').setValue('sub1.example.com\ncdn.example.net')
    await flushPromises()
    expect(wrapper.find('[data-test="settings-save-bar"]').exists()).toBe(true)

    const stay = answerConfirms(false)
    await router.push('/admin/system/audit')
    await flushPromises()
    expect(stay.last()).toMatchObject({ title: 'Discard unsaved changes?', confirmLabel: 'Discard changes', tone: 'danger' })
    expect(router.currentRoute.value.path).toBe('/admin/system/general')
    expect(wrapper.find('[data-settings-panel="general"]').exists()).toBe(true)
    stay.stop?.()

    answerConfirms(true)
    await router.push('/admin/users')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/users')
  })

  it('does not ask when nothing changed', async () => {
    const confirms = answerConfirms(false)
    const { router } = await mountAt('/admin/system/backup')
    await router.push('/admin/system/about')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/system/about')
    expect(confirms.calls).toHaveLength(0)
  })

  it('on phones shows the list first, then a section with a back link', async () => {
    stubViewport(true)
    const { wrapper, router } = await mountAt('/admin/system')
    expect(wrapper.find('[data-test="settings-nav"]').exists()).toBe(true)
    expect(wrapper.find('[data-settings-panel]').exists()).toBe(false)

    await router.push('/admin/system/audit')
    await flushPromises()
    expect(wrapper.find('[data-test="settings-nav"]').exists()).toBe(false)
    expect(wrapper.find('[data-settings-panel="audit"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="settings-back"]').attributes('href')).toBe('/admin/system')
  })
})
