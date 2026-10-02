import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Tickets from '@/views/user/Tickets.vue'
import UiHost from '@/ui/UiHost.vue'
import { inBody, toastMessages, toasts } from './helpers/feedback'

const mockGetTickets = vi.fn()
const mockCreateTicket = vi.fn()
const mockGetTicketDetail = vi.fn()
const mockReplyTicket = vi.fn()
const mockCloseTicket = vi.fn()

vi.mock('@/api/user', () => ({
  getTickets: (...args) => mockGetTickets(...args),
  createTicket: (...args) => mockCreateTicket(...args),
  getTicketDetail: (...args) => mockGetTicketDetail(...args),
  replyTicket: (...args) => mockReplyTicket(...args),
  closeTicket: (...args) => mockCloseTicket(...args),
}))

enableAutoUnmount(afterEach)

describe('User Tickets flow', () => {
  beforeEach(() => {
    mockGetTickets.mockReset()
    mockCreateTicket.mockReset()
    mockGetTicketDetail.mockReset()
    mockReplyTicket.mockReset()
    mockCloseTicket.mockReset()
  })

  it.each([
    [
      'legacy ticket payloads',
      {
        data: [
          {
            id: 1,
            subject: 'Cannot connect',
            status: 0,
            updated_at: 1710000000,
          },
        ],
      },
      {
        data: {
          id: 1,
          subject: 'Cannot connect',
          status: 0,
          messages: [
            {
              id: 10,
              is_admin: false,
              message: 'Need help',
              created_at: '2026-04-05T00:00:00.000Z',
            },
          ],
        },
      },
    ],
    [
      'panel ticket envelopes',
      {
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: [
          {
            id: 1,
            subject: 'Cannot connect',
            status: 0,
            updated_at: 1710000000,
          },
        ],
      },
      {
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: {
          id: 1,
          subject: 'Cannot connect',
          status: 0,
          messages: [
            {
              id: 10,
              is_admin: false,
              message: 'Need help',
              created_at: '2026-04-05T00:00:00.000Z',
            },
          ],
        },
      },
    ],
  ])('loads ticket list and opens ticket detail from %s', async (_label, listResponse, detailResponse) => {
    mockGetTickets.mockResolvedValue(listResponse)
    mockGetTicketDetail.mockResolvedValue(detailResponse)

    const wrapper = mount(Tickets, { attachTo: document.body })
    await flushPromises()

    expect(mockGetTickets).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.ticket-card')).toHaveLength(1)

    await wrapper.find('.ticket-card').trigger('click')
    await flushPromises()

    expect(mockGetTicketDetail).toHaveBeenCalledWith(1)
    expect(inBody('.ticket-detail-sheet').exists()).toBe(true)
    expect(inBody('.ticket-detail-sheet').text()).toContain('Need help')
  })

  it('creates a ticket and refreshes list', async () => {
    mockGetTickets.mockResolvedValue({ data: [] })
    mockCreateTicket.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: { id: 2 },
    })

    const wrapper = mount(Tickets, { attachTo: document.body })
    await flushPromises()

    await wrapper.find('[data-test="ticket-create-button"]').trigger('click')
    await flushPromises()
    await inBody('#ticket-subject').setValue('Billing issue')
    await inBody('#ticket-message').setValue('Please check order status')
    await inBody('[data-test="ticket-submit-button"]').trigger('click')
    await flushPromises()

    expect(mockCreateTicket).toHaveBeenCalledWith({
      subject: 'Billing issue',
      level: 1,
      message: 'Please check order status',
    })
    expect(mockGetTickets).toHaveBeenCalledTimes(2)
    expect(toastMessages('success')).toEqual(['Ticket sent. We’ll reply soon.'])
  })

  it('keeps an incomplete or failed new ticket in the sheet with an inline error', async () => {
    mockGetTickets.mockResolvedValue({ data: [] })
    mockCreateTicket.mockRejectedValue({ response: { data: { message: 'Too many tickets' } } })

    const wrapper = mount(Tickets, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('[data-test="ticket-create-button"]').trigger('click')
    await flushPromises()
    await inBody('[data-test="ticket-submit-button"]').trigger('click')
    expect(inBody('[data-test="ticket-create-error"]').text()).toBe('Enter a subject and a message.')
    expect(mockCreateTicket).not.toHaveBeenCalled()

    await inBody('#ticket-subject').setValue('Billing issue')
    await inBody('#ticket-message').setValue('Please check')
    await inBody('[data-test="ticket-submit-button"]').trigger('click')
    await flushPromises()
    expect(inBody('[data-test="ticket-create-error"]').text()).toBe('Too many tickets')
    expect(inBody('[role="dialog"]').exists()).toBe(true)
    expect(toasts()).toHaveLength(0)
  })

  it('reports a ticket that cannot be opened in an error toast', async () => {
    mockGetTickets.mockResolvedValue({ data: [{ id: 1, subject: 'Cannot connect', status: 0 }] })
    mockGetTicketDetail.mockRejectedValue(new Error('offline'))
    const wrapper = mount(Tickets, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('.ticket-card').trigger('click')
    await flushPromises()
    expect(toastMessages('error')).toEqual(['The ticket couldn’t be opened. Try again later.'])
    expect(inBody('[role="dialog"]').exists()).toBe(false)
  })
})

describe('User Tickets close confirmation', () => {
  const Harness = { components: { Tickets, UiHost }, template: '<div><Tickets /><UiHost /></div>' }
  const ticket = { id: 5, subject: 'Slow node', status: 1, messages: [{ id: 1, is_admin: true, message: 'Fixed?' }] }

  beforeEach(() => {
    mockGetTickets.mockReset().mockResolvedValue({ data: [ticket] })
    mockGetTicketDetail.mockReset().mockResolvedValue({ data: ticket })
    mockCloseTicket.mockReset()
    mockReplyTicket.mockReset()
  })

  async function openDetail(user) {
    render(Harness)
    await user.click(await screen.findByText('Slow node'))
    return screen.findByRole('dialog', { name: 'Slow node' })
  }

  it('asks before closing; Cancel and Esc keep the ticket open', async () => {
    const user = userEvent.setup()
    const sheet = await openDetail(user)
    await user.click(within(sheet).getByRole('button', { name: 'Close ticket' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Close the ticket “Slow node”?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await user.click(within(sheet).getByRole('button', { name: 'Close ticket' }))
    confirm = await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(mockCloseTicket).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', { name: 'Slow node' })).toBeTruthy()
  })

  it('closes the ticket and the sheet after the confirmation', async () => {
    const user = userEvent.setup()
    mockCloseTicket.mockResolvedValue({ data: true })
    const sheet = await openDetail(user)
    await user.click(within(sheet).getByRole('button', { name: 'Close ticket' }))
    const confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Close ticket' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(mockCloseTicket).toHaveBeenCalledWith(5)
    expect(toastMessages('success')).toEqual(['Ticket closed'])
  })

  it('shows a failed reply inline', async () => {
    const user = userEvent.setup()
    mockReplyTicket.mockRejectedValue(new Error('offline'))
    const sheet = await openDetail(user)
    await user.type(within(sheet).getByLabelText('Reply'), 'Still slow')
    await user.click(within(sheet).getByRole('button', { name: 'Send reply' }))
    expect((await within(sheet).findByRole('alert')).textContent).toBe('Submit failed')
    expect(toasts()).toHaveLength(0)
  })
})
