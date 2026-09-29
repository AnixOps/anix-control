import type { Env } from '../types'

/**
 * Token 黑名单管理工具
 */

interface TokenBlacklistEntry {
  user_id: string | number
  revoked_at: string
  reason?: string
}

/**
 * 将 Token 加入黑名单
 */
export async function revokeToken(
  kv: KVNamespace,
  token: string,
  ttl: number,
  userId: string | number,
  reason?: string
): Promise<void> {
  if (ttl <= 0) return

  await kv.put(`blacklist:${token}`, JSON.stringify({
    user_id: userId,
    revoked_at: new Date().toISOString(),
    reason,
  }), {
    expirationTtl: ttl,
  })
}

/**
 * 检查 Token 是否在黑名单中
 */
export async function isTokenRevoked(kv: KVNamespace, token: string): Promise<boolean> {
  const entry = await kv.get(`blacklist:${token}`)
  return entry !== null
}

/**
 * 撤销用户的所有会话
 * 通过在 KV 中存储用户的撤销时间戳，所有该时间之前签发的 Token 都被视为无效
 */
export async function revokeAllUserSessions(
  kv: KVNamespace,
  userId: number
): Promise<void> {
  await kv.put(`user_revoke:${userId}`, new Date().toISOString(), {
    expirationTtl: 86400 * 30, // 30 天后自动清理
  })
}

/**
 * 检查用户的 Token 是否在全局撤销时间之前签发
 */
export async function isUserSessionRevoked(
  kv: KVNamespace,
  userId: number,
  tokenIssuedAt: number
): Promise<boolean> {
  const revokeTime = await kv.get(`user_revoke:${userId}`)
  if (!revokeTime) return false

  const revokeTimestamp = Math.floor(new Date(revokeTime).getTime() / 1000)
  return tokenIssuedAt < revokeTimestamp
}

/**
 * 获取黑名单条目详情
 */
export async function getBlacklistEntry(
  kv: KVNamespace,
  token: string
): Promise<TokenBlacklistEntry | null> {
  const entry = await kv.get(`blacklist:${token}`)
  if (!entry) return null

  try {
    return JSON.parse(entry) as TokenBlacklistEntry
  } catch {
    return null
  }
}