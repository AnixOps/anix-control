---
description: 构建面板端和节点端二进制文件
---

请执行以下构建任务：

1. 构建面板端 (v2board_AnixOps)
```bash
go build -o v2board.exe cmd/server/main.go
```

2. 检查节点端目录是否存在，如果存在则构建
```bash
# 节点端需要 GOEXPERIMENT=jsonv2
cd ../V2bX_AnixOps
GOEXPERIMENT=jsonv2 go build -o V2bX.exe main.go
```

3. 报告构建结果和文件大小