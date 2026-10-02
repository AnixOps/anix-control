import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { setLocale } from '@/i18n'
import Agent from '@/views/admin/Agent.vue'
import Coupons from '@/views/admin/Coupons.vue'
import InviteCodes from '@/views/admin/InviteCodes.vue'
import Orders from '@/views/admin/Orders.vue'
import Tickets from '@/views/admin/Tickets.vue'
import Users from '@/views/admin/Users.vue'

// The U6 list pages keep their filters in the URL query (plan §9): a reload
// or a shared link shows the same list; changes replace the history entry.
const adminApi = vi.hoisted(() => ({
  assignAdminUserTunnel: vi.fn(),
  banUser: vi.fn(),
  cancelOrder: vi.fn(),
  closeTicket: vi.fn(),
  createAgentTask: vi.fn(),
  createCoupon: vi.fn(),
  createUser: vi.fn(),
  deleteCoupon: vi.fn(),
  executeAgentCommand: vi.fn(),
  generateInviteCodes: vi.fn(),
  getAdminUser: vi.fn(),
  getAdminUserTunnelList: vi.fn(),
  getAgents: vi.fn(),
  getCoupons: vi.fn(),
  getForwardTunnels: vi.fn(),
  getInviteCodes: vi.fn(),
  getOrderList: vi.fn(),
  getOrderStats: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  getTickets: vi.fn(),
  getTrafficHourly: vi.fn(),
  getUserList: vi.fn(),
  getUserStats: vi.fn(),
  listAgentDiagnosticTasks: vi.fn(),
  markOrderPaid: vi.fn(),
  removeAdminUserTunnel: vi.fn(),
  replyTicket: vi.fn(),
  resetUserSubscribe: vi.fn(),
  resetUserTraffic: vi.fn(),
  resetUserTunnelTraffic: vi.fn(),
  revokeInviteCode: vi.fn(),
  unbanUser: vi.fn(),
  updateAdminUserTunnel: vi.fn(),
  updateUser: vi.fn()
}))

vi.mock('@/api/admin', () => ({ default: adminApi, ...adminApi }))

const Blank = { render: () => null }
let wrapper

async function mountAt(component, path, url) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path, component: Blank }] })
  await router.push(url)
  wrapper = mount(component, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { router, wrapper }
}

const queryOf = router => router.currentRoute.value.query

beforeEach(async () => {
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  await setLocale('en')
  adminApi.getUserList.mockResolvedValue({ data: { list: [], total: 0 } })
  adminApi.getUserStats.mockResolvedValue({ data: {} })
  adminApi.getSubscriptionGroups.mockResolvedValue({ data: [] })
  adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
  adminApi.getOrderList.mockResolvedValue({ code: 0, data: { list: [], total: 0 } })
  adminApi.getOrderStats.mockResolvedValue({ code: 0, data: {} })
  adminApi.getTickets.mockResolvedValue({ code: 0, data: [] })
  adminApi.getCoupons.mockResolvedValue({ code: 0, data: [] })
  adminApi.getInviteCodes.mockResolvedValue({ code: 0, data: { list: [], total: 0 } })
  adminApi.getAgents.mockResolvedValue({ code: 0, data: [] })
  adminApi.listAgentDiagnosticTasks.mockResolvedValue({ code: 0, data: [] })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('list filters in the URL query', () => {
  it('Users: loads the search, status and page from the URL and writes changes back', async () => {
    const { router, wrapper } = await mountAt(Users, '/admin/users', '/admin/users?q=lin&status=banned&page=3')
    expect(adminApi.getUserList).toHaveBeenLastCalledWith({ page: 3, page_size: 20, email: 'lin', status: 'banned' })
    expect(wrapper.vm.statusFilter).toBe('banned')

    wrapper.vm.statusFilter = 'active'
    await flushPromises()
    expect(adminApi.getUserList).toHaveBeenLastCalledWith({ page: 1, page_size: 20, email: 'lin', status: 'active' })
    expect(queryOf(router)).toEqual({ q: 'lin', status: 'active' })

    wrapper.vm.clearFilters()
    await flushPromises()
    expect(queryOf(router)).toEqual({})
  })

  it('Users: the command palette ?email= becomes the search and leaves the URL', async () => {
    const { router } = await mountAt(Users, '/admin/users', '/admin/users?email=lin%40example.com&page=4')
    expect(adminApi.getUserList).toHaveBeenLastCalledWith({ page: 1, page_size: 20, email: 'lin@example.com', status: '' })
    expect(queryOf(router)).toEqual({ q: 'lin@example.com' })
  })

  it('Users: the client-side 流量用尽 chip is kept in the URL too', async () => {
    const { router } = await mountAt(Users, '/admin/users', '/admin/users?status=exhausted')
    expect(adminApi.getUserList).toHaveBeenLastCalledWith({ page: 1, page_size: 20, email: '', status: '' })
    expect(queryOf(router)).toEqual({ status: 'exhausted' })
  })

  it('Orders: restores the searches, status and page; ignores an unknown status', async () => {
    const { router, wrapper } = await mountAt(Orders, '/admin/orders', '/admin/orders?trade_no=2026&email=a%40b.c&status=9&page=2')
    expect(adminApi.getOrderList).toHaveBeenLastCalledWith({ page: 2, page_size: 20, trade_no: '2026', email: 'a@b.c', status: undefined })
    expect(queryOf(router)).toEqual({ trade_no: '2026', email: 'a@b.c', page: '2' })

    wrapper.vm.filters.status = '0'
    await flushPromises()
    expect(adminApi.getOrderList).toHaveBeenLastCalledWith({ page: 1, page_size: 20, trade_no: '2026', email: 'a@b.c', status: '0' })
    expect(queryOf(router)).toEqual({ trade_no: '2026', email: 'a@b.c', status: '0' })
  })

  it('Tickets: restores and writes the search and status chip', async () => {
    adminApi.getTickets.mockResolvedValue({ code: 0, data: [
      { id: 9, user_id: 3, subject: 'Slow node', level: 1, status: 0, created_at: 1783526400, updated_at: 1783526400 },
      { id: 11, user_id: 5, subject: 'Old issue', level: 2, status: 2, created_at: 1783526000, updated_at: 1783526000 }
    ] })
    const { router, wrapper } = await mountAt(Tickets, '/admin/tickets', '/admin/tickets?status=closed')
    expect(wrapper.vm.visibleTickets.map(ticket => ticket.id)).toEqual([11])

    wrapper.vm.statusFilter = ''
    wrapper.vm.search = ' slow '
    await flushPromises()
    expect(queryOf(router)).toEqual({ q: 'slow' })
  })

  it('Coupons: restores the search, type and page, and a filter change goes back to page 1', async () => {
    adminApi.getCoupons.mockResolvedValue({ code: 0, data: Array.from({ length: 45 }, (_, index) => ({
      id: index + 1, code: `CODE${index + 1}`, name: `Coupon ${index + 1}`, type: 1, value: 10, use_count: 0, limit_use: 10
    })) })
    const { router, wrapper } = await mountAt(Coupons, '/admin/coupons', '/admin/coupons?type=1&page=2')
    expect(wrapper.vm.typeFilter).toBe('1')
    expect(wrapper.vm.page).toBe(2)
    expect(wrapper.text()).toContain('CODE21')

    wrapper.vm.search = 'code4'
    await flushPromises()
    expect(queryOf(router)).toEqual({ q: 'code4', type: '1' })
  })

  it('Invite codes: restores the status chip and page and writes them back', async () => {
    const { router, wrapper } = await mountAt(InviteCodes, '/admin/invite-codes', '/admin/invite-codes?status=used&page=2')
    expect(adminApi.getInviteCodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, status: 'used' })

    wrapper.vm.filter = ''
    await flushPromises()
    expect(adminApi.getInviteCodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20 })
    expect(queryOf(router)).toEqual({})
  })

  it('Agents: restores the open tab and keeps the task history page in the URL', async () => {
    const { router, wrapper } = await mountAt(Agent, '/admin/agent', '/admin/agent?tab=tasks&page=2')
    expect(wrapper.vm.activeTab).toBe('tasks')
    expect(wrapper.vm.taskPage).toBe(2)

    wrapper.vm.activeTab = 'terminal'
    await nextTick()
    await flushPromises()
    expect(queryOf(router)).toEqual({ tab: 'terminal' })

    wrapper.vm.activeTab = 'agents'
    await flushPromises()
    expect(queryOf(router)).toEqual({})
  })
})
