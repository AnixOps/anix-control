/**
 * Server-Sent Events (SSE) Handler
 * Unified realtime transport adapter
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import {
  buildDefaultChannels,
  createRealtimeEvent,
  getRealtimeConnectionSnapshot,
  getRealtimeStats,
  isAllowedRealtimeChannel,
  registerRealtimeClient,
  resetRealtimeState,
  serializeSseEvent,
  unregisterRealtimeClient,
  updateRealtimeUserChannel,
} from '../services/realtime'

/**
 * SSE connection handler
 */
export async function sseHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  if (!user) {
    return c.json({ success: false, error: 'Unauthorized' }, 401)
  }

  const tenant = c.get('tenant') as { id: number } | undefined
  const clientId = crypto.randomUUID()
  const channels = buildDefaultChannels(user, tenant?.id)
  const encoder = new TextEncoder()
  let controllerRef: ReadableStreamDefaultController<Uint8Array> | null = null
  let heartbeatInterval: number | null = null

  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controllerRef = controller

      registerRealtimeClient({
        id: clientId,
        userId: user.sub,
        email: user.email,
        role: user.role,
        tenantId: tenant?.id,
        transport: 'sse',
        channels,
        send: (message) => {
          if (!controllerRef) return
          try {
            controllerRef.enqueue(encoder.encode(message))
          } catch {
            // ignore send errors; cancel() will clean up the client
          }
        },
      })

      const connectedMessage = createRealtimeEvent({
        type: 'connected',
        scope: 'system',
        channels,
        user_id: user.sub,
        tenant_id: tenant?.id,
        payload: {
          client_id: clientId,
          user_id: user.sub,
          email: user.email,
          role: user.role,
          channels,
        },
      })

      controller.enqueue(encoder.encode(serializeSseEvent(connectedMessage)))

      heartbeatInterval = setInterval(() => {
        try {
          controller.enqueue(encoder.encode(': heartbeat\n\n'))
        } catch {
          if (heartbeatInterval) {
            clearInterval(heartbeatInterval)
          }
        }
      }, 30000) as unknown as number
    },
    cancel() {
      if (heartbeatInterval) {
        clearInterval(heartbeatInterval)
      }
      unregisterRealtimeClient(clientId)
      controllerRef = null
    },
  })

  return new Response(stream, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
      'X-Accel-Buffering': 'no',
    },
  })
}

/**
 * SSE subscribe endpoint
 */
export async function sseSubscribeHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenant = c.get('tenant') as { id: number } | undefined
  const body = await c.req.json<{ channel?: string }>()
  const channel = body.channel?.trim()

  if (!channel) {
    return c.json({ success: false, error: 'Channel is required' }, 400)
  }

  if (!isAllowedRealtimeChannel(user, channel, tenant?.id)) {
    return c.json({ success: false, error: 'Invalid channel' }, 403)
  }

  const changed = updateRealtimeUserChannel(user.sub, channel, 'subscribe')

  return c.json({
    success: true,
    message: changed > 0 ? `Subscribed to channel: ${channel}` : `Already subscribed to channel: ${channel}`,
  })
}

/**
 * SSE unsubscribe endpoint
 */
export async function sseUnsubscribeHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json<{ channel?: string }>()
  const channel = body.channel?.trim()

  if (!channel) {
    return c.json({ success: false, error: 'Channel is required' }, 400)
  }

  const changed = updateRealtimeUserChannel(user.sub, channel, 'unsubscribe')

  return c.json({
    success: true,
    message: changed > 0 ? `Unsubscribed from channel: ${channel}` : `Channel not active: ${channel}`,
  })
}

/**
 * SSE realtime status endpoint
 */
export async function sseStatusHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const connections = c.get('user')?.role === 'admin'
    ? getRealtimeConnectionSnapshot()
    : getRealtimeConnectionSnapshot(user.sub)

  return c.json({
    success: true,
    data: {
      connections,
      total: connections.length,
      stats: getRealtimeStats(),
      message: 'Realtime event center is live',
    },
  })
}

/**
 * Reset helper for tests
 */
export { resetRealtimeState }
