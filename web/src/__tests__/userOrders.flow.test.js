import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Orders from '@/views/user/Orders.vue'

const mockGetOrders = vi.fn()
const mockGetOrderDetail = vi.fn()

vi.mock('@/api/user', () => ({
  getOrders: (...args) => mockGetOrders(...args),
  getOrderDetail: (...args) => mockGetOrderDetail(...args),
}))

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
            plan: { name: 'Pro' },
            period: 'month',
            total_amount: 12345,
            status: 0,
            created_at: '2026-04-05T00:00:00.000Z',
          },
          {
            id: 2,
            trade_no: 'ORD-101',
            plan: { name: 'Plus' },
            period: 'quarter',
            total_amount: 67890,
            status: 1,
            created_at: '2026-04-05T01:00:00.000Z',
          },
        ],
      },
    })

    const wrapper = mount(Orders, {
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
      data: {
        id: 5,
        trade_no: 'ORD-200',
        period: 'year',
        total_amount: 250000,
        status: 2,
        created_at: '2026-04-05T02:00:00.000Z',
        paid_at: 1710003600,
        plan: { name: 'Enterprise' },
      },
    })

    const wrapper = mount(Orders, {
      global: {
        stubs: {
          'router-link': true,
        },
      },
    })
    await flushPromises()

    await wrapper.find('.btn-sm.btn-ghost').trigger('click')
    await flushPromises()

    expect(mockGetOrderDetail).toHaveBeenCalledWith(5)
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.find('.detail-grid').text()).toContain('ORD-200')
    expect(wrapper.find('.detail-grid').text()).toContain('Enterprise')
    expect(wrapper.find('.detail-grid').text()).toContain('2500.00')
  })

  it('shows empty state when no orders are returned', async () => {
    mockGetOrders.mockResolvedValue({ data: { list: [] } })

    const wrapper = mount(Orders, {
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
