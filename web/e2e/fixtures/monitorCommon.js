import { FIXED_NOW_MS } from './clock.js'
export const NOW = Math.floor(FIXED_NOW_MS / 1000)
export const HOUR = Math.floor(NOW / 3600) * 3600
export function hourly(hours, scale = 1) {
  return Array.from({ length: hours }, (_, index) => {
    const ts = HOUR - (hours - 1 - index) * 3600
    const h = new Date(ts * 1000).getUTCHours()
    const base = (Math.sin((h - 6) / 24 * Math.PI * 2) + 1.3) * 1.6e9 * scale
    return { hour_ts: ts, traffic: Math.round(base + ((index * 7919) % 13) * 6e7) }
  })
}
