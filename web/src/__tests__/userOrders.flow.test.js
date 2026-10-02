import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Orders from '@/views/user/Orders.vue'
import { toastMessages } from './helpers/feedback'

const mockGetOrders = vi.fn()
const mockGetOrderDetail = vi.fn()

vi.mock('@/api/user', () => ({
  getOrders: (...args) => mockGetOrders(...args),
  getOrderDetail: (...args) => mockGetOrderDetail(...args),
}))

vi.mock('vue-router', async () => {
  const { h } = await vi.importActual('vue')
  return { RouterLink: { props: ['to'], setup: (props, { slots }) => () => h('a', { href: props.to }, slots.default?.()) } }
})

const ORDERS = [
  { id: 1, trade_no: 'ORD-100', plan: { id: 2, name: 'Pro' }, period: 'month', total_amount: 1200, status: 0, created_at: 1790000000 },
  { id: 2, trade_no: 'ORD-101', plan: { id: 3, name: 'Plus' }, period: 'year', total_amount: 9900, discount_amount: 100, status: 3, created_at: 1780000000, paid_at: 1780000300 },
]

function renderPage() {
  const user = userEvent.setup()
  render(Orders)
  return { user }
}

describe('User Orders flow', () => {
  beforeEach(() => {
    mockGetOrders.mockReset()
    mockGetOrderDetail.mockReset()
  })

  it('lists orders with plan, period, amount and status', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: ORDERS, total: 2 } })
    renderPage()
    const rows = await screen.findAllByRole('button', { name: /^Details:/ })
    expect(mockGetOrders).toHaveBeenCalledWith({ page: 1, page_size: 50 })
    expect(rows).toHaveLength(2)
    expect(rows[0].textContent).toContain('Pro')
    expect(rows[0].textContent).toContain('Monthly')
    expect(rows[0].textContent).toContain('¥12.00')
    expect(within(rows[0]).getByText('Unpaid')).toBeTruthy()
    expect(within(rows[1]).getByText('Completed')).toBeTruthy()
    // Only an unpaid order offers to pay.
    expect(screen.getAllByRole('button', { name: 'Pay' })).toHaveLength(1)
  })

  it('opens the details in a sheet', async () => {
    mockGetOrders.mockResolvedValue({ code: 0, data: { list: ORDERS } })
    mockGetOrderDetail.mockResolvedValue({ code: 0, data: ORDERS[1] })
    const { user } = renderPage()
    await user.click((await screen.findAllByRole('button', { name: /^Details:/ }))[1])
    const sheet = await screen.findByRole('dialog', { name: 'Order details' })
    expect(mockGetOrderDetail).toHaveBeenCalledWith(2)
    await waitFor(() => expect(within(sheet).getByText('ORD-101')).toBeTruthy())
    expect(within(sheet).getByText('¥100.00')).toBeTruthy()
    expect(within(sheet).getByText('−¥1.00')).toBeTruthy()
    expect(within(sheet).getByText('¥99.00')).toBeTruthy()
  })

  it('says why a detail could not load, and that online payment is not available yet', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [{ id: 6, trade_no: 'ORD-300', status: 0, total_amount: 100 }] } })
    mockGetOrderDetail.mockRejectedValue(new Error('offline'))
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: /^Details:/ }))
    const sheet = await screen.findByRole('dialog', { name: 'Order details' })
    expect((await within(sheet).findByRole('alert')).textContent).toContain('Could not open the order.')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await user.click(screen.getByRole('button', { name: 'Pay' }))
    expect(toastMessages('info')).toEqual(['Online payment isn’t available yet. Contact your administrator to pay.'])
  })

  it('names an order whose plan no longer exists', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [{ id: 7, plan_id: 404, trade_no: 'ORD-301', status: 2, total_amount: 500, period: 'onetime' }] } })
    renderPage()
    const [row] = await screen.findAllByRole('button', { name: /^Details:/ })
    expect(row.textContent).toContain('Plan no longer offered')
    expect(row.textContent).toContain('One-time')
    expect(within(row).getByText('Cancelled')).toBeTruthy()
  })

  it('shows an empty state that leads to the plans', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [] } })
    renderPage()
    expect(await screen.findByRole('heading', { level: 2, name: 'No orders yet' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Choose a plan' }).getAttribute('href')).toBe('/user/plans')
  })

  it('offers a retry when the orders cannot be loaded', async () => {
    mockGetOrders.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ data: { list: ORDERS } })
    const { user } = renderPage()
    expect(await screen.findByRole('heading', { name: 'Could not load your orders.' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findAllByRole('button', { name: /^Details:/ })).toHaveLength(2)
  })
})
