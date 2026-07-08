import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Orders from '@/views/admin/Orders.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  cancelOrder: vi.fn(),
  getOrderList: vi.fn(),
  getOrderStats: vi.fn(),
  markOrderPaid: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Orders', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getOrderList.mockResolvedValue({ data: { list: [], total: 0 } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders order stats from legacy and panel envelope payloads', async () => {
    adminApi.getOrderStats
      .mockResolvedValueOnce({
        data: {
          total_orders: 12,
          pending_orders: 5,
          total_revenue: 12345,
          today_revenue: 678
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          total_orders: 21,
          pending_orders: 7,
          total_revenue: 23456,
          today_revenue: 789
        },
        ts: 1783526400000
      })

    const wrapper = mount(Orders)
    await flushPromises()

    let metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['12', '5', '¥123.45', '¥6.78'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['21', '7', '¥234.56', '¥7.89'])

    wrapper.unmount()
  })
})
