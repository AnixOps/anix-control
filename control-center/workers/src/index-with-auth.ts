import { Hono } from 'hono'
import { hash, compare } from 'bcryptjs'
import { SignJWT } from 'jose'
import { probeRuntimeServices } from './services/monitoring'
import type { ApiErrorResponse, AuthLoginResponse, AuthRegisterResponse, Env, HealthResponse, ReadinessResponse, ServiceErrorResponse } from './types'

const app = new Hono<{ Bindings: Env }>()

async function generateToken(
  payload: { sub: number; email: string; role: string },
  secret: Uint8Array,
  expiresIn: string
): Promise<string> {
  return await new SignJWT({ ...payload, sub: String(payload.sub) })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(expiresIn)
    .sign(secret)
}

app.get('/health', (c) => {
  return c.json({
    status: 'healthy',
    version: c.env.APP_VERSION || '1.0.0',
    build_sha: c.env.BUILD_SHA || 'unknown',
    timestamp: new Date().toISOString(),
    environment: c.env.ENVIRONMENT,
  } as HealthResponse)
})

app.get('/readiness', async (c) => {
  try {
    const checks = await probeRuntimeServices(c.env)
    const allHealthy = [checks.database, checks.kv, checks.r2].every(check => check.status === 'healthy')

    return c.json({
      status: allHealthy ? 'ready' : 'degraded',
      version: c.env.APP_VERSION || '1.0.0',
      build_sha: c.env.BUILD_SHA || 'unknown',
      checks,
      timestamp: new Date().toISOString(),
    } as ReadinessResponse)
  } catch (err) {
    return c.json({ status: 'error', error: String(err) } as ServiceErrorResponse, 500)
  }
})

app.post('/api/v1/auth/register', async (c) => {
  try {
    const body = await c.req.json()
    const { email, password, role } = body

    // Check if user exists
    const existing = await c.env.DB
      .prepare('SELECT id FROM users WHERE email = ?')
      .bind(email)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Email already registered' } as ApiErrorResponse, 409)
    }

    // Hash password
    const passwordHash = await hash(password, 12)

    // Create user
    const result = await c.env.DB
      .prepare('INSERT INTO users (email, password_hash, role, auth_provider, enabled) VALUES (?, ?, ?, ?, 1) RETURNING id, email, role, created_at')
      .bind(email, passwordHash, role || 'viewer', 'local')
      .first<{ id: number; email: string; role: string; created_at: string }>()

    return c.json({ success: true, data: result } as AuthRegisterResponse, 201)
  } catch (err) {
    return c.json({ success: false, error: String(err) } as ApiErrorResponse, 500)
  }
})

app.post('/api/v1/auth/login', async (c) => {
  try {
    const body = await c.req.json()
    const { email, password } = body

    // Find user
    const user = await c.env.DB
      .prepare('SELECT * FROM users WHERE email = ? AND enabled = 1')
      .bind(email)
      .first<{ id: number; email: string; password_hash: string; role: string }>()

    if (!user || !user.password_hash) {
      return c.json({ success: false, error: 'Invalid credentials' } as ApiErrorResponse, 401)
    }

    // Verify password
    const valid = await compare(password, user.password_hash)
    if (!valid) {
      return c.json({ success: false, error: 'Invalid credentials' } as ApiErrorResponse, 401)
    }

    // Generate JWT
    const secret = new TextEncoder().encode(c.env.JWT_SECRET)
    const expire = parseInt(c.env.JWT_EXPIRE, 10) || 86400
    const accessToken = await generateToken(
      { sub: user.id, email: user.email, role: user.role },
      secret,
      `${expire}s`
    )
    const refreshToken = await generateToken(
      { sub: user.id, email: '', role: '' },
      secret,
      '7d'
    )

    return c.json({
      success: true,
      data: {
        access_token: accessToken,
        refresh_token: refreshToken,
        token_type: 'Bearer',
        expires_in: expire,
        user: {
          id: user.id,
          email: user.email,
          role: user.role,
        },
      },
    } as AuthLoginResponse)
  } catch (err) {
    return c.json({ success: false, error: String(err) } as ApiErrorResponse, 500)
  }
})

export default app
