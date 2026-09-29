/**
 * Service Discovery and Registry
 *
 * Provides service registration, discovery, and health monitoring
 */

import type { Env } from '../types'

// Service instance
export interface ServiceInstance {
  id: string
  name: string
  namespace: string
  host: string
  port: number
  protocol: 'http' | 'https' | 'grpc' | 'tcp'
  metadata: Record<string, string>
  health: 'healthy' | 'unhealthy' | 'starting' | 'draining'
  weight: number
  registeredAt: string
  lastHeartbeat: string
  ttl: number // seconds
}

// Service definition
export interface ServiceDefinition {
  name: string
  namespace: string
  description?: string
  owner?: string
  tags: string[]
  endpoints: ServiceEndpoint[]
  dependencies: string[]
  healthCheckPath?: string
  healthCheckInterval?: number
}

// Service endpoint
export interface ServiceEndpoint {
  name: string
  path: string
  method: string
  description?: string
}

// Service dependency
export interface ServiceDependency {
  serviceName: string
  required: boolean
  healthImpact: 'critical' | 'degraded' | 'none'
}

// Service mesh topology
export interface MeshTopology {
  services: ServiceTopologyNode[]
  connections: ServiceConnection[]
  clusters: ServiceCluster[]
}

export interface ServiceTopologyNode {
  id: string
  name: string
  namespace: string
  type: 'service' | 'gateway' | 'database' | 'cache' | 'queue'
  health: string
  requestRate: number
  errorRate: number
  latency: number
}

export interface ServiceConnection {
  source: string
  target: string
  protocol: string
  requestRate: number
  errorRate: number
}

export interface ServiceCluster {
  name: string
  services: string[]
  region?: string
  zone?: string
}

/**
 * Register a service instance
 */
export async function registerService(
  env: Env,
  instance: Omit<ServiceInstance, 'id' | 'registeredAt' | 'lastHeartbeat'>
): Promise<{ success: boolean; id?: string }> {
  try {
    const id = generateServiceId(instance.name, instance.namespace)
    const now = new Date().toISOString()

    const fullInstance: ServiceInstance = {
      ...instance,
      id,
      registeredAt: now,
      lastHeartbeat: now,
    }

    // Store instance
    await env.KV.put(
      `discovery:instance:${id}`,
      JSON.stringify(fullInstance),
      { expirationTtl: instance.ttl }
    )

    // Update service registry
    await updateServiceRegistry(env, instance.name, instance.namespace, id)

    return { success: true, id }
  } catch (err) {
    console.error('Failed to register service:', err)
    return { success: false }
  }
}

/**
 * Deregister a service instance
 */
export async function deregisterService(
  env: Env,
  instanceId: string
): Promise<{ success: boolean }> {
  try {
    // Get instance info first
    const instance = await env.KV.get(`discovery:instance:${instanceId}`, 'json') as ServiceInstance | null
    if (!instance) {
      return { success: false }
    }

    // Remove instance
    await env.KV.delete(`discovery:instance:${instanceId}`)

    // Update service registry
    await removeFromServiceRegistry(env, instance.name, instance.namespace, instanceId)

    return { success: true }
  } catch (err) {
    console.error('Failed to deregister service:', err)
    return { success: false }
  }
}

/**
 * Send heartbeat for a service instance
 */
export async function sendHeartbeat(
  env: Env,
  instanceId: string,
  health?: 'healthy' | 'unhealthy' | 'starting' | 'draining'
): Promise<{ success: boolean }> {
  try {
    const instance = await env.KV.get(`discovery:instance:${instanceId}`, 'json') as ServiceInstance | null
    if (!instance) {
      return { success: false }
    }

    const updated: ServiceInstance = {
      ...instance,
      lastHeartbeat: new Date().toISOString(),
      health: health || instance.health,
    }

    await env.KV.put(
      `discovery:instance:${instanceId}`,
      JSON.stringify(updated),
      { expirationTtl: instance.ttl }
    )

    return { success: true }
  } catch (err) {
    console.error('Failed to send heartbeat:', err)
    return { success: false }
  }
}

/**
 * Discover service instances
 */
export async function discoverService(
  env: Env,
  name: string,
  namespace: string = 'default',
  options?: {
    healthyOnly?: boolean
    protocol?: string
    metadata?: Record<string, string>
  }
): Promise<ServiceInstance[]> {
  try {
    const registryKey = `discovery:registry:${namespace}:${name}`
    const instanceIds = await env.KV.get(registryKey, 'json') as string[] | null

    if (!instanceIds || instanceIds.length === 0) {
      return []
    }

    const instances: ServiceInstance[] = []
    for (const id of instanceIds) {
      const instance = await env.KV.get(`discovery:instance:${id}`, 'json') as ServiceInstance | null
      if (instance) {
        // Filter by health
        if (options?.healthyOnly && instance.health !== 'healthy') {
          continue
        }
        // Filter by protocol
        if (options?.protocol && instance.protocol !== options.protocol) {
          continue
        }
        // Filter by metadata
        if (options?.metadata) {
          const matches = Object.entries(options.metadata).every(
            ([k, v]) => instance.metadata[k] === v
          )
          if (!matches) continue
        }
        instances.push(instance)
      }
    }

    return instances
  } catch (err) {
    console.error('Failed to discover service:', err)
    return []
  }
}

/**
 * Get a single healthy instance (load balanced)
 */
export async function getHealthyInstance(
  env: Env,
  name: string,
  namespace: string = 'default',
  algorithm: 'round-robin' | 'random' | 'weighted' | 'least-connections' = 'round-robin'
): Promise<ServiceInstance | null> {
  const instances = await discoverService(env, name, namespace, { healthyOnly: true })

  if (instances.length === 0) {
    return null
  }

  switch (algorithm) {
    case 'random':
      return instances[Math.floor(Math.random() * instances.length)]

    case 'weighted': {
      const totalWeight = instances.reduce((sum, i) => sum + i.weight, 0)
      let random = Math.random() * totalWeight
      for (const instance of instances) {
        random -= instance.weight
        if (random <= 0) return instance
      }
      return instances[0]
    }

    case 'least-connections': {
      // Simplified - in production would track actual connections
      return instances.reduce((min, i) => i.weight < min.weight ? i : min, instances[0])
    }

    case 'round-robin':
    default: {
      const counterKey = `discovery:counter:${namespace}:${name}`
      const counter = await env.KV.get(counterKey, 'json') as number | null
      const index = ((counter || 0) + 1) % instances.length
      await env.KV.put(counterKey, JSON.stringify(index))
      return instances[index]
    }
  }
}

/**
 * Create service definition
 */
export async function createServiceDefinition(
  env: Env,
  definition: ServiceDefinition
): Promise<{ success: boolean }> {
  try {
    await env.KV.put(
      `discovery:definition:${definition.namespace}:${definition.name}`,
      JSON.stringify(definition)
    )
    return { success: true }
  } catch (err) {
    console.error('Failed to create service definition:', err)
    return { success: false }
  }
}

/**
 * Get service definition
 */
export async function getServiceDefinition(
  env: Env,
  name: string,
  namespace: string = 'default'
): Promise<ServiceDefinition | null> {
  try {
    const definition = await env.KV.get(
      `discovery:definition:${namespace}:${name}`,
      'json'
    ) as ServiceDefinition | null
    return definition
  } catch {
    return null
  }
}

/**
 * List all service definitions
 */
export async function listServiceDefinitions(
  env: Env,
  namespace?: string
): Promise<ServiceDefinition[]> {
  try {
    const prefix = namespace
      ? `discovery:definition:${namespace}:`
      : 'discovery:definition:'

    const list = await env.KV.list({ prefix })
    const definitions: ServiceDefinition[] = []

    for (const key of list.keys) {
      const def = await env.KV.get(key.name, 'json') as ServiceDefinition | null
      if (def) definitions.push(def)
    }

    return definitions
  } catch {
    return []
  }
}

/**
 * Get mesh topology
 */
export async function getMeshTopology(
  env: Env,
  namespace: string = 'default'
): Promise<MeshTopology> {
  try {
    // Get all services
    const definitions = await listServiceDefinitions(env, namespace)

    // Build topology nodes
    const services: ServiceTopologyNode[] = []
    const connections: ServiceConnection[] = []

    for (const def of definitions) {
      // Get instances for this service
      const instances = await discoverService(env, def.name, namespace)

      // Calculate aggregate metrics
      const healthyInstances = instances.filter(i => i.health === 'healthy').length

      services.push({
        id: `${def.namespace}:${def.name}`,
        name: def.name,
        namespace: def.namespace,
        type: 'service',
        health: healthyInstances > 0 ? 'healthy' : 'unhealthy',
        requestRate: 0, // Would come from metrics
        errorRate: 0,
        latency: 0,
      })

      // Add connections based on dependencies
      for (const dep of def.dependencies) {
        connections.push({
          source: `${def.namespace}:${def.name}`,
          target: `${def.namespace}:${dep}`,
          protocol: 'http',
          requestRate: 0,
          errorRate: 0,
        })
      }
    }

    // Add clusters (simplified)
    const clusters: ServiceCluster[] = [
      {
        name: 'default-cluster',
        services: services.map(s => s.id),
        region: 'default',
      },
    ]

    return { services, connections, clusters }
  } catch (err) {
    console.error('Failed to get mesh topology:', err)
    return { services: [], connections: [], clusters: [] }
  }
}

/**
 * Check service health
 */
export async function checkServiceHealth(
  env: Env,
  name: string,
  namespace: string = 'default'
): Promise<{
  status: 'healthy' | 'degraded' | 'unhealthy'
  instances: { healthy: number; unhealthy: number; total: number }
  lastCheck: string
}> {
  const instances = await discoverService(env, name, namespace)

  const healthy = instances.filter(i => i.health === 'healthy').length
  const unhealthy = instances.length - healthy

  let status: 'healthy' | 'degraded' | 'unhealthy'
  if (instances.length === 0) {
    status = 'unhealthy'
  } else if (healthy === instances.length) {
    status = 'healthy'
  } else if (healthy > 0) {
    status = 'degraded'
  } else {
    status = 'unhealthy'
  }

  return {
    status,
    instances: { healthy, unhealthy, total: instances.length },
    lastCheck: new Date().toISOString(),
  }
}

/**
 * Get service statistics
 */
export async function getServiceStats(
  env: Env
): Promise<{
  totalServices: number
  totalInstances: number
  healthyInstances: number
  namespaces: string[]
}> {
  try {
    const definitions = await listServiceDefinitions(env)
    let totalInstances = 0
    let healthyInstances = 0
    const namespaces = new Set<string>()

    for (const def of definitions) {
      namespaces.add(def.namespace)
      const instances = await discoverService(env, def.name, def.namespace)
      totalInstances += instances.length
      healthyInstances += instances.filter(i => i.health === 'healthy').length
    }

    return {
      totalServices: definitions.length,
      totalInstances,
      healthyInstances,
      namespaces: Array.from(namespaces),
    }
  } catch {
    return {
      totalServices: 0,
      totalInstances: 0,
      healthyInstances: 0,
      namespaces: [],
    }
  }
}

// Helper functions
function generateServiceId(name: string, namespace: string): string {
  return `${namespace}-${name}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
}

async function updateServiceRegistry(
  env: Env,
  name: string,
  namespace: string,
  instanceId: string
): Promise<void> {
  const key = `discovery:registry:${namespace}:${name}`
  const existing = await env.KV.get(key, 'json') as string[] | null
  const ids = existing || []

  if (!ids.includes(instanceId)) {
    ids.push(instanceId)
    await env.KV.put(key, JSON.stringify(ids))
  }
}

async function removeFromServiceRegistry(
  env: Env,
  name: string,
  namespace: string,
  instanceId: string
): Promise<void> {
  const key = `discovery:registry:${namespace}:${name}`
  const existing = await env.KV.get(key, 'json') as string[] | null

  if (existing) {
    const ids = existing.filter(id => id !== instanceId)
    if (ids.length > 0) {
      await env.KV.put(key, JSON.stringify(ids))
    } else {
      await env.KV.delete(key)
    }
  }
}