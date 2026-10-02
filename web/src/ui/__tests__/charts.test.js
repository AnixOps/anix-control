import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import UiChart from '../UiChart.vue'
import UiMetricCard from '../UiMetricCard.vue'
import UiSkeleton from '../UiSkeleton.vue'

const engine = vi.hoisted(() => {
  const charts = []
  return {
    charts,
    registerTheme: vi.fn(),
    init: vi.fn((element, theme) => {
      const chart = { element, theme, setOption: vi.fn(), setTheme: vi.fn(), resize: vi.fn(), dispose: vi.fn() }
      charts.push(chart)
      return chart
    })
  }
})

vi.mock('@/ui/internal/echarts.js', () => ({ init: engine.init, registerTheme: engine.registerTheme }))

const option = { xAxis: { type: 'category', data: ['00:00', '01:00'] }, yAxis: { type: 'value' }, series: [{ type: 'line', data: [1, 2] }] }

let observers
class FakeResizeObserver {
  constructor(callback) {
    this.callback = callback
    observers.push(this)
  }

  observe(element) {
    this.element = element
  }

  disconnect() {
    this.disconnected = true
  }
}

describe('UiChart', () => {
  beforeEach(() => {
    engine.charts.length = 0
    engine.init.mockClear()
    engine.registerTheme.mockClear()
    observers = []
    vi.stubGlobal('ResizeObserver', FakeResizeObserver)
    document.documentElement.setAttribute('data-theme', 'light')
  })

  afterEach(() => {
    document.documentElement.removeAttribute('data-theme')
  })

  it('draws the option with the token theme on a named role="img" plot', async () => {
    const wrapper = mount(UiChart, { props: { option, label: 'Traffic', summary: '3 GB in total' } })
    await flushPromises()

    expect(engine.init).toHaveBeenCalledTimes(1)
    const chart = engine.charts[0]
    expect(chart.theme).toBe('anixops-light')
    expect(engine.registerTheme).toHaveBeenCalledWith('anixops-light', expect.any(Object))
    expect(chart.setOption).toHaveBeenCalledWith(option, { notMerge: true })
    const plot = wrapper.get('[data-chart-canvas]')
    expect(plot.attributes('role')).toBe('img')
    expect(plot.attributes('aria-label')).toBe('Traffic: 3 GB in total')
    wrapper.unmount()
    expect(chart.dispose).toHaveBeenCalled()
  })

  it('updates in place, re-themes when <html data-theme> changes and resizes with its box', async () => {
    const wrapper = mount(UiChart, { props: { option, label: 'Traffic' } })
    await flushPromises()
    const chart = engine.charts[0]

    const next = { ...option, series: [{ type: 'bar', data: [3, 4] }] }
    await wrapper.setProps({ option: next })
    await flushPromises()
    expect(engine.init).toHaveBeenCalledTimes(1)
    expect(chart.setOption).toHaveBeenLastCalledWith(next, { notMerge: true })

    document.documentElement.setAttribute('data-theme', 'dark')
    await flushPromises()
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(chart.setTheme).toHaveBeenCalledWith('anixops-dark')

    expect(observers).toHaveLength(1)
    observers[0].callback([])
    expect(chart.resize).toHaveBeenCalled()
    wrapper.unmount()
    expect(observers[0].disconnected).toBe(true)
  })

  it('shows the empty state instead of a plot', async () => {
    const wrapper = mount(UiChart, { props: { option: null, label: 'Traffic', empty: true, emptyTitle: 'No traffic yet', emptyDescription: 'Nodes have not reported.' } })
    await flushPromises()
    expect(wrapper.text()).toContain('No traffic yet')
    expect(wrapper.text()).toContain('Nodes have not reported.')
    expect(wrapper.get('[data-chart-canvas]').attributes('aria-hidden')).toBe('true')
    expect(engine.init).not.toHaveBeenCalled()
  })

  it('shows the error state with 重试', async () => {
    const wrapper = mount(UiChart, { props: { option, label: 'Traffic', error: 'gateway timeout', errorTitle: 'Couldn’t load traffic' } })
    await flushPromises()
    expect(wrapper.get('[data-error-state]').text()).toContain('Couldn’t load traffic')
    expect(wrapper.text()).toContain('gateway timeout')
    await wrapper.get('[data-error-retry]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('shows a chart skeleton only after 300 ms of the first load', async () => {
    vi.useFakeTimers()
    const wrapper = mount(UiChart, { props: { option: null, label: 'Traffic', loading: true } })
    await nextTick()
    expect(wrapper.find('.ui-skeleton--chart').exists()).toBe(false)
    vi.advanceTimersByTime(320)
    await nextTick()
    expect(wrapper.find('.ui-skeleton--chart').exists()).toBe(true)
    expect(wrapper.get('[role="status"]').text()).toContain('Loading Traffic')
    vi.useRealTimers()
  })

  it('offers the data as a table: hidden for screen readers, shown on 以表格查看', async () => {
    const table = {
      columns: [{ key: 'hour', label: 'Hour' }, { key: 'traffic', label: 'Traffic', numeric: true, format: value => `${value} B` }],
      rows: [{ hour: '00:00', traffic: 1 }, { hour: '01:00', traffic: 2 }]
    }
    const wrapper = mount(UiChart, { props: { option, label: 'Traffic', table } })
    await flushPromises()
    const wrap = wrapper.get('[data-chart-table]')
    expect(wrap.classes()).toContain('visually-hidden')
    expect(wrap.find('caption').text()).toBe('Traffic')
    expect(wrap.findAll('tbody tr')).toHaveLength(2)
    expect(wrap.text()).toContain('2 B')

    const toggle = wrapper.get('[data-chart-table-toggle]')
    expect(toggle.text()).toBe('View as table')
    await toggle.trigger('click')
    expect(wrapper.get('[data-chart-table]').classes()).not.toContain('visually-hidden')
    expect(toggle.attributes('aria-pressed')).toBe('true')
    expect(toggle.text()).toBe('View as chart')
  })

  it('turns an engine failure into the error state and retries it', async () => {
    engine.init.mockImplementationOnce(() => { throw new Error('canvas unavailable') })
    const wrapper = mount(UiChart, { props: { option, label: 'Traffic' } })
    await flushPromises()
    expect(wrapper.text()).toContain('canvas unavailable')
    await wrapper.get('[data-error-retry]').trigger('click')
    await flushPromises()
    expect(engine.init).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-error-state]').exists()).toBe(false)
  })
})

describe('UiMetricCard', () => {
  it('shows the label, the big number, a trend word and the detail', () => {
    const wrapper = mount(UiMetricCard, {
      props: { label: 'Users', value: '1,204', trend: '+3 today', trendDirection: 'up', trendTone: 'positive', detail: '980 active' }
    })
    expect(wrapper.get('.ui-metric-card__label').text()).toBe('Users')
    expect(wrapper.get('[data-metric-value]').text()).toBe('1,204')
    const trend = wrapper.get('[data-metric-trend]')
    expect(trend.text()).toBe('+3 today')
    expect(trend.classes()).toContain('is-positive')
    expect(wrapper.text()).toContain('980 active')
    expect(wrapper.find('[data-metric-sparkline]').exists()).toBe(false)
  })

  it('draws a decorative sparkline from two or more points', () => {
    const wrapper = mount(UiMetricCard, { props: { label: 'Traffic', value: '3 GB', sparkline: [0, 4, 2, 8] } })
    const svg = wrapper.get('[data-metric-sparkline]')
    expect(svg.attributes('aria-hidden')).toBe('true')
    const line = svg.find('.ui-metric-card__line').attributes('d')
    expect(line.startsWith('M0.00 30.00')).toBe(true)
    expect(line.split('L')).toHaveLength(4)
    expect(svg.find('.ui-metric-card__area').attributes('d')).toMatch(/Z$/)
  })

  it('renders an em dash for a missing value and a card skeleton while loading', async () => {
    vi.useFakeTimers()
    const empty = mount(UiMetricCard, { props: { label: 'Tickets', value: '' } })
    expect(empty.get('[data-metric-value]').text()).toBe('—')
    const loading = mount(UiMetricCard, { props: { label: 'Tickets', loading: true } })
    vi.advanceTimersByTime(320)
    await nextTick()
    expect(loading.find('.ui-skeleton--card').exists()).toBe(true)
    vi.useRealTimers()
  })
})

describe('UiSkeleton chart variant', () => {
  it('draws bars at the given height', () => {
    const wrapper = mount(UiSkeleton, { props: { variant: 'chart', height: 200 } })
    expect(wrapper.get('.ui-skeleton__chart').attributes('style')).toContain('height: 200px')
    expect(wrapper.findAll('.ui-skeleton__bar').length).toBeGreaterThan(6)
  })
})
