/**
 * Prometheus Metrics Handler
 *
 * Exposes metrics in Prometheus format for monitoring
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { writeAnalyticsEvent } from '../services/analytics'
import { probeRuntimeServices } from '../services/monitoring'

interface MetricValue {
  name: string
  help: string
  type: 'counter' | 'gauge' | 'histogram'
  value: number
  labels?: Record<string, string>
}

/**
 * Format metric for Prometheus exposition format
 */
function formatMetric(metric: MetricValue): string {
  const lines: string[] = []

  // Help line
  lines.push(`# HELP ${metric.name} ${metric.help}`)

  // Type line
  lines.push(`# TYPE ${metric.name} ${metric.type}`)

  // Value line with labels
  if (metric.labels && Object.keys(metric.labels).length > 0) {
    const labelStr = Object.entries(metric.labels)
      .map(([k, v]) => `${k}="${v}"`)
      .join(',')
    lines.push(`${metric.name}{${labelStr}} ${metric.value}`)
  } else {
    lines.push(`${metric.name} ${metric.value}`)
  }

  return lines.join('\n')
}

/**
 * Collect system metrics
 */
async function collectMetrics(env: Env): Promise<MetricValue[]> {
  const metrics: MetricValue[] = []

  const [userCount, nodeCount, taskCount, playbookCount, nodeStatus, taskStatus] = await Promise.all([
    env.DB.prepare('SELECT COUNT(*) as count FROM users').first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM nodes').first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM tasks').first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM playbooks').first<{ count: number }>(),
    env.DB.prepare('SELECT status, COUNT(*) as count FROM nodes GROUP BY status').all<{ status: string; count: number }>(),
    env.DB.prepare('SELECT status, COUNT(*) as count FROM tasks GROUP BY status').all<{ status: string; count: number }>(),
  ])

  // Basic counts
  metrics.push({
    name: 'anixops_users_total',
    help: 'Total number of users',
    type: 'gauge',
    value: userCount?.count || 0,
  })

  metrics.push({
    name: 'anixops_nodes_total',
    help: 'Total number of nodes',
    type: 'gauge',
    value: nodeCount?.count || 0,
  })

  metrics.push({
    name: 'anixops_tasks_total',
    help: 'Total number of tasks',
    type: 'gauge',
    value: taskCount?.count || 0,
  })

  metrics.push({
    name: 'anixops_playbooks_total',
    help: 'Total number of playbooks',
    type: 'gauge',
    value: playbookCount?.count || 0,
  })

  // Node status metrics
  for (const row of nodeStatus.results) {
    metrics.push({
      name: 'anixops_nodes_by_status',
      help: 'Number of nodes by status',
      type: 'gauge',
      value: row.count,
      labels: { status: row.status },
    })
  }

  // Task status metrics
  for (const row of taskStatus.results) {
    metrics.push({
      name: 'anixops_tasks_by_status',
      help: 'Number of tasks by status',
      type: 'gauge',
      value: row.count,
      labels: { status: row.status },
    })
  }

  // System info
  metrics.push({
    name: 'anixops_info',
    help: 'AnixOps system information',
    type: 'gauge',
    value: 1,
    labels: {
      version: env.APP_VERSION || '1.0.0',
      build_sha: env.BUILD_SHA || 'unknown',
      phase: '4',
    },
  })

  // Uptime (placeholder)
  metrics.push({
    name: 'anixops_uptime_seconds',
    help: 'System uptime in seconds',
    type: 'gauge',
    value: 0,
  })

  return metrics
}

/**
 * Prometheus metrics endpoint handler
 */
export async function prometheusMetricsHandler(c: Context<{ Bindings: Env }>) {
  try {
    const metrics = await collectMetrics(c.env)
    const output = metrics.map(formatMetric).join('\n')

    writeAnalyticsEvent(c.env, {
      indexes: ['metrics.scrape', c.env.APP_VERSION || '1.0.0', c.env.ENVIRONMENT],
      doubles: [metrics.length],
    })

    return c.text(output + '\n', 200, {
      'Content-Type': 'text/plain; version=0.0.4; charset=utf-8',
      'Cache-Control': 'no-cache',
    })
  } catch (err) {
    console.error('Metrics collection error:', err)
    return c.text('# Error collecting metrics\n', 500, {
      'Content-Type': 'text/plain; version=0.0.4',
    })
  }
}

/**
 * Health check with detailed status
 */
export async function detailedHealthHandler(c: Context<{ Bindings: Env }>) {
  const checks = await probeRuntimeServices(c.env)
  const allHealthy = [checks.database, checks.kv, checks.r2].every(check => check.status === 'healthy')

  return c.json({
    status: allHealthy ? 'healthy' : 'degraded',
    timestamp: new Date().toISOString(),
    version: c.env.APP_VERSION || '1.0.0',
    checks: {
      database: {
        status: checks.database.status,
        latency: checks.database.latency,
        ...(checks.database.message ? { error: checks.database.message } : {}),
      },
      kv: {
        status: checks.kv.status,
        latency: checks.kv.latency,
        ...(checks.kv.message ? { error: checks.kv.message } : {}),
      },
      r2: {
        status: checks.r2.status,
        latency: checks.r2.latency,
        ...(checks.r2.message ? { error: checks.r2.message } : {}),
      },
    },
  }, allHealthy ? 200 : 503)
}

/**
 * Readiness probe for Kubernetes
 */
export async function readinessHandler(c: Context<{ Bindings: Env }>) {
  try {
    // Quick check that essential services are available
    await c.env.DB.prepare('SELECT 1').first()
    return c.text('OK', 200)
  } catch (err) {
    return c.text('Not Ready', 503)
  }
}

/**
 * Liveness probe for Kubernetes
 */
export async function livenessHandler(c: Context<{ Bindings: Env }>) {
  return c.text('OK', 200)
}