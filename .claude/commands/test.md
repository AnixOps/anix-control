---
description: 运行完整测试套件并生成覆盖率报告
---

请执行以下任务：

1. 运行所有单元测试
```bash
go test -v -short ./...
```

2. 运行覆盖率测试
```bash
go test -cover ./internal/...
```

3. 生成 gRPC 覆盖率报告
```bash
go test -coverprofile=coverage.out ./internal/grpc/...
go tool cover -html=coverage.out -o coverage.html
```

4. 报告测试结果摘要