// Response helpers of the 通知 page (UI U7): the notification and Telegram
// endpoints answer the legacy body ({ data }, { data: { data } }) or the
// panel envelope; a code other than 0 is an error. Moved unchanged from
// Notifications.vue and Telegram.vue, which read them the same way.

export function readNotifyEnvelopeError(res) {
  const candidates = [res, res?.data]
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
    if (Number(candidate.code) === 0) return null
    return candidate.msg || candidate.message || candidate.error || ''
  }
  return null
}

export function ensureNotifySuccess(res, fallbackMessage) {
  const message = readNotifyEnvelopeError(res)
  if (message !== null) {
    throw new Error(message || fallbackMessage)
  }
  return res
}

export function readNotifyPayload(res, fallbackMessage) {
  ensureNotifySuccess(res, fallbackMessage)
  if (!res || typeof res !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data && typeof res.data === 'object' ? res.data : {}
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data && typeof res.data.data === 'object' ? res.data.data : {}
  }
  return res.data && typeof res.data === 'object' ? res.data : res
}

export function notifyErrorText(error) {
  return error?.msg || error?.response?.data?.error || error?.response?.data?.msg || error?.message || ''
}

export const NOTIFICATION_TYPES = Object.freeze(['email', 'telegram', 'webhook'])
export const NOTIFICATION_EVENTS = Object.freeze([
  'user.register',
  'user.login',
  'user.expire',
  'user.traffic_low',
  'order.paid',
  'ticket.reply',
  'node.offline',
  'node.online'
])
export const LOG_STATUSES = Object.freeze(['pending', 'success', 'failed'])

// i18n keys of the event names (dots are key separators).
export const EVENT_KEYS = Object.freeze({
  'user.register': 'userRegister',
  'user.login': 'userLogin',
  'user.expire': 'userExpire',
  'user.traffic_low': 'userTrafficLow',
  'order.paid': 'orderPaid',
  'ticket.reply': 'ticketReply',
  'node.offline': 'nodeOffline',
  'node.online': 'nodeOnline'
})
