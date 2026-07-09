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
    adminApi.getOrderStats.mockResolvedValue({ data: {} })
    adminApi.markOrderPaid.mockResolvedValue({})
    adminApi.cancelOrder.mockResolvedValue({})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders order lists from legacy, panel envelope, and nested payloads', async () => {
    adminApi.getOrderList
      .mockResolvedValueOnce({
        data: {
          list: [{ id: 1, trade_no: 'LEGACY001', status: 0 }],
          total: 1
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [{ id: 2, trade_no: 'PANEL002', status: 1 }],
          total: 2
        },
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: {
            list: [{ id: 3, trade_no: 'NESTED003', status: 2 }],
            total: 3
          }
        }
      })

    const wrapper = mount(Orders)
    await flushPromises()

    expect(wrapper.vm.orders[0].trade_no).toBe('LEGACY001')
    expect(wrapper.vm.total).toBe(1)

    await wrapper.vm.fetchOrders()
    await flushPromises()

    expect(wrapper.vm.orders[0].trade_no).toBe('PANEL002')
    expect(wrapper.vm.total).toBe(2)

    await wrapper.vm.fetchOrders()
    await flushPromises()

    expect(wrapper.vm.orders[0].trade_no).toBe('NESTED003')
    expect(wrapper.vm.total).toBe(3)

    wrapper.unmount()
  })

  it('renders order stats from legacy, panel envelope, and nested payloads', async () => {
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
      .mockResolvedValueOnce({
        data: {
          data: {
            total_orders: 31,
            pending_orders: 9,
            total_revenue: 34567,
            today_revenue: 890
          }
        }
      })

    const wrapper = mount(Orders)
    await flushPromises()

    let metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['12', '5', '¥123.45', '¥6.78'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['21', '7', '¥234.56', '¥7.89'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['31', '9', '¥345.67', '¥8.90'])

    wrapper.unmount()
  })

  it('treats panel code -1 mark-paid responses as errors', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    adminApi.markOrderPaid.mockResolvedValueOnce({
      code: -1,
      msg: '订单不存在',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Orders)
    await flushPromises()

    await wrapper.vm.handleMarkPaid({ id: 99, trade_no: 'MISSING-ORDER' })
    await flushPromises()

    expect(alertSpy).toHaveBeenCalledWith(expect.stringContaining('订单不存在'))
    expect(adminApi.markOrderPaid).toHaveBeenCalledWith(99)

    wrapper.unmount()
  })

  it('treats panel code -1 cancel responses as errors', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    adminApi.cancelOrder.mockResolvedValueOnce({
      code: -1,
      msg: '订单不存在',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Orders)
    await flushPromises()

    await wrapper.vm.handleCancel({ id: 88, trade_no: 'MISSING-CANCEL' })
    await flushPromises()

    expect(alertSpy).toHaveBeenCalledWith(expect.stringContaining('订单不存在'))
    expect(adminApi.cancelOrder).toHaveBeenCalledWith(88)

    wrapper.unmount()
  })
})
