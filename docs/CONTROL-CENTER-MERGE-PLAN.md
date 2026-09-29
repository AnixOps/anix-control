# Control Center 合并方案

## 结论

两个仓库的产品能力有较高的合并可能性，但不建议把两个仓库的全部代码直接拼成一个运行时。

推荐以 `anix-control` 作为唯一的 Control 后端和主 Web 管理面，将 `anixops-control-center` 中与插件生命周期相关的能力迁入现有的 `anix-control/web`。Control Center 的 Workers、Flutter 移动端、TUI/CLI 和独立 Go 服务先保持独立，作为外部客户端或后续单独迁移对象。

## 可合并范围

| 能力 | 可行性 | 目标位置 | 说明 |
| --- | --- | --- | --- |
| 插件目录、版本、安装状态 | 高 | `anix-control/web` 现有插件管理页 | 两边都使用 `/api/v3` 插件资源，适合统一数据模型 |
| Control/Agent 生命周期操作 | 高 | `web/src/api/kernel.js`、插件管理 store 和页面 | 复用 Control 的 JWT、权限和操作 API |
| 操作链、状态和最近操作 | 高 | 现有插件管理页 | 以 Control 的 operation ID 和 operation chain 为准 |
| 插件配置和 revision 冲突处理 | 高 | 现有配置编辑器 | 保留 Control 的 `expected_revision` 合同 |
| Control Center 的插件页面视觉和交互 | 中高 | 适配到 `web/src/views/admin/Plugins.vue` | 不能直接覆盖现有 Vue、路由和国际化约定 |
| Workers 账号、租户和计费 | 低 | 暂不迁移 | 与 Control 的 `/api/v2` 登录和数据边界不同 |
| Flutter 移动端 | 中 | 暂不迁移 | 应继续作为调用 Control API 的独立客户端 |
| TUI/CLI 和 Center Go 服务 | 低 | 暂不迁移 | 与 Control 的 Go module、数据库和进程入口重复 |

## 不采用整仓物理合并的原因

1. 两个仓库有不同的 Go module、版本约束、服务入口、配置模型和数据库边界。
2. 两边都有独立的 Web 应用和认证状态；直接覆盖会引入重复路由、重复 token 存储和不一致的权限判断。
3. Center 还包含 Cloudflare Workers、Flutter、多平台发布物和 TUI/CLI。把这些目录放进 Control 的发布单元会扩大构建、发布和回滚范围。
4. `anix-control` 已经有正式的插件 API、管理员插件页面、部署分配和签名扩展运行时。应在现有边界上增量迁移，避免建立第二套插件合同。

## 迁移路线

### 阶段 1：冻结边界

- `anix-control` 负责插件目录、签名发布物、安装状态、生命周期操作和配置 revision。
- `anix-control` 的 `/api/v2` JWT 是插件管理页的唯一 Control 会话来源。
- Center Web 只作为客户端参考实现，不再新增第二套 Control API 合同。
- 对照两边的插件字段、错误 envelope、operation header 和权限要求，形成兼容性清单。

### 阶段 2：迁入插件管理体验

- 将 Center 中有价值的 Control/Agent 目标卡片、健康状态、失败原因和最近操作视图适配到 `anix-control/web/src/views/admin/Plugins.vue`。
- 复用 `anix-control` 的 `useKernelPlugins`、`api/kernel.js`、管理员权限守卫和现有国际化键。
- 不复制 Center 的独立登录页；插件页使用已登录的 Control 管理员会话。
- 保留未验证发布物不可安装、配置 revision 冲突、幂等键和 operation 轮询。

### 阶段 3：验证和发布

- 为适配后的页面补充 Vue 单测、API 合同测试和 Chromium E2E。
- 使用真实 `anix-control` 进程验证登录、目录、安装状态和操作查询。
- 在真实 Agent staging 环境验证安装、启用、禁用、更新、回滚和配置保存。
- 通过 Go、Web、文档同步、发布阶段和 legacy smoke gates 后再决定是否下线 Center 中的重复插件页面。

当前证据：`anix-control` 的 live Control Chromium gate 已在同一隔离真实进程中验证签名 `machine-telemetry` 包，并打开合并后的原生 `/admin/plugins` 页面读取发行版本和操作历史；Control Center 还提供了一个可选的真实 Control 生命周期 gate，用固定 Agent checkout 构建正式 `machine-telemetry` 包后，通过 Center 页面完成 Control 目标禁用/启用。两者都覆盖本地真实 Control 进程，不能替代真实 Agent staging 的生命周期变更、回滚和 operator approval。若本机的 Go 版本选择器不可用，可用 `ANIXOPS_GO_BIN=/path/to/go` 指定构建器，默认仍使用 PATH 中的 `go`。

### 阶段 4：客户端收敛（可选）

- Flutter、TUI/CLI 和 Workers 继续以稳定 API 客户端身份运行。
- 只有在客户端的认证、租户和发布策略确定后，才迁移共享 SDK 或目录。
- 迁移客户端前不删除 Center 仓库中的旧入口，先完成一版可回滚发布。

## 合并完成标准

- `anix-control` 的一个插件管理入口覆盖目录、版本、Control/Agent 安装状态、生命周期、配置和操作历史。
- 不再存在两套 Control JWT 存储或两套 `/api/v3` API 客户端实现。
- Control 生命周期 action 带显式幂等键；Agent 目标的 installation intent 由服务端 lifecycle generation 和派生 identity 去重，并能通过 operation ID 查询最终状态。
- 未验证发布物不能安装，配置更新遵守 revision 冲突保护。
- 真实 Control 和 Agent staging 通过后，旧 Center 插件页才标记为 deprecated 或移除。

## 当前建议

阶段 1 已完成，阶段 2 的插件目标卡片、健康状态、配置 revision、生命周期操作、操作历史和取消操作已经落到 `anix-control` 的现有管理员插件页；Center Web 仍保留独立登录页作为兼容客户端参考。下一步只在真实 Agent staging 验证通过后，才考虑标记 Center 的重复插件页为 deprecated。Center 的 Go 后端、Workers、Flutter 和 TUI/CLI 继续保持独立，以保留客户端发布和回滚边界。
