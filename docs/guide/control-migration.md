# AnixOps Control 原位迁移

迁移器只支持当前 AnixOps SQLite 布局，不执行 PostgreSQL 切换，也不改变旧
`v2board.service` 的数据目录。生产执行前应在隔离主机用相同版本标签演练。

## 预检

```bash
sudo bash scripts/install.sh preflight --plan /root/anixops-migration-plan.json
```

预检只读取旧布局并写出脱敏 JSON：路径、服务状态、磁盘可用空间、依赖状态和
配置/数据库/服务单元 SHA-256；不会输出 JWT、支付凭证或 API key。

## 迁移

```bash
sudo bash scripts/install.sh migrate --version v4.2.0
```

`--version` 必须是已发布的发布 tag：把 `v4.2.0` 换成要迁移到的版本，并先在
[发布页](https://github.com/AnixOps/anix-control/releases)确认该 tag 存在。不存在的
tag 在下载阶段返回 404，此时旧服务尚未停止。

发布资产和 `SHA256SUMS.txt` 会在停旧服务前下载并校验。成功后程序、配置、数据、
日志和服务分别位于 `/opt/anixops/control`、`/etc/anixops/control`、
`/var/lib/anixops/control`、`/var/log/anixops/control` 和
`anix-control.service`。原 `/usr/local/v2board`、`/etc/v2board`、
`/var/lib/v2board` 保留不变，迁移快照位于 `/var/backups/anixops-control`。

生产节点清单固定为 `/etc/anixops/control/inventory.ini`。迁移器保留已存在
的清单，不复制或继续使用旧 `ssh.env`。

已有管理员不会被读取安装参数覆盖，也不会重新初始化。迁移失败会恢复旧服务单元、
二进制并重新启动 `v2board.service`。

## 回滚

```bash
sudo bash scripts/install.sh rollback
```

回滚使用最近一次迁移快照恢复旧服务；快照应保留到人工验收窗口结束。

## Agent 灰度

在面板迁移验收后，对排空节点逐个切换到新 Agent。

`scripts/install-agent.sh migrate` 目前不能使用：它从 `AnixOps/anix-agent` 的发布页下载
`anix-agent-linux-amd64.tar.gz`（arm64 为 `anix-agent-linux-arm64.tar.gz`）和
`SHA256SUMS.txt`，而该仓库的所有发布都没有这些资产，第一次 `curl -fL` 就得到 404。
发布实际提供的是 `anix-agent-linux-64.zip`、`anix-agent-linux-arm64-v8a.zip` 和各自的
`.dgst`（`v4.2.0-rc.1` 起另有 `.sig`、`SHA256SUMS` 和 `SHA256SUMS.sig`）。请改用：

- 节点页面的“复制安装命令”（需要 4.2 Control，见
  [Agent 接入指南](agent-onboarding.md)）；
- 仍是旧 V2bX 布局的节点，用 Agent 自带的 root 安装器（anix-agent 仓库
  `docs/INSTALL.md` 的 “Root Install With scripts/install.sh” 和
  “Legacy V2bX_AnixOps Upgrade”）：它检测 `/usr/local/V2bX`、`/etc/V2bX` 和
  `V2bX.service`，只复制新配置目录缺少的文件，备份旧二进制和服务定义，再切换到
  `anix-agent.service`，旧目录保留。

一次只迁移一个 canary 节点，确认 `agent.ping`、`node.reload`、`users.reload`、
配置/用户同步及流量/在线上报后再扩展批次。
