import type { Context } from 'hono'
import { z } from 'zod'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'

const createGroupSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().optional(),
  parent_id: z.number().int().optional(),
})

const updateGroupSchema = z.object({
  name: z.string().min(1).max(100).optional(),
  description: z.string().optional(),
  parent_id: z.number().int().nullable().optional(),
})

/**
 * 获取节点组列表
 */
export async function listNodeGroupsHandler(c: Context<{ Bindings: Env }>) {
  const groups = await c.env.DB
    .prepare(`
      SELECT g.*,
             (SELECT COUNT(*) FROM nodes WHERE config LIKE '%' || g.name || '%') as node_count,
             p.name as parent_name
      FROM node_groups g
      LEFT JOIN node_groups p ON g.parent_id = p.id
      ORDER BY g.name
    `)
    .all()

  return c.json({
    success: true,
    data: groups.results,
  })
}

/**
 * 获取单个节点组
 */
export async function getNodeGroupHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const group = await c.env.DB
    .prepare(`
      SELECT g.*,
             p.name as parent_name
      FROM node_groups g
      LEFT JOIN node_groups p ON g.parent_id = p.id
      WHERE g.id = ?
    `)
    .bind(id)
    .first()

  if (!group) {
    return c.json({ success: false, error: 'Node group not found' }, 404)
  }

  // 获取组内节点
  // 注意：这里简化处理，实际应该有 node_group_id 字段
  const nodes = await c.env.DB
    .prepare('SELECT id, name, host, status FROM nodes')
    .all()

  return c.json({
    success: true,
    data: {
      ...group,
      nodes: nodes.results,
    },
  })
}

/**
 * 创建节点组
 */
export async function createNodeGroupHandler(c: Context<{ Bindings: Env }>) {
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = createGroupSchema.parse(body)

    // 检查名称是否已存在
    const existing = await c.env.DB
      .prepare('SELECT id FROM node_groups WHERE name = ?')
      .bind(data.name)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Group name already exists' }, 409)
    }

    // 验证父组存在
    if (data.parent_id) {
      const parent = await c.env.DB
        .prepare('SELECT id FROM node_groups WHERE id = ?')
        .bind(data.parent_id)
        .first()

      if (!parent) {
        return c.json({ success: false, error: 'Parent group not found' }, 404)
      }
    }

    const result = await c.env.DB
      .prepare(`
        INSERT INTO node_groups (name, description, parent_id)
        VALUES (?, ?, ?)
        RETURNING *
      `)
      .bind(data.name, data.description || null, data.parent_id || null)
      .first()

    await logAudit(c, currentUser.sub, 'create_node_group', 'node_group', {
      group_id: (result as { id: number })?.id,
      name: data.name,
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
 * 更新节点组
 */
export async function updateNodeGroupHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = updateGroupSchema.parse(body)

    const existing = await c.env.DB
      .prepare('SELECT * FROM node_groups WHERE id = ?')
      .bind(id)
      .first()

    if (!existing) {
      return c.json({ success: false, error: 'Node group not found' }, 404)
    }

    const updates: string[] = []
    const values: (string | number | null)[] = []

    if (data.name) {
      // 检查名称是否被其他组使用
      const nameCheck = await c.env.DB
        .prepare('SELECT id FROM node_groups WHERE name = ? AND id != ?')
        .bind(data.name, id)
        .first()

      if (nameCheck) {
        return c.json({ success: false, error: 'Group name already in use' }, 409)
      }

      updates.push('name = ?')
      values.push(data.name)
    }

    if (data.description !== undefined) {
      updates.push('description = ?')
      values.push(data.description)
    }

    if (data.parent_id !== undefined) {
      // 不能将自己设为父组
      if (data.parent_id === parseInt(id, 10)) {
        return c.json({ success: false, error: 'Cannot set self as parent' }, 400)
      }

      updates.push('parent_id = ?')
      values.push(data.parent_id)
    }

    if (updates.length === 0) {
      return c.json({ success: false, error: 'No fields to update' }, 400)
    }

    values.push(id)

    const result = await c.env.DB
      .prepare(`UPDATE node_groups SET ${updates.join(', ')} WHERE id = ? RETURNING *`)
      .bind(...values)
      .first()

    await logAudit(c, currentUser.sub, 'update_node_group', 'node_group', { group_id: id })

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
 * 删除节点组
 */
export async function deleteNodeGroupHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  // 检查是否有子组
  const childGroups = await c.env.DB
    .prepare('SELECT id FROM node_groups WHERE parent_id = ?')
    .bind(id)
    .first()

  if (childGroups) {
    return c.json({ success: false, error: 'Cannot delete group with child groups' }, 400)
  }

  const result = await c.env.DB
    .prepare('DELETE FROM node_groups WHERE id = ? RETURNING id, name')
    .bind(id)
    .first()

  if (!result) {
    return c.json({ success: false, error: 'Node group not found' }, 404)
  }

  await logAudit(c, currentUser.sub, 'delete_node_group', 'node_group', { group_id: id })

  return c.json({
    success: true,
    message: 'Node group deleted successfully',
  })
}

/**
 * 添加节点到组
 */
export async function addNodesToGroupHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  const body = await c.req.json<{ node_ids: number[] }>()
  const { node_ids } = body

  if (!node_ids || !Array.isArray(node_ids) || node_ids.length === 0) {
    return c.json({ success: false, error: 'node_ids is required' }, 400)
  }

  // 验证组存在
  const group = await c.env.DB
    .prepare('SELECT id, name FROM node_groups WHERE id = ?')
    .bind(id)
    .first<{ id: number; name: string }>()

  if (!group) {
    return c.json({ success: false, error: 'Node group not found' }, 404)
  }

  // 更新节点的 config 字段，添加 group 信息
  for (const nodeId of node_ids) {
    const node = await c.env.DB
      .prepare('SELECT id, config FROM nodes WHERE id = ?')
      .bind(nodeId)
      .first<{ id: number; config: string | null }>()

    if (node) {
      const config = node.config ? JSON.parse(node.config) : {}
      if (!config.groups) config.groups = []
      if (!config.groups.includes(group.name)) {
        config.groups.push(group.name)
      }

      await c.env.DB
        .prepare('UPDATE nodes SET config = ?, updated_at = datetime(\'now\') WHERE id = ?')
        .bind(JSON.stringify(config), nodeId)
        .run()
    }
  }

  await logAudit(c, currentUser.sub, 'add_nodes_to_group', 'node_group', {
    group_id: id,
    nodes: node_ids,
  })

  return c.json({
    success: true,
    message: 'Nodes added to group successfully',
  })
}

/**
 * 从组中移除节点
 */
export async function removeNodesFromGroupHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const currentUser = c.get('user')

  const body = await c.req.json<{ node_ids: number[] }>()
  const { node_ids } = body

  if (!node_ids || !Array.isArray(node_ids) || node_ids.length === 0) {
    return c.json({ success: false, error: 'node_ids is required' }, 400)
  }

  // 验证组存在
  const group = await c.env.DB
    .prepare('SELECT id, name FROM node_groups WHERE id = ?')
    .bind(id)
    .first<{ id: number; name: string }>()

  if (!group) {
    return c.json({ success: false, error: 'Node group not found' }, 404)
  }

  // 从节点配置中移除组
  for (const nodeId of node_ids) {
    const node = await c.env.DB
      .prepare('SELECT id, config FROM nodes WHERE id = ?')
      .bind(nodeId)
      .first<{ id: number; config: string | null }>()

    if (node && node.config) {
      const config = JSON.parse(node.config)
      if (config.groups) {
        config.groups = config.groups.filter((g: string) => g !== group.name)
      }

      await c.env.DB
        .prepare('UPDATE nodes SET config = ?, updated_at = datetime(\'now\') WHERE id = ?')
        .bind(JSON.stringify(config), nodeId)
        .run()
    }
  }

  await logAudit(c, currentUser.sub, 'remove_nodes_from_group', 'node_group', {
    group_id: id,
    nodes: node_ids,
  })

  return c.json({
    success: true,
    message: 'Nodes removed from group successfully',
  })
}