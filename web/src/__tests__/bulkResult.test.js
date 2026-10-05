import { describe, expect, it } from 'vitest'
import { BULK_ERROR_CODES, countBulkFailures, isRetryableBulkCode, readBulkResult } from '@/utils/bulkResult'
import { adminV4ErrorMessage, newBulkIdempotencyKey } from '@/utils/adminV4'

// POST /api/v4/admin/users/bulk and /invite-codes/bulk answer per item.
describe('readBulkResult', () => {
  const answer = {
    action: 'ban',
    requested: 5,
    succeeded: 2,
    failed: 3,
    results: [
      { id: 3, ok: true },
      { id: 5, ok: false, error: { code: 'forbidden_self', message: 'cannot ban yourself' } },
      { id: 8, ok: false, error: { code: 'not_found', message: 'user not found' } },
      { id: 9, ok: true },
      { id: 11, ok: false, error: { code: 'package_route_frozen', message: 'the route is frozen' } }
    ]
  }

  it('splits the requested ids into done and failed, in the order asked', () => {
    const outcome = readBulkResult(answer, [3, 5, 8, 9, 11])
    expect(outcome.done).toEqual([3, 9])
    expect(outcome.failed.map(entry => [entry.id, entry.code])).toEqual([[5, 'forbidden_self'], [8, 'not_found'], [11, 'package_route_frozen']])
    expect(outcome.failedIds).toEqual([5, 8, 11])
  })

  it('retries only what a second attempt can change', () => {
    // not_found, conflict and forbidden_self are final; a gateway code, failed and not_attempted are not.
    expect(readBulkResult(answer, [3, 5, 8, 9, 11]).retryable).toEqual([11])
    expect(['not_found', 'conflict', 'forbidden_self'].some(isRetryableBulkCode)).toBe(false)
    expect(['failed', 'not_attempted', 'plugin_host_unavailable'].every(isRetryableBulkCode)).toBe(true)
  })

  it('treats an id the answer leaves out as not attempted, never as done', () => {
    const outcome = readBulkResult({ results: [{ id: 1, ok: true }] }, [1, 2])
    expect(outcome.done).toEqual([1])
    expect(outcome.failed).toEqual([{ id: 2, code: 'not_attempted', message: '' }])
    expect(outcome.retryable).toEqual([2])
  })

  it('reads a failed item with no error object as a plain failure', () => {
    expect(readBulkResult({ results: [{ id: 4, ok: false }] }, [4]).failed).toEqual([{ id: 4, code: 'failed', message: '' }])
  })

  it('tolerates an empty or malformed answer', () => {
    expect(readBulkResult(null, [1]).failed).toEqual([{ id: 1, code: 'not_attempted', message: '' }])
    expect(readBulkResult({}, []).done).toEqual([])
    expect(readBulkResult({ results: [{ id: 6, ok: true }] }).done).toEqual([6])
  })
})

describe('countBulkFailures', () => {
  it('counts the failures per wording code in a fixed order and folds unknown codes into failed', () => {
    const rows = countBulkFailures([
      { id: 1, code: 'failed', message: 'first' },
      { id: 2, code: 'not_found', message: '' },
      { id: 3, code: 'package_route_frozen', message: 'frozen' },
      { id: 4, code: 'not_found', message: '' }
    ])
    expect(rows).toEqual([
      { code: 'not_found', count: 2, message: '' },
      { code: 'failed', count: 2, message: 'first' }
    ])
    expect(BULK_ERROR_CODES).toEqual(['not_found', 'conflict', 'forbidden_self', 'not_attempted', 'failed'])
    expect(countBulkFailures([])).toEqual([])
  })
})

describe('adminV4 helpers', () => {
  it('reads the message of a refused /api/v4/admin request', () => {
    expect(adminV4ErrorMessage({ response: { data: { error: { code: 'invalid_request', message: 'ids must be positive' } } } })).toBe('ids must be positive')
    expect(adminV4ErrorMessage({ response: { data: { error: 'plain' } } })).toBe('plain')
    expect(adminV4ErrorMessage({ response: { data: { message: 'legacy' } } })).toBe('legacy')
    expect(adminV4ErrorMessage(new Error('Network Error'))).toBe('Network Error')
    expect(adminV4ErrorMessage({}, 'fallback')).toBe('fallback')
  })

  it('makes a fresh idempotency key that fits the server limit with the user id appended', () => {
    const first = newBulkIdempotencyKey()
    expect(first).toMatch(/^ub-[0-9a-f-]+$/)
    expect(newBulkIdempotencyKey()).not.toBe(first)
    // The server builds "<key>:<id>" and keeps it under 128 bytes with the action.
    expect(first.length + 1 + String(Number.MAX_SAFE_INTEGER).length).toBeLessThan(64)
  })
})
