import { describe, it, expect, beforeEach } from 'vitest'
import app from '../index'
import type { AuthLoginResponse, Env } from '../types'
import { createMockD1, createMockKV, createMockR2 } from '../../test/setup'

function createEnv(): Env {
  return {
    ENVIRONMENT: 'development',
    JWT_SECRET: 'webhook-test-secret-key-min-32-characters!',
    JWT_EXPIRE: '3600',
    API_KEY_SALT: 'webhook-test-salt',
    DB: createMockD1(),
    KV: createMockKV(),
    R2: createMockR2(),
    AI: {
      run: async () => ({ response: '{"result":"ok"}' }),
    } as Env['AI'],
  }
}

async function registerAndLogin(env: Env, email: string, password: string, role: 'admin' | 'operator' | 'viewer' = 'admin') {
  await app.request('/api/v1/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, role }),
  }, env)

  const loginRes = await app.request('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  }, env)

  const loginData = await loginRes.json() as AuthLoginResponse
  return loginData.data?.access_token || ''
}

describe('webhook handlers', () => {
  let env: Env
  let adminToken: string
  let operatorToken: string

  beforeEach(async () => {
    env = createEnv()
    adminToken = await registerAndLogin(env, 'webhook-admin@example.com', 'WebhookAdmin123!', 'admin')
    operatorToken = await registerAndLogin(env, 'webhook-operator@example.com', 'WebhookOperator123!', 'operator')
  })

  it('creates a webhook', async () => {
    const res = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created', 'incident.resolved'],
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as { success: boolean; data?: { id: string; name: string; events: string[] } }
    expect(data.success).toBe(true)
    expect(data.data?.name).toBe('Test Webhook')
    expect(data.data?.events).toContain('incident.created')
  })

  it('lists webhooks', async () => {
    // Create a webhook first
    await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'List Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created'],
      }),
    }, env)

    const res = await app.request('/api/v1/webhooks', {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: Array<{ name: string }> }
    expect(data.success).toBe(true)
    expect((data.data || []).length).toBeGreaterThan(0)
  })

  it('gets a specific webhook', async () => {
    const createRes = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Get Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created'],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const webhookId = createData.data?.id || ''

    const res = await app.request(`/api/v1/webhooks/${webhookId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { id: string } }
    expect(data.success).toBe(true)
    expect(data.data?.id).toBe(webhookId)
  })

  it('updates a webhook', async () => {
    const createRes = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Update Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created'],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const webhookId = createData.data?.id || ''

    const res = await app.request(`/api/v1/webhooks/${webhookId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Updated Webhook Name',
        events: ['incident.created', 'incident.resolved', 'incident.failed'],
      }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { name: string; events: string[] } }
    expect(data.success).toBe(true)
    expect(data.data?.name).toBe('Updated Webhook Name')
    expect(data.data?.events.length).toBe(3)
  })

  it('deletes a webhook', async () => {
    const createRes = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Delete Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created'],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const webhookId = createData.data?.id || ''

    const deleteRes = await app.request(`/api/v1/webhooks/${webhookId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(deleteRes.status).toBe(200)

    const getRes = await app.request(`/api/v1/webhooks/${webhookId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(getRes.status).toBe(404)
  })

  it('denies operator access to webhooks', async () => {
    const res = await app.request('/api/v1/webhooks', {
      method: 'GET',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(res.status).toBe(403)
  })

  it('validates webhook URL', async () => {
    const res = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Invalid URL Webhook',
        url: 'not-a-url',
        events: ['incident.created'],
      }),
    }, env)

    expect(res.status).toBe(400)
  })

  it('requires at least one event', async () => {
    const res = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'No Events Webhook',
        url: 'https://example.com/webhook',
        events: [],
      }),
    }, env)

    expect(res.status).toBe(400)
  })

  it('can disable a webhook', async () => {
    const createRes = await app.request('/api/v1/webhooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Disable Test Webhook',
        url: 'https://example.com/webhook',
        events: ['incident.created'],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const webhookId = createData.data?.id || ''

    const res = await app.request(`/api/v1/webhooks/${webhookId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        enabled: false,
      }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { enabled: boolean } }
    expect(data.success).toBe(true)
    expect(data.data?.enabled).toBe(false)
  })
})