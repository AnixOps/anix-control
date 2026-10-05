import { describe, expect, it } from 'vitest'
import { parseRetryAfter, readTelegramTestRefusal, telegramTestClass, telegramTestTone } from '@/utils/telegramTest'

describe('Telegram test helpers', () => {
  it('shows each result class in its tone and an unknown class as danger', () => {
    expect(telegramTestTone('ok')).toBe('success')
    for (const cls of ['chat_not_found', 'bot_blocked', 'rate_limited', 'not_configured']) expect(telegramTestTone(cls)).toBe('warning')
    for (const cls of ['invalid_token', 'network_error', 'unknown']) expect(telegramTestTone(cls)).toBe('danger')
    expect(telegramTestClass('something_new')).toBe('unknown')
    expect(telegramTestClass('toString')).toBe('unknown')
    expect(telegramTestTone(undefined)).toBe('danger')
  })

  it('reads Retry-After as seconds or a date, and bounds it', () => {
    expect(parseRetryAfter('17')).toBe(17)
    expect(parseRetryAfter(' 0 ')).toBe(1)
    expect(parseRetryAfter('999999')).toBe(3600)
    expect(parseRetryAfter('')).toBe(60)
    expect(parseRetryAfter(undefined)).toBe(60)
    expect(parseRetryAfter('soon')).toBe(60)
    expect(parseRetryAfter('Wed, 21 Oct 2026 07:28:30 GMT', Date.parse('Wed, 21 Oct 2026 07:28:00 GMT'))).toBe(30)
  })

  it('tells the refusals apart', () => {
    const refusal = (status, code, headers) => ({ response: { status, data: { error: { code } }, headers } })
    expect(readTelegramTestRefusal(refusal(409, 'telegram_not_bound'))).toEqual({ kind: 'not_bound', retryAfter: 0 })
    expect(readTelegramTestRefusal(refusal(403, 'chat_not_allowed')).kind).toBe('not_allowed')
    expect(readTelegramTestRefusal(refusal(403, 'forbidden')).kind).toBe('failed')
    expect(readTelegramTestRefusal(refusal(429, 'rate_limited', { 'retry-after': '12' }))).toEqual({ kind: 'rate_limited', retryAfter: 12 })
    expect(readTelegramTestRefusal(refusal(429, 'rate_limited', { 'Retry-After': '9' })).retryAfter).toBe(9)
    expect(readTelegramTestRefusal(refusal(429, 'rate_limited', new Headers({ 'retry-after': '7' }))).retryAfter).toBe(7)
    expect(readTelegramTestRefusal(refusal(429, 'rate_limited', {})).retryAfter).toBe(60)
    expect(readTelegramTestRefusal(refusal(502)).kind).toBe('failed')
    expect(readTelegramTestRefusal(new Error('timeout')).kind).toBe('failed')
  })
})
