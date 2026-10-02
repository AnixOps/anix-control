import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Plans from '@/views/user/Plans.vue'
import { toastMessages } from './helpers/feedback'

const mockPush = vi.fn()
const mockGetPlans = vi.fn()
const mockCheckCoupon = vi.fn()
const mockSaveOrder = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/api/user', () => ({
  getPlans: (...args) => mockGetPlans(...args),
  checkCoupon: (...args) => mockCheckCoupon(...args),
  saveOrder: (...args) => mockSaveOrder(...args),
}))

const PLANS = [
  { id: 1, name: 'Pro Plan', transfer_enable: 200, speed_limit: 500, device_limit: 3, month_price: 1200, quarter_price: 3000, content: '<p>Every region</p>\n- Priority support' },
  { id: 2, name: 'Starter', transfer_enable: 100, month_price: 1000 },
]

function renderPage() {
  const user = userEvent.setup()
  render(Plans)
  return { user }
}

describe('User Plans flow', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockGetPlans.mockReset()
    mockCheckCoupon.mockReset()
    mockSaveOrder.mockReset()
  })

  it('shows store-style plan cards with the price for the chosen period', async () => {
    mockGetPlans.mockResolvedValue({ data: PLANS })
    const { user } = renderPage()
    const pro = (await screen.findByRole('heading', { level: 2, name: 'Pro Plan' })).closest('li')
    expect(within(pro).getByText('¥12.00')).toBeTruthy()
    expect(within(pro).getByText('/ month')).toBeTruthy()
    expect(within(pro).getByText('200 GB of traffic')).toBeTruthy()
    expect(within(pro).getByText('Up to 500 Mbps')).toBeTruthy()
    expect(within(pro).getByText('Up to 3 devices at a time')).toBeTruthy()
    // The plan description, one line per feature, without its markup.
    expect(within(pro).getByText('Every region')).toBeTruthy()
    expect(within(pro).getByText('Priority support')).toBeTruthy()

    const periods = screen.getByRole('group', { name: 'Billing period' })
    await user.click(within(periods).getByRole('button', { name: 'Quarterly' }))
    expect(within(pro).getByText('¥30.00')).toBeTruthy()
    const starter = screen.getByRole('heading', { level: 2, name: 'Starter' }).closest('li')
    expect(within(starter).getByText('Not offered for this period')).toBeTruthy()
    expect(within(starter).getByRole('button', { name: 'Buy Starter' }).disabled).toBe(true)
  })

  it('places an order from the checkout dialog', async () => {
    mockGetPlans.mockResolvedValue({ data: PLANS })
    mockSaveOrder.mockResolvedValue({ data: { id: 100 } })
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: 'Buy Pro Plan' }))
    const dialog = await screen.findByRole('dialog', { name: 'Review your order' })
    expect(within(dialog).getByText('Pro Plan')).toBeTruthy()
    await user.click(within(dialog).getByRole('button', { name: 'Quarterly' }))
    expect(within(dialog).getByText('¥30.00', { selector: '[data-checkout-total]' })).toBeTruthy()
    await user.click(within(dialog).getByRole('button', { name: 'Place order' }))
    await waitFor(() => expect(mockSaveOrder).toHaveBeenCalledWith({ plan_id: 1, period: 'quarter', coupon_id: null }))
    expect(mockPush).toHaveBeenCalledWith('/user/orders')
    expect(toastMessages('success')).toEqual(['Order placed. Pay for it in Orders.'])
  })

  it('keeps a failed order in the dialog with an inline error', async () => {
    mockGetPlans.mockResolvedValue({ data: PLANS })
    mockSaveOrder.mockRejectedValue({ response: { data: { message: 'Plan sold out' } } })
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: 'Buy Pro Plan' }))
    const dialog = await screen.findByRole('dialog', { name: 'Review your order' })
    await user.click(within(dialog).getByRole('button', { name: 'Place order' }))
    expect((await within(dialog).findByRole('alert')).textContent).toBe('The order was not placed: Plan sold out')
    expect(screen.getByRole('dialog', { name: 'Review your order' })).toBeTruthy()
    expect(mockPush).not.toHaveBeenCalled()
  })

  it.each([
    ['legacy coupon payload', { data: { id: 9, name: 'SPRING', type: 2, value: 100 } }],
    ['panel coupon envelope', { code: 0, msg: '操作成功', ts: 1783536000000, data: { id: 9, name: 'SPRING', type: 2, value: 100 } }],
  ])('applies a coupon for the selected plan (%s)', async (_label, couponResponse) => {
    mockGetPlans.mockResolvedValue({ code: 0, msg: '操作成功', data: [{ id: 2, name: 'Starter', transfer_enable: 100, month_price: 1000 }] })
    mockCheckCoupon.mockResolvedValue(couponResponse)
    mockSaveOrder.mockResolvedValue({ code: 0, data: { id: 1 } })
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: 'Buy Starter' }))
    const dialog = await screen.findByRole('dialog', { name: 'Review your order' })
    await user.type(within(dialog).getByLabelText(/^Coupon code/), 'SPRING')
    await user.click(within(dialog).getByRole('button', { name: 'Apply' }))
    await waitFor(() => expect(mockCheckCoupon).toHaveBeenCalledWith({ code: 'SPRING', plan_id: 2 }))
    expect(await within(dialog).findByText('Coupon “SPRING” applied')).toBeTruthy()
    expect(within(dialog).getByText('−¥1.00')).toBeTruthy()
    expect(within(dialog).getByText('¥9.00', { selector: '[data-checkout-total]' })).toBeTruthy()
    await user.click(within(dialog).getByRole('button', { name: 'Place order' }))
    await waitFor(() => expect(mockSaveOrder).toHaveBeenCalledWith({ plan_id: 2, period: 'month', coupon_id: 9 }))
  })

  it('shows a coupon the server rejects next to the field', async () => {
    mockGetPlans.mockResolvedValue({ data: PLANS })
    mockCheckCoupon.mockResolvedValue({ code: 1, msg: '优惠券已过期' })
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: 'Buy Starter' }))
    const dialog = await screen.findByRole('dialog', { name: 'Review your order' })
    await user.type(within(dialog).getByLabelText(/^Coupon code/), 'OLD')
    await user.click(within(dialog).getByRole('button', { name: 'Apply' }))
    expect(await within(dialog).findByText('优惠券已过期')).toBeTruthy()
  })

  it('shows an empty state, and a retryable error when the plans cannot be loaded', async () => {
    mockGetPlans.mockResolvedValueOnce({ code: -1, msg: '获取套餐列表失败', data: null }).mockResolvedValueOnce({ data: [] })
    const { user } = renderPage()
    expect(await screen.findByRole('heading', { name: 'Could not load the plans.' })).toBeTruthy()
    expect(screen.getByText('获取套餐列表失败')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { level: 2, name: 'No plans on sale' })).toBeTruthy()
  })
})
