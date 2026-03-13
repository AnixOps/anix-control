---
description: 检查代码质量、潜在问题和改进建议
---

请执行以下代码检查：

1. 运行 go vet 检查
```bash
go vet ./...
```

2. 检查代码格式
```bash
gofmt -l .
```

3. 检查未使用的依赖
```bash
go mod tidy
```

4. 检查依赖安全
```bash
go list -m -u all | grep -E '\[.*\]'
```

5. 报告发现的问题和建议修复