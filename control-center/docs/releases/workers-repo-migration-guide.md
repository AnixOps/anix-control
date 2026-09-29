# Workers 独立仓库迁移实施指南

## 目标
将 `workers/` 从当前聚合仓库中拆分为独立 git 仓库，形成独立的 Cloudflare Workers API 后端产品线，同时保证：
- 主仓 Web / Mobile / Go 客户端不受影响
- Cloudflare production / staging 配置可迁移
- CI/CD、版本与文档职责清晰分离
- 整个迁移具备可回滚能力

---

## 现状基线

### 发布与环境状态
- production release workflow 已完成
- dev release workflow 已完成
- production PR 已合并
- `api.anixops.com` 当前 health / readiness 可用

### Workers 关键配置
配置来源：`workers/wrangler.toml`
- Worker service: `anixops-api-v2`
- Route: `api.anixops.com`
- Production D1: `anixops-db`
- Production KV: `35a39ccc4cea47e89aa8b1459f85e1cf`
- Production R2: `anixops-files`
- AI binding: `AI`
- Vectorize index: `anixops-embeddings`
- Staging env 已存在独立 name / D1 / KV / R2 配置框架

### 测试基线
- Workers full suite: `565 passed`
- Web full unit suite: `358 passed`
- Mobile full unit suite: `233 passed`
- Mobile Android integration: `5 passed`

---

## 判断标准
- 主仓还在直接引用 `workers/` 源码路径 = 拆耦未完成
- 新仓还没有独立 CI / release / changelog = 迁移未闭环
- Cloudflare staging 与 production 没有分别验证 = 不能切主路由
- Web / Mobile 还依赖源码级实现细节 = 仍只是契约层解耦

---

## 推荐策略
采用 **双仓过渡迁移**：
1. 先建立新仓并跑通 staging。
2. 再修改主仓 CI / release / 文档。
3. 进入双仓并行观察期。
4. 最后从主仓移除 `workers/`。

---

## 现场执行顺序

### Step 0：进入迁移窗口
- 停止 workers 侧非阻断改动
- 确认主仓 dev / production 已稳定
- 固定当前 `workers/wrangler.toml`、CI、release、README、CHANGELOG 的快照

### Step 1：建新仓
- 创建 `anixops-control-center-workers`
- 初始化 `.github/workflows/ci.yml`、`.github/workflows/release.yml`、`.gitignore`、`CHANGELOG.md`、`VERSION`
- 搭好 `src/`、`test/`、`migrations/`、`docs/` 目录骨架

### Step 2：迁代码与配置
- 迁移 `workers/src/**`、`workers/test/**`、`workers/migrations/**`
- 迁移 `package.json`、`package-lock.json`、`tsconfig.json`、`vitest.config.ts`、`wrangler.toml`
- 迁移 `workers/README.md`、`workers/SETUP.md`，并修复相对路径

### Step 3：跑通新仓
- 在新仓完成 install / lint / test / coverage
- 接入 staging Cloudflare 绑定
- 通过 staging deploy 和冒烟验证

### Step 4：清主仓耦合
- 删除主仓 CI 里所有 `workers/*` 直接引用
- 去掉主仓 release 中的 Workers 发布职责
- 让 `scripts/version.sh` 不再默认推进 Workers 版本
- 让主仓 README / CHANGELOG 只保留独立 workers 仓引用

### Step 5：切换与观察
- 让 Web / Mobile 只通过独立 Workers API 访问
- 切到新仓 production 发布链
- 观察 7 天，确认无 API 契约漂移和回归

---

## 交付口径
- 新仓可以独立执行 install / lint / test / coverage
- staging deploy 成功并通过冒烟
- production deploy 走新仓发布链
- 主仓不再引用 `workers/*` 路径
- Web / Mobile 通过独立 Workers API 正常工作

## 最终检查表
- [ ] 新仓仓库已创建
- [ ] `src/`、`test/`、`migrations/` 已迁入新仓
- [ ] `wrangler.toml` 与 Cloudflare 绑定已在新仓验证
- [ ] 新仓 CI 可独立完成 install / lint / test / coverage
- [ ] 新仓 staging deploy 与冒烟通过
- [ ] 新仓 release workflow 可手动触发并产出版本说明
- [ ] 主仓 CI 已去除 `workers/*` 直接引用
- [ ] 主仓 release 已移除 Workers 发布职责
- [ ] 主仓 version / changelog 已去耦
- [ ] 主仓 README / docs 已改为指向独立 workers 仓
- [ ] Web / Mobile 已通过稳定 API 契约验证
- [ ] production 切换已完成
- [ ] 观察期 7 天无回归

## 回滚策略
- 双仓并行期保留主仓 `workers/` 只读快照
- 若新仓 staging/prod 部署失败：保持旧 route / 旧发布路径
- 若切换后 production 出现异常：回切 Cloudflare route 或 worker version

## 参考清单
- `docs/releases/workers-new-repo-bootstrap-checklist.md`
- `docs/releases/workers-main-repo-decoupling-checklist.md`
- `docs/releases/workers-repo-migration-guide.md`

