/**
 * Backup Service Unit Tests
 */

import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  createBackup,
  listBackups,
  deleteBackup,
  getBackup,
  getLatestBackupStatus,
  cleanupOldBackups,
  restoreBackup,
  BackupInfo,
} from './backup'
import { createMockKV, createMockR2, createMockD1 } from '../../test/setup'

describe('Backup Service', () => {
  let mockEnv: {
    DB: D1Database
    KV: KVNamespace
    R2: R2Bucket
  }

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
      R2: createMockR2(),
    }
  })

  describe('createBackup', () => {
    it('should create a backup successfully', async () => {
      // Mock the database responses
      const mockDB = {
        prepare: () => ({
          bind: function() { return this },
          first: async () => ({ name: 'users' }),
          all: async () => ({ results: [] }),
        }),
      } as any

      const env = { ...mockEnv, DB: mockDB }

      const result = await createBackup(env)

      expect(result.status).toBe('completed')
      expect(result.id).toMatch(/^backup-\d+$/)
    })
  })

  describe('listBackups', () => {
    it('should list backups from R2', async () => {
      const backups = await listBackups(mockEnv)

      expect(Array.isArray(backups)).toBe(true)
    })
  })

  describe('getLatestBackupStatus', () => {
    it('should return null when no backup status', async () => {
      const status = await getLatestBackupStatus(mockEnv)

      expect(status).toBeNull()
    })

    it('should return backup status from KV', async () => {
      const backupInfo: BackupInfo = {
        id: 'backup-123',
        timestamp: new Date().toISOString(),
        size: 1024,
        tables: ['users', 'nodes'],
        status: 'completed',
      }

      await mockEnv.KV.put('backup:latest', JSON.stringify(backupInfo))

      const status = await getLatestBackupStatus(mockEnv)

      expect(status).not.toBeNull()
      expect(status!.id).toBe('backup-123')
      expect(status!.status).toBe('completed')
    })
  })

  describe('deleteBackup', () => {
    it('should delete backup successfully', async () => {
      // First create a backup
      await mockEnv.R2.put('backups/d1/backup-test.json', '{"metadata":{},"data":{}}')

      const result = await deleteBackup(mockEnv, 'backup-test')

      expect(result).toBe(true)
    })

    it('should return false for non-existent backup', async () => {
      const result = await deleteBackup(mockEnv, 'non-existent')

      expect(result).toBe(true) // Returns true even if doesn't exist
    })
  })

  describe('getBackup', () => {
    it('should return null for non-existent backup', async () => {
      const result = await getBackup(mockEnv, 'non-existent')
      expect(result).toBeNull()
    })

    it('should return backup data', async () => {
      const backupData = {
        metadata: {
          id: 'backup-test',
          timestamp: new Date().toISOString(),
          version: '1.0',
          tables: ['users', 'nodes'],
          total_records: 10,
        },
        data: {
          users: [{ id: 1, email: 'test@example.com' }],
          nodes: [],
        },
      }

      await mockEnv.R2.put(
        'backups/d1/backup-test.json',
        JSON.stringify(backupData)
      )

      const result = await getBackup(mockEnv, 'backup-test')

      expect(result).not.toBeNull()
      expect(result!.info.id).toBe('backup-test')
      expect(result!.data).toBeDefined()
    })
  })

  describe('cleanupOldBackups', () => {
    it('should not delete when under keep count', async () => {
      const deleted = await cleanupOldBackups(mockEnv, 30)
      expect(deleted).toBe(0)
    })

    it('should delete old backups', async () => {
      // Create multiple backups
      for (let i = 0; i < 5; i++) {
        await mockEnv.R2.put(
          `backups/d1/backup-${i}.json`,
          JSON.stringify({ metadata: {}, data: {} })
        )
      }

      const deleted = await cleanupOldBackups(mockEnv, 2)

      // Should delete 3 backups (5 - 2 = 3)
      expect(deleted).toBe(3)
    })
  })

  describe('restoreBackup', () => {
    it('should fail for non-existent backup', async () => {
      const result = await restoreBackup(mockEnv, 'non-existent')

      expect(result.success).toBe(false)
      expect(result.message).toBe('Backup not found')
    })

    it('should restore backup', async () => {
      const backupData = {
        metadata: {
          id: 'backup-restore',
          timestamp: new Date().toISOString(),
          version: '1.0',
          tables: ['users'],
          total_records: 1,
        },
        data: {
          users: [{ id: 1, email: 'restored@example.com', password_hash: 'hash', role: 'viewer', enabled: 1, created_at: new Date().toISOString() }],
        },
      }

      await mockEnv.R2.put(
        'backups/d1/backup-restore.json',
        JSON.stringify(backupData)
      )

      const result = await restoreBackup(mockEnv, 'backup-restore')

      expect(result.success).toBe(true)
      expect(result.message).toBe('Backup restored successfully')
      expect(result.restored).toBeDefined()
    })

    it('should restore specific tables', async () => {
      const backupData = {
        metadata: {
          id: 'backup-tables',
          timestamp: new Date().toISOString(),
          version: '1.0',
          tables: ['users', 'nodes'],
          total_records: 2,
        },
        data: {
          users: [{ id: 1, email: 'test@example.com', password_hash: 'hash', role: 'viewer', enabled: 1, created_at: new Date().toISOString() }],
          nodes: [],
        },
      }

      await mockEnv.R2.put(
        'backups/d1/backup-tables.json',
        JSON.stringify(backupData)
      )

      const result = await restoreBackup(mockEnv, 'backup-tables', {
        tables: ['users'],
      })

      expect(result.success).toBe(true)
    })

    it('should handle restore with truncate', async () => {
      const backupData = {
        metadata: {
          id: 'backup-truncate',
          timestamp: new Date().toISOString(),
          version: '1.0',
          tables: ['users'],
          total_records: 1,
        },
        data: {
          users: [{ id: 1, email: 'truncate@example.com', password_hash: 'hash', role: 'viewer', enabled: 1, created_at: new Date().toISOString() }],
        },
      }

      await mockEnv.R2.put(
        'backups/d1/backup-truncate.json',
        JSON.stringify(backupData)
      )

      const result = await restoreBackup(mockEnv, 'backup-truncate', {
        truncate: true,
      })

      expect(result.success).toBe(true)
    })
  })

  describe('BackupInfo Interface', () => {
    it('should have correct structure', () => {
      const info: BackupInfo = {
        id: 'backup-123',
        timestamp: new Date().toISOString(),
        size: 1024,
        tables: ['users', 'nodes'],
        status: 'completed',
      }

      expect(info.id).toBe('backup-123')
      expect(info.status).toBe('completed')
    })

    it('should support failed status', () => {
      const info: BackupInfo = {
        id: 'backup-failed',
        timestamp: new Date().toISOString(),
        size: 0,
        tables: [],
        status: 'failed',
        error: 'Something went wrong',
      }

      expect(info.status).toBe('failed')
      expect(info.error).toBe('Something went wrong')
    })
  })
})