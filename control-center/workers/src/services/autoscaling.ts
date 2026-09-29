/**
 * Auto-Scaling Service
 *
 * Provides automatic scaling based on metrics and thresholds
 */

import type { Env } from '../types'

// Scaling policy
export interface ScalingPolicy {
  id: string
  name: string
  targetType: 'node' | 'service' | 'deployment'
  targetId: string
  namespace?: string
  minReplicas: number
  maxReplicas: number
  metrics: ScalingMetric[]
  enabled: boolean
  cooldownSeconds: number
  lastScaledAt?: string
}

// Scaling metric
export interface ScalingMetric {
  type: 'cpu' | 'memory' | 'requests' | 'custom'
  targetValue: number
  currentValue?: number
  threshold?: number
  query?: string
}

// Scaling event
export interface ScalingEvent {
  id: string
  policyId: string
  timestamp: string
  action: 'scale_up' | 'scale_down'
  fromReplicas: number
  toReplicas: number
  reason: string
  metrics: Record<string, number>
}

// Scaling decision
export interface ScalingDecision {
  shouldScale: boolean
  action?: 'scale_up' | 'scale_down'
  targetReplicas?: number
  currentReplicas?: number
  reason?: string
  metrics?: Record<string, number>
}

// Health check result
export interface HealthCheckResult {
  healthy: boolean
  score: number
  details: {
    cpu: number
    memory: number
    requests: number
    errors: number
  }
}

/**
 * Create scaling policy
 */
export async function createScalingPolicy(
  env: Env,
  policy: Omit<ScalingPolicy, 'id'>
): Promise<{ success: boolean; policy?: ScalingPolicy; error?: string }> {
  try {
    // Validate policy
    if (policy.minReplicas < 1) {
      return { success: false, error: 'minReplicas must be >= 1' }
    }
    if (policy.maxReplicas < policy.minReplicas) {
      return { success: false, error: 'maxReplicas must be >= minReplicas' }
    }
    if (policy.metrics.length === 0) {
      return { success: false, error: 'At least one metric required' }
    }

    const id = generatePolicyId()
    const fullPolicy: ScalingPolicy = {
      ...policy,
      id,
    }

    await env.KV.put(
      `scaling:policy:${id}`,
      JSON.stringify(fullPolicy),
      { expirationTtl: 86400 * 365 }
    )

    // Index by target
    await env.KV.put(
      `scaling:target:${policy.targetType}:${policy.targetId}`,
      id,
      { expirationTtl: 86400 * 365 }
    )

    return { success: true, policy: fullPolicy }
  } catch (err) {
    return {
      success: false,
      error: err instanceof Error ? err.message : 'Unknown error',
    }
  }
}

/**
 * Get scaling policy
 */
export async function getScalingPolicy(
  env: Env,
  id: string
): Promise<ScalingPolicy | null> {
  const policy = await env.KV.get(`scaling:policy:${id}`, 'json')
  return policy as ScalingPolicy | null
}

/**
 * List scaling policies
 */
export async function listScalingPolicies(
  env: Env,
  targetType?: string
): Promise<ScalingPolicy[]> {
  const policies: ScalingPolicy[] = []
  const listResult = await env.KV.list({ prefix: 'scaling:policy:' })

  for (const key of listResult.keys) {
    const policy = await env.KV.get(key.name, 'json') as ScalingPolicy | null
    if (policy) {
      if (!targetType || policy.targetType === targetType) {
        policies.push(policy)
      }
    }
  }

  return policies
}

/**
 * Update scaling policy
 */
export async function updateScalingPolicy(
  env: Env,
  id: string,
  updates: Partial<ScalingPolicy>
): Promise<{ success: boolean; policy?: ScalingPolicy; error?: string }> {
  const existing = await getScalingPolicy(env, id)
  if (!existing) {
    return { success: false, error: 'Policy not found' }
  }

  const updated: ScalingPolicy = {
    ...existing,
    ...updates,
    id, // Ensure ID cannot be changed
  }

  // Validate updates
  if (updated.minReplicas > updated.maxReplicas) {
    return { success: false, error: 'minReplicas cannot exceed maxReplicas' }
  }

  await env.KV.put(
    `scaling:policy:${id}`,
    JSON.stringify(updated),
    { expirationTtl: 86400 * 365 }
  )

  return { success: true, policy: updated }
}

/**
 * Delete scaling policy
 */
export async function deleteScalingPolicy(
  env: Env,
  id: string
): Promise<{ success: boolean }> {
  const policy = await getScalingPolicy(env, id)
  if (policy) {
    await env.KV.delete(`scaling:target:${policy.targetType}:${policy.targetId}`)
  }
  await env.KV.delete(`scaling:policy:${id}`)
  return { success: true }
}

/**
 * Evaluate scaling policy
 */
export async function evaluateScalingPolicy(
  env: Env,
  policy: ScalingPolicy
): Promise<ScalingDecision> {
  try {
    // Get current metrics
    const metrics = await getCurrentMetrics(env, policy)
    let currentReplicas = await getCurrentReplicas(env, policy)

    // Check each metric
    let shouldScaleUp = false
    let shouldScaleDown = true
    const reasonParts: string[] = []

    for (const metric of policy.metrics) {
      const currentValue = metrics[metric.type] || 0
      const targetValue = metric.targetValue

      if (currentValue > targetValue) {
        shouldScaleUp = true
        reasonParts.push(`${metric.type}: ${currentValue.toFixed(2)} > ${targetValue}`)
      } else {
        shouldScaleDown = false
      }
    }

    // Check cooldown
    if (policy.lastScaledAt) {
      const lastScaled = new Date(policy.lastScaledAt).getTime()
      const cooldownMs = policy.cooldownSeconds * 1000
      if (Date.now() - lastScaled < cooldownMs) {
        return { shouldScale: false, currentReplicas }
      }
    }

    // Make decision
    if (shouldScaleUp && currentReplicas < policy.maxReplicas) {
      return {
        shouldScale: true,
        action: 'scale_up',
        targetReplicas: Math.min(currentReplicas + 1, policy.maxReplicas),
        currentReplicas,
        reason: reasonParts.join(', '),
        metrics,
      }
    }

    if (shouldScaleDown && currentReplicas > policy.minReplicas) {
      return {
        shouldScale: true,
        action: 'scale_down',
        targetReplicas: Math.max(currentReplicas - 1, policy.minReplicas),
        currentReplicas,
        reason: 'Metrics below target',
        metrics,
      }
    }

    return { shouldScale: false, currentReplicas }
  } catch (err) {
    return { shouldScale: false }
  }
}

/**
 * Execute scaling action
 */
export async function executeScalingAction(
  env: Env,
  policyId: string,
  decision: ScalingDecision
): Promise<{ success: boolean; event?: ScalingEvent; error?: string }> {
  if (!decision.shouldScale || !decision.action || !decision.targetReplicas) {
    return { success: false, error: 'No scaling action needed' }
  }

  const policy = await getScalingPolicy(env, policyId)
  if (!policy) {
    return { success: false, error: 'Policy not found' }
  }

  // Create scaling event
  const event: ScalingEvent = {
    id: generateEventId(),
    policyId,
    timestamp: new Date().toISOString(),
    action: decision.action,
    fromReplicas: decision.currentReplicas || policy.minReplicas,
    toReplicas: decision.targetReplicas,
    reason: decision.reason || '',
    metrics: decision.metrics || {},
  }

  // Store event
  await env.KV.put(
    `scaling:event:${event.id}`,
    JSON.stringify(event),
    { expirationTtl: 86400 * 30 }
  )

  // Update policy last scaled time
  await updateScalingPolicy(env, policyId, {
    lastScaledAt: event.timestamp,
  })

  // Store in event history
  const historyKey = `scaling:history:${policyId}`
  const history = await env.KV.get(historyKey, 'json') as string[] | null
  const eventIds = history || []
  eventIds.unshift(event.id)
  if (eventIds.length > 100) eventIds.pop()
  await env.KV.put(historyKey, JSON.stringify(eventIds), { expirationTtl: 86400 * 30 })

  return { success: true, event }
}

/**
 * Get scaling history
 */
export async function getScalingHistory(
  env: Env,
  policyId: string,
  limit: number = 50
): Promise<ScalingEvent[]> {
  const historyKey = `scaling:history:${policyId}`
  const history = await env.KV.get(historyKey, 'json') as string[] | null
  const eventIds = (history || []).slice(0, limit)

  const events: ScalingEvent[] = []
  for (const id of eventIds) {
    const event = await env.KV.get(`scaling:event:${id}`, 'json') as ScalingEvent | null
    if (event) events.push(event)
  }

  return events
}

/**
 * Check health score
 */
export async function checkHealth(
  env: Env,
  targetType: string,
  targetId: string
): Promise<HealthCheckResult> {
  // Get metrics
  const cpu = Math.random() * 100 // Mock
  const memory = Math.random() * 100 // Mock
  const requests = Math.random() * 1000 // Mock
  const errors = Math.random() * 10 // Mock

  // Calculate health score
  const cpuScore = Math.max(0, 100 - cpu)
  const memoryScore = Math.max(0, 100 - memory)
  const errorScore = Math.max(0, 100 - errors * 10)
  const score = (cpuScore + memoryScore + errorScore) / 3

  return {
    healthy: score > 50,
    score,
    details: { cpu, memory, requests, errors },
  }
}

/**
 * Get recommended replicas
 */
export async function getRecommendedReplicas(
  env: Env,
  policy: ScalingPolicy
): Promise<number> {
  const decision = await evaluateScalingPolicy(env, policy)
  return decision.targetReplicas || decision.currentReplicas || policy.minReplicas
}

/**
 * Run scaling check for all policies
 */
export async function runScalingCheck(
  env: Env
): Promise<{ checked: number; scaled: number; errors: string[] }> {
  const policies = await listScalingPolicies(env)
  const errors: string[] = []
  let scaled = 0

  for (const policy of policies) {
    if (!policy.enabled) continue

    try {
      const decision = await evaluateScalingPolicy(env, policy)
      if (decision.shouldScale) {
        const result = await executeScalingAction(env, policy.id, decision)
        if (result.success) {
          scaled++
        } else {
          errors.push(`Policy ${policy.name}: ${result.error}`)
        }
      }
    } catch (err) {
      errors.push(`Policy ${policy.name}: ${err instanceof Error ? err.message : 'Unknown error'}`)
    }
  }

  return { checked: policies.filter(p => p.enabled).length, scaled, errors }
}

// Helper functions

function generatePolicyId(): string {
  return `sp_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

function generateEventId(): string {
  return `se_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

async function getCurrentMetrics(
  env: Env,
  policy: ScalingPolicy
): Promise<Record<string, number>> {
  const metrics: Record<string, number> = {}

  for (const metric of policy.metrics) {
    // Mock current values
    switch (metric.type) {
      case 'cpu':
        metrics.cpu = Math.random() * 100
        break
      case 'memory':
        metrics.memory = Math.random() * 100
        break
      case 'requests':
        metrics.requests = Math.random() * 1000
        break
      case 'custom':
        if (metric.query) {
          metrics.custom = Math.random() * 100
        }
        break
    }
  }

  return metrics
}

async function getCurrentReplicas(
  env: Env,
  policy: ScalingPolicy
): Promise<number> {
  // Mock: return current replica count
  const key = `scaling:replicas:${policy.targetType}:${policy.targetId}`
  const replicas = await env.KV.get(key, 'json') as number | null
  return replicas || policy.minReplicas
}