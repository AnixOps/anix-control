/**
 * Kubernetes API Handlers
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'
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
} from '../services/kubernetes'

/**
 * List namespaces
 */
export async function listNamespacesHandler(c: Context<{ Bindings: Env }>) {
  const namespaces = await listNamespaces(c.env)
  return c.json({ success: true, data: namespaces })
}

/**
 * List pods
 */
export async function listPodsHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const pods = await listPods(c.env, namespace)
  return c.json({ success: true, data: pods, namespace })
}

/**
 * List deployments
 */
export async function listDeploymentsHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const deployments = await listDeployments(c.env, namespace)
  return c.json({ success: true, data: deployments, namespace })
}

/**
 * List cluster nodes
 */
export async function listClusterNodesHandler(c: Context<{ Bindings: Env }>) {
  const nodes = await listClusterNodes(c.env)
  return c.json({ success: true, data: nodes })
}

/**
 * List services
 */
export async function listServicesHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const services = await listServices(c.env, namespace)
  return c.json({ success: true, data: services, namespace })
}

/**
 * List events
 */
export async function listEventsHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.query('namespace') || 'default'
  const events = await listEvents(c.env, namespace)
  return c.json({ success: true, data: events, namespace })
}

/**
 * Get pod logs
 */
export async function getPodLogsHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.param('namespace') as string || 'default'
  const podName = c.req.param('pod') as string

  if (!podName) {
    return c.json({ success: false, error: 'Pod name required' }, 400)
  }

  const tailLines = parseInt(c.req.query('tailLines') || '100', 10)
  const container = c.req.query('container')

  const logs = await getPodLogs(c.env, namespace, podName, { tailLines, container })
  return c.json({ success: true, data: logs, pod: podName, namespace })
}

/**
 * Scale deployment
 */
export async function scaleDeploymentHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const namespace = c.req.param('namespace') as string || 'default'
  const name = c.req.param('name') as string

  if (!name) {
    return c.json({ success: false, error: 'Deployment name required' }, 400)
  }

  const body = await c.req.json()
  const replicas = body.replicas

  if (typeof replicas !== 'number' || replicas < 0) {
    return c.json({ success: false, error: 'Valid replicas count required' }, 400)
  }

  const result = await scaleDeployment(c.env, namespace, name, replicas)

  await logAudit(c, user?.sub, 'scale_deployment', 'kubernetes', {
    namespace,
    deployment: name,
    replicas,
    success: result,
  })

  return c.json({
    success: result,
    message: result ? `Scaled ${name} to ${replicas} replicas` : 'Failed to scale deployment',
  })
}

/**
 * Restart deployment
 */
export async function restartDeploymentHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const namespace = c.req.param('namespace') as string || 'default'
  const name = c.req.param('name') as string

  if (!name) {
    return c.json({ success: false, error: 'Deployment name required' }, 400)
  }

  const result = await restartDeployment(c.env, namespace, name)

  await logAudit(c, user?.sub, 'restart_deployment', 'kubernetes', {
    namespace,
    deployment: name,
    success: result,
  })

  return c.json({
    success: result,
    message: result ? `Restarted ${name}` : 'Failed to restart deployment',
  })
}

/**
 * Get cluster overview
 */
export async function getClusterOverviewHandler(c: Context<{ Bindings: Env }>) {
  const metrics = await getClusterMetrics(c.env)

  const nodes = await listClusterNodes(c.env)
  const namespaces = await listNamespaces(c.env)

  // Node status summary
  const nodeStatus = {
    ready: nodes.filter(n => n.status === 'Ready').length,
    notReady: nodes.filter(n => n.status !== 'Ready').length,
  }

  return c.json({
    success: true,
    data: {
      metrics,
      nodes: nodeStatus,
      namespaces: namespaces.length,
      version: 'v1.28.0',
    },
  })
}

/**
 * Get namespace details
 */
export async function getNamespaceDetailsHandler(c: Context<{ Bindings: Env }>) {
  const namespace = c.req.param('namespace') as string

  if (!namespace) {
    return c.json({ success: false, error: 'Namespace required' }, 400)
  }

  const [pods, deployments, services, events] = await Promise.all([
    listPods(c.env, namespace),
    listDeployments(c.env, namespace),
    listServices(c.env, namespace),
    listEvents(c.env, namespace),
  ])

  return c.json({
    success: true,
    data: {
      namespace,
      pods,
      deployments,
      services,
      events: events.slice(0, 10),
      summary: {
        pods: pods.length,
        deployments: deployments.length,
        services: services.length,
        runningPods: pods.filter(p => p.status === 'Running').length,
      },
    },
  })
}