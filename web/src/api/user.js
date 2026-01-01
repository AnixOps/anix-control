import request from '@/utils/request'

// 获取用户资料
export function getProfile() {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

// 获取用户仪表盘数据
export function getDashboard() {
  return request({
    url: '/user/dashboard',
    method: 'get'
  })
}

// 获取用户订阅详情
export function getSubscription(refresh = false) {
  return request({
    url: '/user/subscription',
    method: 'get',
    params: refresh ? { refresh: 'true' } : {}
  })
}

// 获取知识库列表
export function getKnowledgeList() {
  return request({
    url: '/user/knowledge',
    method: 'get'
  })
}

// 获取知识库文章详情
export function getKnowledgeDetail(id) {
  return request({
    url: `/user/knowledge/${id}`,
    method: 'get'
  })
}

// 获取工单列表
export function getTickets() {
  return request({
    url: '/user/ticket',
    method: 'get'
  })
}

// 提交新工单
export function createTicket(data) {
  return request({
    url: '/user/ticket',
    method: 'post',
    data
  })
}

// 获取工单详情
export function getTicketDetail(id) {
  return request({
    url: `/user/ticket/${id}`,
    method: 'get'
  })
}

// 回复工单
export function replyTicket(id, data) {
  return request({
    url: `/user/ticket/${id}/reply`,
    method: 'post',
    data
  })
}

// 关闭工单
export function closeTicket(id) {
  return request({
    url: `/user/ticket/${id}/close`,
    method: 'post'
  })
}

// 获取套餐列表
export function getPlans() {
  return request({
    url: '/user/plan',
    method: 'get'
  })
}

// 检查优惠券
export function checkCoupon(data) {
  return request({
    url: '/user/coupon/check',
    method: 'post',
    data
  })
}

// 保存订单
export function saveOrder(data) {
  return request({
    url: '/user/order/save',
    method: 'post',
    data
  })
}

// 获取订单列表
export function getOrders(params) {
  return request({
    url: '/user/order',
    method: 'get',
    params
  })
}

// 获取订单详情
export function getOrderDetail(id) {
  return request({
    url: `/user/order/${id}`,
    method: 'get'
  })
}
