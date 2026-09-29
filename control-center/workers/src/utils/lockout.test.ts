/**
 * Account Lockout Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
import {
  checkLockout,
  recordFailedAttempt,
  clearFailedAttempts,
  unlockAccount,
  getLockoutInfo,
  DEFAULT_LOCKOUT_CONFIG,
} from './lockout'
import { createMockKV } from '../../test/setup'

describe('Account Lockout', () => {
  let mockKV: KVNamespace

  beforeEach(() => {
    mockKV = createMockKV()
  })

  describe('checkLockout', () => {
    it('should return unlocked status for new identifier', async () => {
      const status = await checkLockout(mockKV, 'test@example.com')
      expect(status.locked).toBe(false)
      expect(status.attempts).toBe(0)
      expect(status.remainingAttempts).toBe(DEFAULT_LOCKOUT_CONFIG.maxAttempts)
    })

    it('should return locked status when account is locked', async () => {
      // First, lock the account
      for (let i = 0; i < DEFAULT_LOCKOUT_CONFIG.maxAttempts; i++) {
        await recordFailedAttempt(mockKV, 'locked@example.com')
      }

      const status = await checkLockout(mockKV, 'locked@example.com')
      expect(status.locked).toBe(true)
      expect(status.remainingAttempts).toBe(0)
    })
  })

  describe('recordFailedAttempt', () => {
    it('should increment attempt count', async () => {
      const status1 = await recordFailedAttempt(mockKV, 'test1@example.com')
      expect(status1.attempts).toBe(1)
      expect(status1.remainingAttempts).toBe(DEFAULT_LOCKOUT_CONFIG.maxAttempts - 1)

      const status2 = await recordFailedAttempt(mockKV, 'test1@example.com')
      expect(status2.attempts).toBe(2)
    })

    it('should lock account after max attempts', async () => {
      let status
      for (let i = 0; i < DEFAULT_LOCKOUT_CONFIG.maxAttempts; i++) {
        status = await recordFailedAttempt(mockKV, 'test2@example.com')
      }

      expect(status!.locked).toBe(true)
      expect(status!.reason).toBe('Too many failed login attempts')
    })

    it('should return remaining attempts', async () => {
      await recordFailedAttempt(mockKV, 'test3@example.com')
      const status = await recordFailedAttempt(mockKV, 'test3@example.com')

      expect(status.remainingAttempts).toBe(DEFAULT_LOCKOUT_CONFIG.maxAttempts - 2)
    })
  })

  describe('clearFailedAttempts', () => {
    it('should clear attempt count', async () => {
      await recordFailedAttempt(mockKV, 'test4@example.com')
      await recordFailedAttempt(mockKV, 'test4@example.com')

      await clearFailedAttempts(mockKV, 'test4@example.com')

      const status = await checkLockout(mockKV, 'test4@example.com')
      expect(status.attempts).toBe(0)
    })
  })

  describe('unlockAccount', () => {
    it('should unlock locked account', async () => {
      // Lock the account
      for (let i = 0; i < DEFAULT_LOCKOUT_CONFIG.maxAttempts; i++) {
        await recordFailedAttempt(mockKV, 'test5@example.com')
      }

      // Verify locked
      let status = await checkLockout(mockKV, 'test5@example.com')
      expect(status.locked).toBe(true)

      // Unlock
      await unlockAccount(mockKV, 'test5@example.com')

      // Verify unlocked
      status = await checkLockout(mockKV, 'test5@example.com')
      expect(status.locked).toBe(false)
      expect(status.attempts).toBe(0)
    })
  })

  describe('getLockoutInfo', () => {
    it('should return null for non-locked account', async () => {
      const info = await getLockoutInfo(mockKV, 'notlocked@example.com')
      expect(info).toBeNull()
    })

    it('should return lockout info for locked account', async () => {
      // Lock the account
      for (let i = 0; i < DEFAULT_LOCKOUT_CONFIG.maxAttempts; i++) {
        await recordFailedAttempt(mockKV, 'locked2@example.com')
      }

      const info = await getLockoutInfo(mockKV, 'locked2@example.com')
      expect(info).not.toBeNull()
      expect(info!.isLocked).toBe(true)
      expect(info!.reason).toBe('Too many failed login attempts')
    })
  })

  describe('Custom Lockout Config', () => {
    it('should use custom max attempts', async () => {
      const customConfig = {
        ...DEFAULT_LOCKOUT_CONFIG,
        maxAttempts: 3,
      }

      let status
      for (let i = 0; i < 3; i++) {
        status = await recordFailedAttempt(mockKV, 'custom@example.com', customConfig)
      }

      expect(status!.locked).toBe(true)
    })
  })
})