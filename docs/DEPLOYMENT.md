# V2Board AnixOps 部署指南

## 目录

1. [环境要求](#环境要求)
2. [快速部署](#快速部署)
3. [生产部署](#生产部署)
4. [配置说明](#配置说明)
5. [SSL 证书](#ssl-证书)
6. [监控告警](#监控告警)
7. [备份恢复](#备份恢复)
8. [故障排查](#故障排查)

---

## 环境要求

### 最低要求

- CPU: 1 核
- 内存: 512MB
- 磁盘: 10GB
- 操作系统: Linux / macOS / Windows

### 推荐配置

- CPU: 2 核+
- 内存: 2GB+
- 磁盘: 50GB+ SSD
- 操作系统: Ubuntu 22.04 / Debian 12

### 软件依赖

| 软件 | 版本 | 说明 |
|------|------|------|
| Docker | 24.0+ | 容器运行时 |
| Docker Compose | 2.0+ | 容器编排 |
| Go | 1.24+ | 本地开发 |
| Node.js | 20+ | 前端构建 |

---

## 快速部署

### Docker Compose (推荐)

```bash
# 1. 克隆仓库
git clone https://github.com/anixops/v2board.git
cd v2board

# 2. 创建配置
cp config/config.yaml.example config/config.yaml
cp .env.example .env

# 3. 修改必要配置
nano config/config.yaml
# 设置 jwt.secret 和 app.api_token

# 4. 启动服务
docker-compose up -d

# 5. 检查状态
docker-compose ps
docker-compose logs -f api
```

访问 `http://localhost:8080` 查看前端界面。

### 二进制部署

```bash
# 1. 编译
go build -o v2board ./cmd/server

# 2. 构建前端
cd web && npm ci && npm run build && cd ..

# 3. 创建目录
mkdir -p data logs public

# 4. 复制前端文件
cp -r web/dist/* public/

# 5. 运行
./v2board -config config/config.yaml
```

---

## 生产部署

### 1. 服务器准备

```bash
# 更新系统
apt update && apt upgrade -y

# 安装 Docker
curl -fsSL https://get.docker.com | sh
systemctl enable docker
systemctl start docker

# 安装 Docker Compose
apt install docker-compose-plugin

# 创建应用目录
mkdir -p /opt/v2board
cd /opt/v2board
```

### 2. 配置文件

```bash
# 创建配置目录
mkdir -p config data logs public

# 创建配置文件
cat > config/config.yaml << 'EOF'
env: "production"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

tls:
  enable: false

database:
  driver: "postgres"
  host: "db"
  port: 5432
  database: "v2board"
  username: "v2board"
  password: "${DB_PASSWORD}"

cache:
  driver: "redis"
  host: "redis"
  port: 6379
  password: "${REDIS_PASSWORD}"

jwt:
  secret: "${JWT_SECRET}"
  expire: 86400

app:
  name: "V2Board"
  version: "2.0.0"
  api_token: "${API_TOKEN}"
EOF

# 创建环境变量
cat > .env << 'EOF'
TZ=Asia/Shanghai
VERSION=latest

DB_PASSWORD=your_secure_password
REDIS_PASSWORD=your_redis_password
JWT_SECRET=your_jwt_secret_at_least_32_chars
API_TOKEN=your_api_token

GRAFANA_ADMIN=admin
GRAFANA_PASSWORD=your_grafana_password
EOF
```

### 3. SSL 证书

```bash
# 创建证书目录
mkdir -p docker/nginx/ssl

# 使用 Let's Encrypt (推荐)
# 安装 certbot
apt install certbot

# 获取证书
certbot certonly --standalone -d panel.example.com

# 复制证书
cp /etc/letsencrypt/live/panel.example.com/fullchain.pem docker/nginx/ssl/cert.pem
cp /etc/letsencrypt/live/panel.example.com/privkey.pem docker/nginx/ssl/key.pem

# 设置自动续签
cat > /etc/cron.d/certbot << 'EOF'
0 3 * * * root certbot renew --quiet --post-hook "docker-compose -f /opt/v2board/docker-compose.prod.yml restart nginx"
EOF
```

### 4. 启动服务

```bash
# 启动基础服务
docker-compose -f docker-compose.prod.yml up -d

# 检查状态
docker-compose -f docker-compose.prod.yml ps
docker-compose -f docker-compose.prod.yml logs -f api
```

### 5. 启用监控 (可选)

```bash
# 启动 Prometheus + Grafana
docker-compose -f docker-compose.prod.yml --profile monitoring up -d

# 访问 Grafana
# http://your-server:3001
# 默认账号: admin / your_grafana_password
```

---

## 配置说明

### 数据库配置

#### SQLite (默认)

```yaml
database:
  driver: "sqlite"
  database: "data/v2board.db"
```

适用于小型部署，无需额外配置。

#### PostgreSQL

```yaml
database:
  driver: "postgres"
  host: "localhost"
  port: 5432
  database: "v2board"
  username: "v2board"
  password: "password"
  sslmode: "disable"
```

推荐用于生产环境。

### 缓存配置

#### 内存缓存 (默认)

```yaml
cache:
  driver: "memory"
```

适用于单实例部署。

#### Redis

```yaml
cache:
  driver: "redis"
  host: "localhost"
  port: 6379
  password: ""
  db: 0
```

推荐用于多实例部署。

### JWT 配置

```yaml
jwt:
  secret: "your-secret-key-at-least-32-characters"
  expire: 86400  # 24 小时
```

**重要**: JWT secret 必须是至少 32 字符的随机字符串！

---

## 监控告警

### Prometheus 指标

系统提供以下指标：

- `v2board_http_requests_total` - HTTP 请求总数
- `v2board_http_request_duration_seconds` - HTTP 请求耗时
- `v2board_active_users` - 活跃用户数
- `v2board_active_nodes` - 活跃节点数

### Grafana Dashboard

导入预置 Dashboard：

```bash
# 导入 JSON 文件
# 位于 docker/grafana/dashboards/
```

### 告警规则

配置 Prometheus 告警规则：

```yaml
# docker/prometheus/alerts.yml
groups:
  - name: v2board
    rules:
      - alert: HighErrorRate
        expr: rate(v2board_http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
```

---

## 备份恢复

### 数据库备份

```bash
# SQLite
cp data/v2board.db data/v2board.db.backup

# PostgreSQL
docker exec v2board-db pg_dump -U v2board v2board > backup.sql
```

### 自动备份脚本

```bash
#!/bin/bash
# /opt/v2board/backup.sh

BACKUP_DIR="/opt/v2board/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 备份数据库
docker exec v2board-db pg_dump -U v2board v2board | gzip > $BACKUP_DIR/db_$DATE.sql.gz

# 保留最近 7 天
find $BACKUP_DIR -name "*.gz" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/db_$DATE.sql.gz"
```

添加到 crontab：

```bash
# 每天凌晨 2 点备份
0 2 * * * /opt/v2board/backup.sh >> /opt/v2board/logs/backup.log 2>&1
```

### 数据恢复

```bash
# PostgreSQL
gunzip -c backup.sql.gz | docker exec -i v2board-db psql -U v2board v2board
```

---

## 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 检查日志
docker-compose logs api

# 常见原因:
# - 配置文件错误
# - 数据库连接失败
# - 端口被占用
```

#### 2. 数据库连接失败

```bash
# 检查数据库状态
docker-compose exec db pg_isready

# 检查连接配置
docker-compose exec api env | grep DB
```

#### 3. 前端无法访问

```bash
# 检查 Nginx 配置
docker-compose exec nginx nginx -t

# 检查前端文件
ls -la public/
```

#### 4. 节点无法连接

```bash
# 检查 API Token
grep api_token config/config.yaml

# 测试 API
curl "http://localhost:8080/api/v2/server/UniProxy/config?node_id=1&token=your_token"
```

### 日志查看

```bash
# 查看所有日志
docker-compose logs -f

# 查看特定服务
docker-compose logs -f api
docker-compose logs -f nginx

# 查看最近 100 行
docker-compose logs --tail=100 api
```

### 性能调优

#### PostgreSQL

```sql
-- 查看连接数
SELECT count(*) FROM pg_stat_activity;

-- 查看慢查询
SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;
```

#### Redis

```bash
# 查看内存使用
docker-compose exec redis redis-cli info memory

# 查看连接数
docker-compose exec redis redis-cli info clients
```

---

## 更新升级

```bash
# 1. 备份数据
/opt/v2board/backup.sh

# 2. 拉取最新代码
git pull origin main

# 3. 重新构建
docker-compose -f docker-compose.prod.yml build

# 4. 重启服务
docker-compose -f docker-compose.prod.yml up -d

# 5. 检查状态
docker-compose -f docker-compose.prod.yml logs -f api
```

---

*最后更新: 2026-03-14*