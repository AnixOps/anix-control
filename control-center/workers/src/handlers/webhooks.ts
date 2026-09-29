import type { Context } from 'hono'
import { z } from 'zod'
import type { Env, WebhookEventType } from '../types'
import { logAudit } from '../utils/audit'
import {
  createWebhook,
  deleteWebhook,
  getWebhook,
  listDeliveries,
  listFailedDeliveries,
  listWebhooks,
  retryDelivery,
  type CreateWebhookInput,
  updateWebhook,
  type UpdateWebhookInput,
} from '../services/webhooks'

const webhookEventSchema = z.enum([
  'incident.created',
  'incident.acknowledged',
  'incident.escalated',
  'incident.assigned',
  'incident.analyzed',
  'incident.approved',
  'incident.executing',
  'incident.resolved',
  'incident.failed',
])

const createWebhookSchema = z.object({
  name: z.string().min(1).max(100),
  url: z.string().url(),
  secret: z.string().max(100).optional(),
  events: z.array(webhookEventSchema).min(1),
  headers: z.record(z.string(), z.string()).optional(),
})

const updateWebhookSchema = z.object({
  name: z.string().min(1).max(100).optional(),
  url: z.string().url().optional(),
  secret: z.string().max(100).optional(),
  events: z.array(webhookEventSchema).min(1).optional(),
  enabled: z.boolean().optional(),
  headers: z.record(z.string(), z.string()).optional(),
})

async function requireWebhook(c: Context<{ Bindings: Env }>, id: string) {
  const webhook = await getWebhook(c.env, id)

  if (!webhook) {
    return c.json({ success: false, error: 'Webhook not found' }, 404)
  }

  return webhook
}

export async function listWebhooksHandler(c: Context<{ Bindings: Env }>) {
  const webhooks = await listWebhooks(c.env)

  return c.json({
    success: true,
    data: webhooks.map(w => ({
      id: w.id,
      name: w.name,
      url: w.url,
      events: w.events,
      enabled: w.enabled,
      created_by: w.created_by,
      created_at: w.created_at,
      updated_at: w.updated_at,
    })),
  })
}

export async function getWebhookHandler(c: Context<{ Bindings: Env }>) {
  const webhookId = c.req.param('id') as string
  const webhook = await requireWebhook(c, webhookId)

  if (webhook instanceof Response) {
    return webhook
  }

  return c.json({ success: true, data: webhook })
}

export async function createWebhookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createWebhookSchema.parse(await c.req.json())
    const webhook = await createWebhook(c.env, principal, body as CreateWebhookInput)

    await logAudit(c, principal.sub, 'create_webhook', 'webhook', {
      webhook_id: webhook.id,
      webhook_name: webhook.name,
      events: webhook.events,
    })

    return c.json({ success: true, data: webhook }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

export async function updateWebhookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const webhookId = c.req.param('id') as string

  const existing = await requireWebhook(c, webhookId)
  if (existing instanceof Response) {
    return existing
  }

  try {
    const body = updateWebhookSchema.parse(await c.req.json())
    const updated = await updateWebhook(c.env, webhookId, body as UpdateWebhookInput)

    if (!updated) {
      return c.json({ success: false, error: 'Failed to update webhook' }, 400)
    }

    await logAudit(c, principal.sub, 'update_webhook', 'webhook', {
      webhook_id: updated.id,
      webhook_name: updated.name,
    })

    return c.json({ success: true, data: updated })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

export async function deleteWebhookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const webhookId = c.req.param('id') as string

  const existing = await requireWebhook(c, webhookId)
  if (existing instanceof Response) {
    return existing
  }

  const deleted = await deleteWebhook(c.env, webhookId)

  if (!deleted) {
    return c.json({ success: false, error: 'Failed to delete webhook' }, 400)
  }

  await logAudit(c, principal.sub, 'delete_webhook', 'webhook', {
    webhook_id: webhookId,
  })

  return c.json({ success: true })
}

export async function listWebhookDeliveriesHandler(c: Context<{ Bindings: Env }>) {
  const webhookId = c.req.param('id') as string

  const existing = await requireWebhook(c, webhookId)
  if (existing instanceof Response) {
    return existing
  }

  const deliveries = await listDeliveries(c.env, webhookId)

  return c.json({
    success: true,
    data: deliveries.map(d => ({
      id: d.id,
      event_type: d.event_type,
      success: d.success,
      response_status: d.response_status,
      attempts: d.attempts,
      delivered_at: d.delivered_at,
      created_at: d.created_at,
    })),
  })
}

export async function retryDeliveryHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const deliveryId = c.req.param('deliveryId') as string

  const delivery = await retryDelivery(c.env, deliveryId)

  if (!delivery) {
    return c.json({ success: false, error: 'Delivery not found, already succeeded, or webhook disabled' }, 404)
  }

  await logAudit(c, principal.sub, 'retry_webhook_delivery', 'webhook', {
    delivery_id: deliveryId,
    new_delivery_id: delivery.id,
    success: delivery.success,
  })

  return c.json({
    success: true,
    data: {
      id: delivery.id,
      event_type: delivery.event_type,
      success: delivery.success,
      response_status: delivery.response_status,
      attempts: delivery.attempts,
      delivered_at: delivery.delivered_at,
    },
  })
}

export async function listFailedDeliveriesHandler(c: Context<{ Bindings: Env }>) {
  const failedDeliveries = await listFailedDeliveries(c.env)

  return c.json({
    success: true,
    data: failedDeliveries.map(d => ({
      id: d.id,
      webhook_id: d.webhook_id,
      event_type: d.event_type,
      success: d.success,
      response_status: d.response_status,
      response_body: d.response_body,
      attempts: d.attempts,
      last_attempt_at: d.last_attempt_at,
      created_at: d.created_at,
    })),
  })
}