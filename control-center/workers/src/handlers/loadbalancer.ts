/**
 * Load Balancing API Handlers
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'
import {
  createLoadBalancer,
  getLoadBalancer,
  listLoadBalancers,
  updateLoadBalancer,
  deleteLoadBalancer,
  selectTarget,
  checkTargetHealth,
  runHealthChecks,
  getLoadBalancerStats,
  recordRequestCompletion,
  addTarget,
  removeTarget,
  updateTargetWeight,
} from '../services/loadbalancer'

/**
 * List load balancers
 */
export async function listLoadBalancersHandler(c: Context<{ Bindings: Env }>) {
  const lbs = await listLoadBalancers(c.env)

  return c.json({
    success: true,
    data: lbs,
    total: lbs.length,
  })
}

/**
 * Get load balancer
 */
export async function getLoadBalancerHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const lb = await getLoadBalancer(c.env, id)

  if (!lb) {
    return c.json({ success: false, error: 'Load balancer not found' }, 404)
  }

  return c.json({ success: true, data: lb })
}

/**
 * Create load balancer
 */
export async function createLoadBalancerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  if (!body.name) {
    return c.json({ success: false, error: 'name is required' }, 400)
  }

  const result = await createLoadBalancer(c.env, {
    name: body.name,
    algorithm: body.algorithm || 'round-robin',
    targets: body.targets || [],
    healthCheck: body.healthCheck || {
      enabled: true,
      interval: 30,
      timeout: 5,
      unhealthyThreshold: 3,
      healthyThreshold: 2,
      path: '/health',
      expectedStatus: [200],
    },
    stickySession: body.stickySession,
    enabled: body.enabled !== false,
  })

  if (result.success) {
    await logAudit(c, user?.sub, 'create_load_balancer', 'loadbalancer', {
      lbId: result.lb?.id,
      name: body.name,
      algorithm: body.algorithm,
    })
  }

  return c.json(result, result.success ? 201 : 400)
}

/**
 * Update load balancer
 */
export async function updateLoadBalancerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const body = await c.req.json()

  const result = await updateLoadBalancer(c.env, id, body)

  if (result.success) {
    await logAudit(c, user?.sub, 'update_load_balancer', 'loadbalancer', {
      lbId: id,
      updates: body,
    })
  }

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Delete load balancer
 */
export async function deleteLoadBalancerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string

  const result = await deleteLoadBalancer(c.env, id)

  await logAudit(c, user?.sub, 'delete_load_balancer', 'loadbalancer', {
    lbId: id,
  })

  return c.json(result)
}

/**
 * Select target
 */
export async function selectTargetHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const clientIp = c.req.query('clientIp')

  const selection = await selectTarget(c.env, id, clientIp)

  if (!selection) {
    return c.json({ success: false, error: 'No available targets' }, 503)
  }

  return c.json({
    success: true,
    data: selection,
  })
}

/**
 * Check target health
 */
export async function checkTargetHealthHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const targetId = c.req.param('targetId') as string

  const result = await checkTargetHealth(c.env, id, targetId)

  return c.json({
    success: true,
    data: result,
  })
}

/**
 * Run health checks
 */
export async function runHealthChecksHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const result = await runHealthChecks(c.env, id)

  return c.json({
    success: true,
    data: result,
  })
}

/**
 * Get load balancer stats
 */
export async function getLoadBalancerStatsHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string

  const stats = await getLoadBalancerStats(c.env, id)

  return c.json({
    success: true,
    data: stats,
  })
}

/**
 * Add target
 */
export async function addTargetHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const body = await c.req.json()

  if (!body.address || !body.port) {
    return c.json({ success: false, error: 'address and port are required' }, 400)
  }

  const result = await addTarget(c.env, id, {
    name: body.name,
    address: body.address,
    port: body.port,
    weight: body.weight || 50,
    responseTime: body.responseTime,
    metadata: body.metadata,
  })

  if (result.success) {
    await logAudit(c, user?.sub, 'add_lb_target', 'loadbalancer', {
      lbId: id,
      targetId: result.target?.id,
      address: body.address,
    })
  }

  return c.json(result, result.success ? 201 : 400)
}

/**
 * Remove target
 */
export async function removeTargetHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const targetId = c.req.param('targetId') as string

  const result = await removeTarget(c.env, id, targetId)

  if (result.success) {
    await logAudit(c, user?.sub, 'remove_lb_target', 'loadbalancer', {
      lbId: id,
      targetId,
    })
  }

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Update target weight
 */
export async function updateTargetWeightHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const targetId = c.req.param('targetId') as string
  const body = await c.req.json()

  if (typeof body.weight !== 'number') {
    return c.json({ success: false, error: 'weight is required and must be a number' }, 400)
  }

  const result = await updateTargetWeight(c.env, id, targetId, body.weight)

  if (result.success) {
    await logAudit(c, user?.sub, 'update_target_weight', 'loadbalancer', {
      lbId: id,
      targetId,
      weight: body.weight,
    })
  }

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Record request completion
 */
export async function recordCompletionHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const targetId = c.req.param('targetId') as string
  const body = await c.req.json()

  await recordRequestCompletion(c.env, id, targetId, body.responseTime || 0)

  return c.json({ success: true })
}

/**
 * Toggle load balancer
 */
export async function toggleLoadBalancerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string

  const lb = await getLoadBalancer(c.env, id)
  if (!lb) {
    return c.json({ success: false, error: 'Load balancer not found' }, 404)
  }

  const result = await updateLoadBalancer(c.env, id, {
    enabled: !lb.enabled,
  })

  if (result.success) {
    await logAudit(c, user?.sub, 'toggle_load_balancer', 'loadbalancer', {
      lbId: id,
      enabled: !lb.enabled,
    })
  }

  return c.json(result)
}