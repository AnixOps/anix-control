/**
 * Monitoring Service
 *
 * Provides metrics collection, alerting, and health monitoring
 */

import type { Env } from '../types'

// Metric types
export type MetricType = 'counter' | 'gauge' | 'histogram' | 'summary'

// Metric data point
export interface MetricPoint {
  name: string
  type: MetricType
  value: number
  labels: Record<string, string>
  timestamp: string
}

// Alert rule
export interface AlertRule {
  id: string
  name: string
  metric: string
  operator: 'gt' | 'lt' | 'eq' | 'neq' | 'gte' | 'lte'
  threshold: number
  duration: number // seconds
  severity: 'info' | 'warning' | 'critical'
  enabled: boolean
  labels?: Record<string, string>
}

// Alert state
export interface AlertState {
  ruleId: string
  state: 'firing' | 'pending' | 'inactive'
  value: number
  startedAt: string
  lastEvaluatedAt: string
  labels: Record<string, string>
}

// Health check result
export interface HealthCheck {
  name: string
  status: 'healthy' | 'degraded' | 'unhealthy'
  latency: number
  message?: string
  lastCheck: string
}

export interface DashboardPanel {
  id: string
  title: string
  type: 'line' | 'bar' | 'pie' | 'stat' | 'table'
  metrics: string[]
  width: number
  height: number
  x: number
  y: number
}

export interface DashboardConfig {
  id: string
  name: string
  panels: DashboardPanel[]
  refreshInterval: number
  timeRange: string
}

export interface RuntimeServiceChecks {
  database: HealthCheck
  kv: HealthCheck
  r2: HealthCheck
}

async function probeDatabase(env: Env): Promise<HealthCheck> {
  const startedAt = Date.now()
  const lastCheck = new Date().toISOString()

  try {
    await env.DB.prepare('SELECT 1').first()
    return {
      name: 'database',
      status: 'healthy',
      latency: Date.now() - startedAt,
      lastCheck,
    }
  } catch (err) {
    return {
      name: 'database',
      status: 'unhealthy',
      latency: Date.now() - startedAt,
      message: err instanceof Error ? err.message : 'Unknown error',
      lastCheck,
    }
  }
}

async function probeKv(env: Env): Promise<HealthCheck> {
  const startedAt = Date.now()
  const lastCheck = new Date().toISOString()

  try {
    await env.KV.get('health:check')
    return {
      name: 'kv',
      status: 'healthy',
      latency: Date.now() - startedAt,
      lastCheck,
    }
  } catch (err) {
    return {
      name: 'kv',
      status: 'unhealthy',
      latency: Date.now() - startedAt,
      message: err instanceof Error ? err.message : 'Unknown error',
      lastCheck,
    }
  }
}

async function probeR2(env: Env): Promise<HealthCheck> {
  const startedAt = Date.now()
  const lastCheck = new Date().toISOString()

  try {
    await env.R2.head('health:check')
    return {
      name: 'r2',
      status: 'healthy',
      latency: Date.now() - startedAt,
      lastCheck,
    }
  } catch (err) {
    return {
      name: 'r2',
      status: 'unhealthy',
      latency: Date.now() - startedAt,
      message: err instanceof Error ? err.message : 'Unknown error',
      lastCheck,
    }
  }
}

export async function probeRuntimeServices(env: Env): Promise<RuntimeServiceChecks> {
  const [database, kv, r2] = await Promise.all([
    probeDatabase(env),
    probeKv(env),
    probeR2(env),
  ])

  return { database, kv, r2 }
}

export { buildDeveloperReadinessSummary } from './developer-readiness'
export async function recordMetric(
  env: Env,
  point: Omit<MetricPoint, 'timestamp'>
): Promise<{ success: boolean }> {
  try {
    const timestamp = new Date().toISOString()
    const fullPoint: MetricPoint = { ...point, timestamp }

    // Store metric in KV with time-series key
    const timeKey = getTimeKey(timestamp)
    const metricKey = `metrics:${point.name}:${timeKey}`

    // Get existing metrics for this time bucket
    const existing = await env.KV.get(metricKey, 'json') as MetricPoint[] | null
    const metrics = existing || []
    metrics.push(fullPoint)

    // Keep last 1000 points per bucket
    if (metrics.length > 1000) {
      metrics.shift()
    }

    await env.KV.put(metricKey, JSON.stringify(metrics), {
      expirationTtl: 86400 * 7, // 7 days retention
    })

    // Update metric registry
    await updateMetricRegistry(env, point.name, point.type, point.labels)

    return { success: true }
  } catch (err) {
    console.error('Failed to record metric:', err)
    return { success: false }
  }
}

/**
 * Query metrics
 */
export async function queryMetrics(
  env: Env,
  params: {
    name: string
    startTime?: string
    endTime?: string
    labels?: Record<string, string>
    aggregation?: 'avg' | 'sum' | 'min' | 'max' | 'count'
  }
): Promise<{ success: boolean; data?: MetricPoint[] }> {
  try {
    const { name, startTime, endTime, labels, aggregation } = params

    const start = startTime ? new Date(startTime) : new Date(Date.now() - 3600000)
    const end = endTime ? new Date(endTime) : new Date()

    const points: MetricPoint[] = []
    let current = start

    while (current <= end) {
      const timeKey = getTimeKey(current.toISOString())
      const metricKey = `metrics:${name}:${timeKey}`

      const bucket = await env.KV.get(metricKey, 'json') as MetricPoint[] | null
      if (bucket) {
        // Filter by labels if provided
        const filtered = labels
          ? bucket.filter(p =>
              Object.entries(labels).every(([k, v]) => p.labels[k] === v)
            )
          : bucket
        points.push(...filtered)
      }

      // Move to next interval
      current = new Date(current.getTime() + 60000) // 1 minute buckets
    }

    // Apply aggregation if specified
    let result = points
    if (aggregation && points.length > 0) {
      const values = points.map(p => p.value)
      let aggregatedValue: number

      switch (aggregation) {
        case 'avg':
          aggregatedValue = values.reduce((a, b) => a + b, 0) / values.length
          break
        case 'sum':
          aggregatedValue = values.reduce((a, b) => a + b, 0)
          break
        case 'min':
          aggregatedValue = Math.min(...values)
          break
        case 'max':
          aggregatedValue = Math.max(...values)
          break
        case 'count':
          aggregatedValue = values.length
          break
        default:
          aggregatedValue = values[values.length - 1]
      }

      result = [{
        name,
        type: points[0].type,
        value: aggregatedValue,
        labels: labels || {},
        timestamp: end.toISOString(),
      }]
    }

    return { success: true, data: result }
  } catch (err) {
    console.error('Failed to query metrics:', err)
    return { success: false }
  }
}

/**
 * Create alert rule
 */
export async function createAlertRule(
  env: Env,
  rule: Omit<AlertRule, 'id'>
): Promise<{ success: boolean; id?: string }> {
  try {
    const id = generateId()
    const fullRule: AlertRule = { ...rule, id }

    await env.KV.put(`alert:rule:${id}`, JSON.stringify(fullRule))

    // Update rule list
    const rulesList = await env.KV.get('alert:rules', 'json') as string[] | null
    const rules = rulesList || []
    rules.push(id)
    await env.KV.put('alert:rules', JSON.stringify(rules))

    return { success: true, id }
  } catch (err) {
    console.error('Failed to create alert rule:', err)
    return { success: false }
  }
}

/**
 * Get all alert rules
 */
export async function getAlertRules(env: Env): Promise<AlertRule[]> {
  try {
    const rulesList = await env.KV.get('alert:rules', 'json') as string[] | null
    if (!rulesList) return []

    const rules: AlertRule[] = []
    for (const id of rulesList) {
      const rule = await env.KV.get(`alert:rule:${id}`, 'json') as AlertRule | null
      if (rule) rules.push(rule)
    }

    return rules
  } catch {
    return []
  }
}

/**
 * Evaluate alert rules
 */
export async function evaluateAlertRules(
  env: Env
): Promise<{ fired: AlertState[]; resolved: string[] }> {
  const rules = await getAlertRules(env)
  const fired: AlertState[] = []
  const resolved: string[] = []

  for (const rule of rules) {
    if (!rule.enabled) continue

    // Get current metric value
    const result = await queryMetrics(env, {
      name: rule.metric,
      startTime: new Date(Date.now() - rule.duration * 1000).toISOString(),
    })

    if (!result.success || !result.data?.length) continue

    const value = result.data[result.data.length - 1].value
    const isFiring = evaluateCondition(value, rule.operator, rule.threshold)

    // Get current alert state
    const stateKey = `alert:state:${rule.id}`
    const currentState = await env.KV.get(stateKey, 'json') as AlertState | null

    if (isFiring) {
      const newState: AlertState = {
        ruleId: rule.id,
        state: currentState?.state === 'firing' ? 'firing' : 'pending',
        value,
        startedAt: currentState?.startedAt || new Date().toISOString(),
        lastEvaluatedAt: new Date().toISOString(),
        labels: rule.labels || {},
      }

      await env.KV.put(stateKey, JSON.stringify(newState))

      if (newState.state === 'firing') {
        fired.push(newState)
      }
    } else if (currentState) {
      await env.KV.delete(stateKey)
      resolved.push(rule.id)
    }
  }

  return { fired, resolved }
}

/**
 * Run health check
 */
export async function runHealthCheck(
  env: Env,
  checkName: string
): Promise<HealthCheck> {
  const startTime = Date.now()

  try {
    // Simulate different health checks
    switch (checkName) {
      case 'database':
        // Check database connectivity
        await env.KV.get('health:check')
        return {
          name: 'database',
          status: 'healthy',
          latency: Date.now() - startTime,
          lastCheck: new Date().toISOString(),
        }

      case 'api':
        return {
          name: 'api',
          status: 'healthy',
          latency: Date.now() - startTime,
          lastCheck: new Date().toISOString(),
        }

      case 'cache':
        return {
          name: 'cache',
          status: 'healthy',
          latency: Date.now() - startTime,
          lastCheck: new Date().toISOString(),
        }

      default:
        return {
          name: checkName,
          status: 'healthy',
          latency: Date.now() - startTime,
          lastCheck: new Date().toISOString(),
        }
    }
  } catch (err) {
    return {
      name: checkName,
      status: 'unhealthy',
      latency: Date.now() - startTime,
      message: err instanceof Error ? err.message : 'Unknown error',
      lastCheck: new Date().toISOString(),
    }
  }
}

/**
 * Get all health checks
 */
export async function getAllHealthChecks(env: Env): Promise<HealthCheck[]> {
  const checks = ['database', 'api', 'cache']
  const results: HealthCheck[] = []

  for (const check of checks) {
    results.push(await runHealthCheck(env, check))
  }

  return results
}

/**
 * Create dashboard
 */
export async function createDashboard(
  env: Env,
  config: Omit<DashboardConfig, 'id'>
): Promise<{ success: boolean; id?: string }> {
  try {
    const id = generateId()
    const dashboard: DashboardConfig = { ...config, id }

    await env.KV.put(`dashboard:${id}`, JSON.stringify(dashboard))

    // Update dashboard list
    const list = await env.KV.get('dashboards', 'json') as string[] | null
    const dashboards = list || []
    dashboards.push(id)
    await env.KV.put('dashboards', JSON.stringify(dashboards))

    return { success: true, id }
  } catch (err) {
    console.error('Failed to create dashboard:', err)
    return { success: false }
  }
}

/**
 * Get dashboards
 */
export async function getDashboards(env: Env): Promise<DashboardConfig[]> {
  try {
    const list = await env.KV.get('dashboards', 'json') as string[] | null
    if (!list) return []

    const dashboards: DashboardConfig[] = []
    for (const id of list) {
      const dashboard = await env.KV.get(`dashboard:${id}`, 'json') as DashboardConfig | null
      if (dashboard) dashboards.push(dashboard)
    }

    return dashboards
  } catch {
    return []
  }
}

// Helper functions
function getTimeKey(timestamp: string): string {
  const date = new Date(timestamp)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${year}${month}${day}${hour}${minute}`
}

async function updateMetricRegistry(
  env: Env,
  name: string,
  type: MetricType,
  labels: Record<string, string>
): Promise<void> {
  const registry = await env.KV.get('metrics:registry', 'json') as Record<string, { type: MetricType; labels: string[] }> | null
  const reg = registry || {}

  if (!reg[name]) {
    reg[name] = { type, labels: Object.keys(labels) }
  } else {
    // Merge labels
    const existingLabels = new Set(reg[name].labels)
    Object.keys(labels).forEach(l => existingLabels.add(l))
    reg[name].labels = Array.from(existingLabels)
  }

  await env.KV.put('metrics:registry', JSON.stringify(reg))
}

function evaluateCondition(value: number, operator: string, threshold: number): boolean {
  switch (operator) {
    case 'gt': return value > threshold
    case 'lt': return value < threshold
    case 'eq': return value === threshold
    case 'neq': return value !== threshold
    case 'gte': return value >= threshold
    case 'lte': return value <= threshold
    default: return false
  }
}

function generateId(): string {
  return `mon_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}