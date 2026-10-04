// Words for the API's stable codes (F5a refusals, sdk/forward/validate
// violation codes). The API's English message is the fallback and the
// detail.

const ERROR_CODES = new Set([
  'super_admin_required', 'revision_conflict', 'idempotency_conflict', 'refused', 'invalid_route', 'invalid_request',
  'not_found', 'forward_unavailable', 'plugin_route_not_found', 'rate_limited', 'timeout', 'network', 'not_implemented'
])

// forwardErrorMessage is one sentence for a failed call.
export function forwardErrorMessage(t, error) {
  if (!error) return ''
  const code = error.code || ''
  if (ERROR_CODES.has(code)) return t(`forwardV4.errors.${code}`)
  if (error.status === 403) return t('forwardV4.errors.super_admin_required')
  return error.message || t('forwardV4.errors.generic')
}

// violationText is the field error for a violation: the UI's words for its
// code, or the API's message.
export function violationText(t, te, violation) {
  if (!violation) return ''
  const key = `forwardV4.codes.${violation.code}`
  return te(key) ? t(key) : (violation.message || violation.code || '')
}
