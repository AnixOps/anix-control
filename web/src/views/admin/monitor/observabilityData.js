// Readers for the forward observability answers (targets, trend, topology,
// multi-ingress) and the runtime job list. The response interceptor
// returns the { code, msg, data } envelope; unwrap defensively, as the old
// Observability page did. No requests here.

export function extractPayload(res) {
  return res?.data?.data ?? res?.data ?? res
}

export function listOf(payload) {
  if (Array.isArray(payload?.list)) return payload.list
  return Array.isArray(payload) ? payload : []
}

// Runtime job status codes (v2_forward_runtime_job.status).
export const JOB_STATES = Object.freeze({ 0: 'pending', 1: 'running', 2: 'success', 3: 'failed' })

// UiBadge tone per job state.
export const JOB_TONES = Object.freeze({ pending: 'neutral', running: 'info', success: 'success', failed: 'danger' })
