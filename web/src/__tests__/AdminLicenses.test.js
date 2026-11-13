// 安全 → 设备授权: ANX device licensing settings (off by default) and the
// devices that asked for a license, with revoke behind a confirmation.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import Licenses from '@/views/admin/security/Licenses.vue'
import { setLocale } from '@/i18n'
import { answerConfirms } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({
  getKernelEntitlement: vi.fn(),
  setKernelEntitlement: vi.fn(),
  listKernelEntitlementDevices: vi.fn(),
  setKernelEntitlementDeviceRevoked: vi.fn()
}))
const adminApi = vi.hoisted(() => ({ getPlans: vi.fn() }))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/api/admin', () => adminApi)

enableAutoUnmount(afterEach)

const OFF = {
  feature: 'anx', enabled: false, require: false, trial_days: 3, plan_ids: [], ca: '', root_key: false,
  devices: { total: 0, seated: 0, revoked: 0 }, validity_days: 7, seat_window_days: 30, max_trial_days: 30
}
const CA = 'c83c22218d9d871e9eb6196777b1ee9715a34420d42646c09e6bbedf3687de26'
const DEVICE = {
  id: 11, user_id: 7, email: 'alice@example.com', device_id: '0101010101010101', alg: 'ed25519', trial: true,
  not_after: 1800604800, revoked: false, revoked_at: null, created_at: 1800000000, last_seen_at: 1800000000, seated: true
}

async function mountPage() {
  const wrapper = mount(Licenses, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

describe('Admin device licenses', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
    adminApi.getPlans.mockResolvedValue({ data: [{ id: 3, name: 'Pro' }, { id: 4, name: 'Lite' }] })
    kernelApi.getKernelEntitlement.mockResolvedValue(OFF)
    kernelApi.listKernelEntitlementDevices.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
  })

  it('is off by default and says when the root key is missing', async () => {
    const wrapper = await mountPage()
    expect(wrapper.get('[data-testid="licenses-enabled"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.find('[data-testid="licenses-no-root-key"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="licenses-ca"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('No devices yet')
    expect(wrapper.text()).toContain('Pro')
  })

  it('turns licensing on and saves the trial and plans', async () => {
    kernelApi.setKernelEntitlement.mockResolvedValueOnce({ ...OFF, enabled: true, ca: CA, root_key: true })
    const wrapper = await mountPage()
    await wrapper.get('[data-testid="licenses-enabled"]').trigger('click')
    await flushPromises()
    expect(kernelApi.setKernelEntitlement).toHaveBeenCalledWith({ enabled: true })
    expect(wrapper.get('[data-testid="licenses-ca"]').text()).toContain('License CA')

    kernelApi.setKernelEntitlement.mockResolvedValueOnce({ ...OFF, enabled: true, ca: CA, root_key: true, plan_ids: [3] })
    const save = wrapper.get('[data-testid="licenses-save"]')
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="licenses-plans"] [role="checkbox"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="licenses-save"]').trigger('click')
    await flushPromises()
    expect(kernelApi.setKernelEntitlement).toHaveBeenLastCalledWith({ trial_days: 3, plan_ids: [3] })
  })

  it('lists devices and revokes one after confirming', async () => {
    kernelApi.listKernelEntitlementDevices.mockResolvedValue({ items: [DEVICE], total: 1, page: 1, page_size: 20 })
    kernelApi.setKernelEntitlementDeviceRevoked.mockResolvedValue({ device: { ...DEVICE, revoked: true, seated: false }, changed: true })
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('0101010101010101')
    expect(wrapper.text()).toContain('Taking a seat')
    expect(wrapper.text()).toContain('Trial')
    const confirms = answerConfirms(true)
    await wrapper.vm.$.setupState.revoke(DEVICE)
    await flushPromises()
    expect(confirms.last()).toMatchObject({ tone: 'danger' })
    expect(kernelApi.setKernelEntitlementDeviceRevoked).toHaveBeenCalledWith(11, true)
    expect(kernelApi.listKernelEntitlementDevices).toHaveBeenCalledTimes(2)
  })
})
