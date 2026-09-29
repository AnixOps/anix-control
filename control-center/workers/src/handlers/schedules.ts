import type { Context } from 'hono'
import { z } from 'zod'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'

const createScheduleSchema = z.object({
  name: z.string().min(1).max(100),
  playbook_id: z.number().int().positive(),
  cron: z.string().min(1),
  timezone: z.string().default('UTC'),
  target_nodes: z.array(z.union([z.number(), z.string()])).min(1),
  variables: z.record(z.string(), z.unknown()).optional(),
  enabled: z.boolean().default(true),
})

const updateScheduleSchema = z.object({
  name: z.string().min(1).max(100).optional(),
  cron: z.string().min(1).optional(),
  timezone: z.string().optional(),
  target_nodes: z.array(z.union([z.number(), z.string()])).min(1).optional(),
  variables: z.record(z.string(), z.unknown()).optional(),
  enabled: z.boolean().optional(),
})

// 简单的 cron 解析器
function parseCron(cron: string): Date | null {
  const parts = cron.split(' ')
  if (parts.length !== 5) return null

  // 简化处理：只支持特定格式
  // 完整实现需要专门的 cron 库
  const now = new Date()

  // 每 N 分钟: */N * * * *
  if (parts[0].startsWith('*/')) {
    const minutes = parseInt(parts[0].slice(2), 10)
    if (!isNaN(minutes) && minutes > 0) {
      const next = new Date(now)
      next.setMinutes(Math.ceil(next.getMinutes() / minutes) * minutes, 0, 0)
      return next
    }
  }

  // 每小时: 0 * * * *
  if (parts[0] === '0' && parts[1] === '*') {
    const next = new Date(now)
    next.setHours(next.getHours() + 1, 0, 0, 0)
    return next
  }

  // 每天指定时间: 0 N * * *
  if (parts[1] !== '*' && !parts[1].includes('/')) {
    const hour = parseInt(parts[1], 10)
    if (!isNaN(hour)) {
      const next = new Date(now)
      next.setHours(hour, 0, 0, 0)
      if (next <= now) {
        next.setDate(next.getDate() + 1)
      }
      return next
    }
  }

  // 默认：下一小时
  const next = new Date(now)
  next.setHours(next.getHours() + 1, 0, 0, 0)
  return next
}

/**
 * 获取调度列表
 */
export async function listSchedulesHandler(c: Context<{ Bindings: Env }>) {
  const result = await c.env.DB
    .prepare(`
      SELECT s.*, p.name as playbook_name, p.category,
             u.email as created_by_email
      FROM schedules s
      LEFT JOIN playbooks p ON s.playbook_id = p.id
      LEFT JOIN users u ON s.created_by = u.id
      ORDER BY s.created_at DESC
    `)
    .all()

  return c.json({
    success: true,
    data: result.results,
  })
}

/**
 * 获取单个调度
 */
export async function getScheduleHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const schedule = await c.env.DB
    .prepare(`
      SELECT s.*, p.name as playbook_name, p.category,
             u.email as created_by_email
      FROM schedules s
      LEFT JOIN playbooks p ON s.playbook_id = p.id
      LEFT JOIN users u ON s.created_by = u.id
      WHERE s.id = ?
    `)
    .bind(id)
    .first()

  if (!schedule) {
    return c.json({ success: false, error: 'Schedule not found' }, 404)
  }

  return c.json({
    success: true,
    data: schedule,
  })
}

/**
 * 创建调度
 */
export async function createScheduleHandler(c: Context<{ Bindings: Env }>) {
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = createScheduleSchema.parse(body)

    // 验证 playbook 存在
    const playbook = await c.env.DB
      .prepare('SELECT id, name FROM playbooks WHERE id = ?')
      .bind(data.playbook_id)
      .first<{ id: number; name: string }>()

    if (!playbook) {
      return c.json({ success: false, error: 'Playbook not found' }, 404)
    }

    // 计算下次运行时间
    const nextRun = parseCron(data.cron)
    const nextRunStr = nextRun ? nextRun.toISOString() : null

    const result = await c.env.DB
      .prepare(`
        INSERT INTO schedules (name, playbook_id, playbook_name, cron, timezone, target_nodes, variables, enabled, next_run, created_by)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        RETURNING *
      `)
      .bind(
        data.name,
        data.playbook_id,
        playbook.name,
        data.cron,
        data.timezone,
        JSON.stringify(data.target_nodes),
        data.variables ? JSON.stringify(data.variables) : null,
        data.enabled ? 1 : 0,
        nextRunStr,
        currentUser.sub
      )
      .first()

    await logAudit(c, currentUser.sub, 'create_schedule', 'schedule', {
      schedule_id: (result as { id: number })?.id,
      name: data.name,
      cron: data.cron,
    })

    return c.json({
      success: true,
      data: result,
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 更新调度
 */
export async function updateScheduleHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = updateScheduleSchema.parse(body)

    // 检查调度是否存在
    const existing = await c.env.DB
      .prepare('SELECT * FROM schedules WHERE id = ?')
      .bind(id)
      .first()

    if (!existing) {
      return c.json({ success: false, error: 'Schedule not found' }, 404)
    }

    const updates: string[] = []
    const values: (string | number | null)[] = []

    if (data.name) {
      updates.push('name = ?')
      values.push(data.name)
    }
    if (data.cron) {
      updates.push('cron = ?')
      values.push(data.cron)
      // 重新计算下次运行时间
      const nextRun = parseCron(data.cron)
      updates.push('next_run = ?')
      values.push(nextRun ? nextRun.toISOString() : null)
    }
    if (data.timezone) {
      updates.push('timezone = ?')
      values.push(data.timezone)
    }
    if (data.target_nodes) {
      updates.push('target_nodes = ?')
      values.push(JSON.stringify(data.target_nodes))
    }
    if (data.variables !== undefined) {
      updates.push('variables = ?')
      values.push(data.variables ? JSON.stringify(data.variables) : null)
    }
    if (data.enabled !== undefined) {
      updates.push('enabled = ?')
      values.push(data.enabled ? 1 : 0)
    }

    if (updates.length === 0) {
      return c.json({ success: false, error: 'No fields to update' }, 400)
    }

    updates.push("updated_at = datetime('now')")
    values.push(id)

    const result = await c.env.DB
      .prepare(`UPDATE schedules SET ${updates.join(', ')} WHERE id = ? RETURNING *`)
      .bind(...values)
      .first()

    await logAudit(c, currentUser.sub, 'update_schedule', 'schedule', { schedule_id: id })

    return c.json({
      success: true,
      data: result,
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 删除调度
 */
export async function deleteScheduleHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  const result = await c.env.DB
    .prepare('DELETE FROM schedules WHERE id = ? RETURNING id, name')
    .bind(id)
    .first()

  if (!result) {
    return c.json({ success: false, error: 'Schedule not found' }, 404)
  }

  await logAudit(c, currentUser.sub, 'delete_schedule', 'schedule', { schedule_id: id })

  return c.json({
    success: true,
    message: 'Schedule deleted successfully',
  })
}

/**
 * 切换调度启用状态
 */
export async function toggleScheduleHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  const existing = await c.env.DB
    .prepare('SELECT id, enabled FROM schedules WHERE id = ?')
    .bind(id)
    .first<{ id: number; enabled: number }>()

  if (!existing) {
    return c.json({ success: false, error: 'Schedule not found' }, 404)
  }

  const newEnabled = existing.enabled ? 0 : 1

  await c.env.DB
    .prepare('UPDATE schedules SET enabled = ?, updated_at = datetime(\'now\') WHERE id = ?')
    .bind(newEnabled, id)
    .run()

  await logAudit(c, currentUser.sub, 'toggle_schedule', 'schedule', {
    schedule_id: id,
    enabled: newEnabled,
  })

  return c.json({
    success: true,
    data: { enabled: newEnabled },
  })
}

/**
 * 立即运行调度
 */
export async function runScheduleNowHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  const schedule = await c.env.DB
    .prepare('SELECT * FROM schedules WHERE id = ?')
    .bind(id)
    .first<{ id: number; playbook_id: number; playbook_name: string; target_nodes: string; variables: string }>()

  if (!schedule) {
    return c.json({ success: false, error: 'Schedule not found' }, 404)
  }

  // 创建任务
  const taskId = crypto.randomUUID()
  const targetNodes = JSON.parse(schedule.target_nodes || '[]')
  const variables = JSON.parse(schedule.variables || '{}')

  // 获取 playbook storage_key
  const playbook = await c.env.DB
    .prepare('SELECT storage_key FROM playbooks WHERE id = ?')
    .bind(schedule.playbook_id)
    .first<{ storage_key: string }>()

  await c.env.DB
    .prepare(`
      INSERT INTO tasks (task_id, playbook_id, playbook_name, status, trigger_type, triggered_by, target_nodes, variables)
      VALUES (?, ?, ?, 'pending', 'scheduled', ?, ?, ?)
    `)
    .bind(
      taskId,
      schedule.playbook_id,
      schedule.playbook_name,
      currentUser.sub,
      schedule.target_nodes,
      schedule.variables
    )
    .run()

  // 添加到队列
  if (playbook) {
    const queueItem = {
      task_id: taskId,
      playbook_id: schedule.playbook_id,
      playbook_name: schedule.playbook_name,
      storage_key: playbook.storage_key,
      nodes: targetNodes,
      variables,
      triggered_by: currentUser.sub,
      created_at: new Date().toISOString(),
    }

    await c.env.KV.put(`task:queue:${taskId}`, JSON.stringify(queueItem), {
      expirationTtl: 86400,
    })
  }

  // 更新调度最后运行时间
  await c.env.DB
    .prepare('UPDATE schedules SET last_run = datetime(\'now\'), last_task_id = ? WHERE id = ?')
    .bind(taskId, id)
    .run()

  await logAudit(c, currentUser.sub, 'run_schedule', 'schedule', {
    schedule_id: id,
    task_id: taskId,
  })

  return c.json({
    success: true,
    data: {
      task_id: taskId,
      status: 'pending',
      message: 'Schedule triggered successfully',
    },
  })
}