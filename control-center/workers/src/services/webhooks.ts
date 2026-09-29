import type {
  AuthPrincipal,
  Env,
  IncidentRecord,
  WebhookDelivery,
  WebhookEndpoint,
  WebhookEventType,
} from '../types'

const WEBHOOK_PREFIX = 'webhook:'
const WEBHOOK_INDEX_KEY = 'webhook:index'
const DELIVERY_PREFIX = 'webhook:delivery:'
const DELIVERY_INDEX_PREFIX = 'webhook:deliveries:'

function nowIso(): string {
  return new Date().toISOString()
}

function webhookKey(id: string): string {
  return `${WEBHOOK_PREFIX}${id}`
}

function deliveryKey(deliveryId: string): string {
  return `${DELIVERY_PREFIX}${deliveryId}`
}

function deliveryIndexKey(webhookId: string): string {
  return `${DELIVERY_INDEX_PREFIX}${webhookId}`
}

async function getWebhookIndex(env: Env): Promise<string[]> {
  return (await env.KV.get(WEBHOOK_INDEX_KEY, 'json') as string[] | null) || []
}

async function setWebhookIndex(env: Env, ids: string[]): Promise<void> {
  await env.KV.put(WEBHOOK_INDEX_KEY, JSON.stringify(ids), { expirationTtl: 86400 * 30 })
}

export async function getWebhook(env: Env, id: string): Promise<WebhookEndpoint | null> {
  return await env.KV.get(webhookKey(id), 'json') as WebhookEndpoint | null
}

export async function listWebhooks(env: Env): Promise<WebhookEndpoint[]> {
  const ids = await getWebhookIndex(env)
  const webhooks = await Promise.all(ids.map(id => getWebhook(env, id)))
  return webhooks.filter((w): w is WebhookEndpoint => w !== null)
}

export interface CreateWebhookInput {
  name: string
  url: string
  secret?: string
  events: WebhookEventType[]
  headers?: Record<string, string>
}

export async function createWebhook(
  env: Env,
  principal: AuthPrincipal,
  input: CreateWebhookInput
): Promise<WebhookEndpoint> {
  const id = crypto.randomUUID()
  const now = nowIso()

  const webhook: WebhookEndpoint = {
    id,
    name: input.name,
    url: input.url,
    secret: input.secret,
    events: input.events,
    enabled: true,
    headers: input.headers,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  await env.KV.put(webhookKey(id), JSON.stringify(webhook), { expirationTtl: 86400 * 30 })

  const ids = await getWebhookIndex(env)
  if (!ids.includes(id)) {
    ids.push(id)
    await setWebhookIndex(env, ids)
  }

  return webhook
}

export interface UpdateWebhookInput {
  name?: string
  url?: string
  secret?: string
  events?: WebhookEventType[]
  enabled?: boolean
  headers?: Record<string, string>
}

export async function updateWebhook(
  env: Env,
  id: string,
  input: UpdateWebhookInput
): Promise<WebhookEndpoint | null> {
  const existing = await getWebhook(env, id)
  if (!existing) {
    return null
  }

  const now = nowIso()
  const updated: WebhookEndpoint = {
    ...existing,
    name: input.name ?? existing.name,
    url: input.url ?? existing.url,
    secret: input.secret ?? existing.secret,
    events: input.events ?? existing.events,
    enabled: input.enabled ?? existing.enabled,
    headers: input.headers ?? existing.headers,
    updated_at: now,
  }

  await env.KV.put(webhookKey(id), JSON.stringify(updated), { expirationTtl: 86400 * 30 })
  return updated
}

export async function deleteWebhook(env: Env, id: string): Promise<boolean> {
  const existing = await getWebhook(env, id)
  if (!existing) {
    return false
  }

  await env.KV.delete(webhookKey(id))

  const ids = await getWebhookIndex(env)
  const newIds = ids.filter(i => i !== id)
  await setWebhookIndex(env, newIds)

  return true
}

export async function getDelivery(env: Env, deliveryId: string): Promise<WebhookDelivery | null> {
  return await env.KV.get(deliveryKey(deliveryId), 'json') as WebhookDelivery | null
}

export async function listDeliveries(env: Env, webhookId: string): Promise<WebhookDelivery[]> {
  const indexKey = deliveryIndexKey(webhookId)
  const index = (await env.KV.get(indexKey, 'json') as string[] | null) || []

  const deliveries = await Promise.all(
    index.map(id => getDelivery(env, id))
  )

  return deliveries.filter((d): d is WebhookDelivery => d !== null)
}

function generateSignature(secret: string, payload: string): string {
  // Simple HMAC-like signature (in production, use proper HMAC)
  const encoder = new TextEncoder()
  const data = encoder.encode(secret + payload)
  // Use a simple hash for demo - in production use Web Crypto API HMAC
  let hash = 0
  for (let i = 0; i < data.length; i++) {
    const char = data[i]
    hash = ((hash << 5) - hash) + char
    hash = hash & hash
  }
  return Math.abs(hash).toString(16).padStart(64, '0')
}

async function generateHmacSignature(secret: string, payload: string): Promise<string> {
  const encoder = new TextEncoder()
  const keyData = encoder.encode(secret)
  const messageData = encoder.encode(payload)

  const key = await crypto.subtle.importKey(
    'raw',
    keyData,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  )

  const signature = await crypto.subtle.sign('HMAC', key, messageData)
  const hashArray = Array.from(new Uint8Array(signature))
  return 'sha256=' + hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
}

const MAX_RETRIES = 3
const RETRY_DELAYS_MS = [1000, 5000, 15000] // 1s, 5s, 15s

interface DeliveryAttempt {
  timestamp: string
  status?: number
  error?: string
}

async function deliverWebhook(
  env: Env,
  webhook: WebhookEndpoint,
  eventType: WebhookEventType,
  payload: Record<string, unknown>
): Promise<WebhookDelivery> {
  const deliveryId = crypto.randomUUID()
  const now = nowIso()

  const delivery: WebhookDelivery = {
    id: deliveryId,
    webhook_id: webhook.id,
    event_type: eventType,
    payload,
    attempts: 0,
    success: false,
    created_at: now,
  }

  const body = JSON.stringify({
    event: eventType,
    timestamp: now,
    data: payload,
  })

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Webhook-Event': eventType,
    'X-Webhook-Delivery': deliveryId,
    ...webhook.headers,
  }

  if (webhook.secret) {
    headers['X-Webhook-Signature'] = await generateHmacSignature(webhook.secret, body)
    // Also include legacy signature for backwards compatibility
    headers['X-Webhook-Signature-Legacy'] = generateSignature(webhook.secret, body)
  }

  const attempts: DeliveryAttempt[] = []

  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    const attemptTime = nowIso()
    delivery.attempts = attempt + 1
    delivery.last_attempt_at = attemptTime

    try {
      const response = await fetch(webhook.url, {
        method: 'POST',
        headers,
        body,
        signal: AbortSignal.timeout(10000), // 10s timeout
      })

      delivery.response_status = response.status
      delivery.response_body = await response.text().then(t => t.substring(0, 1000)).catch(() => '')

      attempts.push({
        timestamp: attemptTime,
        status: response.status,
      })

      if (response.status >= 200 && response.status < 300) {
        delivery.success = true
        delivery.delivered_at = attemptTime
        break
      }

      // Non-2xx response - retry for 5xx errors
      if (response.status < 500 && response.status !== 429) {
        // Client error - don't retry
        break
      }

      // Wait before retry (except on last attempt)
      if (attempt < MAX_RETRIES - 1) {
        await new Promise(resolve => setTimeout(resolve, RETRY_DELAYS_MS[attempt]))
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Unknown error'
      delivery.response_body = errorMessage

      attempts.push({
        timestamp: attemptTime,
        error: errorMessage,
      })

      // Wait before retry (except on last attempt)
      if (attempt < MAX_RETRIES - 1) {
        await new Promise(resolve => setTimeout(resolve, RETRY_DELAYS_MS[attempt]))
      }
    }
  }

  // Store delivery record with attempt history
  const deliveryWithAttempts = {
    ...delivery,
    _attempts: attempts,
  }

  await env.KV.put(deliveryKey(deliveryId), JSON.stringify(deliveryWithAttempts), { expirationTtl: 86400 * 7 })

  // Add to delivery index
  const indexKey = deliveryIndexKey(webhook.id)
  const index = (await env.KV.get(indexKey, 'json') as string[] | null) || []
  index.unshift(deliveryId)
  await env.KV.put(indexKey, JSON.stringify(index.slice(0, 100)), { expirationTtl: 86400 * 7 })

  return delivery
}

export async function triggerWebhooks(
  env: Env,
  eventType: WebhookEventType,
  incident: IncidentRecord
): Promise<WebhookDelivery[]> {
  const webhooks = await listWebhooks(env)
  const enabledWebhooks = webhooks.filter(w => w.enabled && w.events.includes(eventType))

  const payload: Record<string, unknown> = {
    incident_id: incident.id,
    title: incident.title,
    status: incident.status,
    severity: incident.severity,
    source: incident.source,
    correlation_id: incident.correlation_id,
    action_type: incident.action_type,
    action_ref: incident.action_ref,
    created_at: incident.created_at,
    updated_at: incident.updated_at,
  }

  if (incident.execution_result) {
    payload.execution_result = incident.execution_result
  }

  const deliveries = await Promise.all(
    enabledWebhooks.map(webhook => deliverWebhook(env, webhook, eventType, payload))
  )

  return deliveries
}

export async function retryDelivery(
  env: Env,
  deliveryId: string
): Promise<WebhookDelivery | null> {
  const delivery = await getDelivery(env, deliveryId)
  if (!delivery || delivery.success) {
    return null
  }

  const webhook = await getWebhook(env, delivery.webhook_id)
  if (!webhook || !webhook.enabled) {
    return null
  }

  // Redeliver with the same payload
  const newDelivery = await deliverWebhook(
    env,
    webhook,
    delivery.event_type,
    delivery.payload
  )

  return newDelivery
}

export async function listFailedDeliveries(env: Env): Promise<WebhookDelivery[]> {
  const webhooks = await listWebhooks(env)
  const failedDeliveries: WebhookDelivery[] = []

  for (const webhook of webhooks) {
    const deliveries = await listDeliveries(env, webhook.id)
    for (const delivery of deliveries) {
      if (!delivery.success) {
        failedDeliveries.push(delivery)
      }
    }
  }

  return failedDeliveries.sort((a, b) =>
    new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  )
}

export function verifySignature(
  secret: string,
  payload: string,
  signature: string
): Promise<boolean> {
  return generateHmacSignature(secret, payload).then(
    computed => computed === signature
  )
}