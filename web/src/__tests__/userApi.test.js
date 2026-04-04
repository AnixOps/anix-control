import { describe, it, expect, beforeEach, vi } from 'vitest'
import request from '@/utils/request'
import * as userApi from '@/api/user'

vi.mock('@/utils/request', () => ({
  default: vi.fn(() => Promise.resolve({})),
}))

describe('user api mapping', () => {
  beforeEach(() => {
    request.mockClear()
  })

  it('covers user menu read endpoints', async () => {
    const cases = [
      {
        call: () => userApi.getDashboard(),
        expected: { url: '/user/dashboard', method: 'get' },
      },
      {
        call: () => userApi.getSubscription(),
        expected: { url: '/user/subscription', method: 'get', params: {} },
      },
      {
        call: () => userApi.getSubscription(true),
        expected: { url: '/user/subscription', method: 'get', params: { refresh: 'true' } },
      },
      {
        call: () => userApi.getKnowledgeList(),
        expected: { url: '/user/knowledge', method: 'get' },
      },
      {
        call: () => userApi.getTickets(),
        expected: { url: '/user/ticket', method: 'get' },
      },
      {
        call: () => userApi.getPlans(),
        expected: { url: '/user/plan', method: 'get' },
      },
      {
        call: () => userApi.getOrders({ status: 0 }),
        expected: { url: '/user/order', method: 'get', params: { status: 0 } },
      },
    ]

    for (const c of cases) {
      request.mockClear()
      await c.call()
      expect(request).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenCalledWith(c.expected)
    }
  })

  it('formats user write endpoints correctly', async () => {
    const cases = [
      {
        call: () => userApi.createTicket({ subject: 'help', level: 1 }),
        expected: {
          url: '/user/ticket',
          method: 'post',
          data: { subject: 'help', level: 1 },
        },
      },
      {
        call: () => userApi.replyTicket(8, { message: 'thanks' }),
        expected: {
          url: '/user/ticket/8/reply',
          method: 'post',
          data: { message: 'thanks' },
        },
      },
      {
        call: () => userApi.closeTicket(8),
        expected: {
          url: '/user/ticket/8/close',
          method: 'post',
        },
      },
      {
        call: () => userApi.checkCoupon({ code: 'HELLO' }),
        expected: {
          url: '/user/coupon/check',
          method: 'post',
          data: { code: 'HELLO' },
        },
      },
      {
        call: () => userApi.saveOrder({ plan_id: 2, period: 'month' }),
        expected: {
          url: '/user/order/save',
          method: 'post',
          data: { plan_id: 2, period: 'month' },
        },
      },
      {
        call: () => userApi.getOrderDetail(11),
        expected: {
          url: '/user/order/11',
          method: 'get',
        },
      },
      {
        call: () => userApi.getKnowledgeDetail(6),
        expected: {
          url: '/user/knowledge/6',
          method: 'get',
        },
      },
      {
        call: () => userApi.getTicketDetail(3),
        expected: {
          url: '/user/ticket/3',
          method: 'get',
        },
      },
    ]

    for (const c of cases) {
      request.mockClear()
      await c.call()
      expect(request).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenCalledWith(c.expected)
    }
  })
})
