/**
 * 备份管理 API Handler
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, Env } from '../types'
import { createBackup, listBackups, getBackup, deleteBackup, getLatestBackupStatus, cleanupOldBackups, restoreBackup } from '../services/backup'
import { logAudit } from '../utils/audit'

/**
 * 创建数据库备份
 */
export async function createBackupHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  // 只有管理员可以创建备份
  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const result = await createBackup(c.env)

  await logAudit(c, user.sub, 'create_backup', 'backup', {
    backup_id: result.id,
    status: result.status,
    size: result.size,
  })

  if (result.status === 'failed') {
    return c.json({
      success: false,
      error: 'Backup failed',
      details: result.error,
    }, 500)
  }

  return c.json({
    success: true,
    data: result,
  }, 201)
}

/**
 * 列出所有备份
 */
export async function listBackupsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  // 只有管理员可以查看备份
  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const limit = parseInt(c.req.query('limit') || '20', 10)
  const backups = await listBackups(c.env, limit)
  const latestStatus = await getLatestBackupStatus(c.env)

  return c.json({
    success: true,
    data: {
      backups,
      latest: latestStatus,
      total: backups.length,
    },
  })
}

/**
 * 获取备份详情
 */
export async function getBackupHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const backupId = c.req.param('id') as string
  if (!backupId) {
    return c.json({ success: false, error: 'Backup ID is required' } as ApiErrorResponse, 400)
  }

  const backup = await getBackup(c.env, backupId)

  if (!backup) {
    return c.json({ success: false, error: 'Backup not found' } as ApiErrorResponse, 404)
  }

  return c.json({
    success: true,
    data: backup,
  })
}

/**
 * 删除备份
 */
export async function deleteBackupHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const backupId = c.req.param('id') as string
  if (!backupId) {
    return c.json({ success: false, error: 'Backup ID is required' } as ApiErrorResponse, 400)
  }

  const success = await deleteBackup(c.env, backupId)

  if (!success) {
    return c.json({ success: false, error: 'Failed to delete backup' } as ApiErrorResponse, 500)
  }

  await logAudit(c, user.sub, 'delete_backup', 'backup', { backup_id: backupId })

  return c.json({
    success: true,
    message: 'Backup deleted successfully',
  })
}

/**
 * 下载备份
 */
export async function downloadBackupHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const backupId = c.req.param('id') as string
  if (!backupId) {
    return c.json({ success: false, error: 'Backup ID is required' } as ApiErrorResponse, 400)
  }

  const object = await c.env.R2.get(`backups/d1/${backupId}.json`)

  if (!object) {
    return c.json({ success: false, error: 'Backup not found' } as ApiErrorResponse, 404)
  }

  const data = await object.text()

  return new Response(data, {
    headers: {
      'Content-Type': 'application/json',
      'Content-Disposition': `attachment; filename="backup-${backupId}.json"`,
    },
  })
}

/**
 * 恢复备份
 */
export async function restoreBackupHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const backupId = c.req.param('id') as string
  if (!backupId) {
    return c.json({ success: false, error: 'Backup ID is required' } as ApiErrorResponse, 400)
  }

  const body = await c.req.json<{ tables?: string[]; truncate?: boolean }>().catch(() => ({}))

  const result = await restoreBackup(c.env, backupId, body)

  await logAudit(c, user.sub, 'restore_backup', 'backup', {
    backup_id: backupId,
    success: result.success,
    restored: result.restored,
  })

  if (!result.success) {
    return c.json({
      success: false,
      error: result.message,
    }, 500)
  }

  return c.json({
    success: true,
    message: result.message,
    data: result.restored,
  })
}

/**
 * 清理旧备份
 */
export async function cleanupBackupsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const keepCount = parseInt(c.req.query('keep') || '30', 10)
  const deleted = await cleanupOldBackups(c.env, keepCount)

  await logAudit(c, user.sub, 'cleanup_backups', 'backup', {
    deleted_count: deleted,
    keep_count: keepCount,
  })

  return c.json({
    success: true,
    message: `Cleaned up ${deleted} old backups`,
    data: { deleted, kept: keepCount },
  })
}

/**
 * 获取备份状态
 */
export async function backupStatusHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin') {
    return c.json({ success: false, error: 'Forbidden: Admin only' } as ApiErrorResponse, 403)
  }

  const latestBackup = await getLatestBackupStatus(c.env)

  // 获取备份存储统计
  const backups = await listBackups(c.env, 100)
  const totalSize = backups.reduce((sum, b) => sum + b.size, 0)

  return c.json({
    success: true,
    data: {
      latest: latestBackup,
      statistics: {
        total_backups: backups.length,
        total_size_bytes: totalSize,
        total_size_mb: Math.round(totalSize / 1024 / 1024 * 100) / 100,
      },
    },
  })
}