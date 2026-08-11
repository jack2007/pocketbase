# raypx2 Center Client Peer CRUD 设计

**日期：** 2026-08-11  
**状态：** 已批准  
**实现计划：** [docs/superpowers/plans/2026-08-11-raypx2-center-client-peer-crud.md](../plans/2026-08-11-raypx2-center-client-peer-crud.md)  
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center`）  
**前置：** 单节点 Config（`2026-08-10-raypx2-center-node-config-design.md`）、Agent proxy、Ops peer enable/disable

## 1. 背景与目标

Config 已支持 client peer 的查询与 upsert 编辑，但表单缺少「新增 peer」入口，且设计明确禁止经 Config 删除 peer。本阶段补齐 client peer 的完整增删改查：

| 操作 | 入口 | 机制 |
|---|---|---|
| 查 | Config + Ops | 既有 `GET .../config` 与 Ops `GET /api/v1/peers` |
| 改 | Config | 既有 `PUT .../config` + `MergeClientPeers` upsert-保留 |
| 增 | Config 表单 | 手填 `peer_id`，写入 draft 后走既有 PUT upsert |
| 删 | Ops | 新 `DELETE /api/center/nodes/{node_key}/peers/{peer_id}` |

### 1.1 决策摘要

| 项 | 决定 |
|---|---|
| 删除位置 | Ops，不放 Config |
| 删除实现 | Center 专用 API：GET config → 去掉 peer → PUT 整包；写 revision + audit |
| Config merge | **保持** upsert-保留；`{peers:[]}` 仍 `empty_config_update` |
| 新增 peer_id | 操作员手填；已存在 peer 的 `peer_id` 只读 |
| Draft 移除 | Config 仅可移除「本次新增、未保存」的本地草稿 peer |

### 1.2 非目标

- 不改 raypx2 Admin 协议；不依赖 Admin 独立 DELETE peer
- Config / 模板 merge **不**改为 replace-all
- 不做批量删除、不做 server 侧 peer
- 不改 Templates / Apply Jobs 的 peer upsert 语义

## 2. 架构与数据流

```text
[SPA Config]
  Add peer (手填 peer_id) / 编辑字段
       │
       ▼
PUT /api/center/nodes/{node_key}/config
       │
       └─ MergeClientPeers (upsert-保留) → PUT /api/v1/config

[SPA Ops]
  Delete peer (confirm)
       │
       ▼
DELETE /api/center/nodes/{node_key}/peers/{peer_id}
       │
       ├─ GET /api/v1/config → actual/pull revision
       ├─ RemoveClientPeer
       ├─ PUT /api/v1/config 整包 desired
       ├─ desired/peer_delete revision
       └─ audit node.peer.delete
```

## 3. API 契约

### 3.1 `DELETE /api/center/nodes/{node_key}/peers/{peer_id}`

- 认证：PocketBase superuser
- 仅 `role == client`；否则 `400 unsupported_node_role`
- 节点不存在 → `404 node_not_found`
- peer 不在 actual → `404 peer_not_found`（**不**发起 PUT）
- offline：库内预检 `409 node_offline`；hub 竞态 `503 node_offline`

成功响应：

```json
{
  "peer_id": "peer-a",
  "revision_id": "...",
  "admin_status": 200
}
```

处理顺序：

1. 鉴权；节点存在；role=client  
2. online 预检  
3. proxy GET `/api/v1/config`；写 `actual`/`pull`  
4. `NormalizeClientPeers` + `RemoveClientPeer`  
5. proxy PUT `/api/v1/config` 整包 desired  
6. 成功：写 `desired`/`peer_delete`；audit `node.peer.delete`  
7. Admin 失败：保留 pull；不写 desired；返回既有 admin/proxy 错误映射  

审计 `request_summary` 固定键：`node_key`、`peer_id`、`content_hash`、`admin_status`。禁止写入完整 config 或密钥。

### 3.2 Config PUT

行为不变。新增 peer 通过提交含新 `peer_id` 的 `peers[]` 完成 upsert。

## 4. `configmerge`

新增：

```go
func RemoveClientPeer(actual map[string]any, peerID string) (map[string]any, error)
```

- 在 `peers` 中按 `peer_id` 删除一项，返回克隆后的整包  
- 找不到 → error（含可识别文案，供 handler 映射 `peer_not_found`）  
- **不**改变 `MergeClientPeers`

## 5. UI

### 5.1 Config

- **Add peer**：追加空 peer 对象；该 peer 的 `peer_id` 可编辑  
- 已有 peer（加载时已有非空 `peer_id`）：`peer_id` 只读  
- **Remove from draft**：仅对本次新增的草稿 peer 显示；不暗示删除节点上已有 peer  
- 文案：删除请到 Ops  
- 保存前校验：每个 peer 非空 `peer_id`；draft 内 `peer_id` 不重复  

### 5.2 Ops

- Client peers 表 Action 增加 **Delete**  
- `window.confirm` 后调用 Center DELETE API  
- 成功后刷新 Ops 列表  
- offline / writing / 无 peer_id 时禁用  

## 6. 错误码

| 情况 | HTTP | code |
|---|---|---|
| 节点不存在 | 404 | `node_not_found` |
| 非 client | 400 | `unsupported_node_role` |
| peer 不存在 | 404 | `peer_not_found` |
| 库内离线 | 409 | `node_offline` |
| hub 竞态离线 | 503 | `node_offline` |
| Admin / proxy | 既有映射 | `admin_rejected` / `tunnel_timeout` / … |

## 7. 测试策略

- Go：`RemoveClientPeer` 成功/缺失；DELETE handler 成功、审计、`peer_not_found`、非 client、offline  
- SPA：Add peer 可编辑 peer_id；既有 peer_id 只读；Ops Delete confirm + API；既有 Config 测试不回归  

## 8. 成功标准

1. 操作员可在 Config 表单新增并保存 client peer（手填 peer_id）。  
2. 操作员可在 Ops 删除 client peer，revision `source=peer_delete` 与 audit `node.peer.delete` 可追溯。  
3. Config merge 仍 upsert-保留；模板/Apply 行为不回归。  
4. 离线节点不可删除 peer。  

## 9. 相关文档

- [2026-08-10-raypx2-center-node-config-design.md](./2026-08-10-raypx2-center-node-config-design.md)（§4.2 禁止经 Config 删除 peer — 本设计以 Ops API 承接）
