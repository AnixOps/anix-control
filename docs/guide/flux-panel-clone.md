# Flux-panel 复刻指南

## 目标

本项目后续在“流量转发 / 隧道 / 用户隧道授权 / 相关联动页面”上的开发，默认以 [`flux-panel`](https://github.com/bqlpfy/flux-panel) 为源实现做一比一复刻。

这里的“一比一”指的是：

- 请求路径、HTTP 方法、鉴权范围一致
- 请求体字段名、响应 DTO 字段名、返回包装结构一致
- 页面结构、按钮文案、模态框流程、表格/分组/拖拽/导入导出交互一致
- 业务副作用一致
  - 例如 create/update/delete/pause/resume 是否联动运行时服务
  - diagnose 是否沿节点路径执行
  - quota、授权、状态校验是否与参考实现一致

## 参考仓库

### 上游地址

- GitHub: `https://github.com/bqlpfy/flux-panel`

### 当前本机参考副本

- `C:\Users\z7299\AppData\Local\Temp\flux-panel`

### 复刻时优先阅读的文件

后端：

- `springboot-backend/src/main/java/com/admin/controller/ForwardController.java`
- `springboot-backend/src/main/java/com/admin/controller/TunnelController.java`
- `springboot-backend/src/main/java/com/admin/service/impl/ForwardServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/service/impl/TunnelServiceImpl.java`
- `springboot-backend/src/main/java/com/admin/common/dto/`
- `springboot-backend/src/main/java/com/admin/entity/`

前端：

- `vite-frontend/src/pages/forward.tsx`
- `vite-frontend/src/api/`
- `vite-frontend/src/components/`

## 复刻顺序

推荐按照下面顺序进行，而不是只盯着页面：

1. 先确认参考仓库里的 controller 路径、鉴权范围和 DTO。
2. 再确认 service 的真实业务语义，尤其是授权、配额、运行时副作用和异常分支。
3. 再确认实体/关联关系。
4. 最后复刻页面、交互和前端请求调用。
5. 页面完成后回到后端再次核对，避免出现“页面像了，但 API 行为不一样”的情况。

## 强制契约

### 1. 路由和鉴权

- 如果参考接口是登录用户接口，本仓库必须提供用户态入口。
- 如果参考接口是管理员接口，本仓库再挂到 `/api/v2/admin/*`。
- 允许为现有本地页面保留镜像兼容路由，但镜像路由不能代替参考仓库真实作用域。

### 2. 返回包装结构

复刻接口时，优先对齐参考返回包：

```json
{
  "code": 0,
  "msg": "操作成功",
  "ts": 1712300000000,
  "data": {}
}
```

规则：

- `code/msg/ts/data` 缺一不可，除非参考接口本身不是这个格式
- 时间戳单位必须与参考实现一致
- 错误分支也要保留同层包装

### 3. DTO 字段

- 不要删掉参考 DTO 中“当前页面暂时没用到”的字段。
- 不要擅自把 `camelCase` 改成 `snake_case` 或反过来。
- 不要把参考里的 `ip/type/protocol` 简化成只返回 `inIp`。

### 4. 业务语义

- create/update/delete/pause/resume 需要区分：
  - 只是修改数据库状态
  - 还是要联动远端运行时服务
- diagnose 需要区分：
  - 只是本地 `Dial`
  - 还是按入口节点/出口节点链路执行
- 授权要优先复用参考中的显式关系模型。
  - 例如参考里有 `UserTunnel`，本项目就优先用显式关联表，而不是临时拼 `group_id`

## 本仓库映射规则

| 参考仓库 | 本仓库 |
|------|------|
| Spring Boot controller | `internal/handler/` + `internal/router/router.go` |
| service impl | `internal/service/` |
| entity | `internal/model/` |
| React page | `web/src/views/` |
| frontend api | `web/src/api/` |

## 当前已完成的基础

截至 `2026-04-05`，流量转发模块已有这些基础：

- 页面复刻：
  - `web/src/views/admin/Forward.vue`
- 后端兼容服务：
  - `internal/service/forward_panel_service.go`
- 兼容处理器：
  - `internal/handler/forward_panel.go`
- 兼容路由：
  - `internal/router/router.go`
- 前端请求封装：
  - `web/src/api/admin.js`
- 授权关系模型：
  - `internal/model/forward_panel.go` 中的 `ForwardUserTunnel`
- 基础测试：
  - `internal/service/forward_panel_service_test.go`

## 当前 forward 模块已对齐内容

- `forward` 页面主体 UI 与交互
- `/forward/*` 兼容端点
- `/tunnel/user/tunnel` 兼容端点
- `code/msg/ts/data` 返回包装
- `ForwardUserTunnel` 显式授权关系
- tunnel 列表 DTO 的基础字段：
  - `id`
  - `name`
  - `ip`
  - `inIp`
  - `inNodePortSta`
  - `inNodePortEnd`
  - `type`
  - `protocol`

## 当前 forward 模块仍需继续补齐的差距

这些内容在文档和代码里都必须视为“待完成”，不能算完全复刻：

1. 运行时副作用仍未完全按 `flux-panel` 原版实现。
   - 参考实现会联动 Gost 或节点运行时。
   - 本仓库当前主要还是兼容 DB/接口层。
2. diagnose 逻辑仍未完全走参考实现的节点链路语义。
3. `UserTunnel` 周边管理页、限额、流量/过期联动等语义仍可继续向原版贴近。
4. 任何后续新增的 tunnel/forward 相关页面，都需要继续沿用本规范，而不是另起一套字段和路径。

## 建议的下一批复刻目标

按收益和依赖顺序，建议继续做：

1. `UserTunnel` 管理页和授权管理流程
2. forward runtime 语义
   - create/update/delete/pause/resume 对远端服务的真实副作用
3. forward / tunnel diagnose 的节点链路实现
4. 与 `UserTunnel` 相关的 quota、status、expire、flow reset 联动

## 每次复刻完成后的验收

至少执行：

```bash
$env:GOWORK='off'; go test ./internal/router ./internal/handler ./internal/service
cd web && npm run build
```

如果是 forward/tunnel 相关改动，再额外确认：

- 页面请求的路径是否仍与参考一致
- handler 是否仍返回 `code/msg/ts/data`
- DTO 是否没有偷偷删字段
- 用户态接口是否还能在 JWT 用户上下文下访问
- 新增差距是否已经记录到本文件

## 文档维护规则

后续每完成一个 `flux-panel` 模块复刻，需要同步更新三处：

- `AGENTS.md`
- `CLAUDE.md`
- `docs/guide/flux-panel-clone.md`

如果只是局部修补，也至少要更新本文件里的“当前已完成基础”或“仍需补齐的差距”。
