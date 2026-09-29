/**
 * Enhanced Audit Logging Utilities
 *
 * Features:
 * - Structured audit entries
 * - Tenant context
 * - CSV/JSON export
 * - SIEM webhook integration
 * - Retention management
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { buildChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

// Audit log entry
export interface AuditEntry {
  id: number
  tenant_id?: number
  user_id?: number
  user_email?: string
  action: string
  resource: string
  resource_id?: string
  ip?: string
  user_agent?: string
  status: 'success' | 'failure' | 'pending'
  details?: string
  created_at: string
}

// Audit action categories
export const AUDIT_CATEGORIES = {
  AUTH: ['login', 'logout', 'register', 'token_refresh', 'password_change', 'mfa_enable', 'mfa_disable'],
  USER: ['create_user', 'update_user', 'delete_user', 'change_role'],
  NODE: ['create_node', 'update_node', 'delete_node', 'start_node', 'stop_node', 'restart_node'],
  PLAYBOOK: ['create_playbook', 'update_playbook', 'delete_playbook', 'execute_playbook'],
  TASK: ['create_task', 'cancel_task', 'retry_task'],
  TENANT: ['create_tenant', 'update_tenant', 'delete_tenant', 'add_member', 'remove_member'],
  SETTINGS: ['update_settings', 'create_role', 'delete_role'],
  BACKUP: ['create_backup', 'restore_backup', 'delete_backup'],
} as const

// SIEM configuration
export interface SIEMConfig {
  enabled: boolean
  webhook_url: string
  api_key?: string
  format: 'json' | 'cef' | 'syslog'
  filters?: string[] // Action filters
}

/**
 * Get required route parameter, throws if not present
 */
export function getRequiredParam(c: Context<{ Bindings: Env }>, name: string): string {
  const value = c.req.param(name) as string
  if (!value) {
    throw new Error(`Missing required parameter: ${name}`)
  }
  return value
}

/**
 * Get client IP address
 */
export function getClientIP(c: Context<{ Bindings: Env }>): string | null {
  return c.req.header('CF-Connecting-IP') ||
         c.req.header('X-Forwarded-For')?.split(',')[0].trim() ||
         c.req.header('X-Real-IP') ||
         null
}

/**
 * Get user agent
 */
export function getUserAgent(c: Context<{ Bindings: Env }>): string | null {
  return c.req.header('User-Agent') || null
}

/**
 * Get request ID for tracing
 */
export function getRequestId(c: Context<{ Bindings: Env }>): string {
  return c.req.header('X-Request-ID') || crypto.randomUUID()
}

/**
 * Get tenant ID from context
 */
function getTenantId(c: Context<{ Bindings: Env }>): number | null {
  // Try to get from context set by tenant middleware
  const tenant = (c as any).get?.('tenant')
  return tenant?.id || null
}

/**
 * Log audit entry with enhanced fields
 */
export async function logAudit(
  c: Context<{ Bindings: Env }>,
  userId: number | undefined,
  action: string,
  resource: string,
  details?: Record<string, unknown>,
  status: 'success' | 'failure' | 'pending' = 'success'
): Promise<number> {
  const tenantId = getTenantId(c)
  const ip = getClientIP(c)
  const userAgent = getUserAgent(c)
  const requestId = getRequestId(c)

  const enrichedDetails = {
    ...details,
    request_id: requestId,
    path: c.req.path,
    method: c.req.method,
  }

  try {
    const result = await c.env.DB
      .prepare(`
        INSERT INTO audit_logs (tenant_id, user_id, action, resource, ip, user_agent, details)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        RETURNING id
      `)
      .bind(
        tenantId,
        userId ?? null,
        action,
        resource,
        ip,
        userAgent,
        JSON.stringify(enrichedDetails)
      )
      .first<{ id: number }>()

    // Send to SIEM if configured
    await sendToSIEM(c.env, {
      id: result?.id || 0,
      tenant_id: tenantId || undefined,
      user_id: userId,
      action,
      resource,
      ip: ip || undefined,
      user_agent: userAgent || undefined,
      status,
      details: JSON.stringify(enrichedDetails),
      created_at: new Date().toISOString(),
    })

    publishRealtimeEvent(makeRealtimeEvent(
      `audit.${action}`,
      'audit',
      buildChannels(
        'global',
        'audit',
        'operations',
        tenantId ? `tenant:${tenantId}` : undefined,
        userId ? `user:${userId}` : undefined,
      ),
      {
        audit_id: result?.id || 0,
        action,
        resource,
        status,
        details: enrichedDetails,
      },
      {
        user_id: userId,
        tenant_id: tenantId || undefined,
      }
    ))

    return result?.id || 0
  } catch (err) {
    console.error('Failed to log audit:', err)
    return 0
  }
}

/**
 * Log audit with tenant context
 */
export async function logAuditWithTenant(
  env: Env,
  tenantId: number,
  userId: number | undefined,
  action: string,
  resource: string,
  details?: Record<string, unknown>,
  ip?: string,
  userAgent?: string
): Promise<number> {
  try {
    const result = await env.DB
      .prepare(`
        INSERT INTO audit_logs (tenant_id, user_id, action, resource, ip, user_agent, details)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        RETURNING id
      `)
      .bind(
        tenantId,
        userId ?? null,
        action,
        resource,
        ip || null,
        userAgent || null,
        details ? JSON.stringify(details) : null
      )
      .first<{ id: number }>()

    return result?.id || 0
  } catch (err) {
    console.error('Failed to log audit:', err)
    return 0
  }
}

/**
 * Get audit logs with filtering
 */
export async function getAuditLogs(
  env: Env,
  options: {
    tenantId?: number
    userId?: number
    action?: string
    resource?: string
    startDate?: string
    endDate?: string
    status?: string
    page?: number
    perPage?: number
  }
): Promise<{ logs: AuditEntry[]; total: number }> {
  const {
    tenantId,
    userId,
    action,
    resource,
    startDate,
    endDate,
    page = 1,
    perPage = 50,
  } = options

  let sql = `
    SELECT al.*, u.email as user_email
    FROM audit_logs al
    LEFT JOIN users u ON al.user_id = u.id
    WHERE 1=1
  `
  const params: (string | number)[] = []

  if (tenantId) {
    sql += ' AND al.tenant_id = ?'
    params.push(tenantId)
  }

  if (userId) {
    sql += ' AND al.user_id = ?'
    params.push(userId)
  }

  if (action) {
    sql += ' AND al.action LIKE ?'
    params.push(`%${action}%`)
  }

  if (resource) {
    sql += ' AND al.resource = ?'
    params.push(resource)
  }

  if (startDate) {
    sql += " AND al.created_at >= datetime(?)"
    params.push(startDate)
  }

  if (endDate) {
    sql += " AND al.created_at <= datetime(?)"
    params.push(endDate)
  }

  // Count
  const countSql = `SELECT COUNT(*) as total FROM (${sql})`
  const countResult = await env.DB
    .prepare(countSql)
    .bind(...params)
    .first<{ total: number }>()

  // Paginate
  sql += ' ORDER BY al.created_at DESC LIMIT ? OFFSET ?'
  params.push(perPage, (page - 1) * perPage)

  const result = await env.DB
    .prepare(sql)
    .bind(...params)
    .all<AuditEntry>()

  return {
    logs: result.results,
    total: countResult?.total || 0,
  }
}

/**
 * Export audit logs as JSON
 */
export async function exportAuditLogsJSON(
  env: Env,
  options: Parameters<typeof getAuditLogs>[1]
): Promise<string> {
  const { logs } = await getAuditLogs(env, { ...options, perPage: 10000 })

  const exportData = logs.map(log => ({
    id: log.id,
    timestamp: log.created_at,
    tenant_id: log.tenant_id,
    user_id: log.user_id,
    user_email: log.user_email,
    action: log.action,
    resource: log.resource,
    ip: log.ip,
    status: log.status,
    details: log.details ? JSON.parse(log.details) : null,
  }))

  return JSON.stringify(exportData, null, 2)
}

/**
 * Export audit logs as CSV
 */
export async function exportAuditLogsCSV(
  env: Env,
  options: Parameters<typeof getAuditLogs>[1]
): Promise<string> {
  const { logs } = await getAuditLogs(env, { ...options, perPage: 10000 })

  const headers = ['id', 'timestamp', 'tenant_id', 'user_id', 'user_email', 'action', 'resource', 'ip', 'status', 'details']
  const rows = [headers.join(',')]

  for (const log of logs) {
    const row = [
      log.id,
      log.created_at,
      log.tenant_id || '',
      log.user_id || '',
      log.user_email || '',
      `"${(log.action || '').replace(/"/g, '""')}"`,
      `"${(log.resource || '').replace(/"/g, '""')}"`,
      log.ip || '',
      log.status || 'success',
      `"${(log.details || '').replace(/"/g, '""')}"`,
    ]
    rows.push(row.join(','))
  }

  return rows.join('\n')
}

/**
 * Send audit log to SIEM
 */
async function sendToSIEM(env: Env, entry: AuditEntry): Promise<void> {
  // Check if SIEM is configured
  const siemConfigStr = await env.KV.get('settings:siem')
  if (!siemConfigStr) return

  let config: SIEMConfig
  try {
    config = JSON.parse(siemConfigStr)
  } catch {
    return
  }

  if (!config.enabled || !config.webhook_url) return

  // Check filters
  if (config.filters && config.filters.length > 0) {
    if (!config.filters.includes(entry.action)) return
  }

  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }

    if (config.api_key) {
      headers['Authorization'] = `Bearer ${config.api_key}`
    }

    let body: string

    switch (config.format) {
      case 'cef':
        body = formatAsCEF(entry)
        break
      case 'syslog':
        body = formatAsSyslog(entry)
        break
      case 'json':
      default:
        body = JSON.stringify(entry)
    }

    await fetch(config.webhook_url, {
      method: 'POST',
      headers,
      body,
    })
  } catch (err) {
    console.error('Failed to send to SIEM:', err)
  }
}

/**
 * Format audit entry as CEF (Common Event Format)
 */
export function formatAsCEF(entry: AuditEntry): string {
  const timestamp = new Date(entry.created_at).getTime()
  const host = entry.ip || 'unknown'
  const severity = entry.status === 'failure' ? 'High' : 'Low'

  return `CEF:0|AnixOps|ControlCenter|1.0|${entry.action}|${entry.resource}|${severity}|` +
    `rt=${timestamp} ` +
    `src=${entry.ip || 'unknown'} ` +
    `suser=${entry.user_id || 'anonymous'} ` +
    `dvchost=${host} ` +
    `msg=${entry.details || ''}`
}

/**
 * Format audit entry as Syslog
 */
export function formatAsSyslog(entry: AuditEntry): string {
  const timestamp = new Date(entry.created_at).toISOString()
  const priority = entry.status === 'failure' ? 44 : 14 // facility=1 (user), severity=4 (warning) or 6 (info)

  return `<${priority}>${timestamp} anixops audit: ` +
    `user=${entry.user_id || 'anonymous'} ` +
    `action=${entry.action} ` +
    `resource=${entry.resource} ` +
    `ip=${entry.ip || 'unknown'} ` +
    `status=${entry.status}`
}

/**
 * Get audit statistics
 */
export async function getAuditStats(
  env: Env,
  tenantId?: number,
  days: number = 30
): Promise<{
  total: number
  byAction: Array<{ action: string; count: number }>
  byUser: Array<{ user_id: number; user_email: string; count: number }>
  byResource: Array<{ resource: string; count: number }>
  failures: number
}> {
  const startDate = new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString()

  const tenantFilter = tenantId ? 'AND tenant_id = ?' : ''
  const params = tenantId ? [startDate, tenantId] : [startDate]

  // Total count
  const totalResult = await env.DB
    .prepare(`
      SELECT COUNT(*) as count
      FROM audit_logs
      WHERE created_at >= datetime(?) ${tenantFilter}
    `)
    .bind(...params)
    .first<{ count: number }>()

  // By action
  const byActionResult = await env.DB
    .prepare(`
      SELECT action, COUNT(*) as count
      FROM audit_logs
      WHERE created_at >= datetime(?) ${tenantFilter}
      GROUP BY action
      ORDER BY count DESC
      LIMIT 10
    `)
    .bind(...params)
    .all()

  // By user
  const byUserResult = await env.DB
    .prepare(`
      SELECT al.user_id, u.email as user_email, COUNT(*) as count
      FROM audit_logs al
      LEFT JOIN users u ON al.user_id = u.id
      WHERE al.created_at >= datetime(?) ${tenantFilter}
      GROUP BY al.user_id
      ORDER BY count DESC
      LIMIT 10
    `)
    .bind(...params)
    .all()

  // By resource
  const byResourceResult = await env.DB
    .prepare(`
      SELECT resource, COUNT(*) as count
      FROM audit_logs
      WHERE created_at >= datetime(?) ${tenantFilter}
      GROUP BY resource
      ORDER BY count DESC
      LIMIT 10
    `)
    .bind(...params)
    .all()

  // Failures
  const failuresResult = await env.DB
    .prepare(`
      SELECT COUNT(*) as count
      FROM audit_logs
      WHERE created_at >= datetime(?) ${tenantFilter}
      AND (details LIKE '%failure%' OR details LIKE '%error%' OR details LIKE '%failed%')
    `)
    .bind(...params)
    .first<{ count: number }>()

  return {
    total: totalResult?.count || 0,
    byAction: byActionResult.results as Array<{ action: string; count: number }>,
    byUser: byUserResult.results as Array<{ user_id: number; user_email: string; count: number }>,
    byResource: byResourceResult.results as Array<{ resource: string; count: number }>,
    failures: failuresResult?.count || 0,
  }
}

/**
 * Clean up old audit logs (retention policy)
 */
export async function cleanupAuditLogs(
  env: Env,
  retentionDays: number = 90
): Promise<{ deleted: number }> {
  const cutoffDate = new Date(Date.now() - retentionDays * 24 * 60 * 60 * 1000).toISOString()

  const result = await env.DB
    .prepare(`
      DELETE FROM audit_logs
      WHERE created_at < datetime(?)
    `)
    .bind(cutoffDate)
    .run()

  return { deleted: result.meta.changes || 0 }
}

/**
 * Configure SIEM integration
 */
export async function configureSIEM(
  env: Env,
  config: SIEMConfig
): Promise<void> {
  await env.KV.put('settings:siem', JSON.stringify(config))
}

/**
 * Get SIEM configuration
 */
export async function getSIEMConfig(env: Env): Promise<SIEMConfig | null> {
  const config = await env.KV.get('settings:siem')
  if (!config) return null

  try {
    return JSON.parse(config)
  } catch {
    return null
  }
}