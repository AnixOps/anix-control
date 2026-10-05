// Helpers for the kernel-owned admin routes under /api/v4/admin
// (src/api/admin.js): answers are {data: ...}, refusals {error: {code,
// message}}. Plain functions, so a page that mocks the API module keeps them.

// The message of a refused request: the route's {error: {message}} (or a bare
// string), a {message}, the transport's own message, else `fallback`.
export function adminV4ErrorMessage(error, fallback = '') {
  const failure = error?.response?.data?.error
  return (typeof failure === 'string' ? failure : failure?.message) || error?.response?.data?.message || error?.message || fallback
}

// One key per click of a bulk action: the server derives "<key>:<id>" for
// each item, so asking again (the failed ids of that click) changes nothing
// twice. The server keeps the whole id (key, user, action) under 128 bytes.
export function newBulkIdempotencyKey() {
  const cryptoObject = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined
  if (cryptoObject?.randomUUID) return `ub-${cryptoObject.randomUUID()}`
  const random = Array.from({ length: 4 }, () => Math.floor(Math.random() * 0xffffffff).toString(16).padStart(8, '0')).join('')
  return `ub-${Date.now().toString(16)}-${random}`
}
