# Workers 新仓初始化清单

## 目标
把 Workers 代码、测试、迁移与配置完整迁到 `anixops-control-center-workers`，并在新仓完成独立闭环。

## 与总 Runbook 的关系
本清单只负责新仓初始化与首批落地，完整迁移顺序以 `workers-repo-migration-guide.md` 为准。

## 推荐仓库名
- `anixops-control-center-workers`

## 一页执行清单
1. 先创建新仓骨架和 CI / release workflow 占位。
2. 再迁移 `src/`、`test/`、`migrations/` 和基础配置。
3. 然后接入 `wrangler.toml`、环境变量和 Cloudflare 绑定。
4. 最后补齐 README、SETUP、CHANGELOG、VERSION 和文档链接。

## 交付口径
- 新仓可以独立执行 install / lint / test / coverage
- staging deploy 可以完成并通过冒烟验证
- release workflow 可以产出可追踪的版本说明
- 文档不再依赖主仓相对路径或旧 workers 源码位置

## 最终检查表
- [ ] 新仓仓库已创建
- [ ] `.github/workflows/ci.yml` 已建立
- [ ] `.github/workflows/release.yml` 已建立
- [ ] `src/`、`test/`、`migrations/` 已迁入新仓
- [ ] `package.json`、`package-lock.json`、`tsconfig.json`、`vitest.config.ts` 已迁入新仓
- [ ] `wrangler.toml` 已迁入并完成 Cloudflare 绑定验证
- [ ] `README.md` 与 `SETUP.md` 已迁入并修正路径
- [ ] `CHANGELOG.md` 与 `VERSION` 已建立
- [ ] docs 已补齐 `architecture.md`、`deploy.md`、`api-contract.md`
- [ ] 新仓 CI 可独立完成 install / lint / test / coverage
- [ ] staging deploy 与冒烟通过
- [ ] release workflow 可手动触发并产出版本说明

## 首批基础仓骨架
- `.github/workflows/ci.yml`：只保留 install / lint / test / coverage
- `.github/workflows/release.yml`：手动触发的 staging / production 发布入口
- `CHANGELOG.md`：记录新仓自身发布历史
- `VERSION`：由新仓独立维护
- `.gitignore`：排除 node_modules、coverage、.wrangler
- `docs/api-contract.md`：先写清 `/api/v1` 契约边界

## 首批迁移文件
### 从主仓迁移
- `workers/src/**`
- `workers/test/**`
- `workers/migrations/**`
- `workers/package.json`
- `workers/package-lock.json`
- `workers/tsconfig.json`
- `workers/vitest.config.ts`
- `workers/wrangler.toml`
- `workers/README.md`
- `workers/SETUP.md`

### 从主仓文档提取
- `docs/cloudflare-integration-plan.md` 中 workers / Cloudflare 相关章节

## 新仓初始化时要新建
- `.gitignore`
- `CHANGELOG.md`
- `VERSION`
- `docs/architecture.md`
- `docs/deploy.md`
- `docs/api-contract.md`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`

## 不应迁移的内容
- `workers/node_modules/**`
- `workers/.wrangler/**`
- `workers/coverage/**`
- `workers/node_modules/.vite/**`

## 首批 CI 能力
- install dependencies
- typecheck / lint
- vitest
- coverage upload

## 首批 release 能力
- staging deploy
- production deploy
- changelog / release notes
- Cloudflare bindings 校验
