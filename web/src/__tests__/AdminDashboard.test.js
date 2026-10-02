import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import Dashboard from '@/views/admin/Dashboard.vue'
import { setEdition } from '@/composables/useEdition'

const adminApi = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  getTrafficHourly: vi.fn(),
  getTickets: vi.fn(),
  getNodes: vi.fn(),
  getSystemAuditLogs: vi.fn()
}))

const engine = vi.hoisted(() => ({
  charts: [],
  registerTheme: vi.fn(),
  init: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/ui/internal/echarts.js', () => ({ init: engine.init, registerTheme: engine.registerTheme }))

const NOW = Math.floor(Date.now() / 1000)
const HOUR = Math.floor(NOW / 3600) * 3600

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

const series = Array.from({ length: 24 }, (_, index) => ({ hour_ts: HOUR - (23 - index) * 3600, traffic: index === 20 ? 4096 : 1024 }))

function mountDashboard() {
  return mount(Dashboard, { global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('Admin Dashboard', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    engine.charts = []
    engine.init.mockImplementation(() => {
      const chart = { setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() }
      engine.charts.push(chart)
      return chart
    })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getDashboard.mockResolvedValue({ data: dashboardStats })
    adminApi.getTrafficHourly.mockResolvedValue({ code: 0, data: { list: series, meta: { latest_log_at: HOUR } } })
    adminApi.getTickets.mockResolvedValue({
      code: 0,
      data: [
        { id: 1, subject: 'Cannot connect', status: 0, created_at: NOW - 7200 },
        { id: 2, subject: 'Refund', status: 0, created_at: NOW - 600 },
        { id: 3, subject: 'Thanks', status: 1, created_at: NOW - 100 },
        { id: 4, subject: 'Closed', status: 2, created_at: NOW - 100 }
      ]
    })
    adminApi.getNodes.mockResolvedValue({
      code: 0,
      data: {
        list: [
          { id: 7, name: 'hk-01', status: 1, last_check_at: NOW - 30 },
          { id: 8, name: 'tokyo-02', status: 2, last_check_at: NOW - 3600 }
        ],
        total: 2
      }
    })
    adminApi.getSystemAuditLogs.mockResolvedValue({
      data: { data: { list: [{ id: 91, action: 'delete', module: 'nodes', username: 'root', content: 'Deleted node sg-03', status: 'success', created_at: NOW - 120 }], total: 1 } }
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads every block from the existing endpoints', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(adminApi.getDashboard).toHaveBeenCalledWith(false)
    expect(adminApi.getTrafficHourly).toHaveBeenCalledWith(24, 0)
    expect(adminApi.getTickets).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodes).toHaveBeenCalledWith({ page: 1, page_size: 200 })
    expect(adminApi.getSystemAuditLogs).toHaveBeenCalledWith({ page: 1, page_size: 6 })
    wrapper.unmount()
  })

  it('shows the four metric cards of the dashboard template', async () => {
    setEdition('community')
    const wrapper = mountDashboard()
    await flushPromises()

    const cards = wrapper.findAll('[data-metric]')
    expect(cards.map(card => card.attributes('data-metric'))).toEqual(['users', 'nodes', 'traffic', 'tickets'])
    const value = key => wrapper.get(`[data-metric="${key}"] [data-metric-value]`).text()
    expect(value('users')).toBe('42')
    expect(wrapper.get('[data-metric="users"] [data-metric-trend]').text()).toBe('+3 today')
    expect(value('nodes')).toBe('4 / 5')
    expect(value('traffic')).toBe('2.0 KB')
    expect(wrapper.get('[data-metric="traffic"]').text()).toContain('4.00 KB in total')
    expect(wrapper.find('[data-metric="traffic"] [data-metric-sparkline]').exists()).toBe(true)
    expect(value('tickets')).toBe('2')
    expect(wrapper.get('[data-metric="tickets"]').text()).toContain('1 answered')
    expect(wrapper.findAllComponents(RouterLinkStub).map(link => link.props('to'))).toContain('/admin/tickets')
    expect(wrapper.text()).not.toContain('¥')
    wrapper.unmount()
  })

  it('draws the 24-hour traffic chart with an accessible summary and table', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(engine.init).toHaveBeenCalledTimes(1)
    const option = engine.charts[0].setOption.mock.calls[0][0]
    expect(option.series[0].data).toHaveLength(24)
    expect(option.series[0].type).toBe('line')
    const plot = wrapper.get('[data-dashboard-traffic] [data-chart-canvas]')
    expect(plot.attributes('role')).toBe('img')
    expect(plot.attributes('aria-label')).toContain('27.00 KB in total, peak 4.00 KB')
    expect(wrapper.findAll('[data-dashboard-traffic] [data-chart-table] tbody tr')).toHaveLength(24)
    wrapper.unmount()
  })

  it('lists what needs attention and the recent audit log', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    const alerts = wrapper.get('[data-dashboard-alerts]')
    expect(alerts.find('[data-alert="node-8"]').text()).toContain('tokyo-02 is offline')
    expect(alerts.find('[data-alert="node-7"]').exists()).toBe(false)
    expect(alerts.find('[data-alert="tickets"]').text()).toContain('2 tickets waiting for a reply')
    expect(alerts.find('[data-alert="orders"]').exists()).toBe(false)
    expect(alerts.find('[data-alert="traffic-stale"]').exists()).toBe(false)

    const activity = wrapper.get('[data-dashboard-activity]')
    expect(activity.text()).toContain('root')
    expect(activity.text()).toContain('Deleted node sg-03')
    wrapper.unmount()
  })

  it('says all clear when nothing needs attention and flags stalled traffic reports', async () => {
    adminApi.getTickets.mockResolvedValue({ code: 0, data: [] })
    adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [{ id: 7, name: 'hk-01', status: 1 }] } })
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-dashboard-alerts]').text()).toContain('All clear')
    wrapper.unmount()

    adminApi.getTrafficHourly.mockResolvedValue({ code: 0, data: { list: [], meta: { latest_log_at: NOW - 5 * 3600 } } })
    const stale = mountDashboard()
    await flushPromises()
    expect(stale.get('[data-alert="traffic-stale"]').text()).toContain('Traffic reports have stopped')
    expect(stale.get('[data-dashboard-traffic]').text()).toContain('No traffic in the last 24 hours')
    stale.unmount()
  })

  it('keeps revenue and orders for the commercial edition', async () => {
    setEdition('commercial')
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.get('[data-metric="revenue"] [data-metric-value]').text()).toBe('¥123.45')
    expect(wrapper.get('[data-metric="orders"] [data-metric-value]').text()).toBe('6')
    expect(wrapper.get('[data-alert="orders"]').text()).toContain('6 orders awaiting payment')
    wrapper.unmount()
  })

  it('reads the panel envelope and refreshes the cache on 刷新', async () => {
    adminApi.getDashboard.mockResolvedValue({ code: 0, msg: '操作成功', data: dashboardStats, ts: 1783526400000 })
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[data-metric="users"] [data-metric-value]').text()).toBe('42')

    await wrapper.get('[data-dashboard-refresh]').trigger('click')
    await flushPromises()
    expect(adminApi.getDashboard).toHaveBeenLastCalledWith(true)
    expect(adminApi.getTrafficHourly).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows an error with 重试 when the dashboard does not load, and a block error on its own', async () => {
    adminApi.getDashboard.mockRejectedValueOnce(new Error('dashboard down'))
    adminApi.getSystemAuditLogs.mockRejectedValue(new Error('audit down'))
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.get('[data-error-state]').text()).toContain('Couldn’t load the dashboard')
    expect(wrapper.text()).toContain('dashboard down')
    await wrapper.get('[data-error-retry]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-metric="users"] [data-metric-value]').text()).toBe('42')
    expect(wrapper.get('[data-dashboard-activity]').text()).toContain('Couldn’t load the audit log')
    wrapper.unmount()
  })
})
