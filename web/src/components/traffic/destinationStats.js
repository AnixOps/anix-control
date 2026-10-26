// Readers for the destination statistics answers
// (GET /api/v4/admin/traffic/destinations, /api/v4/user/traffic/destinations)
// and the colour of each category. Byte values may arrive as strings.

export const CATEGORY_COLORS = Object.freeze({
  video: 'var(--chart-4)',
  music: 'var(--chart-3)',
  messaging: 'var(--chart-2)',
  social: 'var(--chart-1)',
  gaming: 'var(--chart-5)',
  ai: 'var(--chart-6)',
  download: 'var(--chart-7)',
  web: 'var(--chart-8)',
  unknown: 'var(--label-3)'
})

function count(value) {
  const n = Number(value)
  return Number.isFinite(n) && n > 0 ? n : 0
}

function totals(row) {
  return {
    upload: count(row?.upload),
    download: count(row?.download),
    connections: count(row?.connections)
  }
}

export function bytesOf(row) {
  return count(row?.upload) + count(row?.download)
}

// The summary with every field present: totals, categories ({ category,
// upload, download, connections }), domains ({ domain, category, ... }),
// domain_count and settings ({ enabled, retention_days }).
export function readDestinationSummary(body) {
  const summary = body && typeof body === 'object' ? body : {}
  return {
    ...totals(summary),
    categories: (Array.isArray(summary.categories) ? summary.categories : [])
      .map(row => ({ category: String(row?.category || 'unknown'), ...totals(row) })),
    domains: (Array.isArray(summary.domains) ? summary.domains : [])
      .map(row => ({ domain: String(row?.domain || ''), category: String(row?.category || 'unknown'), ...totals(row) })),
    domain_count: count(summary.domain_count),
    settings: {
      enabled: Boolean(summary.settings?.enabled),
      retention_days: count(summary.settings?.retention_days) || 30
    }
  }
}
