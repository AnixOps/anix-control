/**
 * E2E Integration Tests
 *
 * These tests verify end-to-end flows across multiple handlers and services.
 * They use mocked infrastructure (D1, KV, R2) but test real business logic.
 *
 * Note: Each test suite creates its own mock environment, so tests within
 * a suite share state but different suites are isolated.
 */

import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import app from '../src/index'
import type { ApiErrorResponse, AuthLoginResponse, AuthMeResponse, AuthRefreshResponse, AuthRegisterResponse, DashboardOverviewResponseData, Env, HealthResponse, ReadinessResponse } from '../src/types'
import { createMockKV, createMockR2, createMockD1 } from '../test/setup'

// Create test environment with shared mocks
function createTestEnv(overrides: Partial<Env> = {}): Env {
  return {
    ENVIRONMENT: 'development',
    JWT_SECRET: 'test-secret-key-for-e2e-tests-min-32-characters!',
    JWT_EXPIRE: '3600',
    API_KEY_SALT: 'test-salt-for-api-keys',
    DB: createMockD1(),
    KV: createMockKV(),
    R2: createMockR2(),
    ...overrides,
  }
}

describe('E2E: Health Check', () => {
  let env: Env

  beforeAll(() => {
    env = createTestEnv()
  })

  it('should return healthy status', async () => {
    const res = await app.request('/health', {}, env)
    expect(res.status).toBe(200)
    const data = await res.json() as HealthResponse
    expect(data.status).toBe('healthy')
  })

  it('should return readiness status', async () => {
    const res = await app.request('/readiness', {}, env)
    expect(res.status).toBe(200)
    const data = await res.json() as ReadinessResponse
    expect(data.status).toBe('ready')
  })
})

describe('E2E: Dashboard Overview', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'dashboard@example.com',
        password: 'Dashboard123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'dashboard@example.com',
        password: 'Dashboard123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should return node, user, and activity counts in the overview', async () => {
    const res = await app.request('/api/v1/dashboard', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as DashboardOverviewResponseData

    expect(data.success).toBe(true)
    expect(data.cached).toBe(false)
    expect(typeof data.data.nodes.total).toBe('number')
    expect(typeof data.data.nodes.online).toBe('number')
    expect(typeof data.data.nodes.offline).toBe('number')
    expect(typeof data.data.nodes.maintenance).toBe('number')
    expect(typeof data.data.users.total).toBe('number')
    expect(typeof data.data.activity.last_24h).toBe('number')
    expect(data.data.timestamp).toBeTruthy()
  })

  it('should return the cached dashboard overview on the second request', async () => {
    const firstRes = await app.request('/api/v1/dashboard', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    const secondRes = await app.request('/api/v1/dashboard', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(firstRes.status).toBe(200)
    expect(secondRes.status).toBe(200)

    const firstData = await firstRes.json() as DashboardOverviewResponseData
    const secondData = await secondRes.json() as DashboardOverviewResponseData

    expect(secondData.cached).toBe(true)
    expect(secondData.data).toEqual(firstData.data)
    expect(typeof secondData.data.nodes.total).toBe('number')
  })
})

describe('E2E: WebSocket Flow', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'wsuser@example.com',
        password: 'WsUser123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'wsuser@example.com',
        password: 'WsUser123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should reject websocket access without a token', async () => {
    const res = await app.request('/api/v1/ws', {
      method: 'GET',
      headers: { Upgrade: 'websocket' },
    }, env)

    expect(res.status).toBe(401)
  })

  it('should require websocket upgrade after authentication', async () => {
    const res = await app.request('/api/v1/ws', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(426)
    const data = await res.json() as ApiErrorResponse
    expect(data.success).toBe(false)
    expect(data.error).toBe('Expected WebSocket upgrade')
  })
})

describe('E2E: Authentication Flow', () => {
  let env: Env
  let authToken: string
  let refreshToken: string

  beforeAll(() => {
    env = createTestEnv()
  })

  it('should register a new user', async () => {
    const res = await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'test@example.com',
        password: 'TestPass123!',
        role: 'admin',
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as AuthRegisterResponse
    expect(data.success).toBe(true)
    expect(data.data?.email).toBe('test@example.com')
  })

  it('should login with valid credentials', async () => {
    const res = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'test@example.com',
        password: 'TestPass123!',
      }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as AuthLoginResponse
    expect(data.success).toBe(true)
    expect(data.data?.access_token).toBeDefined()
    authToken = data.data?.access_token || ''
    refreshToken = data.data?.refresh_token || ''
  })

  it('should access protected route with valid token', async () => {
    const res = await app.request('/api/v1/users/me', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as AuthMeResponse
    expect(data.success).toBe(true)
    expect(data.data?.email).toBe('test@example.com')
  })

  it('should reject request without token', async () => {
    const res = await app.request('/api/v1/users/me', {
      method: 'GET',
    }, env)
    expect(res.status).toBe(401)
  })

  it('should refresh tokens', async () => {
    const res = await app.request('/api/v1/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as AuthRefreshResponse
    expect(data.success).toBe(true)
    expect(data.data?.access_token).toBeDefined()
  })
})

describe('E2E: Node Management Flow', () => {
  let env: Env
  let authToken: string
  let nodeId: number

  beforeAll(async () => {
    env = createTestEnv()

    // Register and login
    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'nodeuser@example.com',
        password: 'NodeUser123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'nodeuser@example.com',
        password: 'NodeUser123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should create a node', async () => {
    const res = await app.request('/api/v1/nodes', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'test-server-1',
        host: '192.168.1.100',
        port: 22,
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as { success: boolean; data?: { id: number; name: string } }
    expect(data.success).toBe(true)
    expect(data.data?.name).toBe('test-server-1')
    nodeId = data.data?.id || 0
  })

  it('should list nodes', async () => {
    const res = await app.request('/api/v1/nodes', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { items: Array<{ id: number }> } }
    expect(data.success).toBe(true)
  })
})

describe('E2E: Playbook Flow', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    // Register and login
    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'playbook@example.com',
        password: 'Playbook123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'playbook@example.com',
        password: 'Playbook123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should list built-in playbooks', async () => {
    const res = await app.request('/api/v1/playbooks/built-in', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: Array<{ name: string }> }
    expect(data.success).toBe(true)
    expect(data.data?.length).toBeGreaterThan(0)
  })

  it('should get playbook categories', async () => {
    const res = await app.request('/api/v1/playbooks/categories', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(data.success).toBe(true)
    expect(data.data?.length).toBeGreaterThan(0)
  })

  it('should get a specific built-in playbook', async () => {
    const res = await app.request('/api/v1/playbooks/install-fail2ban', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { name: string; content: string } }
    expect(data.success).toBe(true)
    expect(data.data?.name).toBe('install-fail2ban')
    expect(data.data?.content).toContain('hosts: all')
  })
})

describe('E2E: MFA Flow', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    // Register and login
    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'mfauser@example.com',
        password: 'MfaUser123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'mfauser@example.com',
        password: 'MfaUser123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should get MFA status (disabled initially)', async () => {
    const res = await app.request('/api/v1/mfa/status', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { enabled: boolean } }
    expect(data.success).toBe(true)
    expect(data.data?.enabled).toBe(false)
  })

  it('should setup MFA', async () => {
    const res = await app.request('/api/v1/mfa/setup', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({}),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as {
      success: boolean
      data?: {
        secret: string
        otpauth_url: string
        recovery_codes: string[]
      }
    }
    expect(data.success).toBe(true)
    expect(data.data?.secret).toBeDefined()
    expect(data.data?.otpauth_url).toContain('otpauth://totp/')
    expect(data.data?.recovery_codes?.length).toBe(8)
  })
})

describe('E2E: Realtime Endpoints Smoke', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'realtime@example.com',
        password: 'Realtime123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'realtime@example.com',
        password: 'Realtime123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should reject SSE request without token', async () => {
    const res = await app.request('/api/v1/sse', { method: 'GET' }, env)
    expect(res.status).toBe(401)
  })

  it('should open SSE stream with valid token', async () => {
    const res = await app.request('/api/v1/sse', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    expect(res.headers.get('Content-Type')).toContain('text/event-stream')
    await res.body?.cancel()
  })

  it('should allow SSE subscribe/unsubscribe for valid channel', async () => {
    const subscribeRes = await app.request('/api/v1/sse/subscribe', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ channel: 'nodes' }),
    }, env)

    expect(subscribeRes.status).toBe(200)

    const unsubscribeRes = await app.request('/api/v1/sse/unsubscribe', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ channel: 'nodes' }),
    }, env)

    expect(unsubscribeRes.status).toBe(200)
  })

  it('should reject websocket upgrade when Upgrade header is missing', async () => {
    const res = await app.request('/api/v1/ws', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(426)
  })
})