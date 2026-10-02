import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { setLocale } from '@/i18n'
import { createFormatter, formatBytes, toDate, useFormat } from '../composables/useFormat'

// The formatBytes copy found in Dashboard.vue, Plans.vue, Subscriptions.vue,
// Nodes.vue and Users.vue (the most common of the eleven), verbatim.
function legacyFormatBytes(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(2)} ${units[index]}`
}

// Deterministic pseudo-random values across every unit.
function* sampleBytes() {
  yield* [0, 1, 512, 1000, 1023, 1024, 1536, 10239, 1048575, 1048576, 5e8, 137975824384, 1099511627776, 5.5e15]
  let seed = 42
  for (let i = 0; i < 2000; i += 1) {
    seed = (seed * 1103515245 + 12345) % 2147483648
    const exponent = seed % 46
    yield Math.floor((seed / 2147483648) * 2 ** exponent)
  }
}

describe('useFormat bytes', () => {
  it('matches the legacy formatBytes for every non-negative value, in both locales', () => {
    for (const locale of ['zh-CN', 'en']) {
      const { bytes } = createFormatter(locale)
      for (const value of sampleBytes()) {
        expect(bytes(value), `${locale} ${value}`).toBe(legacyFormatBytes(value))
      }
      for (const missing of [null, undefined, '', NaN]) {
        expect(bytes(missing)).toBe(legacyFormatBytes(missing))
      }
    }
  })

  it('accepts numeric strings and other precisions', () => {
    const { bytes } = createFormatter('en')
    expect(bytes('1536')).toBe('1.50 KB')
    expect(bytes(137975824384, { precision: 1 })).toBe('128.5 GB')
    expect(bytes(-2048)).toBe('-2.00 KB')
  })

  it('is exported as a plain helper that follows the current locale', async () => {
    await setLocale('zh-CN')
    expect(formatBytes(1536)).toBe('1.50 KB')
    await setLocale('en')
    expect(formatBytes(1536)).toBe('1.50 KB')
  })
})

describe('useFormat other formatters', () => {
  const zh = createFormatter('zh-CN')
  const en = createFormatter('en')

  it('formats rates in decimal bits', () => {
    expect(en.rate(12500000)).toBe('100 Mbps')
    expect(en.rate(125)).toBe('1 Kbps')
    expect(en.rate(100)).toBe('800 bps')
    expect(en.rate(500, { input: 'mbps' })).toBe('500 Mbps')
    expect(en.rate(2500, { input: 'mbps' })).toBe('2.5 Gbps')
    expect(en.rate(null)).toBe('—')
  })

  it('formats durations with the two largest units', () => {
    expect(zh.duration(273600)).toBe('3 天 4 小时')
    expect(en.duration(273600)).toBe('3d 4h')
    expect(en.duration(273600, { style: 'long' })).toBe('3 days 4 hours')
    expect(en.duration(3600 * 24 + 5)).toBe('1d')
    expect(en.duration(61, { style: 'long' })).toBe('1 minute 1 second')
    expect(zh.duration(0)).toBe('0 秒')
    expect(en.duration(-1)).toBe('—')
  })

  it('formats money from cents with the ¥ symbol in both languages', () => {
    expect(zh.money(128800)).toBe('¥1,288.00')
    expect(en.money(128800)).toBe('¥1,288.00')
    expect(en.money(12.5, { cents: false })).toBe('¥12.50')
    expect(en.money(999, { currency: 'USD' })).toBe('$9.99')
    expect(en.money(undefined)).toBe('—')
  })

  it('formats numbers and percents', () => {
    expect(en.number(1208)).toBe('1,208')
    expect(zh.percent(0.64)).toBe('64%')
    expect(en.percent(0.6449, { precision: 1 })).toBe('64.5%')
  })

  it('writes dates as 2026-11-30 and times in 24 hours, in the local time zone', () => {
    const date = new Date(2026, 10, 30, 14, 5)
    for (const f of [zh, en]) {
      expect(f.date(date)).toBe('2026-11-30')
      expect(f.dateTime(date)).toBe('2026-11-30 14:05')
    }
    // Epoch seconds, milliseconds and digit strings, as the API returns them.
    const seconds = Math.floor(date.getTime() / 1000)
    expect(en.date(seconds)).toBe('2026-11-30')
    expect(en.date(String(seconds))).toBe('2026-11-30')
    expect(en.date(date.getTime())).toBe('2026-11-30')
    expect(en.date(0)).toBe('—')
    expect(en.date('not a date')).toBe('—')
    expect(en.date(null, { empty: '' })).toBe('')
    expect(toDate(seconds).getTime()).toBe(seconds * 1000)
  })

  it('writes relative times with a space after the number in Chinese', () => {
    const now = new Date(2026, 9, 1, 12, 0, 0).getTime()
    expect(zh.relativeTime(now - 180000, { now })).toBe('3 分钟前')
    expect(en.relativeTime(now - 180000, { now })).toBe('3 minutes ago')
    expect(zh.relativeTime(now - 10000, { now })).toBe('现在')
    expect(en.relativeTime(now - 10000, { now })).toBe('now')
    expect(zh.relativeTime(now - 2 * 3600000, { now })).toBe('2 小时前')
    expect(en.relativeTime(now + 3 * 86400000, { now })).toBe('in 3 days')
    expect(zh.relativeTime(now - 86400000, { now })).toBe('昨天')
    expect(en.relativeTime(now - 400 * 86400000, { now })).toBe('last year')
  })

  it('useFormat() follows the locale on every call', async () => {
    const f = useFormat()
    await setLocale('zh-CN')
    await nextTick()
    expect(f.duration(90061)).toBe('1 天 1 小时')
    await setLocale('en')
    expect(f.duration(90061)).toBe('1d 1h')
  })
})
