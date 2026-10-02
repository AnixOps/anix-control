// The v2 panel envelope: { code, msg, data }. A code other than 0 is a
// failure the server reports with HTTP 200, so callers that only read
// `res.data` would show an empty page instead of the error. User pages read
// responses through these helpers.

// unwrapPanel returns the data of a successful envelope and throws an Error
// with the server's message otherwise. A body without `code` is returned as
// the data itself (`{ data }` bodies keep working).
export function unwrapPanel(body, fallback = '') {
  if (body && typeof body === 'object' && Object.prototype.hasOwnProperty.call(body, 'code')) {
    if (Number(body.code) !== 0) {
      throw new Error(body.msg || body.message || fallback || 'request failed')
    }
    return body.data
  }
  if (body && typeof body === 'object' && Object.prototype.hasOwnProperty.call(body, 'data')) {
    return body.data
  }
  return body
}

// panelErrorMessage reads the most useful message from an axios error, an
// Error thrown by unwrapPanel, or anything else.
export function panelErrorMessage(error, fallback = '') {
  const data = error?.response?.data
  return data?.msg || data?.message || data?.error || error?.message || fallback || String(error || '')
}

// listOf turns the list-ish answers of v2 (null, [], { list }, { data }) into
// an array.
export function listOf(value) {
  if (Array.isArray(value)) return value
  if (Array.isArray(value?.list)) return value.list
  if (Array.isArray(value?.data)) return value.data
  return []
}
