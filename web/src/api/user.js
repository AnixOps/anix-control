import request from '@/utils/request'

export function getProfile() {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

export function getSubscription(refresh = false) {
  return request({
    url: '/user/subscription',
    method: 'get',
    params: refresh ? { refresh: 'true' } : {}
  })
}

export function getKnowledgeList() {
  return request({
    url: '/user/knowledge',
    method: 'get'
  })
}

export function getTickets() {
  return request({
    url: '/user/ticket',
    method: 'get'
  })
}

export function createTicket(data) {
  return request({
    url: '/user/ticket',
    method: 'post',
    data
  })
}

export function getTicketDetail(id) {
  return request({
    url: `/user/ticket/${id}`,
    method: 'get'
  })
}

export function replyTicket(id, data) {
  return request({
    url: `/user/ticket/${id}/reply`,
    method: 'post',
    data
  })
}

export function closeTicket(id) {
  return request({
    url: `/user/ticket/${id}/close`,
    method: 'post'
  })
}

export function getPlans() {
  return request({
    url: '/user/plan',
    method: 'get'
  })
}

export function checkCoupon(data) {
  return request({
    url: '/user/coupon/check',
    method: 'post',
    data
  })
}

export function saveOrder(data) {
  return request({
    url: '/user/order/save',
    method: 'post',
    data
  })
}

export function getOrders(params) {
  return request({
    url: '/user/order',
    method: 'get',
    params
  })
}

export function getOrderDetail(id) {
  return request({
    url: `/user/order/${id}`,
    method: 'get'
  })
}

// Two-factor authentication of the signed-in account (identity package,
// /api/v2/user/mfa/*). The account page (views/Account.vue) uses them.
export function getMfaStatus() {
  return request({
    url: '/user/mfa/status',
    method: 'get'
  })
}

export function setupTotp() {
  return request({
    url: '/user/mfa/totp/setup',
    method: 'post'
  })
}

export function enableTotp(code) {
  return request({
    url: '/user/mfa/totp/enable',
    method: 'post',
    data: { code }
  })
}

export function disableMfa(password) {
  return request({
    url: '/user/mfa/disable',
    method: 'post',
    data: { password }
  })
}

export function regenerateBackupCodes() {
  return request({
    url: '/user/mfa/backup-codes/regenerate',
    method: 'post'
  })
}

// The signed-in user resets their own subscription link (identity package,
// POST /api/v2/user/subscription/reset): `{ password }`, or with two-step
// verification on, `{ code, method }` (method 'totp' or 'backup'). The
// answer is the new subscription token.
export function resetSubscription(credentials) {
  return request({
    url: '/user/subscription/reset',
    method: 'post',
    data: credentials
  })
}
