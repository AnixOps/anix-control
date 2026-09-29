/**
 * Enhanced Audit Log Handlers
 *
 * Features:
 * - List with filtering
 * - Export (JSON/CSV)
 * - Statistics
 * - SIEM configuration
 */

import type { Context } from 'hono'
import type {
  ApiErrorResponse,
  ApiMessageResponse,
  AuditActionsResponse,
  AuditCleanupResponse,
  AuditLog,
  AuditLogDetailResponse,
  AuditLogListResponse,
  AuditStatsResponse,
  Env,
  SIEMConfigResponse,
} from '../types'
import { z } from 'zod'
import {
  getAuditLogs,
  exportAuditLogsJSON,
  exportAuditLogsCSV,
  getAuditStats,
  cleanupAuditLogs,
  configureSIEM,
  getSIEMConfig,
  getRequiredParam,
  type SIEMConfig,
} from '../utils/audit'
import { logAudit } from '../utils/audit'

// Validation schemas
const siemConfigSchema = z.object({
  enabled: z.boolean(),
  webhook_url: z.string().url(),
  api_key: z.string().optional(),
  format: z.enum(['json', 'cef', 'syslog']),
  filters: z.array(z.string()).optional(),
})

/**
 * List audit logs with filtering
 */
export async function listAuditLogsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const page = parseInt(c.req.query('page') || '1', 10)
  const perPage = Math.min(parseInt(c.req.query('per_page') || '50', 10), 200)
  const userId = c.req.query('user_id') ? parseInt(c.req.query('user_id')!, 10) : undefined
  const action = c.req.query('action')
  const resource = c.req.query('resource')
  const startDate = c.req.query('start_date')
  const endDate = c.req.query('end_date')
  const tenantId = c.req.query('tenant_id') ? parseInt(c.req.query('tenant_id')!, 10) : undefined

  const { logs, total } = await getAuditLogs(c.env, {
    tenantId,
    userId,
    action,
    resource,
    startDate,
    endDate,
    page,
    perPage,
  })

  return c.json({
    success: true,
    data: {
      items: logs,
      total,
      page,
      per_page: perPage,
      total_pages: Math.ceil(total / perPage),
    },
  } as AuditLogListResponse)
}

/**
 * Get audit log details
 */
export async function getAuditLogHandler(c: Context<{ Bindings: Env }>) {
  const logId = parseInt(getRequiredParam(c, 'id'), 10)

  if (isNaN(logId)) {
    return c.json({ success: false, error: 'Invalid log ID' } as ApiErrorResponse, 400)
  }

  const log = await c.env.DB
    .prepare(`
      SELECT al.*, u.email as user_email
      FROM audit_logs al
      LEFT JOIN users u ON al.user_id = u.id
      WHERE al.id = ?
    `)
    .bind(logId)
    .first()

  if (!log) {
    return c.json({ success: false, error: 'Audit log not found' } as ApiErrorResponse, 404)
  }

  return c.json({
    success: true,
    data: log as unknown as AuditLogDetailResponse['data'],
  } as AuditLogDetailResponse)
}

/**
 * Export audit logs as JSON
 */
export async function exportAuditLogsJSONHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const startDate = c.req.query('start_date')
  const endDate = c.req.query('end_date')
  const tenantId = c.req.query('tenant_id') ? parseInt(c.req.query('tenant_id')!, 10) : undefined
  const userId = c.req.query('user_id') ? parseInt(c.req.query('user_id')!, 10) : undefined
  const action = c.req.query('action')

  const json = await exportAuditLogsJSON(c.env, {
    tenantId,
    userId,
    action,
    startDate,
    endDate,
  })

  await logAudit(c, user.sub, 'export_audit_logs', 'audit', {
    format: 'json',
    filters: { tenantId, userId, action, startDate, endDate },
  })

  return new Response(json, {
    headers: {
      'Content-Type': 'application/json',
      'Content-Disposition': `attachment; filename="audit-logs-${new Date().toISOString().split('T')[0]}.json"`,
    },
  })
}

/**
 * Export audit logs as CSV
 */
export async function exportAuditLogsCSVHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const startDate = c.req.query('start_date')
  const endDate = c.req.query('end_date')
  const tenantId = c.req.query('tenant_id') ? parseInt(c.req.query('tenant_id')!, 10) : undefined
  const userId = c.req.query('user_id') ? parseInt(c.req.query('user_id')!, 10) : undefined
  const action = c.req.query('action')

  const csv = await exportAuditLogsCSV(c.env, {
    tenantId,
    userId,
    action,
    startDate,
    endDate,
  })

  await logAudit(c, user.sub, 'export_audit_logs', 'audit', {
    format: 'csv',
    filters: { tenantId, userId, action, startDate, endDate },
  })

  return new Response(csv, {
    headers: {
      'Content-Type': 'text/csv',
      'Content-Disposition': `attachment; filename="audit-logs-${new Date().toISOString().split('T')[0]}.csv"`,
    },
  })
}

/**
 * Get audit statistics
 */
export async function getAuditStatsHandler(c: Context<{ Bindings: Env }>) {
  const days = parseInt(c.req.query('days') || '30', 10)
  const tenantId = c.req.query('tenant_id') ? parseInt(c.req.query('tenant_id')!, 10) : undefined

  const stats = await getAuditStats(c.env, tenantId, days)

  return c.json({
    success: true,
    data: stats,
  } as AuditStatsResponse)
}

/**
 * Clean up old audit logs
 */
export async function cleanupAuditLogsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const retentionDays = parseInt(c.req.query('retention_days') || '90', 10)

  const result = await cleanupAuditLogs(c.env, retentionDays)

  await logAudit(c, user.sub, 'cleanup_audit_logs', 'audit', {
    retention_days: retentionDays,
    deleted: result.deleted,
  })

  return c.json({
    success: true,
    data: result,
  } as AuditCleanupResponse)
}

/**
 * Get SIEM configuration
 */
export async function getSIEMConfigHandler(c: Context<{ Bindings: Env }>) {
  const config = await getSIEMConfig(c.env)

  // Don't expose API key in response
  if (config) {
    config.api_key = config.api_key ? '***hidden***' : undefined
  }

  return c.json({
    success: true,
    data: config,
  } as SIEMConfigResponse)
}

/**
 * Update SIEM configuration
 */
export async function updateSIEMConfigHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = siemConfigSchema.parse(body)

    await configureSIEM(c.env, data)

    await logAudit(c, user.sub, 'update_siem_config', 'settings', {
      enabled: data.enabled,
      format: data.format,
      webhook_configured: !!data.webhook_url,
    })

    return c.json({
      success: true,
      message: 'SIEM configuration updated',
    } as ApiMessageResponse)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Test SIEM connection
 */
export async function testSIEMConnectionHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const config = await getSIEMConfig(c.env)

  if (!config || !config.enabled) {
    return c.json({
      success: false,
      error: 'SIEM not configured or disabled',
    }, 400)
  }

  try {
    const testEntry = {
      id: 0,
      action: 'siem_test',
      resource: 'audit',
      ip: '127.0.0.1',
      user_agent: 'AnixOps SIEM Test',
      status: 'success' as const,
      details: JSON.stringify({ test: true, timestamp: new Date().toISOString() }),
      created_at: new Date().toISOString(),
    }

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }

    if (config.api_key) {
      headers['Authorization'] = `Bearer ${config.api_key}`
    }

    const response = await fetch(config.webhook_url, {
      method: 'POST',
      headers,
      body: JSON.stringify(testEntry),
    })

    if (!response.ok) {
      return c.json({
        success: false,
        error: `SIEM returned status ${response.status}`,
      }, 400)
    }

    await logAudit(c, user.sub, 'test_siem_connection', 'settings', {
      success: true,
    })

    return c.json({
      success: true,
      message: 'SIEM connection test successful',
    } as ApiMessageResponse)
  } catch (err) {
    return c.json({
      success: false,
      error: `Failed to connect to SIEM: ${err instanceof Error ? err.message : 'Unknown error'}`,
    }, 400)
  }
}

/**
 * Get available audit actions
 */
export async function getAuditActionsHandler(c: Context<{ Bindings: Env }>) {
  const actions = [
    // Authentication
    { category: 'auth', actions: ['login', 'logout', 'register', 'token_refresh', 'password_change', 'mfa_enable', 'mfa_disable', 'setup_mfa'] },
    // Users
    { category: 'user', actions: ['create_user', 'update_user', 'delete_user', 'change_role', 'unlock_user'] },
    // Nodes
    { category: 'node', actions: ['create_node', 'update_node', 'delete_node', 'start_node', 'stop_node', 'restart_node', 'sync_node', 'test_connection'] },
    // Playbooks
    { category: 'playbook', actions: ['create_playbook', 'update_playbook', 'delete_playbook', 'execute_playbook', 'sync_built_in_playbooks'] },
    // Tasks
    { category: 'task', actions: ['create_task', 'cancel_task', 'retry_task'] },
    // Schedules
    { category: 'schedule', actions: ['create_schedule', 'update_schedule', 'delete_schedule', 'toggle_schedule', 'run_schedule'] },
    // Tenant
    { category: 'tenant', actions: ['create_tenant', 'update_tenant', 'delete_tenant', 'add_member', 'remove_member', 'create_invitation', 'accept_invitation'] },
    // Settings
    { category: 'settings', actions: ['update_settings', 'create_role', 'delete_role', 'update_siem_config'] },
    // Backup
    { category: 'backup', actions: ['create_backup', 'restore_backup', 'delete_backup', 'cleanup_backups'] },
    // Audit
    { category: 'audit', actions: ['export_audit_logs', 'cleanup_audit_logs'] },
  ]

  return c.json({
    success: true,
    data: actions,
  } as AuditActionsResponse)
}