import { describe, it, expect, beforeEach } from 'vitest'
import app from '../index'
import type { AuthLoginResponse, Env } from '../types'
import { createMockD1, createMockKV, createMockR2 } from '../../test/setup'
import type { GovernancePolicy } from '../types'

function createEnv(): Env {
  return {
    ENVIRONMENT: 'development',
    JWT_SECRET: 'governance-test-secret-key-min-32-characters!',
    JWT_EXPIRE: '3600',
    API_KEY_SALT: 'governance-test-salt',
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

describe('governance handlers', () => {
  let env: Env
  let adminToken: string
  let operatorToken: string

  beforeEach(async () => {
    env = createEnv()
    adminToken = await registerAndLogin(env, 'gov-admin@example.com', 'GovAdmin123!', 'admin')
    operatorToken = await registerAndLogin(env, 'gov-operator@example.com', 'GovOperator123!', 'operator')
  })

  it('returns default policy when no custom policies exist', async () => {
    const res = await app.request('/api/v1/governance/policies/active', {
      method: 'GET',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: GovernancePolicy }
    expect(data.success).toBe(true)
    expect(data.data?.id).toBe('default-approval-policy')
    expect(data.data?.name).toContain('Default')
    expect(data.data?.rules.length).toBeGreaterThan(0)
  })

  it('lists policies including default', async () => {
    const res = await app.request('/api/v1/governance/policies', {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(data.success).toBe(true)
    expect((data.data || []).some(p => p.id === 'default-approval-policy')).toBe(true)
  })

  it('creates a custom policy', async () => {
    const res = await app.request('/api/v1/governance/policies', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Custom Approval Policy',
        description: 'A custom policy for testing',
        default_effect: 'deny',
        rules: [
          {
            name: 'Allow all admins',
            conditions: {},
            effect: 'allow',
            principals: { roles: ['admin'] },
            priority: 100,
          },
          {
            name: 'Deny all viewers',
            conditions: {},
            effect: 'deny',
            principals: { roles: ['viewer'] },
            priority: 10,
          },
        ],
      }),
    }, env)

    expect(res.status).toBe(201)
    const data = await res.json() as { success: boolean; data?: GovernancePolicy }
    expect(data.success).toBe(true)
    expect(data.data?.name).toBe('Custom Approval Policy')
    expect(data.data?.rules.length).toBe(2)
    expect(data.data?.version).toBe(1)
  })

  it('gets a specific policy by id', async () => {
    const res = await app.request('/api/v1/governance/policies/default-approval-policy', {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(res.status).toBe(200)
    const data = await res.json() as { success: boolean; data?: GovernancePolicy }
    expect(data.success).toBe(true)
    expect(data.data?.id).toBe('default-approval-policy')
  })

  it('updates a custom policy', async () => {
    const createRes = await app.request('/api/v1/governance/policies', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Policy to Update',
        default_effect: 'deny',
        rules: [
          {
            name: 'Initial rule',
            conditions: {},
            effect: 'allow',
            principals: { roles: ['admin'] },
          },
        ],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const policyId = createData.data?.id || ''

    const updateRes = await app.request(`/api/v1/governance/policies/${policyId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Updated Policy Name',
        rules: [
          {
            name: 'Updated rule',
            conditions: {},
            effect: 'allow',
            principals: { roles: ['admin', 'operator'] },
          },
        ],
      }),
    }, env)

    expect(updateRes.status).toBe(200)
    const updateData = await updateRes.json() as { success: boolean; data?: GovernancePolicy }
    expect(updateData.success).toBe(true)
    expect(updateData.data?.name).toBe('Updated Policy Name')
    expect(updateData.data?.version).toBe(2)
  })

  it('cannot update default policy', async () => {
    const res = await app.request('/api/v1/governance/policies/default-approval-policy', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({ name: 'Modified Default' }),
    }, env)

    expect(res.status).toBe(403)
  })

  it('deletes a custom policy', async () => {
    const createRes = await app.request('/api/v1/governance/policies', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        name: 'Policy to Delete',
        default_effect: 'deny',
        rules: [
          {
            name: 'Rule',
            conditions: {},
            effect: 'allow',
            principals: { roles: ['admin'] },
          },
        ],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const policyId = createData.data?.id || ''

    const deleteRes = await app.request(`/api/v1/governance/policies/${policyId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(deleteRes.status).toBe(200)

    const getRes = await app.request(`/api/v1/governance/policies/${policyId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(getRes.status).toBe(404)
  })

  it('cannot delete default policy', async () => {
    const res = await app.request('/api/v1/governance/policies/default-approval-policy', {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(res.status).toBe(403)
  })

  it('denies operator access to policy management', async () => {
    const res = await app.request('/api/v1/governance/policies', {
      method: 'GET',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(res.status).toBe(403)
  })
})

describe('governance policy evaluation', () => {
  let env: Env
  let adminToken: string
  let operatorToken: string
  let viewerToken: string

  beforeEach(async () => {
    env = createEnv()
    adminToken = await registerAndLogin(env, 'eval-admin@example.com', 'EvalAdmin123!', 'admin')
    operatorToken = await registerAndLogin(env, 'eval-operator@example.com', 'EvalOperator123!', 'operator')
    viewerToken = await registerAndLogin(env, 'eval-viewer@example.com', 'EvalViewer123!', 'viewer')
  })

  it('default policy allows admin to approve any incident', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Critical incident',
        source: 'test',
        severity: 'critical',
        action_type: 'scale_policy',
        action_ref: 'policy-1',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${adminToken}` },
    }, env)

    expect(approveRes.status).toBe(200)
  })

  it('default policy denies operator from approving critical incidents', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Critical incident',
        source: 'test',
        severity: 'critical',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })

  it('default policy allows operator to approve non-critical restart_deployment', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'High severity restart',
        source: 'test',
        severity: 'high',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(200)
  })

  it('default policy denies operator from approving scale_policy', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Medium scale policy',
        source: 'test',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: 'policy-1',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })

  it('default policy denies viewer from approving any incident', async () => {
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Low severity incident',
        source: 'test',
        severity: 'low',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${viewerToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })
})