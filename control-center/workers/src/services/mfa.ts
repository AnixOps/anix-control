/**
 * MFA (Multi-Factor Authentication) Service
 * Implements TOTP (Time-based One-Time Password) authentication
 */

import type { Env } from '../types'

// TOTP Configuration
const TOTP_CONFIG = {
  digits: 6,
  period: 30, // seconds
  window: 1, // allow 1 period before/after
  algorithm: 'SHA-1' as const,
}

// Recovery code configuration
const RECOVERY_CODE_COUNT = 8
const RECOVERY_CODE_LENGTH = 8

/**
 * Generate a random secret for TOTP
 */
export function generateSecret(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567' // Base32 characters
  let secret = ''
  for (let i = 0; i < 20; i++) {
    secret += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return secret
}

/**
 * Base32 decode
 */
function base32Decode(str: string): Uint8Array {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  str = str.toUpperCase().replace(/[^A-Z2-7]/g, '')

  const bytes: number[] = []
  let buffer = 0
  let bits = 0

  for (const char of str) {
    const index = alphabet.indexOf(char)
    if (index === -1) continue

    buffer = (buffer << 5) | index
    bits += 5

    while (bits >= 8) {
      bytes.push((buffer >> (bits - 8)) & 0xff)
      bits -= 8
    }
  }

  return new Uint8Array(bytes)
}

/**
 * Convert number to bytes
 */
function numberToBytes(num: number): Uint8Array {
  const bytes = new Uint8Array(8)
  for (let i = 7; i >= 0; i--) {
    bytes[i] = num & 0xff
    num = Math.floor(num / 256)
  }
  return bytes
}

/**
 * Calculate HMAC-SHA1 (simplified implementation for Workers)
 */
async function hmacSha1(key: Uint8Array, message: Uint8Array): Promise<Uint8Array> {
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    key,
    { name: 'HMAC', hash: 'SHA-1' },
    false,
    ['sign']
  )

  const signature = await crypto.subtle.sign('HMAC', cryptoKey, message)
  return new Uint8Array(signature)
}

/**
 * Generate TOTP code
 */
export async function generateTOTP(secret: string, time?: number): Promise<string> {
  const timestamp = Math.floor((time || Date.now()) / 1000 / TOTP_CONFIG.period)
  const timeBytes = numberToBytes(timestamp)
  const secretBytes = base32Decode(secret)

  const hmac = await hmacSha1(secretBytes, timeBytes)

  // Dynamic truncation
  const offset = hmac[hmac.length - 1] & 0x0f
  const code = (
    ((hmac[offset] & 0x7f) << 24) |
    ((hmac[offset + 1] & 0xff) << 16) |
    ((hmac[offset + 2] & 0xff) << 8) |
    (hmac[offset + 3] & 0xff)
  ) % Math.pow(10, TOTP_CONFIG.digits)

  return code.toString().padStart(TOTP_CONFIG.digits, '0')
}

/**
 * Verify TOTP code
 */
export async function verifyTOTP(
  secret: string,
  code: string,
  window: number = TOTP_CONFIG.window
): Promise<boolean> {
  const now = Date.now()

  // Check current and adjacent periods
  for (let i = -window; i <= window; i++) {
    const time = now + i * TOTP_CONFIG.period * 1000
    const expectedCode = await generateTOTP(secret, time)

    // Use constant-time comparison
    if (constantTimeCompare(code, expectedCode)) {
      return true
    }
  }

  return false
}

/**
 * Constant-time string comparison
 */
function constantTimeCompare(a: string, b: string): boolean {
  if (a.length !== b.length) return false

  let result = 0
  for (let i = 0; i < a.length; i++) {
    result |= a.charCodeAt(i) ^ b.charCodeAt(i)
  }

  return result === 0
}

/**
 * Generate recovery codes
 */
export function generateRecoveryCodes(): string[] {
  const chars = '0123456789ABCDEFGHJKLMNPQRSTUVWXYZ' // Exclude O and I to avoid confusion
  const codes: string[] = []

  for (let i = 0; i < RECOVERY_CODE_COUNT; i++) {
    let code = ''
    for (let j = 0; j < RECOVERY_CODE_LENGTH; j++) {
      code += chars.charAt(Math.floor(Math.random() * chars.length))
      if (j === 3) code += '-' // Add separator
    }
    codes.push(code)
  }

  return codes
}

/**
 * Hash recovery code for storage
 */
export async function hashRecoveryCode(code: string): Promise<string> {
  const encoder = new TextEncoder()
  const data = encoder.encode(code.toUpperCase())

  const hashBuffer = await crypto.subtle.digest('SHA-256', data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))

  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
}

/**
 * Generate OTP Auth URL for QR code
 */
export function generateOTPAuthURL(
  secret: string,
  email: string,
  issuer: string = 'AnixOps'
): string {
  const encodedIssuer = encodeURIComponent(issuer)
  const encodedEmail = encodeURIComponent(email)

  return `otpauth://totp/${encodedIssuer}:${encodedEmail}?secret=${secret}&issuer=${encodedIssuer}&algorithm=SHA1&digits=${TOTP_CONFIG.digits}&period=${TOTP_CONFIG.period}`
}

/**
 * MFA Status for a user
 */
export interface MFAStatus {
  enabled: boolean
  verified: boolean
  secret?: string
  recovery_codes_remaining: number
  created_at?: string
  last_used_at?: string
}

/**
 * Get MFA status for a user
 */
export async function getMFAStatus(env: Env, userId: number): Promise<MFAStatus> {
  const mfaData = await env.DB
    .prepare('SELECT * FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first<{
      user_id: number
      secret: string
      verified: number
      recovery_codes: string
      created_at: string
      last_used_at: string | null
    }>()

  if (!mfaData) {
    return {
      enabled: false,
      verified: false,
      recovery_codes_remaining: 0,
    }
  }

  const recoveryCodes = JSON.parse(mfaData.recovery_codes || '[]') as string[]

  return {
    enabled: mfaData.verified === 1,
    verified: mfaData.verified === 1,
    recovery_codes_remaining: recoveryCodes.length,
    created_at: mfaData.created_at,
    last_used_at: mfaData.last_used_at || undefined,
  }
}

/**
 * Setup MFA for a user (generate secret and recovery codes)
 */
export async function setupMFA(
  env: Env,
  userId: number,
  email: string
): Promise<{
  secret: string
  otpauth_url: string
  recovery_codes: string[]
}> {
  const secret = generateSecret()
  const recoveryCodes = generateRecoveryCodes()

  // Hash recovery codes for storage
  const hashedCodes = await Promise.all(recoveryCodes.map(c => hashRecoveryCode(c)))

  // Check if MFA already exists
  const existing = await env.DB
    .prepare('SELECT user_id FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first()

  if (existing) {
    // Update existing MFA setup
    await env.DB
      .prepare(`
        UPDATE user_mfa SET
          secret = ?,
          recovery_codes = ?,
          verified = 0,
          updated_at = datetime('now')
        WHERE user_id = ?
      `)
      .bind(secret, JSON.stringify(hashedCodes), userId)
      .run()
  } else {
    // Create new MFA setup
    await env.DB
      .prepare(`
        INSERT INTO user_mfa (user_id, secret, recovery_codes, verified, created_at)
        VALUES (?, ?, ?, 0, datetime('now'))
      `)
      .bind(userId, secret, JSON.stringify(hashedCodes))
      .run()
  }

  return {
    secret,
    otpauth_url: generateOTPAuthURL(secret, email),
    recovery_codes: recoveryCodes,
  }
}

/**
 * Verify and enable MFA
 */
export async function enableMFA(
  env: Env,
  userId: number,
  code: string
): Promise<{ success: boolean; error?: string }> {
  const mfaData = await env.DB
    .prepare('SELECT secret, verified FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first<{ secret: string; verified: number }>()

  if (!mfaData) {
    return { success: false, error: 'MFA not set up' }
  }

  if (mfaData.verified === 1) {
    return { success: false, error: 'MFA already enabled' }
  }

  const valid = await verifyTOTP(mfaData.secret, code)
  if (!valid) {
    return { success: false, error: 'Invalid verification code' }
  }

  // Enable MFA
  await env.DB
    .prepare(`
      UPDATE user_mfa SET
        verified = 1,
        updated_at = datetime('now')
      WHERE user_id = ?
    `)
    .bind(userId)
    .run()

  return { success: true }
}

/**
 * Disable MFA for a user
 */
export async function disableMFA(
  env: Env,
  userId: number,
  code: string
): Promise<{ success: boolean; error?: string }> {
  const mfaData = await env.DB
    .prepare('SELECT secret FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first<{ secret: string }>()

  if (!mfaData) {
    return { success: false, error: 'MFA not enabled' }
  }

  const valid = await verifyTOTP(mfaData.secret, code)
  if (!valid) {
    return { success: false, error: 'Invalid verification code' }
  }

  // Delete MFA
  await env.DB
    .prepare('DELETE FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .run()

  return { success: true }
}

/**
 * Verify MFA code (TOTP or recovery code)
 */
export async function verifyMFA(
  env: Env,
  userId: number,
  code: string
): Promise<{ success: boolean; error?: string }> {
  const mfaData = await env.DB
    .prepare('SELECT secret, recovery_codes FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first<{ secret: string; recovery_codes: string }>()

  if (!mfaData) {
    return { success: false, error: 'MFA not enabled' }
  }

  // Try TOTP first
  const totpValid = await verifyTOTP(mfaData.secret, code)
  if (totpValid) {
    // Update last used
    await env.DB
      .prepare('UPDATE user_mfa SET last_used_at = datetime(\'now\') WHERE user_id = ?')
      .bind(userId)
      .run()

    return { success: true }
  }

  // Try recovery code
  const hashedCodes = JSON.parse(mfaData.recovery_codes) as string[]
  const inputHash = await hashRecoveryCode(code)

  const codeIndex = hashedCodes.indexOf(inputHash)
  if (codeIndex !== -1) {
    // Remove used recovery code
    hashedCodes.splice(codeIndex, 1)

    await env.DB
      .prepare('UPDATE user_mfa SET recovery_codes = ?, last_used_at = datetime(\'now\') WHERE user_id = ?')
      .bind(JSON.stringify(hashedCodes), userId)
      .run()

    return { success: true }
  }

  return { success: false, error: 'Invalid MFA code' }
}

/**
 * Regenerate recovery codes
 */
export async function regenerateRecoveryCodes(
  env: Env,
  userId: number,
  code: string
): Promise<{ success: boolean; recovery_codes?: string[]; error?: string }> {
  const mfaData = await env.DB
    .prepare('SELECT secret FROM user_mfa WHERE user_id = ?')
    .bind(userId)
    .first<{ secret: string }>()

  if (!mfaData) {
    return { success: false, error: 'MFA not enabled' }
  }

  const valid = await verifyTOTP(mfaData.secret, code)
  if (!valid) {
    return { success: false, error: 'Invalid verification code' }
  }

  const recoveryCodes = generateRecoveryCodes()
  const hashedCodes = await Promise.all(recoveryCodes.map(c => hashRecoveryCode(c)))

  await env.DB
    .prepare('UPDATE user_mfa SET recovery_codes = ?, updated_at = datetime(\'now\') WHERE user_id = ?')
    .bind(JSON.stringify(hashedCodes), userId)
    .run()

  return {
    success: true,
    recovery_codes: recoveryCodes,
  }
}