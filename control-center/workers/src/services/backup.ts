/**
 * D1 数据库备份服务
 * 自动导出数据库到 R2 存储
 */

import type { Env } from '../types'

export interface BackupInfo {
  id: string
  timestamp: string
  size: number
  tables: string[]
  status: 'completed' | 'failed'
  error?: string
}

const BACKUP_PREFIX = 'backups/d1/'

/**
 * 获取所有表名
 */
async function getTableNames(db: D1Database): Promise<string[]> {
  const result = await db
    .prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '_cf_%'")
    .all<{ name: string }>()

  return result.results.map(r => r.name)
}

/**
 * 导出表数据
 */
async function exportTable(db: D1Database, tableName: string): Promise<unknown[]> {
  const result = await db
    .prepare(`SELECT * FROM ${tableName}`)
    .all()

  return result.results
}

/**
 * 创建备份
 */
export async function createBackup(env: Env): Promise<BackupInfo> {
  const backupId = `backup-${Date.now()}`
  const timestamp = new Date().toISOString()

  try {
    // 获取所有表
    const tables = await getTableNames(env.DB)

    // 导出每个表的数据
    const backupData: Record<string, unknown[]> = {}
    let totalRecords = 0

    for (const table of tables) {
      const data = await exportTable(env.DB, table)
      backupData[table] = data
      totalRecords += data.length
    }

    // 创建备份元数据
    const backupInfo: BackupInfo = {
      id: backupId,
      timestamp,
      size: 0, // 将在下面计算
      tables,
      status: 'completed',
    }

    // 组合备份数据
    const fullBackup = {
      metadata: {
        id: backupId,
        timestamp,
        version: '1.0',
        tables,
        total_records: totalRecords,
      },
      data: backupData,
    }

    const backupJson = JSON.stringify(fullBackup, null, 2)
    backupInfo.size = new Blob([backupJson]).size

    // 上传到 R2
    await env.R2.put(`${BACKUP_PREFIX}${backupId}.json`, backupJson, {
      httpMetadata: {
        contentType: 'application/json',
      },
      customMetadata: {
        'backup-timestamp': timestamp,
        'backup-tables': tables.join(','),
        'backup-size': String(backupInfo.size),
      },
    })

    // 记录备份信息到 KV
    await env.KV.put(`backup:latest`, JSON.stringify(backupInfo))
    await env.KV.put(`backup:${backupId}`, JSON.stringify(backupInfo), {
      expirationTtl: 86400 * 30, // 保留 30 天
    })

    return backupInfo
  } catch (error) {
    const backupInfo: BackupInfo = {
      id: backupId,
      timestamp,
      size: 0,
      tables: [],
      status: 'failed',
      error: error instanceof Error ? error.message : 'Unknown error',
    }

    await env.KV.put(`backup:${backupId}`, JSON.stringify(backupInfo), {
      expirationTtl: 86400 * 7, // 失败记录保留 7 天
    })

    return backupInfo
  }
}

/**
 * 列出所有备份
 */
export async function listBackups(env: Env, limit: number = 20): Promise<BackupInfo[]> {
  // 从 R2 列出备份文件
  const listed = await env.R2.list({
    prefix: BACKUP_PREFIX,
    limit,
  })

  const backups: BackupInfo[] = []

  for (const object of listed.objects) {
    const backupId = object.key.replace(BACKUP_PREFIX, '').replace('.json', '')
    const metadata = object.customMetadata || {}

    backups.push({
      id: backupId,
      timestamp: metadata['backup-timestamp'] || object.uploaded?.toISOString() || '',
      size: object.size,
      tables: metadata['backup-tables']?.split(',') || [],
      status: 'completed',
    })
  }

  // 按时间倒序排列
  return backups.sort((a, b) =>
    new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
  )
}

/**
 * 获取备份详情
 */
export async function getBackup(env: Env, backupId: string): Promise<{
  info: BackupInfo;
  data?: Record<string, unknown[]>;
} | null> {
  const object = await env.R2.get(`${BACKUP_PREFIX}${backupId}.json`)

  if (!object) {
    return null
  }

  const content = await object.json<{
    metadata: {
      id: string;
      timestamp: string;
      version: string;
      tables: string[];
      total_records: number;
    };
    data: Record<string, unknown[]>;
  }>()

  return {
    info: {
      id: backupId,
      timestamp: content.metadata.timestamp,
      size: object.size,
      tables: content.metadata.tables,
      status: 'completed',
    },
    data: content.data,
  }
}

/**
 * 删除备份
 */
export async function deleteBackup(env: Env, backupId: string): Promise<boolean> {
  try {
    await env.R2.delete(`${BACKUP_PREFIX}${backupId}.json`)
    await env.KV.delete(`backup:${backupId}`)
    return true
  } catch {
    return false
  }
}

/**
 * 清理旧备份（保留最近 N 个）
 */
export async function cleanupOldBackups(env: Env, keepCount: number = 30): Promise<number> {
  const backups = await listBackups(env, 100)

  if (backups.length <= keepCount) {
    return 0
  }

  const toDelete = backups.slice(keepCount)
  let deleted = 0

  for (const backup of toDelete) {
    if (await deleteBackup(env, backup.id)) {
      deleted++
    }
  }

  return deleted
}

/**
 * 获取最新备份状态
 */
export async function getLatestBackupStatus(env: Env): Promise<BackupInfo | null> {
  const latest = await env.KV.get('backup:latest', 'json') as BackupInfo | null
  return latest
}

/**
 * 恢复备份（谨慎使用）
 * 注意：这会覆盖现有数据
 */
export async function restoreBackup(
  env: Env,
  backupId: string,
  options: { tables?: string[]; truncate?: boolean } = {}
): Promise<{ success: boolean; message: string; restored?: Record<string, number> }> {
  const backup = await getBackup(env, backupId)

  if (!backup || !backup.data) {
    return { success: false, message: 'Backup not found' }
  }

  const tablesToRestore = options.tables || backup.info.tables
  const restored: Record<string, number> = {}

  try {
    for (const table of tablesToRestore) {
      if (!backup.data[table]) {
        continue
      }

      // 如果需要，先清空表
      if (options.truncate) {
        await env.DB.prepare(`DELETE FROM ${table}`).run()
      }

      // 插入数据
      const rows = backup.data[table] as Record<string, unknown>[]
      if (rows.length === 0) {
        restored[table] = 0
        continue
      }

      // 批量插入（简化版本，实际应该分批处理）
      const columns = Object.keys(rows[0])
      const placeholders = columns.map(() => '?').join(', ')
      const insertSql = `INSERT INTO ${table} (${columns.join(', ')}) VALUES (${placeholders})`

      for (const row of rows) {
        const values = columns.map(col => row[col])
        await env.DB.prepare(insertSql).bind(...values).run()
      }

      restored[table] = rows.length
    }

    return {
      success: true,
      message: 'Backup restored successfully',
      restored,
    }
  } catch (error) {
    return {
      success: false,
      message: `Restore failed: ${error instanceof Error ? error.message : 'Unknown error'}`,
    }
  }
}