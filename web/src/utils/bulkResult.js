// The answer of POST /api/v4/admin/users/bulk and /invite-codes/bulk:
//
//   { action, requested, succeeded, failed,
//     results: [{ id, ok: true } | { id, ok: false, error: { code, message } }] }
//
// A bulk request is per item, never all or nothing, so the console reads
// every outcome: which ids are done, which failed and why, and which of the
// failed ones are worth another try.

// Refusals that a second attempt cannot change: the user or code is gone, an
// invite code was used, the administrator targeted their own account.
const FINAL_CODES = new Set(['not_found', 'conflict', 'forbidden_self'])
// Codes the console words itself; any other code (a package gateway code such
// as package_route_frozen) is shown as a plain failure with the server text.
export const BULK_ERROR_CODES = Object.freeze(['not_found', 'conflict', 'forbidden_self', 'not_attempted', 'failed'])

export function isRetryableBulkCode(code) {
  return !FINAL_CODES.has(code)
}

// readBulkResult sorts `requestedIds` by outcome. An id the answer does not
// mention is `not_attempted` rather than silently done.
export function readBulkResult(data, requestedIds = []) {
  const results = Array.isArray(data?.results) ? data.results : []
  const byId = new Map(results.map(item => [Number(item?.id), item]))
  const ids = requestedIds.length ? requestedIds.map(Number) : results.map(item => Number(item?.id))
  const done = []
  const failed = []
  for (const id of ids) {
    const item = byId.get(id)
    if (item?.ok === true) {
      done.push(id)
      continue
    }
    const error = item?.error || {}
    failed.push({
      id,
      code: String(error.code || (item ? 'failed' : 'not_attempted')),
      message: String(error.message || '')
    })
  }
  return {
    done,
    failed,
    retryable: failed.filter(entry => isRetryableBulkCode(entry.code)).map(entry => entry.id),
    failedIds: failed.map(entry => entry.id)
  }
}

// How many failed items there are per wording code, in the order of
// BULK_ERROR_CODES: [{ code, count, message }] (message: the first server
// message of that code, which the console shows for unnamed codes).
export function countBulkFailures(failed) {
  const counts = new Map()
  for (const entry of failed) {
    const code = BULK_ERROR_CODES.includes(entry.code) ? entry.code : 'failed'
    const row = counts.get(code) || { code, count: 0, message: '' }
    row.count += 1
    if (!row.message && entry.message) row.message = entry.message
    counts.set(code, row)
  }
  return BULK_ERROR_CODES.filter(code => counts.has(code)).map(code => counts.get(code))
}
