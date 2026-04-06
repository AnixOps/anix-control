# V2Board AnixOps

现代化代理面板管理系统，基于 Go + Vue 3，支持多协议节点管理、订阅分发、订单计费和节点通信。

## 主要特性

- 高性能后端：Go 1.24 + Gin，支持高并发
- 现代前端：Vue 3 + Vite + Pinia
- 多协议支持：VMess / VLESS / Trojan / Shadowsocks / Hysteria2 / TUIC
- 多订阅格式：V2Ray / Clash / Sing-box / Surge
- 面板-节点通信：支持 HTTP 与 gRPC 双通道
- 节点自动注册：支持 AuthKey 自动发现
- 可选数据库：SQLite（默认）或 PostgreSQL
- 可选缓存：Memory（默认）或 Redis

## 技术栈

| 组件 | 技术 | 说明 |
|------|------|------|
| 后端语言 | Go 1.24+ | 静态编译、跨平台、高性能 |
| Web 框架 | Gin | HTTP API 与中间件 |
| ORM | GORM | 数据模型与数据库访问 |
| 前端 | Vue 3 + Vite | 管理台与用户端 UI |
| 状态管理 | Pinia | 前端状态管理 |
| 通信 | gRPC + HTTP | 节点配置同步与状态上报 |

## 项目结构

```text
v2board_AnixOps/
├── cmd/                        # 程序入口
├── config/                     # 配置文件
├── data/                       # SQLite 数据目录
├── internal/                   # 后端核心代码
│   ├── handler/                # HTTP 处理器
│   ├── service/                # 业务逻辑层
│   ├── model/                  # GORM 数据模型
│   ├── router/                 # 路由定义
│   ├── parser/                 # 订阅解析与格式化
│   └── grpc/                   # gRPC 服务实现
├── web/                        # Vue 前端
│   └── src/
├── docs/                       # 文档
└── docker-compose.yml          # 本地一键部署
```

## 转发运维文档

Forward/Tunnel 的执行层有两套互斥的运维方案，对应 `forward.runtime_backend`：

| 模式 | 描述 | 核心控制 | 运行时作用的节点 |
|------|------|---------------|----------------|
| NodeX Mode = true | gost + NodeX 控制面，执行时走 `forward.runtime.nodex.*` 控制平面 | `forward.runtime_backend=gost` + `forward.runtime.nodex.base_url`/`token`/`timeout_seconds` | `ForwardNode`（`type=relay`）通过 HTTP 拉取 job；代理节点 `model.Node` 仅承担正常用户代理 |
| NodeX Mode = false | 基于 `ansible-playbook` 触发 SSH 命令的无状态同步 | `forward.runtime_backend=iptables_ansible` + `forward.runtime.iptables_ansible.config`（可选 `inventory`/`playbook`/`extraVars` 等） | 只需转发节点的 SSH 凭证；proxy `model.Node` 不参与 runtime |

### 运行时环境示例

```env
FORWARD_RUNTIME_BACKEND=gost
FORWARD_RUNTIME_NODEX_BASE_URL=http://127.0.0.1:18080
FORWARD_RUNTIME_NODEX_TOKEN=nodex-secret
FORWARD_RUNTIME_NODEX_TIMEOUT_SECONDS=20

# 或者，切换到 Ansible 模式
FORWARD_RUNTIME_BACKEND=iptables_ansible
FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON={"inventory":"/etc/ansible/forward_inventory.ini","playbookApply":"/etc/ansible/forward_apply.yml","playbookRemove":"/etc/ansible/forward_remove.yml"}
FORWARD_RUNTIME_ANSIBLE_INVENTORY=/etc/ansible/forward_inventory.ini
```

### 手工验证与文档

1. NodeX 模式：先确认 `ForwardNode` 记录的 `host`/`api_port`/`api_token` 可访问 NodeX 控制面，再参考 `docs/guide/forward-tunnel-runtime-ops.md` 和 `docs/guide/forward-tunnel-smoke-test.md` 按步骤发起 runtime job（`/api/v2/admin/forward/runtime/jobs` + `v2_forward_runtime_job` 记录）。
2. Ansible 模式：检查 `forward.runtime.iptables_ansible.config` 中的 inventory/playbook/SSH 参数，确认 `ansible-playbook` 能在 inventory 指定的 host 上运行；`docs/guide/forward-tunnel-runtime-ops.md` 和 `docs/guide/forward-tunnel-smoke-test.md` 提供完整 smoke 验证。
3. 任何环境变量刷新后，`InitForwardRuntimeSystemConfigFromEnv` 会把值写入 `forward.runtime.*` 系列配置，前端可在 `/admin/system` 看到当前值。

### 节点角色区分

- **Proxy Node (`model.Node`)** 由 `/admin/nodes` 管理，用于用户连接代理服务，与运行 forwarding job 的控制面没有直接对应关系。
- **Forward Node (`model.ForwardNode`)** 只存在于 `v2_forward_node`，用于 NodeX Mode（REST）或 Ansible Mode（SSH）立即执行 runtime job；其 `host/api_port/api_token` 是 NodeX 控制面调用目标。
- 这些节点角色绝不能混用；在 smoke 目录的文档里也会多次强调 proxy node online 与 forward node job 流程分别校验的步骤。

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
# 1) 克隆仓库
git clone https://github.com/AnixOps/v2board_AnixOps.git
cd v2board_AnixOps

# 2) 准备配置文件
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3) 启动服务
docker-compose up -d

# 4) 查看日志
docker-compose logs -f
```

默认前端地址：`http://localhost:3000`

### 方式零：一键安装脚本

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/AnixOps/v2board_AnixOps/go_dev/install.sh)
```

如果你已经在仓库根目录，也可以用兼容上游命名的本地包装器：

```bash
bash ./panel_install.sh
```

这个安装器会：

- 下载仓库源码归档到目标目录
- 本地构建带 `ansible-playbook` 的 Docker 镜像
- 生成 `config/config.yaml` 与 `.env`
- 初始化 `config/deploy/ansible/inventory.ini` 与 `config/deploy/ssh/`
- 启动前端、API、gRPC 端口映射
- 生产模式下可只启用 Prometheus，不必连带部署 Grafana
- 通过容器环境变量在启动阶段预写入双运行时示例配置

### 方式二：本地开发

```bash
# 后端
go mod download
go run cmd/server/main.go

# 前端
cd web
npm install
npm run dev
```

## 构建

```bash
# 后端可执行文件
go build -o v2board ./cmd/server

# 或使用 Makefile
make build

# 前端构建
cd web
npm run build
```

## 配置说明

默认配置文件：`config/config.yaml`

### SQLite（默认）

```yaml
database:
  driver: "sqlite"
  database: "data/v2board.db"
```

### PostgreSQL

```yaml
database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  database: "v2board"
  username: "postgres"
  password: "your_password"
```

### 必填项

```yaml
jwt:
  secret: "your-jwt-secret-at-least-32-characters"

app:
  api_token: "your-node-communication-token"
```

> 说明：UniProxy（`/api/v2/server/UniProxy/*`）鉴权使用 `query: node_id` + `header: X-API-Key`，不使用 query token。

## 常用接口

### 公开接口

- `GET /health`：健康检查
- `GET /s/:token`：用户订阅

### 用户接口（`/api/v2`）

- `POST /login`
- `POST /register`
- `GET /user/profile`
- `GET /user/dashboard`

### 管理员接口（`/api/v2/admin`）

- `GET /admin/dashboard`
- `GET/POST /admin/users`
- `GET/POST /admin/nodes`
- `GET/POST /admin/plans`

### 节点通信接口

- `GET /api/v2/server/UniProxy/config`
- `GET /api/v2/server/UniProxy/user`
- `POST /api/v2/server/UniProxy/push`
- `POST /api/v2/server/UniProxy/alive`
- 鉴权方式：`query: node_id` + `header: X-API-Key`

## 测试

```bash
# 后端全量测试
go test ./...

# gRPC 模块测试
go test -v ./internal/grpc/...

# 覆盖率
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

如果只验证当前 Flux 兼容的 Forward/Tunnel 改动，建议使用以下命令，避免父级 `go.work` 干扰模块测试：

```bash
$env:GOWORK='off'; go test ./internal/handler -run ForwardPanel
$env:GOWORK='off'; go test ./internal/service -run 'TestPanelForwardService|TestPanelForwardRuntimeService|^TestForwardFlowResetWorker$|TestForwardRuntimeJobExecutor|TestForwardRuntimeProvider|TestForwardGostStatsWorker|TestForwardRuntimeLocalBypass'
cd web && npm test -- adminApi.test.js
cd web && npm run build
```

## Flux-panel 复刻

仓库已经进入 `flux-panel` 定向兼容阶段。后续凡是复刻 `flux-panel` 页面或接口，默认遵循“路径、DTO、返回包、交互和业务语义一比一对齐”的原则，而不是先做本项目风格版本。

- 复刻规范入口：`AGENTS.md`
- 详细执行指南：`docs/guide/flux-panel-clone.md`
- forward/tunnel 契约文档：`docs/guide/flux-forward-contract.md`
- 当前复刻状态与下一步：`docs/guide/flux-panel-workstream.md`
- 对外兼容接口入口：`docs/guide/api-reference.md`
- 历史规划与当前工作流挂钩：`docs/FEATURE_ROADMAP.md`
- 当前已完成基础模块：流量转发页面与兼容 API
- 当前本机参考仓库：`C:\Users\z7299\AppData\Local\Temp\flux-panel`
- 运行时与排查文档：`docs/guide/forward-tunnel-runtime-ops.md`（NodeX / iptables_ansible 启动说明、前端 401 排查）
- 手工联调与验收文档：`docs/guide/forward-tunnel-smoke-test.md`（Tunnel/Forward 页面、兼容 API、双运行时最小验收路径）

## 可选内部执行面部署

流量转发对外仍以 `flux-panel` 的 `/admin/forward` 契约为主。实际执行层可以按部署需要走以下路径：

- `gost`：Flux 对齐路径
- `iptables_ansible`：兼容命名的内部后端入口，对应可选内部执行面 `NodeX`

Docker 镜像已经内置 `ansible-playbook`，示例 playbook 位于 `config/deploy/ansible/`。一键安装脚本会预置 `FORWARD_RUNTIME_BACKEND` 与 `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`，应用启动后再同步到 `forward.runtime_backend` 与 `forward.runtime.iptables_ansible.config`。这些键属于系统部署/内部后端初始化，不属于 Flux `/admin/forward` 页面契约。
如果需要在 Docker 部署时初始化该内部后端，可在 `.env` 提供 SSH 与提权相关变量，容器会生成兼容 inventory 并同步到系统配置。边界说明见 `docs/guide/nodex-internal-extension.md`，部署细节见 `docs/DEPLOYMENT.md`。

## 相关项目

- [V2bX_AnixOps](https://github.com/anixops/V2bX_AnixOps)：节点端程序
- [AnixOps-agent](https://github.com/anixops/anixops-agent)：远程 Agent 管理

## License

MIT License
