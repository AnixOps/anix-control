/**
 * WebSocket handler for Cloudflare Workers
 * Unified realtime transport adapter
 */

import type { Context } from 'hono'
import type {
  ApiErrorResponse,
  Env,
  RealtimeWebSocketBroadcastMessage,
  RealtimeWebSocketErrorMessage,
  RealtimeWebSocketInboundMessage,
  RealtimeWebSocketOutboundMessage,
  RealtimeWebSocketPongMessage,
  RealtimeWebSocketSubscribedMessage,
  RealtimeWebSocketUnsubscribedMessage,
} from '../types'
import {
  buildDefaultChannels,
  createRealtimeEvent,
  getRealtimeConnectionSnapshot,
  getRealtimeStats,
  isAllowedRealtimeChannel,
  registerRealtimeClient,
  resetRealtimeState,
  serializeWebSocketEvent,
  unregisterRealtimeClient,
  updateRealtimeUserChannel,
} from '../services/realtime'

type RealtimeWebSocketReceivedMessage = RealtimeWebSocketInboundMessage | { type: string; payload?: unknown }

function isRealtimeWebSocketInboundMessage(data: unknown): data is RealtimeWebSocketInboundMessage {
  if (!data || typeof data !== 'object' || !('type' in data)) {
    return false
  }

  const message = data as { type?: unknown }
  return typeof message.type === 'string'
}

/**
 * WebSocket upgrade handler
 */
export async function websocketHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  if (!user) {
    return c.json({ success: false, error: 'Unauthorized' } as ApiErrorResponse, 401)
  }

  const upgradeHeader = c.req.header('Upgrade')
  if (upgradeHeader?.toLowerCase() !== 'websocket') {
    return c.json({ success: false, error: 'Expected WebSocket upgrade' } as ApiErrorResponse, 426)
  }

  const tenant = c.get('tenant') as { id: number } | undefined
  const channels = buildDefaultChannels(user, tenant?.id)
  const pair = new WebSocketPair()
  const [client, server] = [pair[0], pair[1]]
  const clientId = crypto.randomUUID()

  server.accept()

  registerRealtimeClient({
    id: clientId,
    userId: user.sub,
    email: user.email,
    role: user.role,
    tenantId: tenant?.id,
    transport: 'websocket',
    channels,
    send: (message) => {
      try {
        server.send(message)
      } catch {
        unregisterRealtimeClient(clientId)
      }
    },
  })

  server.send(serializeWebSocketEvent(createRealtimeEvent({
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
  })))

  let heartbeatInterval: number | null = null
  try {
    heartbeatInterval = setInterval(() => {
      try {
        sendWebSocketMessage(server, { type: 'ping' })
      } catch {
        if (heartbeatInterval) {
          clearInterval(heartbeatInterval)
        }
      }
    }, 30000) as unknown as number
  } catch {
    // ignore interval setup failures
  }

  server.addEventListener('message', (event: MessageEvent) => {
    const data = parseWebSocketMessage(event.data as string)
    if (!data) {
      sendWebSocketMessage(server, { type: 'error', payload: 'Invalid message format' })
      return
    }

    handleWebSocketMessage(server, clientId, user.sub, data, tenant?.id)
  })

  const cleanup = () => {
    if (heartbeatInterval) {
      clearInterval(heartbeatInterval)
    }
    unregisterRealtimeClient(clientId)
  }

  server.addEventListener('close', cleanup)
  server.addEventListener('error', cleanup)

  return new Response(null, {
    status: 101,
    webSocket: client,
  })
}

function parseWebSocketMessage(data: string): RealtimeWebSocketReceivedMessage | null {
  try {
    const parsed = JSON.parse(data) as unknown
    return isRealtimeWebSocketInboundMessage(parsed) ? parsed : null
  } catch {
    return null
  }
}

function sendWebSocketMessage(ws: WebSocket, message: RealtimeWebSocketOutboundMessage) {
  ws.send(JSON.stringify(message))
}

function handleWebSocketMessage(
  ws: WebSocket,
  clientId: string,
  userId: number,
  data: RealtimeWebSocketReceivedMessage,
  tenantId?: number
) {
  switch (data.type) {
    case 'pong':
      break

    case 'ping':
      sendWebSocketMessage(ws, { type: 'pong' } as RealtimeWebSocketPongMessage)
      break

    case 'subscribe': {
      if (typeof data.payload !== 'string') {
        sendWebSocketMessage(ws, { type: 'error', payload: 'Invalid subscription payload' } as RealtimeWebSocketErrorMessage)
        break
      }

      const channel = data.payload.trim()
      if (!isAllowedRealtimeChannel({ sub: userId, role: 'viewer' }, channel, tenantId)) {
        sendWebSocketMessage(ws, { type: 'error', payload: `Invalid channel: ${channel}` } as RealtimeWebSocketErrorMessage)
        break
      }

      const changed = updateRealtimeUserChannel(userId, channel, 'subscribe')
      sendWebSocketMessage(ws, {
        type: 'subscribed',
        payload: {
          channel,
          changed,
        },
      } as RealtimeWebSocketSubscribedMessage)
      break
    }

    case 'unsubscribe': {
      if (typeof data.payload !== 'string') {
        sendWebSocketMessage(ws, { type: 'error', payload: 'Invalid subscription payload' } as RealtimeWebSocketErrorMessage)
        break
      }

      const channel = data.payload.trim()
      const changed = updateRealtimeUserChannel(userId, channel, 'unsubscribe')
      sendWebSocketMessage(ws, {
        type: 'unsubscribed',
        payload: {
          channel,
          changed,
        },
      } as RealtimeWebSocketUnsubscribedMessage)
      break
    }

    case 'broadcast': {
      if (data.payload && typeof data.payload === 'object') {
        sendWebSocketMessage(ws, {
          type: 'message',
          payload: {
            ...data.payload as Record<string, unknown>,
            fromUserId: userId,
            clientId,
          },
          timestamp: new Date().toISOString(),
        } as RealtimeWebSocketBroadcastMessage)
      }
      break
    }

    default:
      sendWebSocketMessage(ws, { type: 'error', payload: `Unknown message type: ${data.type}` } as RealtimeWebSocketErrorMessage)
  }
}

/**
 * WebSocket status endpoint helper
 */
export function getWebSocketStats(): { status: string; data: ReturnType<typeof getRealtimeStats>; connections: ReturnType<typeof getRealtimeConnectionSnapshot> } {
  return {
    status: 'live',
    data: getRealtimeStats(),
    connections: getRealtimeConnectionSnapshot(),
  }
}

export { resetRealtimeState }
