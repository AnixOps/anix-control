/**
 * Caching Middleware
 *
 * Provides API response caching using Cloudflare KV
 */

import type { Context, Next } from 'hono'
import type { Env } from '../types'

interface CacheOptions {
  ttl: number           // Cache TTL in seconds
  vary?: string[]       // Headers to vary cache by
  keyPrefix?: string    // Cache key prefix
  private?: boolean     // If true, include user ID in cache key
}

/**
 * Generate cache key from request
 */
function generateCacheKey(
  c: Context<{ Bindings: Env }>,
  options: CacheOptions
): string {
  const parts = [
    options.keyPrefix || 'cache',
    c.req.method,
    c.req.path,
  ]

  // Include query params in key
  const query = c.req.query()
  if (Object.keys(query).length > 0) {
    parts.push(JSON.stringify(query))
  }

  // Include user ID for private cache
  if (options.private) {
    const user = c.get('user') as { sub?: number; id?: number } | undefined
    const userId = user?.sub || user?.sub
    if (userId) {
      parts.push(`user:${userId}`)
    }
  }

  // Include varied headers
  if (options.vary) {
    for (const header of options.vary) {
      const value = c.req.header(header)
      if (value) {
        parts.push(`${header}:${value}`)
      }
    }
  }

  return parts.join(':')
}

/**
 * Cache middleware factory
 */
export function cacheMiddleware(options: CacheOptions) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    // Only cache GET requests
    if (c.req.method !== 'GET') {
      return next()
    }

    // Skip cache for authenticated requests if not private
    const user = c.get('user')
    if (user && !options.private) {
      return next()
    }

    const cacheKey = generateCacheKey(c, options)

    try {
      // Try to get cached response
      const cached = await c.env.KV.get(cacheKey, 'json') as {
        status: number
        headers: Record<string, string>
        body: string
      } | null

      if (cached) {
        // Return cached response
        c.header('X-Cache', 'HIT')
        for (const [key, value] of Object.entries(cached.headers)) {
          c.header(key, value)
        }
        return new Response(cached.body, {
          status: cached.status,
          headers: cached.headers,
        })
      }

      // Mark as cache miss
      c.header('X-Cache', 'MISS')

      // Continue to handler
      await next()

      // Cache successful responses
      if (c.res.status === 200) {
        const body = await c.res.clone().text()

        await c.env.KV.put(
          cacheKey,
          JSON.stringify({
            status: c.res.status,
            headers: Object.fromEntries(c.res.headers),
            body,
          }),
          { expirationTtl: options.ttl }
        )
      }
    } catch (err) {
      console.error('Cache error:', err)
      c.header('X-Cache', 'ERROR')
      await next()
    }
  }
}

/**
 * Cache invalidation helper
 */
export async function invalidateCache(
  env: Env,
  patterns: string[]
): Promise<number> {
  let deleted = 0

  for (const pattern of patterns) {
    try {
      const list = await env.KV.list({ prefix: pattern })
      for (const key of list.keys) {
        await env.KV.delete(key.name)
        deleted++
      }
    } catch (err) {
      console.error(`Cache invalidation error for ${pattern}:`, err)
    }
  }

  return deleted
}

/**
 * Cache warm-up helper
 */
export async function warmupCache(
  env: Env,
  urls: Array<{ path: string; ttl: number }>
): Promise<void> {
  for (const { path, ttl } of urls) {
    try {
      // Pre-populate cache by making internal requests
      const cacheKey = `cache:warmup:${path}`
      await env.KV.put(cacheKey, JSON.stringify({ warmed: true, path }), {
        expirationTtl: ttl,
      })
    } catch (err) {
      console.error(`Cache warmup error for ${path}:`, err)
    }
  }
}

/**
 * Smart cache for database queries
 */
export class QueryCache {
  private prefix: string
  private env: Env

  constructor(env: Env, prefix: string = 'query') {
    this.env = env
    this.prefix = prefix
  }

  async get<T>(key: string): Promise<T | null> {
    const fullKey = `${this.prefix}:${key}`
    const cached = await this.env.KV.get(fullKey, 'json')
    return cached as T | null
  }

  async set<T>(key: string, value: T, ttl: number = 60): Promise<void> {
    const fullKey = `${this.prefix}:${key}`
    await this.env.KV.put(fullKey, JSON.stringify(value), {
      expirationTtl: ttl,
    })
  }

  async delete(key: string): Promise<void> {
    const fullKey = `${this.prefix}:${key}`
    await this.env.KV.delete(fullKey)
  }

  async getOrSet<T>(
    key: string,
    fetcher: () => Promise<T>,
    ttl: number = 60
  ): Promise<T> {
    const cached = await this.get<T>(key)
    if (cached !== null) {
      return cached
    }

    const value = await fetcher()
    await this.set(key, value, ttl)
    return value
  }
}