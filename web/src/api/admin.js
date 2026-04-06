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
    url: '/user/reset',
    method: 'post',
    data: {
      id,
      type: 1
    }
  })
}

export function resetUserTunnelTraffic(id) {
  return request({
    url: '/user/reset',
    method: 'post',
    data: {
      id,
      type: 2
    }
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

// 获取分组下的物理节点协议列表
export function getSubscriptionProtocols(groupId) {
  return request({
    url: `/admin/subscription/groups/${groupId}/protocols`,
    method: 'get'
  })
}

// 更新分组关联的物理节点协议
export function updateGroupProtocols(groupId, protocolIds) {
  return request({
    url: `/admin/subscription/groups/${groupId}/protocols`,
    method: 'post',
    data: { protocol_ids: protocolIds }
  })
}

// 获取所有可用的物理节点协议 (Protocol Pool)
export function getAvailableProtocols() {
  return request({
    url: '/admin/subscription/protocols/available',
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

// ====== 套餐-订阅分组关联 ======

// 获取套餐关联的订阅分组
export function getPlanGroups(planId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups`,
    method: 'get'
  })
}

// 给套餐添加订阅分组
export function addGroupToPlan(planId, groupId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups`,
    method: 'post',
    data: { group_id: groupId }
  })
}

// 从套餐移除订阅分组
export function removeGroupFromPlan(planId, groupId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups/${groupId}`,
    method: 'delete'
  })
}

// ====== 工单管理 ======

// 获取工单列表
export function getTickets(params) {
  return request({
    url: '/admin/ticket',
    method: 'get',
    params
  })
}

// 回复工单
export function replyTicket(data) {
  return request({
    url: '/admin/ticket/reply',
    method: 'post',
    data
  })
}

// 关闭工单
export function closeTicket(id) {
  return request({
    url: `/admin/ticket/${id}/close`,
    method: 'post'
  })
}

// ====== 优惠券管理 ======

// 获取优惠券列表
export function getCoupons(params) {
  return request({
    url: '/admin/coupon',
    method: 'get',
    params
  })
}

// 创建优惠券
export function createCoupon(data) {
  return request({
    url: '/admin/coupon',
    method: 'post',
    data
  })
}

// 删除优惠券
export function deleteCoupon(id) {
  return request({
    url: `/admin/coupon/${id}`,
    method: 'delete'
  })
}

// ====== 知识库管理 ======

// 获取知识库文章列表
export function getKnowledgeList(params) {
  return request({
    url: '/admin/knowledge',
    method: 'get',
    params
  })
}

// 创建知识库文章
export function createKnowledge(data) {
  return request({
    url: '/admin/knowledge',
    method: 'post',
    data
  })
}

// 更新知识库文章
export function updateKnowledge(id, data) {
  return request({
    url: `/admin/knowledge/${id}`,
    method: 'put',
    data
  })
}

// 删除知识库文章
export function deleteKnowledge(id) {
  return request({
    url: `/admin/knowledge/${id}`,
    method: 'delete'
  })
}

// ====== 流量转发管理 ======

// 获取转发节点列表
export function createForward(data) {
  return request({
    url: '/forward/create',
    method: 'post',
    data
  })
}

export function getForwardList() {
  return request({
    url: '/forward/list',
    method: 'post'
  })
}

export function updateForward(data) {
  return request({
    url: '/forward/update',
    method: 'post',
    data
  })
}

export function deleteForward(id) {
  return request({
    url: '/forward/delete',
    method: 'post',
    data: { id }
  })
}

export function forceDeleteForward(id) {
  return request({
    url: '/forward/force-delete',
    method: 'post',
    data: { id }
  })
}

export function pauseForwardService(id) {
  return request({
    url: '/forward/pause',
    method: 'post',
    data: { id }
  })
}

export function resumeForwardService(id) {
  return request({
    url: '/forward/resume',
    method: 'post',
    data: { id }
  })
}

export function diagnoseForward(forwardId) {
  return request({
    url: '/forward/diagnose',
    method: 'post',
    data: { forwardId }
  })
}

export function updateForwardOrder(data) {
  return request({
    url: '/forward/update-order',
    method: 'post',
    data
  })
}

export function listForwardRuntimeJobs(params) {
  return request({
    url: '/admin/forward/runtime/jobs',
    method: 'get',
    params
  })
}

export function getForwardTunnels() {
  return request({
    url: '/tunnel/user/tunnel',
    method: 'post'
  })
}

export function assignAdminUserTunnel(data) {
  return request({
    url: '/tunnel/user/assign',
    method: 'post',
    data
  })
}

export function getAdminUserTunnelList(data) {
  return request({
    url: '/tunnel/user/list',
    method: 'post',
    data
  })
}

export function removeAdminUserTunnel(data) {
  return request({
    url: '/tunnel/user/remove',
    method: 'post',
    data
  })
}

export function updateAdminUserTunnel(data) {
  return request({
    url: '/tunnel/user/update',
    method: 'post',
    data
  })
}

export function getForwardNodes(params) {
  return request({
    url: '/admin/forward/nodes',
    method: 'get',
    params
  })
}

// 创建转发节点
export function createForwardNode(data) {
  return request({
    url: '/admin/forward/nodes',
    method: 'post',
    data
  })
}

// 获取转发节点详情
export function getForwardNode(id) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'get'
  })
}

// 更新转发节点
export function updateForwardNode(id, data) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'put',
    data
  })
}

// 删除转发节点
export function deleteForwardNode(id) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'delete'
  })
}

// 检查转发节点
export function checkForwardNode(id) {
  return request({
    url: `/admin/forward/nodes/${id}/check`,
    method: 'post'
  })
}

// 切换转发节点状态
export function toggleForwardNode(id, enabled) {
  return request({
    url: `/admin/forward/nodes/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

// 获取转发规则列表
export function getForwardRules(params) {
  return request({
    url: '/admin/forward/rules',
    method: 'get',
    params
  })
}

// 创建转发规则
export function createForwardRule(data) {
  return request({
    url: '/admin/forward/rules',
    method: 'post',
    data
  })
}

// 获取转发规则详情
export function getForwardRule(id) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'get'
  })
}

// 更新转发规则
export function updateForwardRule(id, data) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'put',
    data
  })
}

// 删除转发规则
export function deleteForwardRule(id) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'delete'
  })
}

// 切换转发规则状态
export function toggleForwardRule(id, enabled) {
  return request({
    url: `/admin/forward/rules/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

// 获取转发统计
export function getForwardStats() {
  return request({
    url: '/admin/forward/stats',
    method: 'get'
  })
}

// ====== 支付网关管理 ======

// 获取支付网关列表
export function getPaymentGateways() {
  return request({
    url: '/admin/payment/gateways',
    method: 'get'
  })
}

// 创建支付网关
export function createPaymentGateway(data) {
  return request({
    url: '/admin/payment/gateways',
    method: 'post',
    data
  })
}

// 更新支付网关
export function updatePaymentGateway(id, data) {
  return request({
    url: `/admin/payment/gateways/${id}`,
    method: 'put',
    data
  })
}

// 删除支付网关
export function deletePaymentGateway(id) {
  return request({
    url: `/admin/payment/gateways/${id}`,
    method: 'delete'
  })
}

// 切换支付网关状态
export function togglePaymentGateway(id, enabled) {
  return request({
    url: `/admin/payment/gateways/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

// 获取支付统计
export function getPaymentStats(params) {
  return request({
    url: '/admin/payment/stats',
    method: 'get',
    params
  })
}

// 获取支付记录
export function getPaymentRecords(params) {
  return request({
    url: '/admin/payment/records',
    method: 'get',
    params
  })
}

// ====== Telegram Bot管理 ======

// 获取Bot配置
export function getTelegramBot() {
  return request({
    url: '/admin/telegram/bot',
    method: 'get'
  })
}

// 更新Bot配置
export function updateTelegramBot(data) {
  return request({
    url: '/admin/telegram/bot',
    method: 'put',
    data
  })
}

// 设置Webhook
export function setTelegramWebhook(url) {
  return request({
    url: '/admin/telegram/webhook',
    method: 'post',
    data: { url }
  })
}

// 删除Webhook
export function deleteTelegramWebhook() {
  return request({
    url: '/admin/telegram/webhook',
    method: 'delete'
  })
}

// 发送通知
export function sendTelegramNotification(data) {
  return request({
    url: '/admin/telegram/notify',
    method: 'post',
    data
  })
}

// 广播消息
export function broadcastTelegram(message) {
  return request({
    url: '/admin/telegram/broadcast',
    method: 'post',
    data: { message }
  })
}

// 获取用户绑定列表
export function getTelegramUsers(params) {
  return request({
    url: '/admin/telegram/users',
    method: 'get',
    params
  })
}

// 更新用户通知设置
export function updateTelegramUserNotify(id, data) {
  return request({
    url: `/admin/telegram/users/${id}/notify`,
    method: 'put',
    data
  })
}

// ====== MFA管理 ======

// 获取MFA配置
export function getMFAConfig() {
  return request({
    url: '/admin/mfa/config',
    method: 'get'
  })
}

// 更新MFA配置
export function updateMFAConfig(data) {
  return request({
    url: '/admin/mfa/config',
    method: 'put',
    data
  })
}

// ====== 通知管理 ======

// 获取通知模板列表
export function getNotificationTemplates(params) {
  return request({
    url: '/admin/notification/templates',
    method: 'get',
    params
  })
}

// 创建通知模板
export function createNotificationTemplate(data) {
  return request({
    url: '/admin/notification/templates',
    method: 'post',
    data
  })
}

// 更新通知模板
export function updateNotificationTemplate(id, data) {
  return request({
    url: `/admin/notification/templates/${id}`,
    method: 'put',
    data
  })
}

// 删除通知模板
export function deleteNotificationTemplate(id) {
  return request({
    url: `/admin/notification/templates/${id}`,
    method: 'delete'
  })
}

// 获取通知日志
export function getNotificationLogs(params) {
  return request({
    url: '/admin/notification/logs',
    method: 'get',
    params
  })
}

// 发送测试通知
export function sendTestNotification(data) {
  return request({
    url: '/admin/notification/test',
    method: 'post',
    data
  })
}

// 获取邮件配置
export function getEmailConfig() {
  return request({
    url: '/admin/notification/email/config',
    method: 'get'
  })
}

// 更新邮件配置
export function updateEmailConfig(data) {
  return request({
    url: '/admin/notification/email/config',
    method: 'put',
    data
  })
}

// ====== 邀请返利管理 ======

// 获取邀请配置
export function getInviteConfig() {
  return request({
    url: '/admin/invite/config',
    method: 'get'
  })
}

// 更新邀请配置
export function updateInviteConfig(data) {
  return request({
    url: '/admin/invite/config',
    method: 'put',
    data
  })
}

// 获取邀请统计
export function getInviteStats() {
  return request({
    url: '/admin/invite/stats',
    method: 'get'
  })
}

// 获取提现申请列表
export function getWithdrawals(params) {
  return request({
    url: '/admin/invite/withdrawals',
    method: 'get',
    params
  })
}

// 处理提现申请
export function processWithdrawal(id, data) {
  return request({
    url: `/admin/invite/withdrawals/${id}/process`,
    method: 'post',
    data
  })
}

// ====== 系统配置管理 ======

// 获取系统配置
export function getSystemConfigs(params) {
  return request({
    url: '/admin/system/configs',
    method: 'get',
    params
  })
}

// 获取单个配置
export function getSystemConfig(key) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'get'
  })
}

// 设置系统配置
export function setSystemConfig(key, data) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'put',
    data
  })
}

// 删除系统配置
export function deleteSystemConfig(key) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'delete'
  })
}

// ====== 备份管理 ======

// 获取备份配置
export function getBackupConfig() {
  return request({
    url: '/admin/system/backup/config',
    method: 'get'
  })
}

// 更新备份配置
export function updateBackupConfig(data) {
  return request({
    url: '/admin/system/backup/config',
    method: 'put',
    data
  })
}

// 创建备份
export function createBackup(type = 'database') {
  return request({
    url: '/admin/system/backup',
    method: 'post',
    params: { type }
  })
}

// 获取备份列表
export function getBackups(params) {
  return request({
    url: '/admin/system/backups',
    method: 'get',
    params
  })
}

// 获取备份统计
export function getBackupStats() {
  return request({
    url: '/admin/system/backup/stats',
    method: 'get'
  })
}

// 删除备份
export function deleteBackup(id) {
  return request({
    url: `/admin/system/backups/${id}`,
    method: 'delete'
  })
}

// 恢复备份
export function restoreBackup(id) {
  return request({
    url: `/admin/system/backups/${id}/restore`,
    method: 'post'
  })
}

// ====== Agent 管理 ======

// 获取在线 Agent 列表
export function getAgents() {
  return request({
    url: '/admin/agent/list',
    method: 'get'
  })
}

// 创建 Agent 任务
export function createAgentTask(data) {
  return request({
    url: '/admin/agent/tasks',
    method: 'post',
    data
  })
}

// 执行命令
export function executeAgentCommand(data) {
  return request({
    url: '/admin/agent/execute',
    method: 'post',
    data
  })
}

// 获取 Agent 任务结果
export function getAgentTaskResult(taskId) {
  return request({
    url: `/admin/agent/tasks/${taskId}`,
    method: 'get'
  })
}

// 获取 Agent 监控数据
export function getAgentMonitor(nodeId) {
  return request({
    url: '/admin/agent/monitor',
    method: 'get',
    params: { node_id: nodeId }
  })
}

// ====== 负载均衡管理 ======

// 获取负载均衡器列表
export function getLoadBalancers(params) {
  return request({
    url: '/admin/loadbalancers',
    method: 'get',
    params
  })
}

// 创建负载均衡器
export function createLoadBalancer(data) {
  return request({
    url: '/admin/loadbalancers',
    method: 'post',
    data
  })
}

// 获取负载均衡器详情
export function getLoadBalancer(id) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'get'
  })
}

// 更新负载均衡器
export function updateLoadBalancer(id, data) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'put',
    data
  })
}

// 删除负载均衡器
export function deleteLoadBalancer(id) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'delete'
  })
}

// 获取负载均衡器统计
export function getLoadBalancerStats(id) {
  return request({
    url: `/admin/loadbalancers/${id}/stats`,
    method: 'get'
  })
}

// 执行健康检查
export function runHealthCheck(id) {
  return request({
    url: `/admin/loadbalancers/${id}/check`,
    method: 'post'
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
  resetUserTunnelTraffic,
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
  getSubscriptionProtocols,
  updateGroupProtocols,
  getAvailableProtocols,
  createSubscriptionTemplate,
  getSubscriptionTemplate,
  updateSubscriptionTemplate,
  deleteSubscriptionTemplate,
  previewSubscription,
  getPlans,
  createPlan,
  getPlan,
  updatePlan,
  deletePlan,
  assignPlanToUser,
  getPlanGroups,
  addGroupToPlan,
  removeGroupFromPlan,
  getTickets,
  replyTicket,
  closeTicket,
  getCoupons,
  createCoupon,
  deleteCoupon,
  getKnowledgeList,
  createKnowledge,
  updateKnowledge,
  deleteKnowledge,
  // 流量转发
  createForward,
  getForwardList,
  updateForward,
  deleteForward,
  forceDeleteForward,
  pauseForwardService,
  resumeForwardService,
  diagnoseForward,
  updateForwardOrder,
  listForwardRuntimeJobs,
  getForwardTunnels,
  assignAdminUserTunnel,
  getAdminUserTunnelList,
  removeAdminUserTunnel,
  updateAdminUserTunnel,
  getForwardNodes,
  createForwardNode,
  getForwardNode,
  updateForwardNode,
  deleteForwardNode,
  checkForwardNode,
  toggleForwardNode,
  getForwardRules,
  createForwardRule,
  getForwardRule,
  updateForwardRule,
  deleteForwardRule,
  toggleForwardRule,
  getForwardStats,
  // 支付网关
  getPaymentGateways,
  createPaymentGateway,
  updatePaymentGateway,
  deletePaymentGateway,
  togglePaymentGateway,
  getPaymentStats,
  getPaymentRecords,
  // Telegram
  getTelegramBot,
  updateTelegramBot,
  setTelegramWebhook,
  deleteTelegramWebhook,
  sendTelegramNotification,
  broadcastTelegram,
  getTelegramUsers,
  updateTelegramUserNotify,
  // MFA
  getMFAConfig,
  updateMFAConfig,
  // 通知
  getNotificationTemplates,
  createNotificationTemplate,
  updateNotificationTemplate,
  deleteNotificationTemplate,
  getNotificationLogs,
  sendTestNotification,
  getEmailConfig,
  updateEmailConfig,
  // 邀请返利
  getInviteConfig,
  updateInviteConfig,
  getInviteStats,
  getWithdrawals,
  processWithdrawal,
  // 系统配置
  getSystemConfigs,
  getSystemConfig,
  setSystemConfig,
  deleteSystemConfig,
  // 备份
  getBackupConfig,
  updateBackupConfig,
  createBackup,
  getBackups,
  getBackupStats,
  deleteBackup,
  restoreBackup,
  // Agent
  getAgents,
  createAgentTask,
  executeAgentCommand,
  getAgentTaskResult,
  getAgentMonitor,
  // 负载均衡
  getLoadBalancers,
  createLoadBalancer,
  getLoadBalancer,
  updateLoadBalancer,
  deleteLoadBalancer,
  getLoadBalancerStats,
  runHealthCheck
}

