// Security → API tokens: the admin API tokens of the signed-in administrator
// (id 1) and of two others, answered the way the kernel does
// (GET|POST /api/v4/kernel/api-tokens, DELETE /api/v4/kernel/api-tokens/:id).
// Times are offsets from `now` (what the page's clock reads), so the list reads
// the same on every run. The fixture keeps a little state per opened screen
// (tokens created and revoked in it), reset by `setup`.
import { FIXED_NOW_MS } from './clock.js'

const HOUR = 3_600_000
const DAY = 86_400_000

// A made-up token: 8 + 43 characters like the real ones, and it opens nothing.
export const FIXTURE_TOKEN = `anixadm_${'E2eFixtureTokenNotReal'.padEnd(43, 'X')}`
// What the fixture accepts as the re-authentication.
export const FIXTURE_PASSWORD = 'correct-horse-battery'
export const FIXTURE_CODE = '123456'
export const FIXTURE_RECOVERY = 'AB12-CD34'

const ME = 1

function at(now, offset) {
  return offset === null ? null : new Date(now + offset).toISOString()
}

// [id, user, name, scope, hint, created, expires, lastUsed, ip, revoked, reason]
const SPEC = [
  ['6b0e1c52-0001', ME, 'nightly export', 'read', 'k3Zq', -12 * DAY, 78 * DAY, -3 * HOUR, '203.0.113.9', null, ''],
  ['6b0e1c52-0002', ME, 'deploy bot', 'admin', 'p8Lw', -27 * DAY, 3 * DAY, -2 * DAY, '2001:db8::7', null, ''],
  ['6b0e1c52-0003', ME, 'grafana probe', 'read', 'v2Hd', -60 * DAY, null, null, '', null, ''],
  ['6b0e1c52-0004', ME, 'ci runner', 'admin', 'x9Rt', -9 * DAY, 171 * DAY, -5 * 60_000, '198.51.100.24', null, ''],
  ['6b0e1c52-0005', ME, 'last year’s export', 'read', 'm4Bn', -400 * DAY, -35 * DAY, -36 * DAY, '203.0.113.9', null, ''],
  ['6b0e1c52-0006', ME, 'leaked laptop', 'read', 'c7Js', -45 * DAY, 45 * DAY, -21 * DAY, '192.0.2.55', -20 * DAY, 'owner_revoked'],
  ['6b0e1c52-0007', 3, 'ops dashboard', 'read', 'f1Yu', -30 * DAY, 60 * DAY, -HOUR, '203.0.113.40', null, ''],
  ['6b0e1c52-0008', 9, 'departed admin’s script', 'admin', 'a5Ge', -80 * DAY, 100 * DAY, -6 * DAY, '192.0.2.8', -4 * DAY, 'owner_not_admin'],
  ['6b0e1c52-0009', 3, 'migration job', 'admin', 'n6Kp', -50 * DAY, 20 * DAY, -11 * DAY, '203.0.113.41', -10 * DAY, 'admin_revoked']
]

function row(now, [id, user, name, scope, hint, created, expires, lastUsed, ip, revoked, reason]) {
  return {
    id, user_id: user, name, scope, hint,
    expires_at: at(now, expires), last_used_at: at(now, lastUsed), ...(ip ? { last_used_ip: ip } : {}),
    created_at: at(now, created), revoked_at: at(now, revoked), ...(reason ? { revoke_reason: reason } : {})
  }
}

// 25 active tokens of the signed-in administrator (the limit).
function fullSet(now) {
  return Array.from({ length: 25 }, (_, index) => row(now, [`full-${index}`, ME, `automation ${index + 1}`, 'read', `q${String(index).padStart(3, '0')}`, -index * DAY, 90 * DAY, null, '', null, '']))
}

let state = { created: [], revoked: {} }

function refusal(status, code, message, headers) {
  return { __status: status, headers, body: { error: { code, message } } }
}

function isRevoked(token) {
  return Boolean(token.revoked_at)
}

function list(now, query, scenario) {
  const base = scenario === 'full' ? fullSet(now) : SPEC.map(spec => row(now, spec))
  const all = [...state.created.map(token => ({ ...token })), ...base].map(token => (
    token.id in state.revoked ? { ...token, revoked_at: at(now, state.revoked[token.id]), revoke_reason: 'owner_revoked' } : token
  ))
  const wantAll = query.all === 'true'
  const ended = query.include_inactive === 'true'
  return all
    .filter(token => wantAll || token.user_id === ME)
    .filter(token => ended || (!isRevoked(token) && !(token.expires_at && Date.parse(token.expires_at) <= now)))
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
}

// What the route checks before it mints a token, by scenario.
function create(now, body, scenario) {
  if (scenario === 'cutover') return refusal(403, 'step_up_required', 'sign in again and retry within 10 minutes')
  if (scenario === 'tooMany') return refusal(409, 'too_many_tokens', 'an administrator may hold at most 25 active API tokens: revoke one first')
  if (scenario === 'rateLimited') return refusal(429, 'step_up_rate_limited', 'too many failed re-authentications, try again later', { 'Retry-After': '900' })
  if (!body?.name || !body?.scope) return refusal(400, 'invalid_request', 'name and scope are required; expires_in_days is 1 to 730')
  const proof = body.password
    ? body.password === FIXTURE_PASSWORD
    : body.method === 'totp' ? body.code === FIXTURE_CODE : body.method === 'backup' && body.code === FIXTURE_RECOVERY
  if (!body.password && !body.code) {
    return refusal(403, 'step_up_required', scenario === 'mfa' ? 'an MFA code is required' : 'password is required')
  }
  if (!proof) return refusal(403, 'step_up_failed', 'the password or code is not valid')
  const days = Number(body.expires_in_days || 0)
  const record = row(now, [`created-${state.created.length + 1}`, ME, body.name, body.scope, FIXTURE_TOKEN.slice(-4), 0, days ? days * DAY : null, null, '', null, ''])
  state.created.push(record)
  return { __status: 201, headers: { 'Cache-Control': 'no-store' }, body: { data: { token: FIXTURE_TOKEN, api_token: record } } }
}

export default {
  path: '/admin/security/api-tokens',
  async setup() {
    state = { created: [], revoked: {} }
  },
  api(path, { method, query, body, scenario, now = FIXED_NOW_MS }) {
    if (path === '/api/v2/user/mfa/status') return { code: 0, data: { enabled: scenario === 'mfa' } }
    if (path === '/api/v4/kernel/api-tokens' && method === 'GET') {
      if (scenario === 'error') return refusal(503, 'database_unavailable', 'the database is unavailable')
      if (scenario === 'empty') return { data: [] }
      if (scenario === 'notSuper' && query.all === 'true') return refusal(403, 'super_admin_required', 'only a super administrator may list other administrators’ API tokens')
      return { data: list(now, query, scenario) }
    }
    if (path === '/api/v4/kernel/api-tokens' && method === 'POST') return create(now, body, scenario)
    const match = path.match(/^\/api\/v4\/kernel\/api-tokens\/([^/]+)$/)
    if (match && method === 'DELETE') {
      if (scenario === 'revokeFails') return refusal(500, 'database_error', 'database operation failed')
      if (scenario === 'revokeGone') return refusal(404, 'not_found', 'the API token does not exist or is not yours')
      const [token] = list(now, { all: 'true', include_inactive: 'true' }, scenario).filter(item => item.id === match[1])
      if (!token) return refusal(404, 'not_found', 'the API token does not exist or is not yours')
      const changed = !token.revoked_at
      if (changed) state.revoked[token.id] = 0
      return { data: { api_token: { ...token, revoked_at: at(now, state.revoked[token.id] ?? 0) }, changed } }
    }
    return undefined
  },
  // The screens that start with an open dialog or a switched list.
  async after(page, { scenario }) {
    if (scenario === 'all') {
      await page.getByRole('button', { name: 'Revoked and expired' }).click()
      await page.getByRole('button', { name: 'All administrators' }).click()
      // A table on a desktop, cards on a phone: wait for a row of another owner.
      await page.getByText(/departed admin/).first().waitFor()
      return
    }
    if (scenario !== 'create' && scenario !== 'created') return
    await page.getByTestId('api-token-create-open').click()
    await page.getByTestId('api-token-form-dialog').waitFor()
    if (scenario === 'create') return
    await page.getByRole('textbox', { name: /^Name/ }).fill('nightly export')
    await page.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await page.getByTestId('api-token-submit').click()
    await page.getByTestId('api-token-result').waitFor()
  }
}
