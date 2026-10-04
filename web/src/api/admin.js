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

// The user list shows no subscription token: read it, with the rest of the
// user's row, from the user detail.
export function getAdminUser(id) {
  return request({
    url: `/admin/users/${id}`,
    method: 'get'
  })
}

export function getUserStats() {
  return request({
    url: '/admin/users/stats',
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

// One node with its protocols (the node detail page, UI U7).
export function getNode(id) {
  return request({
    url: `/admin/nodes/${id}`,
    method: 'get'
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

export function generateWireGuardKeypair() {
  return request({
    url: '/admin/wireguard/keypair',
    method: 'post'
  })
}

export function getAuthKeys() {
  return request({
    url: '/admin/auth-keys',
    method: 'get'
  })
}

// The answer is the only one that shows the key; the list masks it.
export function generateAuthKey(data) {
  return request({
    url: '/admin/auth-keys',
    method: 'post',
    data
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

// Invite codes (registration control, every edition).
export function getInviteCodes(params) {
  return request({
    url: '/admin/invite/codes',
    method: 'get',
    params
  })
}

export function generateInviteCodes(data) {
  return request({
    url: '/admin/invite/codes',
    method: 'post',
    data
  })
}

export function revokeInviteCode(id) {
  return request({
    url: `/admin/invite/codes/${id}`,
    method: 'delete'
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

export function listAgentDiagnosticTasks(params) {
  return request({
    url: '/admin/agent/tasks',
    method: 'get',
    params
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
  updateUser,
  banUser,
  unbanUser,
  resetUserTraffic,
  getOrderList,
  getOrderStats,
  markOrderPaid,
  cancelOrder,
  getNodes,
  getNode,
  getNodeStats,
  getNodeLogs,
  createNode,
  getNodeCredentials,
  updateNode,
  deleteNode,
  syncNodeProtocol,
  getNodeProtocols,
  createNodeProtocol,
  updateNodeProtocol,
  deleteNodeProtocol,
  getProtocolTemplates,
  getAuthKeys,
  generateAuthKey,
  getSubscriptionGroups,
  createSubscriptionGroup,
  updateSubscriptionGroup,
  deleteSubscriptionGroup,
  getSubscriptionTemplates,
  getSubscriptionProtocols,
  updateGroupProtocols,
  getAvailableProtocols,
  createSubscriptionTemplate,
  updateSubscriptionTemplate,
  deleteSubscriptionTemplate,
  previewSubscription,
  getPlans,
  createPlan,
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
  getForwardRuntimeStatus,
  runForwardRuntimeDoctor,
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
  getInviteCodes,
  generateInviteCodes,
  revokeInviteCode,
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
  listAgentDiagnosticTasks,
  getLoadBalancers,
  createLoadBalancer,
  updateLoadBalancer,
  deleteLoadBalancer,
  runHealthCheck
}
