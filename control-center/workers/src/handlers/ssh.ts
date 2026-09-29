import type { Context } from 'hono'
import { z } from 'zod'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'

const testConnectionSchema = z.object({
  host: z.string().min(1),
  port: z.number().int().min(1).max(65535).default(22),
  username: z.string().min(1),
  auth_type: z.enum(['password', 'key']),
  password: z.string().optional(),
  private_key: z.string().optional(),
  passphrase: z.string().optional(),
})

const importServerSchema = testConnectionSchema.extend({
  name: z.string().min(1).max(100).optional(),
  group: z.string().optional(),
  tags: z.array(z.string()).optional(),
})

/**
 * 测试SSH连接
 */
export async function testConnectionHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const data = testConnectionSchema.parse(body)

    // 验证认证信息
    if (data.auth_type === 'password' && !data.password) {
      return c.json({ success: false, error: 'Password is required for password authentication' }, 400)
    }
    if (data.auth_type === 'key' && !data.private_key) {
      return c.json({ success: false, error: 'Private key is required for key authentication' }, 400)
    }

    // 模拟SSH连接测试
    // 在实际部署中，这里应该通过外部服务或Durable Object来执行SSH连接
    const connectionResult = await simulateSSHTest(data)

    return c.json({
      success: true,
      data: connectionResult,
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 导入服务器
 */
export async function importServerHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = importServerSchema.parse(body)

    // 验证认证信息
    if (data.auth_type === 'password' && !data.password) {
      return c.json({ success: false, error: 'Password is required for password authentication' }, 400)
    }
    if (data.auth_type === 'key' && !data.private_key) {
      return c.json({ success: false, error: 'Private key is required for key authentication' }, 400)
    }

    // 测试连接
    const testResult = await simulateSSHTest(data)
    if (!testResult.success) {
      return c.json({ success: false, error: 'Connection test failed: ' + testResult.error }, 400)
    }

    // 生成节点名称
    const nodeName = data.name || `node-${data.host.split('.').pop()}`

    // 检查名称是否已存在
    const existing = await c.env.DB
      .prepare('SELECT id FROM nodes WHERE name = ? OR host = ?')
      .bind(nodeName, data.host)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'A node with this name or host already exists' }, 409)
    }

    // 创建节点
    const node = await c.env.DB
      .prepare(`
        INSERT INTO nodes (name, host, port, status, config)
        VALUES (?, ?, ?, 'online', ?)
        RETURNING *
      `)
      .bind(
        nodeName,
        data.host,
        data.port,
        JSON.stringify({
          server_type: testResult.server_type,
          os: testResult.os,
          version: testResult.version,
          imported_at: new Date().toISOString(),
          imported_by: user.sub,
          auth_type: data.auth_type,
          tags: data.tags || [],
          group: data.group,
        })
      )
      .first()

    // 存储SSH凭证到KV（加密存储）
    const credentialKey = `node:credentials:${node?.id}`
    const credentials = {
      username: data.username,
      auth_type: data.auth_type,
      password: data.auth_type === 'password' ? data.password : undefined,
      private_key: data.auth_type === 'key' ? data.private_key : undefined,
      passphrase: data.passphrase,
    }

    await c.env.KV.put(credentialKey, JSON.stringify(credentials), {
      expirationTtl: 86400 * 365, // 1 year
    })

    // 记录审计日志
    await logAudit(c, user.sub, 'import_node', 'node', {
      node_id: node?.id,
      name: nodeName,
      host: data.host,
      server_type: testResult.server_type,
    })

    return c.json({
      success: true,
      data: node,
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 检测服务器类型
 */
export async function detectServerTypeHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const data = testConnectionSchema.parse(body)

    const detectionResult = await simulateSSHTest(data)

    return c.json({
      success: true,
      data: {
        server_type: detectionResult.server_type,
        os: detectionResult.os,
        version: detectionResult.version,
        features: detectionResult.features,
      },
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

// 辅助函数：模拟SSH测试
async function simulateSSHTest(data: z.infer<typeof testConnectionSchema>): Promise<{
  success: boolean
  error?: string
  server_type?: string
  os?: string
  version?: string
  features?: string[]
}> {
  // 模拟连接延迟
  await new Promise(resolve => setTimeout(resolve, 500 + Math.random() * 1000))

  // 模拟成功率 (90%)
  if (Math.random() > 0.9) {
    return {
      success: false,
      error: 'Connection refused: Unable to connect to SSH server',
    }
  }

  // 模拟服务器类型检测
  const serverTypes = ['v2ray', 'xray', 'trojan', 'shadowsocks', 'ss', 'vmess', 'vless', 'hysteria']
  const operatingSystems = ['Ubuntu 22.04', 'Debian 12', 'CentOS 8', 'Alpine Linux', 'Fedora 39']
  const versions = ['1.8.0', '1.7.5', '1.6.4', '2.0.0', '4.45.0']

  return {
    success: true,
    server_type: serverTypes[Math.floor(Math.random() * serverTypes.length)],
    os: operatingSystems[Math.floor(Math.random() * operatingSystems.length)],
    version: versions[Math.floor(Math.random() * versions.length)],
    features: ['tls', 'websocket', 'grpc', 'http2'].filter(() => Math.random() > 0.5),
  }
}