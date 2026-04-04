import { describe, it, expect, beforeEach, vi } from 'vitest'
import request from '@/utils/request'
import * as adminApi from '@/api/admin'

vi.mock('@/utils/request', () => ({
  default: vi.fn(() => Promise.resolve({})),
}))

describe('admin api mapping', () => {
  beforeEach(() => {
    request.mockClear()
  })

  it('covers menu-related read endpoints', async () => {
    const cases = [
      {
        call: () => adminApi.getDashboard(),
        expected: { url: '/admin/dashboard', method: 'get', params: {} },
      },
      {
        call: () => adminApi.getUserList({ page: 1 }),
        expected: { url: '/admin/users', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getOrderList({ status: 0 }),
        expected: { url: '/admin/orders', method: 'get', params: { status: 0 } },
      },
      {
        call: () => adminApi.getTickets({ status: 0 }),
        expected: { url: '/admin/ticket', method: 'get', params: { status: 0 } },
      },
      {
        call: () => adminApi.getNodes({ page: 1 }),
        expected: { url: '/admin/nodes', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getSubscriptionGroups(),
        expected: { url: '/admin/subscription/groups', method: 'get' },
      },
      {
        call: () => adminApi.getForwardNodes({ type: 'relay' }),
        expected: {
          url: '/admin/forward/nodes',
          method: 'get',
          params: { type: 'relay' },
        },
      },
      {
        call: () => adminApi.getAgents(),
        expected: { url: '/admin/agent/list', method: 'get' },
      },
      {
        call: () => adminApi.getPlans(),
        expected: { url: '/admin/plans', method: 'get' },
      },
      {
        call: () => adminApi.getCoupons({ page: 1 }),
        expected: { url: '/admin/coupon', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getInviteStats(),
        expected: { url: '/admin/invite/stats', method: 'get' },
      },
      {
        call: () => adminApi.getPaymentGateways(),
        expected: { url: '/admin/payment/gateways', method: 'get' },
      },
      {
        call: () => adminApi.getTelegramBot(),
        expected: { url: '/admin/telegram/bot', method: 'get' },
      },
      {
        call: () => adminApi.getNotificationTemplates({ page: 1 }),
        expected: {
          url: '/admin/notification/templates',
          method: 'get',
          params: { page: 1 },
        },
      },
      {
        call: () => adminApi.getKnowledgeList({ page: 1 }),
        expected: { url: '/admin/knowledge', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getMFAConfig(),
        expected: { url: '/admin/mfa/config', method: 'get' },
      },
      {
        call: () => adminApi.getSystemConfigs({ group: 'email' }),
        expected: {
          url: '/admin/system/configs',
          method: 'get',
          params: { group: 'email' },
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

  it('formats key dynamic write endpoints correctly', async () => {
    const cases = [
      {
        call: () => adminApi.updateUser(12, { email: 'ops@example.com' }),
        expected: {
          url: '/admin/users/12',
          method: 'put',
          data: { email: 'ops@example.com' },
        },
      },
      {
        call: () => adminApi.updateOrderStatus(3, 2),
        expected: {
          url: '/admin/orders/3/status',
          method: 'put',
          data: { status: 2 },
        },
      },
      {
        call: () => adminApi.updateNode(5, { name: 'edge-5' }),
        expected: {
          url: '/admin/nodes/5',
          method: 'put',
          data: { name: 'edge-5' },
        },
      },
      {
        call: () => adminApi.updateGroupProtocols(7, [1, 2, 3]),
        expected: {
          url: '/admin/subscription/groups/7/protocols',
          method: 'post',
          data: { protocol_ids: [1, 2, 3] },
        },
      },
      {
        call: () => adminApi.toggleForwardRule(9, true),
        expected: {
          url: '/admin/forward/rules/9/toggle',
          method: 'post',
          data: { enabled: true },
        },
      },
      {
        call: () => adminApi.togglePaymentGateway(4, false),
        expected: {
          url: '/admin/payment/gateways/4/toggle',
          method: 'post',
          data: { enabled: false },
        },
      },
      {
        call: () => adminApi.updateTelegramUserNotify(16, { notify: true }),
        expected: {
          url: '/admin/telegram/users/16/notify',
          method: 'put',
          data: { notify: true },
        },
      },
      {
        call: () => adminApi.sendTestNotification({ email: 'test@example.com' }),
        expected: {
          url: '/admin/notification/test',
          method: 'post',
          data: { email: 'test@example.com' },
        },
      },
      {
        call: () => adminApi.processWithdrawal(8, { status: 1 }),
        expected: {
          url: '/admin/invite/withdrawals/8/process',
          method: 'post',
          data: { status: 1 },
        },
      },
      {
        call: () => adminApi.setSystemConfig('mail_host', { value: 'smtp.example.com' }),
        expected: {
          url: '/admin/system/configs/mail_host',
          method: 'put',
          data: { value: 'smtp.example.com' },
        },
      },
      {
        call: () => adminApi.createAgentTask({ node_id: 1, command: 'ping' }),
        expected: {
          url: '/admin/agent/tasks',
          method: 'post',
          data: { node_id: 1, command: 'ping' },
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
