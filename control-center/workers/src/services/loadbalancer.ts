/**
 * Load Balancing Service
 *
 * Provides intelligent load balancing across multiple targets
 */

import type { Env } from '../types'

// Backend target
export interface BackendTarget {
  id: string
  name: string
  address: string
  port: number
  weight: number
  healthy: boolean
  connections: number
  lastHealthCheck?: string
  responseTime?: number
  metadata?: Record<string, string>
}

// Load balancer configuration
export interface LoadBalancerConfig {
  id: string
  name: string
  algorithm: 'round-robin' | 'weighted' | 'least-connections' | 'ip-hash' | 'random' | 'response-time'
  targets: BackendTarget[]
  healthCheck: HealthCheckConfig
  stickySession?: StickySessionConfig
  enabled: boolean
  createdAt: string
  updatedAt: string
}

// Health check configuration
export interface HealthCheckConfig {
  enabled: boolean
  interval: number
  timeout: number
  unhealthyThreshold: number
  healthyThreshold: number
  path: string
  expectedStatus: number[]
}

// Sticky session configuration
export interface StickySessionConfig {
  enabled: boolean
  cookieName: string
  ttl: number
}

// Load balancing statistics
export interface LoadBalancerStats {
  totalRequests: number
  totalConnections: number
  activeConnections: number
  requestsPerTarget: Record<string, number>
  avgResponseTime: number
  healthyTargets: number
  unhealthyTargets: number
}

// Target selection result
export interface TargetSelection {
  target: BackendTarget
  algorithm: string
  reason: string
}

// Round-robin state
const roundRobinCounters = new Map<string, number>()

/**
 * Create load balancer
 */
export async function createLoadBalancer(
  env: Env,
  config: Omit<LoadBalancerConfig, 'id' | 'createdAt' | 'updatedAt'>
): Promise<{ success: boolean; lb?: LoadBalancerConfig; error?: string }> {
  try {
    // Validate configuration
    if (!config.name) {
      return { success: false, error: 'Name is required' }
    }

    if (config.targets.length === 0) {
      return { success: false, error: 'At least one target is required' }
    }

    const validAlgorithms = ['round-robin', 'weighted', 'least-connections', 'ip-hash', 'random', 'response-time']
    if (!validAlgorithms.includes(config.algorithm)) {
      return { success: false, error: `Invalid algorithm. Must be one of: ${validAlgorithms.join(', ')}` }
    }

    // Validate targets
    let totalWeight = 0
    for (const target of config.targets) {
      if (!target.address || !target.port) {
        return { success: false, error: 'Each target must have address and port' }
      }
      if (target.weight < 0 || target.weight > 100) {
        return { success: false, error: 'Weight must be between 0 and 100' }
      }
      totalWeight += target.weight
    }

    if (config.algorithm === 'weighted' && totalWeight !== 100) {
      return { success: false, error: 'Weights must sum to 100 for weighted algorithm' }
    }

    const id = generateLBId()
    const now = new Date().toISOString()

    const lb: LoadBalancerConfig = {
      ...config,
      id,
      createdAt: now,
      updatedAt: now,
    }

    await env.KV.put(
      `lb:config:${id}`,
      JSON.stringify(lb),
      { expirationTtl: 86400 * 365 }
    )

    return { success: true, lb }
  } catch (err) {
    return {
      success: false,
      error: err instanceof Error ? err.message : 'Unknown error',
    }
  }
}

/**
 * Get load balancer
 */
export async function getLoadBalancer(
  env: Env,
  id: string
): Promise<LoadBalancerConfig | null> {
  const lb = await env.KV.get(`lb:config:${id}`, 'json')
  return lb as LoadBalancerConfig | null
}

/**
 * List load balancers
 */
export async function listLoadBalancers(
  env: Env
): Promise<LoadBalancerConfig[]> {
  const lbs: LoadBalancerConfig[] = []
  const listResult = await env.KV.list({ prefix: 'lb:config:' })

  for (const key of listResult.keys) {
    const lb = await env.KV.get(key.name, 'json') as LoadBalancerConfig | null
    if (lb) lbs.push(lb)
  }

  return lbs
}

/**
 * Update load balancer
 */
export async function updateLoadBalancer(
  env: Env,
  id: string,
  updates: Partial<LoadBalancerConfig>
): Promise<{ success: boolean; lb?: LoadBalancerConfig; error?: string }> {
  const existing = await getLoadBalancer(env, id)
  if (!existing) {
    return { success: false, error: 'Load balancer not found' }
  }

  const updated: LoadBalancerConfig = {
    ...existing,
    ...updates,
    id,
    updatedAt: new Date().toISOString(),
  }

  await env.KV.put(
    `lb:config:${id}`,
    JSON.stringify(updated),
    { expirationTtl: 86400 * 365 }
  )

  return { success: true, lb: updated }
}

/**
 * Delete load balancer
 */
export async function deleteLoadBalancer(
  env: Env,
  id: string
): Promise<{ success: boolean }> {
  await env.KV.delete(`lb:config:${id}`)
  await env.KV.delete(`lb:stats:${id}`)
  roundRobinCounters.delete(id)
  return { success: true }
}

/**
 * Select target for request
 */
export async function selectTarget(
  env: Env,
  lbId: string,
  clientIp?: string
): Promise<TargetSelection | null> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb || !lb.enabled) {
    return null
  }

  // Filter healthy targets
  const healthyTargets = lb.targets.filter(t => t.healthy)
  if (healthyTargets.length === 0) {
    return null
  }

  let selected: BackendTarget

  switch (lb.algorithm) {
    case 'round-robin':
      selected = selectRoundRobin(lbId, healthyTargets)
      break

    case 'weighted':
      selected = selectWeighted(healthyTargets)
      break

    case 'least-connections':
      selected = selectLeastConnections(healthyTargets)
      break

    case 'ip-hash':
      selected = selectIpHash(healthyTargets, clientIp || '0.0.0.0')
      break

    case 'random':
      selected = selectRandom(healthyTargets)
      break

    case 'response-time':
      selected = selectResponseTime(healthyTargets)
      break

    default:
      selected = selectRoundRobin(lbId, healthyTargets)
  }

  // Update connection count
  await incrementConnections(env, lbId, selected.id)

  return {
    target: selected,
    algorithm: lb.algorithm,
    reason: `Selected based on ${lb.algorithm} algorithm`,
  }
}

/**
 * Health check for target
 */
export async function checkTargetHealth(
  env: Env,
  lbId: string,
  targetId: string
): Promise<{ healthy: boolean; responseTime: number; error?: string }> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return { healthy: false, responseTime: 0, error: 'Load balancer not found' }
  }

  const target = lb.targets.find(t => t.id === targetId)
  if (!target) {
    return { healthy: false, responseTime: 0, error: 'Target not found' }
  }

  // Simulate health check
  const startTime = Date.now()
  const healthy = Math.random() > 0.1 // 90% success rate
  const responseTime = Date.now() - startTime + Math.random() * 50

  // Update target health status
  const updatedTargets = lb.targets.map(t => {
    if (t.id === targetId) {
      return {
        ...t,
        healthy,
        responseTime,
        lastHealthCheck: new Date().toISOString(),
      }
    }
    return t
  })

  await updateLoadBalancer(env, lbId, { targets: updatedTargets })

  return { healthy, responseTime }
}

/**
 * Run health checks for all targets
 */
export async function runHealthChecks(
  env: Env,
  lbId: string
): Promise<{ checked: number; healthy: number; unhealthy: number }> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return { checked: 0, healthy: 0, unhealthy: 0 }
  }

  let healthy = 0
  let unhealthy = 0

  for (const target of lb.targets) {
    const result = await checkTargetHealth(env, lbId, target.id)
    if (result.healthy) {
      healthy++
    } else {
      unhealthy++
    }
  }

  return { checked: lb.targets.length, healthy, unhealthy }
}

/**
 * Get load balancer statistics
 */
export async function getLoadBalancerStats(
  env: Env,
  lbId: string
): Promise<LoadBalancerStats> {
  const statsKey = `lb:stats:${lbId}`
  const stats = await env.KV.get(statsKey, 'json') as Partial<LoadBalancerStats> | null

  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return {
      totalRequests: 0,
      totalConnections: 0,
      activeConnections: 0,
      requestsPerTarget: {},
      avgResponseTime: 0,
      healthyTargets: 0,
      unhealthyTargets: 0,
    }
  }

  return {
    totalRequests: stats?.totalRequests || 0,
    totalConnections: stats?.totalConnections || 0,
    activeConnections: stats?.activeConnections || 0,
    requestsPerTarget: stats?.requestsPerTarget || {},
    avgResponseTime: stats?.avgResponseTime || 0,
    healthyTargets: lb.targets.filter(t => t.healthy).length,
    unhealthyTargets: lb.targets.filter(t => !t.healthy).length,
  }
}

/**
 * Record request completion
 */
export async function recordRequestCompletion(
  env: Env,
  lbId: string,
  targetId: string,
  responseTime: number
): Promise<void> {
  const statsKey = `lb:stats:${lbId}`
  const stats = await env.KV.get(statsKey, 'json') as Partial<LoadBalancerStats> | null

  const currentRequests = stats?.requestsPerTarget || {}
  currentRequests[targetId] = (currentRequests[targetId] || 0) + 1

  const newStats: LoadBalancerStats = {
    totalRequests: (stats?.totalRequests || 0) + 1,
    totalConnections: (stats?.totalConnections || 0) + 1,
    activeConnections: Math.max(0, (stats?.activeConnections || 0) - 1),
    requestsPerTarget: currentRequests,
    avgResponseTime: ((stats?.avgResponseTime || 0) + responseTime) / 2,
    healthyTargets: stats?.healthyTargets || 0,
    unhealthyTargets: stats?.unhealthyTargets || 0,
  }

  await env.KV.put(statsKey, JSON.stringify(newStats), { expirationTtl: 86400 })
}

/**
 * Add target to load balancer
 */
export async function addTarget(
  env: Env,
  lbId: string,
  target: Omit<BackendTarget, 'id' | 'healthy' | 'connections'>
): Promise<{ success: boolean; target?: BackendTarget; error?: string }> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return { success: false, error: 'Load balancer not found' }
  }

  const newTarget: BackendTarget = {
    ...target,
    id: generateTargetId(),
    healthy: true,
    connections: 0,
  }

  const updatedTargets = [...lb.targets, newTarget]
  await updateLoadBalancer(env, lbId, { targets: updatedTargets })

  return { success: true, target: newTarget }
}

/**
 * Remove target from load balancer
 */
export async function removeTarget(
  env: Env,
  lbId: string,
  targetId: string
): Promise<{ success: boolean; error?: string }> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return { success: false, error: 'Load balancer not found' }
  }

  const updatedTargets = lb.targets.filter(t => t.id !== targetId)
  if (updatedTargets.length === lb.targets.length) {
    return { success: false, error: 'Target not found' }
  }

  await updateLoadBalancer(env, lbId, { targets: updatedTargets })
  return { success: true }
}

/**
 * Update target weight
 */
export async function updateTargetWeight(
  env: Env,
  lbId: string,
  targetId: string,
  weight: number
): Promise<{ success: boolean; error?: string }> {
  if (weight < 0 || weight > 100) {
    return { success: false, error: 'Weight must be between 0 and 100' }
  }

  const lb = await getLoadBalancer(env, lbId)
  if (!lb) {
    return { success: false, error: 'Load balancer not found' }
  }

  const updatedTargets = lb.targets.map(t => {
    if (t.id === targetId) {
      return { ...t, weight }
    }
    return t
  })

  await updateLoadBalancer(env, lbId, { targets: updatedTargets })
  return { success: true }
}

// Algorithm implementations

function selectRoundRobin(lbId: string, targets: BackendTarget[]): BackendTarget {
  const counter = roundRobinCounters.get(lbId) || 0
  const index = counter % targets.length
  roundRobinCounters.set(lbId, counter + 1)
  return targets[index]
}

function selectWeighted(targets: BackendTarget[]): BackendTarget {
  const totalWeight = targets.reduce((sum, t) => sum + t.weight, 0)
  let random = Math.random() * totalWeight

  for (const target of targets) {
    random -= target.weight
    if (random <= 0) {
      return target
    }
  }

  return targets[0]
}

function selectLeastConnections(targets: BackendTarget[]): BackendTarget {
  return targets.reduce((min, t) =>
    t.connections < min.connections ? t : min
  )
}

function selectIpHash(targets: BackendTarget[], clientIp: string): BackendTarget {
  const hash = clientIp.split('.').reduce((sum, octet) => sum + parseInt(octet, 10), 0)
  return targets[hash % targets.length]
}

function selectRandom(targets: BackendTarget[]): BackendTarget {
  return targets[Math.floor(Math.random() * targets.length)]
}

function selectResponseTime(targets: BackendTarget[]): BackendTarget {
  return targets.reduce((best, t) => {
    if (!best.responseTime) return t
    if (!t.responseTime) return best
    return t.responseTime < best.responseTime ? t : best
  })
}

// Helper functions

function generateLBId(): string {
  return `lb_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

function generateTargetId(): string {
  return `tgt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

async function incrementConnections(
  env: Env,
  lbId: string,
  targetId: string
): Promise<void> {
  const lb = await getLoadBalancer(env, lbId)
  if (!lb) return

  const updatedTargets = lb.targets.map(t => {
    if (t.id === targetId) {
      return { ...t, connections: t.connections + 1 }
    }
    return t
  })

  await updateLoadBalancer(env, lbId, { targets: updatedTargets })
}