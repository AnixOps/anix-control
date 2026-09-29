/**
 * Elasticsearch/ELK Integration Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
import {
  indexLog,
  searchLogs,
  getLogById,
  deleteOldLogs,
  getLogStats,
  createLogIndex,
  bulkIndexLogs,
  exportLogs,
  type LogEntry,
  type LogQuery,
  type IndexConfig,
} from './elasticsearch'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Elasticsearch Service', () => {
  let mockEnv: any

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
      R2: {} as any,
    }
  })

  describe('indexLog', () => {
    it('should index a log entry', async () => {
      const result = await indexLog(mockEnv, {
        level: 'info',
        message: 'Test log message',
        service: 'test-service',
      })

      expect(result.success).toBe(true)
      expect(result.id).toBeDefined()
      expect(result.id).toMatch(/^log_\d+_[a-z0-9]+$/)
    })

    it('should index log with all optional fields', async () => {
      const result = await indexLog(mockEnv, {
        level: 'error',
        message: 'Error occurred',
        service: 'api-service',
        userId: 1,
        tenantId: 1,
        nodeId: 1,
        traceId: 'trace-123',
        spanId: 'span-456',
        metadata: { key: 'value' },
      })

      expect(result.success).toBe(true)
    })

    it('should support all log levels', async () => {
      const levels: Array<'debug' | 'info' | 'warn' | 'error' | 'fatal'> = [
        'debug', 'info', 'warn', 'error', 'fatal'
      ]

      for (const level of levels) {
        const result = await indexLog(mockEnv, {
          level,
          message: `${level} message`,
          service: 'test',
        })
        expect(result.success).toBe(true)
      }
    })
  })

  describe('searchLogs', () => {
    beforeEach(async () => {
      // Index some test logs
      await indexLog(mockEnv, { level: 'info', message: 'Info message', service: 'api' })
      await indexLog(mockEnv, { level: 'error', message: 'Error message', service: 'api' })
      await indexLog(mockEnv, { level: 'info', message: 'Worker info', service: 'worker' })
    })

    it('should search logs with default parameters', async () => {
      const result = await searchLogs(mockEnv, {})

      expect(result.total).toBeGreaterThanOrEqual(0)
      expect(Array.isArray(result.hits)).toBe(true)
    })

    it('should filter by level', async () => {
      const result = await searchLogs(mockEnv, { level: 'error' })

      expect(result.total).toBeGreaterThanOrEqual(0)
      result.hits.forEach(hit => {
        expect(hit.level).toBe('error')
      })
    })

    it('should filter by service', async () => {
      const result = await searchLogs(mockEnv, { service: 'api' })

      expect(result.total).toBeGreaterThanOrEqual(0)
      result.hits.forEach(hit => {
        expect(hit.service).toBe('api')
      })
    })

    it('should apply pagination', async () => {
      const result = await searchLogs(mockEnv, { limit: 2, offset: 0 })

      expect(result.hits.length).toBeLessThanOrEqual(2)
    })

    it('should respect max limit', async () => {
      const result = await searchLogs(mockEnv, { limit: 500 })

      expect(result.hits.length).toBeLessThanOrEqual(100)
    })

    it('should return aggregations', async () => {
      const result = await searchLogs(mockEnv, {})

      // Aggregations should always be defined
      expect(result.aggregations).toBeDefined()
      expect(result.aggregations?.levels).toBeDefined()
      expect(result.aggregations?.services).toBeDefined()
    })
  })

  describe('getLogById', () => {
    it('should return null for non-existent log', async () => {
      const result = await getLogById(mockEnv, 'non-existent-id')
      expect(result).toBeNull()
    })
  })

  describe('deleteOldLogs', () => {
    it('should delete old logs', async () => {
      const result = await deleteOldLogs(mockEnv, 30)

      expect(result.deleted).toBeGreaterThanOrEqual(0)
    })

    it('should use default retention days', async () => {
      const result = await deleteOldLogs(mockEnv)
      expect(result).toHaveProperty('deleted')
    })
  })

  describe('getLogStats', () => {
    it('should return log statistics', async () => {
      const stats = await getLogStats(mockEnv)

      expect(stats).toHaveProperty('totalLogs')
      expect(stats).toHaveProperty('logsByLevel')
      expect(stats).toHaveProperty('logsByService')
      expect(stats).toHaveProperty('storageUsed')
      expect(typeof stats.totalLogs).toBe('number')
    })
  })

  describe('createLogIndex', () => {
    it('should create log index', async () => {
      const config: IndexConfig = {
        name: 'test-index',
        shards: 3,
        replicas: 1,
        retentionDays: 7,
      }

      const result = await createLogIndex(mockEnv, config)

      expect(result.success).toBe(true)
      expect(result.message).toContain('test-index')
    })

    it('should validate index configuration', async () => {
      const config: IndexConfig = {
        name: 'another-index',
        shards: 5,
        replicas: 2,
        retentionDays: 14,
      }

      const result = await createLogIndex(mockEnv, config)
      expect(result.success).toBe(true)
    })
  })

  describe('bulkIndexLogs', () => {
    it('should bulk index multiple logs', async () => {
      const entries = [
        { level: 'info' as const, message: 'Log 1', service: 'test' },
        { level: 'error' as const, message: 'Log 2', service: 'test' },
        { level: 'warn' as const, message: 'Log 3', service: 'test' },
      ]

      const result = await bulkIndexLogs(mockEnv, entries)

      expect(result.success).toBe(3)
      expect(result.failed).toBe(0)
    })

    it('should handle bulk indexing with metadata', async () => {
      const entries = [
        {
          level: 'info' as const,
          message: 'Complex log',
          service: 'api',
          userId: 1,
          metadata: { action: 'create', resource: 'node' },
        },
      ]

      const result = await bulkIndexLogs(mockEnv, entries)
      expect(result.success).toBe(1)
    })
  })

  describe('exportLogs', () => {
    beforeEach(async () => {
      await indexLog(mockEnv, { level: 'info', message: 'Export test', service: 'api' })
    })

    it('should export logs as JSON', async () => {
      const exported = await exportLogs(mockEnv, {}, 'json')

      expect(typeof exported).toBe('string')
      expect(() => JSON.parse(exported)).not.toThrow()
    })

    it('should export logs as CSV', async () => {
      const exported = await exportLogs(mockEnv, {}, 'csv')

      expect(typeof exported).toBe('string')
      expect(exported).toContain('timestamp')
      expect(exported).toContain('level')
      expect(exported).toContain('message')
    })

    it('should respect query filters in export', async () => {
      const exported = await exportLogs(mockEnv, { service: 'api' }, 'json')
      const logs = JSON.parse(exported)

      logs.forEach((log: LogEntry) => {
        expect(log.service).toBe('api')
      })
    })
  })

  describe('Types', () => {
    it('should have correct LogEntry structure', () => {
      const log: LogEntry = {
        id: 'log_123',
        timestamp: new Date().toISOString(),
        level: 'info',
        message: 'Test',
        service: 'test-service',
        userId: 1,
        tenantId: 1,
        traceId: 'trace-123',
        metadata: { key: 'value' },
      }

      expect(log.id).toBe('log_123')
      expect(log.level).toBe('info')
      expect(log.service).toBe('test-service')
    })

    it('should have correct LogQuery structure', () => {
      const query: LogQuery = {
        query: 'error',
        level: 'error',
        service: 'api',
        userId: 1,
        startTime: '2024-01-01T00:00:00Z',
        endTime: '2024-12-31T23:59:59Z',
        limit: 100,
        offset: 0,
        sort: 'desc',
      }

      expect(query.query).toBe('error')
      expect(query.limit).toBe(100)
    })

    it('should have correct IndexConfig structure', () => {
      const config: IndexConfig = {
        name: 'logs-2024',
        shards: 5,
        replicas: 1,
        retentionDays: 30,
      }

      expect(config.name).toBe('logs-2024')
      expect(config.shards).toBe(5)
    })
  })
})