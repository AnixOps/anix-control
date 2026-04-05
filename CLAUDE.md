# V2Board AnixOps 项目架构文档

## 项目概览

本系统由三个主要组件构成：

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户/管理员                               │
│                           ↓                                      │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              v2board_AnixOps (面板端)                    │    │
│  │  ┌─────────────────┐    ┌─────────────────────────────┐ │    │
│  │  │  Vue 3 前端      │ ←→ │  Go 后端 API 服务           │ │    │
│  │  │  (web/ 目录)     │    │  (internal/ 目录)           │ │    │
│  │  └─────────────────┘    └─────────────────────────────┘ │    │
│  └─────────────────────────────────────────────────────────┘    │
│                           ↓ API 通信                            │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              V2bX_AnixOps (节点端)                       │    │
│  │  - 代理服务 (VMess/VLESS/Trojan/SS/Hysteria2/TUIC)      │    │
│  │  - 流量统计上报                                          │    │
│  │  - 用户在线状态管理                                       │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 一、v2board_AnixOps (面板管理端)

**路径**: `C:\Users\z7299\Documents\GitHub\v2board_AnixOps`

### 1.1 技术栈

| 组件 | 技术 | 版本 |
|------|------|------|
| **后端语言** | Go | 1.24.0 |
| **Web 框架** | Gin | v1.10.0 |
| **ORM** | GORM | v1.25.12 |
| **数据库** | SQLite (默认) / PostgreSQL | - |
| **缓存** | 内存缓存 / Redis | - |
| **认证** | JWT (golang-jwt/jwt/v5) | - |
| **前端框架** | Vue 3 | ^3.4.0 |
| **前端构建** | Vite | ^5.0.0 |
| **状态管理** | Pinia | ^2.1.7 |
| **路由** | Vue Router | ^4.2.5 |
| **HTTP 客户端** | Axios | ^1.13.2 |
| **国际化** | vue-i18n | ^9.9.0 |

### 1.2 项目结构

```
v2board_AnixOps/
├── cmd/
│   ├── server/main.go          # 主程序入口
│   └── subtest/main.go         # 订阅测试工具
├── config/
│   ├── config.yaml             # 当前配置
│   ├── config.yaml.example     # 配置模板
│   └── config.prod.yaml        # 生产环境配置
├── data/                       # SQLite 数据库目录
├── internal/                   # Go 后端代码
│   ├── cache/                  # 缓存层
│   ├── config/                 # 配置加载
│   ├── database/               # 数据库连接
│   ├── handler/                # HTTP 处理器
│   ├── middleware/             # Gin 中间件
│   ├── model/                  # GORM 数据模型
│   ├── parser/                 # 订阅格式解析器
│   ├── router/                 # 路由配置
│   ├── service/                # 业务逻辑层
│   └── utils/                  # 工具函数
├── public/                     # 前端构建输出
├── web/                        # Vue 前端源代码
│   ├── src/
│   │   ├── api/               # API 调用模块
│   │   ├── layouts/           # 布局组件
│   │   ├── router/            # 路由配置
│   │   ├── stores/            # Pinia 状态管理
│   │   ├── utils/             # 工具函数
│   │   └── views/             # 页面组件
│   ├── package.json
│   └── vite.config.js
└── docker-compose.yml
```

### 1.3 后端核心模块

| 模块 | 文件位置 | 功能描述 |
|------|----------|----------|
| **用户系统** | `internal/model/user.go` | 用户注册/登录/资料/权限管理 |
| **套餐系统** | `internal/model/plan.go` | 套餐定义、计费周期、流量限制 |
| **订单系统** | `internal/model/order.go` | 订单创建、支付、状态管理 |
| **节点系统** | `internal/model/node.go` | 物理节点、协议配置、分组管理 |
| **订阅系统** | `internal/model/subscription.go` | 订阅分组、模板、格式解析 |
| **支付系统** | `internal/model/payment.go` | 支付配置、支付日志 |
| **工单系统** | `internal/model/ticket.go` | 用户工单、消息往来 |
| **优惠券系统** | `internal/model/coupon.go` | 优惠券创建、使用记录 |
| **知识库** | `internal/model/knowledge.go` | 文章分类、内容管理 |

### 1.4 数据库表结构

| 表名 | 说明 |
|------|------|
| `v2_user` | 用户表 (email, password, token, uuid, balance, transfer_enable 等) |
| `v2_plan` | 套餐表 (name, price, transfer_enable, speed_limit, device_limit 等) |
| `v2_order` | 订单表 (trade_no, user_id, plan_id, status, amount 等) |
| `v2_payment` | 支付配置 |
| `v2_payment_log` | 支付日志 |
| `v2_node` | 物理节点 (name, host, port, status, auto_register 等) |
| `v2_node_protocol` | 节点协议配置 (支持多种协议) |
| `v2_node_group` | 节点分组 |
| `v2_authorized_key` | 节点自动注册授权密钥 |
| `v2_subscription_group` | 订阅分组 |
| `v2_subscription_template` | 订阅模板 |
| `v2_server_log` | 流量日志 |
| `v2_online_log` | 在线日志 |
| `v2_stat_user` | 用户统计 |
| `v2_stat_server` | 服务器统计 |
| `v2_ticket` | 工单 |
| `v2_ticket_message` | 工单消息 |
| `v2_coupon` | 优惠券 |
| `v2_coupon_usage` | 优惠券使用记录 |
| `v2_knowledge` | 知识库文章 |

### 1.5 API 路由结构

#### 公开接口
```
GET  /health                      # 健康检查
GET  /s/:token                    # 用户订阅 (支持格式自动检测)
```

#### 用户 API (`/api/v2`)
```
POST /login                       # 登录
POST /register                    # 注册
GET  /user/profile                # 用户资料
GET  /user/dashboard              # 仪表盘
GET  /user/subscription           # 订阅信息
GET  /user/plan                   # 套餐列表
GET  /user/knowledge              # 知识库
GET  /user/ticket                 # 工单列表
POST /user/ticket                 # 创建工单
GET  /user/order                  # 订单列表
POST /user/order/save             # 保存订单
POST /user/coupon/check           # 验证优惠券
```

#### 管理员 API (`/api/v2/admin`)
```
GET  /admin/dashboard             # 管理仪表盘

# 用户管理
GET/POST    /admin/users          # 用户列表/创建
GET/PUT/DEL /admin/users/:id      # 用户详情/更新/删除
POST        /admin/users/:id/ban  # 禁用用户
POST        /admin/users/:id/unban # 解禁用户

# 节点管理
GET/POST    /admin/nodes          # 节点列表/创建
GET/PUT/DEL /admin/nodes/:id      # 节点详情/更新/删除
GET/POST    /admin/nodes/:id/protocols  # 协议管理
POST        /admin/auth-keys      # 生成授权密钥

# 订阅管理
GET/POST    /admin/subscription/groups   # 分组列表/创建
GET/PUT/DEL /admin/subscription/groups/:id # 分组管理

# 套餐管理
GET/POST    /admin/plans          # 套餐列表/创建
GET/PUT/DEL /admin/plans/:id      # 套餐详情/更新/删除

# 工单/优惠券/知识库管理
GET/POST    /admin/ticket         # 工单管理
GET/POST    /admin/coupon         # 优惠券管理
GET/POST    /admin/knowledge      # 知识库管理
```

#### 节点通信 API
```
POST /api/v2/node/register        # 节点自动注册
POST /api/v2/node/heartbeat       # 节点心跳
GET  /api/v2/server/UniProxy/config  # 获取节点配置
GET  /api/v2/server/UniProxy/user    # 获取用户列表
POST /api/v2/server/UniProxy/push    # 上报流量
POST /api/v2/server/UniProxy/alive   # 上报在线状态
```

### 1.6 前端页面结构

```
web/src/
├── views/
│   ├── Login.vue                 # 登录页
│   ├── user/                     # 用户端页面
│   │   ├── Dashboard.vue         # 用户仪表盘
│   │   ├── Subscribe.vue         # 订阅管理
│   │   ├── Plans.vue             # 套餐列表
│   │   ├── Orders.vue            # 订单列表
│   │   ├── Tickets.vue           # 工单系统
│   │   └── Knowledge.vue         # 知识库
│   └── admin/                    # 管理端页面
│       ├── Dashboard.vue         # 管理仪表盘
│       ├── Users.vue             # 用户管理
│       ├── Nodes.vue             # 节点管理
│       ├── Orders.vue            # 订单管理
│       ├── Subscriptions.vue     # 订阅管理
│       ├── Plans.vue             # 套餐管理
│       ├── Tickets.vue           # 工单管理
│       ├── Coupons.vue           # 优惠券管理
│       └── Knowledge.vue         # 知识库管理
├── layouts/
│   ├── UserLayout.vue            # 用户端布局
│   └── AdminLayout.vue           # 管理端布局
├── api/
│   ├── auth.js                   # 认证 API
│   ├── user.js                   # 用户 API
│   └── admin.js                  # 管理员 API
├── stores/
│   └── user.js                   # 用户状态 (Pinia)
└── router/
    └── index.js                  # 路由配置
```

### 1.7 配置文件示例

```yaml
# config/config.yaml
env: "development"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"

tls:
  enable: false
  domain: "localhost"

frontend:
  enable: true
  port: 3000
  path: "public"

database:
  driver: "sqlite"
  database: "data/v2board.db"

cache:
  driver: "memory"  # memory 或 redis

jwt:
  secret: "your-jwt-secret-key"
  expire: 86400

app:
  name: "V2Board"
  version: "2.0.0"
  api_token: ""           # 历史兼容字段（UniProxy 已改为节点 api_key + X-API-Key）
  subscribe_path: "s"     # 订阅路径
```

---

## 二、V2bX_AnixOps (节点端程序)

**路径**: `C:\Users\z7299\Documents\GitHub\V2bX_AnixOps`

### 2.1 技术栈

| 类别 | 技术 |
|------|------|
| **编程语言** | Go 1.25 |
| **代理核心** | Xray-core, Sing-box, Hysteria2 |
| **HTTP 客户端** | go-resty/resty |
| **CLI 框架** | spf13/cobra |
| **配置管理** | spf13/viper |
| **日志** | sirupsen/logrus |
| **证书** | go-acme/lego |

### 2.2 项目结构

```
V2bX_AnixOps/
├── main.go                       # 程序入口
├── cmd/                          # 命令行接口
│   ├── cmd.go                    # Cobra 命令定义
│   ├── server.go                 # 服务器启动逻辑
│   └── version.go                # 版本信息
├── api/panel/                    # 面板 API 客户端
│   ├── panel.go                  # API 客户端结构
│   ├── node.go                   # 节点配置获取
│   ├── user.go                   # 用户管理 API
│   ├── register.go               # 节点注册 API
│   └── sync.go                   # 同步消息类型
├── conf/                         # 配置结构定义
│   ├── conf.go                   # 主配置结构
│   ├── node.go                   # 节点配置
│   ├── xray.go                   # Xray 内核配置
│   ├── sing.go                   # Sing-box 内核配置
│   └── hy.go                     # Hysteria 配置
├── core/                         # 核心抽象层
│   ├── core.go                   # 核心接口定义
│   ├── interface.go              # 通用接口
│   ├── selector.go               # 多内核选择器
│   ├── xray/                     # Xray 内核实现
│   ├── sing/                     # Sing-box 内核实现
│   └── hy2/                      # Hysteria2 实现
├── node/                         # 节点控制器
│   ├── controller.go             # 节点控制逻辑
│   ├── node.go                   # 节点管理
│   ├── task.go                   # 定时任务
│   ├── sync.go                   # 配置同步
│   └── user.go                   # 用户管理
├── limiter/                      # 限流器
│   ├── limiter.go                # 限流实现
│   ├── dynamic.go                # 动态限速
│   └── rule.go                   # 规则限流
└── common/                       # 通用工具
    ├── counter/                  # 流量统计
    ├── crypt/                    # 加密工具
    └── monitor/                  # 系统监控
```

### 2.3 支持的协议

| 协议 | TLS | Reality | WebSocket | gRPC | 状态 |
|------|-----|---------|-----------|------|------|
| VMess | ✅ | ✅ | ✅ | ✅ | 支持 |
| VLESS | ✅ | ✅ | ✅ | ✅ | 支持 |
| Trojan | ✅ | ❌ | ✅ | ✅ | 支持 |
| Shadowsocks | ❌ | ❌ | ❌ | ❌ | 支持 |
| Hysteria 1 | ✅ | ❌ | ❌ | ❌ | 支持 |
| Hysteria 2 | ✅ | ❌ | ❌ | ❌ | 支持 |
| TUIC | ✅ | ❌ | ❌ | ❌ | 支持 |
| AnyTLS | ✅ | ❌ | ❌ | ❌ | 支持 |

### 2.4 与面板通信 API

| 端点 | 方法 | 用途 |
|------|------|------|
| `/api/v2/server/UniProxy/config` | GET | 获取节点配置 |
| `/api/v2/server/UniProxy/user` | GET | 获取用户列表 |
| `/api/v2/server/UniProxy/push` | POST | 上报用户流量 |
| `/api/v2/server/UniProxy/alive` | POST | 上报用户在线 IP |
| `/api/v2/node/register` | POST | 节点自动注册 |
| `/api/v2/node/heartbeat` | POST | 发送心跳 |

### 2.5 核心工作流程

```
1. 读取配置文件
2. 初始化内核 (Xray/Sing-box/Hysteria2)
3. 节点注册 (如果启用 AutoRegister)
4. 拉取节点配置
5. 拉取用户列表
6. 在内核中添加 inbound 和用户
7. 启动定时任务:
   - 配置同步 (pull_interval)
   - 流量上报 (push_interval)
   - 心跳发送 (heartbeat_interval)
   - 证书续签
```

### 2.6 配置文件示例

```json
{
  "Log": { "Level": "info" },
  "Cores": [
    {
      "Type": "sing",
      "Log": { "Level": "info", "Timestamp": true }
    }
  ],
  "Nodes": [
    {
      "Core": "sing",
      "ApiHost": "https://panel.example.com",
      "ApiKey": "your-api-key",
      "NodeID": 1,
      "Timeout": 30,
      "ListenIP": "0.0.0.0",
      "CertConfig": {
        "CertMode": "self",
        "CertDomain": "example.com"
      }
    }
  ]
}
```

### 2.7 关键特性

- **多内核支持**: 可同时运行 Xray、Sing-box、Hysteria2
- **单实例多节点**: 一个进程管理多个代理节点
- **自动证书**: Let's Encrypt 自动申请和续签
- **动态限速**: 按用户、按时段的速率限制
- **IP 限制**: 单用户在线设备数限制
- **跨节点 IP 限制**: 用户跨节点重复登录限制
- **自动发现**: 节点通过 AuthKey 自动注册到面板

---

## 三、前后端交互方式

### 3.1 认证方式

- **JWT Token**: 用户登录后返回 token，后续请求通过 `Authorization: Bearer <token>` 携带
- **订阅 Token**: 用户专属的订阅令牌，通过 URL 路径 `/s/:token` 访问

### 3.2 订阅获取

客户端根据 User-Agent 自动识别格式：

| User-Agent | 输出格式 |
|------------|----------|
| Clash/Stash | Clash YAML |
| Surge | Surge 配置 |
| Shadowrocket | Shadowrocket 配置 |
| V2RayNG/V2RayN | V2Ray Base64 |
| Sing-box | Sing-box JSON |
| 其他 | V2Ray Base64 |

### 3.3 订阅响应头

```http
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Disposition: attachment; filename=subscription.txt
Subscription-Userinfo: upload=0; download=1073741824; total=10737418240; expire=1710316800
Profile-Update-Interval: 24
```

---

## 四、开发指南

### 4.1 后端开发

```bash
# 运行开发服务器
go run cmd/server/main.go

# 构建生产版本
go build -o v2board cmd/server/main.go
```

### 4.2 前端开发

```bash
cd web
npm install
npm run dev      # 开发模式
npm run build    # 构建生产版本
```

### 4.3 节点端开发

```bash
# 运行节点
go run main.go

# 构建生产版本
go build -o V2bX main.go
```

---

## 五、部署架构

```
┌──────────────────────────────────────────────────────────────┐
│                      生产环境部署                             │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────┐                                         │
│  │   Nginx/Caddy   │  ← 反向代理 + TLS 终止                   │
│  └────────┬────────┘                                         │
│           │                                                  │
│  ┌────────┴────────┐                                         │
│  │  v2board (Go)   │  ← 面板 API + 前端静态文件               │
│  │  Port: 8080     │                                         │
│  └────────┬────────┘                                         │
│           │                                                  │
│  ┌────────┴────────┐                                         │
│  │   SQLite/PG     │  ← 数据库                               │
│  └─────────────────┘                                         │
│                                                              │
│  ┌─────────────────┐    ┌─────────────────┐                  │
│  │  V2bX Node 1    │    │  V2bX Node 2    │                  │
│  │  (代理服务)      │    │  (代理服务)      │                  │
│  └────────┬────────┘    └────────┬────────┘                  │
│           │                      │                           │
│           └──────────┬───────────┘                           │
│                      ↓                                       │
│              面板 API 通信                                    │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 六、Git 分支信息

**当前分支**: `go_dev`
**主分支**: `go_dev`

**当前修改的文件**:
- `internal/handler/admin.go`
- `internal/handler/subscribe.go`
- `internal/parser/formatter.go`
- `internal/parser/parser.go`
- `internal/service/init_subscription.go`
- `internal/service/order_service.go`
- `internal/service/plan_service.go`
- `internal/service/server_service.go`

---

*文档生成时间: 2026-03-12*

---

## 七、当前开发进度 (2026-03-12 更新)

### 7.1 已完成的修改

#### 面板端 (v2board_AnixOps)

**修改文件列表**:
- `internal/handler/admin.go` - 流量计算修正
- `internal/handler/subscribe.go` - 添加 `.conf` 扩展名支持 Surge 格式
- `internal/handler/uniproxy.go` - 添加 `type` 字段与 `node_type` 同步
- `internal/middleware/middleware.go` - 添加调试日志
- `internal/parser/formatter.go` - 新增 `SurgeFormatter` 格式化器
- `internal/parser/parser.go` - 注册 Surge 格式化器
- `internal/service/init_subscription.go` - 流量单位修正
- `internal/service/order_service.go` - 流量计算修正 (使用 `1073741824` = 1GB)
- `internal/service/plan_service.go` - 流量计算修正
- `internal/service/server_service.go` - 添加 `type` 字段和调试日志

**关键修复**:
1. **流量计算单位**: 从 `1024 * 1024 * 1024` 改为 `1073741824` (精确的 1GB = 2^30 字节)
2. **Surge 订阅格式**: 新增完整的 Surge 格式化器，支持 VMess/VLESS/Trojan/Shadowsocks
3. **节点 API type 字段**: 确保 `/api/v2/server/UniProxy/config` 返回 `type` 和 `node_type` 两个字段

#### 节点端 (V2bX_AnixOps)

**修改文件列表**:
- `api/panel/panel.go` - 添加 `ProtocolType` 字段
- `api/panel/node.go` - 分离 `node_type` 和 `type` 的解析逻辑

**关键修复**:
1. **node_type 和 type 分离**:
   - `node_type` = 节点分类 (如 "node", "vmess")
   - `type` = 具体协议 (如 "vless", "vmess", "trojan")
2. **兼容逻辑**: 如果其中一个为空，尝试用另一个补充

### 7.2 API 响应示例

**节点配置 API** (`GET /api/v2/server/UniProxy/config?node_id=1 (Header: X-API-Key: <api_key>)`):
```json
{
  "base_config": {"pull_interval": 60, "push_interval": 60},
  "cipher": "aes-256-gcm",
  "host": "127.0.0.1",
  "network": "tcp",
  "network_settings": {},
  "node_type": "shadowsocks",
  "type": "shadowsocks",
  "routes": [],
  "send_through": "0.0.0.0",
  "server_name": "127.0.0.1",
  "server_port": 8388,
  "tls": 0,
  "tls_settings": {}
}
```

### 7.3 待完成事项

1. **移除调试代码**: 清理 `fmt.Printf` 和调试日志文件写入
2. **测试节点端连接**: 使用 V2bX_AnixOps 连接面板端进行集成测试
3. **前端构建**: `cd web && npm run build`
4. **提交代码**: 两边项目分别提交修改

### 7.4 启动命令

```bash
# 面板端
cd C:/Users/z7299/Documents/GitHub/v2board_AnixOps
go build -o v2board.exe cmd/server/main.go
./v2board.exe

# 节点端 (需要 GOEXPERIMENT=jsonv2)
cd C:/Users/z7299/Documents/GitHub/V2bX_AnixOps
GOEXPERIMENT=jsonv2 go build -o V2bX.exe main.go
./V2bX.exe server -c config.json
```

### 7.5 数据库位置

- 面板端数据库: `data/v2board.db` (SQLite)
- 节点端测试数据: Node ID=1, 有 shadowsocks 和 vless 两个协议配置

### 7.6 测试 API 端点

```bash
# 健康检查
curl http://localhost:8080/health

# 节点配置
curl -H "X-API-Key: your_node_api_key" "http://localhost:8080/api/v2/server/UniProxy/config?node_id=1"

# 用户列表
curl -H "X-API-Key: your_node_api_key" "http://localhost:8080/api/v2/server/UniProxy/user?node_id=1"
```

---

## 八、gRPC 服务架构 (2026-03-13 更新)

### 8.1 概述

gRPC 服务用于面板端与节点端之间的高效通信，支持双向流式传输，实现实时配置推送和状态上报。

### 8.2 目录结构

```
v2board_AnixOps/
├── api/grpc/v2boardpb/           # Protobuf 生成的代码
│   ├── v2board.pb.go             # 消息定义
│   └── v2board_grpc.pb.go        # 服务接口
├── internal/grpc/                # gRPC 服务实现
│   ├── server.go                 # gRPC 服务器封装
│   ├── node_server.go            # 节点/用户/流量/健康服务实现
│   ├── interceptor.go            # 认证/日志拦截器 + 连接管理器
│   └── grpc_test.go              # 测试套件

V2bX_AnixOps/
├── api/grpc/                     # gRPC 客户端
│   ├── client.go                 # 完整的 gRPC 客户端实现
│   └── v2boardpb/                # 同步的 protobuf 代码
```

### 8.3 服务定义

| 服务 | 方法 | 类型 | 说明 |
|------|------|------|------|
| **NodeService** | Register | Unary | 节点注册 |
| | GetConfig | Unary | 获取节点配置 |
| | ReportStatus | Unary | 上报节点状态 |
| | StatusStream | Bidirectional | 状态实时通信 |
| **UserService** | GetUsers | Unary | 获取用户列表 |
| | UserChanges | Bidirectional | 用户变更通知 |
| **TrafficService** | ReportTraffic | Unary | 批量上报流量 |
| | ReportOnline | Unary | 上报在线状态 |
| | TrafficStream | Bidirectional | 实时流量上报 |
| | OnlineStream | Bidirectional | 实时在线状态 |
| **HealthService** | Check | Unary | 健康检查 |
| | Watch | Bidirectional | 持续健康检查 |

### 8.4 消息类型

```protobuf
// 节点注册
NodeRegisterRequest { auth_key, name, host, port, server_version, server_os }
NodeRegisterResponse { node_id, api_key, secret, message }

// 节点配置
NodeConfigRequest { node_id }
NodeConfigResponse { host, server_port, type, network, tls, base_config, ... }

// 用户信息
UserInfo { id, uuid, speed_limit, device_limit, transfer_enable, used_upload, used_download }
UserListRequest { node_id }
UserListResponse { users[], total, updated_at }

// 流量上报
TrafficData { upload, download }
TrafficReportRequest { node_id, traffics map[uint32]TrafficData }
TrafficReportResponse { success, message }

// 在线状态
OnlineData { ips[], timestamp }
OnlineReportRequest { node_id, online map[uint32]OnlineData }

// 健康检查
HealthCheckRequest { node_id }
HealthCheckResponse { status, server_version, timestamp }
```

### 8.5 连接管理器

```go
// 跟踪活跃节点连接
type NodeConnectionManager struct {
    connections map[uint32]*NodeConnection  // node_id -> connection
    configVer   map[uint32]int64            // 配置版本追踪
}

// 主要方法
Register(nodeID, addr)           // 注册节点连接
Unregister(nodeID)               // 注销节点连接
UpdateLastSeen(nodeID)           // 更新活跃时间
GetConnection(nodeID)            // 获取连接信息
GetActiveNodes() []uint32        // 获取所有活跃节点
IsConfigChanged(nodeID, ver)     // 检查配置是否变更
```

### 8.6 拦截器

| 拦截器 | 类型 | 功能 |
|--------|------|------|
| `AuthInterceptor` | Unary | Token 认证 (健康检查豁免) |
| `StreamAuthInterceptor` | Stream | 流式 Token 认证 |
| `LoggingInterceptor` | Unary | 请求日志记录 |
| `StreamLoggingInterceptor` | Stream | 流式请求日志 |

### 8.7 客户端使用 (V2bX_AnixOps)

```go
// 创建客户端
client, err := grpc.NewGRPCClient(&grpc.GRPCClientConfig{
    Host:          "localhost:50051",
    NodeID:        1,
    APIKey:        "your-api-key",
    KeepaliveTime: 30 * time.Second,
})

// 节点注册
credential, err := client.Register(authKey, name, host, port, version, os)

// 获取配置
nodeInfo, err := client.GetNodeConfig()

// 获取用户
users, err := client.GetUsers()

// 上报流量
err := client.ReportTraffic([]panel.UserTraffic{...})

// 上报在线状态
err := client.ReportOnline(map[int][]string{userID: {ip1, ip2}})

// 启动双向流
err := client.StartStreams()

// 通过流发送状态
err := client.SendStatus(stats, onlineUsers, upload, download)
```

### 8.8 服务器启动

```go
// 面板端启动 gRPC 服务
server := grpc.NewServer(&grpc.ServerConfig{
    Host:             "0.0.0.0",
    Port:             50051,
    APIToken:         "your-api-token",
    KeepaliveTime:    30 * time.Second,
    KeepaliveTimeout: 10 * time.Second,
})
server.Start()
defer server.Stop()
```

### 8.9 测试

```bash
# 运行 gRPC 测试
cd v2board_AnixOps
go test -v ./internal/grpc/...

# 运行带覆盖率的测试
go test -cover ./internal/grpc/...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./internal/grpc/...
go tool cover -html=coverage.out
```

### 8.10 Protobuf 重新生成

```bash
# 安装 protoc (Windows)
# 下载 https://github.com/protocolbuffers/protobuf/releases

# 安装 Go 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 生成代码
protoc --go_out=. --go-grpc_out=. api/grpc/v2board.proto
```

---

## 九、开发进度记录

### 2026-03-13: gRPC 实现完成

**已完成**:
- [x] Protobuf 定义和代码生成
- [x] 面板端 gRPC 服务实现 (Node/User/Traffic/Health)
- [x] 节点端 gRPC 客户端实现
- [x] 认证和日志拦截器
- [x] 连接管理器
- [x] 测试套件 (基础测试 + 集成测试)
- [x] Heartbeat 函数修复 (返回节点不存在错误)

**测试覆盖**:
- TestHealthCheck: 健康检查
- TestGetNodeConfig_NotFound: 节点不存在场景
- TestGetUsers_NodeNotFound: 用户列表节点不存在
- TestReportTraffic_NodeNotFound: 流量上报节点不存在
- TestReportOnline_NodeNotFound: 在线状态节点不存在
- TestReportStatus_NodeNotFound: 状态上报节点不存在
- TestConnectionManager: 连接管理器功能
- TestGetPeerAddr: 客户端地址获取
- TestNodeIDContext: 上下文节点ID
- TestNodeRegistrationFlow: 完整注册流程
- TestConfigWithProtocol: 协议配置
- TestUsersWithPlan: 带套餐的用户
- TestTrafficReportWithRate: 流量倍率测试

---

## 十、Flux Panel 复刻规范 (2026-04-05)

后续继续复刻 `flux-panel` 时，默认把“源码一比一兼容”作为最高优先级，不允许先按本项目习惯自行发明接口或交互，再事后回调。

### 10.1 参考仓库

- 上游仓库: `https://github.com/bqlpfy/flux-panel`
- 当前本机参考副本: `C:\Users\z7299\AppData\Local\Temp\flux-panel`
- 后端源码优先参考:
  - `springboot-backend/src/main/java/com/admin/controller/`
  - `springboot-backend/src/main/java/com/admin/service/impl/`
  - `springboot-backend/src/main/java/com/admin/common/dto/`
  - `springboot-backend/src/main/java/com/admin/entity/`
- 前端源码优先参考:
  - `vite-frontend/src/pages/`
  - `vite-frontend/src/components/`
  - `vite-frontend/src/api/`

### 10.2 复刻优先级

1. 路径、方法、鉴权范围与请求体字段名必须先对齐。
2. 返回包结构必须对齐，尤其是 `code`、`msg`、`ts`、`data` 以及 DTO 字段大小写。
3. 页面结构、按钮文案、弹窗流程、排序/批量/诊断等交互要与参考页面一致。
4. 业务语义再向下对齐，包括权限关系、限额校验、运行时状态变更、诊断逻辑和副作用。
5. 只有在本仓库架构无法直接承接时，才允许保留兼容镜像路由或过渡实现，但必须在文档里明确写出差距。

### 10.3 强制规则

- 不要把 `flux-panel` 的用户接口随手挂到 `/api/v2/admin/*`。
  - 先看参考控制器是否有管理员角色限制。
  - 如果参考接口是登录用户接口，本仓库必须提供同等用户态入口。
- 不要删减参考 DTO 中“当前页面暂时没用到”的字段。
  - 这些字段通常会被后续页面、导入导出、诊断或联动流程依赖。
- 不要只复刻 UI，不复刻 API 语义。
  - 页面完成后，必须回查 controller/service/entity/dto，确认不是“长得像，但行为不一样”。
- 不要把运行时能力缺失伪装成“已完成复刻”。
  - 如果当前只完成 DB 兼容层，而参考实现还包含 Gost/节点/远程副作用，文档里必须显式标注为待补齐。
- 优先复用参考实现中的授权关系模型。
  - 例如参考仓库有 `UserTunnel` 时，本仓库优先使用显式关联表，而不是临时塞进 `group_id` 或模糊条件判断。

### 10.4 当前已完成的 Flux 复刻基础

- 流量转发页面已按参考页复刻到:
  - `web/src/views/admin/Forward.vue`
- 兼容 API 入口已建立:
  - `/api/v2/forward/*`
  - `/api/v2/tunnel/user/tunnel`
  - 保留 `/api/v2/admin/forward/*` 与 `/api/v2/admin/tunnel/user/tunnel` 作为现有管理后台兼容镜像
- 当前转发授权关系模型:
  - `internal/model/forward_panel.go` 中的 `ForwardUserTunnel`
- 当前关键实现文件:
  - `internal/service/forward_panel_service.go`
  - `internal/handler/forward_panel.go`
  - `internal/router/router.go`
  - `web/src/api/admin.js`
  - `internal/service/forward_panel_service_test.go`

### 10.5 后续复刻时的检查清单

- 逐个对比参考仓库的 controller、service、dto、entity 和前端 page/api 文件。
- 确认路由作用域:
  - 管理员接口是否真的需要 `AdminAuth`
  - 用户接口是否已经暴露在 JWT 用户组下
- 确认返回结构:
  - 包装层字段
  - DTO 字段名
  - 可空字段
  - 时间戳单位
- 确认行为差异:
  - create/update/delete 是否只改库，还是要联动远端运行时
  - pause/resume 是否只改状态，还是要联动远端服务
  - diagnose 是否本地拨测，还是应按节点路径执行
- 补齐自动化验证:
  - service 测试
  - handler/router 测试
  - 前端构建
- 完成后更新:
  - `docs/guide/flux-panel-clone.md`
  - `README.md` 中的说明入口

### 10.6 默认验证命令

```bash
$env:GOWORK='off'; go test ./internal/router ./internal/handler ./internal/service
cd web && npm run build
```

更详细的模块映射、当前完成度和下一步待补项目，见 `docs/guide/flux-panel-clone.md`。
