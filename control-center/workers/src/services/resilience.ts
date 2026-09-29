import { Hono } from 'hono'
import type { ApiErrorResponse, ApiSuccessResponse } from '../types'

const resilience = new Hono()

// Circuit Breaker configurations
const circuitBreakers = new Map<string, {
  state: 'closed' | 'open' | 'half-open'
  failureCount: number
  successCount: number
  lastFailureTime: number | null
  config: {
    failureThreshold: number
    successThreshold: number
    timeout: number
  }
}>()

// Rate Limiter configurations
const rateLimiters = new Map<string, {
  tokens: number
  lastRefill: number
  config: {
    maxTokens: number
    refillRate: number
    refillInterval: number
  }
}>()

// Retry configurations
const retryConfigs = new Map<string, {
  maxRetries: number
  backoffMultiplier: number
  initialDelay: number
  maxDelay: number
}>()

// Get circuit breaker status
resilience.get('/circuit-breakers', (c) => {
  const breakers = Array.from(circuitBreakers.entries()).map(([name, data]) => ({
    name,
    state: data.state,
    failureCount: data.failureCount,
    successCount: data.successCount,
    lastFailureTime: data.lastFailureTime
  }))

  return c.json({ circuitBreakers: breakers })
})

// Create circuit breaker
resilience.post('/circuit-breakers', async (c) => {
  const body = await c.req.json()
  const { name, failureThreshold = 5, successThreshold = 3, timeout = 60000 } = body

  if (!name) {
    return c.json({ success: false, error: 'Name is required' } as ApiErrorResponse, 400)
  }

  circuitBreakers.set(name, {
    state: 'closed',
    failureCount: 0,
    successCount: 0,
    lastFailureTime: null,
    config: { failureThreshold, successThreshold, timeout }
  })

  return c.json({ success: true, name, state: 'closed' })
})

// Update circuit breaker (record success/failure)
resilience.post('/circuit-breakers/:name/event', async (c) => {
  const name = c.req.param('name') as string
  const body = await c.req.json()
  const { type } = body // 'success' or 'failure'

  const breaker = circuitBreakers.get(name)
  if (!breaker) {
    return c.json({ success: false, error: 'Circuit breaker not found' } as ApiErrorResponse, 404)
  }

  const now = Date.now()

  if (type === 'failure') {
    breaker.failureCount++
    breaker.lastFailureTime = now

    if (breaker.state === 'half-open') {
      breaker.state = 'open'
    } else if (breaker.failureCount >= breaker.config.failureThreshold) {
      breaker.state = 'open'
    }
  } else if (type === 'success') {
    breaker.successCount++
    breaker.failureCount = 0

    if (breaker.state === 'half-open' && breaker.successCount >= breaker.config.successThreshold) {
      breaker.state = 'closed'
      breaker.successCount = 0
    }
  }

  return c.json({ success: true, state: breaker.state })
})

// Get rate limiters
resilience.get('/rate-limiters', (c) => {
  const limiters = Array.from(rateLimiters.entries()).map(([name, data]) => ({
    name,
    tokens: data.tokens,
    maxTokens: data.config.maxTokens,
    refillRate: data.config.refillRate
  }))

  return c.json({ rateLimiters: limiters })
})

// Create rate limiter
resilience.post('/rate-limiters', async (c) => {
  const body = await c.req.json()
  const { name, maxTokens = 100, refillRate = 10, refillInterval = 1000 } = body

  if (!name) {
    return c.json({ success: false, error: 'Name is required' } as ApiErrorResponse, 400)
  }

  rateLimiters.set(name, {
    tokens: maxTokens,
    lastRefill: Date.now(),
    config: { maxTokens, refillRate, refillInterval }
  })

  return c.json({ success: true, name, tokens: maxTokens })
})

// Check rate limit
resilience.post('/rate-limiters/:name/check', (c) => {
  const name = c.req.param('name') as string
  const limiter = rateLimiters.get(name)

  if (!limiter) {
    return c.json({ success: false, error: 'Rate limiter not found' } as ApiErrorResponse, 404)
  }

  const now = Date.now()
  const timePassed = now - limiter.lastRefill
  const tokensToAdd = Math.floor(timePassed / limiter.config.refillInterval) * limiter.config.refillRate

  limiter.tokens = Math.min(limiter.config.maxTokens, limiter.tokens + tokensToAdd)
  limiter.lastRefill = now

  if (limiter.tokens > 0) {
    limiter.tokens--
    return c.json({ allowed: true, remaining: limiter.tokens })
  }

  return c.json({ allowed: false, remaining: 0 }, 429)
})

// Get retry configurations
resilience.get('/retries', (c) => {
  const configs = Array.from(retryConfigs.entries()).map(([name, config]) => ({
    name,
    ...config
  }))

  return c.json({ retryConfigs: configs })
})

// Create retry configuration
resilience.post('/retries', async (c) => {
  const body = await c.req.json()
  const { name, maxRetries = 3, backoffMultiplier = 2, initialDelay = 100, maxDelay = 30000 } = body

  if (!name) {
    return c.json({ success: false, error: 'Name is required' } as ApiErrorResponse, 400)
  }

  retryConfigs.set(name, { maxRetries, backoffMultiplier, initialDelay, maxDelay })

  return c.json({ success: true, name })
})

// Calculate retry delay
resilience.post('/retries/:name/delay', (c) => {
  const name = c.req.param('name') as string
  const attempt = parseInt(c.req.query('attempt') || '1')

  const config = retryConfigs.get(name)
  if (!config) {
    return c.json({ success: false, error: 'Retry config not found' } as ApiErrorResponse, 404)
  }

  const delay = Math.min(
    config.initialDelay * Math.pow(config.backoffMultiplier, attempt - 1),
    config.maxDelay
  )

  return c.json({ delay, attempt, maxRetries: config.maxRetries })
})

// Get resilience stats
resilience.get('/stats', (c) => {
  const openBreakers = Array.from(circuitBreakers.values()).filter(b => b.state === 'open').length
  const totalBreakers = circuitBreakers.size
  const totalLimiters = rateLimiters.size
  const totalRetries = retryConfigs.size

  return c.json({
    circuitBreakers: {
      total: totalBreakers,
      open: openBreakers,
      closed: totalBreakers - openBreakers
    },
    rateLimiters: totalLimiters,
    retryConfigs: totalRetries
  })
})

export default resilience