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
        call: () => adminApi.getTrafficHourly(168, 7),
        expected: { url: '/admin/traffic/hourly', method: 'get', params: { hours: 168, user_id: 7 } },
      },
      {
        call: () => adminApi.getUserTrafficRanking(168, 500, true),
        expected: {
          url: '/admin/traffic/user-ranking',
          method: 'get',
          params: { hours: 168, limit: 500, include_zero_users: 'true' },
        },
      },
      {
        call: () => adminApi.getUserList({ page: 1 }),
        expected: { url: '/admin/users', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getAdminUser(12),
        expected: { url: '/admin/users/12', method: 'get' },
      },
      {
        call: () => adminApi.getUserStats(),
        expected: { url: '/admin/users/stats', method: 'get' },
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
        call: () => adminApi.getNode(5),
        expected: { url: '/admin/nodes/5', method: 'get' },
      },
      {
        call: () => adminApi.getNodeStats(),
        expected: { url: '/admin/nodes/stats', method: 'get' },
      },
      {
        call: () => adminApi.getNodeLogs(5, { page: 1 }),
        expected: { url: '/admin/nodes/5/logs', method: 'get', params: { page: 1 } },
      },
      {
        call: () => adminApi.getNodeCredentials(5),
        expected: { url: '/admin/nodes/5/credentials', method: 'get' },
      },
      {
        call: () => adminApi.getNodeProtocols(5),
        expected: { url: '/admin/nodes/5/protocols', method: 'get' },
      },
      {
        call: () => adminApi.getProtocolTemplates(),
        expected: { url: '/admin/protocol-templates', method: 'get' },
      },
      {
        call: () => adminApi.getAuthKeys(),
        expected: { url: '/admin/auth-keys', method: 'get' },
      },
      {
        call: () => adminApi.generateAuthKey({ name: 'tokyo', expire_days: 0 }),
        expected: { url: '/admin/auth-keys', method: 'post', data: { name: 'tokyo', expire_days: 0 } },
      },
      {
        call: () => adminApi.getSubscriptionGroups(),
        expected: { url: '/admin/subscription/groups', method: 'get' },
      },
      {
        call: () => adminApi.getSubscriptionTemplates(7),
        expected: { url: '/admin/subscription/groups/7/templates', method: 'get' },
      },
      {
        call: () => adminApi.getSubscriptionProtocols(7),
        expected: { url: '/admin/subscription/groups/7/protocols', method: 'get' },
      },
      {
        call: () => adminApi.getAvailableProtocols(),
        expected: { url: '/admin/subscription/protocols/available', method: 'get' },
      },
      {
        call: () => adminApi.getSubscriptionStats(),
        expected: { url: '/admin/subscription/stats', method: 'get' },
      },
      {
        call: () => adminApi.getAgents(),
        expected: { url: '/admin/agent/list', method: 'get' },
      },
      {
        call: () => adminApi.listAgentDiagnosticTasks({ limit: 50 }),
        expected: { url: '/admin/agent/tasks', method: 'get', params: { limit: 50 } },
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
        call: () => adminApi.getInviteConfig(),
        expected: { url: '/admin/invite/config', method: 'get' },
      },
      {
        call: () => adminApi.getInviteStats(),
        expected: { url: '/admin/invite/stats', method: 'get' },
      },
      {
        call: () => adminApi.getWithdrawals({ status: 'pending' }),
        expected: { url: '/admin/invite/withdrawals', method: 'get', params: { status: 'pending' } },
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
        call: () => adminApi.getTelegramUsers({ all: true }),
        expected: { url: '/admin/telegram/users', method: 'get', params: { all: true } },
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
        call: () => adminApi.getNotificationLogs({ status: 'failed' }),
        expected: {
          url: '/admin/notification/logs',
          method: 'get',
          params: { status: 'failed' },
        },
      },
      {
        call: () => adminApi.getEmailConfig(),
        expected: { url: '/admin/notification/email/config', method: 'get' },
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
      {
        call: () => adminApi.getSubscriptionSettings(),
        expected: { url: '/admin/system/subscription-settings', method: 'get' },
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
        call: () => adminApi.createUser({ email: 'new@example.com' }),
        expected: {
          url: '/admin/users',
          method: 'post',
          data: { email: 'new@example.com' },
        },
      },
      {
        call: () => adminApi.banUser(12),
        expected: {
          url: '/admin/users/12/ban',
          method: 'post',
        },
      },
      {
        call: () => adminApi.unbanUser(12),
        expected: {
          url: '/admin/users/12/unban',
          method: 'post',
        },
      },
      {
        call: () => adminApi.resetUserSubscribe(12),
        expected: {
          url: '/admin/users/12/reset-subscribe',
          method: 'post',
        },
      },
      {
        call: () => adminApi.resetUserTraffic(12),
        expected: {
          url: '/user/reset',
          method: 'post',
          data: { id: 12, type: 1 },
        },
      },
      {
        call: () => adminApi.markOrderPaid(3),
        expected: {
          url: '/admin/orders/3/paid',
          method: 'post',
        },
      },
      {
        call: () => adminApi.cancelOrder(3),
        expected: {
          url: '/admin/orders/3/cancel',
          method: 'post',
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
        call: () => adminApi.createPlan({ name: 'Starter' }),
        expected: {
          url: '/admin/plans',
          method: 'post',
          data: { name: 'Starter' },
        },
      },
      {
        call: () => adminApi.updatePlan(6, { name: 'Pro' }),
        expected: {
          url: '/admin/plans/6',
          method: 'put',
          data: { name: 'Pro' },
        },
      },
      {
        call: () => adminApi.deletePlan(6),
        expected: {
          url: '/admin/plans/6',
          method: 'delete',
        },
      },
      {
        call: () => adminApi.assignPlanToUser(6, { user_id: 12 }),
        expected: {
          url: '/admin/plans/6/assign',
          method: 'post',
          data: { user_id: 12 },
        },
      },
      {
        call: () => adminApi.createNode({ name: 'edge' }),
        expected: {
          url: '/admin/nodes',
          method: 'post',
          data: { name: 'edge' },
        },
      },
      {
        call: () => adminApi.deleteNode(5),
        expected: {
          url: '/admin/nodes/5',
          method: 'delete',
        },
      },
      {
        call: () => adminApi.syncNodeProtocol(5),
        expected: {
          url: '/admin/nodes/5/sync',
          method: 'post',
        },
      },
      {
        call: () => adminApi.createNodeProtocol(5, { type: 'vless' }),
        expected: {
          url: '/admin/nodes/5/protocols',
          method: 'post',
          data: { type: 'vless' },
        },
      },
      {
        call: () => adminApi.updateNodeProtocol(5, 9, { type: 'trojan' }),
        expected: {
          url: '/admin/nodes/5/protocols/9',
          method: 'put',
          data: { type: 'trojan' },
        },
      },
      {
        call: () => adminApi.deleteNodeProtocol(5, 9),
        expected: {
          url: '/admin/nodes/5/protocols/9',
          method: 'delete',
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
        call: () => adminApi.createSubscriptionGroup({ name: 'VIP' }),
        expected: {
          url: '/admin/subscription/groups',
          method: 'post',
          data: { name: 'VIP' },
        },
      },
      {
        call: () => adminApi.updateSubscriptionGroup(7, { name: 'VIP+' }),
        expected: {
          url: '/admin/subscription/groups/7',
          method: 'put',
          data: { name: 'VIP+' },
        },
      },
      {
        call: () => adminApi.deleteSubscriptionGroup(7),
        expected: {
          url: '/admin/subscription/groups/7',
          method: 'delete',
        },
      },
      {
        call: () => adminApi.createSubscriptionTemplate(7, { name: 'HK' }),
        expected: {
          url: '/admin/subscription/groups/7/templates',
          method: 'post',
          data: { name: 'HK' },
        },
      },
      {
        call: () => adminApi.updateSubscriptionTemplate(9, { name: 'HK+' }),
        expected: {
          url: '/admin/subscription/templates/9',
          method: 'put',
          data: { name: 'HK+' },
        },
      },
      {
        call: () => adminApi.deleteSubscriptionTemplate(9),
        expected: {
          url: '/admin/subscription/templates/9',
          method: 'delete',
        },
      },
      {
        call: () => adminApi.previewSubscription({ user_id: 1, format: 'v2ray' }),
        expected: {
          url: '/admin/subscription/preview',
          method: 'post',
          data: { user_id: 1, format: 'v2ray' },
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
        call: () => adminApi.updateTelegramBot({ token: 'bot-token' }),
        expected: {
          url: '/admin/telegram/bot',
          method: 'put',
          data: { token: 'bot-token' },
        },
      },
      {
        call: () => adminApi.setTelegramWebhook('https://panel.example.com/api/v2/telegram/webhook'),
        expected: {
          url: '/admin/telegram/webhook',
          method: 'post',
          data: { url: 'https://panel.example.com/api/v2/telegram/webhook' },
        },
      },
      {
        call: () => adminApi.deleteTelegramWebhook(),
        expected: {
          url: '/admin/telegram/webhook',
          method: 'delete',
        },
      },
      {
        call: () => adminApi.broadcastTelegram('hello'),
        expected: {
          url: '/admin/telegram/broadcast',
          method: 'post',
          data: { message: 'hello' },
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
        call: () => adminApi.updateEmailConfig({ host: 'smtp.example.com' }),
        expected: {
          url: '/admin/notification/email/config',
          method: 'put',
          data: { host: 'smtp.example.com' },
        },
      },
      {
        call: () => adminApi.updateInviteConfig({ enabled: true }),
        expected: {
          url: '/admin/invite/config',
          method: 'put',
          data: { enabled: true },
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
      {
        call: () => adminApi.executeAgentCommand({ node_id: 1, action: 'service_status' }),
        expected: {
          url: '/admin/agent/execute',
          method: 'post',
          data: { node_id: 1, action: 'service_status' },
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

  // axios and the interceptors load with the first request.
  const { loadRequestClient } = await vi.importActual('@/utils/request')
  await loadRequestClient()

  return {
    requestInterceptors,
    responseInterceptors,
    logout,
  }
}

describe('admin v4 api (/api/v4/admin)', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ data: {} })
  })

  it('sorts the three lists on the server by passing sort and order through', async () => {
    await adminApi.getUserList({ page: 1, page_size: 20, sort: 'traffic', order: 'desc' })
    expect(request).toHaveBeenLastCalledWith({ url: '/admin/users', method: 'get', params: { page: 1, page_size: 20, sort: 'traffic', order: 'desc' } })
    await adminApi.getOrderList({ sort: 'total_amount', order: 'asc' })
    expect(request).toHaveBeenLastCalledWith({ url: '/admin/orders', method: 'get', params: { sort: 'total_amount', order: 'asc' } })
    await adminApi.getNodes({ sort: 'last_check_at', order: 'desc' })
    expect(request).toHaveBeenLastCalledWith({ url: '/admin/nodes', method: 'get', params: { sort: 'last_check_at', order: 'desc' } })
  })

  it('asks for the last online of a page of users with their ids, deduplicated, and answers its list', async () => {
    request.mockResolvedValueOnce({ data: { users: [{ user_id: 3, last_online_at: 1760000000 }, { user_id: 5, last_online_at: null }] } })
    const rows = await adminApi.getUsersActivity([3, 5, 3, '8', 0, -1, 'x'])
    expect(request).toHaveBeenCalledTimes(1)
    expect(request).toHaveBeenLastCalledWith(expect.objectContaining({
      baseURL: '/api/v4',
      url: '/admin/users/activity',
      method: 'get',
      params: { ids: '3,5,8' }
    }))
    expect(rows).toEqual([{ user_id: 3, last_online_at: 1760000000 }, { user_id: 5, last_online_at: null }])
  })

  it('makes no request for no ids and tolerates an answer without a list', async () => {
    expect(await adminApi.getUsersActivity([])).toEqual([])
    expect(await adminApi.getUsersActivity(undefined)).toEqual([])
    expect(request).not.toHaveBeenCalled()
    request.mockResolvedValueOnce({ data: null })
    expect(await adminApi.getUsersActivity([1])).toEqual([])
  })

  it('posts one bulk request for the users, with a timeout over the server budget and the idempotency key', async () => {
    const answer = { action: 'reset_traffic', requested: 2, succeeded: 2, failed: 0, results: [{ id: 3, ok: true }, { id: 5, ok: true }] }
    request.mockResolvedValueOnce({ data: answer })
    expect(await adminApi.bulkUsers('reset_traffic', [3, 5, 3], { idempotencyKey: 'ub-1' })).toEqual(answer)
    expect(request).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/admin/users/bulk',
      method: 'post',
      data: { action: 'reset_traffic', ids: [3, 5] },
      timeout: 30_000,
      headers: { 'Idempotency-Key': 'ub-1' }
    })
    // The server's budget for a bulk request is 25 s; the client default is 5 s.
    expect(request.mock.calls[0][0].timeout).toBeGreaterThan(25_000)
  })

  it('sends no Idempotency-Key header unless asked, and bulk-revokes invite codes', async () => {
    await adminApi.bulkUsers('ban', [3])
    expect(request.mock.calls[0][0]).not.toHaveProperty('headers')
    expect(request.mock.calls[0][0].data).toEqual({ action: 'ban', ids: [3] })
    request.mockResolvedValueOnce({ data: { action: 'revoke', requested: 1, succeeded: 1, failed: 0, results: [{ id: 11, ok: true }] } })
    const answer = await adminApi.bulkInviteCodes('revoke', [11, 12])
    expect(request).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/admin/invite-codes/bulk',
      method: 'post',
      data: { action: 'revoke', ids: [11, 12] },
      timeout: 30_000
    })
    expect(answer.succeeded).toBe(1)
  })

  it('pages the members of a subscription group with search and status, leaving empty filters out', async () => {
    const page = { total: 1, page: 2, page_size: 20, members: [{ user_id: 3, email: 'a@example.test', active: true }] }
    request.mockResolvedValueOnce({ data: page })
    expect(await adminApi.getSubscriptionGroupMembers(4, { page: 2, pageSize: 20, q: 'a@', status: 'active' })).toEqual(page)
    expect(request).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/admin/subscription-groups/4/members',
      method: 'get',
      params: { page: 2, page_size: 20, q: 'a@', status: 'active' }
    })
    await adminApi.getSubscriptionGroupMembers(4)
    expect(request).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/admin/subscription-groups/4/members',
      method: 'get',
      params: { page: 1, page_size: 20 }
    })
  })

  it('sends the Telegram test with an empty body on the kernel route and marks it sensitive', async () => {
    request.mockResolvedValueOnce({ data: { class: 'ok', ok: true, message: 'Telegram accepted the test message.', target: 'self' } })
    const answer = await adminApi.testTelegramBot()
    expect(request).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/notifications/telegram/test', method: 'post', data: {}, sensitive: true })
    expect(answer.class).toBe('ok')
    request.mockResolvedValueOnce({ data: { class: 'ok', ok: true } })
    await adminApi.testTelegramBot(10001)
    expect(request.mock.calls.at(-1)[0].data).toEqual({ chat_id: 10001 })
  })

  it('reads the kernel alerts with their filters and a summary, tolerating a short answer', async () => {
    const page = { alerts: [{ id: 1, kind: 'ca_expiring' }], summary: { active: 1, critical: 0, warning: 1 } }
    request.mockResolvedValueOnce({ data: page })
    expect(await adminApi.getKernelAlerts({ status: 'all', kind: 'ca_expiring', severity: 'critical', limit: 20 })).toEqual(page)
    expect(request).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/alerts', method: 'get', params: { status: 'all', kind: 'ca_expiring', severity: 'critical', limit: 20 }, signal: undefined })
    request.mockResolvedValueOnce({ data: {} })
    expect(await adminApi.getKernelAlerts()).toEqual({ alerts: [], summary: { active: 0, critical: 0, warning: 0 } })
    expect(request.mock.calls.at(-1)[0].params).toEqual({ status: 'active' })
  })
})

describe('request auth handling', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

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

  it('logs a failed request that carries a credential without its body, and strips the body from the error', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { responseInterceptors } = await loadActualRequestModule()
    const error = {
      response: { status: 403 },
      config: { url: '/kernel/api-tokens', method: 'post', sensitive: true, data: '{"name":"probe","password":"correct horse","code":"123456"}' },
    }
    await expect(responseInterceptors.rejected(error)).rejects.toBe(error)
    expect(error.config.data).toBeUndefined()
    expect(logged).toHaveBeenCalledWith('Request error:', 'post', '/kernel/api-tokens', 403)
    expect(JSON.stringify(logged.mock.calls)).not.toMatch(/correct horse|123456/)

    // Any other failure is logged as before.
    logged.mockClear()
    const plain = { response: { status: 500 }, config: { url: '/admin/users', method: 'get' } }
    await expect(responseInterceptors.rejected(plain)).rejects.toBe(plain)
    expect(logged).toHaveBeenCalledWith('Request error:', plain)
    logged.mockRestore()
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
