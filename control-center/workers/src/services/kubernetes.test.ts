/**
 * Kubernetes Service Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
import {
  listNamespaces,
  listPods,
  listDeployments,
  listClusterNodes,
  getPodLogs,
  listEvents,
  listServices,
  scaleDeployment,
  restartDeployment,
  getClusterMetrics,
  type K8sNamespace,
  type K8sPod,
  type K8sDeployment,
  type K8sNode,
  type K8sService,
  type K8sEvent,
} from './kubernetes'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Kubernetes Service', () => {
  let mockEnv: any

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
      R2: {} as any,
    }
  })

  describe('listNamespaces', () => {
    it('should list namespaces', async () => {
      const namespaces = await listNamespaces(mockEnv)

      expect(Array.isArray(namespaces)).toBe(true)
      expect(namespaces.length).toBeGreaterThan(0)
      expect(namespaces[0]).toHaveProperty('name')
      expect(namespaces[0]).toHaveProperty('status')
    })

    it('should return cached namespaces', async () => {
      // First call
      const ns1 = await listNamespaces(mockEnv)

      // Second call should use cache
      const ns2 = await listNamespaces(mockEnv)

      expect(ns1).toEqual(ns2)
    })
  })

  describe('listPods', () => {
    it('should list pods in namespace', async () => {
      const pods = await listPods(mockEnv, 'default')

      expect(Array.isArray(pods)).toBe(true)
      if (pods.length > 0) {
        expect(pods[0]).toHaveProperty('name')
        expect(pods[0]).toHaveProperty('namespace')
        expect(pods[0]).toHaveProperty('status')
        expect(pods[0]).toHaveProperty('containers')
      }
    })

    it('should use default namespace', async () => {
      const pods = await listPods(mockEnv)
      expect(Array.isArray(pods)).toBe(true)
    })
  })

  describe('listDeployments', () => {
    it('should list deployments', async () => {
      const deployments = await listDeployments(mockEnv, 'default')

      expect(Array.isArray(deployments)).toBe(true)
      if (deployments.length > 0) {
        expect(deployments[0]).toHaveProperty('name')
        expect(deployments[0]).toHaveProperty('replicas')
        expect(deployments[0]).toHaveProperty('readyReplicas')
      }
    })
  })

  describe('listClusterNodes', () => {
    it('should list cluster nodes', async () => {
      const nodes = await listClusterNodes(mockEnv)

      expect(Array.isArray(nodes)).toBe(true)
      if (nodes.length > 0) {
        expect(nodes[0]).toHaveProperty('name')
        expect(nodes[0]).toHaveProperty('status')
        expect(nodes[0]).toHaveProperty('roles')
        expect(nodes[0]).toHaveProperty('kubeletVersion')
        expect(nodes[0]).toHaveProperty('capacity')
      }
    })
  })

  describe('getPodLogs', () => {
    it('should get pod logs', async () => {
      const logs = await getPodLogs(mockEnv, 'default', 'test-pod')

      expect(Array.isArray(logs)).toBe(true)
    })

    it('should accept tailLines option', async () => {
      const logs = await getPodLogs(mockEnv, 'default', 'test-pod', { tailLines: 50 })

      expect(Array.isArray(logs)).toBe(true)
    })

    it('should accept container option', async () => {
      const logs = await getPodLogs(mockEnv, 'default', 'test-pod', { container: 'main' })

      expect(Array.isArray(logs)).toBe(true)
    })
  })

  describe('listEvents', () => {
    it('should list events', async () => {
      const events = await listEvents(mockEnv, 'default')

      expect(Array.isArray(events)).toBe(true)
      if (events.length > 0) {
        expect(events[0]).toHaveProperty('name')
        expect(events[0]).toHaveProperty('type')
        expect(events[0]).toHaveProperty('reason')
        expect(events[0]).toHaveProperty('message')
      }
    })
  })

  describe('listServices', () => {
    it('should list services', async () => {
      const services = await listServices(mockEnv, 'default')

      expect(Array.isArray(services)).toBe(true)
      if (services.length > 0) {
        expect(services[0]).toHaveProperty('name')
        expect(services[0]).toHaveProperty('type')
        expect(services[0]).toHaveProperty('ports')
      }
    })
  })

  describe('scaleDeployment', () => {
    it('should scale deployment', async () => {
      const result = await scaleDeployment(mockEnv, 'default', 'test-deploy', 3)

      expect(typeof result).toBe('boolean')
    })
  })

  describe('restartDeployment', () => {
    it('should restart deployment', async () => {
      const result = await restartDeployment(mockEnv, 'default', 'test-deploy')

      expect(typeof result).toBe('boolean')
    })
  })

  describe('getClusterMetrics', () => {
    it('should return cluster metrics', async () => {
      const metrics = await getClusterMetrics(mockEnv)

      expect(metrics).toHaveProperty('nodes')
      expect(metrics).toHaveProperty('pods')
      expect(metrics).toHaveProperty('deployments')
      expect(metrics).toHaveProperty('services')
      expect(metrics).toHaveProperty('namespaces')
      expect(typeof metrics.nodes).toBe('number')
    })
  })

  describe('K8s Types', () => {
    it('should have correct K8sNamespace structure', () => {
      const ns: K8sNamespace = {
        name: 'test-namespace',
        status: 'Active',
        createdAt: new Date().toISOString(),
        labels: { app: 'test' },
      }

      expect(ns.name).toBe('test-namespace')
      expect(ns.status).toBe('Active')
    })

    it('should have correct K8sPod structure', () => {
      const pod: K8sPod = {
        name: 'test-pod',
        namespace: 'default',
        status: 'Running',
        podIP: '10.0.0.1',
        nodeName: 'node-1',
        createdAt: new Date().toISOString(),
        containers: [
          { name: 'main', image: 'nginx:latest', ready: true, restartCount: 0 },
        ],
        labels: {},
      }

      expect(pod.name).toBe('test-pod')
      expect(pod.containers.length).toBe(1)
    })

    it('should have correct K8sDeployment structure', () => {
      const deploy: K8sDeployment = {
        name: 'test-deploy',
        namespace: 'default',
        replicas: 3,
        readyReplicas: 3,
        availableReplicas: 3,
        createdAt: new Date().toISOString(),
        containers: [{ name: 'app', image: 'nginx' }],
      }

      expect(deploy.replicas).toBe(3)
      expect(deploy.readyReplicas).toBe(3)
    })

    it('should have correct K8sNode structure', () => {
      const node: K8sNode = {
        name: 'node-1',
        status: 'Ready',
        roles: ['master'],
        kubeletVersion: 'v1.28.0',
        os: 'linux',
        architecture: 'amd64',
        capacity: { cpu: '4', memory: '16Gi', pods: '110' },
        conditions: [{ type: 'Ready', status: 'True' }],
      }

      expect(node.roles).toContain('master')
      expect(node.capacity.cpu).toBe('4')
    })

    it('should have correct K8sService structure', () => {
      const svc: K8sService = {
        name: 'test-svc',
        namespace: 'default',
        type: 'ClusterIP',
        clusterIP: '10.96.0.1',
        externalIPs: [],
        ports: [{ port: 80, targetPort: 8080, protocol: 'TCP' }],
        selector: { app: 'test' },
      }

      expect(svc.type).toBe('ClusterIP')
      expect(svc.ports.length).toBe(1)
    })

    it('should have correct K8sEvent structure', () => {
      const event: K8sEvent = {
        name: 'test-event',
        namespace: 'default',
        type: 'Normal',
        reason: 'Started',
        message: 'Container started',
        involvedObject: { kind: 'Pod', name: 'test-pod', namespace: 'default' },
        count: 1,
        firstTimestamp: new Date().toISOString(),
        lastTimestamp: new Date().toISOString(),
      }

      expect(event.type).toBe('Normal')
      expect(event.reason).toBe('Started')
    })
  })
})