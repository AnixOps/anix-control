import type { Hono } from 'hono'
import type { Env } from '../types'
import { loginHandler, registerHandler, refreshHandler, logoutHandler } from '../handlers/auth'
import { rateLimitMiddleware } from '../middleware/rate-limit'

export function registerAuthRoutes(app: Hono<{ Bindings: Env }>) {
  app.post('/api/v1/auth/login', rateLimitMiddleware({ windowMs: 60000, max: 5 }), loginHandler)
  app.post('/api/v1/auth/register', rateLimitMiddleware({ windowMs: 60000, max: 3 }), registerHandler)
  app.post('/api/v1/auth/refresh', refreshHandler)
  app.post('/api/v1/auth/logout', logoutHandler)
}
