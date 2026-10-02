import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import Plans from '@/views/user/Plans.vue'
import { inBody, toastMessages } from './helpers/feedback'

const mockPush = vi.fn()
const mockGetPlans = vi.fn()
const mockCheckCoupon = vi.fn()
const mockSaveOrder = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}))

vi.mock('@/api/user', () => ({
  getPlans: (...args) => mockGetPlans(...args),
  checkCoupon: (...args) => mockCheckCoupon(...args),
  saveOrder: (...args) => mockSaveOrder(...args),
}))

enableAutoUnmount(afterEach)

describe('User Plans flow', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockGetPlans.mockReset()
    mockCheckCoupon.mockReset()
    mockSaveOrder.mockReset()
  })

  it('loads plans and submits order successfully', async () => {
    mockGetPlans.mockResolvedValue({
      data: [
        {
          id: 1,
          name: 'Pro Plan',
          transfer_enable: 200,
          month_price: 1200,
          quarter_price: 3000,
        },
      ],
    })
    mockSaveOrder.mockResolvedValue({ data: { id: 100 } })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    expect(mockGetPlans).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.plan-card')).toHaveLength(1)

    await wrapper.find('[data-test="plan-buy-button"]').trigger('click')
    await flushPromises()

    expect(inBody('[role="dialog"]').text()).toContain('Pro Plan')

    await inBody('[data-test="order-submit-button"]').trigger('click')
    await flushPromises()

    expect(mockSaveOrder).toHaveBeenCalledWith({
      plan_id: 1,
      period: 'month',
      coupon_id: null,
    })
    expect(mockPush).toHaveBeenCalledWith('/user/orders')
    expect(toastMessages('success')).toEqual(['Order created. Pay for it in My Orders.'])
  })

  it('keeps a failed order in the dialog with an inline error', async () => {
    mockGetPlans.mockResolvedValue({ data: [{ id: 1, name: 'Pro Plan', month_price: 1200 }] })
    mockSaveOrder.mockRejectedValue({ response: { data: { message: 'Plan sold out' } } })
    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('[data-test="plan-buy-button"]').trigger('click')
    await flushPromises()
    await inBody('[data-test="order-submit-button"]').trigger('click')
    await flushPromises()
    expect(inBody('[data-test="order-error"]').text()).toBe('Plan sold out')
    expect(inBody('[role="dialog"]').exists()).toBe(true)
    expect(mockPush).not.toHaveBeenCalled()
  })

  it.each([
    ['legacy coupon payload', { data: { id: 9, name: 'SPRING', type: 2, value: 100 } }],
    [
      'panel coupon envelope',
      {
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: { id: 9, name: 'SPRING', type: 2, value: 100 },
      },
    ],
  ])('applies coupon with selected plan id from %s', async (_label, couponResponse) => {
    mockGetPlans.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: [
        {
          id: 2,
          name: 'Starter',
          transfer_enable: 100,
          month_price: 1000,
        },
      ],
    })
    mockCheckCoupon.mockResolvedValue(couponResponse)

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    await wrapper.find('[data-test="plan-buy-button"]').trigger('click')
    await flushPromises()
    await inBody('.coupon-input-group input').setValue('SPRING')
    await inBody('[data-test="coupon-verify-button"]').trigger('click')
    await flushPromises()

    expect(mockCheckCoupon).toHaveBeenCalledWith({
      code: 'SPRING',
      plan_id: 2,
    })
    expect(inBody('[data-test="coupon-remove-button"]').exists()).toBe(true)
  })

  it('does not render plans when panel envelope loading fails', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    mockGetPlans.mockResolvedValue({
      code: -1,
      msg: '获取套餐列表失败',
      data: null,
      ts: 1783536000000,
    })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    expect(wrapper.findAll('.plan-card')).toHaveLength(0)
    expect(errorSpy).toHaveBeenCalledWith(
      'Failed to load plans:',
      expect.any(Error)
    )
    errorSpy.mockRestore()
  })
})
