/**
 * Auto-Scaling Service Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
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
  type ScalingPolicy,
  type ScalingMetric,
} from './autoscaling'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Auto-Scaling Service', () => {
  let mockEnv: any

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
      R2: {} as any,
    }
  })

  describe('createScalingPolicy', () => {
    it('should create a scaling policy', async () => {
      const result = await createScalingPolicy(mockEnv, {
        name: 'test-policy',
        targetType: 'deployment',
        targetId: 'test-deployment',
        namespace: 'default',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      expect(result.success).toBe(true)
      expect(result.policy).toBeDefined()
      expect(result.policy?.id).toMatch(/^sp_\d+_[a-z0-9]+$/)
      expect(result.policy?.name).toBe('test-policy')
    })

    it('should reject invalid minReplicas', async () => {
      const result = await createScalingPolicy(mockEnv, {
        name: 'invalid-policy',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 0,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('minReplicas')
    })

    it('should reject maxReplicas < minReplicas', async () => {
      const result = await createScalingPolicy(mockEnv, {
        name: 'invalid-policy',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 5,
        maxReplicas: 2,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('maxReplicas')
    })

    it('should require at least one metric', async () => {
      const result = await createScalingPolicy(mockEnv, {
        name: 'no-metrics',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [],
        enabled: true,
        cooldownSeconds: 300,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('metric')
    })

    it('should support multiple metrics', async () => {
      const result = await createScalingPolicy(mockEnv, {
        name: 'multi-metric-policy',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [
          { type: 'cpu', targetValue: 70 },
          { type: 'memory', targetValue: 80 },
        ],
        enabled: true,
        cooldownSeconds: 300,
      })

      expect(result.success).toBe(true)
      expect(result.policy?.metrics.length).toBe(2)
    })
  })

  describe('getScalingPolicy', () => {
    it('should return null for non-existent policy', async () => {
      const result = await getScalingPolicy(mockEnv, 'non-existent')
      expect(result).toBeNull()
    })

    it('should retrieve created policy', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'test-policy',
        targetType: 'node',
        targetId: 'node-1',
        minReplicas: 1,
        maxReplicas: 5,
        metrics: [{ type: 'cpu', targetValue: 60 }],
        enabled: true,
        cooldownSeconds: 180,
      })

      const retrieved = await getScalingPolicy(mockEnv, created.policy!.id)
      expect(retrieved).toBeDefined()
      expect(retrieved?.name).toBe('test-policy')
    })
  })

  describe('listScalingPolicies', () => {
    beforeEach(async () => {
      await createScalingPolicy(mockEnv, {
        name: 'node-policy',
        targetType: 'node',
        targetId: 'node-1',
        minReplicas: 1,
        maxReplicas: 5,
        metrics: [{ type: 'cpu', targetValue: 60 }],
        enabled: true,
        cooldownSeconds: 180,
      })

      await createScalingPolicy(mockEnv, {
        name: 'deployment-policy',
        targetType: 'deployment',
        targetId: 'dep-1',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'memory', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })
    })

    it('should list all policies', async () => {
      const policies = await listScalingPolicies(mockEnv)
      expect(policies.length).toBeGreaterThanOrEqual(2)
    })

    it('should filter by target type', async () => {
      const nodePolicies = await listScalingPolicies(mockEnv, 'node')
      expect(nodePolicies.every(p => p.targetType === 'node')).toBe(true)
    })
  })

  describe('updateScalingPolicy', () => {
    it('should update policy', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'update-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      const updated = await updateScalingPolicy(mockEnv, created.policy!.id, {
        maxReplicas: 20,
        cooldownSeconds: 600,
      })

      expect(updated.success).toBe(true)
      expect(updated.policy?.maxReplicas).toBe(20)
      expect(updated.policy?.cooldownSeconds).toBe(600)
    })

    it('should reject invalid update', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'invalid-update-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 5,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      const updated = await updateScalingPolicy(mockEnv, created.policy!.id, {
        minReplicas: 15,
      })

      expect(updated.success).toBe(false)
    })

    it('should return error for non-existent policy', async () => {
      const result = await updateScalingPolicy(mockEnv, 'non-existent', { enabled: false })
      expect(result.success).toBe(false)
      expect(result.error).toContain('not found')
    })
  })

  describe('deleteScalingPolicy', () => {
    it('should delete policy', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'delete-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      })

      const deleted = await deleteScalingPolicy(mockEnv, created.policy!.id)
      expect(deleted.success).toBe(true)

      const retrieved = await getScalingPolicy(mockEnv, created.policy!.id)
      expect(retrieved).toBeNull()
    })
  })

  describe('evaluateScalingPolicy', () => {
    it('should evaluate scaling decision', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'eval-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 0,
      })

      const decision = await evaluateScalingPolicy(mockEnv, created.policy!)
      expect(decision.shouldScale).toBeDefined()
      expect(decision.currentReplicas).toBeDefined()
    })

    it('should respect cooldown period', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'cooldown-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 3600,
        lastScaledAt: new Date().toISOString(),
      })

      const decision = await evaluateScalingPolicy(mockEnv, created.policy!)
      expect(decision.shouldScale).toBe(false)
    })
  })

  describe('executeScalingAction', () => {
    it('should return error when no scaling needed', async () => {
      const result = await executeScalingAction(mockEnv, 'test-id', {
        shouldScale: false,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('No scaling action')
    })

    it('should create scaling event', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'execute-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 0,
      })

      const result = await executeScalingAction(mockEnv, created.policy!.id, {
        shouldScale: true,
        action: 'scale_up',
        targetReplicas: 3,
        currentReplicas: 2,
        reason: 'High CPU',
        metrics: { cpu: 85 },
      })

      expect(result.success).toBe(true)
      expect(result.event).toBeDefined()
      expect(result.event?.action).toBe('scale_up')
      expect(result.event?.fromReplicas).toBe(2)
      expect(result.event?.toReplicas).toBe(3)
    })
  })

  describe('getScalingHistory', () => {
    it('should return empty array for non-existent policy', async () => {
      const history = await getScalingHistory(mockEnv, 'non-existent')
      expect(history).toEqual([])
    })
  })

  describe('checkHealth', () => {
    it('should return health check result', async () => {
      const result = await checkHealth(mockEnv, 'deployment', 'test')

      expect(result).toHaveProperty('healthy')
      expect(result).toHaveProperty('score')
      expect(result).toHaveProperty('details')
      expect(result.details).toHaveProperty('cpu')
      expect(result.details).toHaveProperty('memory')
      expect(result.details).toHaveProperty('requests')
      expect(result.details).toHaveProperty('errors')
      expect(typeof result.score).toBe('number')
      expect(result.score).toBeGreaterThanOrEqual(0)
      expect(result.score).toBeLessThanOrEqual(100)
    })
  })

  describe('getRecommendedReplicas', () => {
    it('should return recommended replica count', async () => {
      const created = await createScalingPolicy(mockEnv, {
        name: 'recommend-test',
        targetType: 'deployment',
        targetId: 'test',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 0,
      })

      const recommended = await getRecommendedReplicas(mockEnv, created.policy!)
      expect(recommended).toBeGreaterThanOrEqual(2)
      expect(recommended).toBeLessThanOrEqual(10)
    })
  })

  describe('runScalingCheck', () => {
    it('should run scaling check for all policies', async () => {
      await createScalingPolicy(mockEnv, {
        name: 'check-test-1',
        targetType: 'deployment',
        targetId: 'test1',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 0,
      })

      await createScalingPolicy(mockEnv, {
        name: 'check-test-2',
        targetType: 'deployment',
        targetId: 'test2',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'memory', targetValue: 80 }],
        enabled: false,
        cooldownSeconds: 0,
      })

      const result = await runScalingCheck(mockEnv)

      expect(result.checked).toBeGreaterThanOrEqual(1)
      expect(typeof result.scaled).toBe('number')
      expect(Array.isArray(result.errors)).toBe(true)
    })
  })

  describe('Types', () => {
    it('should have correct ScalingPolicy structure', () => {
      const policy: ScalingPolicy = {
        id: 'sp_123',
        name: 'test-policy',
        targetType: 'deployment',
        targetId: 'test-deployment',
        namespace: 'default',
        minReplicas: 2,
        maxReplicas: 10,
        metrics: [{ type: 'cpu', targetValue: 70 }],
        enabled: true,
        cooldownSeconds: 300,
      }

      expect(policy.id).toBe('sp_123')
      expect(policy.targetType).toBe('deployment')
      expect(policy.minReplicas).toBe(2)
      expect(policy.maxReplicas).toBe(10)
    })

    it('should have correct ScalingMetric structure', () => {
      const metric: ScalingMetric = {
        type: 'cpu',
        targetValue: 70,
        currentValue: 85,
        threshold: 90,
      }

      expect(metric.type).toBe('cpu')
      expect(metric.targetValue).toBe(70)
    })
  })
})