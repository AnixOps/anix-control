import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Plans from '@/views/user/Plans.vue'

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

    const wrapper = mount(Plans)
    await flushPromises()

    expect(mockGetPlans).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.plan-card')).toHaveLength(1)

    await wrapper.find('.plan-card .btn-primary.w-full').trigger('click')
    await flushPromises()

    expect(wrapper.find('.modal').exists()).toBe(true)

    await wrapper.find('.modal-footer .btn-primary').trigger('click')
    await flushPromises()

    expect(mockSaveOrder).toHaveBeenCalledWith({
      plan_id: 1,
      period: 'month',
      coupon_id: null,
    })
    expect(mockPush).toHaveBeenCalledWith('/user/orders')
  })

  it('applies coupon with selected plan id', async () => {
    mockGetPlans.mockResolvedValue({
      data: [
        {
          id: 2,
          name: 'Starter',
          transfer_enable: 100,
          month_price: 1000,
        },
      ],
    })
    mockCheckCoupon.mockResolvedValue({
      data: { id: 9, name: 'SPRING', type: 2, value: 100 },
    })

    const wrapper = mount(Plans)
    await flushPromises()

    await wrapper.find('.plan-card .btn-primary.w-full').trigger('click')
    await wrapper.find('.coupon-input-group input').setValue('SPRING')
    await wrapper.find('.coupon-input-group .btn-secondary').trigger('click')
    await flushPromises()

    expect(mockCheckCoupon).toHaveBeenCalledWith({
      code: 'SPRING',
      plan_id: 2,
    })
    expect(wrapper.find('.coupon-input-group .btn-ghost').exists()).toBe(true)
  })
})
