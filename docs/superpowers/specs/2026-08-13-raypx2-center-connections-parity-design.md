# raypx2 Center Connections 对齐与 Config UI 删除设计

**日期：** 2026-08-13  
**状态：** 已批准  
**实现计划：** [docs/superpowers/plans/2026-08-13-raypx2-center-connections-parity.md](../plans/2026-08-13-raypx2-center-connections-parity.md)  
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center/ui`）  
**对照实现：** `/home/jack/src/raypx2/src/admin_console`（Connections 表、行内速率编辑、Detail）  
**前置：** Client Peers 字段对齐（`2026-08-12-raypx2-center-client-peers-field-parity-design.md`）

## 1. 背景与目标

Center 节点详情的 Connections 页目前只在详情抽屉里改 `min/max_send_rate_kbps`，列也少于 Admin Console。Client 缺少对端 Server 发送速率（`server-send-rate`）。Config 表单与 Peers / ACL 功能重复。

**目标：**

1. Client Connections 的列与行内编辑能力对齐 Admin Console（含 client rate 与 server-send-rate）。
2. Server Connections 的列与行内编辑能力对齐 Admin Console。
3. 删除 client / server（以及 unknown）节点详情的 Config UI；后端 config API 保留给 Apply / Templates。

### 1.1 决策摘要

| 项 | 决定 |
|---|---|
| 实现位置 | 仅改 `apps/raypx2-center/ui/`（方案 1：表格内联） |
| 后端 | 不改 Go；Connections 仍经 `proxyNode` |
| Config | 删除 Config UI 与前端 `getNodeConfig` / `putNodeConfig`；Go `GET/PUT /api/center/nodes/{node_key}/config` 保留 |
| Server compression | 不迁到 ACL；删除 Config 后 Center 不再展示/编辑 |
| Server Detail | 不做（Admin Console 无此抽屉） |
| i18n / 自动刷新 | 不做 |

### 1.2 非目标

- 不改 raypx2 Admin Console 或节点 Admin 协议
- 不改 Center Go config / apply / templates / ACL 后端
- 不把 Server compression desired/applied 迁到 ACL tab
- 不做中英文切换、不做 3s 自动刷新

## 2. Connections UI

改 `ConnectionsTab`。表格放在现有 `DataTableShell` 内横向滚动。速率在行内输入；详情抽屉不再承担速率编辑。

抽出 `parseRateBounds` 到 `connection-form-helpers.ts`（及对应单测），供 Client / Server Apply 共用。

### 2.1 Client 列表

只读列（顺序对齐 Admin Console）：

`connection_id` · `peer_id` · `slot_index` · `generation` · `connected` · `retry_scheduled` · `state` · `encryption` · `compression_mode` · `compression_level` · `path` · `local` · `peer` · `active_tunnels` · `last_error`

行内可写（表头名对齐 Console，绑定字段如下）：

| 表头 | 输入绑定 |
|---|---|
| `client_min_send_rate_kbps` | `min_send_rate_kbps`，缺省 `0` |
| `client_max_send_rate_kbps` | `max_send_rate_kbps`，缺省 `0` |
| `server_min_send_rate_kbps` | `effective_server_tx_min_kbps`，缺省 `0` |
| `server_max_send_rate_kbps` | `effective_server_tx_max_kbps`，缺省 `0` |

操作（同一 actions 单元格）：

| 按钮 | 行为 |
|---|---|
| Detail | `GET /api/v1/peers/{peer_id}/connections/{connection_id}`，Sheet 只展示 JSON |
| Apply client rate | `PATCH /api/v1/peers/{peer_id}/connections/{connection_id}` |
| Apply server rate | `PATCH /api/v1/peers/{peer_id}/connections/{connection_id}/server-send-rate` |

PATCH body 均为：

```json
{
  "min_send_rate_kbps": 0,
  "max_send_rate_kbps": 0
}
```

加载路径不变：`GET /api/v1/peers`，再对每个 peer `GET /api/v1/peers/{id}/connections`，行上附带 `peer_id`。

### 2.2 Server 列表

只读列：

`connection_id` · `peer` · `remote_address` · `state` · `encryption` · `active_streams` · `total_streams_opened` · `active_tunnels` · `last_error`

| 列 | 读取 |
|---|---|
| `peer` | 非空 `client_name`，否则 `peer-{remote_address}`；`remote_address` 为空时用 `peer-unknown` |
| `total_streams_opened` | `total_streams_opened \|\| total_streams` |

行内可写：`min_send_rate_kbps` / `max_send_rate_kbps`，缺省 `0`。

操作：仅 **Apply** → `PATCH /api/v1/server/connections/{connection_id}`，body 与 Client 相同。无 Detail。

加载：`GET /api/v1/server/connections`。

### 2.3 校验与可用性

`parseRateBounds(minText, maxText)` 与 Admin Console 一致：

1. trim 后必须整段匹配 `/^\d+$/`，否则报错：`Minimum and maximum send rates must be safe non-negative integers.`
2. 转为 Number 后必须是 `Number.isSafeInteger` 且 `>= 0`，否则同一错误。
3. 若 min 与 max 均非 0 且 min > max，报错：`Minimum send rate must not exceed maximum send rate when both are non-zero.`
4. 通过则返回 `{ min_send_rate_kbps, max_send_rate_kbps }`。

- 校验失败：页面错误提示，不发 PATCH。
- 节点 offline：输入与 Apply / Detail 禁用；保留现有 offline 提示。
- 缺 `connection_id`（Client 还缺 `peer_id`）：对应按钮禁用。
- 成功：toast，然后重新 load 列表。
- 非 client/server 角色：保持「Connections are not available for this node role.」

行内输入为受控状态，按 `connection_id`（Client 再加 `peer_id`）分键。Refresh / 成功 load 后用接口值重置草稿。

## 3. Config UI 删除

### 3.1 节点详情 tabs

| 角色 | tabs |
|---|---|
| client | `overview` · `peers` · `connections` · `tunnels` · `audit` |
| server | `overview` · `peers` · `connections` · `tunnels` · `acl` · `audit` |
| unknown | `overview` · `audit` |

去掉 Config dirty 离开确认（`configDirty` / `leaveConfig` / `window.confirm`）。

### 3.2 删除的前端文件与符号

删除：

- `apps/raypx2-center/ui/src/pages/node-detail/ConfigTab.tsx`
- `apps/raypx2-center/ui/src/pages/node-detail/config-helpers.ts`

从 `api.ts` 删除（仅 Config UI 使用）：

- `getNodeConfig` / `putNodeConfig`
- `NodeConfigResponse` / `NodeConfigUpdateResult`
- `listConfigRevisions`（已无调用方）

`ConfigRevision` 若删除后无引用则一并删除。

**保留：** Go `internal/centerapi` config handlers、`configmerge`、Apply Jobs、Templates、ACL tab、Peers tab。

## 4. 测试策略

不新增 Go 测试。SPA：`npm test`（`apps/raypx2-center/ui`）。

### 4.1 helpers

`connection-form-helpers.test.ts`：合法 bounds；非法整数；min > max 且均非 0；min 或 max 为 0 时不比较大小。

### 4.2 NodeDetail

新增：

1. Client 列表渲染 Admin 对齐的关键列。
2. Apply client rate 调用 `PATCH /api/v1/peers/{id}/connections/{id}`，body 为解析后的 bounds。
3. Apply server rate 调用 `PATCH .../server-send-rate`。
4. Server 列表渲染 Admin 对齐的关键列；`peer` 与 `total_streams_opened` 按 §2.2 读取。
5. Server Apply 调用 `PATCH /api/v1/server/connections/{id}`。
6. 非法速率不发请求。
7. Client / server / unknown 均无 Config tab。
8. 离线时 Apply 禁用。

删除所有 Config tab 用例（Form/JSON、compression、draft peer、dirty confirm 等）。

`center config API` 的 HTTP 错误暴露用例改绑到仍存在的 `centerRequest` 封装（例如 `proxyNode`），不断言已删除的 `getNodeConfig`。

## 5. 成功标准

1. 操作员可在 Client Connections 行内 Apply client rate 与 Apply server rate，路径与 body 与 Admin Console 一致。
2. 操作员可在 Server Connections 行内 Apply 本地 pacing bounds。
3. Client / Server 列表列与 Admin Console 对齐。
4. 节点详情不再出现 Config tab；前端不再调用 `getNodeConfig` / `putNodeConfig`。
5. Center Go config API 仍可供 Apply / Templates 使用。

## 6. 相关文档

- [2026-08-12-raypx2-center-client-peers-field-parity-design.md](./2026-08-12-raypx2-center-client-peers-field-parity-design.md)
- [2026-08-10-raypx2-center-node-config-design.md](./2026-08-10-raypx2-center-node-config-design.md)
- [2026-08-11-raypx2-center-tunnels-tab-design.md](./2026-08-11-raypx2-center-tunnels-tab-design.md)
