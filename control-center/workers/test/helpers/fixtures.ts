import app from '../../src/index'
import type { AuthLoginResponse, Env } from '../../src/types'
import { createMockD1, createMockKV, createMockR2 } from '../setup'

export type BootstrapRole = 'admin' | 'operator' | 'viewer'

export interface BootstrappedUser {
  email: string
  role: BootstrapRole
  token: string
}

export interface BootstrappedPrincipals {
  admin: BootstrappedUser
  operator: BootstrappedUser
  viewer: BootstrappedUser
}

export function createTestEnv(overrides: Partial<Env> = {}): Env {
  return {
    ENVIRONMENT: 'development',
    DEVELOPER_MODE: 'false',
    JWT_SECRET: 'test-secret-key-for-developer-mode-min-32-chars!',
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

export async function bootstrapUser(env: Env, email: string, password: string, role: BootstrapRole): Promise<BootstrappedUser> {
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

  const data = await loginRes.json() as AuthLoginResponse

  return {
    email,
    role,
    token: data.data?.access_token || '',
  }
}

export async function bootstrapPrincipals(
  env: Env,
  prefix = 'visualizer',
  password = 'VisualizerPass123!',
): Promise<BootstrappedPrincipals> {
  const admin = await bootstrapUser(env, `${prefix}-admin@example.com`, password, 'admin')
  const operator = await bootstrapUser(env, `${prefix}-operator@example.com`, password, 'operator')
  const viewer = await bootstrapUser(env, `${prefix}-viewer@example.com`, password, 'viewer')

  return {
    admin,
    operator,
    viewer,
  }
}
