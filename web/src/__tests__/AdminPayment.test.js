import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Payment from '@/views/admin/Payment.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  createPaymentGateway: vi.fn(),
  deletePaymentGateway: vi.fn(),
  getPaymentGateways: vi.fn(),
  getPaymentRecords: vi.fn(),
  getPaymentStats: vi.fn(),
  togglePaymentGateway: vi.fn(),
  updatePaymentGateway: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Payment', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getPaymentGateways.mockResolvedValue({ data: { list: [] } })
    adminApi.getPaymentRecords.mockResolvedValue({ data: { list: [] } })
    adminApi.getPaymentStats.mockResolvedValue({ data: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders payment stats from legacy and panel envelope payloads', async () => {
    adminApi.getPaymentStats
      .mockResolvedValueOnce({
        data: {
          total_amount: 123.45,
          total_orders: 12,
          success_orders: 9,
          success_rate: 75,
          by_gateway: { stripe: { amount: 100, count: 5 } }
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          total_amount: 234.56,
          total_orders: 21,
          success_orders: 18,
          success_rate: 85.7,
          by_gateway: { epay: { amount: 200, count: 10 } }
        },
        ts: 1783526400000
      })

    const wrapper = mount(Payment)
    await flushPromises()

    let metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['¥123.45', '12', '9', '75.0%'])
    expect(wrapper.text()).toContain('Stripe')
    expect(wrapper.text()).toContain('¥100.00')

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['¥234.56', '21', '18', '85.7%'])
    expect(wrapper.text()).toContain('EPay')
    expect(wrapper.text()).toContain('¥200.00')

    wrapper.unmount()
  })

  it('renders payment gateways from legacy and panel envelope payloads', async () => {
    adminApi.getPaymentGateways
      .mockResolvedValueOnce({
        data: {
          list: [{
            enabled: true,
            fee_rate: 0.01,
            id: 1,
            max_amount: 1000,
            min_amount: 10,
            name: 'Legacy Stripe',
            type: 'stripe'
          }]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [{
            enabled: false,
            fee_rate: 0.02,
            id: 2,
            max_amount: 2000,
            min_amount: 20,
            name: 'Panel EPay',
            type: 'epay'
          }],
          total: 1
        },
        ts: 1783526400000
      })

    adminApi.getPaymentStats.mockResolvedValue({ data: {} })

    const wrapper = mount(Payment)
    await flushPromises()

    expect(wrapper.text()).toContain('Legacy Stripe')
    expect(wrapper.text()).toContain('Stripe')
    expect(wrapper.text()).toContain('1.00%')

    await wrapper.vm.fetchGateways()
    await flushPromises()

    expect(wrapper.text()).toContain('Panel EPay')
    expect(wrapper.text()).toContain('EPay')
    expect(wrapper.text()).toContain('2.00%')

    wrapper.unmount()
  })

  it('renders payment records from legacy and panel envelope payloads', async () => {
    adminApi.getPaymentRecords
      .mockResolvedValueOnce({
        data: {
          list: [{
            amount: 10,
            gateway_type: 'stripe',
            id: 1,
            status: 'paid',
            trade_no: 'LEGACY-PAY-001',
            user_id: 7
          }]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [{
            amount: 20,
            gateway_type: 'epay',
            id: 2,
            status: 'pending',
            trade_no: 'PANEL-PAY-001',
            user_id: 8
          }],
          total: 1
        },
        ts: 1783526400000
      })

    const wrapper = mount(Payment)
    await flushPromises()

    expect(wrapper.vm.records).toHaveLength(1)
    expect(wrapper.text()).toContain('LEGACY-PAY-001')
    expect(wrapper.text()).toContain('Stripe')
    expect(wrapper.text()).toContain('Paid')

    await wrapper.vm.fetchRecords()
    await flushPromises()

    expect(wrapper.vm.records).toHaveLength(1)
    expect(wrapper.text()).toContain('PANEL-PAY-001')
    expect(wrapper.text()).toContain('EPay')
    expect(wrapper.text()).toContain('Pending')

    wrapper.unmount()
  })
})
