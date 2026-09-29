/**
 * Load Balancing Service Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
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
  type LoadBalancerConfig,
  type BackendTarget,
} from './loadbalancer'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Load Balancing Service', () => {
  let mockEnv: any

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
      R2: {} as any,
    }
  })

  describe('createLoadBalancer', () => {
    it('should create a load balancer', async () => {
      const result = await createLoadBalancer(mockEnv, {
        name: 'test-lb',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      expect(result.success).toBe(true)
      expect(result.lb).toBeDefined()
      expect(result.lb?.id).toMatch(/^lb_\d+_[a-z0-9]+$/)
      expect(result.lb?.name).toBe('test-lb')
    })

    it('should require name', async () => {
      const result = await createLoadBalancer(mockEnv, {
        name: '',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('Name')
    })

    it('should require at least one target', async () => {
      const result = await createLoadBalancer(mockEnv, {
        name: 'no-targets',
        algorithm: 'round-robin',
        targets: [],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('target')
    })

    it('should validate algorithm', async () => {
      const result = await createLoadBalancer(mockEnv, {
        name: 'invalid-algo',
        algorithm: 'invalid' as any,
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('algorithm')
    })

    it('should validate weights sum to 100 for weighted algorithm', async () => {
      const result = await createLoadBalancer(mockEnv, {
        name: 'invalid-weights',
        algorithm: 'weighted',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 30, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 30, healthy: true, connections: 0 },
        ],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      expect(result.success).toBe(false)
      expect(result.error).toContain('100')
    })

    it('should support all algorithms', async () => {
      const algorithms = ['round-robin', 'weighted', 'least-connections', 'ip-hash', 'random', 'response-time']

      for (const algo of algorithms) {
        const result = await createLoadBalancer(mockEnv, {
          name: `lb-${algo}`,
          algorithm: algo as any,
          targets: [
            { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
            { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
          ],
          healthCheck: {
            enabled: true,
            interval: 30,
            timeout: 5,
            unhealthyThreshold: 3,
            healthyThreshold: 2,
            path: '/health',
            expectedStatus: [200],
          },
          enabled: true,
        })

        expect(result.success).toBe(true)
        expect(result.lb?.algorithm).toBe(algo)
      }
    })
  })

  describe('getLoadBalancer', () => {
    it('should return null for non-existent lb', async () => {
      const result = await getLoadBalancer(mockEnv, 'non-existent')
      expect(result).toBeNull()
    })

    it('should retrieve created load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'get-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
      })

      const retrieved = await getLoadBalancer(mockEnv, created.lb!.id)
      expect(retrieved).toBeDefined()
      expect(retrieved?.name).toBe('get-test')
    })
  })

  describe('listLoadBalancers', () => {
    it('should list all load balancers', async () => {
      await createLoadBalancer(mockEnv, {
        name: 'lb-1',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      await createLoadBalancer(mockEnv, {
        name: 'lb-2',
        algorithm: 'weighted',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const lbs = await listLoadBalancers(mockEnv)
      expect(lbs.length).toBeGreaterThanOrEqual(2)
    })
  })

  describe('updateLoadBalancer', () => {
    it('should update load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'update-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const updated = await updateLoadBalancer(mockEnv, created.lb!.id, {
        algorithm: 'least-connections',
        enabled: false,
      })

      expect(updated.success).toBe(true)
      expect(updated.lb?.algorithm).toBe('least-connections')
      expect(updated.lb?.enabled).toBe(false)
    })

    it('should return error for non-existent lb', async () => {
      const result = await updateLoadBalancer(mockEnv, 'non-existent', { enabled: false })
      expect(result.success).toBe(false)
    })
  })

  describe('deleteLoadBalancer', () => {
    it('should delete load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'delete-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const deleted = await deleteLoadBalancer(mockEnv, created.lb!.id)
      expect(deleted.success).toBe(true)

      const retrieved = await getLoadBalancer(mockEnv, created.lb!.id)
      expect(retrieved).toBeNull()
    })
  })

  describe('selectTarget', () => {
    it('should return null for non-existent lb', async () => {
      const result = await selectTarget(mockEnv, 'non-existent')
      expect(result).toBeNull()
    })

    it('should select target using round-robin', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'select-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const selection = await selectTarget(mockEnv, created.lb!.id, '192.168.1.1')
      expect(selection).toBeDefined()
      expect(selection?.target).toBeDefined()
      expect(selection?.algorithm).toBe('round-robin')
    })

    it('should only select healthy targets', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'healthy-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: false, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const selection = await selectTarget(mockEnv, created.lb!.id)
      expect(selection?.target.id).toBe('t2')
    })
  })

  describe('checkTargetHealth', () => {
    it('should check target health', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'health-check-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await checkTargetHealth(mockEnv, created.lb!.id, 't1')
      expect(result).toHaveProperty('healthy')
      expect(result).toHaveProperty('responseTime')
    })

    it('should return error for non-existent lb', async () => {
      const result = await checkTargetHealth(mockEnv, 'non-existent', 't1')
      expect(result.healthy).toBe(false)
      expect(result.error).toBeDefined()
    })
  })

  describe('runHealthChecks', () => {
    it('should run health checks for all targets', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'run-health-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await runHealthChecks(mockEnv, created.lb!.id)
      expect(result.checked).toBe(2)
      expect(result.healthy + result.unhealthy).toBe(2)
    })
  })

  describe('getLoadBalancerStats', () => {
    it('should return stats for load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'stats-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const stats = await getLoadBalancerStats(mockEnv, created.lb!.id)
      expect(stats).toHaveProperty('totalRequests')
      expect(stats).toHaveProperty('totalConnections')
      expect(stats).toHaveProperty('activeConnections')
      expect(stats).toHaveProperty('healthyTargets')
      expect(stats).toHaveProperty('unhealthyTargets')
    })
  })

  describe('recordRequestCompletion', () => {
    it('should record request completion', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'completion-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      await recordRequestCompletion(mockEnv, created.lb!.id, 't1', 50)

      const stats = await getLoadBalancerStats(mockEnv, created.lb!.id)
      expect(stats.totalRequests).toBe(1)
      expect(stats.requestsPerTarget['t1']).toBe(1)
    })
  })

  describe('addTarget', () => {
    it('should add target to load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'add-target-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await addTarget(mockEnv, created.lb!.id, {
        name: 'target-2',
        address: '10.0.0.2',
        port: 8080,
        weight: 50,
        responseTime: 20,
      })

      expect(result.success).toBe(true)
      expect(result.target?.name).toBe('target-2')
    })
  })

  describe('removeTarget', () => {
    it('should remove target from load balancer', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'remove-target-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await removeTarget(mockEnv, created.lb!.id, 't1')
      expect(result.success).toBe(true)

      const lb = await getLoadBalancer(mockEnv, created.lb!.id)
      expect(lb?.targets.length).toBe(1)
      expect(lb?.targets[0].id).toBe('t2')
    })
  })

  describe('updateTargetWeight', () => {
    it('should update target weight', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'weight-test',
        algorithm: 'weighted',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 50, healthy: true, connections: 0 },
          { id: 't2', name: 'target-2', address: '10.0.0.2', port: 8080, weight: 50, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await updateTargetWeight(mockEnv, created.lb!.id, 't1', 70)
      expect(result.success).toBe(true)

      const lb = await getLoadBalancer(mockEnv, created.lb!.id)
      expect(lb?.targets[0].weight).toBe(70)
    })

    it('should reject invalid weight', async () => {
      const created = await createLoadBalancer(mockEnv, {
        name: 'invalid-weight-test',
        algorithm: 'round-robin',
        targets: [
          { id: 't1', name: 'target-1', address: '10.0.0.1', port: 8080, weight: 100, healthy: true, connections: 0 },
        ],
        healthCheck: { enabled: true, interval: 30, timeout: 5, unhealthyThreshold: 3, healthyThreshold: 2, path: '/health', expectedStatus: [200] },
        enabled: true,
      })

      const result = await updateTargetWeight(mockEnv, created.lb!.id, 't1', 150)
      expect(result.success).toBe(false)
    })
  })

  describe('Types', () => {
    it('should have correct BackendTarget structure', () => {
      const target: BackendTarget = {
        id: 't1',
        name: 'test-target',
        address: '10.0.0.1',
        port: 8080,
        weight: 100,
        healthy: true,
        connections: 5,
        responseTime: 25,
        metadata: { zone: 'us-east-1' },
      }

      expect(target.id).toBe('t1')
      expect(target.port).toBe(8080)
      expect(target.healthy).toBe(true)
    })

    it('should have correct LoadBalancerConfig structure', () => {
      const config: LoadBalancerConfig = {
        id: 'lb_123',
        name: 'test-lb',
        algorithm: 'round-robin',
        targets: [],
        healthCheck: {
          enabled: true,
          interval: 30,
          timeout: 5,
          unhealthyThreshold: 3,
          healthyThreshold: 2,
          path: '/health',
          expectedStatus: [200],
        },
        enabled: true,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }

      expect(config.algorithm).toBe('round-robin')
      expect(config.healthCheck.interval).toBe(30)
    })
  })
})