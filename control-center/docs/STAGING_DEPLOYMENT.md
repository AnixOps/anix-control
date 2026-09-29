# Staging 环境部署指南

## 概述

AnixOps Control Center 使用多环境部署策略，包括：
- **Development**: 本地开发环境
- **Staging**: 测试环境（用于QA和预发布测试）
- **Production**: 生产环境

## Staging 环境配置

### 前置要求

1. Cloudflare 账户权限
2. Wrangler CLI 已安装并配置
3. Staging 资源已创建（D1、KV、R2）

### 资源创建

```bash
# 1. 创建 Staging D1 数据库
wrangler d1 create anixops-db-staging

# 2. 创建 Staging KV 命名空间
wrangler kv:namespace create "STAGING_KV"

# 3. 创建 Staging R2 存储桶
wrangler r2 bucket create anixops-files-staging
```

### 更新配置

创建资源后，更新 `wrangler.toml` 中的占位符：

```toml
# 替换以下占位符为实际ID
[[env.staging.d1_databases]]
database_id = "your-actual-staging-db-id"

[[env.staging.kv_namespaces]]
id = "your-actual-staging-kv-id"
```

### 部署命令

```bash
# 部署到 Staging 环境
wrangler deploy --env staging

# 部署到 Production 环境
wrangler deploy --env production

# 本地开发
wrangler dev
```

### 数据库迁移

```bash
# Staging 环境迁移
wrangler d1 migrations apply anixops-db-staging --env staging

# Production 环境迁移
wrangler d1 migrations apply anixops-db --env production
```

### 环境变量设置

```bash
# Staging 环境密钥
wrangler secret put JWT_SECRET --env staging
wrangler secret put API_KEY_SALT --env staging

# Production 环境密钥
wrangler secret put JWT_SECRET --env production
wrangler secret put API_KEY_SALT --env production
```

## 环境差异

| 配置项 | Development | Staging | Production |
|--------|-------------|---------|------------|
| 数据库 | 共享生产 | 独立 | 独立 |
| KV | 共享生产 | 独立 | 独立 |
| R2 | 共享生产 | 独立 | 独立 |
| CORS | localhost | staging域名 | 生产域名 |
| 日志级别 | debug | debug | info |

## 验证部署

```bash
# 检查 Staging API 健康
curl https://<staging-api-base>/health

# 检查 Production API 健康
curl https://api.anixops.com/health
```

## 回滚

```bash
# 查看部署历史
wrangler deployments list --env staging

# 回滚到上一版本
wrangler rollback --env staging
```

## 注意事项

1. **Staging 数据独立**: Staging 环境使用独立的数据存储，不会影响生产数据
2. **密钥管理**: 确保每个环境使用不同的密钥
3. **域名配置**: 需要在 Cloudflare 中配置自定义域名
4. **备份**: Staging 环境也需要定期备份