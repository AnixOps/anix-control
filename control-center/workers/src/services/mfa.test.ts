/**
 * MFA Service Unit Tests
 */

import { describe, it, expect } from 'vitest'
import {
  generateSecret,
  generateTOTP,
  verifyTOTP,
  generateRecoveryCodes,
  hashRecoveryCode,
  generateOTPAuthURL,
} from './mfa'

describe('MFA Service', () => {
  describe('generateSecret', () => {
    it('should generate a 20-character Base32 secret', () => {
      const secret = generateSecret()
      expect(secret).toHaveLength(20)
      expect(/^[A-Z2-7]+$/.test(secret)).toBe(true)
    })

    it('should generate unique secrets', () => {
      const secret1 = generateSecret()
      const secret2 = generateSecret()
      expect(secret1).not.toBe(secret2)
    })
  })

  describe('generateTOTP', () => {
    it('should generate a 6-digit code', async () => {
      const secret = generateSecret()
      const code = await generateTOTP(secret)
      expect(code).toHaveLength(6)
      expect(/^\d{6}$/.test(code)).toBe(true)
    })

    it('should generate consistent codes for same time', async () => {
      const secret = generateSecret()
      const time = Date.now()
      const code1 = await generateTOTP(secret, time)
      const code2 = await generateTOTP(secret, time)
      expect(code1).toBe(code2)
    })
  })

  describe('verifyTOTP', () => {
    it('should verify a valid TOTP code', async () => {
      const secret = generateSecret()
      const code = await generateTOTP(secret)
      const valid = await verifyTOTP(secret, code)
      expect(valid).toBe(true)
    })

    it('should reject an invalid TOTP code', async () => {
      const secret = generateSecret()
      const valid = await verifyTOTP(secret, '000000')
      expect(valid).toBe(false)
    })

    it('should accept codes within window', async () => {
      const secret = generateSecret()
      const time = Date.now() - 30000 // 1 period ago
      const code = await generateTOTP(secret, time)
      const valid = await verifyTOTP(secret, code, 1)
      expect(valid).toBe(true)
    })
  })

  describe('generateRecoveryCodes', () => {
    it('should generate 8 recovery codes', () => {
      const codes = generateRecoveryCodes()
      expect(codes).toHaveLength(8)
    })

    it('should generate codes with correct format', () => {
      const codes = generateRecoveryCodes()
      for (const code of codes) {
        // Format: XXXX-XXXX (9 characters)
        expect(code).toHaveLength(9)
        expect(code.charAt(4)).toBe('-')
      }
    })

    it('should generate unique codes', () => {
      const codes = generateRecoveryCodes()
      const unique = new Set(codes)
      expect(unique.size).toBe(8)
    })
  })

  describe('hashRecoveryCode', () => {
    it('should generate consistent hash for same code', async () => {
      const hash1 = await hashRecoveryCode('ABCD-1234')
      const hash2 = await hashRecoveryCode('ABCD-1234')
      expect(hash1).toBe(hash2)
    })

    it('should generate different hashes for different codes', async () => {
      const hash1 = await hashRecoveryCode('ABCD-1234')
      const hash2 = await hashRecoveryCode('EFGH-5678')
      expect(hash1).not.toBe(hash2)
    })

    it('should normalize case', async () => {
      const hash1 = await hashRecoveryCode('abcd-1234')
      const hash2 = await hashRecoveryCode('ABCD-1234')
      expect(hash1).toBe(hash2)
    })
  })

  describe('generateOTPAuthURL', () => {
    it('should generate valid otpauth URL', () => {
      const secret = 'JBSWY3DPEHPK3PXP'
      const email = 'test@example.com'
      const url = generateOTPAuthURL(secret, email)

      expect(url).toContain('otpauth://totp/')
      expect(url).toContain('secret=' + secret)
      expect(url).toContain('issuer=AnixOps')
      expect(url).toContain('digits=6')
      expect(url).toContain('period=30')
    })

    it('should encode special characters', () => {
      const secret = 'JBSWY3DPEHPK3PXP'
      const email = 'test+user@example.com'
      const url = generateOTPAuthURL(secret, email)

      expect(url).toContain('test%2Buser%40example.com')
    })
  })
})