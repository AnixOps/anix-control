import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { websocketHandler } from './websocket'
import type {
  RealtimeWebSocketBroadcastMessage,
  RealtimeWebSocketConnectedMessage,
  RealtimeWebSocketErrorMessage,
  RealtimeWebSocketOutboundMessage,
  RealtimeWebSocketPongMessage,
  RealtimeWebSocketSubscribedMessage,
  RealtimeWebSocketUnsubscribedMessage,
} from '../types'

type MockContext = {
  get: (key: string) => any
  req: {
    header: (name: string) => string | undefined
  }
  json: (data: unknown, status?: number) => Response
}

class FakeWebSocket {
  sent: string[] = []
  listeners: Record<string, Array<(event?: any) => void>> = {}

  accept() {}

  send(message: string) {
    this.sent.push(message)
  }

  addEventListener(type: string, handler: (event?: any) => void) {
    if (!this.listeners[type]) this.listeners[type] = []
    this.listeners[type].push(handler)
  }

  emit(type: string, event?: any) {
    for (const handler of this.listeners[type] || []) {
      handler(event)
    }
  }
}

// Helper to create a mock WebSocketPair class
function createWebSocketPairMock(client: FakeWebSocket, server: FakeWebSocket) {
  return class {
    0 = client
    1 = server
    constructor() {
      return [client, server] as unknown as this
    }
  }
}

describe('websocketHandler', () => {
  let originalWebSocketPair: any

  beforeEach(() => {
    vi.spyOn(globalThis, 'setInterval').mockReturnValue(1 as any)
    vi.spyOn(globalThis, 'clearInterval').mockImplementation(() => undefined)
    originalWebSocketPair = (globalThis as any).WebSocketPair
  })

  afterEach(() => {
    vi.restoreAllMocks()
    if (originalWebSocketPair === undefined) {
      delete (globalThis as any).WebSocketPair
    } else {
      ;(globalThis as any).WebSocketPair = originalWebSocketPair
    }
  })

  function createContext(user: any, upgradeHeader?: string): MockContext {
    return {
      get: (key: string) => (key === 'user' ? user : undefined),
      req: {
        header: (name: string) => (name === 'Upgrade' ? upgradeHeader : undefined),
      },
      json: (data: unknown, status = 200) =>
        new Response(JSON.stringify(data), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
    }
  }

  function parseMessage<T extends RealtimeWebSocketOutboundMessage>(message: string): T {
    return JSON.parse(message) as T
  }

  it('returns 401 when user is missing', async () => {
    const c = createContext(null)
    const response = await websocketHandler(c as any)

    expect(response.status).toBe(401)
    const body = await response.json() as { error: string }
    expect(body.error).toBe('Unauthorized')
  })

  it('returns 426 when upgrade header is missing', async () => {
    const c = createContext({ sub: 1 }, undefined)
    const response = await websocketHandler(c as any)

    expect(response.status).toBe(426)
    const body = await response.json() as { error: string }
    expect(body.error).toBe('Expected WebSocket upgrade')
  })

  it('accepts websocket and handles ping/subscribe/unsubscribe/broadcast messages', async () => {
    const client = new FakeWebSocket()
    const server = new FakeWebSocket()

    ;(globalThis as any).WebSocketPair = createWebSocketPairMock(client, server)

    const c = createContext({ sub: 42, email: 'ws@example.com', role: 'admin' }, 'websocket')
    await expect(websocketHandler(c as any)).rejects.toThrow('init["status"] must be in the range of 200 to 599')

    const connected = parseMessage<RealtimeWebSocketConnectedMessage>(server.sent[0])
    expect(connected.type).toBe('connected')
    expect(connected.payload.user_id).toBe(42)
    expect(connected.payload.email).toBe('ws@example.com')
    expect(connected.payload.channels).toContain('global')

    server.emit('message', { data: JSON.stringify({ type: 'ping' }) })
    expect(server.sent.some(message => parseMessage<RealtimeWebSocketPongMessage>(message).type === 'pong')).toBe(true)

    server.emit('message', { data: JSON.stringify({ type: 'subscribe', payload: 'nodes' }) })
    const subscribed = server.sent
      .map(message => parseMessage<RealtimeWebSocketSubscribedMessage | RealtimeWebSocketOutboundMessage>(message))
      .find((message): message is RealtimeWebSocketSubscribedMessage => message.type === 'subscribed')
    expect(subscribed?.payload.channel).toBe('nodes')
    expect(subscribed?.payload.changed).toBe(1)

    server.emit('message', { data: JSON.stringify({ type: 'unsubscribe', payload: 'nodes' }) })
    const unsubscribed = server.sent
      .map(message => parseMessage<RealtimeWebSocketUnsubscribedMessage | RealtimeWebSocketOutboundMessage>(message))
      .find((message): message is RealtimeWebSocketUnsubscribedMessage => message.type === 'unsubscribed')
    expect(unsubscribed?.payload.channel).toBe('nodes')
    expect(unsubscribed?.payload.changed).toBe(1)

    server.emit('message', { data: JSON.stringify({ type: 'broadcast', payload: { message: 'hello', level: 'info' } }) })
    const broadcast = server.sent
      .map(message => parseMessage<RealtimeWebSocketBroadcastMessage | RealtimeWebSocketOutboundMessage>(message))
      .find((message): message is RealtimeWebSocketBroadcastMessage => message.type === 'message')
    expect(broadcast?.payload.message).toBe('hello')
    expect(broadcast?.payload.level).toBe('info')
    expect(broadcast?.payload.fromUserId).toBe(42)
    expect(broadcast?.payload.clientId).toBeDefined()

    server.emit('message', { data: JSON.stringify({ type: 'invalid_type' }) })
    const errorMessage = server.sent
      .map(message => parseMessage<RealtimeWebSocketErrorMessage | RealtimeWebSocketOutboundMessage>(message))
      .find((message): message is RealtimeWebSocketErrorMessage => message.type === 'error' && message.payload.includes('Unknown message type'))
    expect(errorMessage?.payload).toContain('Unknown message type')
  })

  it('returns error for invalid json message payload', async () => {
    const client = new FakeWebSocket()
    const server = new FakeWebSocket()

    ;(globalThis as any).WebSocketPair = createWebSocketPairMock(client, server)

    const c = createContext({ sub: 7, email: 'ws@example.com', role: 'admin' }, 'websocket')
    await expect(websocketHandler(c as any)).rejects.toThrow('init["status"] must be in the range of 200 to 599')

    server.emit('message', { data: '{not-json' })

    expect(server.sent.some(message => {
      const data = parseMessage<RealtimeWebSocketErrorMessage | RealtimeWebSocketOutboundMessage>(message)
      return data.type === 'error' && data.payload === 'Invalid message format'
    })).toBe(true)
  })

  it('clears the heartbeat interval when the socket closes', async () => {
    const client = new FakeWebSocket()
    const server = new FakeWebSocket()

    ;(globalThis as any).WebSocketPair = createWebSocketPairMock(client, server)

    const c = createContext({ sub: 11, email: 'ws@example.com', role: 'admin' }, 'websocket')
    await expect(websocketHandler(c as any)).rejects.toThrow('init["status"] must be in the range of 200 to 599')

    server.emit('close')

    expect(globalThis.clearInterval).toHaveBeenCalledWith(1)
  })

  it('clears the heartbeat interval when the socket errors', async () => {
    const client = new FakeWebSocket()
    const server = new FakeWebSocket()

    ;(globalThis as any).WebSocketPair = createWebSocketPairMock(client, server)

    const c = createContext({ sub: 12, email: 'ws@example.com', role: 'admin' }, 'websocket')
    await expect(websocketHandler(c as any)).rejects.toThrow('init["status"] must be in the range of 200 to 599')

    server.emit('error')

    expect(globalThis.clearInterval).toHaveBeenCalledWith(1)
  })
})
