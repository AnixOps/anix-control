import type { Context } from 'hono'
import { z } from 'zod'
import type {
  Env,
  Notification,
  NotificationCreateResponse,
  NotificationListResponse,
  NotificationUnreadCountResponse,
  ApiErrorResponse,
  ApiMessageResponse,
  SchemaValidationErrorResponse,
} from '../types'
import { logAudit } from '../utils/audit'
import { buildChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

const createNotificationSchema = z.object({
  title: z.string().min(1).max(200),
  message: z.string().min(1),
  type: z.enum(['info', 'warning', 'error', 'success', 'task', 'system']).default('info'),
  user_id: z.number().int().optional(),
  resource_type: z.string().optional(),
  resource_id: z.string().optional(),
  action_url: z.string().optional(),
})

/**
 * 获取通知列表
 */
export async function listNotificationsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const page = parseInt(c.req.query('page') || '1', 10)
  const perPage = parseInt(c.req.query('per_page') || '20', 10)
  const unreadOnly = c.req.query('unread_only') === 'true'
  const type = c.req.query('type')

  let sql = 'SELECT * FROM notifications WHERE user_id = ?'
  const params: (string | number)[] = [user.sub]

  if (unreadOnly) {
    sql += ' AND read = 0'
  }

  if (type) {
    sql += ' AND type = ?'
    params.push(type)
  }

  // Count
  const countResult = await c.env.DB
    .prepare(`SELECT COUNT(*) as total FROM (${sql})`)
    .bind(...params)
    .first<{ total: number }>()

  // Paginated results
  sql += ' ORDER BY created_at DESC LIMIT ? OFFSET ?'
  params.push(perPage, (page - 1) * perPage)

  const result = await c.env.DB
    .prepare(sql)
    .bind(...params)
    .all()

  // Get unread count
  const unreadResult = await c.env.DB
    .prepare('SELECT COUNT(*) as count FROM notifications WHERE user_id = ? AND read = 0')
    .bind(user.sub)
    .first<{ count: number }>()

  return c.json({
    success: true,
    data: {
      items: result.results as unknown as Notification[],
      total: countResult?.total || 0,
      page,
      per_page: perPage,
      total_pages: Math.ceil((countResult?.total || 0) / perPage),
      unread_count: unreadResult?.count || 0,
    },
  } as NotificationListResponse)
}

/**
 * 标记通知为已读
 */
export async function markNotificationReadHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const notificationId = c.req.param('id') as string

  const result = await c.env.DB
    .prepare('UPDATE notifications SET read = 1 WHERE id = ? AND user_id = ?')
    .bind(notificationId, user.sub)
    .run()

  if (result.meta.changes === 0) {
    return c.json({ success: false, error: 'Notification not found' }, 404)
  }

  return c.json({
    success: true,
    message: 'Notification marked as read',
  } as ApiMessageResponse)
}

/**
 * 标记所有通知为已读
 */
export async function markAllNotificationsReadHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  await c.env.DB
    .prepare('UPDATE notifications SET read = 1 WHERE user_id = ? AND read = 0')
    .bind(user.sub)
    .run()

  publishRealtimeEvent(makeRealtimeEvent(
    'notification.read_all',
    'user',
    buildChannels('global', 'notifications', `user:${user.sub}`),
    {
      user_id: user.sub,
    },
    {
      user_id: user.sub,
    }
  ))

  return c.json({
    success: true,
    message: 'All notifications marked as read',
  } as ApiMessageResponse)
}

/**
 * 删除通知
 */
export async function deleteNotificationHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const notificationId = c.req.param('id') as string

  const result = await c.env.DB
    .prepare('DELETE FROM notifications WHERE id = ? AND user_id = ?')
    .bind(notificationId, user.sub)
    .run()

  if (result.meta.changes === 0) {
    return c.json({ success: false, error: 'Notification not found' }, 404)
  }

  publishRealtimeEvent(makeRealtimeEvent(
    'notification.deleted',
    'user',
    buildChannels('global', 'notifications', `user:${user.sub}`),
    {
      notification_id: notificationId,
      user_id: user.sub,
    },
    {
      user_id: user.sub,
    }
  ))

  return c.json({
    success: true,
    message: 'Notification deleted',
  } as ApiMessageResponse)
}

/**
 * 创建通知
 */
export async function createNotificationHandler(c: Context<{ Bindings: Env }>) {
  const currentUser = c.get('user')

  try {
    const body = await c.req.json()
    const data = createNotificationSchema.parse(body)

    const targetUserId = data.user_id || currentUser.sub

    const result = await c.env.DB
      .prepare(`
        INSERT INTO notifications (user_id, type, title, message, resource_type, resource_id, action_url)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        RETURNING *
      `)
      .bind(
        targetUserId,
        data.type,
        data.title,
        data.message,
        data.resource_type || null,
        data.resource_id || null,
        data.action_url || null
      )
      .first()

    await logAudit(c, currentUser.sub, 'create_notification', 'notification', { title: data.title })

    publishRealtimeEvent(makeRealtimeEvent(
      'notification.created',
      'user',
      buildChannels('global', 'notifications', `user:${targetUserId}`),
      {
        notification_id: result?.id,
        user_id: targetUserId,
        type: data.type,
        title: data.title,
        message: data.message,
      },
      {
        user_id: targetUserId,
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
 * 获取未读通知数量
 */
export async function getUnreadCountHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const result = await c.env.DB
    .prepare('SELECT COUNT(*) as count FROM notifications WHERE user_id = ? AND read = 0')
    .bind(user.sub)
    .first<{ count: number }>()

  return c.json({
    success: true,
    data: {
      unread_count: result?.count || 0,
    },
  } as NotificationUnreadCountResponse)
}

/**
 * 批量创建通知（内部使用）
 */
export async function createNotificationForUsers(
  env: Env,
  userIds: number[],
  notification: {
    type: 'info' | 'warning' | 'error' | 'success' | 'task' | 'system'
    title: string
    message: string
    resource_type?: string
    resource_id?: string
    action_url?: string
  }
) {
  for (const userId of userIds) {
    await env.DB
      .prepare(`
        INSERT INTO notifications (user_id, type, title, message, resource_type, resource_id, action_url)
        VALUES (?, ?, ?, ?, ?, ?, ?)
      `)
      .bind(
        userId,
        notification.type,
        notification.title,
        notification.message,
        notification.resource_type || null,
        notification.resource_id || null,
        notification.action_url || null
      )
      .run()
  }
}