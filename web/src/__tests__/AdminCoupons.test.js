import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import Coupons from '@/views/admin/Coupons.vue'
import { setLocale } from '@/i18n'
import { toastMessages, toasts } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  createCoupon: vi.fn(),
  deleteCoupon: vi.fn(),
  getCoupons: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  default: adminApi,
  ...adminApi
}))

describe('Admin Coupons', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders coupons from legacy and panel envelope payloads', async () => {
    adminApi.getCoupons
      .mockResolvedValueOnce({
        data: [
          {
            id: 1,
            code: 'LEGACY10',
            name: 'Legacy Discount',
            type: 1,
            value: 10,
            use_count: 2,
            limit_use: 100,
            started_at: 1783526400,
            ended_at: 1786118400
          }
        ]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [
          {
            id: 2,
            code: 'PANEL500',
            name: 'Panel Fixed',
            type: 2,
            value: 500,
            use_count: 1,
            limit_use: -1,
            started_at: 1783526400,
            ended_at: 1786118400
          }
        ],
        ts: 1783526400000
      })

    const wrapper = mount(Coupons)
    await flushPromises()

    expect(wrapper.text()).toContain('LEGACY10')
    expect(wrapper.text()).toContain('Legacy Discount')
    expect(wrapper.text()).toContain('10%')
    expect(wrapper.text()).toContain('2 / 100')

    await wrapper.vm.load()
    await flushPromises()

    expect(wrapper.text()).toContain('PANEL500')
    expect(wrapper.text()).toContain('Panel Fixed')
    expect(wrapper.text()).toContain('¥5.00')
    expect(wrapper.text()).toContain('1 / Unlimited')
    expect(wrapper.text()).not.toContain('LEGACY10')

    wrapper.unmount()
  })

  describe('dialogs and feedback', () => {
    const Harness = {
      components: { Coupons, UiHost },
      template: '<div><Coupons /><UiHost /></div>'
    }

    beforeEach(() => {
      adminApi.getCoupons.mockResolvedValue({ data: [{ id: 3, code: 'SUMMER', name: 'Summer', type: 1, value: 10, use_count: 0, limit_use: 10, started_at: 1783526400, ended_at: 1786118400 }] })
    })

    it('creates a coupon in a dialog with inline field and API errors', async () => {
      const user = userEvent.setup()
      adminApi.createCoupon.mockRejectedValueOnce(new Error('code exists')).mockResolvedValueOnce({})
      render(Harness)
      await screen.findByText('SUMMER')
      const opener = screen.getByRole('button', { name: 'New coupon' })
      await user.click(opener)
      const dialog = await screen.findByRole('dialog', { name: 'New coupon' })

      await user.click(within(dialog).getByRole('button', { name: 'Create coupon' }))
      expect(within(dialog).getByLabelText(/Coupon code/).getAttribute('aria-invalid')).toBe('true')
      expect(dialog.textContent).toContain('Enter a coupon code')
      expect(dialog.textContent).toContain('Enter a coupon name')
      expect(adminApi.createCoupon).not.toHaveBeenCalled()

      await user.type(within(dialog).getByLabelText(/Coupon code/), 'new10')
      await user.type(within(dialog).getByLabelText(/Coupon name/), 'New')
      await user.click(within(dialog).getByRole('button', { name: 'Create coupon' }))
      await waitFor(() => expect(dialog.textContent).toContain('code exists'))
      expect(toasts()).toHaveLength(0)

      await user.click(within(dialog).getByRole('button', { name: 'Create coupon' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.createCoupon).toHaveBeenLastCalledWith(expect.objectContaining({ code: 'NEW10', name: 'New' }))
      expect(toastMessages('success')).toEqual(['Coupon created successfully'])
      await waitFor(() => expect(document.activeElement).toBe(opener))
    })

    it('confirms deleting a coupon; Esc keeps it, a failure stays inline', async () => {
      const user = userEvent.setup()
      adminApi.deleteCoupon.mockRejectedValueOnce(new Error('coupon in use')).mockResolvedValueOnce({})
      render(Harness)
      await screen.findByText('SUMMER')
      const menu = screen.getByRole('button', { name: 'Actions for SUMMER' })
      const opener = {
        async click() {
          await user.click(menu)
          await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Delete coupon' }))
        }
      }

      await opener.click()
      await screen.findByRole('alertdialog', { name: 'Delete coupon SUMMER?' })
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteCoupon).not.toHaveBeenCalled()
      expect(document.activeElement).toBe(menu)

      await opener.click()
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete coupon' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('coupon in use')
      await user.click(within(dialog).getByRole('button', { name: 'Delete coupon' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteCoupon).toHaveBeenLastCalledWith(3)
      expect(toastMessages('success')).toEqual(['Coupon SUMMER deleted'])
    })
      it('searches and filters by type in the page, then clears the filters', async () => {
      const user = userEvent.setup()
      adminApi.getCoupons.mockResolvedValue({ data: [
        { id: 3, code: 'SUMMER', name: 'Summer', type: 1, value: 10, use_count: 0, limit_use: 10 },
        { id: 4, code: 'FIVE', name: 'Five off', type: 2, value: 500, use_count: 3, limit_use: -1 }
      ] })
      render(Harness)
      await screen.findByText('SUMMER')
      expect(screen.getByText('¥5.00')).toBeTruthy()
      await user.click(within(screen.getByRole('group', { name: 'Filter by type' })).getByRole('button', { name: 'Fixed amount' }))
      expect(screen.queryByText('SUMMER')).toBeNull()
      expect(screen.getByText('FIVE')).toBeTruthy()
      await user.type(screen.getByRole('searchbox', { name: 'Search code or name' }), 'zzz')
      await user.click(await screen.findByRole('button', { name: 'Clear filters' }))
      expect(await screen.findByText('SUMMER')).toBeTruthy()
      expect(adminApi.getCoupons).toHaveBeenCalledTimes(1)
    })

    it('shows the empty state and a load error with retry', async () => {
      const user = userEvent.setup()
      adminApi.getCoupons.mockRejectedValueOnce(new Error('Network Error')).mockResolvedValueOnce({ data: [] })
      render(Harness)
      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toContain('Coupons didn’t load')
      await user.click(within(alert).getByRole('button', { name: 'Try again' }))
      expect(await screen.findByRole('heading', { name: 'No coupons yet' })).toBeTruthy()
    })
  })
})
