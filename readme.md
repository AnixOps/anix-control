# V2Board AnixOps

鐜颁唬鍖栫殑浠ｇ悊闈㈡澘绠＄悊绯荤粺锛孏o + Vue 3 鎶€鏈爤銆?

## 鐗规€?

- **楂樻€ц兘鍚庣** - Go 1.24 + Gin锛屾敮鎸侀珮骞跺彂
- **鐜颁唬鍓嶇** - Vue 3 + Vite + Element Plus
- **澶氬崗璁敮鎸?* - VMess/VLESS/Trojan/Shadowsocks/Hysteria2/TUIC
- **澶氳闃呮牸寮?* - V2Ray/Clash/Sing-box/Surge
- **gRPC 閫氫俊** - 鑺傜偣涓庨潰鏉垮弻鍚戞祦閫氫俊
- **Agent 绯荤粺** - NAT 绌块€忥紝杩滅▼鑺傜偣绠＄悊
- **娴侀噺杞彂** - gost 闆嗘垚锛屼腑杞妭鐐圭鐞?
- **闆朵緷璧栭儴缃?* - 鍗曚簩杩涘埗鏂囦欢 + SQLite锛屾棤闇€瀹夎浠讳綍澶栭儴鏈嶅姟

## 鎶€鏈爤

| 缁勪欢 | 鎶€鏈?| 璇存槑 |
|------|------|------|
| 璇█ | Go 1.24+ | 楂樻€ц兘銆侀潤鎬佺紪璇戙€佽法骞冲彴 |
| Web妗嗘灦 | Gin | 楂樻€ц兘 HTTP 妗嗘灦 |
| ORM | GORM | Go 璇█ ORM 妗嗘灦 |
| 鏁版嵁搴?| SQLite / PostgreSQL | 杞婚噺绾ф垨浼佷笟绾?|
| 缂撳瓨 | 鍐呭瓨缂撳瓨 / Redis | 闆朵緷璧栨垨鍒嗗竷寮?|
| 鍓嶇 | Vue 3 + Vite | 鐜颁唬鍓嶇妗嗘灦 |
| 閫氫俊 | gRPC + WebSocket | 鍙屽悜瀹炴椂閫氫俊 |

## 椤圭洰缁撴瀯

```
v2board_AnixOps/
鈹溾攢鈹€ cmd/server/           # 涓荤▼搴忓叆鍙?
鈹溾攢鈹€ config/               # 閰嶇疆鏂囦欢
鈹溾攢鈹€ internal/             # Go 鍚庣浠ｇ爜
鈹?  鈹溾攢鈹€ handler/          # HTTP 澶勭悊鍣?
鈹?  鈹溾攢鈹€ service/          # 涓氬姟閫昏緫
鈹?  鈹溾攢鈹€ model/            # 鏁版嵁妯″瀷
鈹?  鈹溾攢鈹€ grpc/             # gRPC 鏈嶅姟
鈹?  鈹溾攢鈹€ gost/             # gost 瀹㈡埛绔?
鈹?  鈹斺攢鈹€ ...
鈹溾攢鈹€ web/                  # Vue 鍓嶇
鈹?  鈹溾攢鈹€ src/views/        # 椤甸潰缁勪欢
鈹?  鈹溾攢鈹€ src/api/          # API 璋冪敤
鈹?  鈹斺攢鈹€ ...
鈹溾攢鈹€ docker/               # Docker 閰嶇疆
鈹溾攢鈹€ docs/                 # Swagger 鏂囨。
鈹斺攢鈹€ deploy/               # 閮ㄧ讲鏂囦欢
```

## 蹇€熷紑濮?

### 浣跨敤 Docker Compose (鎺ㄨ崘)

```bash
# 1. 鍏嬮殕浠撳簱
git clone https://github.com/anixops/v2board.git
cd v2board

# 2. 澶嶅埗閰嶇疆鏂囦欢
cp .env.example .env
cp config/config.yaml.example config/config.yaml

# 3. 缂栬緫閰嶇疆
nano .env
nano config/config.yaml

# 4. 鍚姩鏈嶅姟
docker-compose up -d

# 5. 鏌ョ湅鏃ュ織
docker-compose logs -f api
```

### 鏈湴寮€鍙?

```bash
# 鍚庣
go mod download
go run cmd/server/main.go

# 鍓嶇
cd web
npm install
npm run dev
```

### 缂栬瘧

```bash
# 瀹夎渚濊禆
go mod tidy

# 缂栬瘧
go build -o v2board ./cmd/server

# 鎴栦娇鐢?make
make build
```

## 閰嶇疆璇存槑

### SQLite 閰嶇疆 (榛樿锛岄浂閰嶇疆)

```yaml
database:
  driver: "sqlite"
  database: "config/data/v2board.db"
```

### PostgreSQL 閰嶇疆

```yaml
database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  database: "v2board"
  username: "postgres"
  password: "your_password"
```

### 蹇呰閰嶇疆

```yaml
jwt:
  secret: "your-jwt-secret-at-least-32-characters"

app:
  api_token: "your-node-communication-token"
```

## 閮ㄧ讲鎸囧崡

### 寮€鍙戠幆澧?

```bash
docker-compose up -d
```

### 鐢熶骇鐜

```bash
# 浣跨敤鐢熶骇閰嶇疆
docker-compose -f docker-compose.prod.yml up -d

# 鍚敤鐩戞帶 (Prometheus + Grafana)
docker-compose -f docker-compose.prod.yml --profile monitoring up -d
```

### Systemd 鏈嶅姟

鍒涘缓 `/etc/systemd/system/v2board.service`锛?

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

## API 鏂囨。

鍚姩鏈嶅姟鍚庤闂細`http://localhost:8080/swagger/index.html`

## 涓昏鍔熻兘妯″潡

| 妯″潡 | 璇存槑 |
|------|------|
| 鐢ㄦ埛绯荤粺 | 娉ㄥ唽/鐧诲綍/璧勬枡/鏉冮檺 |
| 濂楅绯荤粺 | 璁¤垂/娴侀噺闄愬埗/缁垂 |
| 鑺傜偣绯荤粺 | 澶氬崗璁?鍒嗙粍/鑷姩娉ㄥ唽 |
| 璁㈤槄绯荤粺 | 澶氭牸寮?鍒嗙粍/妯℃澘 |
| 鏀粯绯荤粺 | 澶氭笭閬?缁熻 |
| 宸ュ崟绯荤粺 | 鍒涘缓/鍥炲/鍏抽棴 |
| Agent 绯荤粺 | NAT 绌块€?杩滅▼鎺у埗 |
| 娴侀噺杞彂 | 涓浆鑺傜偣/gost 闆嗘垚 |
| Telegram Bot | 鍛戒护/閫氱煡/骞挎挱 |
| MFA 璁よ瘉 | TOTP/澶囩敤鐮?|

## 娴嬭瘯

```bash
# 杩愯鎵€鏈夋祴璇?
go test ./...

# 杩愯甯﹁鐩栫巼鐨勬祴璇?
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

## 鐩稿叧椤圭洰

- [V2bX_AnixOps](https://github.com/anixops/V2bX_AnixOps) - 鑺傜偣绔▼搴?
- [AnixOps-agent](https://github.com/anixops/anixops-agent) - 杩滅▼鎺у埗 Agent

## License

MIT License