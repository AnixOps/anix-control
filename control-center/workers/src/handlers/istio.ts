/**
 * Istio Service Mesh API Handlers
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'
import {
  listVirtualServices,
  listDestinationRules,
  listGateways,
  listMeshServices,
  configureTrafficSplit,
  configureCircuitBreaker,
  getMeshMetrics,
  injectFault,
} from '../services/istio'

/**
 * List mesh services
 */
export async function listMeshServicesHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const services = await listMeshServices(c.env, namespace)
  return c.json({ success: true, data: services, namespace })
}

/**
 * List VirtualServices
 */
export async function listVirtualServicesHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const virtualServices = await listVirtualServices(c.env, namespace)
  return c.json({ success: true, data: virtualServices, namespace })
}

/**
 * List DestinationRules
 */
export async function listDestinationRulesHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const destinationRules = await listDestinationRules(c.env, namespace)
  return c.json({ success: true, data: destinationRules, namespace })
}

/**
 * List Gateways
 */
export async function listGatewaysHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const gateways = await listGateways(c.env, namespace)
  return c.json({ success: true, data: gateways, namespace })
}

/**
 * Configure traffic split
 */
export async function configureTrafficSplitHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  // Validate request
  if (!body.service || !body.subsets || !Array.isArray(body.subsets)) {
    return c.json({ success: false, error: 'service and subsets required' }, 400)
  }

  const result = await configureTrafficSplit(c.env, {
    service: body.service,
    namespace: body.namespace || 'default',
    subsets: body.subsets,
  })

  await logAudit(c, user?.sub, 'configure_traffic_split', 'istio', {
    service: body.service,
    subsets: body.subsets,
    success: result.success,
  })

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Configure circuit breaker
 */
export async function configureCircuitBreakerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  if (!body.service) {
    return c.json({ success: false, error: 'service required' }, 400)
  }

  const result = await configureCircuitBreaker(c.env, {
    service: body.service,
    namespace: body.namespace || 'default',
    maxConnections: body.maxConnections || 100,
    maxPendingRequests: body.maxPendingRequests || 100,
    maxRequestsPerConnection: body.maxRequestsPerConnection || 1,
    consecutiveErrors: body.consecutiveErrors || 5,
    ejectionTime: body.ejectionTime || '30s',
    maxEjectionPercent: body.maxEjectionPercent || 50,
  })

  await logAudit(c, user?.sub, 'configure_circuit_breaker', 'istio', {
    service: body.service,
    success: result.success,
  })

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Inject fault
 */
export async function injectFaultHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  if (!body.service || !body.faultType) {
    return c.json({ success: false, error: 'service and faultType required' }, 400)
  }

  if (!['delay', 'abort'].includes(body.faultType)) {
    return c.json({ success: false, error: 'faultType must be delay or abort' }, 400)
  }

  const result = await injectFault(c.env, {
    namespace: body.namespace || 'default',
    service: body.service,
    faultType: body.faultType,
    percentage: body.percentage || 100,
    value: body.value,
    duration: body.duration || '60s',
  })

  await logAudit(c, user?.sub, 'inject_fault', 'istio', {
    service: body.service,
    faultType: body.faultType,
    percentage: body.percentage,
    duration: body.duration,
    success: result.success,
  })

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Get mesh overview
 */
export async function getMeshOverviewHandler(c: Context<{ Bindings: Env }>) {
  const metrics = await getMeshMetrics(c.env)

  return c.json({
    success: true,
    data: {
      metrics,
      status: metrics.healthyServices === metrics.services ? 'Healthy' : 'Degraded',
    },
  })
}