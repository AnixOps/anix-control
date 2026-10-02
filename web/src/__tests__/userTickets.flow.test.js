import { describe, it, expect, beforeEach, vi } from 'vitest'
import { reactive } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Tickets from '@/views/user/Tickets.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getTickets: vi.fn(),
  createTicket: vi.fn(),
  getTicketDetail: vi.fn(),
  replyTicket: vi.fn(),
  closeTicket: vi.fn(),
}))

vi.mock('@/api/user', () => api)

// A small in-memory router: the page keeps the open ticket and the new-ticket
// sheet in the query.
const route = vi.hoisted(() => ({ current: null }))
vi.mock('vue-router', () => ({
  useRoute: () => route.current,
  useRouter: () => ({
    replace: vi.fn(({ query }) => {
      route.current.query = Object.fromEntries(Object.entries(query).filter(([, value]) => value !== undefined))
    }),
  }),
}))

const media = vi.hoisted(() => ({ narrow: false }))
vi.mock('@/composables/useMediaQuery', async () => {
  const { ref } = await vi.importActual('vue')
  return { NARROW_QUERY: '(max-width: 833.98px)', useMediaQuery: () => ref(media.narrow) }
})

const LIST = [
  { id: 1, subject: 'Cannot connect', status: 0, level: 1, updated_at: 1710000000 },
  { id: 2, subject: 'Slow at night', status: 1, level: 2, updated_at: 1790000000 },
]

function detail(id = 1, status = 0) {
  return {
    id,
    subject: LIST.find(item => item.id === id).subject,
    status,
    level: 1,
    messages: [
      { id: 10, is_admin: 0, message: 'Need help', created_at: '2026-04-05T00:00:00.000Z' },
      { id: 11, is_admin: 1, message: 'Try the Tokyo node', created_at: '2026-04-05T01:00:00.000Z' },
    ],
  }
}

const Harness = {
  components: { Tickets, UiHost },
  template: '<div><Tickets /><UiHost /></div>'
}

async function renderPage(query = {}) {
  route.current = reactive({ query })
  const user = userEvent.setup()
  render(Harness)
  return { user }
}

describe('User Tickets flow', () => {
  beforeEach(() => {
    media.narrow = false
    for (const fn of Object.values(api)) fn.mockReset()
  })

  it.each([
    ['legacy payloads', { data: LIST }, { data: detail(1) }],
    ['panel envelopes', { code: 0, msg: '操作成功', data: LIST }, { code: 0, msg: '操作成功', data: detail(1) }],
  ])('lists tickets newest first and opens a conversation beside the list (%s)', async (_name, listBody, detailBody) => {
    api.getTickets.mockResolvedValue(listBody)
    api.getTicketDetail.mockResolvedValue(detailBody)
    const { user } = await renderPage()

    const list = await screen.findByRole('navigation', { name: 'My tickets' })
    const items = within(list).getAllByRole('button')
    expect(items.map(item => item.textContent)).toEqual([expect.stringContaining('Slow at night'), expect.stringContaining('Cannot connect')])
    expect(within(items[0]).getByText('Answered')).toBeTruthy()
    expect(screen.getByText('Select a ticket to read the conversation.')).toBeTruthy()

    await user.click(items[1])
    expect(route.current.query).toEqual({ ticket: '1' })
    const heading = await screen.findByRole('heading', { level: 2, name: 'Cannot connect' })
    expect(api.getTicketDetail).toHaveBeenCalledWith('1')
    await waitFor(() => expect(document.activeElement).toBe(heading))
    expect(items[1].getAttribute('aria-current')).toBe('true')
    const messages = screen.getByRole('list', { name: 'Cannot connect' })
    expect(within(messages).getAllByRole('listitem').map(item => item.querySelector('p').textContent)).toEqual(['Need help', 'Try the Tokyo node'])
    expect(within(messages).getByText('You')).toBeTruthy()
    expect(within(messages).getByText('Support')).toBeTruthy()
  })

  it('opens the ticket named in the query', async () => {
    api.getTickets.mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockResolvedValue({ data: detail(2, 1) })
    await renderPage({ ticket: '2' })
    expect(await screen.findByRole('heading', { level: 2, name: 'Slow at night' })).toBeTruthy()
  })

  it('shows the conversation full width on a phone, with a way back', async () => {
    media.narrow = true
    api.getTickets.mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockResolvedValue({ data: detail(1) })
    const { user } = await renderPage()
    await user.click(within(await screen.findByRole('navigation', { name: 'My tickets' })).getByText('Cannot connect'))
    // One H1 on the page: the ticket replaces the page header and the list.
    expect(await screen.findByRole('heading', { level: 1, name: 'Cannot connect' })).toBeTruthy()
    expect(screen.queryByRole('navigation', { name: 'My tickets' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'My tickets' }))
    expect(route.current.query).toEqual({})
    expect(await screen.findByRole('navigation', { name: 'My tickets' })).toBeTruthy()
  })

  it('replies with the button or Ctrl+Enter and refreshes the conversation', async () => {
    api.getTickets.mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockResolvedValue({ data: detail(1) })
    api.replyTicket.mockResolvedValue({ code: 0, data: null })
    const { user } = await renderPage({ ticket: '1' })
    const box = await screen.findByLabelText('Reply')

    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(await screen.findByText('Write a reply first.')).toBeTruthy()
    expect(api.replyTicket).not.toHaveBeenCalled()

    await user.type(box, 'Still broken')
    await user.keyboard('{Control>}{Enter}{/Control}')
    await waitFor(() => expect(api.replyTicket).toHaveBeenCalledWith(1, { message: 'Still broken' }))
    await waitFor(() => expect(box.value).toBe(''))
    expect(api.getTicketDetail).toHaveBeenCalledTimes(2)
  })

  it('keeps a failed reply in the box with the error', async () => {
    api.getTickets.mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockResolvedValue({ data: detail(1) })
    api.replyTicket.mockRejectedValue({ response: { data: { msg: 'ticket closed' } } })
    const { user } = await renderPage({ ticket: '1' })
    await user.type(await screen.findByLabelText('Reply'), 'Hello')
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(await screen.findByText('The reply was not sent: ticket closed')).toBeTruthy()
    expect(screen.getByLabelText('Reply').value).toBe('Hello')
  })

  it('asks before closing a ticket; Cancel keeps it open', async () => {
    api.getTickets.mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockResolvedValueOnce({ data: detail(1) }).mockResolvedValue({ data: detail(1, 2) })
    api.closeTicket.mockResolvedValue({ code: 0, data: null })
    const { user } = await renderPage({ ticket: '1' })

    await user.click(await screen.findByRole('button', { name: 'Close ticket…' }))
    let dialog = await screen.findByRole('alertdialog', { name: 'Close the ticket “Cannot connect”?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.closeTicket).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Close ticket…' }))
    dialog = await screen.findByRole('alertdialog', { name: 'Close the ticket “Cannot connect”?' })
    await user.click(within(dialog).getByRole('button', { name: 'Close ticket' }))
    await waitFor(() => expect(api.closeTicket).toHaveBeenCalledWith(1))
    expect(await screen.findByText('This ticket is closed. Open a new ticket if you need more help.')).toBeTruthy()
    expect(screen.queryByLabelText('Reply')).toBeNull()
    expect(toastMessages('success')).toEqual(['Ticket closed'])
  })

  it('creates a ticket in a sheet and opens it', async () => {
    api.getTickets.mockResolvedValueOnce({ data: [] }).mockResolvedValue({ data: [{ id: 9, subject: 'New one', status: 0, updated_at: 1790000001 }] })
    api.createTicket.mockResolvedValue({ code: 0, data: { id: 9, subject: 'New one' } })
    api.getTicketDetail.mockResolvedValue({ data: { id: 9, subject: 'New one', status: 0, level: 2, messages: [] } })
    const { user } = await renderPage()

    expect(await screen.findByRole('heading', { level: 2, name: 'No tickets yet' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'New ticket' }))
    const sheet = await screen.findByRole('dialog', { name: 'New ticket' })
    await waitFor(() => expect(document.activeElement).toBe(within(sheet).getByLabelText(/^Subject/)))

    await user.click(within(sheet).getByRole('button', { name: 'Send ticket' }))
    expect(within(sheet).getByText('Enter a subject.')).toBeTruthy()
    expect(within(sheet).getByText('Describe the problem.')).toBeTruthy()
    expect(api.createTicket).not.toHaveBeenCalled()

    await user.type(within(sheet).getByLabelText(/^Subject/), 'New one')
    await user.click(within(sheet).getByRole('button', { name: 'High' }))
    await user.type(within(sheet).getByLabelText(/^Details/), 'It broke')
    await user.click(within(sheet).getByRole('button', { name: 'Send ticket' }))
    await waitFor(() => expect(api.createTicket).toHaveBeenCalledWith({ subject: 'New one', level: 2, message: 'It broke' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(toastMessages('success')).toEqual(['Ticket sent. Support will reply soon.'])
    expect(route.current.query).toEqual({ ticket: '9' })
  })

  it('keeps a failed new ticket in the sheet with the error', async () => {
    api.getTickets.mockResolvedValue({ data: LIST })
    api.createTicket.mockResolvedValue({ code: 1, msg: '创建工单失败' })
    const { user } = await renderPage({ new: '1' })
    const sheet = await screen.findByRole('dialog', { name: 'New ticket' })
    await user.type(within(sheet).getByLabelText(/^Subject/), 'Help')
    await user.type(within(sheet).getByLabelText(/^Details/), 'Details')
    await user.click(within(sheet).getByRole('button', { name: 'Send ticket' }))
    expect((await within(sheet).findByRole('alert')).textContent).toBe('The ticket was not sent: 创建工单失败')
    expect(screen.getByRole('dialog', { name: 'New ticket' })).toBeTruthy()
  })

  it('shows a retryable error for the list and for a ticket that cannot be opened', async () => {
    api.getTickets.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ data: LIST })
    api.getTicketDetail.mockRejectedValueOnce({ response: { status: 500, data: { msg: 'boom' } } }).mockResolvedValue({ data: detail(1) })
    const { user } = await renderPage({ ticket: '1' })
    expect(await screen.findByRole('heading', { name: 'Could not load your tickets.' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { name: 'Could not open this ticket.' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('heading', { level: 2, name: 'Cannot connect' })).toBeTruthy()
  })
})
