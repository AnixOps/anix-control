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
git clone https://github.com/anixops/v2board.git
cd v2board

# 2) 准备配置
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3) 按需修改配置（至少设置 jwt.secret 与 app.api_token）
# nano config/config.yaml

# 4) 启动
docker-compose up -d

# 5) 查看状态与日志
docker-compose ps
docker-compose logs -f
```

默认访问地址：`http://localhost:8080`

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
- 监控：Prometheus + Grafana（可选）

### 启动生产编排

```bash
docker-compose -f docker-compose.prod.yml up -d
```

如需监控组件：

```bash
docker-compose -f docker-compose.prod.yml --profile monitoring up -d
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

app:
  api_token: "your-node-communication-token"
```

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
# 检查 api_token
grep api_token config/config.yaml

# 检查节点接口
curl "http://localhost:8080/api/v2/server/UniProxy/config?node_id=1&token=your_token"
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
