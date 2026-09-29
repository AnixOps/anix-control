import type { Context } from 'hono'
import { z } from 'zod'
import type {
  ApiErrorResponse,
  Env,
  SchemaValidationErrorResponse,
  TaskCancelResponse,
  TaskCreateResponse,
  TaskDetailResponse,
  TaskListItem,
  TaskListResponse,
  TaskLogsResponse,
  TaskRetryResponse,
} from '../types'
import { logAudit } from '../utils/audit'
import { buildTaskChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

const createTaskSchema = z.object({
  playbook_id: z.number().int().positive().optional(),
  playbook_name: z.string().min(1),
  target_nodes: z.array(z.union([z.number(), z.string()])).min(1),
  variables: z.record(z.string(), z.unknown()).optional(),
})

const listTasksSchema = z.object({
  page: z.coerce.number().int().min(1).default(1),
  per_page: z.coerce.number().int().min(1).max(100).default(20),
  status: z.enum(['pending', 'running', 'success', 'failed', 'cancelled']).optional(),
  playbook_id: z.coerce.number().int().optional(),
})

/**
 * 获取任务列表
 */
export async function listTasksHandler(c: Context<{ Bindings: Env }>) {
  const query = Object.fromEntries(
    new URL(c.req.url).searchParams
  )
  const params = listTasksSchema.parse(query)
  const { page, per_page, status, playbook_id } = params

  let sql = `
    SELECT t.*, p.name as playbook_name, p.category,
           u.email as triggered_by_email
    FROM tasks t
    LEFT JOIN playbooks p ON t.playbook_id = p.id
    LEFT JOIN users u ON t.triggered_by = u.id
    WHERE 1=1
  `
  const bindParams: (string | number)[] = []

  if (status) {
    sql += ' AND t.status = ?'
    bindParams.push(status)
  }

  if (playbook_id) {
    sql += ' AND t.playbook_id = ?'
    bindParams.push(playbook_id)
  }

  // Count
  const countSql = `SELECT COUNT(*) as total FROM (${sql})`
  const countResult = await c.env.DB
    .prepare(countSql)
    .bind(...bindParams)
    .first<{ total: number }>()

  // Paginated results
  sql += ' ORDER BY t.created_at DESC LIMIT ? OFFSET ?'
  bindParams.push(per_page, (page - 1) * per_page)

  const result = await c.env.DB
    .prepare(sql)
    .bind(...bindParams)
    .all()

  return c.json({
    success: true,
    data: {
      items: result.results as unknown as TaskListItem[],
      total: countResult?.total || 0,
      page,
      per_page,
      total_pages: Math.ceil((countResult?.total || 0) / per_page),
    },
  } as TaskListResponse)
}

/**
 * 获取任务详情
 */
export async function getTaskHandler(c: Context<{ Bindings: Env }>) {
  const taskId = c.req.param('id') as string

  const task = await c.env.DB
    .prepare(`
      SELECT t.*, p.name as playbook_name, p.category, p.variables as playbook_variables,
             u.email as triggered_by_email
      FROM tasks t
      LEFT JOIN playbooks p ON t.playbook_id = p.id
      LEFT JOIN users u ON t.triggered_by = u.id
      WHERE t.task_id = ? OR t.id = ?
    `)
    .bind(taskId, taskId)
    .first()

  if (!task) {
    return c.json({ success: false, error: 'Task not found' }, 404)
  }

  return c.json({
    success: true,
    data: task as unknown as TaskDetailResponse['data'],
  } as TaskDetailResponse)
}

/**
 * 创建任务 (执行 Playbook)
 */
export async function createTaskHandler(c: Context<{ Bindings: Env }>) {
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = createTaskSchema.parse(body)

    // 查找 playbook
    let playbook: { id: number; name: string; storage_key: string; variables?: string } | null = null

    if (data.playbook_id) {
      playbook = await c.env.DB
        .prepare('SELECT id, name, storage_key, variables FROM playbooks WHERE id = ?')
        .bind(data.playbook_id)
        .first()
    } else if (data.playbook_name) {
      playbook = await c.env.DB
        .prepare('SELECT id, name, storage_key, variables FROM playbooks WHERE name = ?')
        .bind(data.playbook_name)
        .first()
    }

    if (!playbook) {
      return c.json({ success: false, error: 'Playbook not found' }, 404)
    }

    // 验证目标节点
    const targetNodeIds = data.target_nodes.filter(n => typeof n === 'number') as number[]
    const targetNodeNames = data.target_nodes.filter(n => typeof n === 'string') as string[]

    let nodes: { id: number; name: string; host: string }[] = []

    if (targetNodeIds.length > 0) {
      const placeholders = targetNodeIds.map(() => '?').join(',')
      const nodeResults = await c.env.DB
        .prepare(`SELECT id, name, host FROM nodes WHERE id IN (${placeholders})`)
        .bind(...targetNodeIds)
        .all<{ id: number; name: string; host: string }>()
      nodes = nodes.concat(nodeResults.results as { id: number; name: string; host: string }[])
    }

    if (targetNodeNames.length > 0) {
      const placeholders = targetNodeNames.map(() => '?').join(',')
      const nodeResults = await c.env.DB
        .prepare(`SELECT id, name, host FROM nodes WHERE name IN (${placeholders})`)
        .bind(...targetNodeNames)
        .all<{ id: number; name: string; host: string }>()
      nodes = nodes.concat(nodeResults.results as { id: number; name: string; host: string }[])
    }

    if (nodes.length === 0) {
      return c.json({ success: false, error: 'No valid target nodes found' }, 400)
    }

    // 生成任务 ID
    const taskId = crypto.randomUUID()

    // 创建任务记录
    await c.env.DB
      .prepare(`
        INSERT INTO tasks (task_id, playbook_id, playbook_name, status, trigger_type, triggered_by, target_nodes, variables)
        VALUES (?, ?, ?, 'pending', 'manual', ?, ?, ?)
      `)
      .bind(
        taskId,
        playbook.id,
        playbook.name,
        currentUser.sub,
        JSON.stringify(nodes.map(n => ({ id: n.id, name: n.name, host: n.host }))),
        data.variables ? JSON.stringify(data.variables) : null
      )
      .run()

    // 将任务放入队列 (使用 KV 作为简单队列)
    const queueItem = {
      task_id: taskId,
      playbook_id: playbook.id,
      playbook_name: playbook.name,
      storage_key: playbook.storage_key,
      nodes: nodes.map(n => ({ id: n.id, name: n.name, host: n.host })),
      variables: data.variables || {},
      triggered_by: currentUser.sub,
      created_at: new Date().toISOString(),
    }

    await c.env.KV.put(`task:queue:${taskId}`, JSON.stringify(queueItem), {
      expirationTtl: 86400, // 24 hours
    })

    // 触发任务执行 (通过 WebSocket 通知或后台处理)
    // 在 Cloudflare Workers 中可以使用 Durable Objects 或 Queue

    await logAudit(c, currentUser.sub, 'create_task', 'task', {
      task_id: taskId,
      playbook: playbook.name,
      nodes: nodes.length,
    })

    publishRealtimeEvent(makeRealtimeEvent(
      'task.created',
      'task',
      buildTaskChannels(taskId, currentUser.sub),
      {
        task_id: taskId,
        playbook_id: playbook.id,
        playbook_name: playbook.name,
        status: 'pending',
        target_nodes: queueItem.nodes,
      },
      {
        user_id: currentUser.sub,
      }
    ))

    return c.json({
      success: true,
      data: {
        task_id: taskId,
        status: 'pending',
        message: 'Task created and queued for execution',
      },
    } as TaskCreateResponse, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 取消任务
 */
export async function cancelTaskHandler(c: Context<{ Bindings: Env }>) {
  const taskId = c.req.param('id') as string
  const currentUser = c.get('user')

  const task = await c.env.DB
    .prepare("SELECT * FROM tasks WHERE task_id = ? AND status IN ('pending', 'running')")
    .bind(taskId)
    .first()

  if (!task) {
    return c.json({ success: false, error: 'Task not found or cannot be cancelled' }, 404)
  }

  await c.env.DB
    .prepare("UPDATE tasks SET status = 'cancelled', completed_at = datetime('now') WHERE task_id = ?")
    .bind(taskId)
    .run()

  // 从队列中移除
  await c.env.KV.delete(`task:queue:${taskId}`)

  await logAudit(c, currentUser.sub, 'cancel_task', 'task', { task_id: taskId })

  publishRealtimeEvent(makeRealtimeEvent(
    'task.cancelled',
    'task',
    buildTaskChannels(taskId, currentUser.sub),
    {
      task_id: taskId,
      status: 'cancelled',
    },
    {
      user_id: currentUser.sub,
    }
  ))

  return c.json({
    success: true,
    message: 'Task cancelled successfully',
  } as TaskCancelResponse)
}

/**
 * 重试任务
 */
export async function retryTaskHandler(c: Context<{ Bindings: Env }>) {
  const taskId = c.req.param('id') as string
  const currentUser = c.get('user')

  const originalTask = await c.env.DB
    .prepare("SELECT * FROM tasks WHERE task_id = ? AND status IN ('failed', 'cancelled')")
    .bind(taskId)
    .first<{ task_id: string; playbook_id: number; playbook_name: string; target_nodes: string; variables: string }>()

  if (!originalTask) {
    return c.json({ success: false, error: 'Task not found or cannot be retried' }, 404)
  }

  // 创建新任务
  const newTaskId = crypto.randomUUID()

  await c.env.DB
    .prepare(`
      INSERT INTO tasks (task_id, playbook_id, playbook_name, status, trigger_type, triggered_by, target_nodes, variables)
      VALUES (?, ?, ?, 'pending', 'manual', ?, ?, ?)
    `)
    .bind(
      newTaskId,
      originalTask.playbook_id,
      originalTask.playbook_name,
      currentUser.sub,
      originalTask.target_nodes,
      originalTask.variables
    )
    .run()

  // 添加到队列
  const targetNodes = JSON.parse(originalTask.target_nodes || '[]')
  const playbook = await c.env.DB
    .prepare('SELECT storage_key FROM playbooks WHERE id = ?')
    .bind(originalTask.playbook_id)
    .first<{ storage_key: string }>()

  if (playbook) {
    const queueItem = {
      task_id: newTaskId,
      playbook_id: originalTask.playbook_id,
      playbook_name: originalTask.playbook_name,
      storage_key: playbook.storage_key,
      nodes: targetNodes,
      variables: JSON.parse(originalTask.variables || '{}'),
      triggered_by: currentUser.sub,
      created_at: new Date().toISOString(),
    }

    await c.env.KV.put(`task:queue:${newTaskId}`, JSON.stringify(queueItem), {
      expirationTtl: 86400,
    })
  }

  await logAudit(c, currentUser.sub, 'retry_task', 'task', {
    original_task_id: taskId,
    new_task_id: newTaskId,
  })

  publishRealtimeEvent(makeRealtimeEvent(
    'task.retried',
    'task',
    buildTaskChannels(newTaskId, currentUser.sub),
    {
      original_task_id: taskId,
      task_id: newTaskId,
      status: 'pending',
    },
    {
      user_id: currentUser.sub,
    }
  ))

  return c.json({
    success: true,
    data: {
      task_id: newTaskId,
      status: 'pending',
      message: 'Task retry created and queued',
    },
  } as TaskRetryResponse)
}

/**
 * 获取任务日志
 */
export async function getTaskLogsHandler(c: Context<{ Bindings: Env }>) {
  const taskId = c.req.param('id') as string
  const level = c.req.query('level') || 'info'
  const limit = parseInt(c.req.query('limit') || '1000', 10)
  const offset = parseInt(c.req.query('offset') || '0', 10)

  // 验证任务存在
  const task = await c.env.DB
    .prepare('SELECT task_id FROM tasks WHERE task_id = ? OR id = ?')
    .bind(taskId, taskId)
    .first()

  if (!task) {
    return c.json({ success: false, error: 'Task not found' }, 404)
  }

  const logs = await c.env.DB
    .prepare(`
      SELECT * FROM task_logs
      WHERE task_id = ?
      AND (level = ? OR ? = 'all' OR (level IN ('warning', 'error') AND ? IN ('warning', 'error')) OR (level = 'error' AND ? = 'error'))
      ORDER BY created_at ASC
      LIMIT ? OFFSET ?
    `)
    .bind(taskId, level, level, level, level, limit, offset)
    .all()

  return c.json({
    success: true,
    data: logs.results as unknown as TaskLogsResponse['data'],
  } as TaskLogsResponse)
}

/**
 * 添加任务日志 (内部使用)
 */
export async function addTaskLog(
  env: Env,
  taskId: string,
  level: 'debug' | 'info' | 'warning' | 'error',
  message: string,
  nodeId?: number,
  nodeName?: string,
  metadata?: Record<string, unknown>
) {
  await env.DB
    .prepare(`
      INSERT INTO task_logs (task_id, node_id, node_name, level, message, metadata)
      VALUES (?, ?, ?, ?, ?, ?)
    `)
    .bind(taskId, nodeId || null, nodeName || null, level, message, metadata ? JSON.stringify(metadata) : null)
    .run()
}

/**
 * 更新任务状态 (内部使用)
 */
export async function updateTaskStatus(
  env: Env,
  taskId: string,
  status: 'pending' | 'running' | 'success' | 'failed' | 'cancelled',
  result?: Record<string, unknown>,
  error?: string
) {
  const updates: string[] = ['status = ?']
  const values: (string | null)[] = [status]

  if (status === 'running') {
    updates.push("started_at = datetime('now')")
  } else if (['success', 'failed', 'cancelled'].includes(status)) {
    updates.push("completed_at = datetime('now')")
  }

  if (result) {
    updates.push('result = ?')
    values.push(JSON.stringify(result))
  }

  if (error) {
    updates.push('error = ?')
    values.push(error)
  }

  values.push(taskId)

  await env.DB
    .prepare(`UPDATE tasks SET ${updates.join(', ')} WHERE task_id = ?`)
    .bind(...values)
    .run()
}