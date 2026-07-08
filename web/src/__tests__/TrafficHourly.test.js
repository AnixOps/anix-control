import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficHourly from '@/views/admin/TrafficHourly.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  getTrafficHourly: vi.fn(),
  getUserTrafficRanking: vi.fn()
}))

const echartsMock = vi.hoisted(() => ({
  init: vi.fn(() => ({
    setOption: vi.fn(),
    resize: vi.fn(),
    clear: vi.fn(),
    dispose: vi.fn()
  }))
}))

vi.mock('@/api/admin', () => adminApi)
vi.mock('echarts', () => echartsMock)

function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('TrafficHourly.vue', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    echartsMock.init.mockImplementation(() => ({
      setOption: vi.fn(),
      resize: vi.fn(),
      clear: vi.fn(),
      dispose: vi.fn()
    }))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getTrafficHourly.mockResolvedValue({
      data: [{ hour_ts: 1700000000, traffic: 0 }],
      meta: { latest_log_at: 0 }
    })
    adminApi.getUserTrafficRanking.mockRejectedValue({
      response: { data: { message: 'ranking failed' } }
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps loading the chart when the user ranking request fails', async () => {
    const wrapper = mount(TrafficHourly)
    await flushPromises()

    expect(adminApi.getUserTrafficRanking).toHaveBeenCalledWith(168, 1000, true)
    expect(adminApi.getTrafficHourly).toHaveBeenCalledWith(168, 0)
    expect(wrapper.text()).toContain('ranking failed')
    expect(wrapper.text()).toContain('No traffic data in the selected range')

    wrapper.unmount()
  })

  it('ignores stale chart responses when switching users quickly', async () => {
    const firstChart = deferred()
    const secondChart = deferred()

    adminApi.getUserTrafficRanking.mockResolvedValue({
      data: [
        { user_id: 1, email: 'one@example.com', traffic: 1024 },
        { user_id: 2, email: 'two@example.com', traffic: 2048 }
      ]
    })
    adminApi.getTrafficHourly
      .mockReturnValueOnce(firstChart.promise)
      .mockReturnValueOnce(secondChart.promise)

    const wrapper = mount(TrafficHourly)
    await flushPromises()

    const userRow = wrapper.findAll('.ranking-row').find(row => row.text().includes('two@example.com'))
    expect(userRow).toBeTruthy()
    await userRow.trigger('click')

    secondChart.resolve({
      data: [{ hour_ts: 1700003600, traffic: 2048 }],
      meta: { latest_log_at: 1700003600 }
    })
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('.summary-value').text()).toBe('2.00 KB')

    firstChart.resolve({
      data: [{ hour_ts: 1700000000, traffic: 1024 }],
      meta: { latest_log_at: 1700000000 }
    })
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('.summary-value').text()).toBe('2.00 KB')

    wrapper.unmount()
  })

  it('uses the reset selected user when the range change removes the previous user', async () => {
    adminApi.getUserTrafficRanking
      .mockResolvedValueOnce({ data: [{ user_id: 1, email: 'one@example.com', traffic: 1024 }] })
      .mockResolvedValueOnce({ data: [] })
    adminApi.getTrafficHourly.mockResolvedValue({
      data: [{ hour_ts: 1700000000, traffic: 0 }],
      meta: { latest_log_at: 0 }
    })

    const wrapper = mount(TrafficHourly)
    await flushPromises()

    const userRow = wrapper.findAll('.ranking-row').find(row => row.text().includes('one@example.com'))
    expect(userRow).toBeTruthy()
    await userRow.trigger('click')
    await flushPromises()

    await wrapper.findAll('select')[0].setValue('24')
    await flushPromises()
    expect(adminApi.getTrafficHourly).toHaveBeenLastCalledWith(24, 0)

    wrapper.unmount()
  })

  it('normalizes string traffic values before formatting', async () => {
    adminApi.getUserTrafficRanking.mockResolvedValue({
      data: [{ user_id: '1', email: 'one@example.com', traffic: '1536' }]
    })
    adminApi.getTrafficHourly.mockResolvedValue({
      data: [{ hour_ts: '1700000000', traffic: '1536' }],
      meta: { latest_log_at: '1700000000' }
    })

    const wrapper = mount(TrafficHourly)
    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('1.50 KB')

    wrapper.unmount()
  })

  it('accepts panel envelope responses for chart and ranking data', async () => {
    adminApi.getUserTrafficRanking.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{ user_id: 1, email: 'one@example.com', traffic: 2048 }]
      },
      ts: 1700000000000
    })
    adminApi.getTrafficHourly.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{ hour_ts: 1700000000, traffic: 2048 }],
        meta: { latest_log_at: 1700000000 }
      },
      ts: 1700000000000
    })

    const wrapper = mount(TrafficHourly)
    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('one@example.com')
    expect(wrapper.text()).toContain('2.00 KB')

    wrapper.unmount()
  })
})
