import { z } from 'zod'

/**
 * 密码复杂度验证配置
 */

export interface PasswordPolicy {
  minLength: number
  requireUppercase: boolean
  requireLowercase: boolean
  requireNumber: boolean
  requireSpecialChar: boolean
  specialChars: string
}

// 默认密码策略
export const DEFAULT_PASSWORD_POLICY: PasswordPolicy = {
  minLength: 8,
  requireUppercase: true,
  requireLowercase: true,
  requireNumber: true,
  requireSpecialChar: true,
  specialChars: '!@#$%^&*()_+=[]{}|;:\'",.<>?/~`-',
}

/**
 * 密码验证结果
 */
export interface PasswordValidationResult {
  valid: boolean
  errors: string[]
  strength: 'weak' | 'medium' | 'strong' | 'very-strong'
}

/**
 * 验证密码复杂度
 */
export function validatePassword(password: string, policy: PasswordPolicy = DEFAULT_PASSWORD_POLICY): PasswordValidationResult {
  const errors: string[] = []

  // 长度检查
  if (password.length < policy.minLength) {
    errors.push(`Password must be at least ${policy.minLength} characters long`)
  }

  // 大写字母检查
  if (policy.requireUppercase && !/[A-Z]/.test(password)) {
    errors.push('Password must contain at least one uppercase letter')
  }

  // 小写字母检查
  if (policy.requireLowercase && !/[a-z]/.test(password)) {
    errors.push('Password must contain at least one lowercase letter')
  }

  // 数字检查
  if (policy.requireNumber && !/[0-9]/.test(password)) {
    errors.push('Password must contain at least one number')
  }

  // 特殊字符检查
  if (policy.requireSpecialChar) {
    // Escape special regex chars and put - at the end to avoid range interpretation
    const escaped = policy.specialChars.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/-/g, '\\-')
    const specialCharRegex = new RegExp(`[${escaped}]`)
    if (!specialCharRegex.test(password)) {
      errors.push(`Password must contain at least one special character (${policy.specialChars.slice(0, 10)}...)`)
    }
  }

  // 常见弱密码检查 (简化版，只检查完全匹配或非常弱的模式)
  const commonPasswords = [
    'password', '12345678', 'qwerty', 'abc123',
    'letmein', 'welcome', 'admin', 'password123',
  ]
  const lowerPassword = password.toLowerCase()
  if (commonPasswords.some(weak => lowerPassword === weak || lowerPassword === weak + '!')) {
    errors.push('Password is too common or easily guessable')
  }

  // 计算密码强度
  let strengthScore = 0
  if (password.length >= 8) strengthScore++
  if (password.length >= 12) strengthScore++
  if (/[A-Z]/.test(password)) strengthScore++
  if (/[a-z]/.test(password)) strengthScore++
  if (/[0-9]/.test(password)) strengthScore++
  if (/[!@#$%^&*()_+\-=\[\]{}|;:'",.<>?\/~`]/.test(password)) strengthScore++
  if (!commonPasswords.some(weak => lowerPassword === weak || lowerPassword === weak + '!')) strengthScore++

  let strength: PasswordValidationResult['strength']
  if (strengthScore <= 3) strength = 'weak'
  else if (strengthScore <= 5) strength = 'medium'
  else if (strengthScore <= 6) strength = 'strong'
  else strength = 'very-strong'

  return {
    valid: errors.length === 0,
    errors,
    strength,
  }
}

/**
 * Zod schema for password validation
 */
export const passwordSchema = z.string()
  .min(8, 'Password must be at least 8 characters long')
  .refine(
    (val) => /[A-Z]/.test(val),
    'Password must contain at least one uppercase letter'
  )
  .refine(
    (val) => /[a-z]/.test(val),
    'Password must contain at least one lowercase letter'
  )
  .refine(
    (val) => /[0-9]/.test(val),
    'Password must contain at least one number'
  )
  .refine(
    (val) => /[!@#$%^&*()_+\-=\[\]{}|;:'",.<>?\/~`]/.test(val),
    'Password must contain at least one special character'
  )

/**
 * 增强的注册 Schema
 */
export const enhancedRegisterSchema = z.object({
  email: z.string().email('Invalid email address'),
  password: passwordSchema,
  confirmPassword: z.string(),
  role: z.enum(['admin', 'operator', 'viewer']).optional(),
}).refine(
  (data) => data.password === data.confirmPassword,
  {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  }
)

/**
 * 密码修改 Schema
 */
export const changePasswordSchema = z.object({
  currentPassword: z.string().min(1, 'Current password is required'),
  newPassword: passwordSchema,
  confirmPassword: z.string(),
}).refine(
  (data) => data.newPassword === data.confirmPassword,
  {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  }
)

/**
 * 密码重置请求 Schema
 */
export const passwordResetRequestSchema = z.object({
  email: z.string().email('Invalid email address'),
})

/**
 * 密码重置 Schema
 */
export const passwordResetSchema = z.object({
  token: z.string().min(1, 'Reset token is required'),
  newPassword: passwordSchema,
  confirmPassword: z.string(),
}).refine(
  (data) => data.newPassword === data.confirmPassword,
  {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  }
)