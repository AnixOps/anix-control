import type { Context } from 'hono'
import type { DashboardOverviewData, DashboardOverviewResponseData, Env, DeveloperReadinessSummary } from '../types'
import { buildDeveloperReadinessSummary } from '../services/monitoring'

/**
 * Dashboard 概览
 */
export async function dashboardHandler(c: Context<{ Bindings: Env }>) {
  // 尝试从缓存获取
  const cacheKey = 'dashboard:overview'
  const cached = await c.env.KV.get(cacheKey)

  if (cached) {
    return c.json({
      success: true,
      data: JSON.parse(cached),
      cached: true,
    } as DashboardOverviewResponseData)
  }

  let developerReadinessSummary: DeveloperReadinessSummary | null = null
  try {
    developerReadinessSummary = await buildDeveloperReadinessSummary()
  } catch (error) {
    console.error('Failed to build developer readiness summary:', error)
    developerReadinessSummary = null
  }

  const [nodeCount, userCount, auditCount] = await Promise.all([
    c.env.DB.prepare('SELECT COUNT(*) as count FROM nodes').first<{ count: number }>(),
    c.env.DB.prepare('SELECT COUNT(*) as count FROM users WHERE enabled = 1').first<{ count: number }>(),
    c.env.DB.prepare('SELECT COUNT(*) as count FROM audit_logs WHERE created_at > datetime(\'now\', \'-24 hours\')').first<{ count: number }>(),
  ])

  // 获取节点状态分布
  const nodeStatus = await c.env.DB
    .prepare('SELECT status, COUNT(*) as count FROM nodes GROUP BY status')
    .all<{ status: string; count: number }>()

  const data: DashboardOverviewData = {
    nodes: {
      total: nodeCount?.count || 0,
      online: nodeStatus.results.find(r => r.status === 'online')?.count || 0,
      offline: nodeStatus.results.find(r => r.status === 'offline')?.count || 0,
      maintenance: nodeStatus.results.find(r => r.status === 'maintenance')?.count || 0,
    },
    users: {
      total: userCount?.count || 0,
    },
    activity: {
      last_24h: auditCount?.count || 0,
    },
    developer_readiness_summary: developerReadinessSummary,
    timestamp: new Date().toISOString(),
  }

  // 缓存 1 分钟
  await c.env.KV.put(cacheKey, JSON.stringify(data), { expirationTtl: 60 })

  return c.json({
    success: true,
    data,
    cached: false,
  } as DashboardOverviewResponseData)
}

/**
 * 详细统计
 */
export async function statsHandler(c: Context<{ Bindings: Env }>) {
  const range = c.req.query('range') || '24h'

  let timeFilter: string
  switch (range) {
    case '7d':
      timeFilter = "datetime('now', '-7 days')"
      break
    case '30d':
      timeFilter = "datetime('now', '-30 days')"
      break
    default:
      timeFilter = "datetime('now', '-24 hours')"
  }

  // 用户活跃度
  const activeUsers = await c.env.DB
    .prepare(`SELECT COUNT(DISTINCT user_id) as count FROM audit_logs WHERE created_at > ${timeFilter}`)
    .first<{ count: number }>()

  // 操作分布
  const actionDistribution = await c.env.DB
    .prepare(`
      SELECT action, COUNT(*) as count
      FROM audit_logs
      WHERE created_at > ${timeFilter}
      GROUP BY action
      ORDER BY count DESC
      LIMIT 10
    `)
    .all<{ action: string; count: number }>()

  // 节点操作统计
  const nodeOperations = await c.env.DB
    .prepare(`
      SELECT resource, COUNT(*) as count
      FROM audit_logs
      WHERE resource = 'node' AND created_at > ${timeFilter}
      GROUP BY resource
    `)
    .all<{ resource: string; count: number }>()

  return c.json({
    success: true,
    data: {
      range,
      active_users: activeUsers?.count || 0,
      action_distribution: actionDistribution.results,
      node_operations: nodeOperations.results,
    },
  })
}