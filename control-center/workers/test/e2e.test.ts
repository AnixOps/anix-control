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
import type { ApiErrorResponse, AuthLoginResponse, AuthMeResponse, AuthRefreshResponse, AuthRegisterResponse, DashboardOverviewResponseData, DeveloperReadinessSummaryResponseData, Env, HealthResponse, ReadinessResponse } from '../src/types'
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
    AI: {
      run: async () => ({ response: '{"result":"ok"}' }),
    } as Env['AI'],
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
    env = createTestEnv({ DEVELOPER_MODE: 'true' })

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

  it('should include developer readiness summary in the overview', async () => {
    const res = await app.request('/api/v1/dashboard', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as DashboardOverviewResponseData

    expect(data.success).toBe(true)
    expect(data.data.developer_readiness_summary.manifest_total).toBeGreaterThan(0)
    expect(data.data.developer_readiness_summary.readiness_counts.diagnostic).toBeGreaterThan(0)
    expect(data.data.developer_readiness_summary.execution_mode_counts.automated).toBeGreaterThan(0)
    expect(data.data.developer_readiness_summary.ready_endpoints.length).toBeGreaterThan(0)
    expect(data.data.developer_readiness_summary.manual_endpoints.length).toBeGreaterThan(0)
    expect(data.data.developer_readiness_summary.fixture_coverage.total_endpoints).toBeGreaterThan(0)
  })

  it('should match the internal readiness summary contract', async () => {
    const dashboardRes = await app.request('/api/v1/dashboard', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    const internalRes = await app.request('/api/v1/internal/dev/readiness-summary', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(dashboardRes.status).toBe(200)
    expect(internalRes.status).toBe(200)

    const dashboardData = await dashboardRes.json() as DashboardOverviewResponseData
    const internalData = await internalRes.json() as DeveloperReadinessSummaryResponseData

    expect(dashboardData.data.developer_readiness_summary).toEqual(internalData.data.manifest)
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
    expect(secondData.data.developer_readiness_summary).toEqual(firstData.data.developer_readiness_summary)
    expect(secondData.data.developer_readiness_summary.manifest_total).toBeGreaterThan(0)
    expect(secondData.data.developer_readiness_summary.readiness_counts.verified).toBeGreaterThan(0)
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

describe('E2E: AI and Vectorize Flow', () => {
  let env: Env
  let authToken: string

  beforeAll(async () => {
    env = createTestEnv()

    ;(env as Env & {
      AI: { run: (model: string, input: unknown) => Promise<unknown> }
      VECTORIZE: {
        upsert: (vectors: Array<{ id: string; values: number[]; metadata: Record<string, unknown> }>) => Promise<void>
        query: (embedding: number[], options: Record<string, unknown>) => Promise<{ matches: Array<{ id: string; score: number; metadata: Record<string, unknown> }> }>
        deleteByIds: (ids: string[]) => Promise<void>
        getByIds: (ids: string[]) => Promise<Array<{ id: string; values: number[]; metadata: Record<string, unknown> }>>
      }
    }).AI = {
      run: async (model: string) => {
        if (model.includes('bge-base-en-v1.5')) {
          return { embedding: [0.11, 0.22, 0.33] }
        }
        return { response: '{"result":"ok"}' }
      },
    }

    let insertedVectors: Array<{ id: string; values: number[]; metadata: Record<string, unknown> }> = []
    ;(env as Env & {
      VECTORIZE: {
        upsert: (vectors: Array<{ id: string; values: number[]; metadata: Record<string, unknown> }>) => Promise<void>
        query: (embedding: number[], options: Record<string, unknown>) => Promise<{ matches: Array<{ id: string; score: number; metadata: Record<string, unknown> }> }>
        deleteByIds: (ids: string[]) => Promise<void>
        getByIds: (ids: string[]) => Promise<Array<{ id: string; values: number[]; metadata: Record<string, unknown> }>>
      }
    }).VECTORIZE = {
      upsert: async (vectors) => {
        insertedVectors = [...insertedVectors, ...vectors]
      },
      query: async () => ({
        matches: insertedVectors.map((vector) => ({
          id: vector.id,
          score: 0.99,
          metadata: vector.metadata,
        })),
      }),
      deleteByIds: async (ids) => {
        insertedVectors = insertedVectors.filter((vector) => !ids.includes(vector.id))
      },
      getByIds: async (ids) => insertedVectors.filter((vector) => ids.includes(vector.id)),
    }

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'aiuser@example.com',
        password: 'AiUser123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'aiuser@example.com',
        password: 'AiUser123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should generate embedding', async () => {
    const res = await app.request('/api/v1/ai/embedding', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ text: 'deployment failed due to timeout' }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { embedding?: number[] } }
    expect(data.success).toBe(true)
  })

  it('should insert and search vectors', async () => {
    const insertRes = await app.request('/api/v1/vectors', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        vectors: [
          {
            id: 'log-1',
            embedding: [0.11, 0.22, 0.33],
            metadata: {
              id: 'log-1',
              type: 'log',
              timestamp: '2026-03-23T00:00:00Z',
              level: 'error',
            },
          },
        ],
      }),
    }, env)

    expect(insertRes.status).toBe(200)

    const searchRes = await app.request('/api/v1/vectors/search', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        embedding: [0.11, 0.22, 0.33],
        topK: 5,
        type: 'log',
      }),
    }, env)

    expect(searchRes.status).toBe(200)
    const searchData = await searchRes.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(searchData.success).toBe(true)
    expect(searchData.data?.[0]?.id).toBe('log-1')
  })

  it('should chat and translate natural language query', async () => {
    const chatRes = await app.request('/api/v1/ai/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        message: 'How do I troubleshoot a failed deployment?',
        history: [],
      }),
    }, env)

    expect(chatRes.status).toBe(200)

    const queryRes = await app.request('/api/v1/ai/query', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        query: 'show failed tasks',
        schema: 'tasks(status, created_at)',
      }),
    }, env)

    expect(queryRes.status).toBe(200)
    const queryData = await queryRes.json() as { success: boolean; data?: { response?: string } }
    expect(queryData.success).toBe(true)
  })
})

describe('E2E: Web3 and IPFS Flow', () => {
  let env: Env
  let authToken: string
  const walletAddress = '0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18'

  beforeAll(async () => {
    env = createTestEnv()

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'web3user@example.com',
        password: 'Web3User123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'web3user@example.com',
        password: 'Web3User123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''
  })

  it('should issue a SIWE challenge', async () => {
    const res = await app.request('/api/v1/web3/challenge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: walletAddress }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { message: string; nonce: string } }
    expect(data.success).toBe(true)
    expect(data.data?.message).toContain(walletAddress)
    expect(data.data?.nonce).toBeDefined()
  })

  it('should verify a signed message', async () => {
    const challengeRes = await app.request('/api/v1/web3/challenge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: walletAddress }),
    }, env)
    const challengeData = await challengeRes.json() as { data?: { message: string } }

    const verifyRes = await app.request('/api/v1/web3/verify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        address: walletAddress,
        signature: '0xmockedsignature',
        message: challengeData.data?.message,
      }),
    }, env)

    expect(verifyRes.status).toBe(200)
    const verifyData = await verifyRes.json() as { success: boolean; data?: { did: string } }
    expect(verifyData.success).toBe(true)
    expect(verifyData.data?.did).toContain('did:ethr:')
  })

  it('should upload to and retrieve from IPFS', async () => {
    const uploadRes = await app.request('/api/v1/ipfs/upload', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        data: JSON.stringify({ hello: 'ipfs' }),
        filename: 'hello.json',
      }),
    }, env)

    expect(uploadRes.status).toBe(200)
    const uploadData = await uploadRes.json() as { success: boolean; data?: { cid: string } }
    expect(uploadData.success).toBe(true)
    expect(uploadData.data?.cid).toBeDefined()

    const getRes = await app.request(`/api/v1/ipfs/${uploadData.data?.cid}`, {
      method: 'GET',
    }, env)

    expect(getRes.status).toBe(200)
    const body = await getRes.text()
    expect(body).toContain('104')
    expect(body).toContain('112')
  })

  it('should store audit record on chain', async () => {
    const res = await app.request('/api/v1/web3/audit', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        action: 'node.restart',
        userId: 1,
        timestamp: '2026-03-23T00:00:00Z',
        details: 'Restarted web3-enabled node',
      }),
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: { txHash: string; ipfsCid: string } }
    expect(data.success).toBe(true)
    expect(data.data?.txHash).toContain('0x')
    expect(data.data?.ipfsCid).toBeDefined()
  })
})

describe('E2E: Incident Workflow', () => {
  let env: Env
  let authToken: string
  let apiKey: string
  let scalingPolicyId: string
  let incidentId: string

  beforeAll(async () => {
    env = createTestEnv()

    ;(env as Env & {
      AI: { run: (model: string, input: unknown) => Promise<unknown> }
    }).AI = {
      run: async () => ({
        response: JSON.stringify({
          summary: 'CPU saturation on payments deployment',
          severity: 'high',
          likely_cause: 'Traffic spike exceeded current replica budget',
          recommended_actions: ['Scale target workload using existing scaling policy'],
        }),
      }),
    }

    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'incident-admin@example.com',
        password: 'IncidentAdmin123!',
        role: 'admin',
      }),
    }, env)

    const loginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'incident-admin@example.com',
        password: 'IncidentAdmin123!',
      }),
    }, env)

    const loginData = await loginRes.json() as AuthLoginResponse
    authToken = loginData.data?.access_token || ''

    const tokenRes = await app.request('/api/v1/users/me/tokens', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'incident-client',
      }),
    }, env)

    const tokenData = await tokenRes.json() as { data?: { token: string } }
    apiKey = tokenData.data?.token || ''

    const policyRes = await app.request('/api/v1/scaling/policies', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'payments-policy',
        targetType: 'deployment',
        targetId: 'payments',
        namespace: 'prod',
        minReplicas: 1,
        maxReplicas: 4,
        metrics: [{ type: 'cpu', targetValue: 70 }],
      }),
    }, env)

    const policyData = await policyRes.json() as { data?: { id: string } | { policy?: { id: string } }; policy?: { id: string } }
    scalingPolicyId = (policyData.data as { id?: string; policy?: { id?: string } } | undefined)?.id
      || (policyData.data as { policy?: { id?: string } } | undefined)?.policy?.id
      || policyData.policy?.id
      || ''
  })

  it('should create an incident with authenticated user context', async () => {
    const res = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Payments deployment saturation',
        source: 'alerts.cpu',
        severity: 'high',
        summary: 'CPU remained above threshold for 10 minutes',
        action_type: 'scale_policy',
        action_ref: scalingPolicyId,
        evidence: [
          {
            type: 'alert',
            source: 'prometheus',
            content: 'payments deployment CPU > 90%',
          },
        ],
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as { success: boolean; data?: { id: string; requested_via: string; status: string } }
    expect(data.success).toBe(true)
    expect(data.data?.requested_via).toBe('jwt')
    expect(data.data?.status).toBe('open')
    incidentId = data.data?.id || ''
  })

  it('should create an incident with API key auth', async () => {
    const res = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': apiKey,
      },
      body: JSON.stringify({
        title: 'Payments deployment saturation via API key',
        source: 'alerts.cpu',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: scalingPolicyId,
        evidence: [
          {
            type: 'alert',
            source: 'prometheus',
            content: 'API key initiated incident creation',
          },
        ],
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as { success: boolean; data?: { requested_via: string } }
    expect(data.success).toBe(true)
    expect(data.data?.requested_via).toBe('api_key')
  })

  it('should analyze, approve, and execute the incident workflow', async () => {
    const analyzeRes = await app.request(`/api/v1/incidents/${incidentId}/analyze`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(analyzeRes.status).toBe(200)
    const analyzeData = await analyzeRes.json() as { success: boolean; data?: { status: string; analysis?: Record<string, unknown> } }
    expect(analyzeData.success).toBe(true)
    expect(analyzeData.data?.status).toBe('analyzed')
    expect(analyzeData.data?.analysis).toBeDefined()

    const approveRes = await app.request(`/api/v1/incidents/${incidentId}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(approveRes.status).toBe(200)
    const approveData = await approveRes.json() as { success: boolean; data?: { status: string } }
    expect(approveData.data?.status).toBe('approved')

    const executeRes = await app.request(`/api/v1/incidents/${incidentId}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect([200, 400]).toContain(executeRes.status)
    const executeData = await executeRes.json() as { success: boolean; data?: { status: string; execution_id?: string } }
    expect(executeData.success).toBe(true)
    expect(['resolved', 'failed']).toContain(executeData.data?.status || '')
  })

  it('should list and fetch incidents', async () => {
    const listRes = await app.request('/api/v1/incidents?severity=high', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(listRes.status).toBe(200)
    const listData = await listRes.json() as { success: boolean; data?: { items?: Array<{ id: string }> } }
    expect(listData.success).toBe(true)
    expect((listData.data?.items || []).some((incident) => incident.id === incidentId)).toBe(true)

    const getRes = await app.request(`/api/v1/incidents/${incidentId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(getRes.status).toBe(200)
    const getData = await getRes.json() as { success: boolean; data?: { id: string; recommendations: unknown[] } }
    expect(getData.success).toBe(true)
    expect(getData.data?.id).toBe(incidentId)
    expect(Array.isArray(getData.data?.recommendations)).toBe(true)
  })

  it('should create, approve, and execute a restart deployment incident', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Restart payments deployment',
        source: 'alerts.rollout',
        severity: 'critical',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
        evidence: [
          {
            type: 'service',
            source: 'kubernetes',
            content: 'deployment rollout stalled',
          },
        ],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { data?: { id: string } }
    const restartIncidentId = createData.data?.id || ''

    const approveRes = await app.request(`/api/v1/incidents/${restartIncidentId}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    expect(approveRes.status).toBe(200)

    const executeRes = await app.request(`/api/v1/incidents/${restartIncidentId}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(executeRes.status).toBe(200)
    const executeData = await executeRes.json() as { success: boolean; data?: { status: string; execution_result?: { restarted?: boolean } } }
    expect(executeData.success).toBe(true)
    expect(executeData.data?.status).toBe('resolved')
    expect(executeData.data?.execution_result?.success).toBe(true)
    expect(executeData.data?.execution_result?.backend).toBe('kubernetes')
  })
  it('should enrich evidence automatically for restart deployment incidents', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Restart deployment with enrichment',
        source: 'alerts.rollout',
        severity: 'medium',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
        evidence: [
          {
            type: 'manual',
            source: 'operator',
            content: 'rollout observed as stuck',
          },
        ],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { evidence?: Array<{ source: string }> } }
    expect(createData.success).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'kubernetes.deployment')).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'kubernetes.event')).toBe(true)
  })

  it('should enrich evidence automatically for task references', async () => {
    await env.DB
      .prepare(`
        INSERT INTO tasks (task_id, playbook_id, playbook_name, status, trigger_type, triggered_by, target_nodes, variables)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      `)
      .bind('task-e2e-1', 1, 'deploy-app', 'failed', 'manual', 1, '[]', '{}')
      .run()

    await env.DB
      .prepare(`
        INSERT INTO task_logs (task_id, node_id, node_name, level, message, metadata)
        VALUES (?, ?, ?, ?, ?, ?)
      `)
      .bind('task-e2e-1', null, 'node-a', 'error', 'Task execution failed', JSON.stringify({ reason: 'timeout' }))
      .run()

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Task-linked incident',
        source: 'task',
        severity: 'high',
        action_ref: 'task:task-e2e-1',
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { evidence?: Array<{ source: string }> } }
    expect(createData.success).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'tasks.record')).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'tasks.log')).toBe(true)
  })

  it('should enrich evidence automatically for node references', async () => {
    const nodeRes = await app.request('/api/v1/nodes', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'node-incident-evidence',
        host: '10.0.0.11',
        port: 22,
      }),
    }, env)

    const nodeData = await nodeRes.json() as { data?: { id: number } }
    await env.KV.put(`agent:latest:${nodeData.data?.id}`, JSON.stringify({ cpu_usage: 88, memory_usage: 79 }))

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Node-linked incident',
        source: 'node',
        severity: 'medium',
        action_ref: `node:${nodeData.data?.id}`,
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { evidence?: Array<{ source: string }> } }
    expect(createData.success).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'nodes.record')).toBe(true)
    expect((createData.data?.evidence || []).some((item) => item.source === 'nodes.latest_metrics')).toBe(true)
  })

  it('should reject operator approval for critical restart incidents', async () => {
    await app.request('/api/v1/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'incident-operator-policy@example.com',
        password: 'IncidentOperator123!',
        role: 'operator',
      }),
    }, env)

    const operatorLoginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'incident-operator-policy@example.com',
        password: 'IncidentOperator123!',
      }),
    }, env)
    const operatorLoginData = await operatorLoginRes.json() as AuthLoginResponse
    const operatorToken = operatorLoginData.data?.access_token || ''

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Critical restart payments deployment',
        source: 'alerts.rollout',
        severity: 'critical',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })

  it('should reject operator approval for scale policy incidents', async () => {
    const operatorLoginRes = await app.request('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'incident-operator-policy@example.com',
        password: 'IncidentOperator123!',
      }),
    }, env)
    const operatorLoginData = await operatorLoginRes.json() as AuthLoginResponse
    const operatorToken = operatorLoginData.data?.access_token || ''

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Operator should not approve scale policy',
        source: 'alerts.cpu',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: scalingPolicyId,
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })
  it('should return incident summaries in list responses and details in item responses', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Summary/detail incident',
        source: 'alerts.cpu',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: scalingPolicyId,
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string; correlation_id?: string } }
    const incidentId = createData.data?.id || ''
    const detailRes = await app.request(`/api/v1/incidents/${incidentId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    const detailForCorrelation = await detailRes.json() as { data?: { correlation_id?: string } }

    const listRes = await app.request('/api/v1/incidents?correlation_id=' + encodeURIComponent(detailForCorrelation.data?.correlation_id || ''), {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    expect(listRes.status).toBe(200)
    const listData = await listRes.json() as { data?: { items?: Array<Record<string, unknown>>; total?: number; page?: number; per_page?: number; total_pages?: number } }
    const summary = (listData.data?.items || []).find(item => item.id === incidentId)
    expect(listData.data?.page).toBe(1)
    expect(listData.data?.per_page).toBe(20)
    expect((listData.data?.total || 0)).toBeGreaterThan(0)
    expect(summary).toBeDefined()
    expect('evidence' in (summary || {})).toBe(false)
    expect('analysis' in (summary || {})).toBe(false)

    const incidentDetailRes = await app.request(`/api/v1/incidents/${incidentId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)
    expect(incidentDetailRes.status).toBe(200)
    const detailData = await incidentDetailRes.json() as { data?: Record<string, unknown> }
    expect(Array.isArray(detailData.data?.evidence)).toBe(true)
    expect(Array.isArray(detailData.data?.recommendations)).toBe(true)
  })

  it('should expose structured links and query filters', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Linked scale incident',
        source: 'alerts.cpu',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: scalingPolicyId,
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { links?: Array<{ kind: string }> ; correlation_id?: string } }
    expect(createData.success).toBe(true)
    expect((createData.data?.links || []).some((link) => link.kind === 'scaling_policy')).toBe(true)

    const filteredRes = await app.request('/api/v1/incidents?requested_via=jwt&source=alerts.cpu&has_action=true', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(filteredRes.status).toBe(200)
    const filteredData = await filteredRes.json() as { success: boolean; data?: { items?: Array<{ source: string; requested_via: string; action_ref?: string }>; page?: number; per_page?: number } }
    expect(filteredData.success).toBe(true)
    expect(filteredData.data?.page).toBe(1)
    expect((filteredData.data?.items || []).every((incident) => incident.source === 'alerts.cpu')).toBe(true)
    expect((filteredData.data?.items || []).every((incident) => incident.requested_via === 'jwt')).toBe(true)
    expect((filteredData.data?.items || []).every((incident) => !!incident.action_ref)).toBe(true)
  })

  it('should expose incident lifecycle events in realtime history', async () => {
    const statusRes = await app.request('/api/v1/sse/status', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(statusRes.status).toBe(200)
    const statusData = await statusRes.json() as {
      success: boolean
      data?: { stats?: { recent_events?: number }; connections?: unknown[] }
    }
    expect(statusData.success).toBe(true)
    expect((statusData.data?.stats?.recent_events || 0)).toBeGreaterThan(0)
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