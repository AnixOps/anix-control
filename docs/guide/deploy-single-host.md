# 单机部署

这是另一套部署方式：面板和节点都连同一台机器，**没有边缘代理，没有 `grpc.edge_proxies`**。

- 这台机器上运行 Control（Docker Compose）和 nginx。nginx 在 443 上终止网页的 TLS，并反代到 Control 的 8080。
- 节点的 gRPC 直接连这台机器的 50051。Control 自己终止这个端口的 TLS，证书与网页用的是同一个域名的证书。
- 证书由 certbot 申请，保存在这台机器上。

如果你的面板要跨机器部署（面板与 nginx 或节点分开），用 [双机部署](deploy-two-hosts.md)。

下面的例子用 `new-x.anixops.com`，换成你自己的。

## 前置条件

1. 域名的 A 记录指向这台机器。**不要**开启 CDN 代理：节点的 gRPC 必须直连，CDN 无法转发 Agent 的双向 TLS。
2. 80 和 443 空闲（nginx 用），50051 对公网开放（节点用）。8080 只在本机。
3. 外部 PostgreSQL 已就绪，连接方式见 [DEPLOYMENT.md 第 2 节](../DEPLOYMENT.md)。
4. 已安装 Docker、nginx 和 certbot。

## 第 1 步：申请证书

```bash
sudo certbot certonly --nginx -d new-x.anixops.com
```

或用 `--standalone`（这时 nginx 还没启动）。证书写在 `/etc/letsencrypt/live/new-x.anixops.com/`。这一张证书同时给网页和节点的 gRPC 用。

## 第 2 步：nginx 反代网页

`/etc/nginx/sites-available/anix-control.conf`：

```nginx
server {
    listen 80;
    server_name new-x.anixops.com;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name new-x.anixops.com;
    ssl_certificate     /etc/letsencrypt/live/new-x.anixops.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/new-x.anixops.com/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For   $remote_addr;
        proxy_read_timeout 120s;
    }
}
```

```bash
sudo ln -s /etc/nginx/sites-available/anix-control.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

nginx 与 Control 在同一台机器上，所以 `server.trusted_proxies` 保持默认的 `127.0.0.1/32,::1/128` 即可。

## 第 3 步：部署 Control

按 [DEPLOYMENT.md 第 2.0 节](../DEPLOYMENT.md#20-docker-compose--外部-postgresql) 取得 Compose 文件和模板。

`.env`：

```bash
ANIX_CONTROL_IMAGE=ghcr.io/anixops/anix-control@sha256:<docker-image.txt 中的 digest>
ANIX_CONTROL_BIND=127.0.0.1         # 8080 只在本机，由 nginx 反代
ANIX_CONTROL_GRPC_BIND=0.0.0.0      # 50051 对公网开放，节点直连
```

`control.env`：

```bash
ANIX_CONTROL_DATABASE_HOST=<数据库地址>
ANIX_CONTROL_DATABASE_PORT=5432
ANIX_CONTROL_DATABASE_USERNAME=anix_control
ANIX_CONTROL_DATABASE_DATABASE=anix_control
ANIX_CONTROL_DATABASE_SSLMODE=require

ANIX_CONTROL_DOMAIN=new-x.anixops.com
ANIX_CONTROL_TLS_MODE=external       # nginx 终止网页的 TLS
```

密钥与 gRPC 证书。`--grpc-name` 会读取上一步 certbot 的证书，复制到 `tls/` 并在 `control.env` 里启用 gRPC 的 TLS：

```bash
install -d -m 0750 secrets
printf '%s' '<数据库密码>' | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/db_password
sudo bash init-secrets.sh --grpc-name new-x.anixops.com
```

启动并领取管理员：

```bash
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps                  # control 应为 healthy
curl -fsS http://127.0.0.1:8080/readyz
docker compose -f docker-compose.prod.yml exec control /app/anix-control admin claim-link
```

在浏览器打开 `https://new-x.anixops.com/claim#anixclaim_…`，设置管理员。

`init-secrets.sh` 会在 `control.env` 里写入 `ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=new-x.anixops.com:50051`，节点直接拨这个地址。

## 第 4 步：防火墙

```bash
sudo ufw allow 443/tcp
sudo ufw allow 50051/tcp
sudo ufw allow 80/tcp        # 续期的 HTTP-01 与跳转
```

8080 不需要对外开放。

## 第 5 步：验证

```bash
curl -fsSI https://new-x.anixops.com/install.sh          # 200，内容是 shell 脚本
openssl s_client -connect new-x.anixops.com:50051 -servername new-x.anixops.com -alpn h2 </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates         # 公网 CA 签发，名字是 new-x.anixops.com
docker compose -f docker-compose.prod.yml exec control /app/anix-control doctor
```

登录面板，“节点”页的接入就绪检查应全部通过。然后在节点页点“接入节点”，复制命令到 VPS 上以 root 执行。节点会直连 `new-x.anixops.com:50051`。

## 证书续期

certbot 会自动续期。续期后要把新证书复制到 `tls/`，并重启 Control。把这一步挂到续期的部署钩子里：

```bash
sudo certbot renew --deploy-hook \
  "cd /opt/anix-control && bash init-secrets.sh --grpc-name new-x.anixops.com && docker compose -f docker-compose.prod.yml restart control && systemctl reload nginx"
```

`init-secrets.sh` 只复制证书，不会改动已经生成的密钥。

## 排错

| 现象 | 处理 |
|---|---|
| `init-secrets.sh` 报找不到证书 | 先完成第 1 步，确认 `/etc/letsencrypt/live/new-x.anixops.com/` 存在 |
| 节点报 `x509: certificate signed by unknown authority` | 节点拨的不是这个域名，或证书不是公网 CA 签发。检查 `ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET` |
| 节点连不上 50051 | 防火墙没放行，或 `.env` 里 `ANIX_CONTROL_GRPC_BIND` 仍是 `127.0.0.1` |
| 续期后节点报证书错误 | 部署钩子没有执行成功。手动运行 `init-secrets.sh --grpc-name …`，再 `restart control` |

## 与双机部署的区别

| 项 | 单机 | 双机 |
|---|---|---|
| 公网证书 | 在这台机器上 | 只在边缘机 B 上 |
| 节点 gRPC | 直连 Control 的 50051（Control 终止 TLS） | 经 B 的 443 转到 A 的 50051（B 终止 TLS，`grpc.edge_proxies`） |
| 身份的依据 | 节点证书，在 TLS 握手里证明 | 边缘转发的头，B 被攻破则可冒充节点 |
| 配置键 | 不用 `grpc.edge_proxies` | 用 `grpc.edge_proxies` |
