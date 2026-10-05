// The readers and rules of the API tokens page (views/admin/security/apiTokens.js).
import { describe, expect, it } from 'vitest'
import {
  API_TOKEN_MAX_ACTIVE, API_TOKEN_NAME_MAX, DEFAULT_EXPIRY, EXPIRY_CHOICES, checkTokenName, classifyTokenRefusal, expiryDays,
  formatRecoveryCode, isActiveToken, isRecoveryCode, readApiToken, readApiTokens, tokenState
} from '@/views/admin/security/apiTokens'

const NOW = Date.UTC(2026, 9, 5, 12, 0, 0)
const DAY = 86_400_000

const ROW = {
  id: '6b0e1c52-7d6f-4c1a-9a43-0c8d2a9e5b11',
  user_id: 7,
  name: 'nightly export',
  scope: 'read',
  hint: 'k3Zq',
  expires_at: '2027-01-03T12:00:00Z',
  last_used_at: '2026-10-05T08:00:00Z',
  last_used_ip: '203.0.113.9',
  created_at: '2026-10-01T12:00:00Z',
  revoked_at: null
}

function refusal(status, code, message, headers) {
  return { response: { status, data: { error: { code, message } }, headers }, message: `Request failed with status code ${status}` }
}

describe('reading the list', () => {
  it('turns a row into times in milliseconds and keeps nothing that is not in the route', () => {
    expect(readApiToken(ROW)).toEqual({
      id: ROW.id, userId: 7, name: 'nightly export', scope: 'read', hint: 'k3Zq',
      expiresAt: Date.UTC(2027, 0, 3, 12), lastUsedAt: Date.UTC(2026, 9, 5, 8), lastUsedIp: '203.0.113.9',
      createdAt: Date.UTC(2026, 9, 1, 12), revokedAt: null, revokeReason: ''
    })
  })

  it('reads a token without expiry, without a last use and a revoked one', () => {
    const open = readApiToken({ ...ROW, expires_at: null, last_used_at: null, last_used_ip: undefined })
    expect(open).toMatchObject({ expiresAt: null, lastUsedAt: null, lastUsedIp: '' })
    const ended = readApiToken({ ...ROW, revoked_at: '2026-10-04T00:00:00Z', revoke_reason: 'admin_revoked' })
    expect(ended).toMatchObject({ revokedAt: Date.UTC(2026, 9, 4), revokeReason: 'admin_revoked' })
  })

  it('takes an unknown scope for read, the least a token can do', () => {
    expect(readApiToken({ ...ROW, scope: 'root' }).scope).toBe('read')
    expect(readApiToken({ ...ROW, scope: 'admin' }).scope).toBe('admin')
  })

  it('reads an array, a { data } wrapper, and nothing else, and drops rows without an id', () => {
    expect(readApiTokens([ROW, { name: 'no id' }]).map(token => token.id)).toEqual([ROW.id])
    expect(readApiTokens({ data: [ROW] })).toHaveLength(1)
    expect(readApiTokens(null)).toEqual([])
    expect(readApiTokens({ data: 'x' })).toEqual([])
  })
})

describe('the state of a token', () => {
  const token = extra => readApiToken({ ...ROW, ...extra })

  it('is active far from its expiry, and with none', () => {
    expect(tokenState(token(), NOW)).toBe('active')
    expect(tokenState(token({ expires_at: null }), NOW)).toBe('active')
  })

  it('is expiring within a week, expired from the moment it ends', () => {
    expect(tokenState(token({ expires_at: new Date(NOW + 7 * DAY).toISOString() }), NOW)).toBe('expiring')
    expect(tokenState(token({ expires_at: new Date(NOW + 7 * DAY + 1000).toISOString() }), NOW)).toBe('active')
    expect(tokenState(token({ expires_at: new Date(NOW).toISOString() }), NOW)).toBe('expired')
    expect(tokenState(token({ expires_at: new Date(NOW - DAY).toISOString() }), NOW)).toBe('expired')
  })

  it('is revoked whatever its expiry says', () => {
    expect(tokenState(token({ revoked_at: '2026-10-04T00:00:00Z', expires_at: new Date(NOW - DAY).toISOString() }), NOW)).toBe('revoked')
    expect(tokenState(token({ revoked_at: '2026-10-04T00:00:00Z' }), NOW)).toBe('revoked')
  })

  it('counts against the limit while active or expiring, not once expired or revoked', () => {
    expect(isActiveToken(token(), NOW)).toBe(true)
    expect(isActiveToken(token({ expires_at: new Date(NOW + DAY).toISOString() }), NOW)).toBe(true)
    expect(isActiveToken(token({ expires_at: new Date(NOW - DAY).toISOString() }), NOW)).toBe(false)
    expect(isActiveToken(token({ revoked_at: '2026-10-04T00:00:00Z' }), NOW)).toBe(false)
    expect(API_TOKEN_MAX_ACTIVE).toBe(25)
  })
})

describe('the name', () => {
  it('needs 1 to 100 printable characters, counted as characters', () => {
    expect(checkTokenName('nightly export')).toBe('')
    expect(checkTokenName('  ')).toBe('required')
    expect(checkTokenName(undefined)).toBe('required')
    expect(checkTokenName('x'.repeat(API_TOKEN_NAME_MAX))).toBe('')
    expect(checkTokenName('x'.repeat(API_TOKEN_NAME_MAX + 1))).toBe('tooLong')
    // 100 Chinese characters are 300 bytes and fine; so are 100 astral characters.
    expect(checkTokenName('夜'.repeat(100))).toBe('')
    expect(checkTokenName('😀'.repeat(100))).toBe('')
    expect(checkTokenName('😀'.repeat(101))).toBe('tooLong')
  })

  it('refuses control characters', () => {
    expect(checkTokenName('a\u0000b')).toBe('unprintable')
    expect(checkTokenName('a\nb')).toBe('unprintable')
    expect(checkTokenName('a\u2028b')).toBe('unprintable')
    expect(checkTokenName('a\u007fb')).toBe('unprintable')
  })
})

describe('the expiry', () => {
  it('offers 90 days first and ends with custom and never', () => {
    expect(DEFAULT_EXPIRY).toBe('90')
    expect(EXPIRY_CHOICES).toEqual(['30', '90', '180', '365', '730', 'custom', 'never'])
  })

  it('maps a choice to the days the route takes: 1 to 730, 0 for never', () => {
    expect(expiryDays('90')).toBe(90)
    expect(expiryDays('730')).toBe(730)
    expect(expiryDays('never')).toBe(0)
    expect(expiryDays('custom', '45')).toBe(45)
    expect(expiryDays('custom', ' 1 ')).toBe(1)
    expect(expiryDays('custom', '730')).toBe(730)
  })

  it('refuses a custom number the route would refuse', () => {
    for (const bad of ['', '0', '731', '-3', '1.5', 'abc', '1e2']) expect(expiryDays('custom', bad)).toBeNaN()
    expect(expiryDays('custom', undefined)).toBeNaN()
  })
})

describe('recovery codes', () => {
  it('are upper-cased and hyphenated, and checked as XXXX-XXXX', () => {
    expect(formatRecoveryCode('ab12cd34')).toBe('AB12-CD34')
    expect(formatRecoveryCode(' ab12-cd34 ')).toBe('AB12-CD34')
    expect(formatRecoveryCode('ab12')).toBe('AB12')
    expect(isRecoveryCode('AB12-CD34')).toBe(true)
    expect(isRecoveryCode('AB12CD34')).toBe(false)
    expect(isRecoveryCode('ab12-cd34')).toBe(false)
  })
})

describe('telling the route\'s refusals apart', () => {
  it('splits step_up_required by what the message asks for', () => {
    expect(classifyTokenRefusal(refusal(403, 'step_up_required', 'password is required')).kind).toBe('password_required')
    expect(classifyTokenRefusal(refusal(403, 'step_up_required', 'an MFA code is required')).kind).toBe('code_required')
    expect(classifyTokenRefusal(refusal(403, 'step_up_required', 'sign in again and retry within 10 minutes')).kind).toBe('sign_in_again')
  })

  it('names the other refusals of the create route', () => {
    expect(classifyTokenRefusal(refusal(403, 'step_up_failed', 'the password or code is not valid')).kind).toBe('step_up_failed')
    expect(classifyTokenRefusal(refusal(409, 'too_many_tokens', 'at most 25')).kind).toBe('too_many_tokens')
    expect(classifyTokenRefusal(refusal(403, 'not_an_administrator', 'no')).kind).toBe('not_an_administrator')
    expect(classifyTokenRefusal(refusal(400, 'invalid_request', 'scope must be read or admin')).kind).toBe('invalid_request')
  })

  it('names the refusals of the list and revoke routes', () => {
    expect(classifyTokenRefusal(refusal(403, 'super_admin_required', 'only a super administrator')).kind).toBe('super_admin_required')
    expect(classifyTokenRefusal(refusal(404, 'not_found', 'the API token does not exist or is not yours')).kind).toBe('not_found')
  })

  it('reads Retry-After from a plain object or from axios headers, and takes any 429 for rate limited', () => {
    expect(classifyTokenRefusal(refusal(429, 'step_up_rate_limited', 'later', { 'retry-after': '900' }))).toMatchObject({ kind: 'rate_limited', retryAfter: 900 })
    const axiosLike = { get: name => (name === 'retry-after' ? '120' : undefined) }
    expect(classifyTokenRefusal(refusal(429, 'step_up_rate_limited', 'later', axiosLike)).retryAfter).toBe(120)
    // A 429 from the router's limiter has no code; Retry-After may be missing.
    expect(classifyTokenRefusal({ response: { status: 429, data: {} } })).toMatchObject({ kind: 'rate_limited', retryAfter: 0 })
    expect(classifyTokenRefusal(refusal(429, 'step_up_rate_limited', 'later', { 'retry-after': 'soon' })).retryAfter).toBe(0)
  })

  it('keeps the route\'s message, or the transport\'s, for what it does not know', () => {
    expect(classifyTokenRefusal(refusal(500, 'step_up_unavailable', 'the re-authentication could not be checked'))).toMatchObject({
      kind: 'other', message: 'the re-authentication could not be checked'
    })
    expect(classifyTokenRefusal(new Error('Network Error'))).toMatchObject({ kind: 'other', message: 'Network Error', status: 0 })
    expect(classifyTokenRefusal(undefined).kind).toBe('other')
  })
})
