/**
 * 账户锁定管理工具
 * 防止暴力破解攻击
 */

export interface LockoutConfig {
  maxAttempts: number       // 最大尝试次数
  lockoutDuration: number   // 锁定时间（秒）
  windowDuration: number    // 尝试计数窗口（秒）
}

export const DEFAULT_LOCKOUT_CONFIG: LockoutConfig = {
  maxAttempts: 5,
  lockoutDuration: 900, // 15 分钟
  windowDuration: 300,  // 5 分钟
}

export interface LockoutStatus {
  locked: boolean
  attempts: number
  remainingAttempts: number
  lockedUntil?: string
  reason?: string
}

/**
 * 获取锁定状态的 KV key
 */
function getLockoutKey(identifier: string): string {
  return `lockout:${identifier}`
}

/**
 * 获取尝试次数的 KV key
 */
function getAttemptsKey(identifier: string): string {
  return `attempts:${identifier}`
}

/**
 * 检查账户是否被锁定
 */
export async function checkLockout(
  kv: KVNamespace,
  identifier: string,
  config: LockoutConfig = DEFAULT_LOCKOUT_CONFIG
): Promise<LockoutStatus> {
  const lockoutKey = getLockoutKey(identifier)
  const attemptsKey = getAttemptsKey(identifier)

  // 检查是否锁定
  const lockoutData = await kv.get(lockoutKey, 'json') as {
    locked_until: string;
    reason: string;
    attempts: number;
  } | null

  if (lockoutData) {
    const lockedUntil = new Date(lockoutData.locked_until)
    if (lockedUntil > new Date()) {
      return {
        locked: true,
        attempts: lockoutData.attempts,
        remainingAttempts: 0,
        lockedUntil: lockoutData.locked_until,
        reason: lockoutData.reason,
      }
    }
    // 锁定已过期，清除锁定状态
    await kv.delete(lockoutKey)
  }

  // 获取当前尝试次数
  const attemptsData = await kv.get(attemptsKey)
  const attempts = attemptsData ? parseInt(attemptsData, 10) : 0

  return {
    locked: false,
    attempts,
    remainingAttempts: Math.max(0, config.maxAttempts - attempts),
  }
}

/**
 * 记录失败尝试
 */
export async function recordFailedAttempt(
  kv: KVNamespace,
  identifier: string,
  config: LockoutConfig = DEFAULT_LOCKOUT_CONFIG
): Promise<LockoutStatus> {
  const lockoutKey = getLockoutKey(identifier)
  const attemptsKey = getAttemptsKey(identifier)

  // 获取当前尝试次数
  const attemptsData = await kv.get(attemptsKey)
  const currentAttempts = attemptsData ? parseInt(attemptsData, 10) : 0
  const newAttempts = currentAttempts + 1

  // 存储尝试次数
  await kv.put(attemptsKey, String(newAttempts), {
    expirationTtl: config.windowDuration,
  })

  // 检查是否需要锁定
  if (newAttempts >= config.maxAttempts) {
    const lockedUntil = new Date(Date.now() + config.lockoutDuration * 1000)

    await kv.put(lockoutKey, JSON.stringify({
      locked_until: lockedUntil.toISOString(),
      reason: 'Too many failed login attempts',
      attempts: newAttempts,
      locked_at: new Date().toISOString(),
    }), {
      expirationTtl: config.lockoutDuration,
    })

    return {
      locked: true,
      attempts: newAttempts,
      remainingAttempts: 0,
      lockedUntil: lockedUntil.toISOString(),
      reason: 'Too many failed login attempts',
    }
  }

  return {
    locked: false,
    attempts: newAttempts,
    remainingAttempts: config.maxAttempts - newAttempts,
  }
}

/**
 * 清除失败尝试记录（登录成功后调用）
 */
export async function clearFailedAttempts(
  kv: KVNamespace,
  identifier: string
): Promise<void> {
  const attemptsKey = getAttemptsKey(identifier)
  await kv.delete(attemptsKey)
}

/**
 * 手动解锁账户（管理员操作）
 */
export async function unlockAccount(
  kv: KVNamespace,
  identifier: string
): Promise<void> {
  const lockoutKey = getLockoutKey(identifier)
  const attemptsKey = getAttemptsKey(identifier)

  await kv.delete(lockoutKey)
  await kv.delete(attemptsKey)
}

/**
 * 获取锁定信息
 */
export async function getLockoutInfo(
  kv: KVNamespace,
  identifier: string
): Promise<{
  isLocked: boolean;
  lockedUntil?: string;
  attempts: number;
  reason?: string;
} | null> {
  const lockoutKey = getLockoutKey(identifier)
  const lockoutData = await kv.get(lockoutKey, 'json') as {
    locked_until: string;
    reason: string;
    attempts: number;
  } | null

  if (!lockoutData) {
    return null
  }

  const lockedUntil = new Date(lockoutData.locked_until)
  const isLocked = lockedUntil > new Date()

  return {
    isLocked,
    lockedUntil: isLocked ? lockoutData.locked_until : undefined,
    attempts: lockoutData.attempts,
    reason: lockoutData.reason,
  }
}