/**
 * Password Validation Unit Tests
 */

import { describe, it, expect } from 'vitest'
import {
  validatePassword,
  passwordSchema,
  DEFAULT_PASSWORD_POLICY,
} from './password'

describe('Password Validation', () => {
  describe('validatePassword', () => {
    it('should reject password shorter than 8 characters', () => {
      const result = validatePassword('Sh1!')
      expect(result.valid).toBe(false)
      expect(result.errors).toContain('Password must be at least 8 characters long')
    })

    it('should reject password without uppercase letter', () => {
      const result = validatePassword('lowercase1!')
      expect(result.valid).toBe(false)
      expect(result.errors).toContain('Password must contain at least one uppercase letter')
    })

    it('should reject password without lowercase letter', () => {
      const result = validatePassword('UPPERCASE1!')
      expect(result.valid).toBe(false)
      expect(result.errors).toContain('Password must contain at least one lowercase letter')
    })

    it('should reject password without number', () => {
      const result = validatePassword('NoNumbers!')
      expect(result.valid).toBe(false)
      expect(result.errors).toContain('Password must contain at least one number')
    })

    it('should reject password without special character', () => {
      const result = validatePassword('NoSpecial1')
      expect(result.valid).toBe(false)
      expect(result.errors.some((e: string) => e.includes('special character'))).toBe(true)
    })

    it('should reject common passwords', () => {
      // Test with exact match from common passwords list
      const result = validatePassword('Password123!')
      expect(result.valid).toBe(false)
      expect(result.errors).toContain('Password is too common or easily guessable')
    })

    it('should accept valid password', () => {
      const result = validatePassword('ValidP@ss123')
      expect(result.valid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('should accept strong password', () => {
      const result = validatePassword('Str0ng!P@ssw0rd#2024')
      expect(result.valid).toBe(true)
      expect(result.strength).toBe('very-strong')
    })

    it('should calculate strength correctly for weak password', () => {
      // Short password without all requirements
      const result = validatePassword('abc')
      expect(result.strength).toBe('weak')
    })

    it('should calculate strength correctly for medium password', () => {
      // 8 chars, has upper, lower, number, no special
      const result = validatePassword('TestPass1')
      expect(['weak', 'medium']).toContain(result.strength)
    })

    it('should calculate strength correctly for strong password', () => {
      const result = validatePassword('Str0ngP@ss!')
      expect(['strong', 'very-strong']).toContain(result.strength)
    })

    it('should use custom policy', () => {
      const customPolicy = {
        ...DEFAULT_PASSWORD_POLICY,
        minLength: 12,
        requireSpecialChar: false,
      }

      // Use a password without common words
      const result = validatePassword('MyCustomTestPass12', customPolicy)
      expect(result.valid).toBe(true)
    })
  })

  describe('passwordSchema (Zod)', () => {
    it('should validate correct password', () => {
      const result = passwordSchema.safeParse('ValidP@ss123')
      expect(result.success).toBe(true)
    })

    it('should reject short password', () => {
      const result = passwordSchema.safeParse('Sh1!')
      expect(result.success).toBe(false)
    })

    it('should reject password without uppercase', () => {
      const result = passwordSchema.safeParse('lowercase1!')
      expect(result.success).toBe(false)
    })

    it('should reject password without lowercase', () => {
      const result = passwordSchema.safeParse('UPPERCASE1!')
      expect(result.success).toBe(false)
    })

    it('should reject password without number', () => {
      const result = passwordSchema.safeParse('NoNumbers!')
      expect(result.success).toBe(false)
    })

    it('should reject password without special char', () => {
      const result = passwordSchema.safeParse('NoSpecial1')
      expect(result.success).toBe(false)
    })
  })
})