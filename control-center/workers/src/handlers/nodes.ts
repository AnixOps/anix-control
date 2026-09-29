import type { Context } from 'hono'
import { z } from 'zod'
import type {
  ApiErrorResponse,
  ApiMessageResponse,
  Env,
  Node,
  NodeActionResponse,
  NodeBulkActionResponse,
  NodeConnectionTestResponse,
  NodeCreateResponse,
  NodeDeleteResponse,
  NodeGetResponse,
  NodeLogsResponse,
  NodeStatsResponse,
  NodeSummaryResponse,
  NodeSyncResponse,
  NodeUpdateResponse,
  SchemaValidationErrorResponse,
} from '../types'
import { logAudit } from '../utils/audit'
import { buildNodeChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

const createNodeSchema = z.object({
  name: z.string().min(1).max(100),
  host: z.string().min(1),
  port: z.number().int().min(1).max(65535).default(22),
  config: z.record(z.string(), z.unknown()).optional(),
})

const updateNodeSchema = z.object({
  name: z.string().min(1).max(100).optional(),
  host: z.string().min(1).optional(),
  port: z.number().int().min(1).max(65535).optional(),
  status: z.enum(['online', 'offline', 'maintenance']).optional(),
  config: z.record(z.string(), z.unknown()).optional(),
})

/**
 * 获取节点列表
 */
export async function listNodesHandler(c: Context<{ Bindings: Env }>) {
  const page = parseInt(c.req.query('page') || '1', 10)
  const perPage = parseInt(c.req.query('per_page') || '20', 10)
  const status = c.req.query('status')

  // 构建查询条件
  let whereClause = ''
  const params: (string | number)[] = []

  if (status) {
    whereClause = ' WHERE status = ?'
    params.push(status)
  }

  // 获取总数
  const countResult = await c.env.DB
    .prepare(`SELECT COUNT(*) as total FROM nodes${whereClause}`)
    .bind(...params)
    .first<{ total: number }>()

  // 获取分页数据
  const query = `SELECT * FROM nodes${whereClause} ORDER BY created_at DESC LIMIT ? OFFSET ?`
  params.push(perPage, (page - 1) * perPage)

  const result = await c.env.DB
    .prepare(query)
    .bind(...params)
    .all<Node>()

  return c.json({
    success: true,
    data: {
      items: result.results,
      total: countResult?.total || 0,
      page,
      per_page: perPage,
      total_pages: Math.ceil((countResult?.total || 0) / perPage),
    },
  } as NodeSummaryResponse)
}

/**
 * 获取单个节点
 */
export async function getNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  return c.json({
    success: true,
    data: node,
  })
}

/**
 * 创建节点
 */
export async function createNodeHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const data = createNodeSchema.parse(body)
    const user = c.get('user')

    // 检查名称是否已存在
    const existing = await c.env.DB
      .prepare('SELECT id FROM nodes WHERE name = ?')
      .bind(data.name)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Node name already exists' } as ApiErrorResponse, 409)
    }

    const result = await c.env.DB
      .prepare(`
        INSERT INTO nodes (name, host, port, status, config)
        VALUES (?, ?, ?, 'offline', ?)
        RETURNING *
      `)
      .bind(
        data.name,
        data.host,
        data.port,
        data.config ? JSON.stringify(data.config) : null
      )
      .first<Node>()

    // 记录审计日志
    await logAudit(c, user.sub, 'create_node', 'node', { node_id: result?.id, name: data.name })

    publishRealtimeEvent(makeRealtimeEvent(
      'node.created',
      'node',
      buildNodeChannels(result?.id || data.name),
      {
        node_id: result?.id,
        name: data.name,
        host: data.host,
        port: data.port,
        status: 'offline',
      },
      {
        user_id: user.sub,
        resource: {
          kind: 'node',
          id: result?.id || data.name,
          name: data.name,
        },
      }
    ))

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
 * 更新节点
 */
export async function updateNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = updateNodeSchema.parse(body)

    // 检查节点是否存在
    const existing = await c.env.DB
      .prepare('SELECT id FROM nodes WHERE id = ?')
      .bind(id)
      .first()

    if (!existing) {
      return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
    }

    // 构建更新语句
    const updates: string[] = []
    const values: (string | number | null)[] = []

    if (data.name) {
      updates.push('name = ?')
      values.push(data.name)
    }
    if (data.host) {
      updates.push('host = ?')
      values.push(data.host)
    }
    if (data.port) {
      updates.push('port = ?')
      values.push(data.port)
    }
    if (data.status) {
      updates.push('status = ?')
      values.push(data.status)
    }
    if (data.config !== undefined) {
      updates.push('config = ?')
      values.push(JSON.stringify(data.config))
    }

    if (updates.length === 0) {
      return c.json({ success: false, error: 'No fields to update' } as ApiErrorResponse, 400)
    }

    updates.push('updated_at = datetime(\'now\')')
    values.push(id)

    const result = await c.env.DB
      .prepare(`UPDATE nodes SET ${updates.join(', ')} WHERE id = ? RETURNING *`)
      .bind(...values)
      .first<Node>()

    await logAudit(c, user.sub, 'update_node', 'node', { node_id: id })

    publishRealtimeEvent(makeRealtimeEvent(
      'node.updated',
      'node',
      buildNodeChannels(id),
      {
        node_id: Number(id),
        name: data.name || result?.name,
        host: data.host || result?.host,
        port: data.port || result?.port,
        status: data.status || result?.status,
      },
      {
        user_id: user.sub,
        resource: {
          kind: 'node',
          id: Number(id),
          name: result?.name,
        },
      }
    ))

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
 * 删除节点
 */
export async function deleteNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const result = await c.env.DB
    .prepare('DELETE FROM nodes WHERE id = ? RETURNING id')
    .bind(id)
    .first()

  if (!result) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, user.sub, 'delete_node', 'node', { node_id: id })

  publishRealtimeEvent(makeRealtimeEvent(
    'node.deleted',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
    },
    {
      user_id: user.sub,
      resource: {
        kind: 'node',
        id: Number(id),
      },
    }
  ))

  return c.json({
    success: true,
    message: 'Node deleted successfully',
  } as NodeDeleteResponse)
}

/**
 * 启动节点
 */
export async function startNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 更新状态为启动中
  await c.env.DB
    .prepare('UPDATE nodes SET status = ?, updated_at = datetime(\'now\') WHERE id = ?')
    .bind('maintenance', id)
    .run()

  publishRealtimeEvent(makeRealtimeEvent(
    'node.action',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
      action: 'start',
      status: 'starting',
    },
    {
      user_id: user.sub,
      resource: {
        kind: 'node',
        id: Number(id),
        name: node.name,
      },
    }
  ))

  // 模拟启动操作 (实际应该通过SSH或API调用)
  // 这里我们异步更新状态
  c.executionCtx.waitUntil(
    simulateNodeOperation(c.env, id, 'start')
  )

  await logAudit(c, user.sub, 'start_node', 'node', { node_id: id, node_name: node.name })

  return c.json({
    success: true,
    message: 'Node start initiated',
    data: { id, status: 'starting' },
  } as NodeActionResponse)
}

/**
 * 停止节点
 */
export async function stopNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 更新状态为停止中
  await c.env.DB
    .prepare('UPDATE nodes SET status = ?, updated_at = datetime(\'now\') WHERE id = ?')
    .bind('maintenance', id)
    .run()

  publishRealtimeEvent(makeRealtimeEvent(
    'node.action',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
      action: 'stop',
      status: 'stopping',
    },
    {
      user_id: user.sub,
      resource: {
        kind: 'node',
        id: Number(id),
        name: node.name,
      },
    }
  ))

  // 异步执行停止操作
  c.executionCtx.waitUntil(
    simulateNodeOperation(c.env, id, 'stop')
  )

  await logAudit(c, user.sub, 'stop_node', 'node', { node_id: id, node_name: node.name })

  return c.json({
    success: true,
    message: 'Node stop initiated',
    data: { id, status: 'stopping' },
  } as NodeActionResponse)
}

/**
 * 重启节点
 */
export async function restartNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 更新状态为重启中
  await c.env.DB
    .prepare('UPDATE nodes SET status = ?, updated_at = datetime(\'now\') WHERE id = ?')
    .bind('maintenance', id)
    .run()

  publishRealtimeEvent(makeRealtimeEvent(
    'node.action',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
      action: 'restart',
      status: 'restarting',
    },
    {
      user_id: user.sub,
      resource: {
        kind: 'node',
        id: Number(id),
        name: node.name,
      },
    }
  ))

  // 异步执行重启操作
  c.executionCtx.waitUntil(
    simulateNodeOperation(c.env, id, 'restart')
  )

  await logAudit(c, user.sub, 'restart_node', 'node', { node_id: id, node_name: node.name })

  return c.json({
    success: true,
    message: 'Node restart initiated',
    data: { id, status: 'restarting' },
  } as NodeActionResponse)
}

/**
 * 获取节点统计信息
 */
export async function getNodeStatsHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 从KV获取缓存的统计数据，或者生成模拟数据
  const cachedStats = await c.env.KV.get(`node:stats:${id}`, 'json')

  if (cachedStats) {
    return c.json({
      success: true,
      data: cachedStats,
    } as NodeStatsResponse)
  }

  // 生成模拟统计数据
  const stats = {
    node_id: parseInt(id),
    status: node.status,
    uptime: node.status === 'online' ? Math.floor(Math.random() * 86400 * 30) : 0,
    cpu_usage: node.status === 'online' ? Math.random() * 100 : 0,
    memory_usage: node.status === 'online' ? Math.random() * 100 : 0,
    disk_usage: node.status === 'online' ? 20 + Math.random() * 60 : 0,
    network: {
      upload: node.status === 'online' ? Math.floor(Math.random() * 1000000000) : 0,
      download: node.status === 'online' ? Math.floor(Math.random() * 1000000000) : 0,
    },
    connections: node.status === 'online' ? Math.floor(Math.random() * 1000) : 0,
    users: node.status === 'online' ? Math.floor(Math.random() * 100) : 0,
    last_updated: new Date().toISOString(),
  }

  // 缓存统计数据30秒
  await c.env.KV.put(`node:stats:${id}`, JSON.stringify(stats), { expirationTtl: 30 })

  return c.json({
    success: true,
    data: stats,
  } as NodeStatsResponse)
}

/**
 * 获取节点日志
 */
export async function getNodeLogsHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const limit = Math.min(parseInt(c.req.query('limit') || '100'), 1000)
  const level = c.req.query('level') // info, warn, error

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 从KV获取日志
  const logsKey = `node:logs:${id}`
  const logsData = await c.env.KV.get(logsKey, 'json') as Array<{
    timestamp: string;
    level: string;
    message: string;
  }> | null

  let logs = logsData || generateMockLogs(node.status)

  // 按级别过滤
  if (level) {
    logs = logs.filter(log => log.level === level)
  }

  // 限制数量
  logs = logs.slice(0, limit)

  return c.json({
    success: true,
    data: {
      node_id: parseInt(id),
      node_name: node.name,
      logs,
      total: logs.length,
    },
  } as NodeLogsResponse)
}

/**
 * 测试节点连接
 */
export async function testNodeConnectionHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // 模拟连接测试
  const isOnline = Math.random() > 0.2 // 80% 成功率

  const result = {
    node_id: parseInt(id),
    host: node.host,
    port: node.port,
    reachable: isOnline,
    response_time: isOnline ? Math.floor(Math.random() * 100) + 10 : null,
    error: isOnline ? null : 'Connection refused',
    tested_at: new Date().toISOString(),
  }

  // 更新节点状态
  await c.env.DB
    .prepare('UPDATE nodes SET status = ?, last_seen = datetime(\'now\'), updated_at = datetime(\'now\') WHERE id = ?')
    .bind(isOnline ? 'online' : 'offline', id)
    .run()

  publishRealtimeEvent(makeRealtimeEvent(
    'node.connection_tested',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
      reachable: isOnline,
      response_time: result.response_time,
      tested_at: result.tested_at,
    },
    {
      user_id: user?.sub,
      resource: {
        kind: 'node',
        id: Number(id),
        name: node.name,
      },
    }
  ))

  return c.json({
    success: true,
    data: result,
  } as NodeConnectionTestResponse)
}

/**
 * 同步节点配置
 */
export async function syncNodeHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const user = c.get('user')

  const node = await c.env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(id)
    .first<Node>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, user.sub, 'sync_node', 'node', { node_id: id, node_name: node.name })

  publishRealtimeEvent(makeRealtimeEvent(
    'node.synced',
    'node',
    buildNodeChannels(id),
    {
      node_id: Number(id),
      synced_at: new Date().toISOString(),
    },
    {
      user_id: user.sub,
      resource: {
        kind: 'node',
        id: Number(id),
        name: node.name,
      },
    }
  ))

  return c.json({
    success: true,
    message: 'Node configuration synced',
    data: {
      node_id: parseInt(id),
      synced_at: new Date().toISOString(),
    },
  })
}

/**
 * 批量操作节点
 */
export async function bulkActionHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { node_ids, action } = body as { node_ids: string[]; action: string }

    if (!node_ids || !Array.isArray(node_ids) || node_ids.length === 0) {
      return c.json({ success: false, error: 'node_ids is required and must be a non-empty array' }, 400)
    }

    const validActions = ['start', 'stop', 'restart', 'delete']
    if (!validActions.includes(action)) {
      return c.json({ success: false, error: `Invalid action. Must be one of: ${validActions.join(', ')}` }, 400)
    }

    // 获取节点
    const placeholders = node_ids.map(() => '?').join(',')
    const nodes = await c.env.DB
      .prepare(`SELECT id, name, status FROM nodes WHERE id IN (${placeholders})`)
      .bind(...node_ids)
      .all<{ id: number; name: string; status: string }>()

    if (nodes.results.length === 0) {
      return c.json({ success: false, error: 'No nodes found' }, 404)
    }

    const results: Array<{ id: number; success: boolean; error?: string }> = []

    for (const node of nodes.results) {
      try {
        if (action === 'delete') {
          await c.env.DB
            .prepare('DELETE FROM nodes WHERE id = ?')
            .bind(node.id)
            .run()
          publishRealtimeEvent(makeRealtimeEvent(
            'node.deleted',
            'node',
            buildNodeChannels(node.id),
            {
              node_id: node.id,
            },
            {
              user_id: user.sub,
              resource: {
                kind: 'node',
                id: node.id,
                name: node.name,
              },
            }
          ))
        } else {
          // 更新状态
          const newStatus = action === 'stop' ? 'offline' : 'maintenance'
          await c.env.DB
            .prepare('UPDATE nodes SET status = ?, updated_at = datetime(\'now\') WHERE id = ?')
            .bind(newStatus, node.id)
            .run()

          // 异步执行操作
          c.executionCtx.waitUntil(
            simulateNodeOperation(c.env, node.id.toString(), action as 'start' | 'stop' | 'restart')
          )
        }

        results.push({ id: node.id, success: true })
      } catch (err) {
        results.push({ id: node.id, success: false, error: String(err) })
      }
    }

    await logAudit(c, user.sub, `bulk_${action}_nodes`, 'node', {
      node_ids: node_ids,
      action: action,
      results: results,
    })

    return c.json({
      success: true,
      data: {
        action,
        total: node_ids.length,
        successful: results.filter(r => r.success).length,
        failed: results.filter(r => !r.success).length,
        results,
      },
    })
  } catch (err) {
    console.error('Bulk action error:', err)
    return c.json({ success: false, error: 'Failed to perform bulk action' }, 500)
  }
}

// 辅助函数：模拟节点操作
async function simulateNodeOperation(env: Env, nodeId: string, operation: 'start' | 'stop' | 'restart') {
  // 模拟操作延迟
  await new Promise(resolve => setTimeout(resolve, 3000))

  // 根据操作类型设置最终状态
  const finalStatus = operation === 'stop' ? 'offline' : 'online'

  await env.DB
    .prepare('UPDATE nodes SET status = ?, last_seen = datetime(\'now\'), updated_at = datetime(\'now\') WHERE id = ?')
    .bind(finalStatus, nodeId)
    .run()

  // 通过WebSocket广播状态更新
  // TODO: 实现WebSocket广播
}

// 辅助函数：生成模拟日志
function generateMockLogs(status: string): Array<{ timestamp: string; level: string; message: string }> {
  const levels = ['info', 'warn', 'error']
  const messages = [
    { level: 'info', msg: 'Connection established from {ip}' },
    { level: 'info', msg: 'User authentication successful' },
    { level: 'info', msg: 'Configuration reloaded' },
    { level: 'warn', msg: 'High memory usage detected: {percent}%' },
    { level: 'warn', msg: 'Slow response time: {ms}ms' },
    { level: 'error', msg: 'Connection timeout' },
    { level: 'error', msg: 'Failed to bind to port {port}' },
    { level: 'info', msg: 'Server started on port {port}' },
    { level: 'info', msg: 'Health check passed' },
    { level: 'warn', msg: 'Rate limit exceeded for client {ip}' },
  ]

  const logs: Array<{ timestamp: string; level: string; message: string }> = []

  for (let i = 0; i < 50; i++) {
    const template = messages[Math.floor(Math.random() * messages.length)]
    const timestamp = new Date(Date.now() - Math.random() * 86400000).toISOString()

    let message = template.msg
      .replace('{ip}', `192.168.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`)
      .replace('{percent}', Math.floor(70 + Math.random() * 25).toString())
      .replace('{ms}', Math.floor(100 + Math.random() * 900).toString())
      .replace('{port}', Math.floor(Math.random() * 65535).toString())

    logs.push({
      timestamp,
      level: status === 'offline' ? 'error' : template.level,
      message,
    })
  }

  return logs.sort((a, b) => b.timestamp.localeCompare(a.timestamp))
}

