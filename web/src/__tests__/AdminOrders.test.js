import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import Orders from '@/views/admin/Orders.vue'
import { setLocale } from '@/i18n'
import { answerConfirms, inBody, toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  cancelOrder: vi.fn(),
  getOrderList: vi.fn(),
  getOrderStats: vi.fn(),
  markOrderPaid: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

enableAutoUnmount(afterEach)

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

  // The administrator's order answers carry user as {id, email} and plan as
  // {id, name}; the detail modal shows the list row.
  it('renders the buyer and plan of the slim order answer', async () => {
    adminApi.getOrderList.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      ts: 1783526400000,
      data: {
        total: 2,
        list: [
          {
            id: 12,
            user_id: 3,
            plan_id: 2,
            type: 2,
            trade_no: 'SLIM012',
            period: 'quarter',
            total_amount: 8000,
            status: 1,
            paid_at: 1700000000,
            callback_no: 'CB-12',
            created_at: '2026-09-01T08:00:00Z',
            user: { id: 3, email: 'buyer@example.test' },
            plan: { id: 2, name: 'Pro' }
          },
          {
            id: 13,
            user_id: 99,
            plan_id: 404,
            trade_no: 'GONE013',
            period: 'month',
            total_amount: 1000,
            status: 0,
            created_at: '2026-09-01T09:00:00Z'
          }
        ]
      }
    })

    const wrapper = mount(Orders, { attachTo: document.body })
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    const headers = wrapper.findAll('thead th').map(th => th.text())
    const cellOf = (row, header) => row.findAll('td')[headers.indexOf(header)].text()
    expect(cellOf(rows[0], 'User')).toBe('buyer@example.test')
    expect(cellOf(rows[0], 'Plan')).toBe('Pro')
    expect(cellOf(rows[0], 'Amount')).toBe('¥80.00')
    expect(cellOf(rows[1], 'User')).toBe('—')
    expect(cellOf(rows[1], 'Plan')).toBe('—')

    await wrapper.vm.viewDetail(wrapper.vm.orders[0])
    await flushPromises()
    const detail = inBody('[data-test="order-detail"]').text()
    expect(detail).toContain('SLIM012')
    expect(detail).toContain('buyer@example.test')
    expect(detail).toContain('Pro')
    expect(detail).toContain('CB-12')
    // A side sheet, not a centred modal.
    expect(inBody('[role="dialog"]').classes()).toContain('ui-sheet')
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

    let metricValues = wrapper.findAll('[data-test="order-summary"] strong').map(node => node.text())
    expect(metricValues).toEqual(['12', '5', '¥123.45', '¥6.78'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('[data-test="order-summary"] strong').map(node => node.text())
    expect(metricValues).toEqual(['21', '7', '¥234.56', '¥7.89'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('[data-test="order-summary"] strong').map(node => node.text())
    expect(metricValues).toEqual(['31', '9', '¥345.67', '¥8.90'])

    wrapper.unmount()
  })

  it('treats panel code -1 mark-paid responses as errors', async () => {
    const confirms = answerConfirms(true)
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

    expect(confirms.errors.map(error => error.message)).toEqual(['订单不存在'])
    expect(toastMessages('success')).toEqual([])
    expect(adminApi.markOrderPaid).toHaveBeenCalledWith(99)

    wrapper.unmount()
  })

  it('treats panel code -1 cancel responses as errors', async () => {
    const confirms = answerConfirms(true)
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

    expect(confirms.last()).toMatchObject({ tone: 'danger', title: 'Cancel order MISSING-CANCEL?', confirmLabel: 'Cancel order' })
    expect(confirms.errors.map(error => error.message)).toEqual(['订单不存在'])
    expect(adminApi.cancelOrder).toHaveBeenCalledWith(88)

    wrapper.unmount()
  })

  it('asks before marking an order paid; Cancel and Esc keep it', async () => {
    const user = userEvent.setup()
    adminApi.getOrderList.mockResolvedValue({ code: 0, data: { total: 1, list: [{ id: 7, trade_no: 'T-7', total_amount: 1990, status: 0, period: 'month' }] } })
    adminApi.markOrderPaid.mockResolvedValue({ code: 0 })
    render({ components: { Orders, UiHost }, template: '<div><Orders /><UiHost /></div>' })
    const menu = await screen.findByRole('button', { name: 'Actions for T-7' })
    const opener = {
      async click() {
        await user.click(menu)
        await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Mark paid' }))
      }
    }

    await opener.click()
    let dialog = await screen.findByRole('alertdialog', { name: 'Mark order T-7 as paid?' })
    expect(dialog.textContent).toContain('19.90')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(document.activeElement).toBe(menu)
    await opener.click()
    await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.markOrderPaid).not.toHaveBeenCalled()

    await opener.click()
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Mark paid' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.markOrderPaid).toHaveBeenCalledWith(7)
    expect(toastMessages('success')).toEqual(['Order marked as paid'])
  })
  it('opens the details from a row and acts on a pending order there', async () => {
    const user = userEvent.setup()
    adminApi.getOrderList.mockResolvedValue({ code: 0, data: { total: 1, list: [{ id: 7, trade_no: 'T-7', total_amount: 1990, status: 0, period: 'month', user: { id: 1, email: 'a@example.test' } }] } })
    adminApi.cancelOrder.mockResolvedValue({ code: 0 })
    render({ components: { Orders, UiHost }, template: '<div><Orders /><UiHost /></div>' })
    const row = (await screen.findByText('T-7')).closest('tr')
    row.focus()
    await user.keyboard('{Enter}')
    const sheet = await screen.findByRole('dialog', { name: 'Order details' })
    expect(within(sheet).getByText('a@example.test')).toBeTruthy()
    expect(within(sheet).getByText('Pending')).toBeTruthy()
    await user.click(within(sheet).getByRole('button', { name: 'Cancel order' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Cancel order T-7?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel order' }))
    await waitFor(() => expect(adminApi.cancelOrder).toHaveBeenCalledWith(7))
    expect(toastMessages('success')).toEqual(['Order T-7 cancelled'])
  })

  it('filters by status and searches on the server', async () => {
    const user = userEvent.setup()
    render({ components: { Orders, UiHost }, template: '<div><Orders /><UiHost /></div>' })
    await screen.findByRole('heading', { name: 'No orders yet' })
    await user.click(within(screen.getByRole('group', { name: 'Filter by status' })).getByRole('button', { name: 'Paid' }))
    await waitFor(() => expect(adminApi.getOrderList).toHaveBeenLastCalledWith(expect.objectContaining({ status: '1', page: 1 })))
    await user.type(screen.getByRole('searchbox', { name: 'User email' }), 'buyer{Enter}')
    await waitFor(() => expect(adminApi.getOrderList).toHaveBeenLastCalledWith(expect.objectContaining({ email: 'buyer', status: '1' })))
    // Filtered and empty: offers to clear the filters.
    await user.click(await screen.findByRole('button', { name: 'Clear filters' }))
    await waitFor(() => expect(adminApi.getOrderList).toHaveBeenLastCalledWith(expect.objectContaining({ email: '', status: undefined, trade_no: '' })))
  })

  it('sorts on the server by the columns the API supports, from the first page', async () => {
    const user = userEvent.setup()
    adminApi.getOrderList.mockResolvedValue({ code: 0, data: { total: 1, list: [{ id: 7, trade_no: 'T-7', total_amount: 1990, status: 0, period: 'month', user: { id: 1, email: 'a@example.test' } }] } })
    render({ components: { Orders, UiHost }, template: '<div><Orders /><UiHost /></div>' })
    await screen.findByText('T-7')
    // The buyer's e-mail and the plan are other tables' columns: plain headers.
    for (const name of ['Order #', 'Status', 'Amount', 'Created']) {
      expect(within(screen.getByRole('columnheader', { name: new RegExp(`^${name}`) })).getByRole('button')).toBeTruthy()
    }
    for (const name of ['User', 'Plan']) {
      expect(within(screen.getByRole('columnheader', { name })).queryByRole('button')).toBeNull()
    }
    await user.click(within(screen.getByRole('columnheader', { name: /^Amount/ })).getByRole('button'))
    await waitFor(() => expect(adminApi.getOrderList).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, sort: 'total_amount', order: 'desc' })))
    await user.click(within(screen.getByRole('columnheader', { name: /^Order #/ })).getByRole('button'))
    await waitFor(() => expect(adminApi.getOrderList).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'trade_no', order: 'asc' })))
  })

  it('shows a load error with retry', async () => {
    const user = userEvent.setup()
    adminApi.getOrderList.mockRejectedValueOnce(new Error('Network Error'))
    render({ components: { Orders, UiHost }, template: '<div><Orders /><UiHost /></div>' })
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Orders didn’t load')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { name: 'No orders yet' })).toBeTruthy()
  })
})
