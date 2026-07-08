import request from '@/utils/request'

export function getDashboard(refresh = false) {
  return request({
    url: '/admin/dashboard',
    method: 'get',
    params: refresh ? { refresh: 'true' } : {}
  })
}

export function getTrafficHourly(hours = 24, userId = 0) {
  const params = { hours }
  if (userId) params.user_id = userId
  return request({
    url: '/admin/traffic/hourly',
    method: 'get',
    params
  })
}

export function getUserTrafficRanking(hours = 24, limit = 20, includeZeroUsers = false) {
  return request({
    url: '/admin/traffic/user-ranking',
    method: 'get',
    params: { hours, limit, include_zero_users: includeZeroUsers ? 'true' : undefined }
  })
}

export function getSystemInfo() {
  return request({
    url: '/admin/system/info',
    method: 'get'
  })
}

export function createUser(data) {
  return request({
    url: '/admin/users',
    method: 'post',
    data
  })
}

export function getUserList(params) {
  return request({
    url: '/admin/users',
    method: 'get',
    params
  })
}

export function getUserStats() {
  return request({
    url: '/admin/users/stats',
    method: 'get'
  })
}

export function getUser(id) {
  return request({
    url: `/admin/users/${id}`,
    method: 'get'
  })
}

export function updateUser(id, data) {
  return request({
    url: `/admin/users/${id}`,
    method: 'put',
    data
  })
}

export function deleteUser(id) {
  return request({
    url: `/admin/users/${id}`,
    method: 'delete'
  })
}

export function banUser(id) {
  return request({
    url: `/admin/users/${id}/ban`,
    method: 'post'
  })
}

export function unbanUser(id) {
  return request({
    url: `/admin/users/${id}/unban`,
    method: 'post'
  })
}

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

export function resetUserSubscribe(id) {
  return request({
    url: `/admin/users/${id}/reset-subscribe`,
    method: 'post'
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

export function getOrderList(params) {
  return request({
    url: '/admin/orders',
    method: 'get',
    params
  })
}

export function getOrderStats() {
  return request({
    url: '/admin/orders/stats',
    method: 'get'
  })
}

export function getOrder(id) {
  return request({
    url: `/admin/orders/${id}`,
    method: 'get'
  })
}

export function updateOrderStatus(id, status) {
  return request({
    url: `/admin/orders/${id}/status`,
    method: 'put',
    data: { status }
  })
}

export function markOrderPaid(id) {
  return request({
    url: `/admin/orders/${id}/paid`,
    method: 'post'
  })
}

export function cancelOrder(id) {
  return request({
    url: `/admin/orders/${id}/cancel`,
    method: 'post'
  })
}

export function getNodes(params) {
  return request({
    url: '/admin/nodes',
    method: 'get',
    params
  })
}

export function getNodeStats() {
  return request({
    url: '/admin/nodes/stats',
    method: 'get'
  })
}

export function getNodeLogs(id, params) {
  return request({
    url: `/admin/nodes/${id}/logs`,
    method: 'get',
    params
  })
}

export function getNode(id) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'get'
  })
}

export function getNodeCredentials(id) {
  return request({
    url: `/admin/nodes/${id}/credentials`,
    method: 'get'
  })
}

export function createNode(data) {
  return request({
    url: '/admin/nodes',
    method: 'post',
    data
  })
}

export function updateNode(id, data) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'put',
    data
  })
}

export function deleteNode(id) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'delete'
  })
}

export function syncNodeProtocol(id) {
  return request({
    url: `/admin/nodes/${id}/sync`,
    method: 'post'
  })
}

export function getNodeRawConfig(id) {
  return request({
    url: `/admin/nodes/${id}/raw-config`,
    method: 'get'
  })
}

export function updateNodeRawConfig(id, data) {
  return request({
    url: `/admin/nodes/${id}/raw-config`,
    method: 'put',
    data
  })
}

export function validateNodeConfig(data) {
  return request({
    url: '/admin/nodes/validate-config',
    method: 'post',
    data
  })
}

export function getNodeProtocols(nodeId) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols`,
    method: 'get'
  })
}

export function createNodeProtocol(nodeId, data) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols`,
    method: 'post',
    data
  })
}

export function updateNodeProtocol(nodeId, protocolId, data) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols/${protocolId}`,
    method: 'put',
    data
  })
}

export function deleteNodeProtocol(nodeId, protocolId) {
  return request({
    url: `/admin/nodes/${nodeId}/protocols/${protocolId}`,
    method: 'delete'
  })
}

export function getProtocolTemplates() {
  return request({
    url: '/admin/protocol-templates',
    method: 'get'
  })
}

export function getAuthKeys() {
  return request({
    url: '/admin/auth-keys',
    method: 'get'
  })
}

export function generateAuthKey(data) {
  return request({
    url: '/admin/auth-keys',
    method: 'post',
    data
  })
}

export function deleteAuthKey(id) {
  return request({
    url: `/admin/auth-keys/${id}`,
    method: 'delete'
  })
}

export function getSubscriptionGroups() {
  return request({
    url: '/admin/subscription/groups',
    method: 'get'
  })
}

export function createSubscriptionGroup(data) {
  return request({
    url: '/admin/subscription/groups',
    method: 'post',
    data
  })
}

export function getSubscriptionGroup(id) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'get'
  })
}

export function updateSubscriptionGroup(id, data) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'put',
    data
  })
}

export function deleteSubscriptionGroup(id) {
  return request({
    url: `/admin/subscription/groups/${id}`,
    method: 'delete'
  })
}

export function getSubscriptionTemplates(groupId) {
  return request({
    url: `/admin/subscription/groups/${groupId}/templates`,
    method: 'get'
  })
}

export function getSubscriptionProtocols(groupId) {
  return request({
    url: `/admin/subscription/groups/${groupId}/protocols`,
    method: 'get'
  })
}

export function updateGroupProtocols(groupId, protocolIds) {
  return request({
    url: `/admin/subscription/groups/${groupId}/protocols`,
    method: 'post',
    data: { protocol_ids: protocolIds }
  })
}

export function getAvailableProtocols() {
  return request({
    url: '/admin/subscription/protocols/available',
    method: 'get'
  })
}

export function createSubscriptionTemplate(groupId, data) {
  return request({
    url: `/admin/subscription/groups/${groupId}/templates`,
    method: 'post',
    data
  })
}

export function getSubscriptionTemplate(id) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'get'
  })
}

export function updateSubscriptionTemplate(id, data) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'put',
    data
  })
}

export function deleteSubscriptionTemplate(id) {
  return request({
    url: `/admin/subscription/templates/${id}`,
    method: 'delete'
  })
}

export function previewSubscription(data) {
  return request({
    url: '/admin/subscription/preview',
    method: 'post',
    data
  })
}

export function getSubscriptionStats() {
  return request({
    url: '/admin/subscription/stats',
    method: 'get'
  })
}

export function getPlans() {
  return request({
    url: '/admin/plans',
    method: 'get'
  })
}

export function createPlan(data) {
  return request({
    url: '/admin/plans',
    method: 'post',
    data
  })
}

export function getPlan(id) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'get'
  })
}

export function updatePlan(id, data) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'put',
    data
  })
}

export function deletePlan(id) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'delete'
  })
}

export function assignPlanToUser(id, data) {
  return request({
    url: `/admin/plans/${id}/assign`,
    method: 'post',
    data
  })
}

export function getPlanGroups(planId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups`,
    method: 'get'
  })
}

export function addGroupToPlan(planId, groupId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups`,
    method: 'post',
    data: { group_id: groupId }
  })
}

export function removeGroupFromPlan(planId, groupId) {
  return request({
    url: `/admin/subscription/plans/${planId}/groups/${groupId}`,
    method: 'delete'
  })
}

export function getTickets(params) {
  return request({
    url: '/admin/ticket',
    method: 'get',
    params
  })
}

export function replyTicket(data) {
  return request({
    url: '/admin/ticket/reply',
    method: 'post',
    data
  })
}

export function closeTicket(id) {
  return request({
    url: `/admin/ticket/${id}/close`,
    method: 'post'
  })
}

export function getCoupons(params) {
  return request({
    url: '/admin/coupon',
    method: 'get',
    params
  })
}

export function createCoupon(data) {
  return request({
    url: '/admin/coupon',
    method: 'post',
    data
  })
}

export function deleteCoupon(id) {
  return request({
    url: `/admin/coupon/${id}`,
    method: 'delete'
  })
}

export function getKnowledgeList(params) {
  return request({
    url: '/admin/knowledge',
    method: 'get',
    params
  })
}

export function createKnowledge(data) {
  return request({
    url: '/admin/knowledge',
    method: 'post',
    data
  })
}

export function updateKnowledge(id, data) {
  return request({
    url: `/admin/knowledge/${id}`,
    method: 'put',
    data
  })
}

export function deleteKnowledge(id) {
  return request({
    url: `/admin/knowledge/${id}`,
    method: 'delete'
  })
}

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

export function getForwardRuntimeStatus() {
  return request({
    url: '/admin/forward/runtime/status',
    method: 'get'
  })
}

export function runForwardRuntimeDoctor() {
  return request({
    url: '/admin/forward/runtime/doctor',
    method: 'get'
  })
}

export function getLocalRuntimeStatus() {
  return request({
    url: '/admin/forward/local/status',
    method: 'get'
  })
}

export function runLocalRuntimeDoctor() {
  return request({
    url: '/admin/forward/local/doctor',
    method: 'get'
  })
}

export function getNodeXRuntimeStatus() {
  return request({
    url: '/admin/forward/nodex/status',
    method: 'get'
  })
}

export function runNodeXRuntimeDoctor() {
  return request({
    url: '/admin/forward/nodex/doctor',
    method: 'get'
  })
}

export function getForwardObservabilityTargets() {
  return request({
    url: '/admin/forward/observability/targets',
    method: 'get'
  })
}

export function getForwardObservabilityTrend(params) {
  return request({
    url: '/admin/forward/observability/trend',
    method: 'get',
    params
  })
}

export function getForwardObservabilityTopology() {
  return request({
    url: '/admin/forward/observability/topology',
    method: 'get'
  })
}

export function getForwardObservabilityMultiIngress(targetId) {
  return request({
    url: '/admin/forward/observability/multi-ingress',
    method: 'get',
    params: { targetId }
  })
}

export function getForwardTunnels() {
  return request({
    url: '/tunnel/user/tunnel',
    method: 'post'
  })
}

export function createForwardTunnel(data) {
  return request({
    url: '/admin/tunnel/create',
    method: 'post',
    data
  })
}

export function getAdminForwardTunnelList() {
  return request({
    url: '/admin/tunnel/list',
    method: 'post'
  })
}

export function updateForwardTunnel(data) {
  return request({
    url: '/admin/tunnel/update',
    method: 'post',
    data
  })
}

export function deleteForwardTunnel(id) {
  return request({
    url: '/admin/tunnel/delete',
    method: 'post',
    data: { id }
  })
}

export function diagnoseForwardTunnel(tunnelId) {
  return request({
    url: '/admin/tunnel/diagnose',
    method: 'post',
    data: { tunnelId }
  })
}

export function getSpeedLimitList() {
  return request({
    url: '/speed-limit/list',
    method: 'post'
  })
}

export function createSpeedLimit(data) {
  return request({
    url: '/speed-limit/create',
    method: 'post',
    data
  })
}

export function updateSpeedLimit(data) {
  return request({
    url: '/speed-limit/update',
    method: 'post',
    data
  })
}

export function deleteSpeedLimit(id) {
  return request({
    url: '/speed-limit/delete',
    method: 'post',
    data: { id }
  })
}

export function getSpeedLimitTunnels() {
  return request({
    url: '/speed-limit/tunnels',
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

export function createForwardNode(data, options = {}) {
  return request({
    url: '/admin/forward/nodes',
    method: 'post',
    data,
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

function normalizeForwardNodeRequestOptions(options = {}) {
  if (!options || typeof options !== 'object') {
    return {}
  }
  return options
}

export function getForwardNode(id, options = {}) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'get',
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

export function updateForwardNode(id, data, options = {}) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'put',
    data,
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

export function deleteForwardNode(id, options = {}) {
  return request({
    url: `/admin/forward/nodes/${id}`,
    method: 'delete',
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

export function checkForwardNode(id, options = {}) {
  return request({
    url: `/admin/forward/nodes/${id}/check`,
    method: 'post',
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

export function toggleForwardNode(id, enabled, options = {}) {
  return request({
    url: `/admin/forward/nodes/${id}/toggle`,
    method: 'post',
    data: { enabled },
    params: normalizeForwardNodeRequestOptions(options).params || {}
  })
}

export function getAnsibleMachines(params) {
  return request({
    url: '/admin/forward/ansible-machines',
    method: 'get',
    params
  })
}

export function createAnsibleMachine(data) {
  return request({
    url: '/admin/forward/ansible-machines',
    method: 'post',
    data
  })
}

export function getAnsibleMachine(id) {
  return request({
    url: `/admin/forward/ansible-machines/${id}`,
    method: 'get'
  })
}

export function updateAnsibleMachine(id, data) {
  return request({
    url: `/admin/forward/ansible-machines/${id}`,
    method: 'put',
    data
  })
}

export function deleteAnsibleMachine(id) {
  return request({
    url: `/admin/forward/ansible-machines/${id}`,
    method: 'delete'
  })
}

export function checkAnsibleMachine(id) {
  return request({
    url: `/admin/forward/ansible-machines/${id}/check`,
    method: 'post'
  })
}

export function toggleAnsibleMachine(id, enabled) {
  return request({
    url: `/admin/forward/ansible-machines/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

export function syncAnsibleMachineStats(id) {
  return request({
    url: `/admin/forward/ansible-machines/${id}/sync-stats`,
    method: 'post'
  })
}

export function getForwardRules(params) {
  return request({
    url: '/admin/forward/rules',
    method: 'get',
    params
  })
}

export function createForwardRule(data) {
  return request({
    url: '/admin/forward/rules',
    method: 'post',
    data
  })
}

export function getForwardRule(id) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'get'
  })
}

export function updateForwardRule(id, data) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'put',
    data
  })
}

export function deleteForwardRule(id) {
  return request({
    url: `/admin/forward/rules/${id}`,
    method: 'delete'
  })
}

export function toggleForwardRule(id, enabled) {
  return request({
    url: `/admin/forward/rules/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

export function getForwardStats() {
  return request({
    url: '/admin/forward/stats',
    method: 'get'
  })
}

export function getPaymentGateways() {
  return request({
    url: '/admin/payment/gateways',
    method: 'get'
  })
}

export function createPaymentGateway(data) {
  return request({
    url: '/admin/payment/gateways',
    method: 'post',
    data
  })
}

export function updatePaymentGateway(id, data) {
  return request({
    url: `/admin/payment/gateways/${id}`,
    method: 'put',
    data
  })
}

export function deletePaymentGateway(id) {
  return request({
    url: `/admin/payment/gateways/${id}`,
    method: 'delete'
  })
}

export function togglePaymentGateway(id, enabled) {
  return request({
    url: `/admin/payment/gateways/${id}/toggle`,
    method: 'post',
    data: { enabled }
  })
}

export function getPaymentStats(params) {
  return request({
    url: '/admin/payment/stats',
    method: 'get',
    params
  })
}

export function getPaymentRecords(params) {
  return request({
    url: '/admin/payment/records',
    method: 'get',
    params
  })
}

export function getTelegramBot() {
  return request({
    url: '/admin/telegram/bot',
    method: 'get'
  })
}

export function updateTelegramBot(data) {
  return request({
    url: '/admin/telegram/bot',
    method: 'put',
    data
  })
}

export function setTelegramWebhook(url) {
  return request({
    url: '/admin/telegram/webhook',
    method: 'post',
    data: { url }
  })
}

export function deleteTelegramWebhook() {
  return request({
    url: '/admin/telegram/webhook',
    method: 'delete'
  })
}

export function sendTelegramNotification(data) {
  return request({
    url: '/admin/telegram/notify',
    method: 'post',
    data
  })
}

export function broadcastTelegram(message) {
  return request({
    url: '/admin/telegram/broadcast',
    method: 'post',
    data: { message }
  })
}

export function getTelegramUsers(params) {
  return request({
    url: '/admin/telegram/users',
    method: 'get',
    params
  })
}

export function updateTelegramUserNotify(id, data) {
  return request({
    url: `/admin/telegram/users/${id}/notify`,
    method: 'put',
    data
  })
}

export function getMFAConfig() {
  return request({
    url: '/admin/mfa/config',
    method: 'get'
  })
}

export function updateMFAConfig(data) {
  return request({
    url: '/admin/mfa/config',
    method: 'put',
    data
  })
}

export function getNotificationTemplates(params) {
  return request({
    url: '/admin/notification/templates',
    method: 'get',
    params
  })
}

export function createNotificationTemplate(data) {
  return request({
    url: '/admin/notification/templates',
    method: 'post',
    data
  })
}

export function updateNotificationTemplate(id, data) {
  return request({
    url: `/admin/notification/templates/${id}`,
    method: 'put',
    data
  })
}

export function deleteNotificationTemplate(id) {
  return request({
    url: `/admin/notification/templates/${id}`,
    method: 'delete'
  })
}

export function getNotificationLogs(params) {
  return request({
    url: '/admin/notification/logs',
    method: 'get',
    params
  })
}

export function sendTestNotification(data) {
  return request({
    url: '/admin/notification/test',
    method: 'post',
    data
  })
}

export function getEmailConfig() {
  return request({
    url: '/admin/notification/email/config',
    method: 'get'
  })
}

export function updateEmailConfig(data) {
  return request({
    url: '/admin/notification/email/config',
    method: 'put',
    data
  })
}

export function getInviteConfig() {
  return request({
    url: '/admin/invite/config',
    method: 'get'
  })
}

export function updateInviteConfig(data) {
  return request({
    url: '/admin/invite/config',
    method: 'put',
    data
  })
}

export function getInviteStats() {
  return request({
    url: '/admin/invite/stats',
    method: 'get'
  })
}

export function getWithdrawals(params) {
  return request({
    url: '/admin/invite/withdrawals',
    method: 'get',
    params
  })
}

export function processWithdrawal(id, data) {
  return request({
    url: `/admin/invite/withdrawals/${id}/process`,
    method: 'post',
    data
  })
}

export function getSystemConfigs(params) {
  return request({
    url: '/admin/system/configs',
    method: 'get',
    params
  })
}

export function getSystemConfig(key) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'get'
  })
}

export function getSubscriptionSettings() {
  return request({
    url: '/admin/system/subscription-settings',
    method: 'get'
  })
}

export function setSystemConfig(key, data) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'put',
    data
  })
}

export function deleteSystemConfig(key) {
  return request({
    url: `/admin/system/configs/${key}`,
    method: 'delete'
  })
}

export function getSystemAuditLogs(params) {
  return request({
    url: '/admin/system/audit-logs',
    method: 'get',
    params
  })
}

export function getBackupConfig() {
  return request({
    url: '/admin/system/backup/config',
    method: 'get'
  })
}

export function updateBackupConfig(data) {
  return request({
    url: '/admin/system/backup/config',
    method: 'put',
    data
  })
}

export function createBackup(type = 'database') {
  return request({
    url: '/admin/system/backup',
    method: 'post',
    params: { type }
  })
}

export function getBackups(params) {
  return request({
    url: '/admin/system/backups',
    method: 'get',
    params
  })
}

export function getBackupStats() {
  return request({
    url: '/admin/system/backup/stats',
    method: 'get'
  })
}

export function deleteBackup(id) {
  return request({
    url: `/admin/system/backups/${id}`,
    method: 'delete'
  })
}

export function restoreBackup(id) {
  return request({
    url: `/admin/system/backups/${id}/restore`,
    method: 'post'
  })
}

export function getAgents() {
  return request({
    url: '/admin/agent/list',
    method: 'get'
  })
}

export function createAgentTask(data) {
  return request({
    url: '/admin/agent/tasks',
    method: 'post',
    data
  })
}

export function executeAgentCommand(data) {
  return request({
    url: '/admin/agent/execute',
    method: 'post',
    data
  })
}

export function getAgentTaskResult(taskId) {
  return request({
    url: `/admin/agent/tasks/${taskId}`,
    method: 'get'
  })
}

export function listAgentDiagnosticTasks(params) {
  return request({
    url: '/admin/agent/tasks',
    method: 'get',
    params
  })
}

export function getAgentMonitor(nodeId) {
  return request({
    url: '/admin/agent/monitor',
    method: 'get',
    params: { node_id: nodeId }
  })
}

export function getLoadBalancers(params) {
  return request({
    url: '/admin/loadbalancers',
    method: 'get',
    params
  })
}

export function createLoadBalancer(data) {
  return request({
    url: '/admin/loadbalancers',
    method: 'post',
    data
  })
}

export function getLoadBalancer(id) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'get'
  })
}

export function updateLoadBalancer(id, data) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'put',
    data
  })
}

export function deleteLoadBalancer(id) {
  return request({
    url: `/admin/loadbalancers/${id}`,
    method: 'delete'
  })
}

export function getLoadBalancerStats(id) {
  return request({
    url: `/admin/loadbalancers/${id}/stats`,
    method: 'get'
  })
}

export function runHealthCheck(id) {
  return request({
    url: `/admin/loadbalancers/${id}/check`,
    method: 'post'
  })
}

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
  getNodeLogs,
  createNode,
  getNode,
  getNodeCredentials,
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
  createForwardTunnel,
  getAdminForwardTunnelList,
  updateForwardTunnel,
  deleteForwardTunnel,
  diagnoseForwardTunnel,
  getSpeedLimitList,
  createSpeedLimit,
  updateSpeedLimit,
  deleteSpeedLimit,
  getSpeedLimitTunnels,
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
  syncForwardNodeStats,
  testForwardConnection,
  toggleForwardNode,
  getAnsibleMachines,
  createAnsibleMachine,
  getAnsibleMachine,
  updateAnsibleMachine,
  deleteAnsibleMachine,
  checkAnsibleMachine,
  toggleAnsibleMachine,
  syncAnsibleMachineStats,
  getForwardRules,
  createForwardRule,
  getForwardRule,
  updateForwardRule,
  deleteForwardRule,
  toggleForwardRule,
  getForwardStats,
  getForwardRuntimeStatus,
  runForwardRuntimeDoctor,
  getLocalRuntimeStatus,
  runLocalRuntimeDoctor,
  getNodeXRuntimeStatus,
  runNodeXRuntimeDoctor,
  getPaymentGateways,
  createPaymentGateway,
  updatePaymentGateway,
  deletePaymentGateway,
  togglePaymentGateway,
  getPaymentStats,
  getPaymentRecords,
  getTelegramBot,
  updateTelegramBot,
  setTelegramWebhook,
  deleteTelegramWebhook,
  sendTelegramNotification,
  broadcastTelegram,
  getTelegramUsers,
  updateTelegramUserNotify,
  getMFAConfig,
  updateMFAConfig,
  getNotificationTemplates,
  createNotificationTemplate,
  updateNotificationTemplate,
  deleteNotificationTemplate,
  getNotificationLogs,
  sendTestNotification,
  getEmailConfig,
  updateEmailConfig,
  getInviteConfig,
  updateInviteConfig,
  getInviteStats,
  getWithdrawals,
  processWithdrawal,
  getSystemConfigs,
  getSystemConfig,
  getSubscriptionSettings,
  setSystemConfig,
  deleteSystemConfig,
  getSystemAuditLogs,
  getBackupConfig,
  updateBackupConfig,
  createBackup,
  getBackups,
  getBackupStats,
  deleteBackup,
  restoreBackup,
  getAgents,
  createAgentTask,
  executeAgentCommand,
  getAgentTaskResult,
  listAgentDiagnosticTasks,
  getAgentMonitor,
  getLoadBalancers,
  createLoadBalancer,
  getLoadBalancer,
  updateLoadBalancer,
  deleteLoadBalancer,
  getLoadBalancerStats,
  runHealthCheck
}

export function syncForwardNodeStats(id, options = {}) {
  const normalizedOptions = normalizeForwardNodeRequestOptions(options)
  return request({
    url: `/admin/forward/nodes/${id}/sync-stats`,
    method: 'post',
    ...(normalizedOptions.params ? { params: normalizedOptions.params } : {})
  })
}

export function testForwardConnection(data) {
  return request({
    url: '/admin/forward/test-connection',
    method: 'post',
    data
  })
}
