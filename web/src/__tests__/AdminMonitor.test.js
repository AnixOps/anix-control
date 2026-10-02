import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import Monitor from '@/views/admin/Monitor.vue'
import MonitorForward from '@/views/admin/monitor/MonitorForward.vue'
import MonitorLatency from '@/views/admin/monitor/MonitorLatency.vue'
import MonitorLive from '@/views/admin/monitor/MonitorLive.vue'
import MonitorTraffic from '@/views/admin/monitor/MonitorTraffic.vue'

const adminApi = vi.hoisted(() => ({
  getTrafficHourly: vi.fn(),
  getUserTrafficRanking: vi.fn(),
  getForwardObservabilityTargets: vi.fn(),
  getForwardObservabilityTrend: vi.fn(),
  getForwardObservabilityTopology: vi.fn(),
  getForwardObservabilityMultiIngress: vi.fn(),
  listForwardRuntimeJobs: vi.fn()
}))

const engine = vi.hoisted(() => ({ charts: [], init: vi.fn(), registerTheme: vi.fn() }))
const g6 = vi.hoisted(() => ({ graphs: [], Graph: vi.fn() }))

vi.mock('@/api/admin', () => adminApi)
vi.mock('@/ui/internal/echarts.js', () => ({ init: engine.init, registerTheme: engine.registerTheme }))
vi.mock('@antv/g6', () => ({ Graph: g6.Graph }))

function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function mountWithRouter(component, path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/monitor/:section?', component: Monitor },
      { path: '/admin/dashboard', component: { template: '<div />' } }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(component, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

function lastOption() {
  const chart = engine.charts.at(-1)
  return chart?.setOption.mock.calls.at(-1)?.[0]
}

class FakeSocket {
  static instances = []

  constructor(url) {
    this.url = url
    this.closed = false
    FakeSocket.instances.push(this)
  }

  close() {
    this.closed = true
  }

  emit(data) {
    this.onmessage?.({ data: JSON.stringify({ type: 'snapshot', data }) })
  }
}

beforeEach(() => {
  vi.resetAllMocks()
  engine.charts = []
  engine.init.mockImplementation(() => {
    const chart = { setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() }
    engine.charts.push(chart)
    return chart
  })
  g6.graphs = []
  g6.Graph.mockImplementation(function Graph(options) {
    this.options = options
    this.render = vi.fn(async () => {})
    this.destroy = vi.fn()
    this.resize = vi.fn()
    this.fitView = vi.fn(async () => {})
    g6.graphs.push(this)
  })
  vi.spyOn(console, 'error').mockImplementation(() => {})
  FakeSocket.instances = []
  vi.stubGlobal('WebSocket', FakeSocket)
  adminApi.getTrafficHourly.mockResolvedValue({ data: [{ hour_ts: 1700000000, traffic: 0 }], meta: { latest_log_at: 0 } })
  adminApi.getUserTrafficRanking.mockResolvedValue({ data: [] })
  adminApi.getForwardObservabilityTargets.mockResolvedValue({ code: 0, data: { list: [] } })
  adminApi.getForwardObservabilityTrend.mockResolvedValue({ code: 0, data: { points: [] } })
  adminApi.getForwardObservabilityTopology.mockResolvedValue({ code: 0, data: { nodes: [], edges: [] } })
  adminApi.getForwardObservabilityMultiIngress.mockResolvedValue({ code: 0, data: { list: [] } })
  adminApi.listForwardRuntimeJobs.mockResolvedValue({ code: 0, data: { list: [] } })
})

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

describe('流量与监控 page', () => {
  it('opens 实时节点 at /admin/monitor and keeps the section in the path and the range in the query', async () => {
    localStorage.setItem('token', 'admin-token')
    const { wrapper, router } = await mountWithRouter(Monitor, '/admin/monitor?range=7d')

    expect(wrapper.get('h1').text()).toBe('Traffic & Monitoring')
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map(tab => tab.text())).toEqual(['Live nodes', 'User traffic', 'Node latency', 'Forwards'])
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-monitor-live]').exists()).toBe(true)
    expect(FakeSocket.instances).toHaveLength(1)

    await router.push('/admin/monitor/traffic?range=7d')
    await flushPromises()
    expect(wrapper.find('[data-monitor-live]').exists()).toBe(false)
    expect(FakeSocket.instances[0].closed).toBe(true)
    expect(wrapper.find('[data-monitor-traffic]').exists()).toBe(true)
    expect(adminApi.getTrafficHourly).toHaveBeenCalledWith(168, 0)
    wrapper.unmount()
  })

  it('goes to a section from its tab and keeps the range', async () => {
    const { wrapper, router } = await mountWithRouter(Monitor, '/admin/monitor/traffic?range=30d')
    const latencyTab = wrapper.findAll('[role="tab"]').find(tab => tab.text() === 'Node latency')
    await latencyTab.trigger('mousedown', { button: 0 })
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/monitor/latency?range=30d')
    wrapper.unmount()
  })

  it('sends an unknown section back to the page', async () => {
    const { wrapper, router } = await mountWithRouter(Monitor, '/admin/monitor/nowhere')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/monitor')
    wrapper.unmount()
  })
})

describe('实时节点 (WebSocket)', () => {
  it('connects with the token, applies the snapshot and the deltas and orders online first', async () => {
    localStorage.setItem('token', ' admin token ')
    const { wrapper } = await mountWithRouter(MonitorLive, '/admin/monitor')
    const socket = FakeSocket.instances[0]
    expect(socket.url).toMatch(/\/api\/v2\/admin\/ws\/monitor\?token=admin%20token$/)
    expect(wrapper.get('[data-monitor-connection]').attributes('data-state')).toBe('connecting')

    socket.onopen()
    await flushPromises()
    expect(wrapper.get('[data-monitor-connection]').text()).toBe('Live')
    expect(wrapper.text()).toContain('Waiting for the first node snapshot')

    socket.emit({
      overview: { total_nodes: 3, online_nodes: 1, offline_nodes: 1, pending_nodes: 1, total_upload: 2048, total_download: 1024 },
      nodes: [
        { id: 1, name: 'offline-a', host: '10.0.0.1', status: 'offline' },
        { id: 2, name: 'online-b', host: '10.0.0.2', status: 'online', cpu_usage: 42.5, memory_usage: 10, online_users: 7, uptime: 90000 },
        { id: 3, name: 'pending-c', host: '10.0.0.3', status: 'pending' }
      ]
    })
    await flushPromises()
    expect(wrapper.get('[data-overview="total"] [data-metric-value]').text()).toBe('3')
    expect(wrapper.get('[data-overview="online"]').text()).toContain('2.00 KB uploaded')
    const names = () => wrapper.findAll('[data-monitor-nodes] tbody tr').map(row => row.find('td').text())
    expect(names()).toEqual(['online-b', 'pending-c', 'offline-a'])
    expect(wrapper.get('[data-monitor-nodes] tbody tr').text()).toContain('42.5%')

    socket.emit({ node_updates: [{ id: 1, status: 'online' }, { id: 3, changed_fields: ['removed'] }, { id: 4, name: 'new-d', status: 'online' }] })
    await flushPromises()
    expect(names()).toEqual(['offline-a', 'online-b', 'new-d'])
    wrapper.unmount()
    expect(socket.closed).toBe(true)
  })

  it('reconnects after 3 s when the socket closes and offers 立即重连', async () => {
    vi.useFakeTimers()
    localStorage.setItem('token', 'admin-token')
    const { wrapper } = await mountWithRouter(MonitorLive, '/admin/monitor')
    FakeSocket.instances[0].onclose()
    await flushPromises()
    expect(wrapper.get('[data-monitor-connection]').text()).toBe('Reconnecting in 3 s…')
    expect(wrapper.text()).toContain('The live connection dropped')
    vi.advanceTimersByTime(3000)
    expect(FakeSocket.instances).toHaveLength(2)

    await wrapper.get('[data-monitor-reconnect]').trigger('click')
    expect(FakeSocket.instances).toHaveLength(3)
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('stays disconnected without a token', async () => {
    const { wrapper } = await mountWithRouter(MonitorLive, '/admin/monitor')
    expect(FakeSocket.instances).toHaveLength(0)
    expect(wrapper.get('[data-monitor-connection]').text()).toBe('Disconnected')
    wrapper.unmount()
  })
})

describe('用户流量', () => {
  it('loads the ranking and the chart for the range in the URL (24 h by default)', async () => {
    adminApi.getUserTrafficRanking.mockRejectedValue({ response: { data: { message: 'ranking failed' } } })
    const { wrapper } = await mountWithRouter(MonitorTraffic, '/admin/monitor/traffic')

    expect(adminApi.getUserTrafficRanking).toHaveBeenCalledWith(24, 1000, true)
    expect(adminApi.getTrafficHourly).toHaveBeenCalledWith(24, 0)
    expect(wrapper.text()).toContain('ranking failed')
    expect(wrapper.text()).toContain('No traffic data in the selected range')
    expect(wrapper.text()).toContain('No node has reported traffic yet.')
    wrapper.unmount()
  })

  it('reloads both on a new range and writes it to the URL', async () => {
    const { wrapper, router } = await mountWithRouter(MonitorTraffic, '/admin/monitor/traffic')
    const button = wrapper.findAll('[data-monitor-range] button').find(item => item.text() === '7 d')
    await button.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.range).toBe('7d')
    expect(adminApi.getUserTrafficRanking).toHaveBeenLastCalledWith(168, 1000, true)
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(168, 0)
    wrapper.unmount()
  })

  it('draws the chart and ignores stale answers when switching users quickly', async () => {
    const firstChart = deferred()
    const secondChart = deferred()
    adminApi.getUserTrafficRanking.mockResolvedValue({
      data: [
        { user_id: 1, email: 'one@example.com', traffic: 1024 },
        { user_id: 2, email: 'two@example.com', traffic: 2048 }
      ]
    })
    adminApi.getTrafficHourly
      .mockResolvedValueOnce({ data: [{ hour_ts: 1700000000, traffic: 512 }], meta: { latest_log_at: 1700000000 } })
      .mockReturnValueOnce(firstChart.promise)
      .mockReturnValueOnce(secondChart.promise)

    const { wrapper } = await mountWithRouter(MonitorTraffic, '/admin/monitor/traffic')
    expect(lastOption().series[0].data).toEqual([512])

    const rows = wrapper.findAll('[data-monitor-ranking] tbody tr')
    await rows[0].trigger('click')
    await rows[1].trigger('click')
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(24, 2)

    secondChart.resolve({ data: [{ hour_ts: 1700003600, traffic: 2048 }], meta: { latest_log_at: 1700003600 } })
    await flushPromises()
    expect(wrapper.get('[data-summary="total"] [data-metric-value]').text()).toBe('2.00 KB')
    expect(wrapper.get('[data-summary="total"]').text()).toContain('two@example.com')

    firstChart.resolve({ data: [{ hour_ts: 1700000000, traffic: 1024 }], meta: { latest_log_at: 1700000000 } })
    await flushPromises()
    expect(wrapper.get('[data-summary="total"] [data-metric-value]').text()).toBe('2.00 KB')

    await wrapper.get('[data-monitor-clear-user]').trigger('click')
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(24, 0)
    wrapper.unmount()
  })

  it('falls back to all users when the range drops the selected user', async () => {
    adminApi.getUserTrafficRanking
      .mockResolvedValueOnce({ data: [{ user_id: 1, email: 'one@example.com', traffic: 1024 }] })
      .mockResolvedValueOnce({ data: [] })
    const { wrapper, router } = await mountWithRouter(MonitorTraffic, '/admin/monitor/traffic')

    await wrapper.get('[data-monitor-ranking] tbody tr').trigger('click')
    await flushPromises()
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(24, 1)

    await router.replace('/admin/monitor/traffic?range=1h')
    await flushPromises()
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(1, 0)
    wrapper.unmount()
  })

  it('reads string values and panel envelopes', async () => {
    adminApi.getUserTrafficRanking.mockResolvedValue({ code: 0, msg: '操作成功', data: { list: [{ user_id: '1', email: 'one@example.com', traffic: '1536' }] } })
    adminApi.getTrafficHourly.mockResolvedValue({ code: 0, msg: '操作成功', data: { list: [{ hour_ts: '1700000000', traffic: '1536' }], meta: { latest_log_at: '1700000000' } } })
    const { wrapper } = await mountWithRouter(MonitorTraffic, '/admin/monitor/traffic')
    expect(wrapper.text()).toContain('one@example.com')
    expect(wrapper.get('[data-summary="total"] [data-metric-value]').text()).toBe('1.50 KB')
    expect(wrapper.get('[data-monitor-traffic-chart] [data-chart-canvas]').attributes('aria-label')).toContain('1.50 KB in total')
    wrapper.unmount()
  })
})

describe('节点延迟', () => {
  const targets = [
    { targetKey: 'node:1', targetType: 'node', targetId: 1, label: 'hk-01', host: '10.0.0.1', port: 443, latestAvgRtt: 12.5, latestLoss: 0, online: true },
    { targetKey: 'node:2', targetType: 'node', targetId: 2, label: 'jp-02', host: '10.0.0.2', port: 443, latestAvgRtt: 40, latestLoss: 2, online: false },
    { targetKey: 'forward:9', targetType: 'forward', targetId: 9, label: 'web', host: '1.1.1.1', port: 80 }
  ]

  it('asks the trend of the first node target over the range and draws avg, P95 and max', async () => {
    adminApi.getForwardObservabilityTargets.mockResolvedValue({ code: 0, data: { list: targets } })
    adminApi.getForwardObservabilityTrend.mockResolvedValue({ code: 0, data: { points: [{ bucketAt: 1700000000000, avg: 10, p95: 20, max: 30 }, { bucketAt: 1700000060000, avg: 12, p95: 25, max: 31 }] } })
    const { wrapper } = await mountWithRouter(MonitorLatency, '/admin/monitor/latency?range=7d')

    const params = adminApi.getForwardObservabilityTrend.mock.calls[0][0]
    expect(params.targetKey).toBe('node:1')
    expect(params.to - params.from).toBe(168 * 3600 * 1000)
    expect(lastOption().series.map(series => series.name)).toEqual(['Average', 'P95', 'Max'])
    expect(wrapper.findAll('[data-monitor-targets] tbody tr')).toHaveLength(2)
    expect(wrapper.get('[data-monitor-latency-chart] [data-chart-canvas]').attributes('aria-label')).toContain('Average 11 ms, highest P95 25 ms')

    await wrapper.findAll('[data-monitor-targets] tbody tr')[1].trigger('click')
    await flushPromises()
    expect(adminApi.getForwardObservabilityTrend).toHaveBeenLastCalledWith(expect.objectContaining({ targetKey: 'node:2' }))
    wrapper.unmount()
  })

  it('says when there are no targets', async () => {
    const { wrapper } = await mountWithRouter(MonitorLatency, '/admin/monitor/latency')
    expect(adminApi.getForwardObservabilityTrend).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('No probe targets yet')
    wrapper.unmount()
  })
})

describe('转发', () => {
  it('draws the topology, compares the first forward’s ingresses and lists the runtime jobs', async () => {
    adminApi.getForwardObservabilityTargets.mockResolvedValue({ code: 0, data: { list: [{ targetKey: 'forward:9', targetType: 'forward', targetId: 9, label: 'web', host: '1.1.1.1', port: 80 }] } })
    adminApi.getForwardObservabilityTopology.mockResolvedValue({
      code: 0,
      data: {
        nodes: [
          { id: 'relay-1', label: 'relay-hk', kind: 'relay', online: true, latencyMs: 8 },
          { id: 'exit-1', label: 'exit-jp', kind: 'exit', online: false, latencyMs: 30 }
        ],
        edges: [{ id: 'e1', source: 'relay-1', target: 'exit-1', label: 'tls' }]
      }
    })
    adminApi.getForwardObservabilityMultiIngress.mockResolvedValue({ code: 0, data: { list: [{ tunnelId: 1, tunnelName: 'hk-jp', ingressLabel: 'hk', ingressIp: '10.1.1.1', avgRtt: 21.4, loss: 0.5, online: true }] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ code: 0, data: { list: [{ id: 5, action: 'apply', backend: 'gost', status: 3, created_at: '2026-10-01T10:00:00Z' }] } })

    const { wrapper } = await mountWithRouter(MonitorForward, '/admin/monitor/forward')
    await flushPromises()

    expect(adminApi.getForwardObservabilityMultiIngress).toHaveBeenCalledWith('9')
    expect(adminApi.listForwardRuntimeJobs).toHaveBeenCalledWith({ limit: 50 })
    expect(g6.graphs).toHaveLength(1)
    const data = g6.graphs[0].options.data
    expect(data.nodes.map(node => node.id)).toEqual(['relay-1', 'exit-1'])
    expect(data.edges[0]).toMatchObject({ source: 'relay-1', target: 'exit-1' })
    expect(wrapper.get('[data-topology-canvas]').attributes('aria-label')).toBe('Forward topology graph: 2 nodes, 1 paths')
    expect(wrapper.get('[data-topology-text]').text()).toContain('relay-hk → exit-jp')
    expect(wrapper.get('[data-monitor-ingress]').text()).toContain('hk-jp')
    expect(wrapper.get('[data-monitor-jobs]').text()).toContain('Failed')
    wrapper.unmount()
    expect(g6.graphs[0].destroy).toHaveBeenCalled()
  })

  it('redraws the topology when the theme changes', async () => {
    adminApi.getForwardObservabilityTopology.mockResolvedValue({ code: 0, data: { nodes: [{ id: 'a', label: 'a', kind: 'relay', online: true }], edges: [] } })
    document.documentElement.setAttribute('data-theme', 'light')
    const { wrapper } = await mountWithRouter(MonitorForward, '/admin/monitor/forward')
    await flushPromises()
    expect(g6.graphs).toHaveLength(1)
    document.documentElement.setAttribute('data-theme', 'dark')
    await new Promise(resolve => setTimeout(resolve, 0))
    await flushPromises()
    expect(g6.graphs).toHaveLength(2)
    expect(g6.graphs[0].destroy).toHaveBeenCalled()
    wrapper.unmount()
    document.documentElement.removeAttribute('data-theme')
  })
})
