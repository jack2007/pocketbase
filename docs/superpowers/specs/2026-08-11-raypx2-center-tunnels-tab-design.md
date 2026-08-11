# raypx2 Center 节点 Tunnels Tab 设计

**日期：** 2026-08-11  
**状态：** 已评审（待实现计划）  
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center`）  
**依赖：** 既有 Admin proxy（Hub → Agent → 本地 Admin `/api/v1/*`）；raypx2 已提供 tunnels 列表 API  
**前置：** 节点在线态、Ops proxy、节点详情 Tab 已可用

## 1. 目标

在 Center 节点详情中以独立 **Tunnels** Tab 只读展示该节点当前 tunnel 清单，字段与本地 Admin Console 的 Tunnels 页对齐，便于舰队侧排查会话而不必登录各节点 Console。

## 2. 非目标

- 不改 Agent 上报协议，不把 tunnel 列表并入 `status_summary`
- 不落库 tunnel 列表（无新 PocketBase 集合）
- 不做 Abort / Drain 等写操作
- 不做舰队级跨节点 tunnels 汇总页
- 不做 router 角色 tunnels UI
- 不做服务端分页/缓存（全量实时拉取；后续若有性能问题再单独立项）

## 3. 架构

```
Center SPA (Tunnels Tab)
  → POST /api/center/nodes/{node_key}/proxy
  → Hub / WSS http_proxy_req
  → Agent → 本地 Admin
       client: GET /api/v1/tunnels
       server: GET /api/v1/server/tunnels
  → http_proxy_res → SPA 渲染表格
```

- 复用既有 `proxyNode` / center proxy / 路径白名单（`/api/v1/` 前缀）与审计。
- 数据仅在用户打开 Tab 或点击 Refresh 时拉取；不随 `status_summary` 周期推送。

## 4. UI

### 4.1 入口

- `NodeDetail` Tab 集合扩展为：`overview` | `ops` | `tunnels` | `config` | `audit`
- 新组件 `NodeTunnels`（建议放在 `NodeDetail.tsx` 内或同目录拆分，保持与 `NodeOps` 模式一致）

### 4.2 行为

| 条件 | 行为 |
|---|---|
| 节点 online + role=client | `GET /api/v1/tunnels`，渲染 client 列 |
| 节点 online + role=server | `GET /api/v1/server/tunnels`，渲染 server 列 |
| 节点 offline | 不发 proxy；展示离线提示 |
| 其它 / 未知 role | 不发 proxy；提示该角色无 tunnel 列表 |
| 加载中 | Loading 态；提供 Refresh 按钮（加载中禁用） |
| 请求失败 | 展示错误；保留或清空策略与现有 Ops 一致（`setError`） |
| 成功且 `tunnels` 为空 | 空表提示（No items） |

### 4.3 列映射（对齐本地 Admin Console）

**Client**（源：`src/admin_console/app.js` `renderClientTunnels`）：

`tunnel_id`, `peer_id`, `connection_id`, `target`, `state`, `role`, `ingress`, `compress`, `created_at`, `duration_ms`, `tcp_read_bytes`, `tcp_write_bytes`, `pending_bytes`, `relay_backend`, `worker_index`, `last_error`

**Server**（源：`renderServerTunnels`）：

`tunnel_id`, `peer_id`, `connection_id`, `state`, `target`, `role`, `duration_ms`, `active`

- 响应体取 `tunnels` 数组；缺字段用现有 `display()` 显示为 `—`。
- 宽表使用现有 `table-shell` 横向滚动；无行内操作按钮。

## 5. 后端与协议

- **无新 Center HTTP 路由**（方案选定：SPA 直调既有 proxy）。
- **无 Agent 协议变更**。
- 确认既有 proxy 允许上述 GET path（`HasPrefix("/api/v1/")`）；无需为 tunnels 单独加白名单条目，除非实现时发现更严的路径表未覆盖——若未覆盖则最小补丁放行这两个 GET。

## 6. 错误处理与边界

- Proxy 超时沿用现有 `proxyTimeout`（约 10s）。
- Tunnel JSON 无密钥字段；审计继续走现有 proxy 摘要脱敏。
- 超大列表本期全量渲染；不在本设计引入分页。

## 7. 测试

**SPA（Vitest）**

- 存在 Tunnels Tab。
- client 节点加载时调用 `proxyNode(node_key, "GET", "/api/v1/tunnels")`。
- server 节点加载时调用 `proxyNode(node_key, "GET", "/api/v1/server/tunnels")`。
- offline 不调用 proxy。
- 按 role 渲染对应表头。

**Go**

- 无新路由则无强制新单测；若路径白名单需补丁，为放行 path 增加单测。

## 8. 验收

1. 在线 client/server 打开 Tunnels，列与本地 Console 一致，数据来自实时 Admin。
2. Refresh 重新拉取。
3. 离线仅提示、无 proxy 请求。
4. 无 Abort/Drain 等写入口。

## 9. 实现触点（预期）

| 区域 | 变更 |
|---|---|
| `apps/raypx2-center/ui/src/pages/NodeDetail.tsx` | Tab + `NodeTunnels` |
| `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx` | 上述行为测试 |
| `apps/raypx2-center/ui/src/styles.css` | 仅当宽表样式不足时微调 |
| `apps/raypx2-center/README.md` | 可选一行说明 Tunnels Tab |
| raypx2 | 无代码变更（API 已存在） |

## 10. 决策记录

| 决策 | 选择 |
|---|---|
| 数据到达方式 | A：实时代理（非 status_summary 上报） |
| 读写 | A：只读 |
| 列集合 | A：对齐本地 Admin Console |
| UI 位置 | B：节点详情独立 Tunnels Tab |
| 实现路径 | 方案 1：SPA 直调既有 Admin proxy |
