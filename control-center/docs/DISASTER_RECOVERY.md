# AnixOps Control Center - 灾难恢复手册

## 文档版本
- **版本**: 1.0.0
- **更新日期**: 2026-03-19
- **适用版本**: anixops-v1.0.0-rc.9+

---

## 一、概述

本文档描述 AnixOps Control Center 的灾难恢复（Disaster Recovery）流程，包括数据备份、恢复操作和故障排查指南。

### 1.1 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Cloudflare Workers API                    │
│  https://api.anixops.com                                    │
├─────────────────────────────────────────────────────────────┤
│  D1 Database │ KV Namespace │ R2 Storage │ SSE              │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 关键组件

| 组件 | 用途 | 数据类型 | RTO |
|------|------|----------|-----|
| D1 Database | 主数据库 | 用户、节点、任务、配置 | 1小时 |
| KV Namespace | 缓存、会话 | Token黑名单、速率限制 | 4小时 |
| R2 Storage | 文件存储 | Playbooks、备份文件 | 即时 |

---

## 二、备份策略

### 2.1 自动备份

#### D1 数据库备份
- **频率**: 每日自动备份
- **保留期**: 30天
- **存储位置**: R2 (backups/d1/)
- **触发方式**: Cloudflare Workers Cron Trigger 或手动API调用

#### 手动触发备份
```bash
# 创建备份
curl -X POST https://api.anixops.com/api/v1/backups \
  -H "Authorization: Bearer <admin-token>"

# 列出所有备份
curl https://api.anixops.com/api/v1/backups \
  -H "Authorization: Bearer <admin-token>"

# 下载备份
curl https://api.anixops.com/api/v1/backups/<backup-id>/download \
  -H "Authorization: Bearer <admin-token>" \
  -o backup.json
```

### 2.2 备份验证清单

- [ ] 每周验证备份文件完整性
- [ ] 每月执行恢复演练
- [ ] 监控备份存储空间使用量
- [ ] 验证备份数据可读性

### 2.3 配置备份命令

```bash
# Cloudflare D1 手动导出
wrangler d1 export anixops-db --output=backup-$(date +%Y%m%d).sql

# KV 数据导出（通过API）
wrangler kv:key list --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf
```

---

## 三、恢复操作

### 3.1 D1 数据库恢复

#### 场景A: 从API备份恢复

```bash
# 1. 下载备份文件
curl https://api.anixops.com/api/v1/backups/<backup-id>/download \
  -H "Authorization: Bearer <admin-token>" \
  -o backup.json

# 2. 恢复备份（通过API）
curl -X POST https://api.anixops.com/api/v1/backups/<backup-id>/restore \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"truncate": true}'
```

#### 场景B: 从SQL文件恢复

```bash
# 1. 导入SQL文件
wrangler d1 execute anixops-db --file=backup-20260319.sql

# 2. 验证数据完整性
wrangler d1 execute anixops-db --command="SELECT COUNT(*) FROM users"
```

#### 场景C: 完全重建

```bash
# 1. 删除并重建数据库
wrangler d1 delete anixops-db
wrangler d1 create anixops-db

# 2. 运行迁移
wrangler d1 migrations apply anixops-db

# 3. 导入备份
wrangler d1 execute anixops-db --file=backup.sql
```

### 3.2 KV 数据恢复

```bash
# 清除Token黑名单（紧急情况）
wrangler kv:key delete --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf "blacklist:*"

# 批量导入KV数据
wrangler kv:key put --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf \
  --path=kv-backup.json
```

### 3.3 R2 存储恢复

```bash
# 下载Playbook文件
wrangler r2 object get anixops-files/playbooks/backup.yml

# 上传恢复文件
wrangler r2 object put anixops-files/playbooks/backup.yml \
  --file=backup.yml
```

---

## 四、故障场景与恢复流程

### 4.1 场景一: 数据库损坏

**症状**:
- API返回500错误
- 数据库查询超时
- 数据不一致

**恢复步骤**:

1. **确认问题**
```bash
# 检查数据库状态
wrangler d1 execute anixops-db --command="PRAGMA integrity_check"
```

2. **停止服务（可选）**
```bash
# 禁用API（通过Cloudflare Dashboard）
```

3. **恢复备份**
```bash
# 找到最近的备份
curl https://api.anixops.com/api/v1/backups \
  -H "Authorization: Bearer <admin-token>"

# 执行恢复
curl -X POST https://api.anixops.com/api/v1/backups/<backup-id>/restore \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"truncate": true}'
```

4. **验证恢复**
```bash
# 检查用户数
wrangler d1 execute anixops-db --command="SELECT COUNT(*) FROM users"

# 检查节点数
wrangler d1 execute anixops-db --command="SELECT COUNT(*) FROM nodes"
```

5. **通知用户**
- 发送系统通知
- 更新状态页面

**RTO**: 30分钟 - 1小时

### 4.2 场景二: 认证系统故障

**症状**:
- 用户无法登录
- Token验证失败
- 会话丢失

**恢复步骤**:

1. **清除KV缓存**
```bash
# 清除所有Token黑名单
wrangler kv:key delete --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf "blacklist:*"

# 清除用户撤销记录
wrangler kv:key delete --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf "user_revoke:*"
```

2. **检查JWT密钥**
```bash
# 验证JWT_SECRET环境变量配置正确
wrangler secret list
```

3. **重置管理员密码**
```bash
# 生成新密码哈希
# 使用bcrypt生成: password_hash = bcrypt.hashpw("newpassword", bcrypt.gensalt(12))

wrangler d1 execute anixops-db --command="UPDATE users SET password_hash='<new-hash>' WHERE email='admin@example.com'"
```

**RTO**: 15分钟

### 4.3 场景三: API服务不可用

**症状**:
- Cloudflare返回错误
- 请求超时
- 区域性故障

**恢复步骤**:

1. **检查Cloudflare状态**
   - 访问 https://www.cloudflarestatus.com/
   - 检查Workers服务状态

2. **查看日志**
```bash
# 实时日志
wrangler tail

# 查看错误日志
wrangler tail --format=json | jq 'select(.level == "error")'
```

3. **回滚部署**
```bash
# 列出历史版本
wrangler deployments list

# 回滚到上一版本
wrangler rollback
```

4. **重启Worker**
```bash
# 重新部署
wrangler deploy
```

**RTO**: 5-15分钟

### 4.4 场景四: 数据丢失（用户误删除）

**症状**:
- 用户被误删
- 节点配置丢失
- Playbook文件删除

**恢复步骤**:

1. **停止进一步操作**
   - 防止数据覆盖

2. **从备份恢复特定数据**
```bash
# 下载备份
curl https://api.anixops.com/api/v1/backups/<backup-id>/download \
  -H "Authorization: Bearer <admin-token>" \
  -o backup.json

# 手动提取需要的数据
jq '.data.users[] | select(.email == "user@example.com")' backup.json
```

3. **手动恢复记录**
```bash
# 插入特定用户
wrangler d1 execute anixops-db --command="INSERT INTO users (email, password_hash, role, enabled) VALUES (...)"
```

**RTO**: 30分钟

---

## 五、监控与告警

### 5.1 关键指标

| 指标 | 阈值 | 告警级别 |
|------|------|----------|
| API错误率 | > 1% | Warning |
| API错误率 | > 5% | Critical |
| 数据库大小 | > 450MB | Warning |
| 备份失败 | 任何失败 | Critical |
| 登录失败率 | > 10% | Warning |

### 5.2 健康检查端点

```bash
# 基础健康检查
curl https://api.anixops.com/health

# 就绪检查
curl https://api.anixops.com/readiness
```

### 5.3 日志收集

```bash
# 实时日志
wrangler tail --format=pretty

# JSON格式日志（用于分析）
wrangler tail --format=json > logs-$(date +%Y%m%d).json
```

---

## 六、应急联系人与权限

### 6.1 紧急联系人

| 角色 | 联系方式 | 职责 |
|------|----------|------|
| 主要运维 | [待填写] | 系统恢复、故障排查 |
| 备用运维 | [待填写] | 备份支持 |
| 安全负责人 | [待填写] | 安全事件响应 |

### 6.2 权限矩阵

| 操作 | Admin | Operator | Viewer |
|------|-------|----------|--------|
| 创建备份 | ✅ | ❌ | ❌ |
| 恢复备份 | ✅ | ❌ | ❌ |
| 删除备份 | ✅ | ❌ | ❌ |
| 查看备份 | ✅ | ❌ | ❌ |
| 解锁账户 | ✅ | ❌ | ❌ |

---

## 七、恢复演练计划

### 7.1 演练频率

| 演练类型 | 频率 | 参与人员 |
|----------|------|----------|
| 备份验证 | 每周 | 运维 |
| 部分恢复 | 每月 | 运维 + 开发 |
| 全量恢复 | 每季度 | 全团队 |

### 7.2 演练检查清单

- [ ] 备份文件可正常下载
- [ ] 备份数据格式正确
- [ ] 恢复流程文档准确
- [ ] RTO在预期范围内
- [ ] 团队成员熟悉流程

---

## 八、变更记录

| 日期 | 版本 | 变更内容 | 作者 |
|------|------|----------|------|
| 2026-03-19 | 1.0.0 | 初始版本 | AnixOps Team |

---

## 附录

### A. 常用命令速查

```bash
# 数据库操作
wrangler d1 execute anixops-db --command="SELECT COUNT(*) FROM users"
wrangler d1 export anixops-db --output=backup.sql
wrangler d1 migrations apply anixops-db

# KV操作
wrangler kv:key list --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf
wrangler kv:key get --namespace-id=35a39ccc4cea47e89aa8b1459f85e1cf "key-name"

# R2操作
wrangler r2 object list anixops-files
wrangler r2 object get anixops-files/path/to/file

# 部署
wrangler deploy
wrangler tail
wrangler rollback
```

### B. API端点参考

| 端点 | 方法 | 用途 |
|------|------|------|
| /api/v1/backups | GET | 列出备份 |
| /api/v1/backups | POST | 创建备份 |
| /api/v1/backups/:id | GET | 获取备份详情 |
| /api/v1/backups/:id/download | GET | 下载备份 |
| /api/v1/backups/:id/restore | POST | 恢复备份 |
| /api/v1/backups/:id | DELETE | 删除备份 |
| /api/v1/backups/status | GET | 备份状态 |
| /health | GET | 健康检查 |
| /readiness | GET | 就绪检查 |