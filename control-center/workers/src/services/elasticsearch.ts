/**
 * Elasticsearch/ELK Integration Service
 *
 * Provides centralized logging with Elasticsearch
 */

import type { Env } from '../types'

// Log entry structure
export interface LogEntry {
  id: string
  timestamp: string
  level: 'debug' | 'info' | 'warn' | 'error' | 'fatal'
  message: string
  service: string
  userId?: number
  tenantId?: number
  nodeId?: number
  traceId?: string
  spanId?: string
  metadata?: Record<string, unknown>
}

// Log search query
export interface LogQuery {
  query?: string
  level?: string
  service?: string
  userId?: number
  tenantId?: number
  nodeId?: number
  traceId?: string
  startTime?: string
  endTime?: string
  limit?: number
  offset?: number
  sort?: 'asc' | 'desc'
}

// Search result
export interface LogSearchResult {
  total: number
  hits: LogEntry[]
  aggregations?: {
    levels: Record<string, number>
    services: Record<string, number>
  }
}

// Index configuration
export interface IndexConfig {
  name: string
  shards: number
  replicas: number
  retentionDays: number
}

/**
 * Index a log entry
 */
export async function indexLog(
  env: Env,
  entry: Omit<LogEntry, 'id' | 'timestamp'>
): Promise<{ success: boolean; id?: string }> {
  try {
    const id = generateLogId()
    const timestamp = new Date().toISOString()

    const fullEntry: LogEntry = {
      ...entry,
      id,
      timestamp,
    }

    // Store in KV for immediate access (simulating Elasticsearch)
    const indexKey = `logs:${getLogIndex(timestamp)}:${id}`
    await env.KV.put(indexKey, JSON.stringify(fullEntry), {
      expirationTtl: 86400 * 30, // 30 days retention
    })

    // Also store in recent logs list for quick access
    const recentKey = `logs:recent:${entry.service || 'default'}`
    const recent = await env.KV.get(recentKey, 'json') as string[] | null
    const recentIds = recent || []
    recentIds.unshift(id)
    if (recentIds.length > 1000) recentIds.pop()
    await env.KV.put(recentKey, JSON.stringify(recentIds), { expirationTtl: 86400 })

    return { success: true, id }
  } catch (err) {
    console.error('Failed to index log:', err)
    return { success: false }
  }
}

/**
 * Search logs
 */
export async function searchLogs(
  env: Env,
  query: LogQuery
): Promise<LogSearchResult> {
  try {
    const limit = Math.min(query.limit || 50, 100)
    const offset = query.offset || 0

    // Get logs from KV (simulating Elasticsearch search)
    const logs: LogEntry[] = []
    let cursor: string | undefined

    // List logs from KV
    const listResult = await env.KV.list({
      prefix: 'logs:',
      limit: 1000,
      cursor,
    })

    for (const key of listResult.keys) {
      const log = await env.KV.get(key.name, 'json') as LogEntry | null
      if (log && matchesQuery(log, query)) {
        logs.push(log)
      }
    }

    // Sort by timestamp (handle undefined timestamps)
    logs.sort((a, b) => {
      const tsA = a.timestamp || ''
      const tsB = b.timestamp || ''
      const cmp = tsA.localeCompare(tsB)
      return query.sort === 'desc' ? -cmp : cmp
    })

    // Apply pagination
    const total = logs.length
    const paginatedHits = logs.slice(offset, offset + limit)

    // Calculate aggregations on all matching logs (not just paginated)
    const aggregations = {
      levels: aggregateByField(logs, 'level'),
      services: aggregateByField(logs, 'service'),
    }

    return {
      total,
      hits: paginatedHits,
      aggregations,
    }
  } catch (err) {
    console.error('Failed to search logs:', err)
    return {
      total: 0,
      hits: [],
      aggregations: {
        levels: {},
        services: {},
      },
    }
  }
}

/**
 * Get log by ID
 */
export async function getLogById(
  env: Env,
  id: string
): Promise<LogEntry | null> {
  try {
    // Search for log in recent indexes
    const listResult = await env.KV.list({ prefix: 'logs:', limit: 1000 })

    for (const key of listResult.keys) {
      if (key.name.includes(id)) {
        const log = await env.KV.get(key.name, 'json') as LogEntry | null
        if (log && log.id === id) {
          return log
        }
      }
    }

    return null
  } catch (err) {
    console.error('Failed to get log by ID:', err)
    return null
  }
}

/**
 * Delete logs older than retention period
 */
export async function deleteOldLogs(
  env: Env,
  retentionDays: number = 30
): Promise<{ deleted: number }> {
  try {
    const cutoff = Date.now() - retentionDays * 24 * 60 * 60 * 1000
    let deleted = 0

    const listResult = await env.KV.list({ prefix: 'logs:' })

    for (const key of listResult.keys) {
      // Extract timestamp from key
      const match = key.name.match(/logs:(\d{4}\.\d{2}\.\d{2}):/)
      if (match) {
        const logDate = new Date(match[1].replace(/\./g, '-')).getTime()
        if (logDate < cutoff) {
          await env.KV.delete(key.name)
          deleted++
        }
      }
    }

    return { deleted }
  } catch (err) {
    console.error('Failed to delete old logs:', err)
    return { deleted: 0 }
  }
}

/**
 * Get log statistics
 */
export async function getLogStats(
  env: Env
): Promise<{
  totalLogs: number
  logsByLevel: Record<string, number>
  logsByService: Record<string, number>
  storageUsed: number
}> {
  try {
    const listResult = await env.KV.list({ prefix: 'logs:' })
    const logs: LogEntry[] = []

    for (const key of listResult.keys) {
      const log = await env.KV.get(key.name, 'json') as LogEntry | null
      if (log) logs.push(log)
    }

    return {
      totalLogs: logs.length,
      logsByLevel: aggregateByField(logs, 'level'),
      logsByService: aggregateByField(logs, 'service'),
      storageUsed: estimateStorageSize(logs),
    }
  } catch (err) {
    console.error('Failed to get log stats:', err)
    return {
      totalLogs: 0,
      logsByLevel: {},
      logsByService: {},
      storageUsed: 0,
    }
  }
}

/**
 * Create log index
 */
export async function createLogIndex(
  env: Env,
  config: IndexConfig
): Promise<{ success: boolean; message: string }> {
  try {
    // Store index configuration
    await env.KV.put(
      `logs:index:${config.name}`,
      JSON.stringify(config),
      { expirationTtl: 86400 * config.retentionDays }
    )

    return {
      success: true,
      message: `Index ${config.name} created with ${config.shards} shards`,
    }
  } catch (err) {
    return {
      success: false,
      message: err instanceof Error ? err.message : 'Unknown error',
    }
  }
}

/**
 * Bulk index logs
 */
export async function bulkIndexLogs(
  env: Env,
  entries: Array<Omit<LogEntry, 'id' | 'timestamp'>>
): Promise<{ success: number; failed: number }> {
  let success = 0
  let failed = 0

  for (const entry of entries) {
    const result = await indexLog(env, entry)
    if (result.success) {
      success++
    } else {
      failed++
    }
  }

  return { success, failed }
}

/**
 * Export logs
 */
export async function exportLogs(
  env: Env,
  query: LogQuery,
  format: 'json' | 'csv' = 'json'
): Promise<string> {
  const result = await searchLogs(env, { ...query, limit: 10000 })

  if (format === 'csv') {
    const headers = ['timestamp', 'level', 'service', 'message', 'userId', 'traceId']
    const rows = result.hits.map(log => [
      log.timestamp || '',
      log.level || '',
      log.service || '',
      `"${(log.message || '').replace(/"/g, '""')}"`,
      log.userId || '',
      log.traceId || '',
    ].join(','))

    return [headers.join(','), ...rows].join('\n')
  }

  return JSON.stringify(result.hits, null, 2)
}

// Helper functions

function generateLogId(): string {
  return `log_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

function getLogIndex(timestamp: string): string {
  const date = new Date(timestamp)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}.${month}.${day}`
}

function matchesQuery(log: LogEntry, query: LogQuery): boolean {
  if (query.level && log.level !== query.level) return false
  if (query.service && log.service !== query.service) return false
  if (query.userId && log.userId !== query.userId) return false
  if (query.tenantId && log.tenantId !== query.tenantId) return false
  if (query.nodeId && log.nodeId !== query.nodeId) return false
  if (query.traceId && log.traceId !== query.traceId) return false
  if (query.startTime && log.timestamp < query.startTime) return false
  if (query.endTime && log.timestamp > query.endTime) return false
  if (query.query) {
    const searchStr = query.query.toLowerCase()
    const matchStr = `${log.message} ${log.service} ${log.level}`.toLowerCase()
    if (!matchStr.includes(searchStr)) return false
  }
  return true
}

function aggregateByField(
  logs: LogEntry[],
  field: string
): Record<string, number> {
  const result: Record<string, number> = {}
  for (const log of logs) {
    const value = (log as any)[field] as string
    if (value) {
      result[value] = (result[value] || 0) + 1
    }
  }
  return result
}

function estimateStorageSize(logs: LogEntry[]): number {
  // Rough estimate in bytes
  return logs.reduce((sum, log) => sum + JSON.stringify(log).length, 0)
}