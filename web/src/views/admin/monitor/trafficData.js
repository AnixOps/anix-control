// Readers for the hourly traffic and user ranking answers
// (GET /admin/traffic/hourly, /admin/traffic/user-ranking), shared by
// 流量与监控 and the dashboard, and the ECharts option they draw with.
// The API answers a panel envelope ({ code, data }), axios ({ data }) or the
// list itself; traffic values may arrive as strings. No requests here.

export function normalizeTrafficValue(value) {
  const numeric = Number(value)
  if (!Number.isFinite(numeric) || numeric <= 0) return 0
  return numeric
}

export function normalizePoint(item) {
  return {
    hour_ts: Number(item?.hour_ts || 0),
    traffic: normalizeTrafficValue(item?.traffic)
  }
}

export function normalizeRankingRow(item) {
  return {
    user_id: Number(item?.user_id || 0),
    email: item?.email || '',
    traffic: normalizeTrafficValue(item?.traffic)
  }
}

export function extractResponsePayload(res) {
  if (res && typeof res === 'object' && Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? {}
  }
  return res ?? {}
}

function listOf(payload) {
  if (Array.isArray(payload?.list)) return payload.list
  if (Array.isArray(payload?.data)) return payload.data
  return Array.isArray(payload) ? payload : []
}

export function readTrafficSeries(res) {
  return listOf(extractResponsePayload(res)).map(normalizePoint)
}

export function readTrafficMeta(res) {
  const payload = extractResponsePayload(res)
  return payload?.meta || res?.meta || {}
}

export function readRankingRows(res) {
  return listOf(extractResponsePayload(res)).map(normalizeRankingRow).filter(row => row.user_id > 0)
}

export function errorText(error, fallback) {
  return error?.response?.data?.message || error?.response?.data?.msg || error?.message || fallback
}

export function hasTraffic(points) {
  return points.some(point => point.traffic > 0)
}

export function trafficTotal(points) {
  return points.reduce((sum, point) => sum + point.traffic, 0)
}

export function peakPoint(points) {
  if (!hasTraffic(points)) return null
  return points.reduce((max, point) => (point.traffic > (max?.traffic ?? -1) ? point : max), null)
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

// The unit the axis counts in: the largest that keeps the peak ≥ 1, so the
// ticks are round numbers of it (0, 1, 2 GB…) rather than of bytes.
export function axisUnit(points) {
  return axisUnitFor(Math.max(0, ...points.map(point => point.traffic)))
}

// The same, for a peak in bytes (the node traffic chart has two series).
export function axisUnitFor(max) {
  let index = 0
  while (max >= 1024 ** (index + 1) && index < BYTE_UNITS.length - 1) index += 1
  return { unit: BYTE_UNITS[index], scale: 1024 ** index }
}

// A line with a soft area: colours come from the chart theme (UiChart).
// `label(ts)` names an hour on the axis; `bytes(value)` formats a value in
// the tooltip. The series is in the axis unit.
export function trafficOption(points, { label, bytes, seriesName, rotate = 0 }) {
  const { unit, scale } = axisUnit(points)
  return {
    tooltip: { trigger: 'axis', valueFormatter: value => bytes(Number(value) * scale) },
    grid: { left: 8, right: 16, top: 16, bottom: 8, containLabel: true },
    xAxis: {
      type: 'category',
      data: points.map(point => label(point.hour_ts)),
      boundaryGap: false,
      axisLabel: { rotate, hideOverlap: true }
    },
    yAxis: {
      type: 'value',
      axisLabel: { formatter: value => `${value} ${unit}` }
    },
    series: [
      {
        name: seriesName,
        type: 'line',
        smooth: true,
        showSymbol: false,
        areaStyle: { opacity: 0.12 },
        data: points.map(point => Number((point.traffic / scale).toFixed(3)))
      }
    ]
  }
}
