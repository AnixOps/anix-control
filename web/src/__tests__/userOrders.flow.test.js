import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import Orders from '@/views/user/Orders.vue'
import { inBody, toastMessages } from './helpers/feedback'

const mockGetOrders = vi.fn()
const mockGetOrderDetail = vi.fn()

vi.mock('@/api/user', () => ({
  getOrders: (...args) => mockGetOrders(...args),
  getOrderDetail: (...args) => mockGetOrderDetail(...args),
}))

enableAutoUnmount(afterEach)

describe('User Orders flow', () => {
  beforeEach(() => {
    mockGetOrders.mockReset()
    mockGetOrderDetail.mockReset()
  })

  it('loads orders and renders table rows with status and amount', async () => {
    mockGetOrders.mockResolvedValue({
      data: {
        list: [
          {
            id: 1,
            trade_no: 'ORD-100',
            plan: { id: 2, name: 'Pro' },
            period: 'month',
            total_amount: 12345,
            status: 0,
            created_at: '2026-04-05T00:00:00.000Z',
          },
          {
            id: 2,
            trade_no: 'ORD-101',
            plan: { id: 3, name: 'Plus' },
            period: 'quarter',
            total_amount: 67890,
            status: 1,
            created_at: '2026-04-05T01:00:00.000Z',
          },
        ],
      },
    })

    const wrapper = mount(Orders, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': true,
        },
      },
    })
    await flushPromises()

    expect(mockGetOrders).toHaveBeenCalledWith({ page: 1, page_size: 50 })
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)

    const firstRow = rows[0]
    expect(firstRow.find('.trade-no').text()).toBe('ORD-100')
    expect(firstRow.find('.status-badge').classes()).toContain('pending')
    expect(firstRow.find('.status-badge').text()).not.toBe('')
    expect(firstRow.text()).toContain('123.45')

    const secondRow = rows[1]
    expect(secondRow.text()).toContain('Plus')
    expect(secondRow.find('.status-badge').classes()).toContain('paid')
    expect(secondRow.text()).toContain('678.90')
  })

  it('opens detail modal after clicking view detail button', async () => {
    mockGetOrders.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        list: [
          {
            id: 5,
            trade_no: 'ORD-200',
            period: 'year',
            total_amount: 250000,
            status: 2,
            created_at: '2026-04-05T02:00:00.000Z',
          },
        ],
      },
    })
    mockGetOrderDetail.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000001,
      data: {
        id: 5,
        trade_no: 'ORD-200',
        period: 'year',
        total_amount: 250000,
        status: 2,
        created_at: '2026-04-05T02:00:00.000Z',
        paid_at: 1710003600,
        plan: { id: 4, name: 'Enterprise' },
      },
    })

    const wrapper = mount(Orders, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': true,
        },
      },
    })
    await flushPromises()

    await wrapper.find('[data-test="order-detail-button"]').trigger('click')
    await flushPromises()

    expect(mockGetOrderDetail).toHaveBeenCalledWith(5)
    expect(inBody('[role="dialog"]').text()).toContain('Order details')
    expect(inBody('.detail-grid').text()).toContain('ORD-200')
    expect(inBody('.detail-grid').text()).toContain('Enterprise')
    expect(inBody('.detail-grid').text()).toContain('2500.00')
  })

  it('reports a detail that fails to load and an unavailable payment in toasts', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [{ id: 6, trade_no: 'ORD-300', status: 0, total_amount: 100 }] } })
    mockGetOrderDetail.mockRejectedValue(new Error('offline'))
    const wrapper = mount(Orders, { attachTo: document.body, global: { stubs: { 'router-link': true } } })
    await flushPromises()

    await wrapper.find('[data-test="order-detail-button"]').trigger('click')
    await flushPromises()
    expect(inBody('[role="dialog"]').exists()).toBe(false)
    expect(toastMessages('error')).toEqual(['The order details couldn’t be opened. Try again later.'])

    await wrapper.find('.action-buttons .btn-primary').trigger('click')
    expect(toastMessages('info')).toEqual(['Payment is still being integrated. Please try again later.'])
  })

  // The order answers carry plan as {id, name} and no buyer; an order whose
  // plan no longer exists has no plan.
  it('renders the slim order answer and an order without a plan', async () => {
    mockGetOrders.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        total: 2,
        list: [
          {
            id: 7,
            user_id: 3,
            plan_id: 2,
            trade_no: 'ORD-300',
            period: 'month',
            total_amount: 3000,
            discount_amount: 0,
            status: 3,
            paid_at: 1700000000,
            created_at: '2026-09-01T08:00:00Z',
            plan: { id: 2, name: 'Pro' },
          },
          {
            id: 8,
            user_id: 3,
            plan_id: 404,
            trade_no: 'ORD-301',
            period: 'month',
            total_amount: 1000,
            status: 0,
            created_at: '2026-09-01T09:00:00Z',
          },
        ],
      },
    })

    const wrapper = mount(Orders, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': true,
        },
      },
    })
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('Pro')
    expect(rows[0].find('.status-badge').classes()).toContain('completed')
    expect(rows[1].text()).not.toContain('Pro')
    expect(rows[1].text()).not.toContain('undefined')
  })

  it('shows empty state when no orders are returned', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [] } })

    const wrapper = mount(Orders, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
    expect(wrapper.find('table').exists()).toBe(false)
  })
})
