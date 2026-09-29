/**
 * Tests for Service Discovery
 */

import { describe, it, expect, beforeEach } from 'vitest'
import { createMockKV, createMockD1 } from './setup'
import {
  registerService,
  deregisterService,
  sendHeartbeat,
  discoverService,
  getHealthyInstance,
  createServiceDefinition,
  getServiceDefinition,
  listServiceDefinitions,
  getMeshTopology,
  checkServiceHealth,
  getServiceStats,
  type ServiceInstance,
  type ServiceDefinition,
} from '../src/services/discovery'

const createMockEnv = () => ({
  KV: createMockKV(),
  DB: createMockD1(),
  AI: {} as any,
  VECTORIZE: {} as any,
  ASSETS: {} as any,
})

describe('Service Discovery', () => {
  describe('registerService', () => {
    it('should register a service instance', async () => {
      const env = createMockEnv()
      const result = await registerService(env, {
        name: 'test-service',
        namespace: 'default',
        host: 'localhost',
        port: 8080,
        protocol: 'http',
        metadata: { version: '1.0.0' },
        health: 'healthy',
        weight: 1,
        ttl: 300,
      })

      expect(result.success).toBe(true)
      expect(result.id).toBeDefined()
    })

    it('should generate unique service IDs', async () => {
      const env = createMockEnv()

      const result1 = await registerService(env, {
        name: 'service-a',
        namespace: 'ns1',
        host: 'localhost',
        port: 8080,
        protocol: 'http',
        metadata: {},
        health: 'healthy',
        weight: 1,
        ttl: 300,
      })

      const result2 = await registerService(env, {
        name: 'service-a',
        namespace: 'ns1',
        host: 'localhost',
        port: 8081,
        protocol: 'http',
        metadata: {},
        health: 'healthy',
        weight: 1,
        ttl: 300,
      })

      expect(result1.id).not.toBe(result2.id)
    })
  })

  describe('discoverService', () => {
    it('should return empty array when no instances registered', async () => {
      const env = createMockEnv()
      const instances = await discoverService(env, 'nonexistent', 'default')
      expect(instances).toEqual([])
    })

    it('should filter by healthy only', async () => {
      const env = createMockEnv()

      await registerService(env, {
        name: 'test-service',
        namespace: 'default',
        host: 'localhost',
        port: 8080,
        protocol: 'http',
        metadata: {},
        health: 'healthy',
        weight: 1,
        ttl: 300,
      })

      await registerService(env, {
        name: 'test-service',
        namespace: 'default',
        host: 'localhost',
        port: 8081,
        protocol: 'http',
        metadata: {},
        health: 'unhealthy',
        weight: 1,
        ttl: 300,
      })

      const allInstances = await discoverService(env, 'test-service', 'default')
      const healthyInstances = await discoverService(env, 'test-service', 'default', { healthyOnly: true })

      expect(allInstances.length).toBeGreaterThanOrEqual(1)
      expect(healthyInstances.every(i => i.health === 'healthy')).toBe(true)
    })
  })

  describe('sendHeartbeat', () => {
    it('should return false for nonexistent instance', async () => {
      const env = createMockEnv()
      const result = await sendHeartbeat(env, 'nonexistent-id')
      expect(result.success).toBe(false)
    })
  })

  describe('deregisterService', () => {
    it('should return false for nonexistent instance', async () => {
      const env = createMockEnv()
      const result = await deregisterService(env, 'nonexistent-id')
      expect(result.success).toBe(false)
    })
  })

  describe('getHealthyInstance', () => {
    it('should return null when no instances exist', async () => {
      const env = createMockEnv()
      const instance = await getHealthyInstance(env, 'nonexistent', 'default')
      expect(instance).toBeNull()
    })
  })

  describe('Service Definitions', () => {
    it('should create and retrieve service definition', async () => {
      const env = createMockEnv()

      const definition: ServiceDefinition = {
        name: 'api-service',
        namespace: 'production',
        description: 'API Service',
        owner: 'team-a',
        tags: ['api', 'core'],
        endpoints: [
          { name: 'list', path: '/api/v1/items', method: 'GET' },
        ],
        dependencies: ['database', 'cache'],
      }

      await createServiceDefinition(env, definition)
      const retrieved = await getServiceDefinition(env, 'api-service', 'production')

      expect(retrieved).toBeDefined()
      expect(retrieved?.name).toBe('api-service')
      expect(retrieved?.tags).toContain('api')
    })

    it('should list service definitions', async () => {
      const env = createMockEnv()

      await createServiceDefinition(env, {
        name: 'service-1',
        namespace: 'ns1',
        tags: [],
        endpoints: [],
        dependencies: [],
      })

      await createServiceDefinition(env, {
        name: 'service-2',
        namespace: 'ns1',
        tags: [],
        endpoints: [],
        dependencies: [],
      })

      const definitions = await listServiceDefinitions(env, 'ns1')
      expect(definitions.length).toBeGreaterThanOrEqual(2)
    })
  })

  describe('getMeshTopology', () => {
    it('should return empty topology when no services', async () => {
      const env = createMockEnv()
      const topology = await getMeshTopology(env, 'empty-ns')

      expect(topology.services).toEqual([])
      expect(topology.connections).toEqual([])
      expect(topology.clusters).toBeDefined()
    })
  })

  describe('checkServiceHealth', () => {
    it('should return unhealthy when no instances', async () => {
      const env = createMockEnv()
      const health = await checkServiceHealth(env, 'nonexistent', 'default')

      expect(health.status).toBe('unhealthy')
      expect(health.instances.total).toBe(0)
    })
  })

  describe('getServiceStats', () => {
    it('should return stats', async () => {
      const env = createMockEnv()
      const stats = await getServiceStats(env)

      expect(stats).toHaveProperty('totalServices')
      expect(stats).toHaveProperty('totalInstances')
      expect(stats).toHaveProperty('healthyInstances')
      expect(stats).toHaveProperty('namespaces')
    })
  })
})