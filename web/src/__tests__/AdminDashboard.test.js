import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Dashboard from '@/views/admin/Dashboard.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  getDashboard: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const dashboardStats = {
  total_users: 42,
  today_new_users: 3,
  active_users: 27,
  expired_users: 4,
  banned_users: 2,
  total_nodes: 5,
  active_nodes: 4,
  online_users: 19,
  monthly_income: 12345,
  today_income: 678,
  total_revenue: 98765,
  total_orders: 17,
  pending_orders: 6,
  paid_orders: 11,
  total_traffic_used: 4096,
  today_traffic: 2048,
  cached_at: '2026-07-09T00:00:00Z'
}

describe('Admin Dashboard', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders legacy dashboard payloads', async () => {
    adminApi.getDashboard.mockResolvedValue({ data: dashboardStats })

    const wrapper = mount(Dashboard)
    await flushPromises()

    expect(adminApi.getDashboard).toHaveBeenCalledWith(false)
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('27')
    expect(wrapper.text()).toContain('¥123.45')
    expect(wrapper.text()).toContain('4.00 KB')

    wrapper.unmount()
  })

  it('renders panel envelope dashboard payloads', async () => {
    adminApi.getDashboard.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: dashboardStats,
      ts: 1783526400000
    })

    const wrapper = mount(Dashboard)
    await flushPromises()

    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('27')
    expect(wrapper.text()).toContain('¥123.45')
    expect(wrapper.text()).toContain('4.00 KB')

    wrapper.unmount()
  })
})
