import { Hono } from 'hono'
import { logger } from 'hono/logger'
import { prettyJSON } from 'hono/pretty-json'
import { cors } from 'hono/cors'
import type { Env } from '../types'
import { healthHandler, readinessHandler } from '../handlers/health'
import { detailedHealthHandler, livenessHandler, prometheusMetricsHandler } from '../handlers/metrics'

export function registerPlatformRoutes(app: Hono<{ Bindings: Env }>) {
  app.use('*', logger())
  app.use('*', prettyJSON())
  app.use('*', cors({
    origin: (origin) => {
      const allowed = [
        'http://localhost:3000',
        'http://localhost:5173',
        'https://anixops.pages.dev',
        'https://anixops.dev',
        'https://www.anixops.dev',
        'https://api.anixops.com',
      ]
      if (allowed.includes(origin)) return origin
      return allowed[0]
    },
    allowMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS', 'PATCH'],
    allowHeaders: ['Content-Type', 'Authorization', 'X-API-Key'],
    exposeHeaders: ['X-Total-Count'],
    credentials: true,
    maxAge: 86400,
  }))

  app.get('/health', healthHandler)
  app.get('/health/detailed', detailedHealthHandler)
  app.get('/readiness', readinessHandler)
  app.get('/liveness', livenessHandler)
  app.get('/metrics', prometheusMetricsHandler)
}
