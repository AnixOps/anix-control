import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Tickets from '@/views/admin/Tickets.vue'
import { setLocale } from '@/i18n'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  closeTicket: vi.fn(),
  getTickets: vi.fn(),
  replyTicket: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  default: adminApi,
  ...adminApi
}))

describe('Admin Tickets', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders tickets from legacy and panel envelope payloads', async () => {
    adminApi.getTickets
      .mockResolvedValueOnce({
        data: [
          { id: 1, user_id: 10, subject: 'Legacy open', level: 1, status: 0, created_at: 1783526400 },
          { id: 2, user_id: 11, subject: 'Legacy closed', level: 2, status: 2, created_at: 1783526500 }
        ]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [
          { id: 3, user_id: 12, subject: 'Panel answered', level: 0, status: 1, created_at: 1783526600 }
        ],
        ts: 1783526400000
      })

    const wrapper = mount(Tickets)
    await flushPromises()

    let metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['1', '0', '1'])
    expect(wrapper.text()).toContain('Legacy open')
    expect(wrapper.text()).toContain('Legacy closed')

    await wrapper.vm.load()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['0', '1', '0'])
    expect(wrapper.text()).toContain('Panel answered')
    expect(wrapper.text()).not.toContain('Legacy open')

    wrapper.unmount()
  })

  describe('dialogs and feedback', () => {
    const Harness = { components: { Tickets, UiHost }, template: '<div><Tickets /><UiHost /></div>' }

    async function renderPage() {
      adminApi.getTickets.mockResolvedValue({ data: [{ id: 9, user_id: 3, subject: 'Slow node', level: 1, status: 0, created_at: 1783526400 }] })
      render(Harness)
      await screen.findByText('Slow node')
    }

    it('replies in a side sheet with inline validation and errors', async () => {
      const user = userEvent.setup()
      adminApi.replyTicket.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ code: 0 })
      await renderPage()
      const opener = screen.getByRole('button', { name: 'Reply' })
      await user.click(opener)
      const sheet = await screen.findByRole('dialog', { name: 'Reply to Ticket #9' })
      await user.click(within(sheet).getByRole('button', { name: 'Send Reply' }))
      expect(within(sheet).getByRole('alert').textContent).toBe('Please enter a reply')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))

      await user.click(opener)
      const again = await screen.findByRole('dialog')
      await user.type(within(again).getByLabelText('Reply content'), 'Fixed')
      await user.click(within(again).getByRole('button', { name: 'Send Reply' }))
      expect((await within(again).findByRole('alert')).textContent).toBe('offline')
      await user.click(within(again).getByRole('button', { name: 'Send Reply' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.replyTicket).toHaveBeenLastCalledWith({ ticket_id: 9, message: 'Fixed' })
      expect(toastMessages('success')).toEqual(['Reply sent successfully'])
    })

    it('asks before closing a ticket', async () => {
      const user = userEvent.setup()
      adminApi.closeTicket.mockResolvedValue({ code: 0 })
      await renderPage()
      await user.click(screen.getByRole('button', { name: 'Close' }))
      let confirm = await screen.findByRole('alertdialog', { name: 'Close ticket #9 “Slow node”?' })
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.closeTicket).not.toHaveBeenCalled()

      await user.click(screen.getByRole('button', { name: 'Close' }))
      confirm = await screen.findByRole('alertdialog')
      await user.click(within(confirm).getByRole('button', { name: 'Close ticket' }))
      await waitFor(() => expect(adminApi.closeTicket).toHaveBeenCalledWith(9))
      await waitFor(() => expect(toastMessages('success')).toEqual(['Ticket #9 closed']))
    })
  })
})
