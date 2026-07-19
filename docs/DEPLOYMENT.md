# AnixOps Control 部署指南

本文档覆盖本项目的常见部署方式：本地开发、Docker Compose、生产环境部署与运维。

如果你只需要最短启动路径，先看：

- [`control-boundary.md`](control-boundary.md)
- [`README.md`](README.md)
- [`reference/quickstart.md`](reference/quickstart.md)
- [`reference/startup-config.md`](reference/startup-config.md)
- [`reference/configuration.md`](reference/configuration.md)
- [`reference/runtime.md`](reference/runtime.md)
- [`guide/forward-relay-onboarding.md`](guide/forward-relay-onboarding.md)
- [`reference/repository-layout.md`](reference/repository-layout.md)

部署前先记住两条规则：

- 后端始终先读取 `config/config.yaml`；本地 `go run` 不会自动读取 `.env`
- 启动时会先读取 `config/config.yaml.forward_runtime`，并把这些值写入 `v2_system_config`，不再参考 `FORWARD_RUNTIME_*`

## Verified Deployment Baseline (2026-04-07)

当前已经实机验证通过的路径是：

- `binary + SQLite + systemd`
- AnixOps Control UI: `3000`
- AnixOps Control API: `8080`
- NodeX control-plane: `18081`
- relay gost API: `18080`
- 已验证运行时：
  - `nftables_ansible`（推荐默认）
  - `iptables_ansible`（legacy 兼容）
  - `gost`

这套基线的意义：

- 它是当前文档里最权威的“已跑通”版本
- Docker 部署仍然会继续补齐，但不应被误读成当前已经完成同等级实机验证的 forward-runtime 基线

AnixOps Agent 的默认控制通道使用 gRPC `50051`。开发环境可以只在
loopback 或受控私网使用明文；公网部署必须配置 `grpc.tls_cert_file` 和
`grpc.tls_key_file`，或放在支持 HTTP/2 gRPC 的四层/反向代理之后，并通过
防火墙限制来源。旧配置中的 `grpc.enabled: false` 不会被升级程序自动改写。

## 目录

1. 环境要求
2. 快速部署（Docker Compose）
3. 本地开发部署
4. 生产环境部署建议
5. 关键配置说明
6. TLS/HTTPS
7. 监控与日志
8. 备份与恢复
9. 常见故障排查
10. 更新升级

---

## 1. 环境要求

### 最低配置

- CPU: 1 核
- 内存: 512MB
- 磁盘: 10GB
- 系统: Linux / macOS / Windows

### 推荐配置（生产）

- CPU: 2 核及以上
- 内存: 2GB 及以上
- 磁盘: 50GB 及以上 SSD
- 系统: Ubuntu 22.04 / Debian 12

### 软件依赖

| 软件 | 版本建议 | 用途 |
|------|----------|------|
| Docker | 24.0+ | 容器运行时 |
| Docker Compose | 2.0+ | 编排服务 |
| Go | 1.26.5 | 仅用于本地开发/测试；发行构建必须走 GitHub Actions |
| Node.js | 22+ | 仅用于本地开发/测试；发行前端资产必须走 GitHub Actions |

---

## 2. 开发或受控 Docker Compose 部署

This source-checkout Docker Compose path is for development or an operator-owned
container workflow. It is not the default stable release installation path:
production hosts should use the tag-pinned release installer in section 2.1 or
an image digest supplied by the GitHub Release metadata.

```bash
# 1) 克隆仓库
git clone https://github.com/AnixOps/anix-control.git
cd anix-control

# 2) 准备配置
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3) 按需修改配置
# 至少设置 jwt.secret、app.api_token、admin.email、admin.password
# nano config/config.yaml

# 4) 如需 NodeX mode，直接编辑 config/config.yaml 里的 forward_runtime
# backend: gost
# nodex.base_url: http://nodex-control-plane:18081
# nodex.token: replace-with-shared-token

# 5) 启动
docker compose up -d

# 6) 查看状态与日志
docker compose ps
docker compose logs -f anix-control
```

默认访问地址：`http://localhost:8080`

默认前端地址：`http://localhost:3000`

### 2.1 一键安装脚本

生产环境默认使用 GitHub Release 安装器。它只下载版本匹配的发布二进制、前端包、校验和和单个配置模板，不 clone 仓库，也不在服务器构建 Go、前端或 Docker 镜像。对 `v4.*`，它还会下载并校验签名身份包三件套，暂存到 root 所有的引导目录后验证登录路径：

```bash
export VERSION=v4.0.0
curl -fsSL \
  "https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}/scripts/install.sh" \
  -o /tmp/anix-control-install.sh
sudo bash /tmp/anix-control-install.sh install --version "${VERSION}" --admin-email "admin@example.com"
rm -f /tmp/anix-control-install.sh
```

安装器会验证 Release 中的 SHA-256，保留已有配置和数据库，更新失败时恢复上一个二进制/前端快照。完整步骤、反向代理、升级与回滚说明见 [`guide/release-installation.md`](guide/release-installation.md)。

历史源码/Docker 安装器仍保留给受控恢复场景；必须显式设置 `ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1`，不是正式发布路径。

### 2.2 Docker 内置 ansible-playbook

运行时镜像已内置：

- `ansible`
- `openssh-client`
- `bash`

默认容器内路径：

- ansible 配置：`/app/config/deploy/ansible/ansible.cfg`
- inventory：`/app/config/deploy/ansible/inventory.ini`
- nftables 应用 playbook：`/app/config/deploy/ansible/playbooks/forward_apply_nftables.yml`
- nftables 删除 playbook：`/app/config/deploy/ansible/playbooks/forward_remove_nftables.yml`
- iptables legacy 应用 playbook：`/app/config/deploy/ansible/playbooks/forward_apply.yml`
- iptables legacy 删除 playbook：`/app/config/deploy/ansible/playbooks/forward_remove.yml`

默认 system config JSON 示例：

```json
{
  "inventory": "/app/config/deploy/ansible/inventory.ini",
  "playbookApply": "/app/config/deploy/ansible/playbooks/forward_apply_nftables.yml",
  "playbookRemove": "/app/config/deploy/ansible/playbooks/forward_remove_nftables.yml",
  "workingDir": "/app/config/deploy/ansible",
  "targetPattern": "{{node.host}}",
  "timeoutSeconds": 120,
  "become": true,
  "environment": {
    "ANSIBLE_CONFIG": "/app/config/deploy/ansible/ansible.cfg",
    "ANSIBLE_HOST_KEY_CHECKING": "False"
  }
}
```

说明：

- `nftables_ansible` 是当前推荐的本地无状态后端；`iptables_ansible` 仅保留给旧 relay 环境的兼容入口。两者都由 AnixOps Control 内置的 panel-host executor 执行。
- 本地 Ansible 路径不等同于 `NodeX`，也不属于 `flux-panel` 原始 `/forward` 页面契约。
- 示例 inventory 模板位于 `config/deploy/ansible/inventory.ini.example`，安装脚本会复制为 `inventory.ini`。
- SSH 密钥目录为 `config/deploy/ssh/`，会被挂载到容器内的 `/home/v2board/.ssh`。
- Docker 启动时会先读取 `config/config.yaml.forward_runtime`，并把这些值同步到系统配置表；环境变量不影响 runtime 行为。
- 如果 NodeX 与 AnixOps Control 不在同一个网络命名空间，`forward_runtime.nodex.base_url` 不能写成容器内的 `127.0.0.1`，应写成可达的宿主机地址或 Compose service 名。
- 如果没有 SSH 私钥，调整 `config/deploy/ansible/inventory.ini` 或所选本地 ansible block 下的 `extra_vars` 来提供目标主机的账户信息。
- 容器启动时会基于 `config/config.yaml.forward_runtime` 写入 runtime 配置；非 root 用户可以直接在 YAML 中设置 `forward_runtime.nftables_ansible.become=true`，sudo 凭据则放在 inventory 或其他 ansible 变量里。若使用 legacy path，则对应改 `forward_runtime.iptables_ansible.become=true`。
- 后端切换、运行时任务观测和部署引导应停留在系统/部署文档范围内，不应并入 Flux 克隆的 `/admin/forward` 页面。
- 公开边界说明见 [`guide/nodex-internal-extension.md`](guide/nodex-internal-extension.md)。

---

## 3. 本地开发部署

```bash
# 后端
go mod download
cp config/config.yaml.example config/config.yaml
go run ./cmd/server/main.go -config ./config/config.yaml

# 前端
cd web
npm install
npm run dev
```

本地开发补充说明：

- 如果只跑本地开发，不会自动读取 `.env`
- 本地开发优先改 `config/config.yaml.forward_runtime`
- 需要临时调整 runtime 字段时，重新编辑 `config/config.yaml.forward_runtime` 并重启后台
- 运行时合并值会在启动阶段写进 `v2_system_config`，后续可在 `/admin/system` 里继续调整

发行构建：

- 所有发行版本必须由 GitHub Actions release workflow 构建。
- 后端二进制、前端静态包、Docker 镜像元数据、校验和、SBOM 都以 GitHub Release 附件为准。
- 不要在生产机或本机用 `go build`、`npm run build`、`deploy_panel.sh` 生成发行产物。
- 本地 `go run`、前端 `npm run dev` 只用于开发调试，不作为发行或生产更新来源。

---

## 4. 生产环境部署建议

### 架构建议

- 反向代理：Nginx 或 Caddy（统一 TLS 终止）
- 应用服务：`anix-control`（Go 二进制）
- 数据库：PostgreSQL（优先）
- 缓存：Redis（多实例/高并发场景）
- 监控：Prometheus（可选），内置 Grafana（可选）

### systemd 二进制更新

- systemd 服务名默认是 `anix-control.service`，旧部署可用 `SERVICE_NAME=v2board` 原位升级。
- GitHub Release 中的匹配平台二进制是唯一发行二进制来源。
- GitHub Release 中的 `anix-control-frontend.tar.gz` 或 `anix-control-frontend.zip` 是唯一发行前端来源。
- 部署前必须校验 `SHA256SUMS.txt`，再停止服务、替换二进制和前端静态文件、启动服务并验证 `/health`。

`config/deploy/deploy_panel.sh` 会执行本地源码构建，默认拒绝运行。旧入口 `config/scripts/deploy.sh` 只是兼容包装器，也默认拒绝运行；`config/scripts/pre-deploy.sh` 的本地构建检查同样默认拒绝。它们只保留给开发或紧急人工操作，不能作为发行版本构建路径。如确需使用，必须显式设置 `ALLOW_LOCAL_BUILD=1`，并在变更记录中说明原因。CI 会运行 `config/deploy/check_release_build_policy.sh`，阻止未声明 GitHub Actions-only 策略和 `ALLOW_LOCAL_BUILD` guard 的本地部署 build 命令进入部署脚本。

部署前可在仓库根目录运行脚本自检：

```bash
bash config/deploy/deploy_panel.sh --self-test
bash config/deploy/check_release_build_policy.sh --self-test
bash config/deploy/check_release_build_policy.sh
bash config/scripts/deploy.sh --self-test
bash config/scripts/pre-deploy.sh --self-test
```

本机构建残留清理：

```bash
bash config/deploy/clean_local_build_artifacts.sh --dry-run
bash config/deploy/clean_local_build_artifacts.sh

# 如需同时清理源码树里旧本地部署 archive 残留，先 dry-run 再执行：
bash config/deploy/clean_local_build_artifacts.sh --dry-run --include-deploy-backups
bash config/deploy/clean_local_build_artifacts.sh --include-deploy-backups
```

该脚本会清理源码树里的本地构建/验证输出，例如 Go 二进制、
`coverage.out`、`web/public`、`web/public-check`、`web/coverage`、
`web/bundle-reports`、`web/bundle-reports-check` 和 release 暂存目录。
显式加 `--include-deploy-backups` 时，还会清理忽略的旧本地部署 archive
残留，例如 `backups/frontend_*.tar.gz` 和 `internal/*/backups/*.zip`。

如果旧产物由 root 生成导致普通用户无权删除，脚本会输出需要用 root 执行的精确 `sudo rm -rf -- ...` 命令。

清理脚本不会删除 `config/config.yaml`、数据库、TLS 证书、Ansible inventory、数据库备份或 `web/node_modules`。

### 启动生产编排

```bash
docker compose -f docker-compose.prod.yml up -d
```

如需只启动 Prometheus：

```bash
docker compose -f docker-compose.prod.yml --profile prometheus up -d
```

如需同时启用内置 Grafana：

```bash
docker compose -f docker-compose.prod.yml --profile prometheus --profile grafana up -d
```

---

## 5. 关键配置说明

配置文件：`config/config.yaml`

配置边界：

- `config/config.yaml`
  - 启动必读
  - 管 server/database/cache/jwt/app/admin
`.env`
  - 用于 Docker Compose 或安装器的额外端口、密钥等元数据；运行时选择依旧来源于 `config/config.yaml.forward_runtime`
- `v2_system_config`
  - 启动后持久化的运行时配置
  - `/admin/system` 编辑的也是这一层

### NodeX 控制面（当前 `forward` 运行时调用路径）

| 配置项 | 说明 |
|--------|------|
| `forward.runtime.nodex.base_url` | NodeX 控制面基础地址（如 `https://nodex.example.com`）。当前 `/admin/forward` 运行时与 legacy rule sync 的 NodeX 调用都要求显式配置该值。 |
| `forward.runtime.nodex.token` | NodeX 控制面认证令牌。当前 `panel_forward` 运行时要求显式配置该值，请不要把它和 relay `ForwardNode.api_token` 混用。前者用于 `AnixOps Control -> NodeX`，后者用于 `NodeX -> relay gost API`。 |
| `forward.runtime.nodex.timeout_seconds` | 可选；请求超时时间（秒，默认 15）。通过 `config/config.yaml.forward_runtime` 设置即可。 |

> 当前 `panel_forward`（`/admin/forward` 创建、更新、暂停、删除、诊断等）与 `legacy_rule` 同步都会走 NodeX control-plane 的 `/api/v2/internal/forward/runtime/execute`。因此 `forward.runtime.nodex.base_url` 现在必须显式指向 NodeX 控制面；客户端不会再隐式猜测 ingress/relay 节点的 `host:apiPort` 作为外层控制面地址，缺失该值会直接报错。

### 数据库

SQLite（默认，单实例简单部署）：

```yaml
database:
  driver: "sqlite"
  database: "config/data/v2board.db"
```

PostgreSQL（推荐生产）：

```yaml
database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  database: "v2board"
  username: "postgres"
  password: "your_password"
```

从 SQLite 迁移到 PostgreSQL 时，不要直接改生产配置后启动。先按 [`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md) 做备份、dry run、导入、验证和 rollback 记录。

### 缓存

内存缓存（默认）：

```yaml
cache:
  driver: "memory"
```

Redis：

```yaml
cache:
  driver: "redis"
  host: "127.0.0.1"
  port: 6379
  password: ""
  db: 0
```

### 必填项

```yaml
jwt:
  secret: "your-jwt-secret-at-least-32-characters"
```

> 说明：`/api/v1|v2/server/UniProxy/*` 已强制使用节点级鉴权，必须携带 `node_id` 查询参数与 `X-API-Key` 请求头（值为该节点的 `api_key`）。`app.api_token` 仅保留为历史兼容字段，不用于 UniProxy 鉴权。

---

## 6. TLS/HTTPS

推荐在 Nginx/Caddy 层处理证书，应用层保持 HTTP 内网监听。

### Let's Encrypt（示例）

```bash
# 安装 certbot
sudo apt update
sudo apt install -y certbot

# 申请证书（以单域名为例）
sudo certbot certonly --standalone -d panel.example.com
```

证书路径通常为：

- `/etc/letsencrypt/live/panel.example.com/fullchain.pem`
- `/etc/letsencrypt/live/panel.example.com/privkey.pem`

---

## 7. 监控与日志

### 常见监控项

- HTTP 请求总量与耗时
- 活跃用户数
- 活跃节点数
- 错误率（5xx）

### 常用日志命令

```bash
# 全部服务日志
docker compose logs -f

# 指定服务日志
docker compose logs -f api
docker compose logs -f nginx

# 最近 100 行
docker compose logs --tail=100 api
```

---

## 8. 备份与恢复

### 备份

SQLite：

```bash
cp config/data/v2board.db config/data/v2board.db.backup
```

PostgreSQL：

```bash
docker exec anix-control-db pg_dump -U v2board v2board > backup.sql
```

### 恢复（PostgreSQL）

```bash
docker exec -i anix-control-db psql -U v2board v2board < backup.sql
```

建议使用 `crontab` 做每日自动备份，并设置保留策略。

数据库迁移的 dry-run 与 rollback 步骤见 [`reference/sqlite-to-postgres-migration.md`](reference/sqlite-to-postgres-migration.md)。

---

## 9. 常见故障排查

### 1) 服务无法启动

```bash
docker compose logs api
```

重点检查：

- `config/config.yaml` 是否有效
- 数据库连接是否可达
- 端口是否冲突

### 2) 数据库连接失败

```bash
docker compose exec db pg_isready
```

检查数据库主机、端口、用户名、密码、数据库名。

### 3) 前端访问异常

```bash
docker compose exec nginx nginx -t
```

确认反向代理配置、生效证书和静态资源路径。

### 4) 节点无法连接面板

```bash
# 检查节点请求参数（node_id + X-API-Key）
# X-API-Key 应为该节点分配的 api_key

# 检查节点接口
curl -H "X-API-Key: your_node_api_key" "http://localhost:8080/api/v2/server/UniProxy/config?node_id=1"
```

---

## 10. 更新升级

生产升级必须使用 GitHub Release 附件中的已验证产物，不要在生产机
`git pull` 后本地构建发行二进制、前端静态文件或 Docker 镜像。

升级前先阅读 [`UPGRADE.md`](UPGRADE.md)，确认：

- 目标 tag、发布说明、`CHANGELOG.md` 和 `docs/features.md`
- `SHA256SUMS.txt` 校验通过
- `RELEASE_MANIFEST.json` 中 `build_source` 为 `github-actions`
- 数据库和配置已备份
- 如涉及迁移，已保留 `migration-dry-run.txt`
- 已有明确 rollback owner 和 rollback window

建议先在预发布环境验证，再进行生产升级。
