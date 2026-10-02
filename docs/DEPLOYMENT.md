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
6. TLS/HTTPS（6.1 Reverse proxies / `server.trusted_proxies`）
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
| Go | 1.26.8 | 仅用于本地开发/测试；发行构建必须走 GitHub Actions |
| Node.js | 22+ | 仅用于本地开发/测试；发行前端资产必须走 GitHub Actions |

---

## 2. 容器部署（推荐）

容器是主部署路径。发布镜像 `ghcr.io/anixops/anix-control` 由 GitHub Actions
用 Release 附件构建（linux/amd64、linux/arm64），带 SBOM、provenance 和 cosign
签名；镜像内含本版本的二进制、前端和签名身份引导包，不需要配置文件，全部设置
通过 `ANIX_CONTROL_*` 环境变量或 `*_FILE` secret 文件提供（`docker run --rm
<image> -print-env` 列出全部变量）。Control 本体无状态，数据全部在外部
PostgreSQL 中，因此迁移到另一台机器只需复制 `control.env` 与 `secrets/`。

### 2.0 Docker Compose + 外部 PostgreSQL

```bash
# 1) 取得本版本的 Compose 文件与模板（与镜像同一个 tag）
export VERSION=v4.0.1
mkdir -p /opt/anix-control && cd /opt/anix-control
base="https://raw.githubusercontent.com/AnixOps/anix-control/${VERSION}"
curl -fsSLO "${base}/docker-compose.prod.yml"
curl -fsSL "${base}/config/deploy/compose/control.env.example" -o control.env
curl -fsSL "${base}/.env.example" -o .env

# 2) 固定镜像 digest（来自 Release 附件 docker-image.txt），并验证签名
#    编辑 .env: ANIX_CONTROL_IMAGE=ghcr.io/anixops/anix-control@sha256:<digest>
cosign verify ghcr.io/anixops/anix-control@sha256:<digest> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/AnixOps/anix-control/'

# 3) 编辑 control.env（数据库地址、管理员邮箱等），创建 secrets（容器 uid 10001 可读）
install -d -m 0750 secrets
openssl rand -hex 32 | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/jwt_secret
printf '%s' '数据库密码' | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/db_password

# 4) 启动：migrate 一次性服务先准备库结构，control 随后启动
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml logs migrate   # 新库会打印生成的管理员密码
docker compose -f docker-compose.prod.yml ps             # control 应为 healthy
curl -fsS http://127.0.0.1:8080/readyz
```

说明：

- 默认只在 `127.0.0.1` 发布 8080（API）、3000（UI）、50051（gRPC）；对外由宿主机
  反向代理终止 TLS，示例见 [`config/deploy/examples/nginx/anix-control.conf`](../config/deploy/examples/nginx/anix-control.conf)。
  节点直连 gRPC 时在 `.env` 设置 `ANIX_CONTROL_GRPC_BIND`，并在 `control.env` 设置 `ANIX_CONTROL_GRPC_ENABLED=true`。
- 数据库在本机时，`ANIX_CONTROL_DATABASE_HOST=host.docker.internal`：PostgreSQL 需在
  Docker 网桥地址上监听（`listen_addresses`），并在 `pg_hba.conf` 允许网桥网段使用密码认证。
  托管数据库使用 `ANIX_CONTROL_DATABASE_SSLMODE=require` 或 `verify-full`。
- 容器以只读根文件系统、uid 10001、`cap_drop: ALL` 运行；`/tmp` 是可执行 tmpfs
  （插件宿主进程与 ansible 状态）。已验证的插件包副本放在 `plugin-artifacts` 卷
  （`/var/lib/anixops`，约 0.5 GB 磁盘，可随时删除，启动时会按需从数据库重建）。
- 迁移到新机器：在新机器上放同样的 `docker-compose.prod.yml`、`.env`、`control.env`、
  `secrets/`，指向同一个数据库（或先 `pg_dump -Fc` / `pg_restore` 迁移数据库），然后
  `docker compose -f docker-compose.prod.yml up -d`。
- 开发环境可直接 `docker compose up -d --build`（`docker-compose.yml`，从源码构建并自带一个开发用 PostgreSQL）。

### 2.0.1 Kubernetes（Helm，单副本）

Chart 位于 [`config/deploy/helm/anix-control`](../config/deploy/helm/anix-control/README.md)：
单副本、`Recreate` 更新策略、`migrate` init container、`/livez`/`/readyz` 探针、
只读根文件系统，数据库为外部 PostgreSQL。

```bash
kubectl create namespace anix
kubectl -n anix create secret generic anix-control \
  --from-literal=jwt_secret="$(openssl rand -hex 32)" \
  --from-literal=db_password='数据库密码'
helm install control config/deploy/helm/anix-control -n anix \
  --set secrets.existingSecret=anix-control \
  --set image.digest=sha256:<docker-image.txt 中的 digest> \
  --set config.ANIX_CONTROL_DATABASE_HOST=postgres.example.internal
kubectl -n anix rollout status deploy/control-anix-control
```

多副本（HA）尚未支持：chart 会拒绝 `replicaCount` 大于 1。

### 2.1 一键安装脚本（systemd，已冻结）

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

该 systemd 安装路径已冻结：保持可用，但不再增加功能；新部署请使用第 2.0 节的容器方式。历史源码/Docker 安装器（`ANIX_CONTROL_LEGACY_SOURCE_INSTALL=1`）已移除，因为它会在目标主机上构建镜像。

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
- 容器内 SSH 目录为 `/home/anixops/.ssh`：在 `docker-compose.prod.yml` 中取消注释 inventory 与 SSH 的只读挂载。
- 启动时会读取 `forward_runtime`（配置文件或 `ANIX_CONTROL_FORWARD_RUNTIME_*` 环境变量），并把这些值同步到系统配置表。
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
- GitHub Release 中的 `anix-control-frontend.tar.gz` 是唯一发行前端来源（`v4.1.0-rc.2` 及更早的版本另附内容相同的 `anix-control-frontend.zip`）。
- 官方插件包（`.anxp`、`.manifest.json`、`.manifest.sig`、`.sbom.spdx.json`）从 `v4.1.0-rc.3` 起统一放在已签名的 `anix-control-packages-<version>.tar.gz` 中（签名为同名 `.sig`）；先校验签名再解压所需的包，命令见 [`UPGRADE.md`](UPGRADE.md#getting-a-package-from-the-release)。`identity-platform` 引导包仍单独附在 Release 中。
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

见第 2.0 节：`docker-compose.prod.yml` 只运行 Control（`migrate` + `control`），
数据库、监控与反向代理由宿主机或平台提供。Prometheus 抓取示例见
[`config/deploy/examples/prometheus/prometheus.yml`](../config/deploy/examples/prometheus/prometheus.yml)；
`/metrics` 无认证，只应在私有地址上暴露。

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

### 6.1 Reverse proxies：反向代理与 `server.trusted_proxies`

**从 4.1.0-rc.3 起，Control 只信任 `server.trusted_proxies`
（`ANIX_CONTROL_SERVER_TRUSTED_PROXIES`）中列出的反向代理发来的转发头。**
TCP 对端不在列表内时，`X-Forwarded-Proto`、`X-Forwarded-Host`、
`X-Forwarded-For`、`X-Real-IP` 一律忽略，scheme / host / 客户端 IP 取自连接本身
（TLS 连接为 https，`Host` 头，对端地址）。`Forwarded`（RFC 7239）不读取。

这些值决定 Control 发出的链接：clean agent 安装脚本中的面板地址、订阅链接、
Telegram webhook 地址、发给 package host 的 `request_scheme`/`request_host`，
以及限流、登录节流和审计日志使用的客户端 IP。

| 设置 | 含义 |
|---|---|
| 未设置（默认） | `127.0.0.1/32,::1/128`：只信任本机代理（含 Control 自带的 UI 端口 3000，它把 `/api` 转发到 API 端口） |
| `[]` / 空环境变量 | 不信任任何代理 |
| IP 或 CIDR 列表 | 只信任这些地址；**会替换默认值**，本机代理仍需要时请保留 `127.0.0.1/32,::1/128` |

规则：

- 代理必须**覆盖**（set）而不是追加 `X-Forwarded-Proto`/`X-Forwarded-Host`；
  有多个值时 Control 取最后一个（最近一层代理写入的值）。
- `X-Forwarded-Host` 必须是合法的 `host[:port]`（无 scheme、路径、空格或 CRLF），
  否则忽略；`X-Forwarded-Proto` 只接受 `http`/`https`。
- `X-Forwarded-For` 从右向左跳过可信代理，取第一个不可信地址（与 gin 的
  `ClientIP()` 一致）。
- 配置了公开地址时优先使用配置，请求只是后备：
  `forward_runtime.clean_agent.public_url`（安装脚本）、系统设置
  `app.subscribe_domains`（订阅链接的 host）、Telegram webhook 请求体中的 `url`。
  生产环境建议都配置上。
- 列表中的每个地址都能伪造客户端 IP 和链接 host：只列出真正的代理，
  不要把整个内网或 `0.0.0.0/0` 放进去。

**Nginx（与 Control 同机）**：默认值即可。

```nginx
server {
    listen 443 ssl;
    server_name panel.example.com;
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    }
}
```

Nginx 在另一台主机（例如 `10.0.5.20`）上时：

```bash
ANIX_CONTROL_SERVER_TRUSTED_PROXIES=127.0.0.1/32,::1/128,10.0.5.20
```

**Caddy**：Caddy 自动设置 `X-Forwarded-For/Proto/Host`，并覆盖来自不可信客户端的同名头。

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:3000
}
```

**Traefik（Docker）**：Traefik 默认设置 `X-Forwarded-*`，并丢弃来自不在
`entryPoints.<name>.forwardedHeaders.trustedIPs` 中客户端的同名头。给共享网络固定子网，
再把 Traefik 所在子网（或固定的容器 IP）加入 Control：

```yaml
networks:
  edge:
    ipam:
      config:
        - subnet: 172.30.0.0/24
```

```bash
ANIX_CONTROL_SERVER_TRUSTED_PROXIES=127.0.0.1/32,::1/128,172.30.0.0/24
```

**Docker Compose（`docker-compose.prod.yml`）**：端口默认只发布在宿主机 `127.0.0.1`，
宿主机上的代理经 Docker 网桥网关到达容器，对端是网关地址（`172.16.0.0/12` 范围内）。
`config/deploy/compose/control.env.example` 已包含 `172.16.0.0/12`；可收窄为
`docker network inspect anix-control_default` 中的网关地址。若直接把 Control
发布到公网地址（`ANIX_CONTROL_BIND=0.0.0.0`）且前面没有代理，请改回
`127.0.0.1/32,::1/128`。

**Kubernetes（Helm + Ingress）**：对端是 Ingress Controller 的 Pod IP。chart 默认值
信任 `127.0.0.1/32,::1/128` 和三个私有网段；请收窄为集群的 Pod CIDR 或 Ingress
Controller 所在网段（`kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}'`）。
ingress-nginx 默认（`use-forwarded-headers: "false"`）会覆盖 `X-Forwarded-Proto/Host`。
不要在信任私有网段的同时把 Service 以 `LoadBalancer`/`NodePort` 直接暴露。

```bash
helm upgrade control config/deploy/helm/anix-control -n anix --reuse-values \
  --set-string config.ANIX_CONTROL_SERVER_TRUSTED_PROXIES='127.0.0.1/32\,::1/128\,10.244.0.0/16'
```

**检查**：从一台不在列表中的机器直接访问 Control，伪造的头不得出现在输出中：

```bash
curl -s -H 'X-Forwarded-Host: evil.example' -H 'X-Forwarded-Proto: https' \
  http://<control 地址>:8080/api/v2/forward-agent/install.sh | grep PANEL_URL
# 期望：PANEL_URL 为 public_url 或你访问时使用的地址，而不是 https://evil.example
```

经由代理访问时链接变成 `http://` 或内部地址，说明代理的地址不在
`server.trusted_proxies` 中（或代理没有设置 `X-Forwarded-Proto`）。

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
