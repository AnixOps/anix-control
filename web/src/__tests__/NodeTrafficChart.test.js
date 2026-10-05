// The node traffic chart (views/admin/nodes/NodeTrafficChart.vue, used by the
// node page's 流量 section and the 实时节点 sheet): range switch, the totals,
// the data for screen readers, and its loading, empty and error states.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeTrafficChart from '@/views/admin/nodes/NodeTrafficChart.vue'
import { setLocale } from '@/i18n'
import { trafficQuery } from '@/views/admin/nodes/nodeTraffic'

const kernelApi = vi.hoisted(() => ({ getKernelNodeTraffic: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

const engine = vi.hoisted(() => ({ charts: [], init: vi.fn(), registerTheme: vi.fn() }))
vi.mock('@/ui/internal/echarts.js', () => ({ init: engine.init, registerTheme: engine.registerTheme }))

const NOW = Date.UTC(2026, 9, 2, 8, 20, 30)
const GIB = 1024 ** 3

function hourly(hours, { up = 1 * GIB, down = 3 * GIB } = {}) {
  const until = Date.UTC(2026, 9, 2, 9)
  const points = Array.from({ length: hours }, (_, index) => ({
    start_unix_ms: until - (hours - index) * 3_600_000,
    up_bytes: index === hours - 1 ? up : 0,
    down_bytes: index === hours - 1 ? down : 0
  }))
  return { node_id: 7, granularity: 'hour', since_unix_ms: points[0].start_unix_ms, until_unix_ms: until, points, total: { up_bytes: up, down_bytes: down } }
}

function daily(days) {
  const points = Array.from({ length: days }, (_, index) => ({
    start_unix_ms: Date.UTC(2026, 9, 2) - (days - 1 - index) * 86_400_000,
    up_bytes: GIB, down_bytes: 2 * GIB
  }))
  return { node_id: 7, granularity: 'day', since_unix_ms: points[0].start_unix_ms, until_unix_ms: Date.UTC(2026, 9, 3), points, total: { up_bytes: days * GIB, down_bytes: days * 2 * GIB } }
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

// A bucket's label as the page prints it: local time (the tests do not pin the zone).
const pad = value => String(value).padStart(2, '0')
function local(ms) {
  const date = new Date(ms)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const lastOption = () => engine.charts.at(-1)?.setOption.mock.calls.at(-1)?.[0]

function renderChart(props = {}) {
  return render(NodeTrafficChart, { props: { nodeId: 7, nodeName: 'hk-01', ...props } })
}

describe('NodeTrafficChart', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    // Only the clock: timers (the chart's delayed skeleton) keep running.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(NOW)
    await setLocale('en')
    engine.charts = []
    engine.init.mockImplementation(() => {
      const chart = { setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() }
      engine.charts.push(chart)
      return chart
    })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('loads the last 24 hours, shows the totals and draws upload and download', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(hourly(24))
    renderChart()

    await waitFor(() => expect(screen.getByText('4.0 GB')).toBeTruthy())
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledWith(7, trafficQuery('24h', NOW))
    expect(screen.getByRole('button', { name: '24 h', pressed: true })).toBeTruthy()

    const totals = screen.getByRole('region', { name: 'Traffic totals' })
    expect(totals.querySelector('[data-summary="up"]').textContent).toContain('1.0 GB')
    expect(totals.querySelector('[data-summary="down"]').textContent).toContain('3.0 GB')
    expect(totals.querySelector('[data-summary="total"]').textContent).toContain('4.0 GB')

    await waitFor(() => expect(lastOption()).toBeTruthy())
    const option = lastOption()
    expect(option.series.map(item => item.name)).toEqual(['Download', 'Upload'])
    expect(option.series[0].data).toHaveLength(24)
    expect(option.series[0].data.at(-1)).toBe(3)
    expect(option.yAxis.axisLabel.formatter(2)).toBe('2 GB')
  })

  it('names the chart and says what it shows for screen readers, and has the data as a table', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(hourly(24))
    renderChart()
    const plot = await screen.findByRole('img', { name: /Traffic of hk-01, the last 24 hours/ })
    const lastHour = local(Date.UTC(2026, 9, 2, 8))
    expect(plot.getAttribute('aria-label')).toBe(`Traffic of hk-01, the last 24 hours: 1.0 GB uploaded and 3.0 GB downloaded. Busiest hour: 4.0 GB, starting ${lastHour}.`)

    const table = await screen.findByRole('table', { hidden: true })
    expect(within(table).getAllByRole('columnheader', { hidden: true }).map(cell => cell.textContent)).toEqual(['Time', 'Upload', 'Download', 'Total'])
    const rows = within(table).getAllByRole('row', { hidden: true })
    expect(rows).toHaveLength(25)
    expect(rows.at(-1).textContent).toContain(lastHour)
    expect(rows.at(-1).textContent).toContain('4.00 GB')
  })

  it('names the node by its id when the page gives no name', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(hourly(24))
    renderChart({ nodeName: '' })
    expect(await screen.findByRole('img', { name: /Traffic of #7/ })).toBeTruthy()
  })

  it('asks for the hours of 7 d and 30 d, and for days with 90 d and 1 y', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    kernelApi.getKernelNodeTraffic.mockImplementation(async (id, query) => (query.granularity === 'day' ? daily(query.since ? 90 : 30) : hourly(24)))
    renderChart()
    await waitFor(() => expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledTimes(1))

    for (const [name, range, granularity] of [['7 d', '7d', 'hour'], ['30 d', '30d', 'hour'], ['90 d', '90d', 'day'], ['1 y', '1y', 'day']]) {
      await user.click(screen.getByRole('button', { name }))
      await waitFor(() => expect(kernelApi.getKernelNodeTraffic).toHaveBeenLastCalledWith(7, trafficQuery(range, NOW)))
      expect(kernelApi.getKernelNodeTraffic.mock.lastCall[1].granularity).toBe(granularity)
      expect(screen.getByRole('button', { name, pressed: true })).toBeTruthy()
    }
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledTimes(5)
    // 1 y is 365 daily buckets and 30 d 720 hourly ones: inside the route's limits.
    const calls = kernelApi.getKernelNodeTraffic.mock.calls.map(call => call[1])
    expect((calls[2].until - calls[2].since) / 3_600_000).toBe(720)
  })

  it('says "the day" for a daily chart and notes where its numbers come from', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    kernelApi.getKernelNodeTraffic.mockResolvedValueOnce(hourly(24)).mockResolvedValueOnce(daily(90))
    renderChart()
    expect((await screen.findByTestId('node-traffic-note')).textContent).toContain('Hourly buckets (UTC hours)')
    await user.click(screen.getByRole('button', { name: '90 d' }))
    await waitFor(() => expect(screen.getByTestId('node-traffic-note').textContent).toContain('Daily totals'))
    const plot = screen.getByRole('img', { name: /the last 90 days/ })
    expect(plot.getAttribute('aria-label')).toContain('Busiest day')
  })

  it('shows an empty state, not a flat chart, when nothing was moved', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(hourly(24, { up: 0, down: 0 }))
    renderChart()
    expect(await screen.findByText('No traffic in this range')).toBeTruthy()
    expect(screen.getByText(/Hourly history only goes back as far as the traffic log is kept/)).toBeTruthy()
    expect(screen.queryByRole('table', { hidden: true })).toBeNull()
    expect(screen.getAllByText('0 B').length).toBeGreaterThanOrEqual(3)
  })

  it('shows the route\'s message with Try again when the request fails, and loads on retry', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    kernelApi.getKernelNodeTraffic.mockRejectedValueOnce({
      response: { status: 500, data: { error: { code: 'internal_error', message: 'the traffic store is unavailable' } } }
    })
    renderChart()
    expect(await screen.findByText('Couldn’t load the traffic')).toBeTruthy()
    expect(screen.getByText('the traffic store is unavailable')).toBeTruthy()
    // The totals do not pretend to know.
    expect(within(screen.getByRole('region', { name: 'Traffic totals' })).getAllByText('—')).toHaveLength(3)

    kernelApi.getKernelNodeTraffic.mockResolvedValueOnce(hourly(24))
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => expect(screen.getByText('4.0 GB')).toBeTruthy())
    expect(screen.queryByText('Couldn’t load the traffic')).toBeNull()
  })

  it('keeps the answer of the range that was chosen last when the requests finish out of order', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const slow = deferred()
    kernelApi.getKernelNodeTraffic
      .mockResolvedValueOnce(hourly(24))
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValueOnce(daily(90))
    renderChart()
    await waitFor(() => expect(screen.getByText('4.0 GB')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: '7 d' }))
    await user.click(screen.getByRole('button', { name: '90 d' }))
    await waitFor(() => expect(screen.getByText('270.0 GB')).toBeTruthy())
    slow.resolve(hourly(168, { up: 9 * GIB, down: 9 * GIB }))
    await Promise.resolve()
    await Promise.resolve()
    expect(screen.queryByText('18.0 GB')).toBeNull()
    expect(screen.getByText('270.0 GB')).toBeTruthy()
  })

  it('reloads from Refresh and for another node', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    kernelApi.getKernelNodeTraffic.mockResolvedValue(hourly(24))
    const { rerender } = renderChart()
    await waitFor(() => expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledTimes(1))
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledTimes(2))
    await rerender({ nodeId: 9, nodeName: 'jp-01' })
    await waitFor(() => expect(kernelApi.getKernelNodeTraffic).toHaveBeenLastCalledWith(9, expect.anything()))
  })
})
