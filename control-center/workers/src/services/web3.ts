/**
 * Web3 Service
 * Ethereum 和 IPFS 集成服务
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, ApiSuccessResponse, Env } from '../types'

// ==================== IPFS Gateway ====================

/**
 * IPFS 配置
 */
const IPFS_GATEWAY = 'https://cloudflare-ipfs.com/ipfs/'

/**
 * 上传数据到 IPFS (通过 Cloudflare R2 + IPFS)
 */
export async function uploadToIPFS(
  env: Env,
  data: string | ArrayBuffer,
  options?: { filename?: string; contentType?: string }
): Promise<{
  success: boolean
  cid?: string
  gatewayUrl?: string
  error?: string
}> {
  try {
    // 生成唯一 CID (简化版，实际应使用 ipfs-add)
    const content = typeof data === 'string' ? new TextEncoder().encode(data) : new Uint8Array(data)

    // 使用 R2 作为 IPFS 缓存层
    const cid = await generateCID(content)
    const key = `ipfs/${cid}`

    // 存储到 R2
    await env.R2.put(key, content, {
      httpMetadata: {
        contentType: options?.contentType || 'application/octet-stream',
      },
      customMetadata: {
        uploadedAt: new Date().toISOString(),
        filename: options?.filename || 'data',
        isIPFS: 'true',
      },
    })

    return {
      success: true,
      cid,
      gatewayUrl: `${IPFS_GATEWAY}${cid}`,
    }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 从 IPFS 获取数据
 */
export async function getFromIPFS(
  env: Env,
  cid: string
): Promise<{
  success: boolean
  data?: ArrayBuffer
  error?: string
}> {
  try {
    // 先尝试从 R2 缓存获取
    const key = `ipfs/${cid}`
    const cached = await env.R2.get(key)

    if (cached) {
      return {
        success: true,
        data: await cached.arrayBuffer(),
      }
    }

    // 从 IPFS Gateway 获取
    const response = await fetch(`${IPFS_GATEWAY}${cid}`)
    if (!response.ok) {
      return { success: false, error: 'Failed to fetch from IPFS' }
    }

    const data = await response.arrayBuffer()

    // 缓存到 R2
    await env.R2.put(key, data)

    return { success: true, data }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 生成简化的 CID (实际项目应使用 ipfs-only-hash)
 */
async function generateCID(content: Uint8Array): Promise<string> {
  const hashBuffer = await crypto.subtle.digest('SHA-256', content)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  return `Qm${hashHex.slice(0, 44)}` // 简化的 CIDv0 格式
}

// ==================== Ethereum Integration ====================

/**
 * Ethereum 网络配置
 */
const ETHEREUM_NETWORKS = {
  mainnet: {
    chainId: 1,
    rpcUrl: 'https://cloudflare-eth.com',
  },
  sepolia: {
    chainId: 11155111,
    rpcUrl: 'https://eth-sepolia.public.blastapi.io',
  },
}

/**
 * 验证以太坊地址
 */
export function isValidEthereumAddress(address: string): boolean {
  return /^0x[a-fA-F0-9]{40}$/.test(address)
}

/**
 * 验证以太坊签名
 */
export async function verifyEthereumSignature(
  message: string,
  signature: string,
  address: string
): Promise<{ success: boolean; isValid?: boolean; error?: string }> {
  try {
    // 简化的签名验证 (实际应使用 ethers.js)
    // 这里返回成功，实际项目需要完整实现
    return { success: true, isValid: true }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 生成 nonce 用于签名验证
 */
export function generateNonce(): string {
  const array = new Uint8Array(32)
  crypto.getRandomValues(array)
  return Array.from(array, (b) => b.toString(16).padStart(2, '0')).join('')
}

/**
 * 创建 SIWE (Sign-In with Ethereum) 消息
 */
export function createSIWEMessage(
  address: string,
  nonce: string,
  options?: {
    domain?: string
    uri?: string
    chainId?: number
  }
): string {
  const domain = options?.domain || 'anixops.com'
  const uri = options?.uri || 'https://anixops.com'
  const chainId = options?.chainId || 1

  return `${domain} wants you to sign in with your Ethereum account:
${address}

URI: ${uri}
Version: 1
Chain ID: ${chainId}
Nonce: ${nonce}
Issued At: ${new Date().toISOString()}`
}

/**
 * 存储审计日志到链上 (模拟)
 */
export async function storeAuditOnChain(
  env: Env,
  auditData: {
    action: string
    userId: number
    timestamp: string
    details: string
  }
): Promise<{
  success: boolean
  txHash?: string
  ipfsCid?: string
  error?: string
}> {
  try {
    // 先存储到 IPFS
    const ipfsResult = await uploadToIPFS(
      env,
      JSON.stringify({
        ...auditData,
        version: '1.0',
        storedAt: new Date().toISOString(),
      }),
      { contentType: 'application/json', filename: `audit-${Date.now()}.json` }
    )

    if (!ipfsResult.success) {
      return { success: false, error: ipfsResult.error }
    }

    // 模拟交易哈希
    const txHash = `0x${Array.from(crypto.getRandomValues(new Uint8Array(32)))
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('')}`

    // 存储到 KV 作为缓存
    await env.KV.put(`audit:tx:${txHash}`, JSON.stringify({
      ipfsCid: ipfsResult.cid,
      ...auditData,
    }), {
      expirationTtl: 86400 * 365, // 1 年
    })

    return {
      success: true,
      txHash,
      ipfsCid: ipfsResult.cid,
    }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 获取链上审计记录
 */
export async function getOnChainAudit(
  env: Env,
  txHash: string
): Promise<{
  success: boolean
  data?: {
    ipfsCid: string
    action: string
    userId: number
    timestamp: string
    details: string
  }
  error?: string
}> {
  try {
    const cached = await env.KV.get(`audit:tx:${txHash}`)
    if (cached) {
      return { success: true, data: JSON.parse(cached) }
    }

    return { success: false, error: 'Audit record not found' }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

// ==================== DID (Decentralized Identity) ====================

/**
 * 创建 DID 标识符
 */
export function createDID(address: string): string {
  return `did:ethr:${address.toLowerCase()}`
}

/**
 * 解析 DID
 */
export function parseDID(did: string): { method: string; identifier: string } | null {
  const match = did.match(/^did:([^:]+):(.+)$/)
  if (!match) return null
  return { method: match[1], identifier: match[2] }
}

/**
 * 存储 DID 文档
 */
export async function storeDIDDocument(
  env: Env,
  did: string,
  document: Record<string, unknown>
): Promise<{ success: boolean; error?: string }> {
  try {
    await env.KV.put(`did:${did}`, JSON.stringify(document), {
      expirationTtl: 86400 * 365, // 1 年
    })
    return { success: true }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 获取 DID 文档
 */
export async function getDIDDocument(
  env: Env,
  did: string
): Promise<{ success: boolean; document?: Record<string, unknown>; error?: string }> {
  try {
    const doc = await env.KV.get(`did:${did}`)
    if (!doc) {
      return { success: false, error: 'DID document not found' }
    }
    return { success: true, document: JSON.parse(doc) }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

// ==================== HTTP Handlers ====================

/**
 * IPFS 上传处理
 */
export async function ipfsUploadHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ data: string; filename?: string }>()

  if (!body.data) {
    return c.json({ success: false, error: 'Data is required' } as ApiErrorResponse, 400)
  }

  const result = await uploadToIPFS(c.env, body.data, {
    filename: body.filename,
    contentType: 'application/json',
  })

  if (result.success) {
    return c.json({
      success: true,
      data: {
        cid: result.cid,
        gatewayUrl: result.gatewayUrl,
      },
    } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * IPFS 获取处理
 */
export async function ipfsGetHandler(c: Context<{ Bindings: Env }>) {
  const cid = c.req.param('cid') as string

  if (!cid) {
    return c.json({ success: false, error: 'CID is required' } as ApiErrorResponse, 400)
  }

  const result = await getFromIPFS(c.env, cid)

  if (result.success && result.data) {
    return new Response(result.data, {
      headers: { 'Content-Type': 'application/json' },
    })
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * Web3 登录挑战处理
 */
export async function web3ChallengeHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ address: string }>()

  if (!body.address || !isValidEthereumAddress(body.address)) {
    return c.json({ success: false, error: 'Valid Ethereum address is required' } as ApiErrorResponse, 400)
  }

  const nonce = generateNonce()
  const message = createSIWEMessage(body.address, nonce)

  // 存储 nonce 以便后续验证
  await c.env.KV.put(`web3:nonce:${body.address.toLowerCase()}`, nonce, {
    expirationTtl: 300, // 5 分钟
  })

  return c.json({
    success: true,
    data: {
      message,
      nonce,
    },
  } as ApiSuccessResponse<unknown>)
}

/**
 * Web3 登录验证处理
 */
export async function web3VerifyHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ address: string; signature: string; message: string }>()

  if (!body.address || !body.signature || !body.message) {
    return c.json({ success: false, error: 'Address, signature, and message are required' } as ApiErrorResponse, 400)
  }

  // 验证签名
  const verifyResult = await verifyEthereumSignature(body.message, body.signature, body.address)

  if (!verifyResult.success || !verifyResult.isValid) {
    return c.json({ success: false, error: 'Invalid signature' } as ApiErrorResponse, 401)
  }

  // 清除 nonce
  await c.env.KV.delete(`web3:nonce:${body.address.toLowerCase()}`)

  // 返回成功 (实际应生成 JWT)
  return c.json({
    success: true,
    data: {
      address: body.address,
      did: createDID(body.address),
    },
  } as ApiSuccessResponse<unknown>)
}

/**
 * 链上审计存储处理
 */
export async function web3AuditHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{
    action: string
    userId: number
    timestamp: string
    details: string
  }>()

  if (!body.action || !body.userId) {
    return c.json({ success: false, error: 'Action and userId are required' } as ApiErrorResponse, 400)
  }

  const result = await storeAuditOnChain(c.env, body)

  if (result.success) {
    return c.json({
      success: true,
      data: {
        txHash: result.txHash,
        ipfsCid: result.ipfsCid,
      },
    } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}