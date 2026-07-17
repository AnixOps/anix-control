import request from '@/utils/request'

export function getProfile() {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

export function getDashboard() {
  return request({
    url: '/user/dashboard',
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

export function getKnowledgeDetail(id) {
  return request({
    url: `/user/knowledge/${id}`,
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
