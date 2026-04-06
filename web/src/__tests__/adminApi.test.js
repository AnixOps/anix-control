import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
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
        call: () => adminApi.getAdminForwardTunnelList(),
        expected: { url: '/admin/tunnel/list', method: 'post' },
      },
      {
        call: () => adminApi.getForwardTunnels(),
        expected: { url: '/tunnel/user/tunnel', method: 'post' },
      },
      {
        call: () => adminApi.diagnoseForward(77),
        expected: {
          url: '/forward/diagnose',
          method: 'post',
          data: { forwardId: 77 },
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
        call: () => adminApi.createForwardTunnel({ name: 'Tunnel A', type: 1 }),
        expected: {
          url: '/admin/tunnel/create',
          method: 'post',
          data: { name: 'Tunnel A', type: 1 },
        },
      },
      {
        call: () => adminApi.updateForwardTunnel({ id: 8, name: 'Tunnel B' }),
        expected: {
          url: '/admin/tunnel/update',
          method: 'post',
          data: { id: 8, name: 'Tunnel B' },
        },
      },
      {
        call: () => adminApi.deleteForwardTunnel(6),
        expected: {
          url: '/admin/tunnel/delete',
          method: 'post',
          data: { id: 6 },
        },
      },
      {
        call: () => adminApi.diagnoseForwardTunnel(11),
        expected: {
          url: '/admin/tunnel/diagnose',
          method: 'post',
          data: { tunnelId: 11 },
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

async function loadActualRequestModule({ token = '' } = {}) {
  vi.resetModules()

  const requestInterceptors = {}
  const responseInterceptors = {}
  const logout = vi.fn()

  vi.doMock('axios', () => ({
    default: {
      create: vi.fn(() => ({
        interceptors: {
          request: {
            use: vi.fn((fulfilled, rejected) => {
              requestInterceptors.fulfilled = fulfilled
              requestInterceptors.rejected = rejected
            }),
          },
          response: {
            use: vi.fn((fulfilled, rejected) => {
              responseInterceptors.fulfilled = fulfilled
              responseInterceptors.rejected = rejected
            }),
          },
        },
      })),
    },
  }))

  vi.doMock('@/stores/user', () => ({
    useUserStore: () => ({
      token,
      logout,
    }),
  }))

  await vi.importActual('@/utils/request')

  return {
    requestInterceptors,
    responseInterceptors,
    logout,
  }
}

describe('request auth handling', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.resetModules()
  })

  it('adds a Bearer authorization header when a token exists', async () => {
    const { requestInterceptors } = await loadActualRequestModule({ token: 'panel-token' })
    const config = await requestInterceptors.fulfilled({
      headers: {},
      url: '/admin/dashboard',
    })

    expect(config.headers.Authorization).toBe('Bearer panel-token')
  })

  it('clears auth state and redirects to login on stale 401 responses', async () => {
    window.history.replaceState({}, '', '/admin/dashboard')
    const replaceSpy = vi.spyOn(window.location, 'replace').mockImplementation(() => {})
    const { responseInterceptors, logout } = await loadActualRequestModule({ token: 'stale-token' })
    const error = {
      response: { status: 401 },
      config: { url: '/admin/users' },
    }

    await expect(responseInterceptors.rejected(error)).rejects.toBe(error)
    expect(logout).toHaveBeenCalledTimes(1)
    expect(replaceSpy).toHaveBeenCalledWith('/login')
  })

  it('does not force redirect on login endpoint 401 responses', async () => {
    window.history.replaceState({}, '', '/login')
    const replaceSpy = vi.spyOn(window.location, 'replace').mockImplementation(() => {})
    const { responseInterceptors, logout } = await loadActualRequestModule()
    const error = {
      response: { status: 401 },
      config: { url: '/login' },
    }

    await expect(responseInterceptors.rejected(error)).rejects.toBe(error)
    expect(logout).not.toHaveBeenCalled()
    expect(replaceSpy).not.toHaveBeenCalled()
  })
})
