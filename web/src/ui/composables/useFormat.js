// One formatter for bytes, rates, durations, money, numbers, dates and
// relative times, built on Intl and the current vue-i18n locale.
//
// useFormat() returns functions that read the locale on every call, so a
// template that calls them re-renders when the locale changes.
// createFormatter(locale) gives the same functions for a fixed locale (tests,
// non-component code).
//
// Rules (docs/reference/frontend-design.md, guidelines/voice-and-tone.md):
// - a space between a number and its unit, in both languages: 128.4 GB,
//   500 Mbps, 30 天, 3 分钟前;
// - dates are 2026-11-30 and times 24-hour 14:05 in every locale;
// - missing values render as an em dash (option `empty`).
import i18n from '@/i18n'

export const EMPTY = '—'

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB']
const RATE_UNITS = ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps']

const cache = new Map()

function numberFormat(locale, options) {
  const key = `${locale}|${JSON.stringify(options)}`
  let format = cache.get(key)
  if (!format) {
    format = new Intl.NumberFormat(locale, options)
    cache.set(key, format)
  }
  return format
}

function toNumber(value) {
  if (value === null || value === undefined || value === '') return NaN
  return typeof value === 'number' ? value : Number(value)
}

// Accepts a Date, epoch seconds or milliseconds (number or digit string, as
// the API returns both) or a date string. Same rules as useAppI18n.
export function toDate(value) {
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    if (value <= 0) return null
    return new Date(value > 1e12 ? value : value * 1000)
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return null
    if (/^\d+$/.test(trimmed)) return toDate(Number(trimmed))
    const date = new Date(trimmed)
    return Number.isNaN(date.getTime()) ? null : date
  }
  return null
}

function isChinese(locale) {
  return String(locale).toLowerCase().startsWith('zh')
}

// Intl writes zh-CN units without a space (3分钟前); the voice rules want one.
function spaceAfterNumber(parts, locale) {
  if (!isChinese(locale)) return parts.map(part => part.value).join('')
  let out = ''
  parts.forEach((part, index) => {
    const previous = parts[index - 1]
    const numeric = previous && ['integer', 'fraction', 'decimal', 'group'].includes(previous.type)
    if (numeric && part.type === 'literal' && !/^\s/.test(part.value)) {
      out += ' '
    }
    out += part.value
  })
  return out
}

export function createFormatter(locale = 'zh-CN', translate = (key, n) => i18n.global.t(key, n, { locale })) {
  // Bytes in binary steps (1 KB = 1024 B) with the unit names the product
  // already uses. Defaults match the formatBytes copies in the pages:
  // two decimals, '0 B' for zero or a missing value. Hero and summary
  // numbers (user home, dashboard cards) pass { precision: 1 }.
  function bytes(value, { precision = 2, empty = '0 B' } = {}) {
    const n = toNumber(value)
    if (!Number.isFinite(n) || n === 0) return empty
    let scaled = Math.abs(n)
    let index = 0
    while (scaled >= 1024 && index < BYTE_UNITS.length - 1) {
      scaled /= 1024
      index += 1
    }
    const text = numberFormat(locale, {
      minimumFractionDigits: precision,
      maximumFractionDigits: precision,
      useGrouping: 'min2'
    }).format(scaled)
    return `${n < 0 ? '-' : ''}${text} ${BYTE_UNITS[index]}`
  }

  // Network rates in decimal bit steps (1 Mbps = 1 000 000 bit/s).
  // `input`: 'bytes' (bytes per second, the default), 'bits' or 'mbps'.
  function rate(value, { input = 'bytes', precision = 1, empty = EMPTY } = {}) {
    const n = toNumber(value)
    if (!Number.isFinite(n)) return empty
    let bits = n
    if (input === 'bytes') bits = n * 8
    if (input === 'mbps') bits = n * 1e6
    let scaled = Math.abs(bits)
    let index = 0
    while (scaled >= 1000 && index < RATE_UNITS.length - 1) {
      scaled /= 1000
      index += 1
    }
    const text = numberFormat(locale, {
      maximumFractionDigits: index === 0 ? 0 : precision,
      useGrouping: 'min2'
    }).format(scaled)
    return `${bits < 0 ? '-' : ''}${text} ${RATE_UNITS[index]}`
  }

  function number(value, { empty = EMPTY, ...options } = {}) {
    const n = toNumber(value)
    if (!Number.isFinite(n)) return empty
    return numberFormat(locale, options).format(n)
  }

  // `value` is a ratio: 0.42 → 42%.
  function percent(value, { precision = 0, empty = EMPTY } = {}) {
    const n = toNumber(value)
    if (!Number.isFinite(n)) return empty
    return numberFormat(locale, { style: 'percent', maximumFractionDigits: precision, minimumFractionDigits: precision }).format(n)
  }

  // Money. Amounts from the API are in cents (fen) unless `cents: false`.
  // The narrow symbol keeps ¥ in both languages (en would print CN¥).
  function money(value, { currency = 'CNY', cents = true, empty = EMPTY } = {}) {
    const n = toNumber(value)
    if (!Number.isFinite(n)) return empty
    return numberFormat(locale, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    }).format(cents ? n / 100 : n)
  }

  // Durations from seconds: the two largest non-zero units, "3 天 4 小时" /
  // "3d 4h" (style 'short', the default) or "3 days 4 hours" (style 'long').
  function duration(seconds, { style = 'short', units = 2, empty = EMPTY } = {}) {
    const n = toNumber(seconds)
    if (!Number.isFinite(n) || n < 0) return empty
    const steps = [['day', 86400], ['hour', 3600], ['minute', 60], ['second', 1]]
    const group = style === 'long' ? 'duration' : 'durationShort'
    let rest = Math.floor(n)
    const out = []
    for (const [unit, size] of steps) {
      const amount = Math.floor(rest / size)
      rest -= amount * size
      if (amount > 0) out.push(translate(`ui.format.${group}.${unit}`, amount))
      else if (out.length > 0) break
      if (out.length === units) break
    }
    if (out.length === 0) return translate(`ui.format.${group}.second`, 0)
    return out.join(' ')
  }

  function dateParts(date, withTime) {
    const options = { year: 'numeric', month: '2-digit', day: '2-digit', calendar: 'gregory', numberingSystem: 'latn' }
    if (withTime) Object.assign(options, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
    const key = `${locale}|date|${withTime}`
    let format = cache.get(key)
    if (!format) {
      format = new Intl.DateTimeFormat(locale, options)
      cache.set(key, format)
    }
    const parts = Object.fromEntries(format.formatToParts(date).map(part => [part.type, part.value]))
    return parts
  }

  // 2026-11-30 in every locale (voice-and-tone.md), in the viewer's time zone.
  function date(value, { empty = EMPTY } = {}) {
    const d = toDate(value)
    if (!d) return empty
    const p = dateParts(d, false)
    return `${p.year}-${p.month}-${p.day}`
  }

  // 2026-11-30 14:05, 24-hour.
  function dateTime(value, { empty = EMPTY } = {}) {
    const d = toDate(value)
    if (!d) return empty
    const p = dateParts(d, true)
    return `${p.year}-${p.month}-${p.day} ${p.hour}:${p.minute}`
  }

  // "3 分钟前" / "3 minutes ago". Show dateTime(value) on hover.
  function relativeTime(value, { now = Date.now(), empty = EMPTY } = {}) {
    const d = toDate(value)
    if (!d) return empty
    const diff = (d.getTime() - toNumberNow(now)) / 1000
    const abs = Math.abs(diff)
    const steps = [
      ['second', 60, 1],
      ['minute', 3600, 60],
      ['hour', 86400, 3600],
      ['day', 86400 * 30, 86400],
      ['month', 86400 * 365, 86400 * 30],
      ['year', Infinity, 86400 * 365]
    ]
    const key = `${locale}|relative`
    let format = cache.get(key)
    if (!format) {
      format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
      cache.set(key, format)
    }
    if (abs < 45) return format.format(0, 'second')
    for (const [unit, limit, size] of steps) {
      if (abs < limit) {
        return spaceAfterNumber(format.formatToParts(Math.round(diff / size), unit), locale)
      }
    }
    return empty
  }

  return { locale, bytes, rate, number, percent, money, duration, date, dateTime, relativeTime }
}

function toNumberNow(now) {
  return now instanceof Date ? now.getTime() : Number(now)
}

const NAMES = ['bytes', 'rate', 'number', 'percent', 'money', 'duration', 'date', 'dateTime', 'relativeTime']

const formatters = new Map()

function currentFormatter() {
  const locale = i18n.global.locale.value
  let formatter = formatters.get(locale)
  if (!formatter) {
    formatter = createFormatter(locale)
    formatters.set(locale, formatter)
  }
  return formatter
}

// The functions read i18n.global.locale on each call (a reactive read), so
// templates follow locale changes.
export function useFormat() {
  const api = {}
  for (const name of NAMES) {
    api[name] = (...args) => currentFormatter()[name](...args)
  }
  return api
}

// Plain helper for code that is not a component (and for the page copies of
// formatBytes, which U4 onwards replace with this one).
export function formatBytes(value, options) {
  return currentFormatter().bytes(value, options)
}
