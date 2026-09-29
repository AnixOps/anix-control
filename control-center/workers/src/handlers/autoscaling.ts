/**
 * Auto-Scaling API Handlers
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, ApiSuccessResponse, Env } from '../types'
import { logAudit } from '../utils/audit'
import {
  createScalingPolicy,
  getScalingPolicy,
  listScalingPolicies,
  updateScalingPolicy,
  deleteScalingPolicy,
  evaluateScalingPolicy,
  executeScalingAction,
  getScalingHistory,
  checkHealth,
  getRecommendedReplicas,
  runScalingCheck,
  type ScalingMetric,
} from '../services/autoscaling'

/**
 * List scaling policies
 */
export async function listScalingPoliciesHandler(c: Context<{ Bindings: Env }>) {
  const targetType = c.req.query('type')
  const policies = await listScalingPolicies(c.env, targetType)

  return c.json({
    success: true,
    data: policies,
    total: policies.length,
  })
}

/**
 * Get scaling policy
 */
export async function getScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const policy = await getScalingPolicy(c.env, id)

  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: policy } as ApiSuccessResponse<unknown>)
}

/**
 * Create scaling policy
 */
export async function createScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  // Validate required fields
  if (!body.name || !body.targetType || !body.targetId) {
    return c.json({ success: false, error: 'name, targetType, and targetId required' } as ApiErrorResponse, 400)
  }

  const validTargetTypes = ['node', 'service', 'deployment']
  if (!validTargetTypes.includes(body.targetType)) {
    return c.json({ success: false, error: `targetType must be one of: ${validTargetTypes.join(', ')}` } as ApiErrorResponse, 400)
  }

  const result = await createScalingPolicy(c.env, {
    name: body.name,
    targetType: body.targetType,
    targetId: body.targetId,
    namespace: body.namespace,
    minReplicas: body.minReplicas || 1,
    maxReplicas: body.maxReplicas || 10,
    metrics: body.metrics || [{ type: 'cpu', targetValue: 70 }],
    enabled: body.enabled !== false,
    cooldownSeconds: body.cooldownSeconds || 300,
  })

  if (result.success) {
    await logAudit(c, user?.sub, 'create_scaling_policy', 'autoscaling', {
      policyId: result.policy?.id,
      name: body.name,
      targetType: body.targetType,
    })
  }

  return c.json(result, result.success ? 201 : 400)
}

/**
 * Update scaling policy
 */
export async function updateScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const body = await c.req.json()

  const result = await updateScalingPolicy(c.env, id, body)

  if (result.success) {
    await logAudit(c, user?.sub, 'update_scaling_policy', 'autoscaling', {
      policyId: id,
      updates: body,
    })
  }

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Delete scaling policy
 */
export async function deleteScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string

  const result = await deleteScalingPolicy(c.env, id)

  await logAudit(c, user?.sub, 'delete_scaling_policy', 'autoscaling', {
    policyId: id,
  })

  return c.json(result)
}

/**
 * Evaluate scaling policy
 */
export async function evaluateScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const policy = await getScalingPolicy(c.env, id)

  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  const decision = await evaluateScalingPolicy(c.env, policy)

  return c.json({
    success: true,
    data: {
      policy: {
        id: policy.id,
        name: policy.name,
        targetType: policy.targetType,
        targetId: policy.targetId,
      },
      decision,
    },
  })
}

/**
 * Execute scaling action
 */
export async function executeScalingActionHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string
  const body = await c.req.json()

  const policy = await getScalingPolicy(c.env, id)
  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  // Get evaluation or use provided values
  const decision = body.decision || await evaluateScalingPolicy(c.env, policy)

  const result = await executeScalingAction(c.env, id, decision)

  if (result.success) {
    await logAudit(c, user?.sub, 'execute_scaling_action', 'autoscaling', {
      policyId: id,
      action: result.event?.action,
      fromReplicas: result.event?.fromReplicas,
      toReplicas: result.event?.toReplicas,
    })
  }

  return c.json(result, result.success ? 200 : 400)
}

/**
 * Get scaling history
 */
export async function getScalingHistoryHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const limit = c.req.query('limit') ? parseInt(c.req.query('limit')!) : 50

  const history = await getScalingHistory(c.env, id, limit)

  return c.json({
    success: true,
    data: history,
    total: history.length,
  })
}

/**
 * Check health
 */
export async function checkHealthHandler(c: Context<{ Bindings: Env }>) {
  const targetType = c.req.param('type') as string
  const targetId = c.req.param('id') as string

  const health = await checkHealth(c.env, targetType, targetId)

  return c.json({
    success: true,
    data: health,
  })
}

/**
 * Get recommended replicas
 */
export async function getRecommendedReplicasHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const policy = await getScalingPolicy(c.env, id)

  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  const recommended = await getRecommendedReplicas(c.env, policy)

  return c.json({
    success: true,
    data: {
      currentReplicas: policy.minReplicas,
      recommendedReplicas: recommended,
      minReplicas: policy.minReplicas,
      maxReplicas: policy.maxReplicas,
    },
  })
}

/**
 * Run scaling check
 */
export async function runScalingCheckHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const result = await runScalingCheck(c.env)

  await logAudit(c, user?.sub, 'run_scaling_check', 'autoscaling', {
    checked: result.checked,
    scaled: result.scaled,
    errors: result.errors.length,
  })

  return c.json({
    success: true,
    data: result,
  })
}

/**
 * Toggle scaling policy
 */
export async function toggleScalingPolicyHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const id = c.req.param('id') as string

  const policy = await getScalingPolicy(c.env, id)
  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  const result = await updateScalingPolicy(c.env, id, {
    enabled: !policy.enabled,
  })

  if (result.success) {
    await logAudit(c, user?.sub, 'toggle_scaling_policy', 'autoscaling', {
      policyId: id,
      enabled: !policy.enabled,
    })
  }

  return c.json(result)
}

/**
 * Get scaling metrics
 */
export async function getScalingMetricsHandler(c: Context<{ Bindings: Env }>) {
  const id = c.req.param('id') as string
  const policy = await getScalingPolicy(c.env, id)

  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  // Get current metrics
  const metrics: Record<string, { current: number; target: number }> = {}
  for (const metric of policy.metrics) {
    // Mock current values
    metrics[metric.type] = {
      current: Math.random() * 100,
      target: metric.targetValue,
    }
  }

  return c.json({
    success: true,
    data: {
      policy: {
        id: policy.id,
        name: policy.name,
      },
      metrics,
      lastScaledAt: policy.lastScaledAt,
    },
  })
}