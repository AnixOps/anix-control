// 安全 (UI U7): the MFA policy and the access groups as sections of one
// page on the settings template, the section in the path; the old pages
// redirect. The sections keep their own requests (AdminMFA, AccessGroups).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import Security from '@/views/admin/Security.vue'
import MFA from '@/views/admin/MFA.vue'
import { setLocale } from '@/i18n'
import { answerConfirms } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  getMFAConfig: vi.fn(),
  updateMFAConfig: vi.fn()
}))
const kernelApi = vi.hoisted(() => ({
  getKernelScopes: vi.fn(),
  getKernelAccessGroups: vi.fn(),
  listKernelApiTokens: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/api/kernel', () => kernelApi)

enableAutoUnmount(afterEach)

async function mountAt(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/security/:section?', component: Security },
      { path: '/admin/mfa', redirect: '/admin/security/mfa' },
      { path: '/admin/access-groups', redirect: '/admin/security/access-groups' },
      { path: '/admin/users', component: { template: '<div />' } }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return { wrapper, router }
}

describe('Admin security page', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    setActivePinia(createPinia())
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    await setLocale('en')
    adminApi.getMFAConfig.mockResolvedValue({
      data: { enabled: true, required: false, methods: { totp: true, sms: false, email: true }, backup_codes_count: 8, max_attempts: 4, lockout_duration: 20 }
    })
    adminApi.updateMFAConfig.mockResolvedValue({})
    kernelApi.getKernelScopes.mockResolvedValue([{ id: 'forward', name: 'Forward' }])
    kernelApi.getKernelAccessGroups.mockResolvedValue([{ id: 7, scope_id: 'forward', name: 'Canary operators', enabled: true }])
    kernelApi.listKernelApiTokens.mockResolvedValue([])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows the three sections and the MFA policy at /admin/security', async () => {
    const { wrapper } = await mountAt('/admin/security')
    const links = wrapper.findAll('[data-settings-section]')
    expect(links.map(link => link.text())).toEqual(['Two-factor authentication', 'Access groups', 'API tokens'])
    expect(links.map(link => link.attributes('href'))).toEqual(['/admin/security/mfa', '/admin/security/access-groups', '/admin/security/api-tokens'])
    expect(wrapper.find('[data-security-panel="mfa"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Recovery codes')
    expect(kernelApi.getKernelAccessGroups).not.toHaveBeenCalled()
    // The tokens section is a lazy chunk: nothing of it loads until it opens.
    expect(kernelApi.listKernelApiTokens).not.toHaveBeenCalled()
  })

  it('opens the API tokens section under the page heading, with one H1', async () => {
    const { wrapper } = await mountAt('/admin/security/api-tokens')
    await vi.waitFor(() => expect(wrapper.find('[data-security-panel="api-tokens"]').exists()).toBe(true))
    await flushPromises()
    expect(wrapper.findAll('h1').map(h => h.text())).toEqual(['Security'])
    expect(wrapper.find('h2').text()).toBe('API tokens')
    expect(wrapper.get('[data-settings-section="api-tokens"]').attributes('aria-current')).toBe('page')
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(1)
    expect(adminApi.getMFAConfig).not.toHaveBeenCalled()
    expect(kernelApi.getKernelAccessGroups).not.toHaveBeenCalled()
  })

  it('shows the access groups under the page heading, with one H1', async () => {
    const { wrapper } = await mountAt('/admin/security/access-groups')
    expect(wrapper.findAll('h1').map(h => h.text())).toEqual(['Security'])
    expect(wrapper.find('h2').text()).toBe('Access groups')
    expect(wrapper.text()).toContain('Canary operators')
    expect(adminApi.getMFAConfig).not.toHaveBeenCalled()
  })

  it('redirects the old pages to their sections', async () => {
    let { router } = await mountAt('/admin/mfa')
    expect(router.currentRoute.value.path).toBe('/admin/security/mfa');
    ({ router } = await mountAt('/admin/access-groups'))
    expect(router.currentRoute.value.path).toBe('/admin/security/access-groups')
  })

  it('asks before leaving an edited MFA policy', async () => {
    const { wrapper, router } = await mountAt('/admin/security/mfa')
    await wrapper.get('#mfa-required').trigger('click')
    await flushPromises()
    expect(document.querySelector('[data-test="settings-save-bar"]')).not.toBeNull()
    const confirms = answerConfirms(false)
    await router.push('/admin/security/access-groups')
    await flushPromises()
    expect(confirms.calls).toHaveLength(1)
    expect(router.currentRoute.value.path).toBe('/admin/security/mfa')
  })
})

describe('MFA policy validation', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
    adminApi.getMFAConfig.mockResolvedValue({ data: { enabled: true, methods: { totp: true, sms: false, email: false } } })
  })

  it('needs a method while MFA is on and numbers in range', async () => {
    const wrapper = mount(MFA)
    await flushPromises()
    wrapper.vm.config.methods.totp = false
    wrapper.vm.config.max_attempts = 11
    await flushPromises()
    expect(wrapper.vm.methodsError).toBe('With two-factor authentication on, keep at least one method.')
    expect(wrapper.vm.errors.max_attempts).toBe('Enter a whole number from 1 to 10')
    await wrapper.vm.saveConfig()
    expect(adminApi.updateMFAConfig).not.toHaveBeenCalled()
  })
})
