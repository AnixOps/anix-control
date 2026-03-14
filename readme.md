# V2Board AnixOps

现代化的代理面板管理系统，Go + Vue 3 技术栈。

## 特性

- **高性能后端** - Go 1.24 + Gin，支持高并发
- **现代前端** - Vue 3 + Vite + Element Plus
- **多协议支持** - VMess/VLESS/Trojan/Shadowsocks/Hysteria2/TUIC
- **多订阅格式** - V2Ray/Clash/Sing-box/Surge
- **gRPC 通信** - 节点与面板双向流通信
- **Agent 系统** - NAT 穿透，远程节点管理
- **流量转发** - gost 集成，中转节点管理
- **零依赖部署** - 单二进制文件 + SQLite，无需安装任何外部服务

## 技术栈

| 组件 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.24+ | 高性能、静态编译、跨平台 |
| Web框架 | Gin | 高性能 HTTP 框架 |
| ORM | GORM | Go 语言 ORM 框架 |
| 数据库 | SQLite / PostgreSQL | 轻量级或企业级 |
| 缓存 | 内存缓存 / Redis | 零依赖或分布式 |
| 前端 | Vue 3 + Vite | 现代前端框架 |
| 通信 | gRPC + WebSocket | 双向实时通信 |

## 项目结构

```
v2board_AnixOps/
├── cmd/server/           # 主程序入口
├── config/               # 配置文件
├── internal/             # Go 后端代码
│   ├── handler/          # HTTP 处理器
│   ├── service/          # 业务逻辑
│   ├── model/            # 数据模型
│   ├── grpc/             # gRPC 服务
│   ├── gost/             # gost 客户端
│   └── ...
├── web/                  # Vue 前端
│   ├── src/views/        # 页面组件
│   ├── src/api/          # API 调用
│   └── ...
├── docker/               # Docker 配置
├── docs/                 # Swagger 文档
└── deploy/               # 部署文件
```

## 快速开始

### 使用 Docker Compose (推荐)

```bash
# 1. 克隆仓库
git clone https://github.com/anixops/v2board.git
cd v2board

# 2. 复制配置文件
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3. 编辑配置
nano .env
nano config/config.yaml

# 4. 启动服务
docker-compose up -d

# 5. 查看日志
docker-compose logs -f api
```

### 本地开发

```bash
# 后端
go mod download
go run cmd/server/main.go

# 前端
cd web
npm install
npm run dev
```

### 编译

```bash
# 安装依赖
go mod tidy

# 编译
go build -o v2board ./cmd/server

# 或使用 make
make build
```

## 配置说明

### SQLite 配置 (默认，零配置)

```yaml
database:
  driver: "sqlite"
  database: "data/v2board.db"
```

### PostgreSQL 配置

```yaml
database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  database: "v2board"
  username: "postgres"
  password: "your_password"
```

### 必要配置

```yaml
jwt:
  secret: "your-jwt-secret-at-least-32-characters"

app:
  api_token: "your-node-communication-token"
```

## 部署指南

### 开发环境

```bash
docker-compose up -d
```

### 生产环境

```bash
# 使用生产配置
docker-compose -f docker-compose.prod.yml up -d

# 启用监控 (Prometheus + Grafana)
docker-compose -f docker-compose.prod.yml --profile monitoring up -d
```

### Systemd 服务

创建 `/etc/systemd/system/v2board.service`：

```ini
[Unit]
Description=V2Board AnixOps
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/v2board
ExecStart=/opt/v2board/v2board -config /opt/v2board/config/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable v2board
sudo systemctl start v2board
```

## API 文档

启动服务后访问：`http://localhost:8080/swagger/index.html`

## 主要功能模块

| 模块 | 说明 |
|------|------|
| 用户系统 | 注册/登录/资料/权限 |
| 套餐系统 | 计费/流量限制/续费 |
| 节点系统 | 多协议/分组/自动注册 |
| 订阅系统 | 多格式/分组/模板 |
| 支付系统 | 多渠道/统计 |
| 工单系统 | 创建/回复/关闭 |
| Agent 系统 | NAT 穿透/远程控制 |
| 流量转发 | 中转节点/gost 集成 |
| Telegram Bot | 命令/通知/广播 |
| MFA 认证 | TOTP/备用码 |

## 测试

```bash
# 运行所有测试
go test ./...

# 运行带覆盖率的测试
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

## 相关项目

- [V2bX_AnixOps](https://github.com/anixops/V2bX_AnixOps) - 节点端程序
- [AnixOps-agent](https://github.com/anixops/anixops-agent) - 远程控制 Agent

## License

MIT License