import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MFA from '@/views/admin/MFA.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  getMFAConfig: vi.fn(),
  updateMFAConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin MFA', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.spyOn(window, 'alert').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getMFAConfig.mockResolvedValue({
      data: {
        enabled: true,
        required: false,
        methods: { totp: true, sms: false, email: true },
        backup_codes_count: 8,
        max_attempts: 4,
        lockout_duration: 20
      }
    })
    adminApi.updateMFAConfig.mockResolvedValue({})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads config from legacy and panel envelope payloads', async () => {
    const wrapper = mount(MFA)
    await flushPromises()

    expect(wrapper.vm.config.enabled).toBe(true)
    expect(wrapper.vm.config.required).toBe(false)
    expect(wrapper.vm.config.methods).toEqual({ totp: true, sms: false, email: true })
    expect(wrapper.vm.config.backup_codes_count).toBe(8)
    expect(wrapper.vm.config.max_attempts).toBe(4)
    expect(wrapper.vm.config.lockout_duration).toBe(20)

    adminApi.getMFAConfig.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        enabled: false,
        required: true,
        methods: { totp: true, sms: true, email: false },
        backup_codes_count: 12,
        max_attempts: 6,
        lockout_duration: 30
      },
      ts: 1783526400000
    })

    await wrapper.vm.fetchConfig()
    await flushPromises()

    expect(wrapper.vm.config.enabled).toBe(false)
    expect(wrapper.vm.config.required).toBe(true)
    expect(wrapper.vm.config.methods).toEqual({ totp: true, sms: true, email: false })
    expect(wrapper.vm.config.backup_codes_count).toBe(12)
    expect(wrapper.vm.config.max_attempts).toBe(6)
    expect(wrapper.vm.config.lockout_duration).toBe(30)

    wrapper.unmount()
  })

  it('saves the current MFA config through the admin API', async () => {
    const wrapper = mount(MFA)
    await flushPromises()

    wrapper.vm.config.required = true
    wrapper.vm.config.methods.sms = true
    wrapper.vm.config.backup_codes_count = 14

    await wrapper.vm.saveConfig()
    await flushPromises()

    expect(adminApi.updateMFAConfig).toHaveBeenCalledWith({
      enabled: true,
      required: true,
      methods: { totp: true, sms: true, email: true },
      backup_codes_count: 14,
      max_attempts: 4,
      lockout_duration: 20
    })
    expect(window.alert).toHaveBeenCalledWith('Saved successfully')

    wrapper.unmount()
  })

  it('shows panel envelope save errors', async () => {
    adminApi.updateMFAConfig.mockResolvedValueOnce({
      code: -1,
      msg: 'invalid config',
      ts: 1783612800000,
      data: null
    })

    const wrapper = mount(MFA)
    await flushPromises()

    await wrapper.vm.saveConfig()
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith('Save failed: invalid config')
    expect(window.alert).not.toHaveBeenCalledWith('Saved successfully')

    wrapper.unmount()
  })
})
