# 双机部署：面板机 A 与边缘机 B

这是 AnixOps Control 的主要部署方式，适用于大多数生产环境：

- **A（面板机）**只运行 Control（Docker Compose），连接外部 PostgreSQL。A 上没有任何公网证书。
- **B（边缘机）**运行 nginx，持有全部公网证书，负责 TLS：网页走 443 反代到 A，节点的 gRPC 也走 443 终止 TLS 后转给 A。
- **A 与 B 之间**只走私网（同机房内网、WireGuard 或 VPN）。gRPC 在这条链路上是明文 HTTP/2。

证书只在 B 上申请、保存和续期。A 不做 ACME，不读取 gRPC 的证书文件。

机制细节见设计文档 [`docs/design/edge-grpc/README.md`](../design/edge-grpc/README.md)。单机部署是另一套方式，见 [单机部署](deploy-single-host.md)。

```
节点 Agent ──TLS 443（grpc.new-x.anixops.com，公网证书在 B）──▶ B nginx ──明文 HTTP/2（私网）──▶ A Control:50051
浏览器   ──HTTPS 443（new-x.anixops.com，公网证书在 B）──────▶ B nginx ──HTTP（私网）─────────▶ A Control:8080
                                        B 把节点证书放进 x-anix-client-cert 头
```

下面的例子用这些名字和地址，换成你自己的：

| 项 | 例子 |
|---|---|
| 面板域名 | `new-x.anixops.com` |
| 节点 gRPC 域名 | `grpc.new-x.anixops.com`（DNS 指向 B，不走 CDN 代理） |
| A 的私网地址 | `10.0.0.1` |
| B 的私网地址 | `10.0.0.2`（A 看到的 B 的来源地址） |

## 前置条件

1. 两个域名的 A 记录都指向 B 的公网地址。若 `new-x.anixops.com` 前面有 CDN，它回源到 B；`grpc.new-x.anixops.com` 必须是 DNS-only，不能走 CDN。
2. A 与 B 之间有私网连通。A 的 8080 与 50051 只对 B 的私网地址开放，公网看不到。
3. A 上有外部 PostgreSQL（地址、库、用户、密码已就绪），连接方式见 [DEPLOYMENT.md 第 2 节](../DEPLOYMENT.md)。
4. B 上有 nginx 1.13.10 及以上版本（`grpc_pass` 需要；示例里的 `listen … http2` 在 1.25 之后会提示改用 `http2 on;`，功能相同）和 certbot。

## 第 1 步：A 上部署 Control

按 [DEPLOYMENT.md 第 2.0 节](../DEPLOYMENT.md#20-docker-compose--外部-postgresql) 取得 Compose 文件和模板，然后改成下面的配置。

`.env`（只放部署参数）：

```bash
ANIX_CONTROL_IMAGE=ghcr.io/anixops/anix-control@sha256:<docker-image.txt 中的 digest>
ANIX_CONTROL_BIND=10.0.0.1          # 8080 只在 A 的私网地址上发布
ANIX_CONTROL_GRPC_BIND=10.0.0.1     # 50051 只在 A 的私网地址上发布
```

`control.env`：

```bash
# 数据库（外部 PostgreSQL）
ANIX_CONTROL_DATABASE_HOST=<数据库地址>
ANIX_CONTROL_DATABASE_PORT=5432
ANIX_CONTROL_DATABASE_USERNAME=anix_control
ANIX_CONTROL_DATABASE_DATABASE=anix_control
ANIX_CONTROL_DATABASE_SSLMODE=require

# 面板：B 终止 TLS，Control 只收明文
ANIX_CONTROL_DOMAIN=new-x.anixops.com
ANIX_CONTROL_TLS_MODE=external

# 节点 gRPC：经 B 的 443 进入，B 把请求转到 A:50051
ANIX_CONTROL_GRPC_PUBLIC_NAME=grpc.new-x.anixops.com
ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET=grpc.new-x.anixops.com:443
ANIX_CONTROL_GRPC_EDGE_PROXIES=10.0.0.2

# 只信任 B 转发的 X-Forwarded-*（网页用）
ANIX_CONTROL_SERVER_TRUSTED_PROXIES=127.0.0.1/32,::1/128,10.0.0.2
```

几个要点：

- `ANIX_CONTROL_AGENT_INSTALL_GRPC_TARGET` 是节点拨号的地址，端口是 **443**（B 的端口）。Control 的监听端口仍是 50051，两者可以不同。
- `ANIX_CONTROL_GRPC_EDGE_PROXIES` 必须是 Control 看到的 B 的来源地址。Control 只接受这个地址的连接，其他来源的连接在接入时直接关闭。
- **不要**给 `init-secrets.sh` 传 `--grpc-name` 或证书参数。那样会在 A 上启用 gRPC 的 TLS，与边缘模式冲突，启动时会被拒绝。只生成 JWT 和 CA 密钥：

```bash
install -d -m 0750 secrets
printf '%s' '<数据库密码>' | install -m 0400 -o 10001 -g 10001 /dev/stdin secrets/db_password
sudo bash init-secrets.sh          # 不带 gRPC 参数
```

启动并领取管理员：

```bash
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps                  # control 应为 healthy
docker compose -f docker-compose.prod.yml exec control /app/anix-control admin claim-link
```

在浏览器打开打印出的 `https://new-x.anixops.com/claim#anixclaim_…`，设置管理员。链接一次有效，24 小时内有效。B 配好之前，这个链接打不开，先完成第 2 步。

## 第 2 步：A 的防火墙

A 的 8080 和 50051 只允许 B 访问。例如用 ufw：

```bash
sudo ufw allow from 10.0.0.2 to any port 8080 proto tcp
sudo ufw allow from 10.0.0.2 to any port 50051 proto tcp
sudo ufw deny 8080/tcp
sudo ufw deny 50051/tcp
```

A 只需要 B 的私网访问。SSH 等其他端口照常管理。

## 第 3 步：B 上申请证书

两个名字各申请一张证书，都在 B 上，都用 HTTP-01（80 端口由 B 的 nginx 应答）：

```bash
sudo mkdir -p /var/www/certbot
sudo certbot certonly --webroot -w /var/www/certbot -d new-x.anixops.com
sudo certbot certonly --webroot -w /var/www/certbot -d grpc.new-x.anixops.com
```

certbot 的续期由它自己的定时任务完成，证书写在 `/etc/letsencrypt/live/<name>/`。续期后让 nginx 重新加载：

```bash
sudo certbot renew --deploy-hook "systemctl reload nginx"
```

A 不参与证书。续期只发生在 B 上。

## 第 4 步：B 上配置 nginx

使用仓库里的示例 [`config/deploy/examples/nginx/edge-two-hosts.conf`](../../config/deploy/examples/nginx/edge-two-hosts.conf)：

```bash
sudo cp edge-two-hosts.conf /etc/nginx/sites-available/anix-control.conf
sudo ln -s /etc/nginx/sites-available/anix-control.conf /etc/nginx/sites-enabled/
# 把示例里的 10.0.0.1 改成 A 的私网地址，两个域名改成你的
sudo nginx -t && sudo systemctl reload nginx
```

示例里的要点，不要删：

- `grpc_pass grpc://…` 是明文 HTTP/2 转给 A。这条链路必须是私网。
- `grpc_set_header x-anix-client-cert $ssl_client_escaped_cert` 把节点的客户端证书交给 Control。B 每次都覆盖这个头，客户端自带的同名头会被替换掉。
- `grpc_set_header x-forwarded-for $remote_addr` 让 Control 记录节点的真实地址（入职时记录的公网地址就来自它）。
- `ssl_verify_client optional_no_ca`：B 请求但不强制客户端证书。新入职的 Agent 还没有证书，必须能连上。证书链由 Control 校验，B 不需要 CA 文件。
- `grpc_read_timeout 1d`：Agent 的控制流长期保持，nginx 默认的 60 秒会断开它。

## 第 5 步：验证

在 B 上或能访问 B 的机器上：

```bash
# 网页与安装脚本：应返回 200，内容是 shell 脚本，而不是 HTML
curl -fsSI https://new-x.anixops.com/install.sh
curl -fsS https://new-x.anixops.com/readyz

# gRPC 的公网证书：应是 Let's Encrypt 签发，名字是 grpc.new-x.anixops.com
openssl s_client -connect grpc.new-x.anixops.com:443 -servername grpc.new-x.anixops.com -alpn h2 </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates
```

在 A 上：

```bash
docker compose -f docker-compose.prod.yml exec control /app/anix-control doctor
```

登录面板，打开“节点”页：接入就绪检查应全部通过。其中 gRPC 证书一项写的是“由边缘代理持有”，这是正常的，证书要在 B 上检查。

## 第 6 步：接入节点

和其他部署一样：节点页点“接入节点”，复制命令到 VPS 上以 root 执行。命令里的面板地址是 `https://new-x.anixops.com`，节点会经 B 的 443 连上 gRPC。

Agent 不需要任何改动，也不需要知道 A 的地址。

## 日常运维

| 任务 | 怎么做 |
|---|---|
| 证书续期 | 只在 B 上：certbot 自动续期，部署钩子重载 nginx |
| 升级 Control | 在 A 上改 `ANIX_CONTROL_IMAGE` 的 digest，`docker compose ... up -d`；B 不动 |
| 备份 | 数据库，以及 A 的 `control-data` 卷中的 `root.key`，两者必须一起备份。`root.key` 丢了，所有 Agent 要重新接入 |
| 改 B 的地址 | 同时改 A 的 `ANIX_CONTROL_GRPC_EDGE_PROXIES`、`ANIX_CONTROL_SERVER_TRUSTED_PROXIES`，以及 B 上 nginx 的 `proxy_pass`、`grpc_pass` |

## 排错

| 现象 | 原因与处理 |
|---|---|
| Agent 报 `x509: certificate signed by unknown authority` | B 上的 `grpc.<domain>` 证书不是公网 CA 签发，或 Agent 拨的不是这个名字。检查第 3 步 |
| 节点连上了，但入职时记录的地址是 B 的 | B 没有设置 `x-forwarded-for`。检查示例里的 `grpc_set_header x-forwarded-for` |
| 控制流被 `unauthenticated` 拒绝，已入职的节点连不上 | B 没有转发客户端证书。检查 `ssl_verify_client optional_no_ca` 与 `grpc_set_header x-anix-client-cert` |
| A 的日志里有 `refused a gRPC connection from outside grpc.edge_proxies` | 来源地址不是 `ANIX_CONTROL_GRPC_EDGE_PROXIES` 里的那个。确认 B 连 A 用的是哪个私网地址 |
| 控制流几分钟后断开 | nginx 的 `grpc_read_timeout` 太短。示例里是 1d |
| 启动报错 `grpc.edge_proxies excludes grpc.tls_cert_file and tls.cert_file` | A 的配置里还有 gRPC 证书。不要给 `init-secrets.sh` 传 `--grpc-name`，并删掉 `control.env` 里它写入的 `ANIX_CONTROL_GRPC_ENABLED`、`ANIX_CONTROL_GRPC_TLS_CERT_FILE`、`ANIX_CONTROL_GRPC_TLS_KEY_FILE` 三行 |

## 安全边界（必读）

这套方式的代价，要在上线前理解：

- **B 是身份的一部分。** 直连 TLS 时，节点证明身份靠它的私钥，攻击者拿不到私钥就冒充不了。边缘模式下，身份由 B 转发的头决定。**如果 B 被攻破，攻击者可以转发任何曾经在 B 上出现过的节点证书，从而冒充这些节点。** 证书本身是公开的，区别在于它不是秘密。
- 所以 B 要当作面板的一部分来加固：只开放 443 和 SSH，系统和 nginx 及时更新，SSH 只允许密钥登录。
- A 与 B 之间的链路必须是私网。如果它暴露在公网，节点的控制流（含凭据）会明文穿过公网。
- Control 只接受 `ANIX_CONTROL_GRPC_EDGE_PROXIES` 里的来源，并且只在这些来源上读取转发的证书和地址。其他来源的头一律忽略。
