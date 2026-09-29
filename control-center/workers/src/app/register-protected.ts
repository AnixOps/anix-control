import type { Hono } from 'hono'
import type { Env } from '../types'
import { registerProtectedCoreRoutes } from './register-protected-core'
import { registerProtectedIncidentRoutes } from './register-protected-incidents'
import { registerProtectedSystemRoutes } from './register-protected-system'

export function registerProtectedRoutes(app: Hono<{ Bindings: Env }>) {
  registerProtectedCoreRoutes(app)
  registerProtectedIncidentRoutes(app)
  registerProtectedSystemRoutes(app)
}
