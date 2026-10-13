import { FIXED_NOW_MS } from './clock.js'

// The diagnostics of a node (GET and POST /api/v4/kernel/nodes/:id/
// diagnostics) and the Agent upgrade plan (GET /api/v4/kernel/agents/
// upgrade-plan), derived from the fixed clock.
const MINUTE = 60_000
const iso = offset => new Date(FIXED_NOW_MS + offset).toISOString()

const GENERIC = [
  { action: 'log_tail', mutating: false, params: ['service', 'lines'], forward: false },
  { action: 'service_restart', mutating: true, params: ['service'], forward: false },
  { action: 'service_status', mutating: false, params: ['service'], forward: false }
]
const CHECKS = ['forward.connect', 'forward.listen', 'forward.port_conflict', 'forward.udp_probe']
  .map(action => ({ action, mutating: false, params: ['route_id', 'hop_index'], forward: true }))

const STATUS_OUTPUT = [
  '● gost.service - gost forwarding engine',
  '     Loaded: loaded (/etc/systemd/system/gost.service; enabled; preset: enabled)',
  '     Active: active (running) since Thu 2026-10-01 08:00:12 UTC; 23h ago',
  '   Main PID: 1182 (gost)',
  '      Tasks: 9 (limit: 4623)',
  '     Memory: 31.4M'
].join('\n')

const RECENT = [
  {
    task_id: 'diag-0a41', action: 'log_tail', params: { service: 'gost', lines: 100 }, status: 'completed', success: true,
    output: 'Oct 02 07:41:03 hk-01 gost[1182]: listener 0.0.0.0:443 accepted\nOct 02 07:41:09 hk-01 gost[1182]: upstream 198.51.100.7:8443 healthy',
    duration_ms: 118, created_at: iso(-19 * MINUTE), updated_at: iso(-19 * MINUTE)
  },
  {
    task_id: 'diag-09c7', action: 'service_status', params: { service: 'gost' }, status: 'completed', success: true,
    output: STATUS_OUTPUT, duration_ms: 182, created_at: iso(-3 * 60 * MINUTE), updated_at: iso(-3 * 60 * MINUTE)
  },
  {
    task_id: 'diag-0951', action: 'service_restart', params: { service: 'gost' }, status: 'failed', success: false,
    output: '', error: 'Failed to restart gost.service: Unit gost.service not found.', duration_ms: 64,
    created_at: iso(-26 * 60 * MINUTE), updated_at: iso(-26 * 60 * MINUTE)
  }
]

// ref: proxy-<id> or forward-<id>. scenario: 'offline' answers a node whose
// Agent is not connected; anything else a connected Agent that runs
// diagnostics (and, for a forward node, the forward checks).
export function nodeDiagnosticsAnswer(ref, { method = 'GET', scenario = '', body = null } = {}) {
  const forward = ref.startsWith('forward-')
  if (method === 'POST') {
    const action = body?.action
    if (action === 'service_restart') return { data: { task_id: 'diag-0b02', node: ref, action, params: body.params, success: true, output: '', error: '', duration_ms: 1840 } }
    if (action === 'log_tail') {
      return { data: { task_id: 'diag-0b03', node: ref, action, params: body.params, success: true, output: RECENT[0].output, error: '', duration_ms: 118 } }
    }
    if (String(action).startsWith('forward.')) {
      return {
        data: {
          task_id: 'diag-0b04', node: ref, action, params: body.params, success: true, output: '2 of 2 upstreams reachable', error: '', duration_ms: 46,
          result: {
            check: action, route_id: body.params?.route_id, hop_index: body.params?.hop_index, status: 'ok',
            items: [
              { target: '198.51.100.7:8443', protocol: 'tcp', status: 'ok', rtt_us: 12400 },
              { target: '198.51.100.8:8443', protocol: 'tcp', status: 'ok', rtt_us: 15800 }
            ]
          }
        }
      }
    }
    return { data: { task_id: 'diag-0b01', node: ref, action, params: body?.params, success: true, output: STATUS_OUTPUT, error: '', duration_ms: 182 } }
  }
  const offline = scenario === 'offline'
  return {
    data: {
      node: ref,
      connected: !offline,
      agent_version: offline ? '' : '4.2.0',
      diagnostics: !offline,
      forward_checks: !offline && forward,
      actions: offline ? [] : [...GENERIC, ...(forward ? CHECKS : [])],
      services: ['gost'],
      log_tail_max: 1000,
      timeout_seconds: { default: 30, max: 120 },
      can_restart: true,
      history: !forward,
      tasks: forward ? [] : RECENT
    }
  }
}

export function agentUpgradePlanAnswer(overrides = {}) {
  return {
    data: {
      target_version: 'v4.2.0',
      control_version: 'v4.2.0',
      configured: true,
      unconfigured_reason: '',
      eligible_nodes: 12,
      batches: [
        { percent: 5, min_duration_seconds: 1800 },
        { percent: 25, min_duration_seconds: 1800 },
        { percent: 100, min_duration_seconds: 1800 }
      ],
      min_batch_seconds: 1800,
      first_batch_max_percent: 5,
      failure_threshold_percent: 5,
      reconnect_timeout_seconds: 600,
      active_campaign_id: '',
      can_start: true,
      ...overrides
    }
  }
}
