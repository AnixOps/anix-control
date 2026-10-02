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
  getBackupConfig: vi.fn(),
  updateBackupConfig: vi.fn(),
  createBackup: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  deleteBackup: vi.fn(),
  restoreBackup: vi.fn(),
  getLoadBalancers: vi.fn(),
  createLoadBalancer: vi.fn(),
  updateLoadBalancer: vi.fn(),
  deleteLoadBalancer: vi.fn(),
  runHealthCheck: vi.fn(),
  listForwardRuntimeJobs: vi.fn(),
  getForwardRuntimeStatus: vi.fn(),
  runForwardRuntimeDoctor: vi.fn(),
  getSystemInfo: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

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
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardRuntimeStatus.mockResolvedValue({ data: { data: null } })
    adminApi.getSystemInfo.mockResolvedValue({ code: 0, data: { version: '4.1.0', build_code: '202610020001', commit: 'abc123', build_time: '2026-10-02' } })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the sections as links and shows 通用 at /admin/system', async () => {
    const { wrapper } = await mountAt('/admin/system')
    const links = wrapper.findAll('[data-settings-section]')
    expect(links.map(link => link.attributes('data-settings-section'))).toEqual(['general', 'runtime', 'backup', 'balancer', 'audit', 'about'])
    expect(links.map(link => link.attributes('href'))).toEqual([
      '/admin/system/general', '/admin/system/runtime', '/admin/system/backup',
      '/admin/system/balancer', '/admin/system/audit', '/admin/system/about'
    ])
    expect(wrapper.get('[aria-current="page"]').attributes('data-settings-section')).toBe('general')
    expect(wrapper.find('[data-settings-panel="general"]').exists()).toBe(true)
    expect(wrapper.findAll('h1')).toHaveLength(1)
  })

  it('loads only the section in the URL and switches with the path', async () => {
    const { wrapper, router } = await mountAt('/admin/system/backup')
    expect(wrapper.find('[data-settings-panel="backup"]').exists()).toBe(true)
    expect(adminApi.getBackupConfig).toHaveBeenCalledTimes(1)
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

  it('sends an unknown section back to the page', async () => {
    const { router } = await mountAt('/admin/system/nope')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/system')
  })

  it('asks before leaving a section with unsaved changes', async () => {
    const { wrapper, router } = await mountAt('/admin/system/backup')
    await wrapper.get('#backup-enabled').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="settings-save-bar"]').exists()).toBe(true)

    const stay = answerConfirms(false)
    await router.push('/admin/system/audit')
    await flushPromises()
    expect(stay.last()).toMatchObject({ title: 'Discard unsaved changes?', confirmLabel: 'Discard changes', tone: 'danger' })
    expect(router.currentRoute.value.path).toBe('/admin/system/backup')
    expect(wrapper.find('[data-settings-panel="backup"]').exists()).toBe(true)
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

    await router.push('/admin/system/balancer')
    await flushPromises()
    expect(wrapper.find('[data-test="settings-nav"]').exists()).toBe(false)
    expect(wrapper.find('[data-settings-panel="balancer"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="settings-back"]').attributes('href')).toBe('/admin/system')
  })
})
