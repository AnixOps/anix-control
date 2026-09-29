/**
 * Tenant Middleware
 *
 * Provides tenant context for multi-tenant requests.
 * Extracts tenant ID from:
 * 1. X-Tenant-ID header
 * 2. User's default tenant
 * 3. URL parameter
 */

import type { Context, Next } from 'hono'
import type { Env } from '../types'
import { getTenant, getUserTenantMembership, getTenantQuotas, checkQuota } from '../services/tenant'

// Extend Hono context with tenant info
declare module 'hono' {
  interface ContextVariableMap {
    tenant?: {
      id: number
      slug: string
      plan: string
    }
    quotas?: Record<string, number>
  }
}

/**
 * Middleware to set tenant context
 */
export async function tenantMiddleware(c: Context<{ Bindings: Env }>, next: Next) {
  const user = c.get('user')
  if (!user) {
    return next()
  }

  // Try to get tenant ID from various sources
  let tenantId: number | null = null

  // 1. From header
  const tenantHeader = c.req.header('X-Tenant-ID')
  if (tenantHeader) {
    tenantId = parseInt(tenantHeader, 10)
  }

  // 2. From URL parameter
  if (!tenantId) {
    const tenantParam = c.req.param('tenantId') as string
    if (tenantParam) {
      tenantId = parseInt(tenantParam, 10)
    }
  }

  // 3. From query parameter
  if (!tenantId) {
    const tenantQuery = c.req.query('tenant_id')
    if (tenantQuery) {
      tenantId = parseInt(tenantQuery, 10)
    }
  }

  // 4. From user's default tenant context

  // Validate tenant membership
  if (tenantId) {
    const membership = await getUserTenantMembership(c.env, user.sub, tenantId)

    if (membership) {
      const tenant = await getTenant(c.env, tenantId)

      if (tenant && tenant.status === 'active') {
        c.set('tenant', {
          id: tenant.id,
          slug: tenant.slug,
          plan: tenant.plan,
        })

        // Load quotas
        const quotas = await getTenantQuotas(c.env, tenantId)
        c.set('quotas', quotas as unknown as Record<string, number>)
      }
    }
  }

  return next()
}

/**
 * Require tenant context
 */
export async function requireTenant(c: Context<{ Bindings: Env }>, next: Next) {
  const tenant = c.get('tenant')

  if (!tenant) {
    return c.json({
      success: false,
      error: 'Tenant context required',
    }, 400)
  }

  return next()
}

/**
 * Check quota middleware factory
 */
export function quotaMiddleware(resource: string, increment: number = 1) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const tenant = c.get('tenant')

    if (!tenant) {
      return next()
    }

    const quotaCheck = await checkQuota(c.env, tenant.id, resource as any, increment)

    if (!quotaCheck.allowed) {
      return c.json({
        success: false,
        error: 'Quota exceeded',
        resource,
        current: quotaCheck.current,
        limit: quotaCheck.limit,
      }, 403)
    }

    return next()
  }
}

/**
 * Check tenant status middleware
 */
export async function checkTenantStatus(c: Context<{ Bindings: Env }>, next: Next) {
  const tenant = c.get('tenant')

  if (tenant) {
    const tenantData = await getTenant(c.env, tenant.id)

    if (!tenantData || tenantData.status === 'suspended') {
      return c.json({
        success: false,
        error: 'Tenant is suspended',
      }, 403)
    }

    if (tenantData.status === 'cancelled') {
      return c.json({
        success: false,
        error: 'Tenant is cancelled',
      }, 403)
    }
  }

  return next()
}

/**
 * Get current tenant from context
 */
export function getCurrentTenant(c: Context<{ Bindings: Env }>): { id: number; slug: string; plan: string } | undefined {
  return c.get('tenant')
}

/**
 * Get tenant ID from context
 */
export function getTenantId(c: Context<{ Bindings: Env }>): number | undefined {
  const tenant = c.get('tenant')
  return tenant?.id
}