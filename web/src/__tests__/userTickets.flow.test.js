import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Tickets from '@/views/user/Tickets.vue'

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

    const wrapper = mount(Tickets)
    await flushPromises()

    expect(mockGetTickets).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.ticket-card')).toHaveLength(1)

    await wrapper.find('.ticket-card').trigger('click')
    await flushPromises()

    expect(mockGetTicketDetail).toHaveBeenCalledWith(1)
    expect(wrapper.find('.ticket-detail-modal').exists()).toBe(true)
  })

  it('creates a ticket and refreshes list', async () => {
    mockGetTickets.mockResolvedValue({ data: [] })
    mockCreateTicket.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: { id: 2 },
    })

    const wrapper = mount(Tickets)
    await flushPromises()

    await wrapper.find('[data-test="ticket-create-button"]').trigger('click')
    await wrapper.find('.modal .form-group input').setValue('Billing issue')
    await wrapper.find('.modal .form-group textarea').setValue('Please check order status')
    await wrapper.find('[data-test="ticket-submit-button"]').trigger('click')
    await flushPromises()

    expect(mockCreateTicket).toHaveBeenCalledWith({
      subject: 'Billing issue',
      level: 1,
      message: 'Please check order status',
    })
    expect(mockGetTickets).toHaveBeenCalledTimes(2)
  })
})
