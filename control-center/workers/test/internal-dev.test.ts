import { describe, expect, it } from 'vitest'
import app from '../src/index'
import type {
  DeveloperDiagnosticsResponse,
  DeveloperFixtureCatalogResponse,
  DeveloperModeStatusResponse,
  DeveloperReadinessSummaryResponse,
} from '../src/types'
import { bootstrapPrincipals, createTestEnv } from './helpers/fixtures'

describe('internal developer mode', () => {
  it('hides developer endpoints when developer mode is disabled', async () => {
    const env = createTestEnv({ DEVELOPER_MODE: 'false' })
    const principals = await bootstrapPrincipals(env, 'dev-disabled')

    const res = await app.request('/api/v1/internal/dev/status', {
      headers: { Authorization: `Bearer ${principals.admin.token}` },
    }, env)

    expect(res.status).toBe(404)
  })

  it('exposes developer endpoints to admin jwt users when enabled', async () => {
    const env = createTestEnv({ DEVELOPER_MODE: 'true' })
    const principals = await bootstrapPrincipals(env, 'dev-enabled')

    const res = await app.request('/api/v1/internal/dev/status', {
      headers: { Authorization: `Bearer ${principals.admin.token}` },
    }, env)

    expect(res.status).toBe(200)
    const body = await res.json() as DeveloperModeStatusResponse

    expect(body.success).toBe(true)
    expect(body.data.developer_mode).toBe(true)
    expect(body.data.actor.role).toBe('admin')
    expect(body.data.actor.auth_method).toBe('jwt')
  })

  it('rejects non-admin users even when developer mode is enabled', async () => {
    const env = createTestEnv({ DEVELOPER_MODE: 'true' })
    const principals = await bootstrapPrincipals(env, 'dev-viewer')

    const res = await app.request('/api/v1/internal/dev/status', {
      headers: { Authorization: `Bearer ${principals.viewer.token}` },
    }, env)

    expect(res.status).toBe(403)
  })

  it('returns diagnostics and fixture catalog for admin jwt users', async () => {
    const env = createTestEnv({ DEVELOPER_MODE: 'true' })
    const principals = await bootstrapPrincipals(env, 'dev-catalog')

    const diagnosticsRes = await app.request('/api/v1/internal/dev/diagnostics', {
      headers: { Authorization: `Bearer ${principals.admin.token}` },
    }, env)
    const fixturesRes = await app.request('/api/v1/internal/dev/fixtures', {
      headers: { Authorization: `Bearer ${principals.admin.token}` },
    }, env)

    expect(diagnosticsRes.status).toBe(200)
    expect(fixturesRes.status).toBe(200)

    const diagnostics = await diagnosticsRes.json() as DeveloperDiagnosticsResponse
    const fixtures = await fixturesRes.json() as DeveloperFixtureCatalogResponse

    expect(diagnostics.data.security.admin_only).toBe(true)
    expect(diagnostics.data.security.jwt_only).toBe(true)
    expect(fixtures.data.fixtures.some(item => item.key === 'principals' && item.status === 'available')).toBe(true)
  })

  it('returns the readiness summary for admin jwt users', async () => {
    const env = createTestEnv({ DEVELOPER_MODE: 'true' })
    const principals = await bootstrapPrincipals(env, 'dev-readiness')

    const res = await app.request('/api/v1/internal/dev/readiness-summary', {
      headers: { Authorization: `Bearer ${principals.admin.token}` },
    }, env)

    expect(res.status).toBe(200)

    const body = await res.json() as DeveloperReadinessSummaryResponse

    expect(body.success).toBe(true)
    expect(body.data.actor.role).toBe('admin')
    expect(body.data.readiness.status).toMatch(/ready|degraded/)
    expect(body.data.manifest.manifest_total).toBeGreaterThan(0)
    expect(body.data.manifest.ready_endpoints.length).toBeGreaterThan(0)
  })
})