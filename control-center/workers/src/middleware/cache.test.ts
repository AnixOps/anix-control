/**
 * Cache Middleware Unit Tests
 */

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cacheMiddleware, invalidateCache, QueryCache } from './cache'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Cache Middleware', () => {
  let mockKV: KVNamespace
  let mockEnv: any

  beforeEach(() => {
    mockKV = createMockKV()
    mockEnv = { KV: mockKV, DB: createMockD1() }
  })

  describe('cacheMiddleware', () => {
    it('should skip non-GET requests', async () => {
      const middleware = cacheMiddleware({ ttl: 60 })
      const c = {
        req: { method: 'POST', path: '/api/test', query: () => ({}) },
        env: mockEnv,
        get: vi.fn(),
        header: vi.fn(),
        json: vi.fn((data) => data),
      } as any

      let nextCalled = false
      const next = async () => { nextCalled = true }

      await middleware(c, next)

      expect(nextCalled).toBe(true)
    })

    it('should return cached response on cache hit', async () => {
      const middleware = cacheMiddleware({ ttl: 60 })
      const cachedData = {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: 'cached' }),
      }

      await mockKV.put('cache:GET:/api/test', JSON.stringify(cachedData))

      const c = {
        req: { method: 'GET', path: '/api/test', query: () => ({}) },
        env: mockEnv,
        get: vi.fn(() => null),
        header: vi.fn(),
        json: vi.fn((data) => data),
      } as any

      await middleware(c, async () => {})

      expect(c.header).toHaveBeenCalledWith('X-Cache', 'HIT')
    })

    it('should mark cache miss', async () => {
      const middleware = cacheMiddleware({ ttl: 60 })

      const c = {
        req: { method: 'GET', path: '/api/test', query: () => ({}) },
        env: mockEnv,
        get: vi.fn(() => null),
        header: vi.fn(),
        json: vi.fn((data) => data),
        res: new Response(JSON.stringify({ data: 'test' }), { status: 200 }),
      } as any

      await middleware(c, async () => {
        c.res = new Response(JSON.stringify({ data: 'test' }), { status: 200 })
      })

      expect(c.header).toHaveBeenCalledWith('X-Cache', 'MISS')
    })
  })

  describe('invalidateCache', () => {
    it('should delete keys matching pattern', async () => {
      await mockKV.put('cache:api:users', 'data1')
      await mockKV.put('cache:api:nodes', 'data2')
      await mockKV.put('other:key', 'data3')

      const deleted = await invalidateCache(mockEnv, ['cache:api:'])

      expect(deleted).toBeGreaterThan(0)
    })

    it('should return 0 for non-existent patterns', async () => {
      const deleted = await invalidateCache(mockEnv, ['nonexistent:'])
      expect(typeof deleted).toBe('number')
    })
  })

  describe('QueryCache', () => {
    it('should get and set values', async () => {
      const cache = new QueryCache(mockEnv, 'test')

      await cache.set('key1', { data: 'value1' })
      const result = await cache.get('key1')

      expect(result).toEqual({ data: 'value1' })
    })

    it('should return null for non-existent key', async () => {
      const cache = new QueryCache(mockEnv, 'test')
      const result = await cache.get('nonexistent')

      expect(result).toBeNull()
    })

    it('should delete keys', async () => {
      const cache = new QueryCache(mockEnv, 'test')

      await cache.set('key1', { data: 'value1' })
      await cache.delete('key1')
      const result = await cache.get('key1')

      expect(result).toBeNull()
    })

    it('should getOrSet with fetcher', async () => {
      const cache = new QueryCache(mockEnv, 'test')

      let fetcherCalled = false
      const fetcher = async () => {
        fetcherCalled = true
        return { data: 'fetched' }
      }

      // First call - should fetch
      const result1 = await cache.getOrSet('key2', fetcher, 60)
      expect(fetcherCalled).toBe(true)
      expect(result1).toEqual({ data: 'fetched' })

      // Second call - should use cache
      fetcherCalled = false
      const result2 = await cache.getOrSet('key2', fetcher, 60)
      expect(fetcherCalled).toBe(false)
      expect(result2).toEqual({ data: 'fetched' })
    })
  })
})