import type { Hono } from 'hono'
import type { Env } from '../types'

// Handlers
import { sseHandler, sseSubscribeHandler, sseUnsubscribeHandler, sseStatusHandler } from '../handlers/sse'
import { websocketHandler } from '../handlers/websocket'
import { createBackupHandler, listBackupsHandler, getBackupHandler, deleteBackupHandler, downloadBackupHandler, restoreBackupHandler, cleanupBackupsHandler, backupStatusHandler } from '../handlers/backup'
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
import { batchOperationsHandler } from '../handlers/batch'
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

// Middleware
import { authMiddleware, rbacMiddleware } from '../middleware/auth'

export function registerProtectedSystemRoutes(app: Hono<{ Bindings: Env }>) {
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

  // ==================== WebSocket ====================
  // WebSocket 实时通信端点
  app.get('/api/v1/ws', authMiddleware, websocketHandler)

}
