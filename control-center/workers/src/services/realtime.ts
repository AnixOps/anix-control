import type { AuthPrincipal, RealtimeEvent, RealtimeScope } from '../types'

export type RealtimeTransport = 'sse' | 'websocket'

export interface RealtimeClient {
  id: string
  userId: number
  email: string
  role: string
  tenantId?: number
  transport: RealtimeTransport
  channels: Set<string>
  send: (message: string) => void
}

export interface RealtimeClientRegistration {
  id?: string
  userId: number
  email: string
  role: string
  tenantId?: number
  transport: RealtimeTransport
  channels?: string[]
  send: (message: string) => void
}

export interface RealtimeStats {
  total_connections: number
  sse_connections: number
  websocket_connections: number
  channels: Record<string, number>
  recent_events: number
}

const clients = new Map<string, RealtimeClient>()
const channelIndex = new Map<string, Set<string>>()
const recentEvents: RealtimeEvent[] = []
const MAX_RECENT_EVENTS = 100

const PUBLIC_CHANNELS = new Set([
  'global',
  'nodes',
  'tasks',
  'logs',
  'notifications',
  'audit',
  'operations',
])

function dedupeChannels(channels: Array<string | undefined | null>): string[] {
  return Array.from(new Set(channels.filter((channel): channel is string => !!channel && channel.trim().length > 0)))
}

function addToChannelIndex(clientId: string, channel: string) {
  let clientsForChannel = channelIndex.get(channel)
  if (!clientsForChannel) {
    clientsForChannel = new Set()
    channelIndex.set(channel, clientsForChannel)
  }

  clientsForChannel.add(clientId)
}

function removeFromChannelIndex(clientId: string, channel: string) {
  const clientsForChannel = channelIndex.get(channel)
  if (!clientsForChannel) {
    return
  }

  clientsForChannel.delete(clientId)
  if (clientsForChannel.size === 0) {
    channelIndex.delete(channel)
  }
}

export function buildChannels(...channels: Array<string | undefined | null>): string[] {
  return dedupeChannels(channels)
}

export function buildDefaultChannels(user: Pick<AuthPrincipal, 'sub'>, tenantId?: number): string[] {
  return buildChannels('global', `user:${user.sub}`, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildAuditChannels(userId?: number, tenantId?: number): string[] {
  return buildChannels('global', 'audit', 'operations', userId ? `user:${userId}` : undefined, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildTaskChannels(taskId: string, userId?: number, tenantId?: number): string[] {
  return buildChannels('global', 'tasks', 'operations', `task:${taskId}`, userId ? `user:${userId}` : undefined, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildNotificationChannels(userId: number, tenantId?: number): string[] {
  return buildChannels('global', 'notifications', `user:${userId}`, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildNodeChannels(nodeId: number | string, tenantId?: number): string[] {
  return buildChannels('global', 'nodes', 'operations', `node:${nodeId}`, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildAgentChannels(nodeId: number | string, tenantId?: number): string[] {
  return buildChannels('global', 'nodes', 'operations', `node:${nodeId}`, tenantId ? `tenant:${tenantId}` : undefined)
}

export function buildIncidentChannels(incidentId: string, userId?: number, tenantId?: number): string[] {
  return buildChannels('global', 'operations', `incident:${incidentId}`, userId ? `user:${userId}` : undefined, tenantId ? `tenant:${tenantId}` : undefined)
}

export function createRealtimeEvent<T>(event: Omit<RealtimeEvent<T>, 'id' | 'timestamp' | 'version'>): RealtimeEvent<T> {
  return {
    ...event,
    id: crypto.randomUUID(),
    timestamp: new Date().toISOString(),
    version: 1,
  }
}

export function serializeSseEvent(event: RealtimeEvent): string {
  return `data: ${JSON.stringify(event)}\n\n`
}

export function serializeWebSocketEvent(event: RealtimeEvent): string {
  return JSON.stringify(event)
}

export function registerRealtimeClient(registration: RealtimeClientRegistration): RealtimeClient {
  const clientId = registration.id || crypto.randomUUID()
  const client: RealtimeClient = {
    id: clientId,
    userId: registration.userId,
    email: registration.email,
    role: registration.role,
    tenantId: registration.tenantId,
    transport: registration.transport,
    channels: new Set(buildChannels(...(registration.channels || buildDefaultChannels({ sub: registration.userId })))),
    send: registration.send,
  }

  clients.set(client.id, client)
  for (const channel of client.channels) {
    addToChannelIndex(client.id, channel)
  }

  return client
}

export function unregisterRealtimeClient(clientId: string): void {
  const client = clients.get(clientId)
  if (!client) {
    return
  }

  for (const channel of client.channels) {
    removeFromChannelIndex(client.id, channel)
  }

  clients.delete(client.id)
}

export function getRealtimeClient(clientId: string): RealtimeClient | undefined {
  return clients.get(clientId)
}

export function getRealtimeClientsByUser(userId: number): RealtimeClient[] {
  return Array.from(clients.values()).filter(client => client.userId === userId)
}

export function updateRealtimeClientChannel(clientId: string, channel: string, action: 'subscribe' | 'unsubscribe'): boolean {
  const client = clients.get(clientId)
  if (!client) {
    return false
  }

  if (action === 'subscribe') {
    if (client.channels.has(channel)) {
      return false
    }

    client.channels.add(channel)
    addToChannelIndex(client.id, channel)
    return true
  }

  if (!client.channels.has(channel)) {
    return false
  }

  client.channels.delete(channel)
  removeFromChannelIndex(client.id, channel)
  return true
}

export function updateRealtimeUserChannel(userId: number, channel: string, action: 'subscribe' | 'unsubscribe'): number {
  let changed = 0

  for (const client of getRealtimeClientsByUser(userId)) {
    if (updateRealtimeClientChannel(client.id, channel, action)) {
      changed += 1
    }
  }

  return changed
}

export function isAllowedRealtimeChannel(user: Pick<AuthPrincipal, 'sub' | 'role'>, channel: string, tenantId?: number): boolean {
  if (PUBLIC_CHANNELS.has(channel)) {
    return true
  }

  if (channel === `user:${user.sub}`) {
    return true
  }

  if (tenantId && channel === `tenant:${tenantId}`) {
    return true
  }

  if (/^task:[A-Za-z0-9_-]+$/.test(channel)) {
    return true
  }

  if (/^incident:[A-Za-z0-9_-]+$/.test(channel)) {
    return true
  }

  return false
}

export function publishRealtimeEvent(event: RealtimeEvent): void {
  recentEvents.push(event)
  if (recentEvents.length > MAX_RECENT_EVENTS) {
    recentEvents.splice(0, recentEvents.length - MAX_RECENT_EVENTS)
  }

  const recipients = new Set<string>()
  for (const channel of event.channels) {
    const clientsForChannel = channelIndex.get(channel)
    if (!clientsForChannel) {
      continue
    }

    for (const clientId of clientsForChannel) {
      recipients.add(clientId)
    }
  }

  for (const clientId of recipients) {
    const client = clients.get(clientId)
    if (!client) {
      continue
    }

    const payload = client.transport === 'sse'
      ? serializeSseEvent(event)
      : serializeWebSocketEvent(event)

    try {
      client.send(payload)
    } catch {
      unregisterRealtimeClient(clientId)
    }
  }
}

export function getRecentRealtimeEvents(options: {
  channel?: string
  userId?: number
  tenantId?: number
  limit?: number
} = {}): RealtimeEvent[] {
  const limit = Math.max(1, Math.min(options.limit ?? 50, 100))
  const { channel, userId, tenantId } = options

  return recentEvents
    .filter((event) => {
      if (channel && !event.channels.includes(channel)) {
        return false
      }

      if (userId && !event.channels.includes(`user:${userId}`) && event.user_id !== userId) {
        return false
      }

      if (tenantId && !event.channels.includes(`tenant:${tenantId}`) && event.tenant_id !== tenantId) {
        return false
      }

      return true
    })
    .slice(-limit)
}

export function getRealtimeStats(): RealtimeStats {
  const channels: Record<string, number> = {}
  for (const [channel, clientIds] of channelIndex.entries()) {
    channels[channel] = clientIds.size
  }

  let sseConnections = 0
  let websocketConnections = 0
  for (const client of clients.values()) {
    if (client.transport === 'sse') {
      sseConnections += 1
    } else {
      websocketConnections += 1
    }
  }

  return {
    total_connections: clients.size,
    sse_connections: sseConnections,
    websocket_connections: websocketConnections,
    channels,
    recent_events: recentEvents.length,
  }
}

export function getRealtimeConnectionSnapshot(userId?: number) {
  return Array.from(clients.values())
    .filter(client => !userId || client.userId === userId)
    .map(client => ({
      id: client.id,
      user_id: client.userId,
      email: client.email,
      role: client.role,
      tenant_id: client.tenantId,
      transport: client.transport,
      channels: Array.from(client.channels),
    }))
}

export function getRealtimeChannelCount(channel: string): number {
  return channelIndex.get(channel)?.size || 0
}

export function makeRealtimeEvent<T>(
  type: string,
  scope: RealtimeScope,
  channels: string[],
  payload: T,
  extras: {
    actor?: RealtimeEvent<T>['actor']
    resource?: RealtimeEvent<T>['resource']
    tenant_id?: number
    user_id?: number
    correlation_id?: string
  } = {}
): RealtimeEvent<T> {
  return createRealtimeEvent({
    type,
    scope,
    channels: buildChannels(...channels),
    payload,
    actor: extras.actor,
    resource: extras.resource,
    tenant_id: extras.tenant_id,
    user_id: extras.user_id,
    correlation_id: extras.correlation_id,
  })
}

export function resetRealtimeState(): void {
  clients.clear()
  channelIndex.clear()
  recentEvents.length = 0
}
