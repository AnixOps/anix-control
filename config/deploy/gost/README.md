# gost 转发节点部署指南

本目录包含 gost 转发节点的部署配置文件和脚本。

## 架构说明

```
┌─────────────────────────────────────────────────────────────────┐
│                      v2board 面板端                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  ForwardService + gost.Manager                           │    │
│  │  - 管理 ForwardNode/ForwardRule 数据模型                  │    │
│  │  - 通过 HTTP API 远程控制各转发节点                        │    │
│  │  - 同步节点状态和流量统计                                  │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │ HTTP API (端口 18080)
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                    转发节点 (运行 gost)                          │
│  ┌─────────────────┐    ┌─────────────────┐                     │
│  │  gost (中转)     │    │  gost (落地)     │                    │
│  │  Relay Node     │    │  Exit Node      │                    │
│  │  API: :18080    │    │  API: :18080    │                    │
│  │  Metrics: :9000 │    │  Metrics: :9000 │                    │
│  └─────────────────┘    └─────────────────┘                     │
└─────────────────────────────────────────────────────────────────┘
```

## 部署方式

### 方式一：Shell 脚本部署 (推荐)

```bash
# 1. 下载脚本
wget https://raw.githubusercontent.com/anixops/v2board/main/deploy/gost/deploy.sh
chmod +x deploy.sh

# 2. 运行部署 (relay=中转节点, exit=落地节点)
./deploy.sh relay your-api-token-here

# 3. 服务管理
systemctl status gost
systemctl restart gost
journalctl -u gost -f
```

### 方式二：Docker 部署

```bash
# 1. 复制配置文件
cp gost.yml.example gost.yml

# 2. 修改 API Token
vim gost.yml

# 3. 启动服务
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

### 方式三：手动部署

```bash
# 1. 下载 gost
wget https://github.com/go-gost/gost/releases/download/v3.0.0-rc10/gost_3.0.0-rc10_linux_amd64.tar.gz
tar -xzf gost_*.tar.gz
mv gost /usr/local/bin/

# 2. 创建配置目录
mkdir -p /opt/gost
cd /opt/gost

# 3. 创建配置文件
cat > gost.yml << 'EOF'
services:
  - name: api
    addr: ":18080"
    handler:
      type: api
      auth:
        username: admin
        password: YOUR_API_TOKEN
    listener:
      type: tcp

api:
  addr: ":18080"
  pathPrefix: /api
  accesslog: true
  auth:
    username: admin
    password: YOUR_API_TOKEN

log:
  output: stderr
  level: info

metrics:
  addr: :9000
  path: /metrics
EOF

# 4. 创建 systemd 服务
cat > /etc/systemd/system/gost.service << 'EOF'
[Unit]
Description=gost tunnel service
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C /opt/gost/gost.yml
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# 5. 启动服务
systemctl daemon-reload
systemctl enable gost
systemctl start gost
```

## 面板端配置

### 1. 添加转发节点

在管理后台 > 转发管理 > 中转节点 中添加：

| 字段 | 说明 |
|------|------|
| 名称 | 节点名称，如 "HK中转" |
| 类型 | relay (中转) 或 exit (落地) |
| 主机 | 节点服务器 IP |
| 端口 | SSH/管理端口 (非必须) |
| API端口 | gost API 端口，默认 18080 |
| API Token | 部署时设置的 Token |
| 地区 | HK, US, JP, SG 等 |
| 带宽 | 节点带宽 (Mbps) |

### 2. 创建转发规则

在管理后台 > 转发管理 > 转发规则 中添加：

| 字段 | 说明 |
|------|------|
| 名称 | 规则名称 |
| 中转节点 | 选择中转节点 |
| 监听端口 | 中转节点上监听的端口 |
| 协议 | tcp/udp/both |
| 落地节点 | 选择落地节点 |
| 目标地址 | 最终转发的目标地址 |
| 目标端口 | 最终转发的目标端口 |

## API 端点

gost 节点启动后，可通过以下 API 管理转发规则：

```bash
# 获取服务列表
curl -u admin:YOUR_TOKEN http://NODE_IP:18080/api/config/services

# 创建转发服务
curl -u admin:YOUR_TOKEN -X POST http://NODE_IP:18080/api/config/services \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "name": "forward-10000",
      "addr": ":10000",
      "handler": {"type": "tcp"},
      "listener": {"type": "tcp"},
      "forwarder": {
        "nodes": [{"name": "target", "addr": "1.2.3.4:80"}]
      }
    }
  }'

# 删除转发服务
curl -u admin:YOUR_TOKEN -X DELETE http://NODE_IP:18080/api/config/services/forward-10000

# 获取统计信息
curl -u admin:YOUR_TOKEN http://NODE_IP:18080/api/stats

# Prometheus 监控
curl http://NODE_IP:9000/metrics
```

## 多级转发链

配置中转 -> 落地的多级转发：

```yaml
# 在中转节点配置
chains:
  - name: chain-to-exit
    hops:
      - name: hop-exit
        nodes:
          - name: exit-node
            addr: EXIT_NODE_IP:1080
            connector:
              type: socks5
              auth:
                username: user
                password: pass
            dialer:
              type: tcp

services:
  - name: forward-10000
    addr: ":10000"
    handler:
      type: tcp
      chain: chain-to-exit
    listener:
      type: tcp
    forwarder:
      nodes:
        - name: target
          addr: TARGET_HOST:TARGET_PORT
```

## 监控告警

### Prometheus 配置

```yaml
scrape_configs:
  - job_name: 'gost'
    static_configs:
      - targets: ['relay1:9000', 'relay2:9000', 'exit1:9000']
```

### Grafana Dashboard

可使用标准 Prometheus 指标创建监控面板，主要指标：
- `gost_service_connections_current` - 当前连接数
- `gost_service_traffic_bytes_total` - 总流量
- `gost_service_requests_total` - 总请求数

## 故障排查

```bash
# 检查服务状态
systemctl status gost

# 查看日志
journalctl -u gost -f

# 检查端口
netstat -tlnp | grep 18080
netstat -tlnp | grep 9000

# 测试 API
curl -u admin:YOUR_TOKEN http://localhost:18080/api/config/services

# 检查防火墙
iptables -L -n
```

## 安全建议

1. **更改默认端口**：不要使用默认的 18080 端口
2. **强密码**：使用足够复杂的 API Token
3. **防火墙**：只允许面板 IP 访问 API 端口
4. **TLS**：生产环境启用 TLS 加密
5. **定期更新**：保持 gost 版本更新