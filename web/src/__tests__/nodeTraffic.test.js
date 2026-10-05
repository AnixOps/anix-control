// The node traffic chart's model (views/admin/nodes/nodeTraffic.js and
// useNodeTraffic.js): the ranges and the windows they ask for, the reader of
// GET /api/v4/kernel/nodes/:id/traffic, and the option the chart is drawn with.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import {
  DAY_MS, DEFAULT_TRAFFIC_RANGE, HOUR_MS, TRAFFIC_RANGES, TRAFFIC_RANGE_KEYS, axisLabelOf, bucketLabelTime, hasTraffic,
  normalizeTrafficRange, peakBucket, readNodeTraffic, trafficQuery, trafficSeriesOption
} from '@/views/admin/nodes/nodeTraffic'
import { useNodeTraffic } from '@/views/admin/nodes/useNodeTraffic'

const kernelApi = vi.hoisted(() => ({ getKernelNodeTraffic: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

// 2026-10-02 08:20:30 UTC: not on an hour.
const NOW = Date.UTC(2026, 9, 2, 8, 20, 30)

describe('ranges and their windows', () => {
  it('offers 24 h, 7 d and 30 d by the hour and 90 d and 1 y by the day', () => {
    expect(TRAFFIC_RANGE_KEYS).toEqual(['24h', '7d', '30d', '90d', '1y'])
    expect(DEFAULT_TRAFFIC_RANGE).toBe('24h')
    expect(TRAFFIC_RANGE_KEYS.map(key => TRAFFIC_RANGES[key].granularity)).toEqual(['hour', 'hour', 'hour', 'day', 'day'])
    expect(normalizeTrafficRange('7d')).toBe('7d')
    expect(normalizeTrafficRange('week')).toBe('24h')
    expect(normalizeTrafficRange(undefined)).toBe('24h')
  })

  it('asks for whole UTC hours ending with the current hour, never more than the 720 the route allows', () => {
    for (const [range, hours] of [['24h', 24], ['7d', 168], ['30d', 720]]) {
      const query = trafficQuery(range, NOW)
      expect(query.granularity).toBe('hour')
      // Both ends are on the hour, so the route does not round them out.
      expect(query.until % HOUR_MS).toBe(0)
      expect(query.since % HOUR_MS).toBe(0)
      expect((query.until - query.since) / HOUR_MS).toBe(hours)
      // The current hour is the last bucket.
      expect(query.until).toBe(Date.UTC(2026, 9, 2, 9, 0, 0))
    }
    expect((trafficQuery('30d', NOW).until - trafficQuery('30d', NOW).since) / HOUR_MS).toBeLessThanOrEqual(720)
  })

  it('does not drift when "now" is exactly on the hour', () => {
    const onTheHour = Date.UTC(2026, 9, 2, 8, 0, 0)
    expect(trafficQuery('24h', onTheHour).until).toBe(Date.UTC(2026, 9, 2, 9, 0, 0))
  })

  it('asks for days from an instant `buckets - 1` days back and leaves `until` to the host', () => {
    const ninety = trafficQuery('90d', NOW)
    expect(ninety).toEqual({ granularity: 'day', since: NOW - 89 * DAY_MS })
    const year = trafficQuery('1y', NOW)
    expect(year).toEqual({ granularity: 'day', since: NOW - 364 * DAY_MS })
    expect(year).not.toHaveProperty('until')
    // 365 buckets, plus one day for a host midnight that falls a day earlier: inside the 366 allowed.
    expect(TRAFFIC_RANGES['1y'].buckets + 1).toBeLessThanOrEqual(366)
  })

  it('falls back to 24 h for an unknown range', () => {
    expect(trafficQuery('forever', NOW)).toEqual(trafficQuery('24h', NOW))
  })
})

describe('reading the answer', () => {
  const answer = {
    node_id: 7,
    granularity: 'hour',
    since_unix_ms: 1_000,
    until_unix_ms: 7_201_000,
    points: [
      { start_unix_ms: 1_000, up_bytes: 10, down_bytes: 40 },
      { start_unix_ms: 3_601_000, up_bytes: 0, down_bytes: 0 },
      { start_unix_ms: 7_201_000, up_bytes: '5', down_bytes: -3 }
    ],
    total: { up_bytes: 15, down_bytes: 40 }
  }

  it('reads points and total as numbers, a negative or odd value as zero', () => {
    const series = readNodeTraffic(answer)
    expect(series.granularity).toBe('hour')
    expect(series.since).toBe(1_000)
    expect(series.until).toBe(7_201_000)
    expect(series.points).toEqual([
      { start: 1_000, up: 10, down: 40 },
      { start: 3_601_000, up: 0, down: 0 },
      { start: 7_201_000, up: 5, down: 0 }
    ])
    expect(series.total).toEqual({ up: 15, down: 40 })
  })

  it('also reads the route\'s {data} envelope and sums the points when the total is missing', () => {
    const series = readNodeTraffic({ data: { ...answer, granularity: 'day', total: undefined } })
    expect(series.granularity).toBe('day')
    expect(series.total).toEqual({ up: 15, down: 40 })
  })

  it('survives an empty or odd answer', () => {
    for (const odd of [null, undefined, {}, { points: 'nope' }, { data: [] }]) {
      const series = readNodeTraffic(odd)
      expect(series.points).toEqual([])
      expect(series.total).toEqual({ up: 0, down: 0 })
      expect(hasTraffic(series)).toBe(false)
      expect(peakBucket(series)).toBeNull()
    }
  })

  it('finds the busiest bucket by up plus down, the first of equals', () => {
    const series = readNodeTraffic({ points: [
      { start_unix_ms: 1, up_bytes: 1, down_bytes: 1 },
      { start_unix_ms: 2, up_bytes: 5, down_bytes: 5 },
      { start_unix_ms: 3, up_bytes: 9, down_bytes: 1 }
    ] })
    expect(hasTraffic(series)).toBe(true)
    expect(peakBucket(series).start).toBe(2)
    expect(hasTraffic(readNodeTraffic({ points: [{ start_unix_ms: 1, up_bytes: 0, down_bytes: 0 }] }))).toBe(false)
  })
})

describe('labels and the option', () => {
  it('names a day by noon of it, so the date is right in a browser up to twelve hours from the host', () => {
    const midnight = Date.UTC(2026, 9, 2, 0, 0, 0)
    expect(bucketLabelTime(midnight, 'day')).toBe(midnight + 12 * HOUR_MS)
    // An hour is named by where it starts.
    expect(bucketLabelTime(midnight, 'hour')).toBe(midnight)
    // A browser five hours behind a UTC host still reads the 2nd.
    const behind = new Date(bucketLabelTime(midnight, 'day') - 5 * HOUR_MS)
    expect(behind.getUTCDate()).toBe(2)
  })

  it('shortens the axis labels per range', () => {
    expect(axisLabelOf('24h', 'hour')('2026-10-02 08:00')).toBe('08:00')
    expect(axisLabelOf('7d', 'hour')('2026-10-02 08:00')).toBe('10-02 08:00')
    expect(axisLabelOf('90d', 'day')('2026-10-02')).toBe('10-02')
    expect(axisLabelOf('1y', 'day')('2026-10-02')).toBe('2026-10-02')
  })

  const series = readNodeTraffic({
    granularity: 'hour',
    points: [
      { start_unix_ms: Date.UTC(2026, 9, 2, 8), up_bytes: 1.5 * 1024 ** 3, down_bytes: 3 * 1024 ** 3 },
      { start_unix_ms: Date.UTC(2026, 9, 2, 9), up_bytes: 0, down_bytes: 0 }
    ]
  })
  const bytes = value => `${Math.round(value)} B`
  const label = (time, granularity) => `${granularity}:${new Date(time).toISOString().slice(0, 16)}`

  it('draws download and upload as two lines on one axis in the unit of the peak', () => {
    const option = trafficSeriesOption(series, { range: '24h', label, bytes, upName: 'Upload', downName: 'Download' })
    expect(option.series.map(item => [item.name, item.type, item.data])).toEqual([
      ['Download', 'line', [3, 0]],
      ['Upload', 'line', [1.5, 0]]
    ])
    expect(option.xAxis.data).toEqual(['hour:2026-10-02T08:00', 'hour:2026-10-02T09:00'])
    expect(option.yAxis.axisLabel.formatter(2)).toBe('2 GB')
    expect(option.yAxis.min).toBe(0)
    expect(option.xAxis.axisLabel.formatter('hour:2026-10-02T08:00')).toBe('hour:2026-10-02T08:00'.slice(11))
    // The tooltip formats the scaled value back in bytes.
    expect(option.tooltip.valueFormatter(1)).toBe(`${1024 ** 3} B`)
    // No colours: they come from the chart theme.
    expect(JSON.stringify(option)).not.toMatch(/#[0-9a-f]{3,6}|rgb/i)
  })

  it('counts in bytes while the peak is small', () => {
    const small = readNodeTraffic({ points: [{ start_unix_ms: 1, up_bytes: 10, down_bytes: 20 }] })
    const option = trafficSeriesOption(small, { range: '24h', label, bytes, upName: 'u', downName: 'd' })
    expect(option.yAxis.axisLabel.formatter(10)).toBe('10 B')
    expect(option.series[0].data).toEqual([20])
  })
})

describe('useNodeTraffic', () => {
  let scope

  beforeEach(() => {
    vi.resetAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })

  afterEach(() => {
    scope?.stop()
    vi.useRealTimers()
  })

  function setup(id = 7, options = {}) {
    scope = effectScope()
    return scope.run(() => useNodeTraffic(id, options))
  }

  const emptyAnswer = { granularity: 'hour', points: [], total: { up_bytes: 0, down_bytes: 0 } }

  it('loads the range it is on and reads the answer', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue({ ...emptyAnswer, points: [{ start_unix_ms: 1, up_bytes: 2, down_bytes: 3 }], total: { up_bytes: 2, down_bytes: 3 } })
    const traffic = setup()
    expect(traffic.status.value).toBe('idle')
    await traffic.load()
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledWith(7, trafficQuery('24h', NOW))
    expect(traffic.status.value).toBe('ready')
    expect(traffic.series.value.total).toEqual({ up: 2, down: 3 })
  })

  it('asks again, with the granularity of the new range, when the range changes', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(emptyAnswer)
    const traffic = setup()
    await traffic.load()
    traffic.select('90d')
    await nextTick()
    expect(traffic.range.value).toBe('90d')
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenLastCalledWith(7, trafficQuery('90d', NOW))
    expect(kernelApi.getKernelNodeTraffic.mock.lastCall[1].granularity).toBe('day')
    traffic.select('nonsense')
    await nextTick()
    expect(traffic.range.value).toBe('24h')
  })

  it('starts on the range it is given', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(emptyAnswer)
    const traffic = setup(7, { initialRange: '1y' })
    await traffic.load()
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenCalledWith(7, trafficQuery('1y', NOW))
  })

  it('reloads for another node', async () => {
    kernelApi.getKernelNodeTraffic.mockResolvedValue(emptyAnswer)
    const id = ref(7)
    const traffic = setup(id)
    await traffic.load()
    id.value = 9
    await nextTick()
    expect(kernelApi.getKernelNodeTraffic).toHaveBeenLastCalledWith(9, trafficQuery('24h', NOW))
  })

  it('drops the answer of a request that a newer one replaced', async () => {
    let releaseFirst
    kernelApi.getKernelNodeTraffic
      .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
      .mockResolvedValueOnce({ ...emptyAnswer, total: { up_bytes: 9, down_bytes: 9 } })
    const traffic = setup()
    const first = traffic.load()
    await traffic.load()
    releaseFirst({ ...emptyAnswer, total: { up_bytes: 1, down_bytes: 1 } })
    await first
    expect(traffic.series.value.total).toEqual({ up: 9, down: 9 })
    expect(traffic.status.value).toBe('ready')
  })

  it('drops the answer that comes after the page is gone', async () => {
    let release
    kernelApi.getKernelNodeTraffic.mockReturnValueOnce(new Promise((resolve) => { release = resolve }))
    const traffic = setup()
    const pending = traffic.load()
    scope.stop()
    release(emptyAnswer)
    await pending
    expect(traffic.series.value).toBeNull()
  })

  it('shows the route\'s own message and keeps the status for "copy error details"; a later success clears it', async () => {
    kernelApi.getKernelNodeTraffic.mockRejectedValueOnce({
      message: 'Request failed with status code 400',
      response: { status: 400, headers: { 'x-request-id': 'r-1' }, data: { error: { code: 'invalid_request', message: 'at most 720 hour buckets' } } }
    })
    const traffic = setup()
    await traffic.load()
    expect(traffic.status.value).toBe('error')
    expect(traffic.series.value).toBeNull()
    expect(traffic.error.value.message).toBe('at most 720 hour buckets')
    expect(traffic.error.value.response).toEqual({ status: 400, headers: { 'x-request-id': 'r-1' }, data: { message: 'at most 720 hour buckets' } })
    kernelApi.getKernelNodeTraffic.mockResolvedValueOnce(emptyAnswer)
    await traffic.load()
    expect(traffic.status.value).toBe('ready')
    expect(traffic.error.value).toBeNull()
  })

  it('makes no request without a node', async () => {
    const traffic = setup('')
    await traffic.load()
    expect(kernelApi.getKernelNodeTraffic).not.toHaveBeenCalled()
    expect(traffic.status.value).toBe('idle')
  })
})
