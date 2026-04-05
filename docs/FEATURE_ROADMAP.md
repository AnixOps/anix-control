# V2Board AnixOps 功能规划

## 一、现有功能 vs 市面最佳对比

### 1.1 功能对比矩阵

| 功能模块 | V2Board原版 | XBoard | Marzban | **当前项目** | 优先级 |
|----------|-------------|--------|---------|--------------|--------|
| **用户系统** |
| 用户注册/登录 | ✅ | ✅ | ✅ | ✅ | - |
| 多因素认证(MFA) | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 用户等级系统 | ✅ | ✅ | ✅ | ❌ | 🟡 中 |
| 邀请返利系统 | ✅ | ✅ | ❌ | ❌ | 🟡 中 |
| 用户备注/标签 | ✅ | ✅ | ✅ | ❌ | 🟢 低 |
| **节点系统** |
| 多节点管理 | ✅ | ✅ | ✅ | ✅ | - |
| 节点分组 | ✅ | ✅ | ✅ | ✅ | - |
| 节点负载均衡 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 节点健康检查 | ✅ | ✅ | ✅ | 部分 | 🟡 中 |
| 中转节点管理 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 节点自动部署 | ❌ | ❌ | ✅ | ❌ | 🟡 中 |
| **订阅系统** |
| 多格式订阅 | ✅ | ✅ | ✅ | ✅ | - |
| 订阅自动更新 | ✅ | ✅ | ✅ | 部分 | 🟡 中 |
| 自定义订阅规则 | ✅ | ✅ | ❌ | ❌ | 🟡 中 |
| 订阅统计 | ❌ | ✅ | ❌ | ❌ | 🟢 低 |
| **支付系统** |
| 支付宝/微信 | ✅ | ✅ | ❌ | ❌ | 🔴 高 |
| Stripe | ✅ | ✅ | ❌ | ❌ | 🟡 中 |
| USDT加密支付 | ✅ | ✅ | ❌ | ❌ | 🟡 中 |
| 余额系统 | ✅ | ✅ | ❌ | ✅ | - |
| 订单管理 | ✅ | ✅ | ❌ | ✅ | - |
| **通知系统** |
| 邮件通知 | ✅ | ✅ | ✅ | ❌ | 🟡 中 |
| Telegram Bot | ❌ | ❌ | ✅ | ❌ | 🔴 高 |
| 企业微信/钉钉 | ❌ | ❌ | ❌ | ❌ | 🟡 中 |
| Webhook | ❌ | ❌ | ✅ | ❌ | 🟡 中 |
| **流量转发** |
| 中转节点管理 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 端口转发规则 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 智能路由 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| 转发流量统计 | ❌ | ❌ | ❌ | ❌ | 🔴 高 |
| **高级功能** |
| 自动化运维 | ❌ | ❌ | ✅ | ❌ | 🟡 中 |
| 数据备份 | ❌ | ✅ | ✅ | ❌ | 🟡 中 |
| API接口 | ✅ | ✅ | ✅ | ✅ | - |
| gRPC支持 | ❌ | ❌ | ❌ | ✅ | - |
| 多语言支持 | ✅ | ✅ | ✅ | 部分 | 🟢 低 |

### 1.2 核心差距分析

#### 🔴 高优先级缺失功能

1. **流量转发/中转系统** - 市面面板普遍缺失
2. **Telegram Bot** - 用户通知和客服
3. **多因素认证** - 账户安全
4. **支付网关** - 商业化必需
5. **节点负载均衡** - 提升用户体验

---

## 二、流量转发面板功能设计

### 2.1 架构设计

Current clone note (`2026-04-05`):

- dual-runtime abstraction exists locally for `gost` and `iptables_ansible`
- runtime backend selection and runtime job observability live under the admin system page
- pending `iptables_ansible` runtime jobs are now executed by an in-process worker started with the server
- this extension must not change the Flux-shaped forward page while clone work is still in progress

```
┌─────────────────────────────────────────────────────────────────────┐
│                        流量转发架构                                   │
│                                                                      │
│   用户 ──→ 入口节点(中转) ──→ 出口节点(落地) ──→ 目标服务            │
│            (国内/近)         (国外/远)                               │
│                                                                      │
│   ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐     │
│   │ 用户请求 │───→│ 中转节点 │───→│ 落地节点 │───→│ 目标服务 │     │
│   └──────────┘    │ (Relay)  │    │ (Exit)   │    └──────────┘     │
│                   └──────────┘    └──────────┘                      │
│                        │                │                           │
│                        ▼                ▼                           │
│                   ┌─────────────────────────┐                       │
│                   │     面板统一管理         │                       │
│                   │  - 规则配置              │                       │
│                   │  - 流量统计              │                       │
│                   │  - 健康检查              │                       │
│                   │  - 智能路由              │                       │
│                   └─────────────────────────┘                       │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 数据模型设计

```go
// ForwardNode 中转节点
type ForwardNode struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`                    // 节点名称
    Type        string    `json:"type"`                    // relay/exit
    Host        string    `json:"host"`                    // 节点地址
    Port        int       `json:"port"`                    // 管理端口
    Region      string    `json:"region"`                  // 地区
    ISP         string    `json:"isp"`                     // 运营商
    Bandwidth   int64     `json:"bandwidth"`               // 带宽(Mbps)
    Status      int       `json:"status"`                  // 0=离线 1=在线
    LastCheck   time.Time `json:"last_check"`              // 最后检查
    Latency     int       `json:"latency"`                 // 延迟(ms)
    Load        float64   `json:"load"`                    // 负载
    Tags        string    `json:"tags"`                    // 标签(JSON)
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// ForwardRule 转发规则
type ForwardRule struct {
    ID           uint      `gorm:"primaryKey" json:"id"`
    Name         string    `json:"name"`                    // 规则名称
    Enabled      bool      `json:"enabled"`                 // 启用状态

    // 入口配置
    RelayNodeID  uint      `json:"relay_node_id"`           // 中转节点ID
    ListenPort   int       `json:"listen_port"`             // 监听端口
    Protocol     string    `json:"protocol"`                // tcp/udp/both

    // 出口配置
    ExitNodeID   uint      `json:"exit_node_id"`            // 落地节点ID
    TargetHost   string    `json:"target_host"`             // 目标地址
    TargetPort   int       `json:"target_port"`             // 目标端口

    // 用户绑定
    UserID       *uint     `json:"user_id"`                 // 绑定用户(可选)
    UserGroupID  *uint     `json:"user_group_id"`           // 用户组(可选)

    // 流量控制
    SpeedLimit   *int64    `json:"speed_limit"`             // 速度限制(KB/s)
    TrafficLimit *int64    `json:"traffic_limit"`           // 流量限制(字节)
    ExpireTime   *time.Time `json:"expire_time"`            // 过期时间

    // 统计
    Upload       int64     `json:"upload"`                  // 上传流量
    Download     int64     `json:"download"`                // 下载流量
    Connections  int       `json:"connections"`             // 当前连接数

    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

// ForwardRoute 智能路由规则
type ForwardRoute struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`
    Priority    int       `json:"priority"`                // 优先级
    Enabled     bool      `json:"enabled"`

    // 匹配条件
    SourceIP    string    `json:"source_ip"`               // 源IP/网段
    Region      string    `json:"region"`                  // 来源地区
    PortRange   string    `json:"port_range"`              // 端口范围
    Protocol    string    `json:"protocol"`                // 协议类型

    // 目标节点组
    RelayGroup  string    `json:"relay_group"`             // 中转节点组
    ExitGroup   string    `json:"exit_group"`              // 落地节点组

    // 负载均衡策略
    BalanceMode string    `json:"balance_mode"`            // round-robin/least-conn/latency

    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// ForwardLog 转发日志
type ForwardLog struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    RuleID      uint      `json:"rule_id"`
    UserID      *uint     `json:"user_id"`

    SourceIP    string    `json:"source_ip"`
    TargetHost  string    `json:"target_host"`

    Upload      int64     `json:"upload"`
    Download    int64     `json:"download"`
    Duration    int64     `json:"duration"`               // 持续时间(秒)

    RelayNodeID uint      `json:"relay_node_id"`
    ExitNodeID  uint      `json:"exit_node_id"`

    CreatedAt   time.Time `json:"created_at"`
}
```

### 2.3 API 设计

```
# 中转节点管理
GET    /api/v2/admin/forward/nodes           # 节点列表
POST   /api/v2/admin/forward/nodes           # 创建节点
GET    /api/v2/admin/forward/nodes/:id       # 节点详情
PUT    /api/v2/admin/forward/nodes/:id       # 更新节点
DELETE /api/v2/admin/forward/nodes/:id       # 删除节点
POST   /api/v2/admin/forward/nodes/:id/check # 健康检查

# 转发规则管理
GET    /api/v2/admin/forward/rules           # 规则列表
POST   /api/v2/admin/forward/rules           # 创建规则
GET    /api/v2/admin/forward/rules/:id       # 规则详情
PUT    /api/v2/admin/forward/rules/:id       # 更新规则
DELETE /api/v2/admin/forward/rules/:id       # 删除规则
POST   /api/v2/admin/forward/rules/:id/toggle # 启用/禁用

# 智能路由管理
GET    /api/v2/admin/forward/routes          # 路由列表
POST   /api/v2/admin/forward/routes          # 创建路由
PUT    /api/v2/admin/forward/routes/:id      # 更新路由
DELETE /api/v2/admin/forward/routes/:id      # 删除路由

# 统计分析
GET    /api/v2/admin/forward/stats           # 整体统计
GET    /api/v2/admin/forward/stats/:node_id  # 节点统计
GET    /api/v2/admin/forward/logs            # 转发日志

# 用户接口
GET    /api/v2/user/forward/rules            # 用户的转发规则
POST   /api/v2/user/forward/rules            # 用户创建规则(需权限)
```

---

## 三、新增核心功能设计

### 3.1 Telegram Bot 系统

```go
// TelegramBot 配置
type TelegramBotConfig struct {
    Token       string   `json:"token"`        // Bot Token
    AdminIDs    []int64  `json:"admin_ids"`    // 管理员ID
    WelcomeMsg  string   `json:"welcome_msg"`  // 欢迎消息
    Commands    []BotCommand `json:"commands"` // 命令列表
}

// Bot 命令
// /start     - 开始使用
// /bind      - 绑定账户
// /unbind    - 解绑账户
// /info      - 查看账户信息
// /sub       - 获取订阅链接
// /renew     - 续费套餐
// /ticket    - 创建工单
// /help      - 帮助信息
// /admin     - 管理员面板(仅管理员)
```

### 3.2 多因素认证 (MFA)

```go
// MFA 配置
type UserMFA struct {
    ID           uint      `gorm:"primaryKey"`
    UserID       uint      `json:"user_id"`
    Enabled      bool      `json:"enabled"`
    Secret       string    `json:"secret"`        // TOTP密钥
    BackupCodes  string    `json:"backup_codes"`  // 备用码(JSON)
    LastUsed     time.Time `json:"last_used"`
    CreatedAt    time.Time `json:"created_at"`
}

// 支持方式
// - TOTP (Google Authenticator, Authy等)
// - 短信验证码
// - 邮箱验证码
// - Telegram 验证
```

### 3.3 支付网关集成

```go
// PaymentGateway 支付网关
type PaymentGateway struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`           // 名称
    Type        string    `json:"type"`           // alipay/wechat/stripe/usdt
    Enabled     bool      `json:"enabled"`
    Config      string    `json:"config"`         // 配置(JSON)
    FeeRate     float64   `json:"fee_rate"`       // 手续费率
    MinAmount   float64   `json:"min_amount"`     // 最小金额
    MaxAmount   float64   `json:"max_amount"`     // 最大金额
    CreatedAt   time.Time `json:"created_at"`
}

// 支持的支付方式
// 1. 支付宝 - 即时到账/当面付
// 2. 微信支付 - Native/JSAPI
// 3. Stripe - 信用卡
// 4. USDT - TRC20/ERC20
// 5. EPay - 通用支付
```

### 3.4 节点负载均衡

```go
// LoadBalancer 负载均衡器
type LoadBalancer struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`
    GroupID     uint      `json:"group_id"`       // 节点组ID

    Strategy    string    `json:"strategy"`       // round-robin/least-conn/latency/weight
    HealthCheck bool      `json:"health_check"`   // 健康检查
    CheckInterval int      `json:"check_interval"` // 检查间隔(秒)
    CheckTimeout  int      `json:"check_timeout"`  // 超时时间(秒)

    // 节点权重
    NodeWeights string    `json:"node_weights"`   // JSON: {node_id: weight}

    CreatedAt   time.Time `json:"created_at"`
}

// 均衡策略
// - round-robin: 轮询
// - least-conn: 最少连接
// - latency: 最低延迟
// - weight: 加权轮询
// - random: 随机
```

### 3.5 通知系统

```go
// NotificationTemplate 通知模板
type NotificationTemplate struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`
    Type        string    `json:"type"`           // email/telegram/webhook
    Event       string    `json:"event"`          // 触发事件
    Title       string    `json:"title"`          // 标题模板
    Content     string    `json:"content"`        // 内容模板
    Enabled     bool      `json:"enabled"`
    CreatedAt   time.Time `json:"created_at"`
}

// 触发事件
// - user.register    用户注册
// - user.login       用户登录
// - user.expire      用户到期
// - user.traffic_low 流量不足
// - order.paid       订单支付
// - ticket.reply     工单回复
// - node.offline     节点离线
// - node.online      节点上线
```

---

## 四、前端页面规划

### 4.1 用户端新增页面

```
/web/src/views/user/
├── Security.vue         # 安全设置(MFA/密码修改)
├── Telegram.vue         # Telegram 绑定
├── Forward.vue          # 转发规则管理
├── Invite.vue           # 邀请返利
└── Notification.vue     # 通知设置
```

### 4.2 管理端新增页面

```
/web/src/views/admin/
├── Forward/
│   ├── Nodes.vue        # 中转节点管理
│   ├── Rules.vue        # 转发规则管理
│   ├── Routes.vue       # 智能路由配置
│   └── Stats.vue        # 转发统计
├── Payment/
│   ├── Gateways.vue     # 支付网关管理
│   └── Orders.vue       # 订单管理(已有)
├── Notification/
│   ├── Templates.vue    # 通知模板
│   └── Logs.vue         # 通知日志
├── Telegram/
│   ├── Config.vue       # Bot配置
│   └── Commands.vue     # 命令管理
└── Security/
    └── MFA.vue          # MFA设置
```

---

## 五、实现进度

### ✅ 已完成 (2026-03-13)

#### 流量转发系统
- [x] ForwardNode 数据模型 (中转/落地节点)
- [x] ForwardRule 数据模型 (转发规则)
- [x] ForwardRoute 数据模型 (智能路由)
- [x] ForwardLog 数据模型 (转发日志)
- [x] ForwardStats 数据模型 (统计数据)
- [x] ForwardNodeService 节点服务
  - 节点 CRUD
  - 健康检查
  - 负载均衡 (round-robin/latency/least-conn/weight/random)
  - 节点选择算法
- [x] ForwardRuleService 规则服务
  - 规则 CRUD
  - 端口分配
  - 流量统计
  - 用户规则管理
- [x] ForwardHandler API 处理器
  - 管理员节点管理 API
  - 管理员规则管理 API
  - 用户规则 API
  - 统计 API
- [x] 数据库迁移集成
- [x] API 路由注册 (router.go)

#### 支付网关系统
- [x] PaymentGateway 数据模型
- [x] PaymentRecord 数据模型
- [x] 支付宝/微信/Stripe/USDT/EPay 配置结构
- [x] PaymentGatewayService 服务
  - 网关 CRUD
  - 费率计算
  - 支付记录管理
  - 支付状态管理
  - 支付统计
- [x] PaymentGatewayHandler API 处理器
- [x] API 路由注册 (router.go)

#### Telegram Bot 系统
- [x] TelegramBot 配置模型
- [x] TelegramUser 绑定模型
- [x] TelegramChat 聊天记录模型
- [x] TelegramNotification 通知模型
- [x] TelegramBotService 服务
  - Webhook管理
  - 命令处理 (/start, /help, /bind, /unbind, /info, /sub, /ticket, /admin)
  - 消息发送
  - 通知推送
  - 广播消息
  - 用户绑定/解绑
- [x] TelegramHandler API 处理器
- [x] API 路由注册 (router.go)

#### 通知系统
- [x] NotificationTemplate 模板模型
- [x] NotificationLog 日志模型
- [x] NotificationService 服务
  - 邮件发送 (SMTP/TLS)
  - Telegram通知
  - Webhook通知
  - 模板管理
  - 事件通知 (到期/流量/工单/订单/节点)

#### 多因素认证 (MFA)
- [x] UserMFA 数据模型
- [x] MFALoginAttempt 登录尝试模型
- [x] MFAService 服务
  - TOTP设置和验证
  - 备用码生成和验证
  - MFA启用/禁用
  - 暴力破解检测
  - 登录尝试记录
- [x] MFAHandler API 处理器
  - 用户: 状态、设置、启用、禁用、验证、备用码
  - 管理员: 全局配置
- [x] API 路由注册

#### 通知系统
- [x] NotificationTemplate 模板模型
- [x] NotificationLog 日志模型
- [x] NotificationService 服务
  - 邮件发送 (SMTP/TLS)
  - Telegram通知
  - Webhook通知
  - 模板管理
  - 事件通知 (到期/流量/工单/订单/节点)
- [x] NotificationHandler API 处理器
  - 用户: 通知列表、已读标记、未读计数
  - 管理员: 模板管理、日志查看、测试通知、邮件配置
- [x] API 路由注册

### Phase 1: 核心功能 (1-2周) ✅ 已完成
1. ✅ 基础架构已完善
2. ✅ 支付网关集成 (支付宝/微信)
3. ✅ Telegram Bot
4. ✅ 邮件通知系统
5. ✅ API 路由注册完成

### Phase 2: 增强功能 (2-3周) ✅ 后端完成
1. ✅ 多因素认证 (TOTP)
2. ✅ MFA API Handler
3. ✅ 通知系统 Handler
4. 🔲 节点负载均衡
5. ✅ 用户等级系统 (模型完成)
6. ✅ 邀请返利系统 (完整实现)

#### 邀请返利系统
- [x] UserLevel 数据模型
- [x] InviteCode 邀请码模型
- [x] CommissionRecord 佣金记录模型
- [x] CommissionWithdraw 提现模型
- [x] InviteConfig 配置模型
- [x] InviteService 服务
  - 邀请码生成/验证
  - 佣金计算/发放
  - 提现申请/处理
  - 统计数据
- [x] InviteHandler API 处理器
  - 用户: 邀请信息、生成邀请码、佣金记录、提现申请
  - 管理员: 配置管理、提现审核、统计
- [x] API 路由注册
- [x] 数据库迁移

### Phase 3: 流量转发 (2-3周) ✅ 后端完成
1. ✅ 中转节点管理
2. ✅ 转发规则引擎
3. 🔲 智能路由 (ForwardRoute 前端)
4. ✅ 转发流量统计

### Phase 4: 高级功能 ✅ 后端完成
1. ✅ Webhook 通知 (NotificationService 已支持)
2. ✅ 数据备份/恢复
3. ✅ 系统配置管理
4. ✅ 负载均衡器
5. ✅ 操作日志

#### 前端页面 (2026-03-13 完成)
- [x] Forward.vue - 流量转发管理 (节点+规则)
- [x] Payment.vue - 支付网关管理 (网关+记录+统计)
- [x] Telegram.vue - Telegram Bot 管理 (配置+用户+通知)
- [x] MFA.vue - 多因素认证设置
- [x] Notifications.vue - 通知管理 (模板+邮件配置+日志)
- [x] Invite.vue - 邀请返利管理 (配置+提现+统计)
- [x] System.vue - 系统管理 (配置+备份+负载均衡)
- [x] 路由更新 (router/index.js)
- [x] 管理布局更新 (AdminLayout.vue 侧边栏)
- [x] 前端构建成功

#### API 文档与部署 (2026-03-13 完成)
- [x] Swagger/OpenAPI 文档生成 (docs/)
- [x] Swagger UI 集成 (/swagger/index.html)
- [x] Dockerfile 多阶段构建 (包含前端+Swagger)
- [x] docker-compose.yml (开发+生产配置)
- [x] GitHub Actions CI/CD (测试+构建+部署+发布)
- [x] .dockerignore 优化
- [x] Makefile 增强 (swagger 命令)

#### 系统管理功能
- [x] LoadBalancer 负载均衡器模型
- [x] SystemConfig 系统配置模型
- [x] BackupRecord 备份记录模型
- [x] BackupConfig 备份配置模型
- [x] OperationLog 操作日志模型
- [x] LoadBalancerService 服务
  - 多种策略 (round-robin/least-load/latency/weight/random)
  - 健康检查
  - 统计数据
- [x] BackupService 服务
  - 数据库备份/恢复
  - 自动清理旧备份
  - 备份统计
- [x] SystemConfigService 服务
  - 配置键值存储
  - JSON配置支持
- [x] SystemHandler API 处理器
  - 系统配置 CRUD
  - 备份管理
- [x] LoadBalancerHandler API 处理器
  - 负载均衡器 CRUD
  - 健康检查
  - 统计数据
- [x] API 路由注册
- [x] 数据库迁移

---

## 六、差异化竞争优势

| 功能 | V2Board | XBoard | Marzban | **本项目** |
|------|---------|--------|---------|------------|
| Go高性能后端 | ❌ | ❌ | ❌ | ✅ |
| gRPC支持 | ❌ | ❌ | ❌ | ✅ |
| 流量转发面板 | ❌ | ❌ | ❌ | ✅ |
| 智能路由 | ❌ | ❌ | ❌ | ✅ |
| Telegram Bot | ❌ | ❌ | ✅ | ✅ |
| MFA认证 | ❌ | ❌ | ❌ | ✅ |
| 邀请返利系统 | ✅ | ✅ | ❌ | ✅ |
| 节点负载均衡 | ❌ | ❌ | ❌ | ✅ |
| 数据备份/恢复 | ❌ | ❌ | ✅ | ✅ |
| 系统配置管理 | ❌ | ❌ | ❌ | ✅ |

---

## 七、API 总览

### 管理员 API (`/api/v2/admin/*`)

| 模块 | 端点数量 | 功能 |
|------|----------|------|
| 用户管理 | 8 | 用户CRUD、封禁、重置流量 |
| 节点管理 | 14 | 节点CRUD、协议管理、授权密钥 |
| 订阅管理 | 15 | 分组、模板、预览 |
| 套餐管理 | 6 | 套餐CRUD、分配 |
| 订单管理 | 7 | 订单列表、状态更新 |
| 流量转发 | 12 | 节点、规则、统计 |
| 支付网关 | 8 | 网关管理、统计 |
| Telegram | 7 | Bot配置、通知 |
| MFA | 2 | 全局配置 |
| 通知 | 8 | 模板、日志、邮件配置 |
| 邀请返利 | 5 | 配置、提现审核 |
| 系统配置 | 4 | 键值配置 |
| 备份管理 | 7 | 配置、备份、恢复 |
| 负载均衡 | 7 | 负载均衡器管理 |
| **总计** | **110** | |

### 用户 API (`/api/v2/user/*`)

| 模块 | 端点数量 | 功能 |
|------|----------|------|
| 资料仪表盘 | 3 | 资料、仪表盘、订阅 |
| 工单 | 5 | 工单CRUD、回复 |
| 订单 | 3 | 订单列表、创建 |
| 流量转发 | 2 | 规则管理 |
| 支付 | 4 | 渠道、创建、状态 |
| Telegram | 3 | 状态、解绑 |
| MFA | 6 | 状态、设置、验证 |
| 通知 | 4 | 列表、已读 |
| 邀请 | 5 | 邀请码、佣金、提现 |
| **总计** | **35** | |

---

*文档生成时间: 2026-03-13*

## Flux-panel Clone Workstream

- Keep `docs/guide/flux-panel-workstream.md` as the single source of truth for flux-panel clone status, reference mapping, status scale, implementation order, and definition of done.
- Use the document to interpret the `Not Started / Partial / Aligned` states before starting work on forward/tunnel/diagnose flows and to pick the next implementation milestone.
- After delivering a piece, refresh the workstream and ensure other docs (like `docs/guide/flux-forward-contract.md`) and this roadmap keep pointing to the latest reference.
