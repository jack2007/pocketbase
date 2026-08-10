# raypx2 Center 单节点在线配置设计

**日期：** 2026-08-10  
**状态：** 对话设计已批准；待用户审阅本文件  
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center`）  
**前置：** Agent 出站连接、Admin proxy、节点在线态已可用（见 `2026-08-08-raypx2-center-console-design.md` 与 raypx2 e2e 冒烟）

## 1. 背景与目标

在中心 SPA 对**单个在线节点**编辑并下发配置，支持 **UI 表单**与 **JSON** 两种编辑方式。保存后经已有 Agent 隧道写入节点 Admin，并记录配置修订与审计。

### 1.1 决策摘要

| 项 | 决定 |
|---|---|
| 范围 | 单节点（非本阶段多节点 apply） |
| 编辑单位 | 角色主配置文档（client `/api/v1/config`，server `/api/v1/server/config`） |
| 表单深度 | 高频可写子集；其余仅 JSON 编辑，保存前裁剪到同一白名单 |
| 保存策略 | 直接经中心 API 写节点；成功后写 `config_revisions` |
| 多余字段 | 白名单裁剪；`ignored_fields` 提示；不静默当成功写入 |
| 架构 | 中心 `GET/PUT /api/center/nodes/{node_key}/config`；SPA 不直写 merge/白名单 |
| Ops 关系 | Config 管配置文档；Ops 管运行态；ACL 编辑从 Ops 迁到 Config |

### 1.2 非目标

- 中心 desired 草稿箱、确认后再 Apply、持续声明式调和
- 本阶段改 Templates / Apply Jobs 产品行为（可共享 `configmerge` 演进）
- 修改 raypx2 Admin 协议或语义
- 经 Config 删除 client peer（省略 peer = 保留；删除留给后续/专用 API）
- 多操作员 RBAC、密钥明文入库

## 2. 架构与数据流

```text
[SPA Config 页]
  表单 ←→ 同一份 draft
       │
       ▼
GET/PUT /api/center/nodes/{node_key}/config   (superuser)
       │
       ├─ 校验节点、role、写路径要求 online
       ├─ GET 实际配置（proxy）
       ├─ 密钥拒绝；白名单裁剪；记录 ignored_fields
       ├─ merge 后 proxy 下发
       │    server: PATCH /api/v1/server/config
       │    client: PUT  /api/v1/config（整包 merge 结果）
       ├─ config_revisions: actual/pull + desired/manual_edit
       └─ audit: node.config.update
       │
       ▼
[在线 Agent WS 隧道] → 本机 loopback Admin
```

| 模块 | 职责 |
|---|---|
| `centerapi` | 新 config GET/PUT handler、错误映射 |
| `configmerge`（扩展） | 白名单裁剪、密钥拒绝、server/client merge；与 apply 共用规则 |
| `agenthub` proxy | 既有反向隧道，不改帧协议 |
| SPA Config | 双模式编辑器、脏检查、Ops ACL 迁移 |

## 3. API 契约

### 3.1 `GET /api/center/nodes/{node_key}/config`

- 认证：PocketBase superuser  
- 节点不存在 → `404 node_not_found`  
- **offline 允许**：`live` 为 `null`，仍返回 `recent_revisions`  
- 在线时 proxy GET：
  - `server` → `/api/v1/server/config`
  - `client` → `/api/v1/config`
- 响应形状：

```json
{
  "node_key": "node-prod-client-01",
  "role": "client",
  "online": true,
  "live": {},
  "editor_draft": {},
  "writable_paths": ["allow_targets", "peers", "..."],
  "recent_revisions": [
    {
      "id": "...",
      "kind": "desired",
      "source": "manual_edit",
      "created": "..."
    }
  ]
}
```

- `live`：Admin 返回体，经展示脱敏（去掉已知密钥路径）  
- `editor_draft`：从 `live` 抽出的**可写投影**（见 §4），供表单/JSON 初始绑定  
- `role=unknown`：可返回 live（若有），但标明不可写

### 3.2 `PUT /api/center/nodes/{node_key}/config`

表示「提交编辑器意图」；中心内部映射为 server `PATCH` / client Admin `PUT`。

请求：

```json
{
  "content": {}
}
```

处理顺序：

1. 鉴权；节点存在；`role` ∈ `{client,server}`，否则 `400 unsupported_node_role`  
2. `online != true` → `409 node_offline`  
3. proxy GET actual；失败映射既有 proxy 错误码  
4. `rejectSecrets(content)` → 失败 `400 secret_field_forbidden`  
5. `patch, ignored = whitelistTrim(content, role)`  
6. 若 patch 无可写变更 → `400 empty_config_update`  
7. merge + 下发（§4）  
8. 成功：写 `actual`/`pull` 与 `desired`/`manual_edit` revisions；`audit` `node.config.update`  
9. 响应：

```json
{
  "applied": {},
  "ignored_fields": ["tls.ca"],
  "revision_id": "...",
  "admin_status": 200
}
```

Admin 业务错误：携带 `admin_status` 与脱敏后的 `admin_body`（或等价字段）；**不**写 `desired` revision。

### 3.3 不新增

- 不为表单单独开字段级 API；表单与 JSON 均提交 `content`  
- 不改变既有通用 `.../proxy` 契约（Ops 仍可用）

## 4. 白名单、投影与 merge

字段名以实现时 Admin JSON 为准；中心在读写边界做一次归一（例如 client peer 的 `id`/`peer_id`、`proto_peer`/`quic_peer`），`configmerge` 与 apply 共用同一归一+白名单表。若现有 `MergeClientPeers` 尚未允许 `connection.min_send_rate_kbps` / `max_send_rate_kbps`，本功能实现时扩展白名单并补测，避免表单可编、merge 却拒绝。

### 4.1 Server

**可写下发白名单**

| 路径 | 表单 MVP | 说明 |
|---|---|---|
| `allow_targets` | 是 | string[]；多行文本 |
| `deny_targets` | 是 | string[] |
| `connection.compression.level` | 是 | 与 Admin PATCH 一致；可能伴随 `restart_required` |
| `connection.max_send_rate_kbps` | 是 | 与 Admin PATCH 一致 |
| `connection.min_send_rate_kbps` | 是（若 Admin 已支持） | 不支持则列入只读/ignored，不假装可写 |

**编辑投影：** GET 常含 `connection_config.desired|applied`、`pending_fields`、`restart_required` 等。`editor_draft` 只含可写扁平/嵌套字段（ACL + `connection.*` 取 desired）；只读态在 UI 旁路展示，不得进入 PUT `content` 的下发体。

**merge / 下发：** `writeBody = whitelist(content)` → `PATCH /api/v1/server/config`。

### 4.2 Client

**可写范围：** 仅通过白名单改 `peers`（upsert）。未出现在提交 peers 列表中的既有 peer **保留**。

**Peer 表单 MVP 字段：** 标识、远端地址、`socks_listen` / `http_listen`、`port_forwards`、连接数、`enabled`、`connection.min/max_send_rate_kbps`。`connection.encryption` / `compression` 等允许出现在 JSON 且在白名单内则可写；表单可后置。

**禁止经 Config 删除 peer：** 表单不提供「保存后从节点删除 peer」；避免与 upsert-保留语义冲突。

**merge / 下发：** `desired = MergeClientPeers(actual, {peers: ...})` → `PUT /api/v1/config` 整包 `desired`。

### 4.3 密钥拒绝

扩展现有 `rejectSecrets`：拒绝 TLS 私钥材料、`admin` token 类字段、`center.enroll_secret*` 等。命中则整请求失败，不裁剪后继续写。

### 4.4 `ignored_fields`

不在角色白名单内的路径进入该列表并提示；不写入节点。白名单内但被 Admin 拒绝的字段不算 ignored，走 Admin 错误。

## 5. UI

### 5.1 Config 页

- 标题区：online 徽章、刷新、保存  
- 模式切换：`表单 | JSON`，绑定同一 draft  
- JSON → 表单：解析失败则禁止切换并报错  
- 脏检查：相对上次成功加载/保存的 snapshot；未保存离开时页内确认（MVP）  
- offline / `unknown`：只读；写按钮 disabled  
- 保存成功：用 `applied`（或重新 GET）刷新 draft；展示 `ignored_fields`；刷新 revision 表  
- 保存失败：保留 draft，展示错误  

Revision 历史表保留在编辑器下方。

### 5.2 Ops 调整

- 移除 Ops 内 server ACL 编辑（迁入 Config 表单）  
- 保留 health、connections、peer enable/disable/drain、abort 等运行态操作  
- 跨 tab 不自动推送；各自 Refresh 即可  

## 6. 错误、审计与修订

| 情况 | HTTP | code |
|---|---|---|
| 未授权 | 401/403 | 现有 |
| 节点不存在 | 404 | `node_not_found` |
| 角色不支持 | 400 | `unsupported_node_role` |
| 离线写 | 409 | `node_offline` |
| 密钥字段 | 400 | `secret_field_forbidden` |
| 无有效变更 | 400 | `empty_config_update` |
| 隧道/proxy | 现有映射 | `node_offline` / `tunnel_timeout` / `admin_unreachable` / … |
| Admin 拒绝 | 包装/透传 | 含 `admin_status` |

审计 `node.config.update`：记录 actor、node、role、content_hash、ignored_fields 摘要、admin_status；不存完整配置明文与密钥。

修订：下发成功才写 `desired/manual_edit`；拉取 actual 的 `pull` 在写路径中于下发前落库（与 apply runner 模式对齐）。

## 7. 测试策略

### 7.1 中心 Go

- 白名单裁剪、密钥拒绝、empty update  
- server patch 子集；client merge 保留未提及 peer  
- handler：offline 409、角色错误；proxy mock 成功后 revision + audit  

### 7.2 SPA

- 表单 ↔ JSON 同步；非法 JSON 不可切表单  
- offline 禁用保存  
- Ops 不再含 ACL 编辑；Config 含 ACL/peers 表单  
- 成功保存展示 ignored_fields 并出现新 revision  

### 7.3 手工冒烟

1. 在线 server 改 ACL → 节点生效 → revision 可见  
2. 在线 client 改某一 peer → 其他 peer 仍在  
3. JSON 含密钥 → 400  
4. 离线保存 → 409  

## 8. 成功标准

1. 操作员可在 Config 用表单或 JSON 修改在线 client/server 的白名单配置并成功下发。  
2. 非白名单字段被忽略并可见提示；密钥字段被拒绝。  
3. 每次成功下发可在 revision 与 audit 中追溯（无密钥明文）。  
4. Ops 不再承担 ACL 配置编辑；运行态操作仍可用。  
5. 不回归既有 enroll / WS / proxy 运维路径。  

## 9. 相关文档

- `docs/superpowers/specs/2026-08-08-raypx2-center-console-design.md`  
- raypx2：`docs/admin-api/interface.md`、`docs/config_guide_cn.md`、`docs/center-agent_cn.md`  
