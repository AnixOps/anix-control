/**
 * MFA API Handler
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { z } from 'zod'
import {
  setupMFA,
  enableMFA,
  disableMFA,
  verifyMFA,
  getMFAStatus,
  regenerateRecoveryCodes,
} from '../services/mfa'
import { logAudit } from '../utils/audit'
import { revokeAllUserSessions } from '../utils/token'

// Validation schemas
const setupMFASchema = z.object({})

const verifyMFASchema = z.object({
  code: z.string().length(6, 'Code must be 6 digits'),
})

const verifyMFAOrRecoverySchema = z.object({
  code: z.string().min(1, 'Code is required'),
})

const disableMFASchema = z.object({
  code: z.string().min(1, 'Code is required'),
})

/**
 * Get MFA status
 */
export async function getMFAStatusHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const status = await getMFAStatus(c.env, user.sub)

  return c.json({
    success: true,
    data: status,
  })
}

/**
 * Setup MFA (generate secret and recovery codes)
 */
export async function setupMFAHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  // Get user email
  const userInfo = await c.env.DB
    .prepare('SELECT email FROM users WHERE id = ?')
    .bind(user.sub)
    .first<{ email: string }>()

  if (!userInfo) {
    return c.json({ success: false, error: 'User not found' }, 404)
  }

  const result = await setupMFA(c.env, user.sub, userInfo.email)

  await logAudit(c, user.sub, 'setup_mfa', 'mfa', {
    status: 'initialized',
  })

  // Don't expose the secret directly, only return it once during setup
  return c.json({
    success: true,
    data: {
      secret: result.secret,
      otpauth_url: result.otpauth_url,
      recovery_codes: result.recovery_codes,
    },
  })
}

/**
 * Enable MFA (verify setup and enable)
 */
export async function enableMFAHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { code } = verifyMFASchema.parse(body)

    const result = await enableMFA(c.env, user.sub, code)

    if (!result.success) {
      return c.json({ success: false, error: result.error }, 400)
    }

    await logAudit(c, user.sub, 'enable_mfa', 'mfa', {
      status: 'enabled',
    })

    return c.json({
      success: true,
      message: 'MFA enabled successfully',
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Disable MFA
 */
export async function disableMFAHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { code } = disableMFASchema.parse(body)

    const result = await disableMFA(c.env, user.sub, code)

    if (!result.success) {
      return c.json({ success: false, error: result.error }, 400)
    }

    await logAudit(c, user.sub, 'disable_mfa', 'mfa', {
      status: 'disabled',
    })

    return c.json({
      success: true,
      message: 'MFA disabled successfully',
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Verify MFA code (for login flow)
 */
export async function verifyMFAHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { code } = verifyMFAOrRecoverySchema.parse(body)

    const result = await verifyMFA(c.env, user.sub, code)

    if (!result.success) {
      await logAudit(c, user.sub, 'mfa_verify_failed', 'mfa', {
        ip: c.req.header('CF-Connecting-IP'),
      })

      return c.json({ success: false, error: result.error }, 401)
    }

    await logAudit(c, user.sub, 'mfa_verify_success', 'mfa', {
      ip: c.req.header('CF-Connecting-IP'),
    })

    return c.json({
      success: true,
      message: 'MFA verified successfully',
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Regenerate recovery codes
 */
export async function regenerateRecoveryCodesHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { code } = verifyMFASchema.parse(body)

    const result = await regenerateRecoveryCodes(c.env, user.sub, code)

    if (!result.success) {
      return c.json({ success: false, error: result.error }, 400)
    }

    await logAudit(c, user.sub, 'regenerate_recovery_codes', 'mfa', {})

    return c.json({
      success: true,
      data: {
        recovery_codes: result.recovery_codes,
      },
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Admin: Disable MFA for a user
 */
export async function adminDisableMFAHandler(c: Context<{ Bindings: Env }>) {
  const currentUser = c.get('user')
  const userId = c.req.param('id') as string

  if (!userId) {
    return c.json({ success: false, error: 'User ID is required' }, 400)
  }

  // Check if user exists
  const user = await c.env.DB
    .prepare('SELECT id, email FROM users WHERE id = ?')
    .bind(userId)
    .first<{ id: number; email: string }>()

  if (!user) {
    return c.json({ success: false, error: 'User not found' }, 404)
  }

  // Delete MFA
  await c.env.DB
    .prepare('DELETE FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .run()

  // Revoke all sessions for the user
  await revokeAllUserSessions(c.env.KV, parseInt(userId, 10))

  await logAudit(c, currentUser.sub, 'admin_disable_mfa', 'mfa', {
    target_user_id: userId,
    target_email: user.email,
  })

  return c.json({
    success: true,
    message: 'MFA disabled for user',
  })
}