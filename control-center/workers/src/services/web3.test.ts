import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  isValidEthereumAddress,
  generateNonce,
  createSIWEMessage,
  createDID,
  parseDID,
  uploadToIPFS,
  getFromIPFS,
  storeAuditOnChain,
  getOnChainAudit,
  ipfsUploadHandler,
  ipfsGetHandler,
  web3ChallengeHandler,
  web3VerifyHandler,
  web3AuditHandler,
} from './web3'
import type { Env } from '../types'

function createEnv() {
  const kv = new Map<string, string>()
  const r2 = new Map<string, Uint8Array>()

  return {
    KV: {
      get: vi.fn(async (key: string) => kv.get(key) ?? null),
      put: vi.fn(async (key: string, value: string) => {
        kv.set(key, value)
      }),
      delete: vi.fn(async (key: string) => {
        kv.delete(key)
      }),
    },
    R2: {
      put: vi.fn(async (key: string, value: string | ArrayBuffer | Uint8Array) => {
        const bytes = typeof value === 'string'
          ? new TextEncoder().encode(value)
          : value instanceof Uint8Array
            ? value
            : new Uint8Array(value)
        r2.set(key, bytes)
      }),
      get: vi.fn(async (key: string) => {
        const value = r2.get(key)
        if (!value) return null
        return {
          arrayBuffer: async () => value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength),
        }
      }),
    },
  } as unknown as Env & {
    KV: {
      get: ReturnType<typeof vi.fn>
      put: ReturnType<typeof vi.fn>
      delete: ReturnType<typeof vi.fn>
    }
    R2: {
      put: ReturnType<typeof vi.fn>
      get: ReturnType<typeof vi.fn>
    }
  }
}

function createContext({ body, params, env, user }: {
  body?: unknown
  params?: Record<string, string>
  env: Env
  user?: unknown
}) {
  return {
    env,
    get: (key: string) => (key === 'user' ? user : undefined),
    req: {
      json: async () => body,
      param: (name: string) => params?.[name],
    },
    json: (data: unknown, status = 200) => new Response(JSON.stringify(data), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  }
}

describe('web3 service', () => {
  let env: ReturnType<typeof createEnv>

  beforeEach(() => {
    env = createEnv()
  })

  it('validates ethereum addresses', () => {
    expect(isValidEthereumAddress('0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18')).toBe(true)
    expect(isValidEthereumAddress('0x123')).toBe(false)
  })

  it('generates nonce and SIWE message', () => {
    const nonce = generateNonce()
    const message = createSIWEMessage('0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18', nonce)

    expect(nonce).toHaveLength(64)
    expect(message).toContain(nonce)
    expect(message).toContain('Ethereum account')
  })

  it('creates and parses DID', () => {
    const did = createDID('0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18')
    expect(did).toBe('did:ethr:0x742d35cc6634c0532925a3b844bc9e7595f2bd18')
    expect(parseDID(did)).toEqual({
      method: 'ethr',
      identifier: '0x742d35cc6634c0532925a3b844bc9e7595f2bd18',
    })
  })

  it('uploads to and reads from IPFS-backed R2 storage', async () => {
    const upload = await uploadToIPFS(env, JSON.stringify({ hello: 'world' }), {
      filename: 'test.json',
      contentType: 'application/json',
    })

    expect(upload.success).toBe(true)
    expect(upload.cid).toBeDefined()
    expect(upload.gatewayUrl).toContain(upload.cid!)

    const fetched = await getFromIPFS(env, upload.cid!)
    expect(fetched.success).toBe(true)
    const decoded = new TextDecoder().decode(fetched.data)
    expect(decoded).toContain('hello')
  })

  it('stores and retrieves on-chain audit metadata', async () => {
    const stored = await storeAuditOnChain(env, {
      action: 'node.restart',
      userId: 1,
      timestamp: '2026-03-23T00:00:00Z',
      details: 'Restarted edge node',
    })

    expect(stored.success).toBe(true)
    expect(stored.txHash).toMatch(/^0x[a-f0-9]{64}$/)
    expect(stored.ipfsCid).toBeDefined()

    const fetched = await getOnChainAudit(env, stored.txHash!)
    expect(fetched.success).toBe(true)
    expect(fetched.data?.action).toBe('node.restart')
    expect(fetched.data?.ipfsCid).toBe(stored.ipfsCid)
  })
})

describe('web3 handlers', () => {
  let env: ReturnType<typeof createEnv>

  beforeEach(() => {
    env = createEnv()
  })

  it('ipfsUploadHandler validates data', async () => {
    const response = await ipfsUploadHandler(createContext({ body: {}, env }) as never)
    expect(response.status).toBe(400)
  })

  it('ipfsUploadHandler returns cid and gateway url', async () => {
    const response = await ipfsUploadHandler(createContext({
      body: { data: '{"hello":"world"}', filename: 'hello.json' },
      env,
    }) as never)

    expect(response.status).toBe(200)
    const data = await response.json() as { success: boolean; data: { cid: string; gatewayUrl: string } }
    expect(data.success).toBe(true)
    expect(data.data.cid).toBeDefined()
    expect(data.data.gatewayUrl).toContain(data.data.cid)
  })

  it('ipfsGetHandler returns uploaded content', async () => {
    const uploaded = await uploadToIPFS(env, '{"k":"v"}')
    const response = await ipfsGetHandler(createContext({
      params: { cid: uploaded.cid! },
      env,
    }) as never)

    expect(response.status).toBe(200)
    const text = await response.text()
    expect(text).toContain('k')
  })

  it('web3ChallengeHandler validates address and stores nonce', async () => {
    const bad = await web3ChallengeHandler(createContext({ body: { address: 'bad' }, env }) as never)
    expect(bad.status).toBe(400)

    const good = await web3ChallengeHandler(createContext({
      body: { address: '0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18' },
      env,
    }) as never)
    expect(good.status).toBe(200)
    const data = await good.json() as { success: boolean; data: { message: string; nonce: string } }
    expect(data.success).toBe(true)
    expect(data.data.message).toContain(data.data.nonce)
  })

  it('web3VerifyHandler requires address signature and message', async () => {
    const response = await web3VerifyHandler(createContext({ body: {}, env }) as never)
    expect(response.status).toBe(400)
  })

  it('web3VerifyHandler returns did for valid payload', async () => {
    const response = await web3VerifyHandler(createContext({
      body: {
        address: '0x742d35Cc6634C0532925a3b844Bc9e7595f2bD18',
        signature: '0xsigned',
        message: 'anixops.com wants you to sign in',
      },
      env,
    }) as never)

    expect(response.status).toBe(200)
    const data = await response.json() as { success: boolean; data: { did: string } }
    expect(data.success).toBe(true)
    expect(data.data.did).toContain('did:ethr:')
  })

  it('web3AuditHandler validates required fields', async () => {
    const bad = await web3AuditHandler(createContext({ body: {}, env }) as never)
    expect(bad.status).toBe(400)

    const good = await web3AuditHandler(createContext({
      body: {
        action: 'node.restart',
        userId: 1,
        timestamp: '2026-03-23T00:00:00Z',
        details: 'Restarted edge node',
      },
      env,
      user: { sub: 1 },
    }) as never)

    expect(good.status).toBe(200)
    const data = await good.json() as { success: boolean; data: { txHash: string; ipfsCid: string } }
    expect(data.success).toBe(true)
    expect(data.data.txHash).toMatch(/^0x[a-f0-9]{64}$/)
    expect(data.data.ipfsCid).toBeDefined()
  })
})
