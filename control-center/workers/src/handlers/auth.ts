import type { Context } from 'hono'
import { SignJWT, jwtVerify } from 'jose'
import { hash, compare } from 'bcryptjs'
import { z } from 'zod'
import type { ApiErrorResponse, AuthInvalidCredentialsResponse, AuthLockoutResponse, AuthLoginResponse, AuthLogoutResponse, AuthMeResponse, AuthPasswordValidationErrorResponse, AuthRefreshResponse, AuthRegisterResponse, AuthSchemaValidationErrorResponse, Env, User } from '../types'
import { logAudit } from '../utils/audit'
import { passwordSchema, validatePassword } from '../utils/password'
import { checkLockout, recordFailedAttempt, clearFailedAttempts } from '../utils/lockout'

// 验证 schema
const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
})

const registerSchema = z.object({
  email: z.string().email(),
  password: passwordSchema,
  role: z.enum(['admin', 'operator', 'viewer']).optional(),
})

/**
 * 生成 JWT Token
 */
async function generateToken(
  payload: { sub: number; email: string; role: string },
  secret: Uint8Array,
  expiresIn: string
): Promise<string> {
  // Convert sub to string for JWT spec compliance
  const jwtPayload = { ...payload, sub: String(payload.sub) }
  return await new SignJWT(jwtPayload)
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(expiresIn)
    .sign(secret)
}

/**
 * 登录
 */
export async function loginHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const { email, password } = loginSchema.parse(body)

    // 检查账户锁定状态
    const lockoutStatus = await checkLockout(c.env.KV, email)
    if (lockoutStatus.locked) {
      await logAudit(c, undefined, 'login_attempt_locked', 'auth', {
        email,
        ip: c.req.header('CF-Connecting-IP'),
        locked_until: lockoutStatus.lockedUntil,
      })

      return c.json({
        success: false,
        error: 'Account temporarily locked due to too many failed attempts',
        locked_until: lockoutStatus.lockedUntil,
        retry_after: lockoutStatus.lockedUntil
          ? Math.ceil((new Date(lockoutStatus.lockedUntil).getTime() - Date.now()) / 1000)
          : undefined,
      } as AuthLockoutResponse, 423)
    }

    // 查找用户
    const user = await c.env.DB
      .prepare('SELECT * FROM users WHERE email = ? AND enabled = 1')
      .bind(email)
      .first<User>()

    if (!user || !user.password_hash) {
      // 记录失败尝试
      const newStatus = await recordFailedAttempt(c.env.KV, email)

      await logAudit(c, undefined, 'login_failed', 'auth', {
        email,
        ip: c.req.header('CF-Connecting-IP'),
        reason: 'user_not_found',
        attempts: newStatus.attempts,
      })

      return c.json({
        success: false,
        error: 'Invalid credentials',
        remaining_attempts: newStatus.remainingAttempts,
        account_locked: newStatus.locked,
      } as AuthInvalidCredentialsResponse, 401)
    }

    // 验证密码
    const valid = await compare(password, user.password_hash)
    if (!valid) {
      // 记录失败尝试
      const newStatus = await recordFailedAttempt(c.env.KV, email)

      await logAudit(c, user.id, 'login_failed', 'auth', {
        email,
        ip: c.req.header('CF-Connecting-IP'),
        reason: 'invalid_password',
        attempts: newStatus.attempts,
      })

      return c.json({
        success: false,
        error: 'Invalid credentials',
        remaining_attempts: newStatus.remainingAttempts,
        account_locked: newStatus.locked,
      } as AuthInvalidCredentialsResponse, 401)
    }

    // 登录成功，清除失败尝试记录
    await clearFailedAttempts(c.env.KV, email)

    // 生成 JWT
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

    // 更新最后登录时间
    await c.env.DB
      .prepare('UPDATE users SET last_login_at = datetime(\'now\') WHERE id = ?')
      .bind(user.id)
      .run()

    // 记录审计日志
    await logAudit(c, user.id, 'login', 'auth', { ip: c.req.header('CF-Connecting-IP') })

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
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as AuthSchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

/**
 * 注册
 */
export async function registerHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const { email, password, role } = registerSchema.parse(body)

    // 额外的密码复杂度验证
    const passwordValidation = validatePassword(password)
    if (!passwordValidation.valid) {
      return c.json({
        success: false,
        error: 'Password does not meet complexity requirements',
        details: passwordValidation.errors,
        strength: passwordValidation.strength,
      } as AuthPasswordValidationErrorResponse, 400)
    }

    // 检查用户是否已存在
    const existing = await c.env.DB
      .prepare('SELECT id FROM users WHERE email = ?')
      .bind(email)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Email already registered' } as ApiErrorResponse, 409)
    }

    // 哈希密码
    const passwordHash = await hash(password, 12)

    // 创建用户
    const result = await c.env.DB
      .prepare(`
        INSERT INTO users (email, password_hash, role, auth_provider, enabled)
        VALUES (?, ?, ?, 'local', 1)
        RETURNING id, email, role, created_at
      `)
      .bind(email, passwordHash, role || 'viewer')
      .first<{ id: number; email: string; role: string; created_at: string }>()

    // 记录审计日志
    await logAudit(c, result?.id, 'register', 'user', { email })

    return c.json({ success: true, data: result } as AuthRegisterResponse, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as AuthSchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

/**
 * 刷新 Token
 */
export async function refreshHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ refresh_token?: string }>()

  if (!body.refresh_token) {
    return c.json({ success: false, error: 'Missing refresh token' } as ApiErrorResponse, 400)
  }

  try {
    const secret = new TextEncoder().encode(c.env.JWT_SECRET)
    const { payload } = await jwtVerify(body.refresh_token, secret)

    // 获取用户信息
    const user = await c.env.DB
      .prepare('SELECT id, email, role FROM users WHERE id = ? AND enabled = 1')
      .bind(payload.sub)
      .first<User>()

    if (!user) {
      return c.json({ success: false, error: 'User not found' } as ApiErrorResponse, 404)
    }

    // 生成新的 access token
    const expire = parseInt(c.env.JWT_EXPIRE, 10) || 86400
    const accessToken = await generateToken(
      { sub: user.id, email: user.email, role: user.role },
      secret,
      `${expire}s`
    )

    return c.json({
      success: true,
      data: {
        access_token: accessToken,
        token_type: 'Bearer',
        expires_in: expire,
      },
    } as AuthRefreshResponse)
  } catch {
    return c.json({ success: false, error: 'Invalid refresh token' } as ApiErrorResponse, 401)
  }
}

/**
 * 登出
 * 将 Token 加入黑名单，使其失效
 */
export async function logoutHandler(c: Context<{ Bindings: Env }>) {
  const authHeader = c.req.header('Authorization')

  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return c.json({ success: false, error: 'No token provided' } as ApiErrorResponse, 400)
  }

  const token = authHeader.substring(7)

  try {
    // 解析 Token 获取过期时间
    const secret = new TextEncoder().encode(c.env.JWT_SECRET)
    const { payload } = await jwtVerify(token, secret)

    // 计算剩余有效时间
    const now = Math.floor(Date.now() / 1000)
    const ttl = (payload.exp || now) - now

    // 将 Token 加入黑名单
    // 使用 Token 的剩余有效期作为 KV 的 TTL
    if (ttl > 0) {
      await c.env.KV.put(`blacklist:${token}`, JSON.stringify({
        user_id: payload.sub,
        revoked_at: new Date().toISOString(),
      }), {
        expirationTtl: ttl,
      })
    }

    // 记录审计日志
    const user = c.get('user')
    if (user) {
      await logAudit(c, user.sub, 'logout', 'auth', { ip: c.req.header('CF-Connecting-IP') })
    }

    return c.json({
      success: true,
      message: 'Logged out successfully',
    } as AuthLogoutResponse)
  } catch (err) {
    // Token 已过期或无效，仍然返回成功
    return c.json({
      success: true,
      message: 'Logged out successfully',
    } as AuthLogoutResponse)
  }
}

/**
 * 获取当前用户信息
 */
export async function meHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const userInfo = await c.env.DB
    .prepare('SELECT id, email, role, auth_provider, last_login_at, created_at FROM users WHERE id = ?')
    .bind(user.sub)
    .first<User>()

  if (!userInfo) {
    return c.json({ success: false, error: 'User not found' } as ApiErrorResponse, 404)
  }

  return c.json({
    success: true,
    data: userInfo,
  } as AuthMeResponse)
}

/**
 * 记录审计日志 - 已移至 utils/audit.ts
 */