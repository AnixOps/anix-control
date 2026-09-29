/**
 * Elasticsearch/ELK API Handlers
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, Env } from '../types'
import { logAudit } from '../utils/audit'
import {
  indexLog,
  searchLogs,
  getLogById,
  deleteOldLogs,
  getLogStats,
  createLogIndex,
  bulkIndexLogs,
  exportLogs,
  type LogQuery,
  type IndexConfig,
} from '../services/elasticsearch'

/**
 * Search logs
 */
export async function searchLogsHandler(c: Context<{ Bindings: Env }>) {
  const query: LogQuery = {
    query: c.req.query('query'),
    level: c.req.query('level'),
    service: c.req.query('service'),
    userId: c.req.query('userId') ? parseInt(c.req.query('userId')!) : undefined,
    tenantId: c.req.query('tenantId') ? parseInt(c.req.query('tenantId')!) : undefined,
    nodeId: c.req.query('nodeId') ? parseInt(c.req.query('nodeId')!) : undefined,
    traceId: c.req.query('traceId'),
    startTime: c.req.query('startTime'),
    endTime: c.req.query('endTime'),
    limit: c.req.query('limit') ? parseInt(c.req.query('limit')!) : 50,
    offset: c.req.query('offset') ? parseInt(c.req.query('offset')!) : 0,
    sort: c.req.query('sort') as 'asc' | 'desc' || 'desc',
  }

  const result = await searchLogs(c.env, query)

  return c.json({
    success: true,
    data: result.hits,
    meta: {
      total: result.total,
      limit: query.limit,
      offset: query.offset,
      aggregations: result.aggregations,
    },
  })
}

/**
 * Get log by ID
 */
export async function getLogHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const log = await getLogById(c.env, id)

  if (!log) {
    return c.json({ success: false, error: 'Log not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: log })
}

/**
 * Index a log entry
 */
export async function indexLogHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json()

  if (!body.level || !body.message || !body.service) {
    return c.json({ success: false, error: 'level, message, and service required' } as ApiErrorResponse, 400)
  }

  const validLevels = ['debug', 'info', 'warn', 'error', 'fatal']
  if (!validLevels.includes(body.level)) {
    return c.json({ success: false, error: `level must be one of: ${validLevels.join(', ')}` }, 400)
  }

  const result = await indexLog(c.env, {
    level: body.level,
    message: body.message,
    service: body.service,
    userId: body.userId,
    tenantId: body.tenantId,
    nodeId: body.nodeId,
    traceId: body.traceId,
    spanId: body.spanId,
    metadata: body.metadata,
  })

  return c.json(result, result.success ? 201 : 500)
}

/**
 * Bulk index logs
 */
export async function bulkIndexLogsHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json()

  if (!Array.isArray(body.entries) || body.entries.length === 0) {
    return c.json({ success: false, error: 'entries array required' } as ApiErrorResponse, 400)
  }

  if (body.entries.length > 1000) {
    return c.json({ success: false, error: 'Maximum 1000 entries per batch' } as ApiErrorResponse, 400)
  }

  const result = await bulkIndexLogs(c.env, body.entries)

  return c.json({
    success: true,
    data: result,
  })
}

/**
 * Get log statistics
 */
export async function getLogStatsHandler(c: Context<{ Bindings: Env }>) {
  const stats = await getLogStats(c.env)

  return c.json({
    success: true,
    data: stats,
  })
}

/**
 * Delete old logs
 */
export async function deleteOldLogsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const retentionDays = c.req.query('retentionDays')
    ? parseInt(c.req.query('retentionDays')!)
    : 30

  const result = await deleteOldLogs(c.env, retentionDays)

  await logAudit(c, user?.sub, 'delete_old_logs', 'elasticsearch', {
    retentionDays,
    deleted: result.deleted,
  })

  return c.json({
    success: true,
    data: result,
  })
}

/**
 * Create log index
 */
export async function createLogIndexHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  if (!body.name) {
    return c.json({ success: false, error: 'name required' }, 400)
  }

  const config: IndexConfig = {
    name: body.name,
    shards: body.shards || 3,
    replicas: body.replicas || 1,
    retentionDays: body.retentionDays || 30,
  }

  const result = await createLogIndex(c.env, config)

  await logAudit(c, user?.sub, 'create_log_index', 'elasticsearch', {
    name: config.name,
    shards: config.shards,
  })

  return c.json(result, result.success ? 201 : 400)
}

/**
 * Export logs
 */
export async function exportLogsHandler(c: Context<{ Bindings: Env }>) {
  const format = (c.req.query('format') as 'json' | 'csv') || 'json'

  const query: LogQuery = {
    query: c.req.query('query'),
    level: c.req.query('level'),
    service: c.req.query('service'),
    startTime: c.req.query('startTime'),
    endTime: c.req.query('endTime'),
    limit: 10000,
  }

  const exported = await exportLogs(c.env, query, format)

  const filename = `logs-export-${new Date().toISOString().split('T')[0]}.${format}`

  return new Response(exported, {
    headers: {
      'Content-Type': format === 'json' ? 'application/json' : 'text/csv',
      'Content-Disposition': `attachment; filename="${filename}"`,
    },
  })
}

/**
 * Get logs for a specific trace
 */
export async function getTraceLogsHandler(c: Context<{ Bindings: Env }>) {
  const traceId = c.req.param('traceId') as string

  const result = await searchLogs(c.env, {
    traceId,
    limit: 100,
    sort: 'asc',
  })

  return c.json({
    success: true,
    data: result.hits,
    meta: {
      traceId,
      total: result.total,
    },
  })
}

/**
 * Get logs for a specific node
 */
export async function getNodeLogsV2Handler(c: Context<{ Bindings: Env }>) {
  const nodeId = parseInt(c.req.param('nodeId') as string)

  const result = await searchLogs(c.env, {
    nodeId,
    limit: 100,
    sort: 'desc',
  })

  return c.json({
    success: true,
    data: result.hits,
    meta: {
      nodeId,
      total: result.total,
    },
  })
}

/**
 * Get service logs
 */
export async function getServiceLogsHandler(c: Context<{ Bindings: Env }>) {
  const service = c.req.param('service') as string

  const result = await searchLogs(c.env, {
    service,
    limit: 100,
    sort: 'desc',
  })

  return c.json({
    success: true,
    data: result.hits,
    meta: {
      service,
      total: result.total,
    },
  })
}