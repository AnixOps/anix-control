import type { Context, Next } from 'hono'
import { compare } from 'bcryptjs'
import { jwtVerify } from 'jose'
import type { AuthPrincipal, Env } from '../types'
import { isTokenRevoked, isUserSessionRevoked } from '../utils/token'

// Internal JWT Payload type
interface AppJWTPayload {
  sub: number
  email: string
  role: string
  iat: number
  exp: number
}

type ApiTokenRecord = {
  token_id: number
  token_name: string
  token_hash: string
  user_id: number
  email: string
  role: string
}

/**
 * 检查 Token 是否在黑名单中
 */
async function isTokenBlacklisted(kv: KVNamespace, token: string): Promise<boolean> {
  return isTokenRevoked(kv, token)
}

async function authenticateJwt(c: Context<{ Bindings: Env }>, token: string): Promise<AuthPrincipal | Response> {
  if (await isTokenBlacklisted(c.env.KV, token)) {
    return c.json({ success: false, error: 'Token has been revoked' }, 401)
  }

  const secret = new TextEncoder().encode(c.env.JWT_SECRET)
  const { payload } = await jwtVerify(token, secret)

  const userPayload: AppJWTPayload = {
    sub: typeof payload.sub === 'string' ? parseInt(payload.sub, 10) : (typeof payload.sub === 'number' ? payload.sub : 0),
    email: payload.email as string,
    role: payload.role as string,
    iat: payload.iat || 0,
    exp: payload.exp || 0,
  }

  if (await isUserSessionRevoked(c.env.KV, userPayload.sub, userPayload.iat)) {
    return c.json({ success: false, error: 'Session has been revoked. Please login again.' }, 401)
  }

  return {
    ...userPayload,
    kind: 'user',
    auth_method: 'jwt',
  }
}

async function authenticateApiKey(c: Context<{ Bindings: Env }>, apiKey: string): Promise<AuthPrincipal | Response> {
  const tokens = await c.env.DB
    .prepare(`
      SELECT t.id as token_id, t.name as token_name, t.token as token_hash, t.user_id, u.email, u.role
      FROM api_tokens t
      JOIN users u ON t.user_id = u.id
      WHERE (t.expires_at IS NULL OR t.expires_at > datetime('now'))
      ORDER BY t.created_at DESC
    `)
    .all<ApiTokenRecord>()

  for (const record of tokens.results) {
    if (await compare(apiKey, record.token_hash)) {
      await c.env.DB
        .prepare('UPDATE api_tokens SET last_used = datetime(\'now\') WHERE id = ?')
        .bind(record.token_id)
        .run()

      return {
        sub: record.user_id,
        email: record.email,
        role: record.role,
        iat: Math.floor(Date.now() / 1000),
        exp: 0,
        kind: 'api_key',
        auth_method: 'api_key',
        token_id: record.token_id,
        token_name: record.token_name,
      }
    }
  }

  return c.json({ success: false, error: 'Invalid or expired API Key' }, 401)
}

async function resolvePrincipal(c: Context<{ Bindings: Env }>): Promise<AuthPrincipal | Response> {
  const authHeader = c.req.header('Authorization')
  if (authHeader?.startsWith('Bearer ')) {
    return authenticateJwt(c, authHeader.substring(7))
  }

  const apiKey = c.req.header('X-API-Key')
  if (apiKey) {
    return authenticateApiKey(c, apiKey)
  }

  return c.json({ success: false, error: 'Unauthorized: Missing or invalid Authorization header' }, 401)
}

/**
 * 认证中间件
 */
export async function authMiddleware(c: Context<{ Bindings: Env }>, next: Next) {
  try {
    const principal = await resolvePrincipal(c)

    if (principal instanceof Response) {
      return principal
    }

    c.set('user', principal)
    await next()
  } catch (err) {
    if (err instanceof Error) {
      if (err.message.includes('expired')) {
        return c.json({ success: false, error: 'Token expired' }, 401)
      }
    }
    return c.json({ success: false, error: 'Invalid token' }, 401)
  }
}

/**
 * 基于角色的访问控制中间件
 */
export function rbacMiddleware(allowedRoles: string[]) {
  return async (c: Context<{ Bindings: Env }>, next: Next) => {
    const user = c.get('user')

    if (!user) {
      return c.json({ success: false, error: 'Unauthorized' }, 401)
    }

    if (!allowedRoles.includes(user.role)) {
      return c.json({
        success: false,
        error: 'Forbidden: Insufficient permissions',
        required_roles: allowedRoles,
        your_role: user.role,
      }, 403)
    }

    await next()
  }
}

/**
 * API Key 认证中间件 (用于代理/CLI)
 */
export async function apiKeyMiddleware(c: Context<{ Bindings: Env }>, next: Next) {
  return authMiddleware(c, next)
}
