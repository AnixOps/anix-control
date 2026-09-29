import { Hono } from 'hono'
import type { ApiErrorResponse, Env } from './types'
import { createApp } from './app/create-app'

// Handlers
import { healthHandler, readinessHandler } from './handlers/health'
import { loginHandler, registerHandler, refreshHandler, logoutHandler, meHandler } from './handlers/auth'
import { listNodesHandler, getNodeHandler, createNodeHandler, updateNodeHandler, deleteNodeHandler, startNodeHandler, stopNodeHandler, restartNodeHandler, getNodeStatsHandler, getNodeLogsHandler, testNodeConnectionHandler, syncNodeHandler, bulkActionHandler } from './handlers/nodes'
import { listPlaybooksHandler, getPlaybookHandler, uploadPlaybookHandler, deletePlaybookHandler, listBuiltInPlaybooksHandler, getPlaybookCategoriesHandler, syncBuiltInPlaybooksHandler } from './handlers/playbooks'
import { listPluginsHandler, getPluginHandler, executePluginHandler } from './handlers/plugins'
import { dashboardHandler, statsHandler } from './handlers/dashboard'
import { listAuditLogsHandler } from './handlers/audit'
import { listUsersHandler, getUserHandler, createUserHandler, updateUserHandler, deleteUserHandler, changePasswordHandler, getCurrentUserHandler, updateCurrentUserHandler, listApiTokensHandler, createApiTokenHandler, deleteApiTokenHandler, listSessionsHandler, deleteOtherSessionsHandler, getUserLockoutHandler, unlockUserHandler } from './handlers/users'
import { testConnectionHandler, importServerHandler, detectServerTypeHandler } from './handlers/ssh'
import { listNotificationsHandler, markNotificationReadHandler, markAllNotificationsReadHandler, deleteNotificationHandler, createNotificationHandler, getUnreadCountHandler } from './handlers/notifications'
import { listTasksHandler, getTaskHandler, createTaskHandler, cancelTaskHandler, retryTaskHandler, getTaskLogsHandler } from './handlers/tasks'
import { listSchedulesHandler, getScheduleHandler, createScheduleHandler, updateScheduleHandler, deleteScheduleHandler, toggleScheduleHandler, runScheduleNowHandler } from './handlers/schedules'
import { listNodeGroupsHandler, getNodeGroupHandler, createNodeGroupHandler, updateNodeGroupHandler, deleteNodeGroupHandler, addNodesToGroupHandler, removeNodesFromGroupHandler } from './handlers/node-groups'
import { sseHandler, sseSubscribeHandler, sseUnsubscribeHandler, sseStatusHandler } from './handlers/sse'
import { websocketHandler } from './handlers/websocket'
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
} from './handlers/incidents'
import {
  listPoliciesHandler,
  getPolicyHandler,
  getActivePolicyHandler,
  createPolicyHandler,
  updatePolicyHandler,
  deletePolicyHandler,
} from './handlers/governance'
import {
  listWebhooksHandler,
  getWebhookHandler,
  createWebhookHandler,
  updateWebhookHandler,
  deleteWebhookHandler,
  listWebhookDeliveriesHandler,
  retryDeliveryHandler,
  listFailedDeliveriesHandler,
} from './handlers/webhooks'
import { createBackupHandler, listBackupsHandler, getBackupHandler, deleteBackupHandler, downloadBackupHandler, restoreBackupHandler, cleanupBackupsHandler, backupStatusHandler } from './handlers/backup'

// AI Services
import {
  aiChatHandler,
  aiAnalyzeLogHandler,
  aiOpsAdviceHandler,
  aiEmbeddingHandler,
  aiQueryHandler,
} from './services/ai'
import {
  vectorSearchHandler,
  vectorInsertHandler,
  vectorDeleteHandler,
} from './services/vectorize'

// Web3 Services
import {
  ipfsUploadHandler,
  ipfsGetHandler,
  web3ChallengeHandler,
  web3VerifyHandler,
  web3AuditHandler,
} from './services/web3'
import {
  registerAgentHandler,
  agentHeartbeatHandler,
  agentMetricsHandler,
  agentCommandResultHandler,
  sendAgentCommandHandler,
  getAgentMetricsHandler,
  generateInstallScriptHandler,
} from './handlers/agents'
import {
  getMFAStatusHandler,
  setupMFAHandler,
  enableMFAHandler,
  disableMFAHandler,
  verifyMFAHandler,
  regenerateRecoveryCodesHandler,
  adminDisableMFAHandler,
} from './handlers/mfa'
import { batchOperationsHandler, bulkNodeStatusHandler } from './handlers/batch'
import { prometheusMetricsHandler, detailedHealthHandler, readinessHandler as k8sReadinessHandler, livenessHandler } from './handlers/metrics'
import { cacheMiddleware } from './middleware/cache'
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
} from './handlers/kubernetes'
import {
  listMeshServicesHandler,
  listVirtualServicesHandler,
  listDestinationRulesHandler,
  listGatewaysHandler,
  configureTrafficSplitHandler,
  configureCircuitBreakerHandler,
  injectFaultHandler,
  getMeshOverviewHandler,
} from './handlers/istio'
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
} from './handlers/elasticsearch'
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
} from './handlers/autoscaling'
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
} from './handlers/loadbalancer'

import {
  developerModeStatusHandler,
  developerDiagnosticsHandler,
  developerFixturesCatalogHandler,
  developerReadinessSummaryHandler,
} from './handlers/internal-dev'

// Middleware
import { authMiddleware, rbacMiddleware } from './middleware/auth'
import { rateLimitMiddleware } from './middleware/rate-limit'

// 创建应用
const app = createApp(new Hono<{ Bindings: Env }>())

// ==================== 受保护路由 ====================

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

// Incident workflows
app.get('/api/v1/incidents', authMiddleware, listIncidentsHandler)
app.post('/api/v1/incidents', authMiddleware, createIncidentHandler)
app.get('/api/v1/incidents/statistics', authMiddleware, getIncidentStatisticsHandler)
app.get('/api/v1/incidents/report', authMiddleware, rbacMiddleware(['admin', 'operator']), getIncidentReportHandler)
app.get('/api/v1/incidents/search', authMiddleware, searchIncidentsHandler)

// Bulk incident operations (must come before :id routes)
app.post('/api/v1/incidents/bulk/analyze', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkAnalyzeIncidentsHandler)
app.post('/api/v1/incidents/bulk/approve', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkApproveIncidentsHandler)
app.post('/api/v1/incidents/bulk/execute', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkExecuteIncidentsHandler)
app.post('/api/v1/incidents/bulk/delete', authMiddleware, rbacMiddleware(['admin']), bulkDeleteIncidentsHandler)
app.post('/api/v1/incidents/merge', authMiddleware, rbacMiddleware(['admin', 'operator']), mergeIncidentsHandler)

app.get('/api/v1/incidents/:id', authMiddleware, getIncidentHandler)
app.get('/api/v1/incidents/:id/timeline', authMiddleware, getIncidentTimelineHandler)
app.get('/api/v1/incidents/:id/sla', authMiddleware, getIncidentSlaStatusHandler)
app.get('/api/v1/incidents/:id/comments', authMiddleware, listIncidentCommentsHandler)
app.post('/api/v1/incidents/:id/comments', authMiddleware, addIncidentCommentHandler)
app.put('/api/v1/incidents/:id/comments/:commentId', authMiddleware, updateIncidentCommentHandler)
app.delete('/api/v1/incidents/:id/comments/:commentId', authMiddleware, deleteIncidentCommentHandler)
app.post('/api/v1/incidents/:id/acknowledge', authMiddleware, rbacMiddleware(['admin', 'operator']), acknowledgeIncidentHandler)
app.post('/api/v1/incidents/:id/escalate', authMiddleware, rbacMiddleware(['admin', 'operator']), escalateIncidentHandler)
app.post('/api/v1/incidents/:id/assign', authMiddleware, rbacMiddleware(['admin', 'operator']), assignIncidentHandler)
app.delete('/api/v1/incidents/:id/assign', authMiddleware, rbacMiddleware(['admin', 'operator']), unassignIncidentHandler)
app.post('/api/v1/incidents/:id/analyze', authMiddleware, rbacMiddleware(['admin', 'operator']), analyzeIncidentHandler)
app.post('/api/v1/incidents/:id/approve', authMiddleware, rbacMiddleware(['admin', 'operator']), approveIncidentHandler)
app.post('/api/v1/incidents/:id/execute', authMiddleware, rbacMiddleware(['admin', 'operator']), executeIncidentHandler)

// Incident links and evidence
app.post('/api/v1/incidents/:id/links', authMiddleware, rbacMiddleware(['admin', 'operator']), addIncidentLinkHandler)
app.delete('/api/v1/incidents/:id/links/:linkKind/:linkId', authMiddleware, rbacMiddleware(['admin', 'operator']), removeIncidentLinkHandler)
app.post('/api/v1/incidents/:id/evidence', authMiddleware, rbacMiddleware(['admin', 'operator']), addIncidentEvidenceHandler)
app.get('/api/v1/incidents/:id/activity', authMiddleware, getIncidentActivityLogHandler)
app.get('/api/v1/incidents/:id/runbooks', authMiddleware, rbacMiddleware(['admin', 'operator']), getRunbookSuggestionsHandler)
app.post('/api/v1/incidents/:id/runbooks/execute', authMiddleware, rbacMiddleware(['admin', 'operator']), executeRunbookHandler)

// Incident tags
app.get('/api/v1/incidents/tags', authMiddleware, listTagsHandler)
app.post('/api/v1/incidents/:id/tags', authMiddleware, addTagsHandler)
app.delete('/api/v1/incidents/:id/tags', authMiddleware, removeTagsHandler)
app.put('/api/v1/incidents/:id/tags', authMiddleware, setTagsHandler)

// Incident suppression rules
app.get('/api/v1/incidents/suppression-rules', authMiddleware, rbacMiddleware(['admin']), listSuppressionRulesHandler)
app.post('/api/v1/incidents/suppression-rules', authMiddleware, rbacMiddleware(['admin']), createSuppressionRuleHandler)
app.delete('/api/v1/incidents/suppression-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), deleteSuppressionRuleHandler)
app.patch('/api/v1/incidents/suppression-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), toggleSuppressionRuleHandler)

// Incident notification rules
app.get('/api/v1/incidents/notification-rules', authMiddleware, rbacMiddleware(['admin']), listNotificationRulesHandler)
app.post('/api/v1/incidents/notification-rules', authMiddleware, rbacMiddleware(['admin']), createNotificationRuleHandler)
app.delete('/api/v1/incidents/notification-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), deleteNotificationRuleHandler)
app.patch('/api/v1/incidents/notification-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), toggleNotificationRuleHandler)

// Incident templates
app.get('/api/v1/incidents/templates', authMiddleware, rbacMiddleware(['admin', 'operator']), listTemplatesHandler)
app.get('/api/v1/incidents/templates/:templateId', authMiddleware, rbacMiddleware(['admin', 'operator']), getTemplateHandler)
app.post('/api/v1/incidents/templates', authMiddleware, rbacMiddleware(['admin']), createTemplateHandler)
app.put('/api/v1/incidents/templates/:templateId', authMiddleware, rbacMiddleware(['admin']), updateTemplateHandler)
app.delete('/api/v1/incidents/templates/:templateId', authMiddleware, rbacMiddleware(['admin']), deleteTemplateHandler)
app.post('/api/v1/incidents/templates/:templateId/create', authMiddleware, rbacMiddleware(['admin', 'operator']), createFromTemplateHandler)

// Incident automation rules
app.get('/api/v1/incidents/automation-rules', authMiddleware, rbacMiddleware(['admin']), listAutomationRulesHandler)
app.post('/api/v1/incidents/automation-rules', authMiddleware, rbacMiddleware(['admin']), createAutomationRuleHandler)
app.delete('/api/v1/incidents/automation-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), deleteAutomationRuleHandler)
app.patch('/api/v1/incidents/automation-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), toggleAutomationRuleHandler)

// Incident post-mortems
app.get('/api/v1/incidents/:id/postmortem', authMiddleware, rbacMiddleware(['admin', 'operator']), getPostMortemHandler)
app.post('/api/v1/incidents/:id/postmortem', authMiddleware, rbacMiddleware(['admin', 'operator']), createPostMortemHandler)
app.put('/api/v1/incidents/:id/postmortem', authMiddleware, rbacMiddleware(['admin', 'operator']), updatePostMortemHandler)
app.patch('/api/v1/incidents/:id/postmortem/action-items/:actionItemId', authMiddleware, rbacMiddleware(['admin', 'operator']), updateActionItemHandler)

// Incident dashboard metrics
app.get('/api/v1/incidents/dashboard/metrics', authMiddleware, rbacMiddleware(['admin', 'operator']), getIncidentDashboardMetricsHandler)

// Incident correlation
app.get('/api/v1/incidents/:id/correlation', authMiddleware, rbacMiddleware(['admin', 'operator']), getIncidentCorrelationHandler)

// Incident watch
app.post('/api/v1/incidents/:id/watch', authMiddleware, watchIncidentHandler)
app.delete('/api/v1/incidents/:id/watch', authMiddleware, unwatchIncidentHandler)
app.get('/api/v1/incidents/:id/watchers', authMiddleware, getIncidentWatchersHandler)

// Incident external tickets
app.get('/api/v1/incidents/:id/tickets', authMiddleware, rbacMiddleware(['admin', 'operator']), listExternalTicketsHandler)
app.post('/api/v1/incidents/:id/tickets', authMiddleware, rbacMiddleware(['admin', 'operator']), createExternalTicketHandler)
app.patch('/api/v1/incidents/:id/tickets/:ticketId', authMiddleware, rbacMiddleware(['admin', 'operator']), updateExternalTicketHandler)

// Response playbooks
app.get('/api/v1/incidents/response-playbooks', authMiddleware, rbacMiddleware(['admin', 'operator']), listResponsePlaybooksHandler)
app.get('/api/v1/incidents/response-playbooks/:playbookId', authMiddleware, rbacMiddleware(['admin', 'operator']), getResponsePlaybookHandler)
app.post('/api/v1/incidents/response-playbooks', authMiddleware, rbacMiddleware(['admin']), createResponsePlaybookHandler)
app.put('/api/v1/incidents/response-playbooks/:playbookId', authMiddleware, rbacMiddleware(['admin']), updateResponsePlaybookHandler)
app.delete('/api/v1/incidents/response-playbooks/:playbookId', authMiddleware, rbacMiddleware(['admin']), deleteResponsePlaybookHandler)
app.get('/api/v1/incidents/:id/matching-playbooks', authMiddleware, rbacMiddleware(['admin', 'operator']), matchResponsePlaybooksHandler)
app.post('/api/v1/incidents/:id/execute-playbook', authMiddleware, rbacMiddleware(['admin', 'operator']), startPlaybookExecutionHandler)
app.get('/api/v1/incidents/playbook-executions/:executionId', authMiddleware, rbacMiddleware(['admin', 'operator']), getPlaybookExecutionHandler)
app.post('/api/v1/incidents/playbook-executions/:executionId/steps/:stepId/complete', authMiddleware, rbacMiddleware(['admin', 'operator']), completePlaybookStepHandler)
app.post('/api/v1/incidents/playbook-executions/:executionId/steps/:stepId/skip', authMiddleware, rbacMiddleware(['admin', 'operator']), skipPlaybookStepHandler)

// Custom fields
app.get('/api/v1/incidents/custom-fields', authMiddleware, rbacMiddleware(['admin']), listCustomFieldsHandler)
app.get('/api/v1/incidents/custom-fields/:fieldId', authMiddleware, rbacMiddleware(['admin']), getCustomFieldHandler)
app.post('/api/v1/incidents/custom-fields', authMiddleware, rbacMiddleware(['admin']), createCustomFieldHandler)
app.put('/api/v1/incidents/custom-fields/:fieldId', authMiddleware, rbacMiddleware(['admin']), updateCustomFieldHandler)
app.delete('/api/v1/incidents/custom-fields/:fieldId', authMiddleware, rbacMiddleware(['admin']), deleteCustomFieldHandler)
app.get('/api/v1/incidents/:id/custom-fields', authMiddleware, getIncidentCustomFieldsHandler)
app.put('/api/v1/incidents/:id/custom-fields/:fieldId', authMiddleware, rbacMiddleware(['admin', 'operator']), setIncidentCustomFieldHandler)

// AI Root Cause Analysis
app.post('/api/v1/incidents/:id/ai-analysis', authMiddleware, rbacMiddleware(['admin', 'operator']), generateAIRootCauseAnalysisHandler)

// War Room
app.post('/api/v1/incidents/:id/war-room', authMiddleware, rbacMiddleware(['admin', 'operator']), createWarRoomHandler)
app.get('/api/v1/incidents/:id/war-room', authMiddleware, getWarRoomHandler)
app.post('/api/v1/incidents/:id/war-room/join', authMiddleware, joinWarRoomHandler)
app.post('/api/v1/incidents/:id/war-room/leave', authMiddleware, leaveWarRoomHandler)
app.post('/api/v1/incidents/:id/war-room/messages', authMiddleware, addWarRoomMessageHandler)
app.post('/api/v1/incidents/:id/war-room/resources', authMiddleware, rbacMiddleware(['admin', 'operator']), addWarRoomResourceHandler)
app.post('/api/v1/incidents/:id/war-room/close', authMiddleware, rbacMiddleware(['admin', 'operator']), closeWarRoomHandler)

// Incident Export
app.post('/api/v1/incidents/export', authMiddleware, rbacMiddleware(['admin', 'operator']), exportIncidentsHandler)
app.get('/api/v1/incidents/export/:exportId', authMiddleware, rbacMiddleware(['admin', 'operator']), getExportStatusHandler)
app.get('/api/v1/incidents/export/:exportId/download', authMiddleware, rbacMiddleware(['admin', 'operator']), downloadExportHandler)

// Incident Reviews
app.get('/api/v1/incidents/:id/reviews', authMiddleware, listReviewsHandler)
app.post('/api/v1/incidents/:id/reviews', authMiddleware, rbacMiddleware(['admin', 'operator']), createReviewHandler)
app.post('/api/v1/incidents/:id/reviews/:reviewId/complete', authMiddleware, rbacMiddleware(['admin', 'operator']), completeReviewHandler)

// Response Analytics
app.get('/api/v1/incidents/analytics/response', authMiddleware, rbacMiddleware(['admin', 'operator']), getResponseAnalyticsHandler)

// Incident Feedback
app.get('/api/v1/incidents/:id/feedback', authMiddleware, rbacMiddleware(['admin', 'operator']), getFeedbackHandler)
app.post('/api/v1/incidents/:id/feedback', authMiddleware, submitFeedbackHandler)

// Incident Cost
app.get('/api/v1/incidents/:id/cost', authMiddleware, rbacMiddleware(['admin', 'operator']), getCostHandler)
app.post('/api/v1/incidents/:id/cost', authMiddleware, rbacMiddleware(['admin', 'operator']), calculateCostHandler)

// Incident Compliance
app.get('/api/v1/incidents/:id/compliance', authMiddleware, rbacMiddleware(['admin', 'operator']), getComplianceHandler)
app.post('/api/v1/incidents/:id/compliance', authMiddleware, rbacMiddleware(['admin']), createComplianceHandler)
app.patch('/api/v1/incidents/:id/compliance/:requirementId', authMiddleware, rbacMiddleware(['admin', 'operator']), updateComplianceHandler)

// On-Call Schedules
app.get('/api/v1/incidents/oncall/schedules', authMiddleware, rbacMiddleware(['admin', 'operator']), listOnCallSchedulesHandler)
app.get('/api/v1/incidents/oncall/schedules/:scheduleId', authMiddleware, rbacMiddleware(['admin', 'operator']), getOnCallScheduleHandler)
app.post('/api/v1/incidents/oncall/schedules', authMiddleware, rbacMiddleware(['admin']), createOnCallScheduleHandler)
app.get('/api/v1/incidents/oncall/schedules/:scheduleId/current', authMiddleware, rbacMiddleware(['admin', 'operator']), getCurrentOnCallHandler)

// Incident Checklists
app.get('/api/v1/incidents/:id/checklists', authMiddleware, listChecklistsHandler)
app.post('/api/v1/incidents/:id/checklists', authMiddleware, rbacMiddleware(['admin', 'operator']), createChecklistHandler)
app.patch('/api/v1/incidents/:id/checklists/:checklistId/items/:itemId', authMiddleware, rbacMiddleware(['admin', 'operator']), updateChecklistItemHandler)

// Incident Change Links
app.get('/api/v1/incidents/:id/changes', authMiddleware, listChangesHandler)
app.post('/api/v1/incidents/:id/changes', authMiddleware, rbacMiddleware(['admin', 'operator']), linkChangeHandler)

// Incident Run History
app.get('/api/v1/incidents/:id/runs', authMiddleware, rbacMiddleware(['admin', 'operator']), listRunHistoryHandler)

// Responder Teams
app.get('/api/v1/incidents/teams', authMiddleware, rbacMiddleware(['admin', 'operator']), listResponderTeamsHandler)
app.get('/api/v1/incidents/teams/:teamId', authMiddleware, rbacMiddleware(['admin', 'operator']), getResponderTeamHandler)
app.post('/api/v1/incidents/teams', authMiddleware, rbacMiddleware(['admin']), createResponderTeamHandler)
app.put('/api/v1/incidents/teams/:teamId', authMiddleware, rbacMiddleware(['admin']), updateResponderTeamHandler)
app.delete('/api/v1/incidents/teams/:teamId', authMiddleware, rbacMiddleware(['admin']), deleteResponderTeamHandler)

// SLA Calendars
app.get('/api/v1/incidents/sla-calendars', authMiddleware, rbacMiddleware(['admin', 'operator']), listSLACalendarsHandler)
app.get('/api/v1/incidents/sla-calendars/:calendarId', authMiddleware, rbacMiddleware(['admin', 'operator']), getSLACalendarHandler)
app.post('/api/v1/incidents/sla-calendars', authMiddleware, rbacMiddleware(['admin']), createSLACalendarHandler)

// Notification Templates
app.get('/api/v1/incidents/notification-templates', authMiddleware, rbacMiddleware(['admin', 'operator']), listNotificationTemplatesHandler)
app.get('/api/v1/incidents/notification-templates/:templateId', authMiddleware, rbacMiddleware(['admin', 'operator']), getNotificationTemplateHandler)
app.post('/api/v1/incidents/notification-templates', authMiddleware, rbacMiddleware(['admin']), createNotificationTemplateHandler)

// Escalation Rules
app.get('/api/v1/incidents/escalation-rules', authMiddleware, rbacMiddleware(['admin', 'operator']), listEscalationRulesHandler)
app.post('/api/v1/incidents/escalation-rules', authMiddleware, rbacMiddleware(['admin']), createEscalationRuleHandler)

// Incident Attachments
app.get('/api/v1/incidents/:id/attachments', authMiddleware, listAttachmentsHandler)
app.post('/api/v1/incidents/:id/attachments', authMiddleware, rbacMiddleware(['admin', 'operator']), uploadAttachmentHandler)
app.get('/api/v1/incidents/:id/attachments/:attachmentId', authMiddleware, downloadAttachmentHandler)
app.delete('/api/v1/incidents/:id/attachments/:attachmentId', authMiddleware, rbacMiddleware(['admin', 'operator']), deleteAttachmentHandler)

// Incident Related Items
app.get('/api/v1/incidents/:id/related-items', authMiddleware, listRelatedItemsHandler)
app.post('/api/v1/incidents/:id/related-items', authMiddleware, rbacMiddleware(['admin', 'operator']), addRelatedItemHandler)
app.delete('/api/v1/incidents/:id/related-items/:itemId', authMiddleware, rbacMiddleware(['admin', 'operator']), removeRelatedItemHandler)

// Response Time Targets
app.get('/api/v1/incidents/response-targets', authMiddleware, rbacMiddleware(['admin', 'operator']), listResponseTargetsHandler)
app.post('/api/v1/incidents/response-targets', authMiddleware, rbacMiddleware(['admin']), createResponseTargetHandler)

// Incident Integrations
app.get('/api/v1/incidents/integrations', authMiddleware, rbacMiddleware(['admin', 'operator']), listIntegrationsHandler)
app.get('/api/v1/incidents/integrations/:integrationId', authMiddleware, rbacMiddleware(['admin', 'operator']), getIntegrationHandler)
app.post('/api/v1/incidents/integrations', authMiddleware, rbacMiddleware(['admin']), createIntegrationHandler)
app.patch('/api/v1/incidents/integrations/:integrationId', authMiddleware, rbacMiddleware(['admin']), updateIntegrationHandler)
app.delete('/api/v1/incidents/integrations/:integrationId', authMiddleware, rbacMiddleware(['admin']), deleteIntegrationHandler)

// Incident Timeline Events
app.get('/api/v1/incidents/:id/timeline-events', authMiddleware, listTimelineEventsHandler)
app.post('/api/v1/incidents/:id/timeline-events', authMiddleware, rbacMiddleware(['admin', 'operator']), addTimelineEventHandler)

// Incident Runbooks (Management)
app.get('/api/v1/incidents/runbooks', authMiddleware, rbacMiddleware(['admin', 'operator']), listRunbooksHandler)
app.get('/api/v1/incidents/runbooks/:runbookId', authMiddleware, rbacMiddleware(['admin', 'operator']), getRunbookHandler)
app.post('/api/v1/incidents/runbooks', authMiddleware, rbacMiddleware(['admin']), createRunbookHandler)
app.put('/api/v1/incidents/runbooks/:runbookId', authMiddleware, rbacMiddleware(['admin']), updateRunbookHandler)
app.delete('/api/v1/incidents/runbooks/:runbookId', authMiddleware, rbacMiddleware(['admin']), deleteRunbookHandler)

// Auto-Remediation Rules
app.get('/api/v1/incidents/auto-remediation-rules', authMiddleware, rbacMiddleware(['admin', 'operator']), listAutoRemediationRulesHandler)
app.post('/api/v1/incidents/auto-remediation-rules', authMiddleware, rbacMiddleware(['admin']), createAutoRemediationRuleHandler)
app.patch('/api/v1/incidents/auto-remediation-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), updateAutoRemediationRuleHandler)
app.delete('/api/v1/incidents/auto-remediation-rules/:ruleId', authMiddleware, rbacMiddleware(['admin']), deleteAutoRemediationRuleHandler)

// Maintenance Windows
app.get('/api/v1/incidents/maintenance-windows', authMiddleware, rbacMiddleware(['admin', 'operator']), listMaintenanceWindowsHandler)
app.get('/api/v1/incidents/maintenance-windows/:windowId', authMiddleware, rbacMiddleware(['admin', 'operator']), getMaintenanceWindowHandler)
app.post('/api/v1/incidents/maintenance-windows', authMiddleware, rbacMiddleware(['admin']), createMaintenanceWindowHandler)
app.patch('/api/v1/incidents/maintenance-windows/:windowId', authMiddleware, rbacMiddleware(['admin']), updateMaintenanceWindowHandler)
app.post('/api/v1/incidents/maintenance-windows/:windowId/cancel', authMiddleware, rbacMiddleware(['admin']), cancelMaintenanceWindowHandler)

// Bulk Operations
app.get('/api/v1/incidents/bulk-operations', authMiddleware, rbacMiddleware(['admin', 'operator']), listBulkOperationsHandler)
app.get('/api/v1/incidents/bulk-operations/:operationId', authMiddleware, rbacMiddleware(['admin', 'operator']), getBulkOperationHandler)
app.post('/api/v1/incidents/bulk-operations', authMiddleware, rbacMiddleware(['admin', 'operator']), createBulkOperationHandler)
app.post('/api/v1/incidents/bulk-operations/:operationId/execute', authMiddleware, rbacMiddleware(['admin', 'operator']), executeBulkOperationHandler)

// SLA Breaches
app.get('/api/v1/incidents/sla-breaches', authMiddleware, rbacMiddleware(['admin', 'operator']), listSLABreachesHandler)
app.post('/api/v1/incidents/sla-breaches/:breachId/acknowledge', authMiddleware, rbacMiddleware(['admin', 'operator']), acknowledgeSLABreachHandler)

// Analytics Snapshots
app.get('/api/v1/incidents/analytics', authMiddleware, rbacMiddleware(['admin', 'operator']), listAnalyticsSnapshotsHandler)
app.post('/api/v1/incidents/analytics/generate', authMiddleware, rbacMiddleware(['admin']), generateAnalyticsSnapshotHandler)

// Webhook Subscriptions (Incident-specific)
app.get('/api/v1/incidents/webhooks', authMiddleware, rbacMiddleware(['admin', 'operator']), listWebhookSubscriptionsHandler)
app.get('/api/v1/incidents/webhooks/:subscriptionId', authMiddleware, rbacMiddleware(['admin', 'operator']), getWebhookSubscriptionHandler)
app.post('/api/v1/incidents/webhooks', authMiddleware, rbacMiddleware(['admin']), createWebhookSubscriptionHandler)
app.patch('/api/v1/incidents/webhooks/:subscriptionId', authMiddleware, rbacMiddleware(['admin']), updateWebhookSubscriptionHandler)
app.delete('/api/v1/incidents/webhooks/:subscriptionId', authMiddleware, rbacMiddleware(['admin']), deleteWebhookSubscriptionHandler)

// Incident Snooze
app.get('/api/v1/incidents/snoozes', authMiddleware, rbacMiddleware(['admin', 'operator']), listSnoozesHandler)
app.post('/api/v1/incidents/:id/snooze', authMiddleware, rbacMiddleware(['admin', 'operator']), createSnoozeHandler)
app.post('/api/v1/incidents/snoozes/:snoozeId/wake', authMiddleware, rbacMiddleware(['admin', 'operator']), wakeSnoozeHandler)

// Incident Merge (additional route)
app.get('/api/v1/incidents/merges', authMiddleware, rbacMiddleware(['admin', 'operator']), listMergesHandler)
app.post('/api/v1/incidents/:id/merge', authMiddleware, rbacMiddleware(['admin', 'operator']), createMergeHandler)

// Incident Split
app.get('/api/v1/incidents/splits', authMiddleware, rbacMiddleware(['admin', 'operator']), listSplitsHandler)
app.post('/api/v1/incidents/:id/split', authMiddleware, rbacMiddleware(['admin', 'operator']), createSplitHandler)

// Incident Recurrence
app.get('/api/v1/incidents/recurrences', authMiddleware, rbacMiddleware(['admin', 'operator']), listRecurrencesHandler)
app.post('/api/v1/incidents/:id/detect-recurrence', authMiddleware, rbacMiddleware(['admin', 'operator']), detectRecurrenceHandler)
app.post('/api/v1/incidents/recurrences/:recurrenceId/resolve', authMiddleware, rbacMiddleware(['admin', 'operator']), markRootCauseResolvedHandler)

// Governance policies
app.get('/api/v1/governance/policies', authMiddleware, rbacMiddleware(['admin']), listPoliciesHandler)
app.get('/api/v1/governance/policies/active', authMiddleware, getActivePolicyHandler)
app.get('/api/v1/governance/policies/:id', authMiddleware, rbacMiddleware(['admin']), getPolicyHandler)
app.post('/api/v1/governance/policies', authMiddleware, rbacMiddleware(['admin']), createPolicyHandler)
app.put('/api/v1/governance/policies/:id', authMiddleware, rbacMiddleware(['admin']), updatePolicyHandler)
app.delete('/api/v1/governance/policies/:id', authMiddleware, rbacMiddleware(['admin']), deletePolicyHandler)

// Webhooks
app.get('/api/v1/webhooks', authMiddleware, rbacMiddleware(['admin']), listWebhooksHandler)
app.post('/api/v1/webhooks', authMiddleware, rbacMiddleware(['admin']), createWebhookHandler)
app.get('/api/v1/webhooks/failed-deliveries', authMiddleware, rbacMiddleware(['admin']), listFailedDeliveriesHandler)
app.get('/api/v1/webhooks/:id', authMiddleware, rbacMiddleware(['admin']), getWebhookHandler)
app.put('/api/v1/webhooks/:id', authMiddleware, rbacMiddleware(['admin']), updateWebhookHandler)
app.delete('/api/v1/webhooks/:id', authMiddleware, rbacMiddleware(['admin']), deleteWebhookHandler)
app.get('/api/v1/webhooks/:id/deliveries', authMiddleware, rbacMiddleware(['admin']), listWebhookDeliveriesHandler)
app.post('/api/v1/webhooks/deliveries/:deliveryId/retry', authMiddleware, rbacMiddleware(['admin']), retryDeliveryHandler)

// ==================== SSE (实时通信) ====================
app.get('/api/v1/sse', authMiddleware, sseHandler)
app.post('/api/v1/sse/subscribe', authMiddleware, sseSubscribeHandler)
app.post('/api/v1/sse/unsubscribe', authMiddleware, sseUnsubscribeHandler)
app.get('/api/v1/sse/status', authMiddleware, sseStatusHandler)

// ==================== 备份管理 ====================
app.get('/api/v1/backups', authMiddleware, rbacMiddleware(['admin']), listBackupsHandler)
app.get('/api/v1/backups/status', authMiddleware, rbacMiddleware(['admin']), backupStatusHandler)
app.post('/api/v1/backups', authMiddleware, rbacMiddleware(['admin']), createBackupHandler)
app.get('/api/v1/backups/:id', authMiddleware, rbacMiddleware(['admin']), getBackupHandler)
app.get('/api/v1/backups/:id/download', authMiddleware, rbacMiddleware(['admin']), downloadBackupHandler)
app.post('/api/v1/backups/:id/restore', authMiddleware, rbacMiddleware(['admin']), restoreBackupHandler)
app.delete('/api/v1/backups/:id', authMiddleware, rbacMiddleware(['admin']), deleteBackupHandler)
app.post('/api/v1/backups/cleanup', authMiddleware, rbacMiddleware(['admin']), cleanupBackupsHandler)

// ==================== Agent 管理 ====================
// Agent API (认证通过Header)
app.post('/api/v1/agents/register', registerAgentHandler)
app.post('/api/v1/agents/heartbeat', agentHeartbeatHandler)
app.post('/api/v1/agents/metrics', agentMetricsHandler)
app.post('/api/v1/agents/command-result', agentCommandResultHandler)

// Agent 管理API (需要用户认证)
app.get('/api/v1/agents/:agentId/metrics', authMiddleware, getAgentMetricsHandler)
app.post('/api/v1/agents/:agentId/command', authMiddleware, rbacMiddleware(['admin', 'operator']), sendAgentCommandHandler)
app.get('/api/v1/nodes/:nodeId/install-script', authMiddleware, rbacMiddleware(['admin', 'operator']), generateInstallScriptHandler)

// ==================== MFA 双因素认证 ====================
app.get('/api/v1/mfa/status', authMiddleware, getMFAStatusHandler)
app.post('/api/v1/mfa/setup', authMiddleware, setupMFAHandler)
app.post('/api/v1/mfa/enable', authMiddleware, enableMFAHandler)
app.post('/api/v1/mfa/disable', authMiddleware, disableMFAHandler)
app.post('/api/v1/mfa/verify', authMiddleware, verifyMFAHandler)
app.post('/api/v1/mfa/recovery-codes', authMiddleware, regenerateRecoveryCodesHandler)
app.post('/api/v1/admin/users/:id/mfa/disable', authMiddleware, rbacMiddleware(['admin']), adminDisableMFAHandler)

// ==================== 批量操作 ====================
app.post('/api/v1/batch', authMiddleware, rbacMiddleware(['admin', 'operator']), batchOperationsHandler)

// ==================== Kubernetes 集成 ====================
app.get('/api/v1/kubernetes/overview', authMiddleware, rbacMiddleware(['admin', 'operator']), getClusterOverviewHandler)
app.get('/api/v1/kubernetes/namespaces', authMiddleware, rbacMiddleware(['admin', 'operator']), listNamespacesHandler)
app.get('/api/v1/kubernetes/namespaces/:namespace', authMiddleware, rbacMiddleware(['admin', 'operator']), getNamespaceDetailsHandler)
app.get('/api/v1/kubernetes/nodes', authMiddleware, rbacMiddleware(['admin', 'operator']), listClusterNodesHandler)
app.get('/api/v1/kubernetes/pods', authMiddleware, rbacMiddleware(['admin', 'operator']), listPodsHandler)
app.get('/api/v1/kubernetes/deployments', authMiddleware, rbacMiddleware(['admin', 'operator']), listDeploymentsHandler)
app.get('/api/v1/kubernetes/services', authMiddleware, rbacMiddleware(['admin', 'operator']), listServicesHandler)
app.get('/api/v1/kubernetes/events', authMiddleware, rbacMiddleware(['admin', 'operator']), listEventsHandler)
app.get('/api/v1/kubernetes/namespaces/:namespace/pods/:pod/logs', authMiddleware, rbacMiddleware(['admin', 'operator']), getPodLogsHandler)
app.post('/api/v1/kubernetes/namespaces/:namespace/deployments/:name/scale', authMiddleware, rbacMiddleware(['admin']), scaleDeploymentHandler)
app.post('/api/v1/kubernetes/namespaces/:namespace/deployments/:name/restart', authMiddleware, rbacMiddleware(['admin', 'operator']), restartDeploymentHandler)

// ==================== Istio 服务网格 ====================
app.get('/api/v1/mesh/overview', authMiddleware, rbacMiddleware(['admin', 'operator']), getMeshOverviewHandler)
app.get('/api/v1/mesh/services', authMiddleware, rbacMiddleware(['admin', 'operator']), listMeshServicesHandler)
app.get('/api/v1/mesh/virtualservices', authMiddleware, rbacMiddleware(['admin', 'operator']), listVirtualServicesHandler)
app.get('/api/v1/mesh/destinationrules', authMiddleware, rbacMiddleware(['admin', 'operator']), listDestinationRulesHandler)
app.get('/api/v1/mesh/gateways', authMiddleware, rbacMiddleware(['admin', 'operator']), listGatewaysHandler)
app.post('/api/v1/mesh/traffic/split', authMiddleware, rbacMiddleware(['admin']), configureTrafficSplitHandler)
app.post('/api/v1/mesh/circuit-breaker', authMiddleware, rbacMiddleware(['admin']), configureCircuitBreakerHandler)
app.post('/api/v1/mesh/fault/inject', authMiddleware, rbacMiddleware(['admin']), injectFaultHandler)

// ==================== Elasticsearch/ELK 日志 ====================
app.get('/api/v1/logs', authMiddleware, rbacMiddleware(['admin', 'operator']), searchLogsHandler)
app.get('/api/v1/logs/stats', authMiddleware, rbacMiddleware(['admin', 'operator']), getLogStatsHandler)
app.get('/api/v1/logs/export', authMiddleware, rbacMiddleware(['admin']), exportLogsHandler)
app.post('/api/v1/logs', authMiddleware, indexLogHandler)
app.post('/api/v1/logs/bulk', authMiddleware, rbacMiddleware(['admin', 'operator']), bulkIndexLogsHandler)
app.get('/api/v1/logs/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), getLogHandler)
app.post('/api/v1/logs/index', authMiddleware, rbacMiddleware(['admin']), createLogIndexHandler)
app.delete('/api/v1/logs/old', authMiddleware, rbacMiddleware(['admin']), deleteOldLogsHandler)
app.get('/api/v1/logs/trace/:traceId', authMiddleware, rbacMiddleware(['admin', 'operator']), getTraceLogsHandler)
app.get('/api/v1/logs/node/:nodeId', authMiddleware, rbacMiddleware(['admin', 'operator']), getNodeLogsV2Handler)
app.get('/api/v1/logs/service/:service', authMiddleware, rbacMiddleware(['admin', 'operator']), getServiceLogsHandler)

// ==================== Auto-Scaling ====================
app.get('/api/v1/scaling/policies', authMiddleware, rbacMiddleware(['admin', 'operator']), listScalingPoliciesHandler)
app.post('/api/v1/scaling/policies', authMiddleware, rbacMiddleware(['admin']), createScalingPolicyHandler)
app.get('/api/v1/scaling/policies/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), getScalingPolicyHandler)
app.put('/api/v1/scaling/policies/:id', authMiddleware, rbacMiddleware(['admin']), updateScalingPolicyHandler)
app.delete('/api/v1/scaling/policies/:id', authMiddleware, rbacMiddleware(['admin']), deleteScalingPolicyHandler)
app.post('/api/v1/scaling/policies/:id/toggle', authMiddleware, rbacMiddleware(['admin']), toggleScalingPolicyHandler)
app.get('/api/v1/scaling/policies/:id/evaluate', authMiddleware, rbacMiddleware(['admin', 'operator']), evaluateScalingPolicyHandler)
app.post('/api/v1/scaling/policies/:id/execute', authMiddleware, rbacMiddleware(['admin']), executeScalingActionHandler)
app.get('/api/v1/scaling/policies/:id/history', authMiddleware, rbacMiddleware(['admin', 'operator']), getScalingHistoryHandler)
app.get('/api/v1/scaling/policies/:id/recommend', authMiddleware, rbacMiddleware(['admin', 'operator']), getRecommendedReplicasHandler)
app.get('/api/v1/scaling/policies/:id/metrics', authMiddleware, rbacMiddleware(['admin', 'operator']), getScalingMetricsHandler)
app.get('/api/v1/scaling/health/:type/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), checkHealthHandler)
app.post('/api/v1/scaling/check', authMiddleware, rbacMiddleware(['admin']), runScalingCheckHandler)

// ==================== Load Balancing ====================
app.get('/api/v1/lb', authMiddleware, rbacMiddleware(['admin', 'operator']), listLoadBalancersHandler)
app.post('/api/v1/lb', authMiddleware, rbacMiddleware(['admin']), createLoadBalancerHandler)
app.get('/api/v1/lb/:id', authMiddleware, rbacMiddleware(['admin', 'operator']), getLoadBalancerHandler)
app.put('/api/v1/lb/:id', authMiddleware, rbacMiddleware(['admin']), updateLoadBalancerHandler)
app.delete('/api/v1/lb/:id', authMiddleware, rbacMiddleware(['admin']), deleteLoadBalancerHandler)
app.post('/api/v1/lb/:id/toggle', authMiddleware, rbacMiddleware(['admin']), toggleLoadBalancerHandler)
app.get('/api/v1/lb/:id/select', authMiddleware, selectTargetHandler)
app.get('/api/v1/lb/:id/stats', authMiddleware, rbacMiddleware(['admin', 'operator']), getLoadBalancerStatsHandler)
app.post('/api/v1/lb/:id/health-check', authMiddleware, rbacMiddleware(['admin', 'operator']), runHealthChecksHandler)
app.get('/api/v1/lb/:id/targets/:targetId/health', authMiddleware, rbacMiddleware(['admin', 'operator']), checkTargetHealthHandler)
app.post('/api/v1/lb/:id/targets', authMiddleware, rbacMiddleware(['admin']), addTargetHandler)
app.delete('/api/v1/lb/:id/targets/:targetId', authMiddleware, rbacMiddleware(['admin']), removeTargetHandler)
app.put('/api/v1/lb/:id/targets/:targetId/weight', authMiddleware, rbacMiddleware(['admin']), updateTargetWeightHandler)
app.post('/api/v1/lb/:id/targets/:targetId/complete', authMiddleware, recordCompletionHandler)

// ==================== WebSocket ====================
// WebSocket 实时通信端点
app.get('/api/v1/ws', authMiddleware, websocketHandler)

// ==================== AI Services ====================
// AI 聊天助手
app.post('/api/v1/ai/chat', authMiddleware, aiChatHandler)
// AI 日志分析
app.post('/api/v1/ai/analyze-log', authMiddleware, aiAnalyzeLogHandler)
// AI 运维建议
app.post('/api/v1/ai/ops-advice', authMiddleware, aiOpsAdviceHandler)
// AI 文本嵌入
app.post('/api/v1/ai/embedding', authMiddleware, aiEmbeddingHandler)
// AI 查询转换
app.post('/api/v1/ai/query', authMiddleware, aiQueryHandler)

// ==================== Vectorize ====================
// 向量搜索
app.post('/api/v1/vectors/search', authMiddleware, vectorSearchHandler)
// 向量插入
app.post('/api/v1/vectors', authMiddleware, vectorInsertHandler)
// 向量删除
app.delete('/api/v1/vectors/:id', authMiddleware, vectorDeleteHandler)

// ==================== Web3 / IPFS ====================
// IPFS 上传
app.post('/api/v1/ipfs/upload', authMiddleware, ipfsUploadHandler)
// IPFS 获取
app.get('/api/v1/ipfs/:cid', ipfsGetHandler)

// ==================== Web3 / Ethereum ====================
// Web3 登录挑战
app.post('/api/v1/web3/challenge', web3ChallengeHandler)
// Web3 登录验证
app.post('/api/v1/web3/verify', web3VerifyHandler)
// 链上审计
app.post('/api/v1/web3/audit', authMiddleware, web3AuditHandler)

// ==================== 内部开发者模式 ====================
app.get('/api/v1/internal/dev/status', authMiddleware, rbacMiddleware(['admin']), developerModeStatusHandler)
app.get('/api/v1/internal/dev/diagnostics', authMiddleware, rbacMiddleware(['admin']), developerDiagnosticsHandler)
app.get('/api/v1/internal/dev/fixtures', authMiddleware, rbacMiddleware(['admin']), developerFixturesCatalogHandler)
app.get('/api/v1/internal/dev/readiness-summary', authMiddleware, rbacMiddleware(['admin']), developerReadinessSummaryHandler)

// ==================== 错误处理 ====================

app.notFound((c) => {
  return c.json({ success: false, error: 'Not Found' } as ApiErrorResponse, 404)
})

app.onError((err, c) => {
  console.error('Error:', err)

  // 开发环境返回详细错误
  if (c.env.ENVIRONMENT === 'development') {
    return c.json({
      success: false,
      error: err.message,
      stack: err.stack,
    }, 500)
  }

  return c.json({ success: false, error: 'Internal Server Error' } as ApiErrorResponse, 500)
})

// Export
export default app