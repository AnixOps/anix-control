import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Coupons from '@/views/admin/Coupons.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  createCoupon: vi.fn(),
  deleteCoupon: vi.fn(),
  getCoupons: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  default: adminApi,
  ...adminApi
}))

describe('Admin Coupons', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders coupons from legacy and panel envelope payloads', async () => {
    adminApi.getCoupons
      .mockResolvedValueOnce({
        data: [
          {
            id: 1,
            code: 'LEGACY10',
            name: 'Legacy Discount',
            type: 1,
            value: 10,
            use_count: 2,
            limit_use: 100,
            started_at: 1783526400,
            ended_at: 1786118400
          }
        ]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [
          {
            id: 2,
            code: 'PANEL500',
            name: 'Panel Fixed',
            type: 2,
            value: 500,
            use_count: 1,
            limit_use: -1,
            started_at: 1783526400,
            ended_at: 1786118400
          }
        ],
        ts: 1783526400000
      })

    const wrapper = mount(Coupons)
    await flushPromises()

    expect(wrapper.text()).toContain('LEGACY10')
    expect(wrapper.text()).toContain('Legacy Discount')
    expect(wrapper.text()).toContain('10%')
    expect(wrapper.text()).toContain('2 / 100')

    await wrapper.vm.load()
    await flushPromises()

    expect(wrapper.text()).toContain('PANEL500')
    expect(wrapper.text()).toContain('Panel Fixed')
    expect(wrapper.text()).toContain('¥5.00')
    expect(wrapper.text()).toContain('1 / Unlimited')
    expect(wrapper.text()).not.toContain('LEGACY10')

    wrapper.unmount()
  })
})
