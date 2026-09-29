/**
 * Audit Utilities Unit Tests
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  getRequiredParam,
  getClientIP,
  getUserAgent,
  getRequestId,
  logAudit,
  logAuditWithTenant,
  getAuditLogs,
  exportAuditLogsJSON,
  exportAuditLogsCSV,
  getAuditStats,
  cleanupAuditLogs,
  configureSIEM,
  getSIEMConfig,
  AUDIT_CATEGORIES,
  formatAsCEF,
  formatAsSyslog,
  type AuditEntry,
  type SIEMConfig,
} from './audit'
import { createMockKV, createMockD1 } from '../../test/setup'

// Mock Hono Context
function createMockContext(overrides: any = {}): any {
  return {
    req: {
      param: vi.fn((name: string) => overrides.param?.[name]),
      header: vi.fn((name: string) => overrides.headers?.[name]),
    },
    env: {
      DB: createMockD1(),
      KV: createMockKV(),
      ...overrides.env,
    },
    ...overrides,
  }
}

describe('Audit Utilities', () => {
  describe('getRequiredParam', () => {
    it('should return param value when present', () => {
      const c = createMockContext({ param: { id: '123' } })
      const result = getRequiredParam(c, 'id')
      expect(result).toBe('123')
    })

    it('should throw error when param is missing', () => {
      const c = createMockContext()
      expect(() => getRequiredParam(c, 'missing')).toThrow('Missing required parameter: missing')
    })
  })

  describe('getClientIP', () => {
    it('should return client IP from CF-Connecting-IP header', () => {
      const c = createMockContext({ headers: { 'CF-Connecting-IP': '192.168.1.1' } })
      const result = getClientIP(c)
      expect(result).toBe('192.168.1.1')
    })

    it('should return null when header is missing', () => {
      const c = createMockContext({ headers: {} })
      const result = getClientIP(c)
      expect(result).toBeNull()
    })
  })

  describe('getUserAgent', () => {
    it('should return user agent from header', () => {
      const c = createMockContext({ headers: { 'User-Agent': 'Mozilla/5.0' } })
      const result = getUserAgent(c)
      expect(result).toBe('Mozilla/5.0')
    })

    it('should return null when header is missing', () => {
      const c = createMockContext({ headers: {} })
      const result = getUserAgent(c)
      expect(result).toBeNull()
    })
  })

  describe('logAudit', () => {
    it('should insert audit log to database', async () => {
      const mockDB = createMockD1()
      const c = createMockContext({
        env: { DB: mockDB },
        headers: {
          'CF-Connecting-IP': '10.0.0.1',
          'User-Agent': 'TestAgent',
        },
      })

      await logAudit(c, 1, 'login', 'auth', { email: 'test@example.com' })

      // Verify prepare was called
      expect(mockDB.prepare).toHaveBeenCalled()
    })

    it('should handle undefined user ID', async () => {
      const c = createMockContext()
      await logAudit(c, undefined, 'test', 'test')
      expect(c.env.DB.prepare).toHaveBeenCalled()
    })

    it('should handle errors gracefully', async () => {
      const c = createMockContext({
        env: {
          DB: {
            prepare: vi.fn(() => ({
              bind: vi.fn(() => ({
                run: vi.fn(() => Promise.reject(new Error('DB error'))),
              })),
            })),
          },
        },
      })

      // Should not throw
      await expect(logAudit(c, 1, 'test', 'test')).resolves.not.toThrow()
    })
  })

  describe('logAuditWithTenant', () => {
    it('should log audit with tenant context', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await logAuditWithTenant(
        env,
        1,
        123,
        'create_node',
        'node',
        { node_name: 'test-node' },
        '192.168.1.1',
        'TestAgent'
      )

      expect(typeof result).toBe('number')
    })

    it('should handle undefined user ID', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await logAuditWithTenant(
        env,
        1,
        undefined,
        'login',
        'auth'
      )

      expect(typeof result).toBe('number')
    })

    it('should handle database errors', async () => {
      const env = {
        DB: {
          prepare: vi.fn(() => ({
            bind: vi.fn(() => ({
              first: vi.fn(() => Promise.reject(new Error('DB error'))),
            })),
          })),
        },
        KV: createMockKV(),
      } as any

      const result = await logAuditWithTenant(env, 1, 1, 'test', 'test')

      expect(result).toBe(0)
    })
  })

  describe('AUDIT_CATEGORIES', () => {
    it('should have auth category', () => {
      expect(AUDIT_CATEGORIES.AUTH).toBeDefined()
      expect(AUDIT_CATEGORIES.AUTH).toContain('login')
      expect(AUDIT_CATEGORIES.AUTH).toContain('logout')
      expect(AUDIT_CATEGORIES.AUTH).toContain('register')
    })

    it('should have user category', () => {
      expect(AUDIT_CATEGORIES.USER).toBeDefined()
      expect(AUDIT_CATEGORIES.USER).toContain('create_user')
      expect(AUDIT_CATEGORIES.USER).toContain('delete_user')
    })

    it('should have node category', () => {
      expect(AUDIT_CATEGORIES.NODE).toBeDefined()
      expect(AUDIT_CATEGORIES.NODE).toContain('create_node')
      expect(AUDIT_CATEGORIES.NODE).toContain('start_node')
      expect(AUDIT_CATEGORIES.NODE).toContain('stop_node')
    })

    it('should have tenant category', () => {
      expect(AUDIT_CATEGORIES.TENANT).toBeDefined()
      expect(AUDIT_CATEGORIES.TENANT).toContain('create_tenant')
      expect(AUDIT_CATEGORIES.TENANT).toContain('add_member')
    })
  })

  describe('formatAsCEF', () => {
    it('should format audit entry as CEF', () => {
      const entry: AuditEntry = {
        id: 1,
        user_id: 123,
        action: 'login',
        resource: 'auth',
        ip: '192.168.1.1',
        status: 'success',
        created_at: '2024-01-15T10:30:00Z',
      }

      const cef = formatAsCEF(entry)

      expect(cef).toContain('CEF:0')
      expect(cef).toContain('AnixOps')
      expect(cef).toContain('login')
      expect(cef).toContain('auth')
      expect(cef).toContain('192.168.1.1')
    })

    it('should set high severity for failures', () => {
      const entry: AuditEntry = {
        id: 1,
        action: 'login',
        resource: 'auth',
        status: 'failure',
        created_at: '2024-01-15T10:30:00Z',
      }

      const cef = formatAsCEF(entry)

      expect(cef).toContain('High')
    })

    it('should set low severity for success', () => {
      const entry: AuditEntry = {
        id: 1,
        action: 'login',
        resource: 'auth',
        status: 'success',
        created_at: '2024-01-15T10:30:00Z',
      }

      const cef = formatAsCEF(entry)

      expect(cef).toContain('Low')
    })
  })

  describe('formatAsSyslog', () => {
    it('should format audit entry as syslog', () => {
      const entry: AuditEntry = {
        id: 1,
        user_id: 123,
        action: 'create_node',
        resource: 'node',
        ip: '10.0.0.1',
        status: 'success',
        created_at: '2024-01-15T10:30:00Z',
      }

      const syslog = formatAsSyslog(entry)

      expect(syslog).toContain('<')
      expect(syslog).toContain('anixops audit')
      expect(syslog).toContain('create_node')
      expect(syslog).toContain('node')
      expect(syslog).toContain('10.0.0.1')
    })

    it('should handle anonymous user', () => {
      const entry: AuditEntry = {
        id: 1,
        action: 'login',
        resource: 'auth',
        status: 'failure',
        created_at: '2024-01-15T10:30:00Z',
      }

      const syslog = formatAsSyslog(entry)

      expect(syslog).toContain('user=anonymous')
    })
  })

  describe('AuditEntry Interface', () => {
    it('should have correct structure', () => {
      const entry: AuditEntry = {
        id: 1,
        tenant_id: 1,
        user_id: 123,
        user_email: 'test@example.com',
        action: 'login',
        resource: 'auth',
        ip: '192.168.1.1',
        user_agent: 'Mozilla/5.0',
        status: 'success',
        details: '{"foo":"bar"}',
        created_at: '2024-01-15T10:30:00Z',
      }

      expect(entry.id).toBe(1)
      expect(entry.action).toBe('login')
      expect(entry.status).toBe('success')
    })

    it('should support all status types', () => {
      const statuses: Array<'success' | 'failure' | 'pending'> = ['success', 'failure', 'pending']

      statuses.forEach(status => {
        const entry: AuditEntry = {
          id: 1,
          action: 'test',
          resource: 'test',
          status,
          created_at: new Date().toISOString(),
        }
        expect(entry.status).toBe(status)
      })
    })
  })

  describe('SIEMConfig Interface', () => {
    it('should have correct structure', () => {
      const config: SIEMConfig = {
        enabled: true,
        webhook_url: 'https://siem.example.com/api/webhook',
        api_key: 'secret-key',
        format: 'json',
        filters: ['login', 'logout'],
      }

      expect(config.enabled).toBe(true)
      expect(config.webhook_url).toBe('https://siem.example.com/api/webhook')
      expect(config.format).toBe('json')
    })

    it('should support all format types', () => {
      const formats: Array<'json' | 'cef' | 'syslog'> = ['json', 'cef', 'syslog']

      formats.forEach(format => {
        const config: SIEMConfig = {
          enabled: true,
          webhook_url: 'https://example.com',
          format,
        }
        expect(config.format).toBe(format)
      })
    })

    it('should work without optional fields', () => {
      const config: SIEMConfig = {
        enabled: false,
        webhook_url: 'https://example.com',
        format: 'json',
      }

      expect(config.api_key).toBeUndefined()
      expect(config.filters).toBeUndefined()
    })
  })

  describe('CSV Export Logic', () => {
    it('should escape quotes in CSV', () => {
      const value = 'action with "quotes"'
      const escaped = `"${value.replace(/"/g, '""')}"`

      expect(escaped).toBe('"action with ""quotes"""')
    })

    it('should handle empty values', () => {
      const row = [1, '', null, 'value']
      const csv = row.map(v => v ?? '').join(',')

      expect(csv).toBe('1,,,value')
    })
  })

  describe('Retention Logic', () => {
    it('should calculate correct cutoff date', () => {
      const retentionDays = 90
      const now = Date.now()
      const cutoffDate = new Date(now - retentionDays * 24 * 60 * 60 * 1000)

      const expectedCutoff = new Date(now)
      expectedCutoff.setDate(expectedCutoff.getDate() - retentionDays)

      expect(cutoffDate.toDateString()).toBe(expectedCutoff.toDateString())
    })

    it('should handle different retention periods', () => {
      const periods = [30, 60, 90, 180, 365]

      periods.forEach(days => {
        const cutoff = new Date(Date.now() - days * 24 * 60 * 60 * 1000)
        const now = new Date()
        const diffMs = now.getTime() - cutoff.getTime()
        const diffDays = Math.floor(diffMs / (24 * 60 * 60 * 1000))

        expect(diffDays).toBe(days)
      })
    })
  })

  describe('Statistics Calculation', () => {
    it('should calculate correct total pages', () => {
      const total = 150
      const perPage = 50
      const totalPages = Math.ceil(total / perPage)

      expect(totalPages).toBe(3)
    })

    it('should handle partial pages', () => {
      const total = 151
      const perPage = 50
      const totalPages = Math.ceil(total / perPage)

      expect(totalPages).toBe(4)
    })

    it('should handle zero total', () => {
      const total = 0
      const perPage = 50
      const totalPages = Math.ceil(total / perPage)

      expect(totalPages).toBe(0)
    })
  })

  describe('getAuditLogs', () => {
    it('should get audit logs with default options', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, {})

      expect(result).toHaveProperty('logs')
      expect(result).toHaveProperty('total')
      expect(Array.isArray(result.logs)).toBe(true)
    })

    it('should filter by tenant ID', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, { tenantId: 1 })

      expect(result).toHaveProperty('logs')
    })

    it('should filter by user ID', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, { userId: 1 })

      expect(result).toHaveProperty('logs')
    })

    it('should filter by action', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, { action: 'login' })

      expect(result).toHaveProperty('logs')
    })

    it('should filter by date range', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, {
        startDate: '2024-01-01',
        endDate: '2024-12-31',
      })

      expect(result).toHaveProperty('logs')
    })

    it('should support pagination', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await getAuditLogs(env, { page: 2, perPage: 10 })

      expect(result).toHaveProperty('logs')
    })
  })

  describe('exportAuditLogsJSON', () => {
    it('should export logs as JSON', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await exportAuditLogsJSON(env, {})

      expect(typeof result).toBe('string')
      expect(() => JSON.parse(result)).not.toThrow()
    })
  })

  describe('exportAuditLogsCSV', () => {
    it('should export logs as CSV', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await exportAuditLogsCSV(env, {})

      expect(typeof result).toBe('string')
      expect(result).toContain('id,timestamp')
    })
  })

  describe('getAuditStats', () => {
    it('should return audit statistics', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const stats = await getAuditStats(env, undefined, 30)

      expect(stats).toHaveProperty('total')
      expect(stats).toHaveProperty('byAction')
      expect(stats).toHaveProperty('byUser')
      expect(stats).toHaveProperty('byResource')
      expect(stats).toHaveProperty('failures')
      expect(typeof stats.total).toBe('number')
      expect(Array.isArray(stats.byAction)).toBe(true)
    })

    it('should filter by tenant ID', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const stats = await getAuditStats(env, 1, 30)

      expect(stats).toHaveProperty('total')
    })
  })

  describe('cleanupAuditLogs', () => {
    it('should clean up old audit logs', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await cleanupAuditLogs(env, 90)

      expect(result).toHaveProperty('deleted')
      expect(typeof result.deleted).toBe('number')
    })

    it('should use default retention days', async () => {
      const mockDB = createMockD1()
      const env = { DB: mockDB, KV: createMockKV() } as any

      const result = await cleanupAuditLogs(env)

      expect(result).toHaveProperty('deleted')
    })
  })

  describe('configureSIEM', () => {
    it('should save SIEM configuration', async () => {
      const mockKV = createMockKV()
      const env = { DB: createMockD1(), KV: mockKV } as any

      const config: SIEMConfig = {
        enabled: true,
        webhook_url: 'https://siem.example.com/webhook',
        format: 'json',
      }

      await configureSIEM(env, config)

      // Verify the config was saved
      const saved = await mockKV.get('settings:siem', 'json')
      expect(saved).toEqual(config)
    })
  })

  describe('getSIEMConfig', () => {
    it('should return null when no config', async () => {
      const mockKV = createMockKV()
      const env = { DB: createMockD1(), KV: mockKV } as any

      const config = await getSIEMConfig(env)

      expect(config).toBeNull()
    })

    it('should return saved config', async () => {
      const mockKV = createMockKV()
      const env = { DB: createMockD1(), KV: mockKV } as any

      const testConfig: SIEMConfig = {
        enabled: true,
        webhook_url: 'https://test.example.com',
        format: 'cef',
        api_key: 'test-key',
      }

      await mockKV.put('settings:siem', JSON.stringify(testConfig))

      const config = await getSIEMConfig(env)

      expect(config).not.toBeNull()
      expect(config!.enabled).toBe(true)
      expect(config!.format).toBe('cef')
    })
  })

  describe('getRequestId', () => {
    it('should return request ID from header', () => {
      const c = createMockContext({ headers: { 'X-Request-ID': 'req-123' } })
      const result = getRequestId(c)
      expect(result).toBe('req-123')
    })

    it('should generate UUID when header is missing', () => {
      const c = createMockContext({ headers: {} })
      const result = getRequestId(c)
      expect(result).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i)
    })
  })
})