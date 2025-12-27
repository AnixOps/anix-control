import request from '@/utils/request'

// ====== 管理员接口 ======

// 获取仪表盘数据
export function getDashboard(refresh = false) {
  return request({
    url: '/admin/dashboard',
    method: 'get',
    params: refresh ? { refresh: 'true' } : {}
  })
}

// ====== 用户管理 ======

// 创建用户
export function createUser(data) {
  return request({
    url: '/admin/users',
    method: 'post',
    data
  })
}

// 获取用户列表
export function getUserList(params) {
  return request({
    url: '/admin/users',
    method: 'get',
    params
  })
}

// 获取用户统计
export function getUserStats() {
  return request({
    url: '/admin/users/stats',
    method: 'get'
  })
}

// 获取用户详情
export function getUser(id) {
  return request({
    url: `/admin/users/${id}`,
    method: 'get'
  })
}

// 更新用户
export function updateUser(id, data) {
  return request({
    url: `/admin/users/${id}`,
    method: 'put',
    data
  })
}

// 删除用户
export function deleteUser(id) {
  return request({
    url: `/admin/users/${id}`,
    method: 'delete'
  })
}

// 封禁用户
export function banUser(id) {
  return request({
    url: `/admin/users/${id}/ban`,
    method: 'post'
  })
}

// 解封用户
export function unbanUser(id) {
  return request({
    url: `/admin/users/${id}/unban`,
    method: 'post'
  })
}

// 重置用户流量
export function resetUserTraffic(id) {
  return request({
    url: `/admin/users/${id}/reset-traffic`,
    method: 'post'
  })
}

// ====== 订单管理 ======

// 获取订单列表
export function getOrderList(params) {
  return request({
    url: '/admin/orders',
    method: 'get',
    params
  })
}

// 获取订单统计
export function getOrderStats() {
  return request({
    url: '/admin/orders/stats',
    method: 'get'
  })
}

// 获取订单详情
export function getOrder(id) {
  return request({
    url: `/admin/orders/${id}`,
    method: 'get'
  })
}

// 更新订单状态
export function updateOrderStatus(id, status) {
  return request({
    url: `/admin/orders/${id}/status`,
    method: 'put',
    data: { status }
  })
}

// 标记订单已支付
export function markOrderPaid(id) {
  return request({
    url: `/admin/orders/${id}/paid`,
    method: 'post'
  })
}

// 取消订单
export function cancelOrder(id) {
  return request({
    url: `/admin/orders/${id}/cancel`,
    method: 'post'
  })
}

// ====== 节点管理 ======

// 获取节点列表
export function getNodes(params) {
  return request({
    url: '/admin/nodes',
    method: 'get',
    params
  })
}

// 获取节点统计
export function getNodeStats() {
  return request({
    url: '/admin/nodes/stats',
    method: 'get'
  })
}

// 获取节点详情
export function getNode(id) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'get'
  })
}

// 创建节点
export function createNode(data) {
  return request({
    url: '/admin/nodes',
    method: 'post',
    data
  })
}

// 更新节点
export function updateNode(id, data) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'put',
    data
  })
}

// 删除节点
export function deleteNode(id) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'delete'
  })
}

// 同步节点协议
export function syncNodeProtocol(id) {
  return request({
    url: `/admin/nodes/${id}/sync`,
    method: 'post'
  })
}

// 获取节点原始配置
export function getNodeRawConfig(id) {
  return request({
    url: `/admin/nodes/${id}/raw-config`,
    method: 'get'
  })
}

// 更新节点原始配置
export function updateNodeRawConfig(id, data) {
  return request({
    url: `/admin/nodes/${id}/raw-config`,
    method: 'put',
    data
  })
}

// 验证节点配置
export function validateNodeConfig(data) {
  return request({
    url: '/admin/nodes/validate-config',
    method: 'post',
    data
  })
}

// ====== 节点协议管理 ======

// 获取节点协议列表
export function getNodeProtocols(nodeId) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols`,
    method: 'get'
  })
}

// 创建节点协议
export function createNodeProtocol(nodeId, data) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols`,
    method: 'post',
    data
  })
}

// 更新节点协议
export function updateNodeProtocol(nodeId, protocolId, data) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols/${protocolId}`,
    method: 'put',
    data
  })
}

// 删除节点协议
export function deleteNodeProtocol(nodeId, protocolId) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols/${protocolId}`,
    method: 'delete'
  })
}

// 获取协议模板
export function getProtocolTemplates() {
  return request({
    url: '/admin/protocol-templates',
    method: 'get'
  })
}

// ====== 授权密钥管理 ======

// 获取授权密钥列表
export function getAuthKeys() {
  return request({
    url: '/admin/auth-keys',
    method: 'get'
  })
}

// 生成授权密钥
export function generateAuthKey(data) {
  return request({
    url: '/admin/auth-keys',
    method: 'post',
    data
  })
}

// 删除授权密钥
export function deleteAuthKey(id) {
  return request({
    url: `/admin/auth-keys/${id}`,
    method: 'delete'
  })
}

// ====== 订阅管理 ======

// 获取订阅分组列表
export function getSubscriptionGroups() {
  return request({
    url: '/admin/subscription/groups',
    method: 'get'
  })
}

// 创建订阅分组
export function createSubscriptionGroup(data) {
  return request({
    url: '/admin/subscription/groups',
    method: 'post',
    data
  })
}

// 获取订阅分组详情
export function getSubscriptionGroup(id) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'get'
  })
}

// 更新订阅分组
export function updateSubscriptionGroup(id, data) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'put',
    data
  })
}

// 删除订阅分组
export function deleteSubscriptionGroup(id) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'delete'
  })
}

// 获取分组下的模板列表
export function getSubscriptionTemplates(groupId) {
  return request({
    url: `/admin/subscription/groups/${groupId}/templates`,
    method: 'get'
  })
}

// 创建订阅模板
export function createSubscriptionTemplate(groupId, data) {
  return request({
    url: `/admin/subscription/groups/${groupId}/templates`,
    method: 'post',
    data
  })
}

// 获取订阅模板详情
export function getSubscriptionTemplate(id) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'get'
  })
}

// 更新订阅模板
export function updateSubscriptionTemplate(id, data) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'put',
    data
  })
}

// 删除订阅模板
export function deleteSubscriptionTemplate(id) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'delete'
  })
}

// 预览订阅内容
export function previewSubscription(data) {
  return request({
    url: '/admin/subscription/preview',
    method: 'post',
    data
  })
}

// ====== 套餐管理 ======

// 获取套餐列表
export function getPlans() {
  return request({
    url: '/admin/plans',
    method: 'get'
  })
}

// 创建套餐
export function createPlan(data) {
  return request({
    url: '/admin/plans',
    method: 'post',
    data
  })
}

// 获取套餐
export function getPlan(id) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'get'
  })
}

// 更新套餐
export function updatePlan(id, data) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'put',
    data
  })
}

// 删除套餐
export function deletePlan(id) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'delete'
  })
}

// 分配套餐给用户
export function assignPlanToUser(id, data) {
  return request({
    url: `/admin/plans/${id}/assign`,
    method: 'post',
    data
  })
}

// 默认导出
export default {
  getDashboard,
  createUser,
  getUserList,
  getUserStats,
  getUser,
  updateUser,
  deleteUser,
  banUser,
  unbanUser,
  resetUserTraffic,
  getOrderList,
  getOrderStats,
  getOrder,
  updateOrderStatus,
  markOrderPaid,
  cancelOrder,
  getNodes,
  getNodeStats,
  createNode,
  getNode,
  updateNode,
  deleteNode,
  syncNodeProtocol,
  getNodeRawConfig,
  updateNodeRawConfig,
  validateNodeConfig,
  getNodeProtocols,
  createNodeProtocol,
  updateNodeProtocol,
  deleteNodeProtocol,
  getProtocolTemplates,
  getAuthKeys,
  generateAuthKey,
  deleteAuthKey,
  getSubscriptionGroups,
  createSubscriptionGroup,
  getSubscriptionGroup,
  updateSubscriptionGroup,
  deleteSubscriptionGroup,
  getSubscriptionTemplates,
  createSubscriptionTemplate,
  getSubscriptionTemplate,
  updateSubscriptionTemplate,
  deleteSubscriptionTemplate,
  previewSubscription
  ,getPlans
  ,createPlan
  ,getPlan
  ,updatePlan
  ,deletePlan
  ,assignPlanToUser
}
