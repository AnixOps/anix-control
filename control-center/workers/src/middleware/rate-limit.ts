import type { Context, Next } from 'hono'
import type { Env } from '../types'

interface RateLimitOptions {
  windowMs: number  // 时间窗口 (毫秒)
  max: number       // 最大请求数
  keyGenerator?: (c: Context<{ Bindings: Env }>) => string
  skipFailedRequests?: boolean  // 是否跳过失败请求
  skipSuccessfulRequests?: boolean  // 是否跳过成功请求
}

interface TenantRateLimit {
  tenant_id: number
  requests_per_minute: number
  requests_per_hour: number
  requests_per_day: number
}

// 默认租户限流配置
const DEFAULT_TENANT_LIMITS: Record<string, TenantRateLimit> = {
  free: {
    tenant_id: 0,
    requests_per_minute: 60,
    requests_per_hour: 1000,
    requests_per_day: 10000,
  },
  pro: {
    tenant_id: 0,
    requests_per_minute: 300,
    requests_per_hour: 5000,
    requests_per_day: 100000,
  },
  enterprise: {
    tenant_id: 0,
    requests_per_minute: 1000,
    requests_per_hour: 50000,
    requests_per_day: -1, // 无限制
  },
}

/**
 * 速率限制中间件
 * 使用 KV 存储实现
 */
export function rateLimitMiddleware(options: RateLimitOptions) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const key = options.keyGenerator
      ? options.keyGenerator(c)
      : `ratelimit:${c.req.header('CF-Connecting-IP') || 'unknown'}:${c.req.path}`

    try {
      // 获取当前计数
      const stored = await c.env.KV.get(key)
      const count = stored ? parseInt(stored, 10) : 0

      if (count >= options.max) {
        return c.json({
          success: false,
          error: 'Too Many Requests',
          retry_after: Math.ceil(options.windowMs / 1000),
        }, 429, {
          'Retry-After': String(Math.ceil(options.windowMs / 1000)),
          'X-RateLimit-Limit': String(options.max),
          'X-RateLimit-Remaining': '0',
          'X-RateLimit-Reset': String(Math.ceil(Date.now() / 1000 + options.windowMs / 1000)),
        })
      }

      // 增加计数
      await c.env.KV.put(key, String(count + 1), {
        expirationTtl: Math.ceil(options.windowMs / 1000),
      })

      // 设置响应头
      c.header('X-RateLimit-Limit', String(options.max))
      c.header('X-RateLimit-Remaining', String(options.max - count - 1))

      await next()
    } catch (err) {
      // KV 错误不应该阻止请求
      console.error('Rate limit error:', err)
      await next()
    }
  }
}

/**
 * 租户级速率限制中间件
 */
export function tenantRateLimitMiddleware() {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const tenant = c.get('tenant')
    if (!tenant) {
      return next()
    }

    const plan = tenant.plan || 'free'
    const limits = DEFAULT_TENANT_LIMITS[plan] || DEFAULT_TENANT_LIMITS.free
    const tenantId = tenant.id

    try {
      // 检查分钟级限制
      const minuteKey = `ratelimit:tenant:${tenantId}:minute:${Math.floor(Date.now() / 60000)}`
      const minuteCount = parseInt(await c.env.KV.get(minuteKey) || '0', 10)

      if (limits.requests_per_minute > 0 && minuteCount >= limits.requests_per_minute) {
        return c.json({
          success: false,
          error: 'Rate limit exceeded for your plan',
          plan,
          limit: limits.requests_per_minute,
          window: 'minute',
        }, 429, {
          'X-RateLimit-Limit': String(limits.requests_per_minute),
          'X-RateLimit-Remaining': '0',
          'X-RateLimit-Plan': plan,
        })
      }

      // 检查小时级限制
      const hourKey = `ratelimit:tenant:${tenantId}:hour:${Math.floor(Date.now() / 3600000)}`
      const hourCount = parseInt(await c.env.KV.get(hourKey) || '0', 10)

      if (limits.requests_per_hour > 0 && hourCount >= limits.requests_per_hour) {
        return c.json({
          success: false,
          error: 'Hourly rate limit exceeded for your plan',
          plan,
          limit: limits.requests_per_hour,
          window: 'hour',
        }, 429)
      }

      // 检查日级限制
      const dayKey = `ratelimit:tenant:${tenantId}:day:${Math.floor(Date.now() / 86400000)}`
      const dayCount = parseInt(await c.env.KV.get(dayKey) || '0', 10)

      if (limits.requests_per_day > 0 && dayCount >= limits.requests_per_day) {
        return c.json({
          success: false,
          error: 'Daily rate limit exceeded for your plan',
          plan,
          limit: limits.requests_per_day,
          window: 'day',
        }, 429)
      }

      // 增加计数
      await c.env.KV.put(minuteKey, String(minuteCount + 1), { expirationTtl: 60 })
      await c.env.KV.put(hourKey, String(hourCount + 1), { expirationTtl: 3600 })
      await c.env.KV.put(dayKey, String(dayCount + 1), { expirationTtl: 86400 })

      // 设置响应头
      c.header('X-RateLimit-Limit', String(limits.requests_per_minute))
      c.header('X-RateLimit-Remaining', String(Math.max(0, limits.requests_per_minute - minuteCount - 1)))
      c.header('X-RateLimit-Plan', plan)

      await next()
    } catch (err) {
      console.error('Tenant rate limit error:', err)
      await next()
    }
  }
}

/**
 * API端点级限流
 */
export function endpointRateLimitMiddleware(
  limits: Record<string, { windowMs: number; max: number }>
) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const path = c.req.path
    const endpoint = Object.keys(limits).find(ep => path.startsWith(ep))

    if (!endpoint) {
      return next()
    }

    const config = limits[endpoint]
    const key = `ratelimit:endpoint:${endpoint}:${c.req.header('CF-Connecting-IP') || 'unknown'}`

    try {
      const count = parseInt(await c.env.KV.get(key) || '0', 10)

      if (count >= config.max) {
        return c.json({
          success: false,
          error: 'Endpoint rate limit exceeded',
          endpoint,
          retry_after: Math.ceil(config.windowMs / 1000),
        }, 429)
      }

      await c.env.KV.put(key, String(count + 1), {
        expirationTtl: Math.ceil(config.windowMs / 1000),
      })

      await next()
    } catch (err) {
      console.error('Endpoint rate limit error:', err)
      await next()
    }
  }
}

/**
 * IP 白名单中间件
 */
export function ipWhitelistMiddleware(allowedIPs: string[]) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const clientIP = c.req.header('CF-Connecting-IP')

    if (!clientIP || !allowedIPs.includes(clientIP)) {
      return c.json({ success: false, error: 'Forbidden: IP not allowed' }, 403)
    }

    await next()
  }
}

/**
 * 获取租户限流使用情况
 */
export async function getTenantRateLimitUsage(
  env: Env,
  tenantId: number
): Promise<{
  minute: { used: number; limit: number }
  hour: { used: number; limit: number }
  day: { used: number; limit: number }
}> {
  const minuteKey = `ratelimit:tenant:${tenantId}:minute:${Math.floor(Date.now() / 60000)}`
  const hourKey = `ratelimit:tenant:${tenantId}:hour:${Math.floor(Date.now() / 3600000)}`
  const dayKey = `ratelimit:tenant:${tenantId}:day:${Math.floor(Date.now() / 86400000)}`

  const [minuteUsed, hourUsed, dayUsed] = await Promise.all([
    parseInt(await env.KV.get(minuteKey) || '0', 10),
    parseInt(await env.KV.get(hourKey) || '0', 10),
    parseInt(await env.KV.get(dayKey) || '0', 10),
  ])

  return {
    minute: { used: minuteUsed, limit: DEFAULT_TENANT_LIMITS.free.requests_per_minute },
    hour: { used: hourUsed, limit: DEFAULT_TENANT_LIMITS.free.requests_per_hour },
    day: { used: dayUsed, limit: DEFAULT_TENANT_LIMITS.free.requests_per_day },
  }
}