# V2Board AnixOps 文档

## 文档目录

| 文档 | 说明 |
|------|------|
| [节点管理](node-management.md) | 节点注册、心跳、协议配置 |
| [客户端兼容](client-compatibility.md) | V2bX/XrayR 配置和修改指南 |
| [API 参考](api-reference.md) | 完整 API 接口文档 |

## 快速开始

### 1. 部署面板

```bash
# 使用 Docker Compose
docker-compose up -d

# 或直接运行
go build -o v2board ./cmd/server
./v2board
```

### 2. 配置节点

1. 在管理后台生成授权密钥
2. 将密钥配置到节点程序
3. 启动节点，自动注册并获取配置

### 3. 客户端要求

- V2bX: 需要修改源码支持 `node_type` 字段
- XrayR: 需要配置 API 路径为 `/api/v2`

详细说明请参考各文档。
