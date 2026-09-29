import type { Hono } from 'hono'
import type { Env } from '../types'
import { registerAuthRoutes } from './register-auth'
import { registerPlatformRoutes } from './register-platform'
import { registerProtectedRoutes } from './register-protected'

export function createApp(app: Hono<{ Bindings: Env }>) {
  registerPlatformRoutes(app)
  registerAuthRoutes(app)
  registerProtectedRoutes(app)
  return app
}
