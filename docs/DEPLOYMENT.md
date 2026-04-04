# V2Board AnixOps 閮ㄧ讲鎸囧崡

## 鐩綍

1. [鐜瑕佹眰](#鐜瑕佹眰)
2. [蹇€熼儴缃瞉(#蹇€熼儴缃?
3. [鐢熶骇閮ㄧ讲](#鐢熶骇閮ㄧ讲)
4. [閰嶇疆璇存槑](#閰嶇疆璇存槑)
5. [SSL 璇佷功](#ssl-璇佷功)
6. [鐩戞帶鍛婅](#鐩戞帶鍛婅)
7. [澶囦唤鎭㈠](#澶囦唤鎭㈠)
8. [鏁呴殰鎺掓煡](#鏁呴殰鎺掓煡)

---

## 鐜瑕佹眰

### 鏈€浣庤姹?

- CPU: 1 鏍?
- 鍐呭瓨: 512MB
- 纾佺洏: 10GB
- 鎿嶄綔绯荤粺: Linux / macOS / Windows

### 鎺ㄨ崘閰嶇疆

- CPU: 2 鏍?
- 鍐呭瓨: 2GB+
- 纾佺洏: 50GB+ SSD
- 鎿嶄綔绯荤粺: Ubuntu 22.04 / Debian 12

### 杞欢渚濊禆

| 杞欢 | 鐗堟湰 | 璇存槑 |
|------|------|------|
| Docker | 24.0+ | 瀹瑰櫒杩愯鏃?|
| Docker Compose | 2.0+ | 瀹瑰櫒缂栨帓 |
| Go | 1.24+ | 鏈湴寮€鍙?|
| Node.js | 20+ | 鍓嶇鏋勫缓 |

---

## 蹇€熼儴缃?

### Docker Compose (鎺ㄨ崘)

```bash
# 1. 鍏嬮殕浠撳簱
git clone https://github.com/anixops/v2board.git
cd v2board

# 2. 鍒涘缓閰嶇疆
cp config/config.yaml.example config/config.yaml
cp .env.example .env

# 3. 淇敼蹇呰閰嶇疆
nano config/config.yaml
# 璁剧疆 jwt.secret 鍜?app.api_token

# 4. 鍚姩鏈嶅姟
docker-compose up -d

# 5. 妫€鏌ョ姸鎬?
docker-compose ps
docker-compose logs -f api
```

璁块棶 `http://localhost:8080` 鏌ョ湅鍓嶇鐣岄潰銆?

### 浜岃繘鍒堕儴缃?

```bash
# 1. 缂栬瘧
go build -o v2board ./cmd/server

# 2. 鏋勫缓鍓嶇
cd web && npm ci && npm run build && cd ..

# 3. 鍒涘缓鐩綍
mkdir -p config/data logs web/public

# 4. 澶嶅埗鍓嶇鏂囦欢
# 前端构建产物输出到 web/public/

# 5. 杩愯
./v2board -config config/config.yaml
```

---

## 鐢熶骇閮ㄧ讲

### 1. 鏈嶅姟鍣ㄥ噯澶?

```bash
# 鏇存柊绯荤粺
apt update && apt upgrade -y

# 瀹夎 Docker
curl -fsSL https://get.docker.com | sh
systemctl enable docker
systemctl start docker

# 瀹夎 Docker Compose
apt install docker-compose-plugin

# 鍒涘缓搴旂敤鐩綍
mkdir -p /opt/v2board
cd /opt/v2board
```

### 2. 閰嶇疆鏂囦欢

```bash
# 鍒涘缓閰嶇疆鐩綍
mkdir -p config config/data logs web/public

# 鍒涘缓閰嶇疆鏂囦欢
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

# 鍒涘缓鐜鍙橀噺
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

### 3. SSL 璇佷功

```bash
# 鍒涘缓璇佷功鐩綍
mkdir -p config/docker/nginx/ssl

# 浣跨敤 Let's Encrypt (鎺ㄨ崘)
# 瀹夎 certbot
apt install certbot

# 鑾峰彇璇佷功
certbot certonly --standalone -d panel.example.com

# 澶嶅埗璇佷功
cp /etc/letsencrypt/live/panel.example.com/fullchain.pem config/docker/nginx/ssl/cert.pem
cp /etc/letsencrypt/live/panel.example.com/privkey.pem config/docker/nginx/ssl/key.pem

# 璁剧疆鑷姩缁
cat > /etc/cron.d/certbot << 'EOF'
0 3 * * * root certbot renew --quiet --post-hook "docker-compose -f /opt/v2board/docker-compose.prod.yml restart nginx"
EOF
```

### 4. 鍚姩鏈嶅姟

```bash
# 鍚姩鍩虹鏈嶅姟
docker-compose -f docker-compose.prod.yml up -d

# 妫€鏌ョ姸鎬?
docker-compose -f docker-compose.prod.yml ps
docker-compose -f docker-compose.prod.yml logs -f api
```

### 5. 鍚敤鐩戞帶 (鍙€?

```bash
# 鍚姩 Prometheus + Grafana
docker-compose -f docker-compose.prod.yml --profile monitoring up -d

# 璁块棶 Grafana
# http://your-server:3001
# 榛樿璐﹀彿: admin / your_grafana_password
```

---

## 閰嶇疆璇存槑

### 鏁版嵁搴撻厤缃?

#### SQLite (榛樿)

```yaml
database:
  driver: "sqlite"
  database: "config/data/v2board.db"
```

閫傜敤浜庡皬鍨嬮儴缃诧紝鏃犻渶棰濆閰嶇疆銆?

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

鎺ㄨ崘鐢ㄤ簬鐢熶骇鐜銆?

### 缂撳瓨閰嶇疆

#### 鍐呭瓨缂撳瓨 (榛樿)

```yaml
cache:
  driver: "memory"
```

閫傜敤浜庡崟瀹炰緥閮ㄧ讲銆?

#### Redis

```yaml
cache:
  driver: "redis"
  host: "localhost"
  port: 6379
  password: ""
  db: 0
```

鎺ㄨ崘鐢ㄤ簬澶氬疄渚嬮儴缃层€?

### JWT 閰嶇疆

```yaml
jwt:
  secret: "your-secret-key-at-least-32-characters"
  expire: 86400  # 24 灏忔椂
```

**閲嶈**: JWT secret 蹇呴』鏄嚦灏?32 瀛楃鐨勯殢鏈哄瓧绗︿覆锛?

---

## 鐩戞帶鍛婅

### Prometheus 鎸囨爣

绯荤粺鎻愪緵浠ヤ笅鎸囨爣锛?

- `v2board_http_requests_total` - HTTP 璇锋眰鎬绘暟
- `v2board_http_request_duration_seconds` - HTTP 璇锋眰鑰楁椂
- `v2board_active_users` - 娲昏穬鐢ㄦ埛鏁?
- `v2board_active_nodes` - 娲昏穬鑺傜偣鏁?

### Grafana Dashboard

瀵煎叆棰勭疆 Dashboard锛?

```bash
# 瀵煎叆 JSON 鏂囦欢
# 浣嶄簬 config/docker/grafana/dashboards/
```

### 鍛婅瑙勫垯

閰嶇疆 Prometheus 鍛婅瑙勫垯锛?

```yaml
# config/docker/prometheus/alerts.yml
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

## 澶囦唤鎭㈠

### 鏁版嵁搴撳浠?

```bash
# SQLite
cp config/data/v2board.db config/data/v2board.db.backup

# PostgreSQL
docker exec v2board-db pg_dump -U v2board v2board > backup.sql
```

### 鑷姩澶囦唤鑴氭湰

```bash
#!/bin/bash
# /opt/v2board/backup.sh

BACKUP_DIR="/opt/v2board/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 澶囦唤鏁版嵁搴?
docker exec v2board-db pg_dump -U v2board v2board | gzip > $BACKUP_DIR/db_$DATE.sql.gz

# 淇濈暀鏈€杩?7 澶?
find $BACKUP_DIR -name "*.gz" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/db_$DATE.sql.gz"
```

娣诲姞鍒?crontab锛?

```bash
# 姣忓ぉ鍑屾櫒 2 鐐瑰浠?
0 2 * * * /opt/v2board/backup.sh >> /opt/v2board/logs/backup.log 2>&1
```

### 鏁版嵁鎭㈠

```bash
# PostgreSQL
gunzip -c backup.sql.gz | docker exec -i v2board-db psql -U v2board v2board
```

---

## 鏁呴殰鎺掓煡

### 甯歌闂

#### 1. 鏈嶅姟鏃犳硶鍚姩

```bash
# 妫€鏌ユ棩蹇?
docker-compose logs api

# 甯歌鍘熷洜:
# - 閰嶇疆鏂囦欢閿欒
# - 鏁版嵁搴撹繛鎺ュけ璐?
# - 绔彛琚崰鐢?
```

#### 2. 鏁版嵁搴撹繛鎺ュけ璐?

```bash
# 妫€鏌ユ暟鎹簱鐘舵€?
docker-compose exec db pg_isready

# 妫€鏌ヨ繛鎺ラ厤缃?
docker-compose exec api env | grep DB
```

#### 3. 鍓嶇鏃犳硶璁块棶

```bash
# 妫€鏌?Nginx 閰嶇疆
docker-compose exec nginx nginx -t

# 妫€鏌ュ墠绔枃浠?
ls -la web/public/
```

#### 4. 鑺傜偣鏃犳硶杩炴帴

```bash
# 妫€鏌?API Token
grep api_token config/config.yaml

# 娴嬭瘯 API
curl "http://localhost:8080/api/v2/server/UniProxy/config?node_id=1&token=your_token"
```

### 鏃ュ織鏌ョ湅

```bash
# 鏌ョ湅鎵€鏈夋棩蹇?
docker-compose logs -f

# 鏌ョ湅鐗瑰畾鏈嶅姟
docker-compose logs -f api
docker-compose logs -f nginx

# 鏌ョ湅鏈€杩?100 琛?
docker-compose logs --tail=100 api
```

### 鎬ц兘璋冧紭

#### PostgreSQL

```sql
-- 鏌ョ湅杩炴帴鏁?
SELECT count(*) FROM pg_stat_activity;

-- 鏌ョ湅鎱㈡煡璇?
SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;
```

#### Redis

```bash
# 鏌ョ湅鍐呭瓨浣跨敤
docker-compose exec redis redis-cli info memory

# 鏌ョ湅杩炴帴鏁?
docker-compose exec redis redis-cli info clients
```

---

## 鏇存柊鍗囩骇

```bash
# 1. 澶囦唤鏁版嵁
/opt/v2board/backup.sh

# 2. 鎷夊彇鏈€鏂颁唬鐮?
git pull origin main

# 3. 閲嶆柊鏋勫缓
docker-compose -f docker-compose.prod.yml build

# 4. 閲嶅惎鏈嶅姟
docker-compose -f docker-compose.prod.yml up -d

# 5. 妫€鏌ョ姸鎬?
docker-compose -f docker-compose.prod.yml logs -f api
```

---

*鏈€鍚庢洿鏂? 2026-03-14*