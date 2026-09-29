import type { Context } from 'hono'
import type { Env, HealthResponse, ReadinessResponse, ServiceErrorResponse } from '../types'
import { probeRuntimeServices } from '../services/monitoring'

/**
 * 健康检查
 */
export async function healthHandler(c: Context<{ Bindings: Env }>) {
  return c.json({
    status: 'healthy',
    version: c.env.APP_VERSION || '1.0.0',
    build_sha: c.env.BUILD_SHA || 'unknown',
    timestamp: new Date().toISOString(),
    environment: c.env.ENVIRONMENT,
  } as HealthResponse)
}

/**
 * 就绪检查
 */
export async function readinessHandler(c: Context<{ Bindings: Env }>) {
  const checks = await probeRuntimeServices(c.env)
  const allHealthy = [checks.database, checks.kv, checks.r2].every(check => check.status === 'healthy')

  return c.json({
    status: allHealthy ? 'ready' : 'degraded',
    version: c.env.APP_VERSION || '1.0.0',
    build_sha: c.env.BUILD_SHA || 'unknown',
    checks,
    timestamp: new Date().toISOString(),
  } as ReadinessResponse, allHealthy ? 200 : 503)
}
