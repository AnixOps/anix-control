// A proxy node's traffic over time (GET /api/v4/kernel/nodes/:id/traffic):
// the ranges the chart offers and the window each one asks for, a reader for
// the answer, and the ECharts option it is drawn with. Shared by the node
// page's 流量 section and the 实时节点 traffic sheet. No requests here.
//
// The route's buckets (docs/reference/traffic-stats-operations.md):
//   hour  UTC hours from the raw traffic log, at most 720 buckets;
//   day   the Control host's calendar days from the daily statistics, at most
//         366 buckets.
// `since` rounds down and `until` up to whole buckets, and a longer window is
// a 400, not clamped: so the windows below are built to stay inside the limit
// whatever the time zone of the host.
import { axisUnitFor } from '../monitor/trafficData'

export const HOUR_MS = 3_600_000
export const DAY_MS = 86_400_000

// 24 h, 7 d and 30 d are hourly; 90 d and 1 y are daily.
export const TRAFFIC_RANGES = Object.freeze({
  '24h': { granularity: 'hour', buckets: 24 },
  '7d': { granularity: 'hour', buckets: 168 },
  '30d': { granularity: 'hour', buckets: 720 },
  '90d': { granularity: 'day', buckets: 90 },
  '1y': { granularity: 'day', buckets: 365 }
})
export const TRAFFIC_RANGE_KEYS = Object.freeze(Object.keys(TRAFFIC_RANGES))
export const DEFAULT_TRAFFIC_RANGE = '24h'

export function normalizeTrafficRange(value) {
  return TRAFFIC_RANGES[value] ? value : DEFAULT_TRAFFIC_RANGE
}

// The query of a range at `now` (Unix milliseconds).
//   hour: the last `buckets` whole UTC hours, ending with the current one.
//         Both ends are on the hour, so the count is exact (720 for 30 d).
//   day:  `since` is `buckets - 1` days back and `until` is left out (the end
//         of today on the host). The host rounds `since` down to its own
//         midnight, which may be one day earlier than a clock reading here:
//         365 buckets (1 y) stay inside the limit of 366 that way.
export function trafficQuery(range, now = Date.now()) {
  const { granularity, buckets } = TRAFFIC_RANGES[normalizeTrafficRange(range)]
  if (granularity === 'hour') {
    const until = (Math.floor(now / HOUR_MS) + 1) * HOUR_MS
    return { granularity, since: until - buckets * HOUR_MS, until }
  }
  return { granularity, since: now - (buckets - 1) * DAY_MS }
}

function bytes(value) {
  const numeric = Number(value)
  return Number.isFinite(numeric) && numeric > 0 ? numeric : 0
}

// The answer ({ node_id, granularity, since_unix_ms, until_unix_ms, points,
// total }, or the same inside { data }) as numbers. `total` falls back to the
// sum of the points.
export function readNodeTraffic(answer) {
  const body = answer && typeof answer === 'object' && answer.data && typeof answer.data === 'object' && !Array.isArray(answer.data) ? answer.data : answer
  const list = Array.isArray(body?.points) ? body.points : []
  const points = list
    .map(point => ({ start: Number(point?.start_unix_ms || 0), up: bytes(point?.up_bytes), down: bytes(point?.down_bytes) }))
    .filter(point => point.start > 0)
  const sum = points.reduce((all, point) => ({ up: all.up + point.up, down: all.down + point.down }), { up: 0, down: 0 })
  const total = body?.total && typeof body.total === 'object'
    ? { up: bytes(body.total.up_bytes), down: bytes(body.total.down_bytes) }
    : sum
  return {
    granularity: body?.granularity === 'day' ? 'day' : 'hour',
    since: Number(body?.since_unix_ms || 0) || points[0]?.start || 0,
    until: Number(body?.until_unix_ms || 0) || 0,
    points,
    total
  }
}

export function hasTraffic(series) {
  return Boolean(series) && series.points.some(point => point.up > 0 || point.down > 0)
}

// The busiest bucket (up + down), the first of equals; null without traffic.
export function peakBucket(series) {
  if (!hasTraffic(series)) return null
  return series.points.reduce((best, point) => (!best || point.up + point.down > best.up + best.down ? point : best), null)
}

// The time a bucket is labelled with. An hour starts where it starts; a day
// starts at the host's midnight, which is another clock time (and may be
// another date) in a browser in another zone: noon of the day names the date
// right for a difference of up to twelve hours.
export function bucketLabelTime(start, granularity) {
  return granularity === 'day' ? start + 12 * HOUR_MS : start
}

// The axis label of a bucket label ("2026-10-05 12:00" or "2026-10-05").
export function axisLabelOf(range, granularity) {
  if (granularity === 'day') {
    return range === '1y' ? label => label : label => label.slice(5)
  }
  return range === '24h' ? label => label.slice(11) : label => label.slice(5)
}

// Two lines (download with a soft area) on one axis in the unit of the peak,
// so ticks are round numbers of it. `label(time, granularity)` is the full
// bucket label (date and time for an hour, the date for a day);
// `bytes(value)` formats a value in the tooltip; colours come from the chart
// theme (UiChart).
export function trafficSeriesOption(series, { range, label, bytes: formatBytes, upName, downName }) {
  const points = series.points
  const { unit, scale } = axisUnitFor(Math.max(0, ...points.map(point => Math.max(point.up, point.down))))
  const scaled = value => Number((value / scale).toFixed(3))
  const shorten = axisLabelOf(range, series.granularity)
  return {
    tooltip: { trigger: 'axis', valueFormatter: value => formatBytes(Number(value) * scale) },
    legend: { top: 0 },
    grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
    xAxis: {
      type: 'category',
      data: points.map(point => label(bucketLabelTime(point.start, series.granularity), series.granularity)),
      boundaryGap: false,
      axisLabel: { hideOverlap: true, formatter: value => shorten(String(value)) }
    },
    yAxis: { type: 'value', min: 0, axisLabel: { formatter: value => `${value} ${unit}` } },
    series: [
      { name: downName, type: 'line', showSymbol: false, areaStyle: { opacity: 0.12 }, data: points.map(point => scaled(point.down)) },
      { name: upName, type: 'line', showSymbol: false, data: points.map(point => scaled(point.up)) }
    ]
  }
}
