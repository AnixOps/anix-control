/**
 * Token Blacklist Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
import {
  revokeToken,
  isTokenRevoked,
  revokeAllUserSessions,
  isUserSessionRevoked,
  getBlacklistEntry,
} from './token'
import { createMockKV } from '../../test/setup'

describe('Token Blacklist', () => {
  let mockKV: KVNamespace

  beforeEach(() => {
    mockKV = createMockKV()
  })

  describe('revokeToken', () => {
    it('should add token to blacklist', async () => {
      await revokeToken(mockKV, 'test-token-1', 3600, 1, 'logout')

      const revoked = await isTokenRevoked(mockKV, 'test-token-1')
      expect(revoked).toBe(true)
    })

    it('should not revoke token with ttl <= 0', async () => {
      await revokeToken(mockKV, 'test-token-2', 0, 1, 'test')

      const revoked = await isTokenRevoked(mockKV, 'test-token-2')
      expect(revoked).toBe(false)
    })

    it('should store token metadata', async () => {
      await revokeToken(mockKV, 'test-token-3', 3600, 1, 'logout')

      const entry = await getBlacklistEntry(mockKV, 'test-token-3')
      expect(entry).not.toBeNull()
      expect(entry!.user_id).toBe(1)
      expect(entry!.reason).toBe('logout')
    })
  })

  describe('isTokenRevoked', () => {
    it('should return false for non-revoked token', async () => {
      const revoked = await isTokenRevoked(mockKV, 'non-existent-token')
      expect(revoked).toBe(false)
    })

    it('should return true for revoked token', async () => {
      await revokeToken(mockKV, 'revoked-token', 3600, 1, 'test')

      const revoked = await isTokenRevoked(mockKV, 'revoked-token')
      expect(revoked).toBe(true)
    })
  })

  describe('revokeAllUserSessions', () => {
    it('should store user revocation timestamp', async () => {
      await revokeAllUserSessions(mockKV, 1)

      // Check that session is revoked for tokens issued before
      const now = Math.floor(Date.now() / 1000)
      const oldTimestamp = now - 3600 // 1 hour ago
      const revoked = await isUserSessionRevoked(mockKV, 1, oldTimestamp)
      expect(revoked).toBe(true)
    })

    it('should not affect tokens issued after revocation', async () => {
      // Record revocation time
      const beforeRevoke = Math.floor(Date.now() / 1000) + 1000 // 1 second in future
      await revokeAllUserSessions(mockKV, 2)

      // Token issued "before" revocation should still be valid
      // (since we're testing with future timestamp)
      const revoked = await isUserSessionRevoked(mockKV, 2, beforeRevoke)
      expect(revoked).toBe(false)
    })
  })

  describe('isUserSessionRevoked', () => {
    it('should return false for user without revocation', async () => {
      const revoked = await isUserSessionRevoked(mockKV, 999, Date.now())
      expect(revoked).toBe(false)
    })

    it('should return true for token issued before revocation', async () => {
      const userId = 3
      const now = Math.floor(Date.now() / 1000)
      const oldTimestamp = now - 3600 // 1 hour ago

      await revokeAllUserSessions(mockKV, userId)

      const revoked = await isUserSessionRevoked(mockKV, userId, oldTimestamp)
      expect(revoked).toBe(true)
    })
  })

  describe('getBlacklistEntry', () => {
    it('should return null for non-existent entry', async () => {
      const entry = await getBlacklistEntry(mockKV, 'non-existent')
      expect(entry).toBeNull()
    })

    it('should return entry for revoked token', async () => {
      await revokeToken(mockKV, 'test-entry', 3600, 1, 'test-reason')

      const entry = await getBlacklistEntry(mockKV, 'test-entry')
      expect(entry).not.toBeNull()
      expect(entry!.user_id).toBe(1)
      expect(entry!.reason).toBe('test-reason')
    })
  })
})