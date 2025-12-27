# 客户端兼容指南

## 概述

本文档说明如何配置和修改常见节点程序（V2bX、XrayR）以兼容 V2Board AnixOps。

---

## V2bX 配置

### 配置文件修改

```yaml
# config.yml
Log:
  Level: warning
  Output: console

Nodes:
  - ApiHost: "https://your-v2board.com"
    ApiPath: "/api/v2"        # ← 必须使用 v2
    NodeID: 1
    NodeType: V2ray           # 可选: V2ray, Shadowsocks, Trojan, Hysteria, Hysteria2, TUIC, AnyTLS
    Token: "your-api-token"
```

### 源码修改（必须）

V2bX 需要修改源码以支持 `node_type` 字段。

#### 问题描述

V2bX 期望响应中的 `type` 字段，但后端返回 `node_type`：

```
node type not found in response
```

#### 修改方案

**文件**: `api/panel/node.go`

```go
// 修改前
var baseInfo struct {
    Type string `json:"type"`
}

// 修改后
var baseInfo struct {
    Type     string `json:"type"`
    NodeType string `json:"node_type"`  // 新增
}
if err = json.Unmarshal(r.Body(), &baseInfo); err != nil {
    return nil, fmt.Errorf("decode node type error: %s", err)
}

// 优先使用 type，如果为空则使用 node_type
nodeType := strings.ToLower(baseInfo.Type)
if nodeType == "" {
    nodeType = strings.ToLower(baseInfo.NodeType)
}
if nodeType == "" {
    return nil, fmt.Errorf("node type not found in response")
}
```

#### 完整 Diff

```diff
--- a/api/panel/node.go
+++ b/api/panel/node.go
@@ -183,13 +183,18 @@ func (c *Client) GetNodeInfo() (node *NodeInfo, err error) {
 	// 先解析基础信息获取节点类型
 	var baseInfo struct {
-		Type string `json:"type"`
+		Type     string `json:"type"`
+		NodeType string `json:"node_type"`
 	}
 	if err = json.Unmarshal(r.Body(), &baseInfo); err != nil {
 		return nil, fmt.Errorf("decode node type error: %s", err)
 	}
-	nodeType := strings.ToLower(baseInfo.Type)
+	nodeType := strings.ToLower(baseInfo.Type)
+	if nodeType == "" {
+		nodeType = strings.ToLower(baseInfo.NodeType)
+	}
 	if nodeType == "" {
 		return nil, fmt.Errorf("node type not found in response")
 	}
```

---

## XrayR 配置

### 配置文件修改

```yaml
# config.yml
Nodes:
  - ApiConfig:
      ApiHost: "https://your-panel.com/api/v2"  # 添加 /api/v2 后缀
      NodeID: 1
      NodeType: V2ray
      Token: "your-token"
```

---

## 空用户列表错误

### 问题描述

节点启动时报错：

```
Run nodes failed: add users error: not have any user
```

### 原因分析

节点程序将空用户列表视为错误，但新节点可能暂时没有用户订阅。

### 解决方案

#### 方案 1: 修改节点程序（推荐）

**V2bX** - 找到 `core/node.go` 或类似文件：

```go
// 修改前
if len(users) == 0 {
    return fmt.Errorf("add users error: not have any user")
}

// 修改后
if len(users) == 0 {
    log.Warnln("No users subscribed to this node yet, continuing...")
    return nil  // 允许空用户列表
}
```

**XrayR** - 找到 `panel/panel.go` 或类似文件：

```go
// 修改前
if len(userInfo) == 0 {
    return errors.New("not have any user")
}

// 修改后
if len(userInfo) == 0 {
    log.Println("Warning: No users found for this node")
    // 继续运行，不返回错误
}
```

#### 方案 2: 临时解决

在面板中创建测试用户并购买包含该节点的套餐。

---

## API 路径变更

| 旧路径 (v1) | 新路径 (v2) |
|-------------|-------------|
| `/api/v1/server/UniProxy/config` | `/api/v2/server/UniProxy/config` |
| `/api/v1/server/UniProxy/user` | `/api/v2/server/UniProxy/user` |
| `/api/v1/server/UniProxy/push` | `/api/v2/server/UniProxy/push` |
| `/api/v1/server/UniProxy/alive` | `/api/v2/server/UniProxy/alive` |

---

## 响应格式差异

### 节点配置响应

```json
{
  "node_type": "vmess",        // 注意: 使用 node_type 而非 type
  "server_port": 443,
  "host": "node1.example.com",
  "server_name": "node1.example.com",
  "send_through": "0.0.0.0",
  "network": "tcp",
  "tls": 0,
  "routes": [],
  "base_config": {
    "push_interval": 60,
    "pull_interval": 60
  }
}
```

### 用户列表响应

```json
{
  "users": [
    {
      "id": 1,
      "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "speed_limit": 0,
      "device_limit": 3
    }
  ]
}
```

---

## 编译说明

### V2bX 编译

```bash
git clone https://github.com/InazumaV/V2bX.git
cd V2bX

# 应用上述修改后
go build -o v2bx .

# 运行
./v2bx run -c config.yml
```

### XrayR 编译

```bash
git clone https://github.com/XrayR-project/XrayR.git
cd XrayR

# 应用上述修改后
go build -o xrayr main.go

# 运行
./xrayr -config config.yml
```

---

## 验证测试

### 测试 API 连接

```bash
# 健康检查
curl https://your-panel.com/health

# 测试节点配置接口
curl "https://your-panel.com/api/v2/server/UniProxy/config?node_id=1&token=your-token"
```

### 检查节点日志

```bash
# V2bX
./v2bx run -c config.yml 2>&1 | grep -E "(error|warning|info)"

# 确认以下信息:
# - "Get node info successfully"
# - "Get user list successfully"  或 "No users found" (不是错误)
```

---

## 常见问题

### Q: 为什么要修改客户端源码？

A: 后端使用 `node_type` 而非 `type` 字段，语义更清晰。这需要客户端适配。

### Q: v1 API 还能用吗？

A: 不能。系统已完全移除 v1 API 支持。

### Q: 修改后的客户端还能连接其他面板吗？

A: 可以。方案 1 同时支持 `type` 和 `node_type`，向后兼容。
