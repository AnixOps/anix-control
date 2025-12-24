# V2bX 客户端兼容性修改建议

## 问题描述

V2bX 客户端在解析节点配置时，硬编码期望 `type` 字段，但后端返回的是 `node_type` 字段，导致报错：

```
node type not found in response
```

## 问题根源

文件: `api/panel/node.go` 第 183-197 行

```go
// 先解析基础信息获取节点类型
var baseInfo struct {
    Type string `json:"type"`  // ← 只识别 "type" 字段
}
if err = json.Unmarshal(r.Body(), &baseInfo); err != nil {
    return nil, fmt.Errorf("decode node type error: %s", err)
}
nodeType := strings.ToLower(baseInfo.Type)
if nodeType == "" {
    return nil, fmt.Errorf("node type not found in response")  // ← 报错来源
}
```

## 修改方案

### 方案 1: 同时支持 `type` 和 `node_type` (推荐)

修改 `api/panel/node.go` 中的 `baseInfo` 结构体，使其同时支持两种字段名：

```go
// 先解析基础信息获取节点类型
var baseInfo struct {
    Type     string `json:"type"`
    NodeType string `json:"node_type"`  // 新增：支持 node_type 字段
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

### 方案 2: 只使用 `node_type`

如果确定后端只返回 `node_type`，可以直接修改：

```go
var baseInfo struct {
    NodeType string `json:"node_type"`
}
if err = json.Unmarshal(r.Body(), &baseInfo); err != nil {
    return nil, fmt.Errorf("decode node type error: %s", err)
}
nodeType := strings.ToLower(baseInfo.NodeType)
if nodeType == "" {
    return nil, fmt.Errorf("node type not found in response")
}
```

## 完整 Diff (方案 1)

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
+	// 优先使用 type，如果为空则使用 node_type
+	nodeType := strings.ToLower(baseInfo.Type)
+	if nodeType == "" {
+		nodeType = strings.ToLower(baseInfo.NodeType)
+	}
 	if nodeType == "" {
 		return nil, fmt.Errorf("node type not found in response")
 	}
```

## 为什么修改客户端而不是后端

1. **语义更清晰**: `node_type` 比 `type` 更明确表示这是节点类型
2. **避免命名冲突**: `type` 是很多语言的保留字，使用 `node_type` 更安全
3. **后端已统一**: v2board_AnixOps 后端已经统一使用 `node_type`
4. **向前兼容**: 方案 1 同时支持两种字段，可以兼容不同版本的后端

## 测试验证

修改后，重新编译 V2bX 并测试：

```bash
cd V2bX_AnixOps
go build -o v2bx .
./v2bx run -c config.json
```

确认日志中不再出现 `node type not found in response` 错误。
