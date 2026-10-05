import { FIXED_NOW_MS } from './clock.js'

// A proxy node's traffic series (GET /api/v4/kernel/nodes/:id/traffic) and
// the answer of the credential rotation (POST /api/v4/kernel/agents/
// rotate-credentials), as the admin API gives them, derived from the fixed
// clock. The numbers are a smooth daily curve with a few spikes, so the chart
// looks like a node's day, and are the same on every run.
const HOUR = 3_600_000
const DAY = 86_400_000
const GIB = 1024 ** 3

function curve(index, scale) {
  const hour = ((index % 24) + 24) % 24
  const day = (Math.sin(((hour - 6) / 24) * Math.PI * 2) + 1.4) * scale
  const spike = index % 37 === 0 ? 1.8 : 1
  return Math.round(day * spike * (0.85 + ((index * 7919) % 13) / 40))
}

// The window the route would answer: since rounds down and until up to whole
// buckets (UTC hours; UTC days here, the e2e host runs in UTC), an omitted
// until is the end of the current bucket.
function window(query) {
  const day = query.granularity === 'day'
  const size = day ? DAY : HOUR
  const until = query.until ? Math.ceil(Number(query.until) / size) * size : (Math.floor(FIXED_NOW_MS / size) + 1) * size
  const span = day ? 30 : 24
  const since = query.since ? Math.floor(Number(query.since) / size) * size : until - span * size
  return { day, size, since, until }
}

// scenario: 'traffic' (default), 'empty' (no traffic at all) or 'error'.
export function nodeTrafficAnswer(nodeId, query = {}, scenario = 'traffic') {
  if (scenario === 'error') return { __status: 500, body: { error: { code: 'internal_error', message: 'the traffic store is unavailable' } } }
  const { day, size, since, until } = window(query)
  const points = []
  for (let start = since, index = 0; start < until; start += size, index += 1) {
    const empty = scenario === 'empty'
    points.push({
      start_unix_ms: start,
      up_bytes: empty ? 0 : curve(index, day ? 22 * GIB : 0.9 * GIB),
      down_bytes: empty ? 0 : curve(index + 5, day ? 90 * GIB : 3.6 * GIB)
    })
  }
  const total = points.reduce((all, point) => ({ up_bytes: all.up_bytes + point.up_bytes, down_bytes: all.down_bytes + point.down_bytes }), { up_bytes: 0, down_bytes: 0 })
  return {
    data: {
      node_id: Number(nodeId),
      granularity: day ? 'day' : 'hour',
      since_unix_ms: since,
      until_unix_ms: until,
      points,
      total
    }
  }
}

// A made-up credential: it enrolls nothing anywhere.
export const FIXTURE_CREDENTIAL = 'anixagt_E2eFixtureCredentialNotRealXXXXXXXXXXXXXXXXXXXX'

// `now` is what the page's clock reads, so the credential expires `ttl_seconds` after it.
export function rotateAnswer(body = {}, now = FIXED_NOW_MS) {
  const ttl = Number(body.ttl_seconds || 3600)
  return {
    data: {
      node: body.node,
      revoked: { certificates: 1, enrollments: 2, link_certificates: String(body.node).startsWith('forward-') ? 1 : 0 },
      api_key_rotated: Boolean(body.rotate_api_key),
      enrollment: { id: 'enr-e2e', node_kind: String(body.node).split('-')[0], method: 'enrollment_credential', expires_at: new Date(now + ttl * 1000).toISOString() },
      expires_at: new Date(now + ttl * 1000).toISOString(),
      credential: FIXTURE_CREDENTIAL
    }
  }
}
