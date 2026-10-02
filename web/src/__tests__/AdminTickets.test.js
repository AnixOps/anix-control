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

const Harness = { components: { Tickets, UiHost }, template: '<div><Tickets /><UiHost /></div>' }
const TICKETS = [
  { id: 9, user_id: 3, subject: 'Slow node', level: 1, status: 0, created_at: 1783526400, updated_at: 1783526400 },
  { id: 10, user_id: 4, subject: 'Billing question', level: 0, status: 1, created_at: 1783526500, updated_at: 1783526500 },
  { id: 11, user_id: 5, subject: 'Old issue', level: 2, status: 2, created_at: 1783526000, updated_at: 1783526000 }
]

async function renderPage(list = TICKETS) {
  adminApi.getTickets.mockResolvedValue({ code: 0, data: list })
  render(Harness)
  await screen.findByText('Slow node')
}

function queue() {
  return screen.getByRole('navigation', { name: 'Ticket queue' })
}

describe('Admin Tickets', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('lists tickets from legacy and panel envelope payloads with status counts', async () => {
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
        data: [{ id: 3, user_id: 12, subject: 'Panel answered', level: 0, status: 1, created_at: 1783526600 }],
        ts: 1783526400000
      })

    const wrapper = mount(Tickets)
    await flushPromises()
    const counts = () => wrapper.findAll('.ui-filter-chips__count').map(node => node.text())
    expect(counts()).toEqual(['1', '0', '1'])
    // Open tickets come first.
    expect(wrapper.findAll('.ticket-list__subject').map(node => node.text())).toEqual(['Legacy open', 'Legacy closed'])

    await wrapper.vm.load()
    await flushPromises()
    expect(counts()).toEqual(['0', '1', '0'])
    expect(wrapper.text()).toContain('Panel answered')
    expect(wrapper.text()).not.toContain('Legacy open')
    wrapper.unmount()
  })

  it('shows the empty state, and an error with retry', async () => {
    const user = userEvent.setup()
    adminApi.getTickets.mockRejectedValueOnce(new Error('bad gateway')).mockResolvedValueOnce({ code: 0, data: [] })
    render(Harness)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Tickets didn’t load')
    expect(alert.textContent).toContain('bad gateway')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { name: 'No tickets' })).toBeTruthy()
  })

  it('filters the queue by status and by search', async () => {
    const user = userEvent.setup()
    await renderPage()
    await user.click(within(screen.getByRole('group', { name: 'Filter by status' })).getByRole('button', { name: 'Answered 1' }))
    expect(within(queue()).getAllByRole('button').map(button => button.dataset.ticketId)).toEqual(['10'])

    await user.click(screen.getByRole('button', { name: 'Answered 1' }))
    await user.type(screen.getByRole('searchbox', { name: 'Search subject, #number or user ID' }), '#11')
    expect(within(queue()).getAllByRole('button').map(button => button.dataset.ticketId)).toEqual(['11'])

    await user.clear(screen.getByRole('searchbox'))
    await user.type(screen.getByRole('searchbox'), 'nothing like this')
    expect(within(queue()).getByRole('heading', { name: 'No tickets match' })).toBeTruthy()
    await user.click(within(queue()).getByRole('button', { name: 'Clear filters' }))
    expect(within(queue()).getAllByRole('button')).toHaveLength(3)
  })

  it('replies to the selected ticket with inline validation, quick replies and errors', async () => {
    const user = userEvent.setup()
    adminApi.replyTicket.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ code: 0 })
    await renderPage()
    expect(screen.getByText('Select a ticket to reply to it.')).toBeTruthy()
    await user.click(within(queue()).getByRole('button', { name: /Slow node/ }))
    const panel = screen.getByRole('region', { name: 'Slow node' })
    expect(within(queue()).getByRole('button', { name: /Slow node/ }).getAttribute('aria-current')).toBe('true')

    await user.click(within(panel).getByRole('button', { name: 'Send reply' }))
    expect(within(panel).getByRole('alert').textContent).toBe('Write a reply first')

    await user.click(within(panel).getByRole('button', { name: 'Ask for details' }))
    expect(within(panel).getByLabelText('Reply').value).toContain('which client and node')
    await user.click(within(panel).getByRole('button', { name: 'Send reply' }))
    expect((await within(panel).findByRole('alert')).textContent).toBe('offline')

    await user.click(within(panel).getByRole('button', { name: 'Send reply' }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Reply sent to ticket #9']))
    expect(adminApi.replyTicket).toHaveBeenLastCalledWith({ ticket_id: 9, message: expect.stringContaining('which client and node') })
    expect(within(panel).getByLabelText('Reply').value).toBe('')
  })

  it('asks before closing a ticket; a closed ticket takes no replies', async () => {
    const user = userEvent.setup()
    adminApi.closeTicket.mockResolvedValue({ code: 0 })
    await renderPage()
    await user.click(within(queue()).getByRole('button', { name: /Slow node/ }))
    await user.click(screen.getByRole('button', { name: 'Close ticket…' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Close ticket #9 “Slow node”?' })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.closeTicket).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Close ticket…' }))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Close ticket' }))
    await waitFor(() => expect(adminApi.closeTicket).toHaveBeenCalledWith(9))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Ticket #9 closed']))

    await user.click(within(queue()).getByRole('button', { name: /Old issue/ }))
    expect(screen.getByText('This ticket is closed and takes no more replies.')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Send reply' })).toBeNull()
  })

  it('shows the queue, then the ticket full width with a way back on phones', async () => {
    const user = userEvent.setup()
    vi.stubGlobal('matchMedia', query => ({ matches: query.includes('833.98'), media: query, addEventListener() {}, removeEventListener() {} }))
    await renderPage()
    expect(screen.queryByText('Select a ticket to reply to it.')).toBeNull()
    await user.click(within(queue()).getByRole('button', { name: /Slow node/ }))
    expect(screen.queryByRole('navigation', { name: 'Ticket queue' })).toBeNull()
    const title = screen.getByRole('heading', { level: 1, name: 'Slow node' })
    await waitFor(() => expect(document.activeElement).toBe(title))
    await user.click(screen.getByRole('button', { name: 'All tickets' }))
    expect(queue()).toBeTruthy()
  })
})
