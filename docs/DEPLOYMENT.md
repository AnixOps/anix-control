# V2Board AnixOps 部署指南

本文档覆盖本项目的常见部署方式：本地开发、Docker Compose、生产环境部署与运维。

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
| Go | 1.24+ | 本地构建后端 |
| Node.js | 20+ | 本地构建前端 |

---

## 2. 快速部署（Docker Compose）

```bash
# 1) 克隆仓库
git clone https://github.com/AnixOps/v2board_AnixOps.git
cd v2board_AnixOps

# 2) 准备配置
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3) 按需修改配置（至少设置 jwt.secret）
# nano config/config.yaml

# 4) 启动
docker-compose up -d

# 5) 查看状态与日志
docker-compose ps
docker-compose logs -f
```

默认访问地址：`http://localhost:8080`

默认前端地址：`http://localhost:3000`

### 2.1 一键安装脚本

本仓库提供与 `flux-panel` 类似的菜单式一键安装脚本，可直接通过 `raw.githubusercontent.com` 执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/AnixOps/v2board_AnixOps/go_dev/install.sh)
```

如果你已经在仓库根目录，也可以用兼容上游命名的本地包装器：

```bash
bash ./panel_install.sh
```

安装器会完成以下动作：

1. 下载或更新仓库源码归档。
2. 生成 `config/config.yaml` 与 `.env`。
3. 构建带 `ansible-playbook` 的应用镜像。
4. 启动 Docker Compose。
5. 通过环境变量在启动阶段预写入双运行时示例配置。

### 2.2 Docker 内置 ansible-playbook

运行时镜像已内置：

- `ansible`
- `openssh-client`
- `bash`

默认容器内路径：

- ansible 配置：`/app/config/deploy/ansible/ansible.cfg`
- inventory：`/app/config/deploy/ansible/inventory.ini`
- 应用 playbook：`/app/config/deploy/ansible/playbooks/forward_apply.yml`
- 删除 playbook：`/app/config/deploy/ansible/playbooks/forward_remove.yml`

默认 system config JSON 示例：

```json
{
  "inventory": "/app/config/deploy/ansible/inventory.ini",
  "playbookApply": "/app/config/deploy/ansible/playbooks/forward_apply.yml",
  "playbookRemove": "/app/config/deploy/ansible/playbooks/forward_remove.yml",
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

- `iptables_ansible` 是兼容命名的内部后端入口，用于把执行委托给可选内部执行面（文档中统称 `NodeX`），不属于 `flux-panel` 原始 `/forward` 页面契约。
- 示例 inventory 模板位于 `config/deploy/ansible/inventory.ini.example`，安装脚本会复制为 `inventory.ini`。
- SSH 密钥目录为 `config/deploy/ssh/`，会被挂载到容器内的 `/home/v2board/.ssh`。
- Docker 启动时会读取 `FORWARD_RUNTIME_BACKEND` 与 `FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON`，并把它们作为内部后端初始化值同步到系统配置表。
- 如果没有 SSH 私钥，可以直接在 `.env` 中设置 `FORWARD_RUNTIME_ANSIBLE_HOST`、`FORWARD_RUNTIME_ANSIBLE_USER`、`FORWARD_RUNTIME_ANSIBLE_PASSWORD`。
- 容器启动时会基于这些环境变量生成兼容 inventory，并把路径同步到运行时配置；非 root 用户可再设置 `FORWARD_RUNTIME_ANSIBLE_BECOME=true` 与 `FORWARD_RUNTIME_ANSIBLE_BECOME_PASSWORD`。
- 后端切换、运行时任务观测和部署引导应停留在系统/部署文档范围内，不应并入 Flux 克隆的 `/admin/forward` 页面。
- 公开边界说明见 `docs/guide/nodex-internal-extension.md`。

---

## 3. 本地开发部署

```bash
# 后端
go mod download
go run cmd/server/main.go

# 前端
cd web
npm install
npm run dev
```

生产构建：

```bash
# 后端
go build -o v2board ./cmd/server

# 前端
cd web
npm run build
```

---

## 4. 生产环境部署建议

### 架构建议

- 反向代理：Nginx 或 Caddy（统一 TLS 终止）
- 应用服务：`v2board`（Go 二进制）
- 数据库：PostgreSQL（优先）
- 缓存：Redis（多实例/高并发场景）
- 监控：Prometheus（可选），内置 Grafana（可选）

### 启动生产编排

```bash
docker-compose -f docker-compose.prod.yml up -d
```

如需只启动 Prometheus：

```bash
docker-compose -f docker-compose.prod.yml --profile prometheus up -d
```

如需同时启用内置 Grafana：

```bash
docker-compose -f docker-compose.prod.yml --profile prometheus --profile grafana up -d
```

---

## 5. 关键配置说明

配置文件：`config/config.yaml`

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
docker-compose logs -f

# 指定服务日志
docker-compose logs -f api
docker-compose logs -f nginx

# 最近 100 行
docker-compose logs --tail=100 api
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
docker exec v2board-db pg_dump -U v2board v2board > backup.sql
```

### 恢复（PostgreSQL）

```bash
docker exec -i v2board-db psql -U v2board v2board < backup.sql
```

建议使用 `crontab` 做每日自动备份，并设置保留策略。

---

## 9. 常见故障排查

### 1) 服务无法启动

```bash
docker-compose logs api
```

重点检查：

- `config/config.yaml` 是否有效
- 数据库连接是否可达
- 端口是否冲突

### 2) 数据库连接失败

```bash
docker-compose exec db pg_isready
```

检查数据库主机、端口、用户名、密码、数据库名。

### 3) 前端访问异常

```bash
docker-compose exec nginx nginx -t
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

```bash
# 1) 备份
# SQLite: 复制 db 文件
# PostgreSQL: pg_dump

# 2) 拉取新代码
git pull

# 3) 重新构建并重启
docker-compose -f docker-compose.prod.yml build
docker-compose -f docker-compose.prod.yml up -d

# 4) 检查日志
docker-compose -f docker-compose.prod.yml logs -f api
```

建议先在预发布环境验证，再进行生产升级。
