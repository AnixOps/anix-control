// Response helpers of the 系统设置 sections. The system endpoints answer
// either the legacy body ({ data }, { data: { data } }) or the panel
// envelope ({ code, msg, data }); a code other than 0 is an error even with
// HTTP 200. Moved unchanged from the single System.vue (UI U7).

export function readPanelEnvelopeError(res) {
  const candidates = [res, res?.data]
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
    if (Number(candidate.code) === 0) return null
    return candidate.msg || candidate.error || ''
  }
  return null
}

// ensureSystemSuccess throws the server's message (or the fallback) when the
// answer is an error envelope.
export function ensureSystemSuccess(res, fallbackMessage) {
  const message = readPanelEnvelopeError(res)
  if (message !== null) {
    throw new Error(message || fallbackMessage)
  }
  return res
}

export async function ensureSystemMutation(promise, fallbackMessage) {
  return ensureSystemSuccess(await promise, fallbackMessage)
}

export function readSystemPayload(res, fallbackMessage) {
  const payload = ensureSystemSuccess(res, fallbackMessage)
  if (!payload || typeof payload !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(payload, 'code')) {
    return payload.data && typeof payload.data === 'object' ? payload.data : {}
  }
  if (payload.data && typeof payload.data === 'object') {
    if (Object.prototype.hasOwnProperty.call(payload.data, 'code')) {
      return payload.data.data && typeof payload.data.data === 'object' ? payload.data.data : {}
    }
    return payload.data.data && typeof payload.data.data === 'object' ? payload.data.data : payload.data
  }
  return payload
}

export function readBackupPayload(res, fallbackMessage) {
  const payload = ensureSystemSuccess(res, fallbackMessage)
  if (!payload || typeof payload !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(payload, 'code')) {
    return payload.data && typeof payload.data === 'object' ? payload.data : {}
  }
  if (payload.data && typeof payload.data === 'object' && Object.prototype.hasOwnProperty.call(payload.data, 'data')) {
    return payload.data.data && typeof payload.data.data === 'object' ? payload.data.data : {}
  }
  return payload.data && typeof payload.data === 'object' ? payload.data : payload
}

// systemErrorText: the most useful message of an axios error, an Error, or
// a panel envelope error.
export function systemErrorText(error) {
  return error?.response?.data?.msg || error?.response?.data?.error || error?.msg || error?.message || ''
}
