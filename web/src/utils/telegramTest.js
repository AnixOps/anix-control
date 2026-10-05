// Helpers of the Telegram "send test message" button (通知 → Telegram):
// POST /api/v4/kernel/notifications/telegram/test answers a result class, and
// refuses with 409 telegram_not_bound, 403 chat_not_allowed or 429 +
// Retry-After. Plain functions so a page that mocks the API keeps them.

// The tone each result class is shown in. A class the server adds later is
// shown as `unknown`.
export const TELEGRAM_TEST_TONES = Object.freeze({
  ok: 'success',
  chat_not_found: 'warning',
  bot_blocked: 'warning',
  rate_limited: 'warning',
  not_configured: 'warning',
  invalid_token: 'danger',
  network_error: 'danger',
  unknown: 'danger'
})

export const TELEGRAM_TEST_NETWORK_REASONS = Object.freeze(['timeout', 'dns', 'tls', 'connect', 'canceled', 'other'])

// The limit is 5 a minute per administrator; without a usable Retry-After the
// button waits that minute.
const DEFAULT_RETRY_SECONDS = 60
const MAX_RETRY_SECONDS = 3600

export function telegramTestClass(value) {
  return Object.prototype.hasOwnProperty.call(TELEGRAM_TEST_TONES, value) ? value : 'unknown'
}

export function telegramTestTone(value) {
  return TELEGRAM_TEST_TONES[telegramTestClass(value)]
}

// Seconds from a Retry-After header: delta-seconds, or an HTTP date.
export function parseRetryAfter(value, now = Date.now()) {
  const text = String(value ?? '').trim()
  if (!text) return DEFAULT_RETRY_SECONDS
  let seconds = /^\d+$/.test(text) ? Number(text) : Math.ceil((Date.parse(text) - now) / 1000)
  if (!Number.isFinite(seconds)) seconds = DEFAULT_RETRY_SECONDS
  return Math.min(Math.max(seconds, 1), MAX_RETRY_SECONDS)
}

function headerOf(headers, name) {
  if (!headers) return ''
  if (typeof headers.get === 'function') return headers.get(name) || ''
  const key = Object.keys(headers).find(item => item.toLowerCase() === name)
  return key ? headers[key] : ''
}

// What a refused test request means: { kind: 'not_bound' | 'not_allowed' |
// 'rate_limited' | 'failed', retryAfter }. Read from the status and the
// route's error code only; the request, which carries no secret, is never
// inspected.
export function readTelegramTestRefusal(error, now = Date.now()) {
  const status = error?.response?.status
  const code = error?.response?.data?.error?.code
  if (status === 429) return { kind: 'rate_limited', retryAfter: parseRetryAfter(headerOf(error.response.headers, 'retry-after'), now) }
  if (status === 409 && (!code || code === 'telegram_not_bound')) return { kind: 'not_bound', retryAfter: 0 }
  if (status === 403 && code === 'chat_not_allowed') return { kind: 'not_allowed', retryAfter: 0 }
  return { kind: 'failed', retryAfter: 0 }
}
