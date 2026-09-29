---
name: cloudflare_integration_plan
description: Cloudflare database and API integration plan
type: project
---

# Cloudflare 集成方案规划

## Why
- 降低基础设施成本（Cloudflare 免费套餐 + 按需付费）
- 全球边缘部署，降低延迟
- 无服务器架构，简化运维
- 内置 DDoS 防护和 CDN

## How to apply
采用混合架构：Mobile App 通过 Cloudflare Workers API 访问 D1 数据库，本地 CLI/TUI 继续使用本地 SQLite。

---

## 一、架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端层                                   │
├──────────────┬──────────────┬──────────────┬───────────────────┤
│  Flutter App │   Web App    │   CLI/TUI    │   Admin Panel     │
│   (Mobile)   │   (Browser)  │   (Local)    │   (Dashboard)     │
└──────┬───────┴──────┬───────┴──────┬───────┴────────┬──────────┘
       │              │              │                │
       ▼              ▼              ▼                ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Cloudflare Edge Network                        │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Cloudflare Workers                        │ │
│  │         (Serverless API - 全球边缘执行)                       │ │
│  └──────────────────────────┬──────────────────────────────────┘ │
│                             │                                     │
│  ┌─────────────┬────────────┼────────────┬─────────────────────┐ │
│  │    D1       │     KV     │     R2     │   Access/Durable    │ │
│  │ (SQLite)    │  (Cache)   │  (Files)   │    (Auth/State)     │ │
│  └─────────────┴────────────┴────────────┴─────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
       │
       ▼ (可选：内网穿透)
┌──────────────────────────────────────────────────────────────────┐
│                      私有基础设施                                  │
│  ┌──────────────┬──────────────┬──────────────────────────────┐ │
│  │  Ansible     │    V2Board   │        Agents                │ │
│  │  (自动化)    │   (代理管理)  │      (节点代理)               │ │
│  └──────────────┴──────────────┴──────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

---

## 二、Cloudflare 服务选型

| 服务 | 用途 | 免费额度 | 付费方案 |
|------|------|----------|----------|
| **Workers** | API 端点 | 100k 请求/天 | $5/月起 |
| **D1** | 主数据库 | 5GB 存储，500万行读/天 | $5/月起 |
| **KV** | 会话/缓存 | 1GB 存储，10万读/天 | $5/月起 |
| **R2** | 文件存储 | 10GB 存储 | $4.50/月起 |
| **Access** | 零信任认证 | 50 用户免费 | $3/用户/月 |
| **Durable Objects** | 有状态服务 | 按使用计费 | 按需 |

---

## 三、数据库设计 (D1)

### 3.1 表结构迁移

```sql
-- 用户表 (从 models.go 迁移)
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT DEFAULT 'viewer',
    auth_provider TEXT DEFAULT 'local',
    enabled INTEGER DEFAULT 1,
    last_login_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- API Token 表
CREATE TABLE api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT,
    token TEXT UNIQUE NOT NULL,
    expires_at TEXT,
    last_used TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 审计日志表
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL,
    resource TEXT,
    ip TEXT,
    user_agent TEXT,
    details TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 节点表 (新增)
CREATE TABLE nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    host TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    status TEXT DEFAULT 'offline',
    last_seen TEXT,
    config TEXT,  -- JSON
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX idx_nodes_status ON nodes(status);
```

### 3.2 D1 配置

```bash
# wrangler.toml
name = "anixops-api"
main = "src/index.ts"
compatibility_date = "2024-01-01"

[[d1_databases]]
binding = "DB"
database_name = "anixops-db"
database_id = "xxx-xxx-xxx"

[[kv_namespaces]]
binding = "KV"
id = "xxx-xxx-xxx"

[[r2_buckets]]
binding = "R2"
bucket_name = "anixops-files"
```

---

## 四、API 设计 (Workers)

### 4.1 API 路由

```typescript
// src/index.ts
import { Hono } from 'hono'
import { cors } from 'hono/cors'
import { jwt } from 'hono/jwt'

const app = new Hono()

// CORS 配置
app.use('/*', cors({
  origin: ['https://anixops.pages.dev', 'http://localhost:3000'],
  allowMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
  allowHeaders: ['Content-Type', 'Authorization'],
  credentials: true,
}))

// 公开路由
app.post('/api/v1/auth/login', handleLogin)
app.post('/api/v1/auth/register', handleRegister)
app.post('/api/v1/auth/refresh', handleRefresh)
app.get('/api/v1/health', handleHealth)

// JWT 保护路由
app.use('/api/v1/*', jwt({ secret: env.JWT_SECRET }))

// 用户
app.get('/api/v1/users/me', handleGetCurrentUser)
app.put('/api/v1/users/me', handleUpdateUser)

// 节点
app.get('/api/v1/nodes', handleListNodes)
app.post('/api/v1/nodes', handleCreateNode)
app.get('/api/v1/nodes/:id', handleGetNode)
app.put('/api/v1/nodes/:id', handleUpdateNode)
app.delete('/api/v1/nodes/:id', handleDeleteNode)
app.post('/api/v1/nodes/:id/exec', handleExecNode)

// Playbooks
app.get('/api/v1/playbooks', handleListPlaybooks)
app.post('/api/v1/playbooks/:name/run', handleRunPlaybook)

// 插件
app.get('/api/v1/plugins', handleListPlugins)
app.post('/api/v1/plugins/:name/execute', handleExecutePlugin)

// Dashboard
app.get('/api/v1/dashboard', handleGetDashboard)
app.get('/api/v1/dashboard/stats', handleGetStats)

// 审计日志
app.get('/api/v1/audit-logs', handleListAuditLogs)

export default app
```

### 4.2 认证中间件

```typescript
// src/middleware/auth.ts
import { Context, Next } from 'hono'
import { verify } from 'jsonwebtoken'

export async function authMiddleware(c: Context, next: Next) {
  const token = c.req.header('Authorization')?.replace('Bearer ', '')

  if (!token) {
    return c.json({ error: 'unauthorized' }, 401)
  }

  try {
    const payload = verify(token, c.env.JWT_SECRET)
    c.set('user', payload)
    await next()
  } catch (err) {
    return c.json({ error: 'invalid token' }, 401)
  }
}

export async function rbacMiddleware(roles: string[]) {
  return async (c: Context, next: Next) => {
    const user = c.get('user')
    if (!roles.includes(user.role)) {
      return c.json({ error: 'forbidden' }, 403)
    }
    await next()
  }
}
```

### 4.3 D1 数据访问

```typescript
// src/services/user.service.ts
export class UserService {
  constructor(private db: D1Database) {}

  async findByEmail(email: string) {
    const result = await this.db
      .prepare('SELECT * FROM users WHERE email = ?')
      .bind(email)
      .first()
    return result
  }

  async create(data: { email: string; passwordHash: string; role?: string }) {
    const result = await this.db
      .prepare(`
        INSERT INTO users (email, password_hash, role)
        VALUES (?, ?, ?)
        RETURNING *
      `)
      .bind(data.email, data.passwordHash, data.role || 'viewer')
      .first()
    return result
  }

  async list(page: number, perPage: number) {
    const offset = (page - 1) * perPage
    const results = await this.db
      .prepare('SELECT id, email, role, enabled, created_at FROM users LIMIT ? OFFSET ?')
      .bind(perPage, offset)
      .all()
    return results.results
  }
}
```

---

## 五、缓存策略 (KV)

### 5.1 会话管理

```typescript
// src/services/session.service.ts
export class SessionService {
  constructor(private kv: KVNamespace) {}

  async create(userId: number, payload: object): Promise<string> {
    const sessionId = crypto.randomUUID()
    const key = `session:${sessionId}`

    await this.kv.put(key, JSON.stringify({
      userId,
      ...payload,
      createdAt: new Date().toISOString()
    }), {
      expirationTtl: 86400, // 24 hours
    })

    return sessionId
  }

  async get(sessionId: string) {
    const data = await this.kv.get(`session:${sessionId}`)
    return data ? JSON.parse(data) : null
  }

  async delete(sessionId: string) {
    await this.kv.delete(`session:${sessionId}`)
  }

  async refresh(sessionId: string) {
    const data = await this.get(sessionId)
    if (data) {
      await this.kv.put(`session:${sessionId}`, JSON.stringify(data), {
        expirationTtl: 86400,
      })
    }
  }
}
```

### 5.2 缓存层

```typescript
// src/services/cache.service.ts
export class CacheService {
  constructor(private kv: KVNamespace) {}

  // 节点状态缓存 (5分钟)
  async getNodeStatus(nodeId: number) {
    const cached = await this.kv.get(`node:status:${nodeId}`)
    if (cached) return JSON.parse(cached)
    return null
  }

  async setNodeStatus(nodeId: number, status: object) {
    await this.kv.put(`node:status:${nodeId}`, JSON.stringify(status), {
      expirationTtl: 300, // 5 minutes
    })
  }

  // Dashboard 统计缓存 (1分钟)
  async getDashboardStats() {
    const cached = await this.kv.get('dashboard:stats')
    if (cached) return JSON.parse(cached)
    return null
  }

  async setDashboardStats(stats: object) {
    await this.kv.put('dashboard:stats', JSON.stringify(stats), {
      expirationTtl: 60, // 1 minute
    })
  }
}
```

---

## 六、文件存储 (R2)

### 6.1 Playbook 存储

```typescript
// src/services/playbook.service.ts
export class PlaybookService {
  constructor(
    private r2: R2Bucket,
    private db: D1Database
  ) {}

  async upload(name: string, content: string) {
    await this.r2.put(`playbooks/${name}.yml`, content, {
      httpMetadata: {
        contentType: 'text/yaml',
      },
    })

    // 记录元数据到 D1
    await this.db
      .prepare(`
        INSERT INTO playbooks (name, storage_key, created_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(name) DO UPDATE SET updated_at = CURRENT_TIMESTAMP
      `)
      .bind(name, `playbooks/${name}.yml`)
      .run()
  }

  async download(name: string) {
    const object = await this.r2.get(`playbooks/${name}.yml`)
    if (!object) return null
    return await object.text()
  }

  async list() {
    const result = await this.db
      .prepare('SELECT name, created_at, updated_at FROM playbooks ORDER BY name')
      .all()
    return result.results
  }
}
```

---

## 七、Cloudflare Access 集成

### 7.1 配置方案

```yaml
# Access Application 配置
application:
  name: AnixOps API
  domain: api.anixops.dev

policies:
  - name: "Admin Access"
    include:
      - email: admin@anixops.dev
      - group: admins
    require:
      - mfa: true

  - name: "Developer Access"
    include:
      - group: developers
    require:
      - mfa: false

identity_providers:
  - type: github
    name: GitHub
  - type: google
    name: Google
  - type: oidc
    name: Custom OIDC
    config:
      client_id: xxx
      client_secret: xxx
```

### 7.2 Workers 与 Access 集成

```typescript
// 验证 Access Token
export async function validateAccess(c: Context, next: Next) {
  const jwt = c.req.header('CF-Access-Jwt-Assertion')

  if (!jwt) {
    // 如果没有 Access JWT，使用自定义 JWT
    return authMiddleware(c, next)
  }

  try {
    // 验证 Cloudflare Access JWT
    const payload = await verifyAccessJWT(jwt, c.env.ACCESS_PUBLIC_KEY)
    c.set('user', {
      id: payload.sub,
      email: payload.email,
      role: payload.groups?.includes('admins') ? 'admin' : 'viewer',
    })
    await next()
  } catch (err) {
    return c.json({ error: 'invalid access token' }, 401)
  }
}
```

---

## 八、混合部署架构

### 8.1 云端 API (Workers)

```
┌─────────────────────────────────────────────────────────┐
│                  Cloudflare Workers API                  │
│                                                          │
│  端点: https://api.anixops.dev                           │
│                                                          │
│  功能:                                                   │
│  ├── 用户认证与管理                                       │
│  ├── 节点元数据管理                                       │
│  ├── Playbook 存储与管理                                  │
│  ├── 审计日志记录                                        │
│  └── Dashboard 统计                                      │
│                                                          │
│  数据存储:                                               │
│  ├── D1: 用户、节点、审计日志                             │
│  ├── KV: 会话、缓存                                      │
│  └── R2: Playbook 文件                                   │
└─────────────────────────────────────────────────────────┘
```

### 8.2 本地执行器 (自托管)

```
┌─────────────────────────────────────────────────────────┐
│                   本地执行代理                            │
│                                                          │
│  组件:                                                   │
│  ├── anixops-agent: 节点代理                             │
│  ├── anixops-executor: 任务执行器                        │
│  └── anixops-tunnel: Cloudflare Tunnel (可选)           │
│                                                          │
│  功能:                                                   │
│  ├── 执行 Ansible Playbook                               │
│  ├── 节点监控与状态上报                                   │
│  ├── WebSocket 实时通信                                  │
│  └── 本地日志收集                                        │
│                                                          │
│  通信:                                                   │
│  └── 通过 API Key 与云端同步                              │
└─────────────────────────────────────────────────────────┘
```

### 8.3 数据同步策略

```typescript
// 节点状态同步
export async function syncNodeStatus(nodeId: number, status: object) {
  // 1. 更新本地缓存
  await localCache.set(`node:${nodeId}:status`, status)

  // 2. 上报到云端
  await fetch('https://api.anixops.dev/api/v1/nodes/' + nodeId + '/status', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${API_KEY}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(status),
  })
}

// Playbook 同步
export async function syncPlaybooks() {
  // 1. 获取云端列表
  const remote = await fetch('https://api.anixops.dev/api/v1/playbooks')
  const remoteList = await remote.json()

  // 2. 对比本地
  const local = await localDb.query('SELECT * FROM playbooks')

  // 3. 下载新增/更新的
  for (const playbook of remoteList) {
    if (!local.includes(playbook.name) || playbook.updated_at > local.updated_at) {
      const content = await fetch(playbook.download_url)
      await fs.writeFile(`./playbooks/${playbook.name}.yml`, await content.text())
    }
  }
}
```

---

## 九、实施路线图

### Phase 1: 基础设施搭建 (Week 1-2)

- [ ] 创建 Cloudflare 项目
- [ ] 配置 D1 数据库
- [ ] 配置 KV 命名空间
- [ ] 配置 R2 存储桶
- [ ] 设置自定义域名 (api.anixops.dev)

### Phase 2: API 开发 (Week 3-4)

- [ ] 实现 Workers API 框架
- [ ] 实现用户认证 (JWT + Access)
- [ ] 实现节点 CRUD API
- [ ] 实现 Playbook 管理 API
- [ ] 实现 Dashboard API

### Phase 3: 客户端适配 (Week 5-6)

- [ ] 更新 Flutter App API 客户端
- [ ] 更新 Web App API 客户端
- [ ] 实现 CLI 云端模式
- [ ] 测试跨平台兼容性

### Phase 4: 高级功能 (Week 7-8)

- [ ] 实现实时通信 (WebSocket over Durable Objects)
- [ ] 实现离线同步
- [ ] 实现 Cloudflare Access SSO
- [ ] 性能优化与监控

---

## 十、成本估算

### 免费套餐 (开发/测试)

| 服务 | 免费额度 | 预计使用 |
|------|----------|----------|
| Workers | 100k 请求/天 | 足够开发测试 |
| D1 | 5GB, 500万读/天 | 足够小型部署 |
| KV | 1GB, 10万读/天 | 需监控 |
| R2 | 10GB | 足够 |
| Access | 50 用户 | 足够小团队 |

**月成本: $0**

### 生产环境 (估算)

| 服务 | 配置 | 月成本 |
|------|------|--------|
| Workers Paid | $5/月 | $5 |
| D1 Paid | $5/月 | $5 |
| KV Paid | $5/月 | $5 |
| R2 | 20GB | $9 |
| Access | 10 用户 | $30 |
| **总计** | | **~$54/月** |

---

## 十一、安全考量

1. **JWT 签名**: 使用 EdDSA 或 RS256
2. **API Key**: 使用 UUID v4，定期轮换
3. **敏感数据**: 密码使用 bcrypt/scrypt 哈希
4. **审计日志**: 记录所有敏感操作
5. **Rate Limiting**: 在 Workers 层实现
6. **CORS**: 严格限制来源域名

---

## 十二、监控与告警

```typescript
// 集成 Cloudflare Analytics
export async function logAnalytics(event: string, data: object) {
  await fetch('https://api.anixops.dev/analytics', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      event,
      data,
      timestamp: new Date().toISOString(),
    }),
  })
}

// 错误追踪
export async function trackError(error: Error, context: object) {
  // 发送到 Sentry 或 Cloudflare Analytics Engine
  console.error({
    error: error.message,
    stack: error.stack,
    context,
  })
}
```