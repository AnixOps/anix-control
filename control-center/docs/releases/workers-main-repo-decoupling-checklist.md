# 主仓第一批拆耦改动清单

## 目标
在 Workers 独立仓准备完成后，主仓先完成第一批拆耦，移除对 `workers/` 源码路径的直接工程依赖。

## 与总 Runbook 的关系
本清单只负责主仓第一批拆耦，完整迁移顺序以 `workers-repo-migration-guide.md` 为准。

## 一页执行清单
1. 先改 `.github/workflows/ci.yml`。
2. 再改 `.github/workflows/release.yml`。
3. 再改 `scripts/version.sh`。
4. 最后改 `README.md` / `CHANGELOG.md`。
5. 确认主仓不再直接依赖 `workers/` 源码路径。

## 交付口径
- 主仓 CI 不再读取 `workers/package-lock.json`、`workers/coverage/lcov.info` 或 `working-directory: workers`
- 主仓 release 不再承载 Workers 发布职责
- 主仓 version 脚本不再默认推进 Workers 版本
- 主仓文档只保留独立 Workers 仓的引用与集成说明

## 最终检查表
- [ ] `.github/workflows/ci.yml` 已去除 `workers/*` 直接引用
- [ ] `.github/workflows/release.yml` 已移除 Workers 发布职责
- [ ] `scripts/version.sh` 已去掉 Workers 统一版本推进假设
- [ ] `README.md` 已指向独立 workers 仓
- [ ] `CHANGELOG.md` 已仅保留产品级整合影响
- [ ] 主仓不再依赖 `workers/` 源码路径
- [ ] Web / Mobile 相关契约验证位点已复核

## 主仓保留内容
- `web/`
- `mobile/`
- Go 主程序
- 聚合文档
- 指向 workers 新仓的集成说明

## 主仓移除内容
- `workers/` 源码实现目录
- workers 专属 CI / release 逻辑
- workers 版本统一推进假设

## 必改文件
### 1. CI
- `.github/workflows/ci.yml`

#### 需要移除/改造
- `working-directory: workers`
- `cache-dependency-path: workers/package-lock.json`
- `./workers/coverage/lcov.info`
- `workers-test` 作为主仓 quality gate 的直接输入

### 2. Release
- `.github/workflows/release.yml`

#### 需要移除/改造
- 主仓 release 中对 Workers 发布职责的承载
- 保留 Web / Mobile / CLI / 聚合说明
- Workers release 改由新仓负责

### 3. 版本与脚本
- `scripts/version.sh`
- `VERSION`
- `CHANGELOG.md`

#### 需要移除/改造
- 不再默认统一推进 Workers 版本
- 主仓 changelog 只记录产品级整合影响

### 4. 文档
- `README.md`
- `CHANGELOG.md`
- 后续主仓 docs 中与 workers 实现细节强绑定的内容

#### 需要补充
- workers 独立仓地址
- API 文档入口
- 集成关系说明

### 5. 客户端契约验证位点（优先验证，不一定第一批立刻改）
#### Web
- `web/src/api/index.js`
- `web/src/composables/useSSE.js`
- `web/src/stores/nodes.js`
- `web/src/stores/tasks.js`

#### Mobile
- `mobile/lib/core/services/api_client.dart`
- `mobile/lib/core/providers/api_providers.dart`
- `mobile/lib/core/services/sse_service.dart`
- `mobile/lib/core/services/websocket_service.dart`

## 风险提示
- 如果先删 `workers/` 再改 CI，会直接把主仓 pipeline 打坏
- 如果先改文档但 API 仓未准备好，会造成对外说明失真
- 应始终在新仓 staging 验证完成后，再做主仓路径移除
