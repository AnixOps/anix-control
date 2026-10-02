import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { runAction } from '@/ui/composables/useToast'
import Payment from '@/views/admin/Payment.vue'
import { setLocale } from '@/i18n'
import { inBody, toastMessages, toasts } from './helpers/feedback'

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

enableAutoUnmount(afterEach)

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

  it('shows panel envelope errors for gateway mutations', async () => {
    adminApi.getPaymentGateways.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{
          enabled: false,
          fee_rate: 0.01,
          id: 1,
          max_amount: 1000,
          min_amount: 10,
          name: 'Panel Stripe',
          type: 'stripe'
        }]
      },
      ts: 1783526400000
    })
    adminApi.togglePaymentGateway.mockResolvedValue({
      code: -1,
      msg: 'gateway not found',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Payment)
    await flushPromises()

    await wrapper.vm.toggleGatewayStatus({ id: 1, enabled: false })
    await flushPromises()

    expect(adminApi.togglePaymentGateway).toHaveBeenCalledWith(1, true)
    expect(toastMessages('error')).toEqual([expect.stringContaining('gateway not found')])
    expect(adminApi.getPaymentGateways).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  describe('dialogs and feedback', () => {
    const gateway = { enabled: true, fee_rate: 0.01, id: 4, max_amount: 1000, min_amount: 10, name: 'Stripe EU', type: 'stripe', config: { key: 'k' } }
    const Harness = {
      components: { Payment, UiHost },
      template: '<div><Payment /><UiHost /></div>'
    }

    beforeEach(() => {
      adminApi.getPaymentGateways.mockResolvedValue({ code: 0, data: { list: [gateway] } })
      adminApi.getPaymentRecords.mockResolvedValue({ code: 0, data: { list: [{ id: 9, user_id: 3, gateway_type: 'stripe', trade_no: 'T-900', amount: 1234, status: 'paid', created_at: 1783526400 }] } })
    })

    it('disables a gateway at once and offers undo', async () => {
      adminApi.togglePaymentGateway.mockResolvedValue({ code: 0 })
      const wrapper = mount(Payment)
      await flushPromises()

      await wrapper.vm.toggleGatewayStatus(gateway)
      expect(adminApi.togglePaymentGateway).toHaveBeenCalledWith(4, false)
      const [toast] = toasts('success')
      expect(toast.message).toBe('Stripe EU disabled')
      await runAction(toast.id)
      expect(adminApi.togglePaymentGateway).toHaveBeenLastCalledWith(4, true)
    })

    it('confirms deleting a gateway; Cancel keeps it, a failure stays inline', async () => {
      const user = userEvent.setup()
      adminApi.deletePaymentGateway.mockResolvedValueOnce({ code: -1, msg: 'gateway has payments' }).mockResolvedValueOnce({ code: 0 })
      render(Harness)
      await screen.findByText('Stripe EU')
      const opener = screen.getByRole('button', { name: 'Delete' })

      await user.click(opener)
      let dialog = await screen.findByRole('alertdialog', { name: 'Delete payment gateway Stripe EU?' })
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deletePaymentGateway).not.toHaveBeenCalled()

      await user.click(opener)
      dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete gateway' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('gateway has payments')
      await user.click(within(dialog).getByRole('button', { name: 'Delete gateway' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deletePaymentGateway).toHaveBeenLastCalledWith(4)
      expect(toastMessages('success')).toEqual(['Gateway Stripe EU deleted'])
    })

    it('keeps invalid JSON and save failures inside the gateway dialog', async () => {
      adminApi.updatePaymentGateway.mockResolvedValueOnce({ code: -1, msg: 'name taken' }).mockResolvedValueOnce({ code: 0 })
      const wrapper = mount(Payment, { attachTo: document.body })
      await flushPromises()

      wrapper.vm.openGatewayModal(gateway)
      await flushPromises()
      await inBody('[data-test="payment-gateway-config"]').setValue('{bad json')
      await inBody('[data-test="payment-gateway-save"]').trigger('click')
      await flushPromises()
      expect(inBody('#payment-gateway-config-error').text()).toBe('Configuration JSON is invalid')
      expect(inBody('[data-test="payment-gateway-config"]').attributes('aria-invalid')).toBe('true')
      expect(adminApi.updatePaymentGateway).not.toHaveBeenCalled()

      await inBody('[data-test="payment-gateway-config"]').setValue('{"key":"k2"}')
      await inBody('[data-test="payment-gateway-save"]').trigger('click')
      await flushPromises()
      expect(inBody('[data-test="payment-gateway-error"]').text()).toContain('name taken')
      expect(wrapper.vm.showGatewayModal).toBe(true)
      expect(toasts()).toHaveLength(0)

      await inBody('[data-test="payment-gateway-save"]').trigger('click')
      await flushPromises()
      expect(adminApi.updatePaymentGateway).toHaveBeenLastCalledWith(4, expect.objectContaining({ config: { key: 'k2' } }))
      expect(wrapper.vm.showGatewayModal).toBe(false)
      expect(toastMessages('success')).toEqual(['Gateway saved successfully'])
    })

    it('shows a payment record in a side sheet that closes with Esc', async () => {
      const user = userEvent.setup()
      render(Harness)
      await user.click(await screen.findByRole('button', { name: 'Records' }))
      const opener = await screen.findByRole('button', { name: 'Details' })
      await user.click(opener)
      const sheet = await screen.findByRole('dialog', { name: 'Payment Details' })
      expect(sheet.textContent).toContain('T-900')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))
    })
  })
})
