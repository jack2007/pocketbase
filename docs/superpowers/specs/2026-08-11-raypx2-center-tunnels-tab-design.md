# raypx2 Center 节点 Tunnels Tab 设计

**日期：** 2026-08-11  
**状态：** 已评审（实现计划已就绪）  
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center`）  
**依赖：** 既有 Admin proxy（Hub → Agent → 本地 Admin `/api/v1/*`）；raypx2 已提供 tunnels 列表 API  
**前置：** 节点在线态、Ops proxy、节点详情 Tab 已可用  
**实现计划：** [docs/superpowers/plans/2026-08-11-raypx2-center-tunnels-tab.md](../plans/2026-08-11-raypx2-center-tunnels-tab.md)

## 1. 目标

在 Center 节点详情中以独立 **Tunnels** Tab 只读展示该节点当前 tunnel 清单，字段与本地 Admin Console 的 Tunnels 页对齐，便于舰队侧排查会话而不必登录各节点 Console。

## 2. 非目标

- 不改 Agent 上报协议，不把 tunnel 列表并入 `status_summary`
- 不落库 tunnel 列表（无新 PocketBase 集合）
- 不做 Abort / Drain 等写操作
- 不做舰队级跨节点 tunnels 汇总页
- 不做 router / unknown 角色的 tunnel 列表（仅提示不可用）
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

- 复用既有 `proxyNode` / center proxy / 路径前缀校验（`strings.HasPrefix(path, "/api/v1/")`，见 `centerapi/proxy.go`）与审计。上述两条 GET 路径已满足前缀规则，**无需**新增白名单条目或 Go 补丁。
- **按需拉取：** `NodeTunnels` 必须仅在 `tab === "tunnels"` 时挂载（与现有 Ops/Config 条件渲染一致），以便只在进入 Tab 或点击 Refresh 时发请求；禁止在节点详情常驻挂载该组件。
- 不随 `status_summary` 周期推送。

## 4. UI

### 4.1 入口

- `NodeDetail` Tab 集合扩展为：`overview` | `ops` | `tunnels` | `config` | `audit`
- 新组件 `NodeTunnels`（放在 `NodeDetail.tsx` 内，保持与 `NodeOps` 模式一致；本期不强制拆文件）

### 4.2 行为

| 条件 | 行为 |
|---|---|
| 节点 online + role=client | `GET /api/v1/tunnels`，渲染 client 列 |
| 节点 online + role=server | `GET /api/v1/server/tunnels`，渲染 server 列 |
| 节点 offline | 不发 proxy；展示专用离线提示文案（见下） |
| 其它 / 未知 role（含 online） | 不发 proxy；提示该角色无 tunnel 列表 |
| 加载中 | Loading 态；提供 Refresh 按钮（加载中禁用） |
| 请求失败 | `setError`；**保留**上一次成功拉取的行（与 Ops 失败时不清空已有数据一致）；首次失败则表为空 |
| 成功且 `tunnels` 为空 | 空表提示（No items） |

离线提示文案（须可被测试精确匹配，避免与页眉 `Offline` 状态混淆）：

`Tunnels are unavailable while this node is offline.`

未知 role 提示：

`Tunnel inventory is not available for role "{role}".`

### 4.3 列映射（对齐本地 Admin Console）

**Client**（源：raypx2 `src/admin_console/app.js` `renderClientTunnels`）：

`tunnel_id`, `peer_id`, `connection_id`, `target`, `state`, `role`, `ingress`, `compress`, `created_at`, `duration_ms`, `tcp_read_bytes`, `tcp_write_bytes`, `pending_bytes`, `relay_backend`, `worker_index`, `last_error`

**Server**（源：`renderServerTunnels`）：

`tunnel_id`, `peer_id`, `connection_id`, `state`, `target`, `role`, `duration_ms`, `active`

- 响应体取 `tunnels` 数组；缺字段或空字符串用现有 `display()` 显示为 `—`。
- 宽表使用现有 `.table-shell { overflow-x: auto; }`（已在 `styles.css`）；无行内操作按钮；**默认不改** `styles.css`。

## 5. 后端与协议

- **无新 Center HTTP 路由**（方案选定：SPA 直调既有 proxy）。
- **无 Agent 协议变更**；raypx2 无代码变更。
- Proxy 超时：`proxyTimeout = 10 * time.Second`（`centerapi/proxy.go`）。

## 6. 错误处理与边界

- Tunnel JSON 无密钥字段；审计继续走现有 proxy 摘要脱敏。
- 超大列表本期全量渲染；不在本设计引入分页。

## 7. 测试

**SPA（Vitest）**

- 存在 Tunnels Tab。
- client 节点进入 Tab 时调用 `proxyNode(node_key, "GET", "/api/v1/tunnels")`，并出现 client 特有列（如 `tcp_read_bytes`）。
- server 节点进入 Tab 时调用 `proxyNode(node_key, "GET", "/api/v1/server/tunnels")`，并出现 `active` 列、不出现 `tcp_read_bytes`。
- offline：出现精确离线文案，且 `proxyNode` 零调用。
- unknown role（online）：出现角色不可用提示，且 `proxyNode` 零调用。
- 无 Abort / Drain 按钮。

**Go**

- 无新路由、无白名单变更 → 无强制新 Go 单测。

## 8. 验收

1. 在线 client/server 打开 Tunnels，列与本地 Console 一致，数据来自实时 Admin。
2. Refresh 重新拉取。
3. 离线仅提示、无 proxy 请求。
4. unknown role 仅提示、无 proxy 请求。
5. 无 Abort/Drain 等写入口。

## 9. 实现触点（预期）

| 区域 | 变更 |
|---|---|
| `apps/raypx2-center/ui/src/pages/NodeDetail.tsx` | Tab + `NodeTunnels` |
| `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx` | 上述行为测试 |
| `apps/raypx2-center/ui/dist/**` | `npm run build` 嵌入产物 |
| `apps/raypx2-center/README.md` | 一行说明 Tunnels Tab |
| `apps/raypx2-center/ui/src/styles.css` | 默认不改 |
| raypx2 | 无代码变更（API 已存在） |

## 10. 决策记录

| 决策 | 选择 |
|---|---|
| 数据到达方式 | A：实时代理（非 status_summary 上报） |
| 读写 | A：只读 |
| 列集合 | A：对齐本地 Admin Console |
| UI 位置 | B：节点详情独立 Tunnels Tab |
| 实现路径 | 方案 1：SPA 直调既有 Admin proxy |
