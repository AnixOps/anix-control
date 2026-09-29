import type { Hono } from 'hono'
import type { ApiErrorResponse, Env } from '../types'

// Handlers
import { healthHandler, readinessHandler } from '../handlers/health'
import { loginHandler, registerHandler, refreshHandler, logoutHandler, meHandler } from '../handlers/auth'
import { listNodesHandler, getNodeHandler, createNodeHandler, updateNodeHandler, deleteNodeHandler, startNodeHandler, stopNodeHandler, restartNodeHandler, getNodeStatsHandler, getNodeLogsHandler, testNodeConnectionHandler, syncNodeHandler, bulkActionHandler } from '../handlers/nodes'
import { listPlaybooksHandler, getPlaybookHandler, uploadPlaybookHandler, deletePlaybookHandler, listBuiltInPlaybooksHandler, getPlaybookCategoriesHandler, syncBuiltInPlaybooksHandler } from '../handlers/playbooks'
import { listPluginsHandler, getPluginHandler, executePluginHandler } from '../handlers/plugins'
import { dashboardHandler, statsHandler } from '../handlers/dashboard'
import { listAuditLogsHandler } from '../handlers/audit'
import { listUsersHandler, getUserHandler, createUserHandler, updateUserHandler, deleteUserHandler, changePasswordHandler, getCurrentUserHandler, updateCurrentUserHandler, listApiTokensHandler, createApiTokenHandler, deleteApiTokenHandler, listSessionsHandler, deleteOtherSessionsHandler, getUserLockoutHandler, unlockUserHandler } from '../handlers/users'
import { testConnectionHandler, importServerHandler, detectServerTypeHandler } from '../handlers/ssh'
import { listNotificationsHandler, markNotificationReadHandler, markAllNotificationsReadHandler, deleteNotificationHandler, createNotificationHandler, getUnreadCountHandler } from '../handlers/notifications'
import { listTasksHandler, getTaskHandler, createTaskHandler, cancelTaskHandler, retryTaskHandler, getTaskLogsHandler } from '../handlers/tasks'
import { listSchedulesHandler, getScheduleHandler, createScheduleHandler, updateScheduleHandler, deleteScheduleHandler, toggleScheduleHandler, runScheduleNowHandler } from '../handlers/schedules'
import { listNodeGroupsHandler, getNodeGroupHandler, createNodeGroupHandler, updateNodeGroupHandler, deleteNodeGroupHandler, addNodesToGroupHandler, removeNodesFromGroupHandler } from '../handlers/node-groups'
import { sseHandler, sseSubscribeHandler, sseUnsubscribeHandler, sseStatusHandler } from '../handlers/sse'
import { websocketHandler } from '../handlers/websocket'
import {
  listIncidentsHandler,
  getIncidentHandler,
  createIncidentHandler,
  analyzeIncidentHandler,
  approveIncidentHandler,
  executeIncidentHandler,
  acknowledgeIncidentHandler,
  escalateIncidentHandler,
  assignIncidentHandler,
  unassignIncidentHandler,
  getIncidentSlaStatusHandler,
  getIncidentTimelineHandler,
  getIncidentStatisticsHandler,
  getIncidentReportHandler,
  searchIncidentsHandler,
  listIncidentCommentsHandler,
  addIncidentCommentHandler,
  updateIncidentCommentHandler,
  deleteIncidentCommentHandler,
  bulkApproveIncidentsHandler,
  bulkExecuteIncidentsHandler,
  bulkAnalyzeIncidentsHandler,
  bulkDeleteIncidentsHandler,
  mergeIncidentsHandler,
  listTagsHandler,
  addTagsHandler,
  removeTagsHandler,
  setTagsHandler,
  listSuppressionRulesHandler,
  createSuppressionRuleHandler,
  deleteSuppressionRuleHandler,
  toggleSuppressionRuleHandler,
  listNotificationRulesHandler,
  createNotificationRuleHandler,
  deleteNotificationRuleHandler,
  toggleNotificationRuleHandler,
  addIncidentLinkHandler,
  removeIncidentLinkHandler,
  addIncidentEvidenceHandler,
  getIncidentActivityLogHandler,
  getRunbookSuggestionsHandler,
  executeRunbookHandler,
  listTemplatesHandler,
  getTemplateHandler,
  createTemplateHandler,
  updateTemplateHandler,
  deleteTemplateHandler,
  createFromTemplateHandler,
  listAutomationRulesHandler,
  createAutomationRuleHandler,
  deleteAutomationRuleHandler,
  toggleAutomationRuleHandler,
  getPostMortemHandler,
  createPostMortemHandler,
  updatePostMortemHandler,
  updateActionItemHandler,
  getIncidentDashboardMetricsHandler,
  getIncidentCorrelationHandler,
  watchIncidentHandler,
  unwatchIncidentHandler,
  getIncidentWatchersHandler,
  createExternalTicketHandler,
  listExternalTicketsHandler,
  updateExternalTicketHandler,
  // Advanced features
  listResponsePlaybooksHandler,
  getResponsePlaybookHandler,
  createResponsePlaybookHandler,
  updateResponsePlaybookHandler,
  deleteResponsePlaybookHandler,
  matchResponsePlaybooksHandler,
  startPlaybookExecutionHandler,
  getPlaybookExecutionHandler,
  completePlaybookStepHandler,
  skipPlaybookStepHandler,
  listCustomFieldsHandler,
  getCustomFieldHandler,
  createCustomFieldHandler,
  updateCustomFieldHandler,
  deleteCustomFieldHandler,
  setIncidentCustomFieldHandler,
  getIncidentCustomFieldsHandler,
  generateAIRootCauseAnalysisHandler,
  createWarRoomHandler,
  getWarRoomHandler,
  joinWarRoomHandler,
  leaveWarRoomHandler,
  addWarRoomMessageHandler,
  addWarRoomResourceHandler,
  closeWarRoomHandler,
  exportIncidentsHandler,
  getExportStatusHandler,
  downloadExportHandler,
  // Reviews
  createReviewHandler,
  listReviewsHandler,
  completeReviewHandler,
  // Analytics
  getResponseAnalyticsHandler,
  // Feedback
  submitFeedbackHandler,
  getFeedbackHandler,
  // Cost
  calculateCostHandler,
  getCostHandler,
  // Compliance
  createComplianceHandler,
  getComplianceHandler,
  updateComplianceHandler,
  // On-call
  listOnCallSchedulesHandler,
  getOnCallScheduleHandler,
  createOnCallScheduleHandler,
  getCurrentOnCallHandler,
  // Checklists
  listChecklistsHandler,
  createChecklistHandler,
  updateChecklistItemHandler,
  // Change links
  linkChangeHandler,
  listChangesHandler,
  // Run history
  listRunHistoryHandler,
  // Responder teams
  listResponderTeamsHandler,
  getResponderTeamHandler,
  createResponderTeamHandler,
  updateResponderTeamHandler,
  deleteResponderTeamHandler,
  // SLA calendars
  listSLACalendarsHandler,
  getSLACalendarHandler,
  createSLACalendarHandler,
  // Notification templates
  listNotificationTemplatesHandler,
  getNotificationTemplateHandler,
  createNotificationTemplateHandler,
  // Escalation rules
  listEscalationRulesHandler,
  createEscalationRuleHandler,
  // Attachments
  listAttachmentsHandler,
  uploadAttachmentHandler,
  downloadAttachmentHandler,
  deleteAttachmentHandler,
  // Related items
  listRelatedItemsHandler,
  addRelatedItemHandler,
  removeRelatedItemHandler,
  // Response targets
  listResponseTargetsHandler,
  createResponseTargetHandler,
  // Integrations
  listIntegrationsHandler,
  getIntegrationHandler,
  createIntegrationHandler,
  updateIntegrationHandler,
  deleteIntegrationHandler,
  // Timeline events
  listTimelineEventsHandler,
  addTimelineEventHandler,
  // Runbooks
  listRunbooksHandler,
  getRunbookHandler,
  createRunbookHandler,
  updateRunbookHandler,
  deleteRunbookHandler,
  // Auto-remediation
  listAutoRemediationRulesHandler,
  createAutoRemediationRuleHandler,
  updateAutoRemediationRuleHandler,
  deleteAutoRemediationRuleHandler,
  // Maintenance windows
  listMaintenanceWindowsHandler,
  getMaintenanceWindowHandler,
  createMaintenanceWindowHandler,
  updateMaintenanceWindowHandler,
  cancelMaintenanceWindowHandler,
  // Bulk operations
  listBulkOperationsHandler,
  getBulkOperationHandler,
  createBulkOperationHandler,
  executeBulkOperationHandler,
  // SLA breaches
  listSLABreachesHandler,
  acknowledgeSLABreachHandler,
  // Analytics
  listAnalyticsSnapshotsHandler,
  generateAnalyticsSnapshotHandler,
  // Webhook subscriptions
  listWebhookSubscriptionsHandler,
  getWebhookSubscriptionHandler,
  createWebhookSubscriptionHandler,
  updateWebhookSubscriptionHandler,
  deleteWebhookSubscriptionHandler,
  // Snooze
  listSnoozesHandler,
  createSnoozeHandler,
  wakeSnoozeHandler,
  // Merge
  listMergesHandler,
  createMergeHandler,
  // Split
  listSplitsHandler,
  createSplitHandler,
  // Recurrence
  listRecurrencesHandler,
  detectRecurrenceHandler,
  markRootCauseResolvedHandler,
} from '../handlers/incidents'
import {
  listPoliciesHandler,
  getPolicyHandler,
  getActivePolicyHandler,
  createPolicyHandler,
  updatePolicyHandler,
  deletePolicyHandler,
} from '../handlers/governance'
import {
  listWebhooksHandler,
  getWebhookHandler,
  createWebhookHandler,
  updateWebhookHandler,
  deleteWebhookHandler,
  listWebhookDeliveriesHandler,
  retryDeliveryHandler,
  listFailedDeliveriesHandler,
} from '../handlers/webhooks'
import { createBackupHandler, listBackupsHandler, getBackupHandler, deleteBackupHandler, downloadBackupHandler, restoreBackupHandler, cleanupBackupsHandler, backupStatusHandler } from '../handlers/backup'

// AI Services
import {
  aiChatHandler,
  aiAnalyzeLogHandler,
  aiOpsAdviceHandler,
  aiEmbeddingHandler,
  aiQueryHandler,
} from '../services/ai'
import {
  vectorSearchHandler,
  vectorInsertHandler,
  vectorDeleteHandler,
} from '../services/vectorize'

// Web3 Services
import {
  ipfsUploadHandler,
  ipfsGetHandler,
  web3ChallengeHandler,
  web3VerifyHandler,
  web3AuditHandler,
} from '../services/web3'
import {
  registerAgentHandler,
  agentHeartbeatHandler,
  agentMetricsHandler,
  agentCommandResultHandler,
  sendAgentCommandHandler,
  getAgentMetricsHandler,
  generateInstallScriptHandler,
} from '../handlers/agents'
import {
  getMFAStatusHandler,
  setupMFAHandler,
  enableMFAHandler,
  disableMFAHandler,
  verifyMFAHandler,
  regenerateRecoveryCodesHandler,
  adminDisableMFAHandler,
} from '../handlers/mfa'
import { batchOperationsHandler, bulkNodeStatusHandler } from '../handlers/batch'
import { prometheusMetricsHandler, detailedHealthHandler, readinessHandler as k8sReadinessHandler, livenessHandler } from '../handlers/metrics'
import { cacheMiddleware } from '../middleware/cache'
import {
  listNamespacesHandler,
  listPodsHandler,
  listDeploymentsHandler,
  listClusterNodesHandler,
  listServicesHandler,
  listEventsHandler,
  getPodLogsHandler,
  scaleDeploymentHandler,
  restartDeploymentHandler,
  getClusterOverviewHandler,
  getNamespaceDetailsHandler,
} from '../handlers/kubernetes'
import {
  listMeshServicesHandler,
  listVirtualServicesHandler,
  listDestinationRulesHandler,
  listGatewaysHandler,
  configureTrafficSplitHandler,
  configureCircuitBreakerHandler,
  injectFaultHandler,
  getMeshOverviewHandler,
} from '../handlers/istio'
import {
  searchLogsHandler,
  getLogHandler,
  indexLogHandler,
  bulkIndexLogsHandler,
  getLogStatsHandler,
  deleteOldLogsHandler,
  createLogIndexHandler,
  exportLogsHandler,
  getTraceLogsHandler,
  getNodeLogsV2Handler,
  getServiceLogsHandler,
} from '../handlers/elasticsearch'
import {
  listScalingPoliciesHandler,
  getScalingPolicyHandler,
  createScalingPolicyHandler,
  updateScalingPolicyHandler,
  deleteScalingPolicyHandler,
  evaluateScalingPolicyHandler,
  executeScalingActionHandler,
  getScalingHistoryHandler,
  checkHealthHandler,
  getRecommendedReplicasHandler,
  runScalingCheckHandler,
  toggleScalingPolicyHandler,
  getScalingMetricsHandler,
} from '../handlers/autoscaling'
import {
  listLoadBalancersHandler,
  getLoadBalancerHandler,
  createLoadBalancerHandler,
  updateLoadBalancerHandler,
  deleteLoadBalancerHandler,
  selectTargetHandler,
  checkTargetHealthHandler,
  runHealthChecksHandler,
  getLoadBalancerStatsHandler,
  addTargetHandler,
  removeTargetHandler,
  updateTargetWeightHandler,
  recordCompletionHandler,
  toggleLoadBalancerHandler,
} from '../handlers/loadbalancer'

import {
  developerModeStatusHandler,
  developerDiagnosticsHandler,
  developerFixturesCatalogHandler,
  developerReadinessSummaryHandler,
} from '../handlers/internal-dev'

// Middleware
import { authMiddleware, rbacMiddleware } from '../middleware/auth'
import { rateLimitMiddleware } from '../middleware/rate-limit'

export function registerProtectedCoreRoutes(app: Hono<{ Bindings: Env }>) {
  // 用户信息
  app.get('/api/v1/users/me', authMiddleware, getCurrentUserHandler)
  app.put('/api/v1/users/me', authMiddleware, updateCurrentUserHandler)
  app.put('/api/v1/auth/password', authMiddleware, changePasswordHandler)

  // API Tokens
  app.get('/api/v1/users/me/tokens', authMiddleware, listApiTokensHandler)
  app.post('/api/v1/users/me/tokens', authMiddleware, createApiTokenHandler)
  app.delete('/api/v1/users/me/tokens/:id', authMiddleware, deleteApiTokenHandler)

  // Sessions
  app.get('/api/v1/users/me/sessions', authMiddleware, listSessionsHandler)
  app.delete('/api/v1/users/me/sessions/others', authMiddleware, deleteOtherSessionsHandler)

  // 用户管理 (需要管理员权限)
  app.get('/api/v1/users', authMiddleware, rbacMiddleware(['admin']), listUsersHandler)
  app.get('/api/v1/users/:id', authMiddleware, rbacMiddleware(['admin']), getUserHandler)
  app.post('/api/v1/users', authMiddleware, rbacMiddleware(['admin']), createUserHandler)
  app.put('/api/v1/users/:id', authMiddleware, rbacMiddleware(['admin']), updateUserHandler)
  app.delete('/api/v1/users/:id', authMiddleware, rbacMiddleware(['admin']), deleteUserHandler)

  // 账户锁定管理 (需要管理员权限)
  app.get('/api/v1/users/:id/lockout', authMiddleware, rbacMiddleware(['admin']), getUserLockoutHandler)
  app.post('/api/v1/users/:id/unlock', authMiddleware, rbacMiddleware(['admin']), unlockUserHandler)

  // SSH导入 - 所有登录用户都可以导入服务器
  app.post('/api/v1/ssh/test', authMiddleware, testConnectionHandler)
  app.post('/api/v1/ssh/import', authMiddleware, importServerHandler)
  app.post('/api/v1/ssh/detect', authMiddleware, detectServerTypeHandler)

  // 节点管理 - 所有登录用户可查看和添加，operator/admin可操作，仅admin可删除
  app.get('/api/v1/nodes', authMiddleware, listNodesHandler)
  app.get('/api/v1/nodes/:id', authMiddleware, getNodeHandler)
  app.get('/api/v1/nodes/:id/stats', authMiddleware, getNodeStatsHandler)
  app.get('/api/v1/nodes/:id/logs', authMiddleware, getNodeLogsHandler)
  app.post('/api/v1/nodes', authMiddleware, createNodeHandler)  // 所有登录用户可添加
  app.post('/api/v1/nodes/bulk', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkActionHandler)
  app.post('/api/v1/nodes/bulk-status', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkNodeStatusHandler)
  app.post('/api/v1/nodes/:id/start', authMiddleware, rbacMiddleware(['admin', 'operator']), startNodeHandler)
  app.post('/api/v1/nodes/:id/stop', authMiddleware, rbacMiddleware(['admin', 'operator']), stopNodeHandler)
  app.post('/api/v1/nodes/:id/restart', authMiddleware, rbacMiddleware(['admin', 'operator']), restartNodeHandler)
  app.post('/api/v1/nodes/:id/test', authMiddleware, testNodeConnectionHandler)  // 所有用户可测试连接
  app.post('/api/v1/nodes/:id/sync', authMiddleware, rbacMiddleware(['admin', 'operator']), syncNodeHandler)
  app.put('/api/v1/nodes/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), updateNodeHandler)
  app.delete('/api/v1/nodes/:id', authMiddleware, rbacMiddleware(['admin']), deleteNodeHandler)

  // Playbook 管理
  app.get('/api/v1/playbooks', authMiddleware, listPlaybooksHandler)
  app.get('/api/v1/playbooks/built-in', authMiddleware, listBuiltInPlaybooksHandler)
  app.get('/api/v1/playbooks/categories', authMiddleware, getPlaybookCategoriesHandler)
  app.post('/api/v1/playbooks/sync-builtin', authMiddleware, rbacMiddleware(['admin']), syncBuiltInPlaybooksHandler)
  app.get('/api/v1/playbooks/:name', authMiddleware, getPlaybookHandler)
  app.post('/api/v1/playbooks', authMiddleware, rbacMiddleware(['admin', 'operator']), uploadPlaybookHandler)
  app.delete('/api/v1/playbooks/:name', authMiddleware, rbacMiddleware(['admin']), deletePlaybookHandler)

  // 任务管理
  app.get('/api/v1/tasks', authMiddleware, listTasksHandler)
  app.post('/api/v1/tasks', authMiddleware, rbacMiddleware(['admin', 'operator']), createTaskHandler)
  app.get('/api/v1/tasks/:id', authMiddleware, getTaskHandler)
  app.get('/api/v1/tasks/:id/logs', authMiddleware, getTaskLogsHandler)
  app.post('/api/v1/tasks/:id/cancel', authMiddleware, rbacMiddleware(['admin', 'operator']), cancelTaskHandler)
  app.post('/api/v1/tasks/:id/retry', authMiddleware, rbacMiddleware(['admin', 'operator']), retryTaskHandler)

  // 调度管理
  app.get('/api/v1/schedules', authMiddleware, listSchedulesHandler)
  app.post('/api/v1/schedules', authMiddleware, rbacMiddleware(['admin', 'operator']), createScheduleHandler)
  app.get('/api/v1/schedules/:id', authMiddleware, getScheduleHandler)
  app.put('/api/v1/schedules/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), updateScheduleHandler)
  app.delete('/api/v1/schedules/:id', authMiddleware, rbacMiddleware(['admin']), deleteScheduleHandler)
  app.post('/api/v1/schedules/:id/toggle', authMiddleware, rbacMiddleware(['admin', 'operator']), toggleScheduleHandler)
  app.post('/api/v1/schedules/:id/run', authMiddleware, rbacMiddleware(['admin', 'operator']), runScheduleNowHandler)

  // 节点组管理
  app.get('/api/v1/node-groups', authMiddleware, listNodeGroupsHandler)
  app.post('/api/v1/node-groups', authMiddleware, rbacMiddleware(['admin', 'operator']), createNodeGroupHandler)
  app.get('/api/v1/node-groups/:id', authMiddleware, getNodeGroupHandler)
  app.put('/api/v1/node-groups/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), updateNodeGroupHandler)
  app.delete('/api/v1/node-groups/:id', authMiddleware, rbacMiddleware(['admin']), deleteNodeGroupHandler)
  app.post('/api/v1/node-groups/:id/nodes', authMiddleware, rbacMiddleware(['admin', 'operator']), addNodesToGroupHandler)
  app.delete('/api/v1/node-groups/:id/nodes', authMiddleware, rbacMiddleware(['admin', 'operator']), removeNodesFromGroupHandler)

  // 插件管理
  app.get('/api/v1/plugins', authMiddleware, listPluginsHandler)
  app.get('/api/v1/plugins/:name', authMiddleware, getPluginHandler)
  app.post('/api/v1/plugins/:name/execute', authMiddleware, rbacMiddleware(['admin', 'operator']), executePluginHandler)

  // Dashboard
  app.get('/api/v1/dashboard', authMiddleware, dashboardHandler)
  app.get('/api/v1/dashboard/stats', authMiddleware, statsHandler)

  // 审计日志
  app.get('/api/v1/audit-logs', authMiddleware, rbacMiddleware(['admin']), listAuditLogsHandler)

  // 通知管理
  app.get('/api/v1/notifications', authMiddleware, listNotificationsHandler)
  app.get('/api/v1/notifications/unread-count', authMiddleware, getUnreadCountHandler)
  app.post('/api/v1/notifications', authMiddleware, rbacMiddleware(['admin', 'operator']), createNotificationHandler)
  app.put('/api/v1/notifications/:id/read', authMiddleware, markNotificationReadHandler)
  app.put('/api/v1/notifications/read-all', authMiddleware, markAllNotificationsReadHandler)
  app.delete('/api/v1/notifications/:id', authMiddleware, deleteNotificationHandler)

}
