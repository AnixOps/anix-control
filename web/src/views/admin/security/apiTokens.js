// Readers and rules of the admin API tokens page (security/ApiTokens.vue,
// POST|GET /api/v4/kernel/api-tokens, DELETE /api/v4/kernel/api-tokens/:id;
// docs/reference/admin-api-tokens.md). Plain functions, so a test drives them
// without a component.

// The route's bounds.
export const API_TOKEN_MAX_ACTIVE = 25
export const API_TOKEN_NAME_MAX = 100
export const API_TOKEN_MAX_DAYS = 730

// A token that ends within this many days is "expiring".
export const EXPIRING_SOON_DAYS = 7

export const SCOPES = Object.freeze(['read', 'admin'])

// The expiry choices of the form. 90 days is what the guide recommends; "custom"
// asks for a number of days (1 to 730) and "never" is allowed but not advised.
export const EXPIRY_PRESETS = Object.freeze(['30', '90', '180', '365', '730'])
export const EXPIRY_CHOICES = Object.freeze([...EXPIRY_PRESETS, 'custom', 'never'])
export const DEFAULT_EXPIRY = '90'

const DAY = 86_400_000

function time(value) {
  if (!value) return null
  const at = Date.parse(value)
  return Number.isFinite(at) ? at : null
}

// One row of the list (or the record of a creation) as the page keeps it:
// times in milliseconds, `null` when absent. Never a secret: the route sends
// the last four characters (`hint`) and nothing else about the token.
export function readApiToken(row) {
  const value = row && typeof row === 'object' ? row : {}
  return {
    id: String(value.id || ''),
    userId: Number(value.user_id) || 0,
    ownerEmail: String(value.owner_email || ''),
    name: String(value.name || ''),
    scope: value.scope === 'admin' ? 'admin' : 'read',
    hint: String(value.hint || ''),
    expiresAt: time(value.expires_at),
    lastUsedAt: time(value.last_used_at),
    lastUsedIp: String(value.last_used_ip || ''),
    createdAt: time(value.created_at),
    revokedAt: time(value.revoked_at),
    revokeReason: String(value.revoke_reason || '')
  }
}

export function readApiTokens(answer) {
  const rows = Array.isArray(answer) ? answer : Array.isArray(answer?.data) ? answer.data : []
  return rows.map(readApiToken).filter(token => token.id)
}

// 'revoked', 'expired', 'expiring' (ends within a week) or 'active'. A
// revoked token stays revoked, whatever its expiry says.
export function tokenState(token, now = Date.now()) {
  if (token.revokedAt) return 'revoked'
  if (token.expiresAt !== null && token.expiresAt <= now) return 'expired'
  if (token.expiresAt !== null && token.expiresAt - now <= EXPIRING_SOON_DAYS * DAY) return 'expiring'
  return 'active'
}

// Counts against the limit of 25: neither revoked nor expired.
export function isActiveToken(token, now = Date.now()) {
  const state = tokenState(token, now)
  return state === 'active' || state === 'expiring'
}

// What the list shows for a token that was cut short: the route's
// revoke_reason (owner_revoked, admin_revoked, owner_not_admin).
export const REVOKE_REASONS = Object.freeze(['owner_revoked', 'admin_revoked', 'owner_not_admin'])

// The name's rules: 1 to 100 printable characters (counted as characters, not
// bytes). '' when it is fine, else 'required', 'tooLong' or 'unprintable'.
export function checkTokenName(value) {
  const name = String(value ?? '').trim()
  if (!name) return 'required'
  const characters = Array.from(name)
  if (characters.length > API_TOKEN_NAME_MAX) return 'tooLong'
  if (/[\u0000-\u001f\u007f-\u009f\u2028\u2029]/.test(name)) return 'unprintable'
  return ''
}

// The days to send for an expiry choice: a number from 1 to 730, 0 for "never"
// and NaN for a custom value that is not a whole number in range.
export function expiryDays(choice, custom = '') {
  if (choice === 'never') return 0
  const raw = choice === 'custom' ? String(custom ?? '').trim() : choice
  if (!/^\d+$/.test(raw)) return Number.NaN
  const days = Number(raw)
  return days >= 1 && days <= API_TOKEN_MAX_DAYS ? days : Number.NaN
}

// The refusals of the three routes, by what the page does about them. The
// kernel answers { error: { code, message } }.
//
//   sign_in_again        403 step_up_sign_in_stale: creating a token needs a
//                        sign-in at most ten minutes old
//   too_many_tokens      409 25 active tokens
//   super_admin_required 403 listing other administrators' tokens
//   not_an_administrator 403 the owner is no longer an administrator
//   invalid_request      400
//   not_found            404 revoking a token that is gone or not yours
//   other                anything else (`message` is the route's, or the transport's)
export function classifyTokenRefusal(error) {
  const status = Number(error?.response?.status) || 0
  const failure = error?.response?.data?.error
  const code = (failure && typeof failure === 'object' ? failure.code : '') || ''
  const message = (typeof failure === 'string' ? failure : failure?.message) || error?.response?.data?.message || error?.message || ''
  const out = { kind: 'other', status, code, message }

  if (code === 'step_up_sign_in_stale') {
    out.kind = 'sign_in_again'
  } else if (code === 'too_many_tokens' || status === 409) {
    out.kind = 'too_many_tokens'
  } else if (code === 'super_admin_required') {
    out.kind = 'super_admin_required'
  } else if (code === 'not_an_administrator') {
    out.kind = 'not_an_administrator'
  } else if (code === 'invalid_request' || status === 400) {
    out.kind = 'invalid_request'
  } else if (code === 'not_found' || status === 404) {
    out.kind = 'not_found'
  }
  return out
}
