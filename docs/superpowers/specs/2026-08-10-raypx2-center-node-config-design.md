# raypx2 Center 单节点在线配置设计

**日期：** 2026-08-10  
**状态：** 已批准（2026-08-10 评审修订：server patch-only、速率字段、deep merge、offline 双码、`applied`/脱敏/脏检查）  
**实现计划：** [docs/superpowers/plans/2026-08-10-raypx2-center-node-config.md](../plans/2026-08-10-raypx2-center-node-config.md)  
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
- 本阶段不改 Templates / Apply Jobs 的产品流程与 UI（创建 job、模板 CRUD 等）
- 修改 raypx2 Admin 协议或语义
- 经 Config 删除 client peer（省略 peer = 保留；删除留给后续/专用 API）
- 多操作员 RBAC、密钥明文入库
- Server 配置文档上的 `min/max_send_rate_kbps`（现网 `PATCH /api/v1/server/config` 不接受；速率热更新走其它 Admin 入口，本阶段不做）

**共享 merge 说明：** 扩展 `configmerge` 白名单（例如 client peer `connection` 速率字段）会被 Apply 模板路径一并继承，属预期。Server 模板校验仍走仅 ACL 的 `MergeServerACL`（或等价约束），除非另开任务显式放开模板里的 `connection.compression.level`。

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

- `live`：Admin 返回体，经展示脱敏（已知密钥路径替换为 `[REDACTED]`，复用/对齐 apply `redact` 规则）  
- `editor_draft`：从 `live` 抽出的**可写投影**（见 §4），供表单/JSON 初始绑定；offline 时为空对象 `{}`  
- `role=unknown`：可返回脱敏 `live`（若在线），`editor_draft` 为空；标明不可写（PUT 仍 `400 unsupported_node_role`）

### 3.2 `PUT /api/center/nodes/{node_key}/config`

表示「提交编辑器意图」；中心内部映射为 server `PATCH` / client Admin `PUT`。

请求：

```json
{
  "content": {}
}
```

处理顺序（先本地校验，再打隧道）：

1. 鉴权；节点存在；`role` ∈ `{client,server}`，否则 `400 unsupported_node_role`  
2. `online != true` → `409 node_offline`（库内离线预检）  
3. `rejectSecrets(content)` → 失败 `400 secret_field_forbidden`（**不**发起 proxy）  
4. `patch, ignored = whitelistTrim(content, role)`  
5. 若 patch 无可写变更 → `400 empty_config_update`（**不**发起 proxy）  
6. proxy GET actual；失败映射既有 proxy 错误码（见下）  
7. 写 `actual`/`pull` revision（脱敏 content）  
8. merge + 下发（§4）：  
   - **server：** `desired = MergeServerConfig(actual, patch)` 仅用于修订/响应；**`PATCH` body = `patch`（裁剪结果）**，不得发送 merge 后的整包 actual  
   - **client：** `desired = MergeClientPeers(actual, patch)`；**`PUT` body = `desired` 整包**  
9. 成功：写 `desired`/`manual_edit` revision；`audit` `node.config.update`  
10. 响应：

```json
{
  "applied": {},
  "ignored_fields": ["tls.ca"],
  "revision_id": "...",
  "admin_status": 200
}
```

**`applied` 语义（固定）：** 成功下发后，对节点再做一次 GET，取其 `editor_draft` 投影（脱敏后）作为 `applied`，供 SPA 刷新 draft。避免把 Admin 原始响应、裸 patch、或未投影的整包 desired 混用。实现上也可在同一次 handler 内 GET→写→GET；客户端不得猜测其它含义。

Admin 业务错误：携带 `admin_status` 与脱敏后的 `admin_body`（或等价字段）；**不**写 `desired` revision。已写的 `actual/pull` 可保留（与 apply 对齐）或回滚——**本设计选择保留 pull**，因它反映写前真实快照。

**`node_offline` 双状态码：**

| 来源 | HTTP | code |
|---|---|---|
| 库内 `online=false` 预检 | 409 | `node_offline` |
| 预检通过但 hub `ErrNodeOffline`（竞态掉线） | 503 | `node_offline` |

SPA 对两者均按离线处理（禁用写、提示刷新）。
### 3.3 不新增

- 不为表单单独开字段级 API；表单与 JSON 均提交 `content`  
- 不改变既有通用 `.../proxy` 契约（Ops 仍可用）

## 4. 白名单、投影与 merge

字段名以实现时 Admin JSON 为准；中心在读写边界做一次归一（例如 client peer 的 `id`/`peer_id`、`proto_peer`/`quic_peer`），`configmerge` 与 apply 共用同一归一+白名单表。现有 `MergeClientPeers` 若尚未允许 `connection.min_send_rate_kbps` / `max_send_rate_kbps`，本功能实现时扩展白名单并补测。

### 4.1 Server

以 raypx2 `TqParseServerConfigPatch` 为准（现网）。

**可写下发白名单**

| 路径 | 表单 MVP | 说明 |
|---|---|---|
| `allow_targets` | 是 | string[]；多行文本 |
| `deny_targets` | 是 | string[] |
| `connection.compression.level` | 是 | 1–22；可能伴随 `restart_required` |

**明确不可写（trim → `ignored_fields`）：** `connection.min_send_rate_kbps`、`connection.max_send_rate_kbps`、以及一切 startup / 未知字段（如 `listen`、`tls`）。

**编辑投影：** GET 常含 `connection_config.desired|applied`、`pending_fields`、`restart_required` 等。`editor_draft` 只含可写字段（ACL + `connection.compression.level`，level 取 desired）；`restart_required` / `pending_fields` 在 UI **旁路只读展示**，不得进入 PUT 下发体。

**merge / 下发：**

- `patch = whitelistTrim(content)`  
- `desired = MergeServerConfig(actual, patch)` → 写入 revision / 推导展示  
- **`PATCH /api/v1/server/config` 的 body = `patch` only**（不得发送 merge 后的整包）

### 4.2 Client

**可写范围：** 仅通过白名单改 `peers`（upsert）。未出现在提交 peers 列表中的既有 peer **保留**。

**Peer 表单 MVP 字段：** 标识、远端地址、`socks_listen` / `http_listen`、`port_forwards`、连接数、`enabled`、`connection.min/max_send_rate_kbps`。`connection.encryption` / `compression` 等允许出现在 JSON 且在白名单内则可写；表单可后置。

**`connection` deep merge（必须）：** upsert peer 时，对 `connection` 及其嵌套 `compression` 做深层合并，禁止用部分 `connection` 对象整键覆盖 actual，以免表单只改速率时抹掉 `encryption` / `compression` 等既有字段。补回归测试覆盖此行为。

**禁止经 Config 删除 peer：** 表单不提供「保存后从节点删除 peer」；避免与 upsert-保留语义冲突。提交 `{peers:[]}` 视为无可写变更 → `400 empty_config_update`（不发起 PUT）。

**merge / 下发：** `desired = MergeClientPeers(actual, {peers: ...})` → `PUT /api/v1/config` 整包 `desired`。

### 4.3 密钥拒绝

扩展现有 `rejectSecrets`，至少拒绝：

- TLS 私钥材料（含 `tls.key`）  
- `admin` token 类字段（键名含 `token` / `password` / `secret` 的惯例路径）  
- `enroll_secret`、`enroll_secret_file`、以及 `center.enroll_secret*`  

命中则整请求 `400 secret_field_forbidden`，不裁剪后继续写。与 Agent 入站 `config_snapshot` 敏感键判定对齐。

### 4.4 `ignored_fields`

不在角色白名单内的路径进入该列表并提示；不写入节点。白名单内但被 Admin 拒绝的字段不算 ignored，走 Admin 错误。

## 5. UI

### 5.1 Config 页

- 标题区：online 徽章、刷新、保存  
- 模式切换：`表单 | JSON`，绑定同一 draft  
- JSON → 表单：解析失败则禁止切换并报错  
- 脏检查：相对上次成功加载/保存的 `baseline`；未保存时切换 Node Detail tab 或返回列表须页内确认（MVP；`beforeunload` 可选）  
- offline / `unknown`：只读；写按钮 disabled  
- 对 API `node_offline`（HTTP **409 或 503**）统一提示离线并禁用保存  
- server 旁路只读展示 `restart_required` / `pending_fields`（来自 `live`，非 draft）  
- 保存成功：用响应 `applied`（再 GET 的 editor_draft）刷新 draft 与 baseline；展示 `ignored_fields`；刷新 revision 表  
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
| 库内离线预检 | 409 | `node_offline` |
| hub 竞态离线 | 503 | `node_offline` |
| 密钥字段 | 400 | `secret_field_forbidden` |
| 无有效变更 | 400 | `empty_config_update` |
| 其它隧道/proxy | 现有映射 | `tunnel_timeout` / `admin_unreachable` / … |
| Admin 拒绝 | 包装/透传 | 含 `admin_status` |

审计 `node.config.update` 的 `request_summary` **固定键**：

- `node_key`、`role`、`content_hash`、`ignored_fields`（字符串数组，可截断）、`admin_status`  
- **禁止**写入完整 `content` / `applied` / 密钥明文  

修订：下发成功才写 `desired/manual_edit`；`actual/pull` 在写路径中于下发前落库（脱敏）；Admin 失败时保留已写入的 pull。

## 7. 测试策略

### 7.1 中心 Go

- 白名单裁剪、密钥拒绝（含 enroll_secret）、empty update（含 `{peers:[]}`）  
- server：**PATCH body = patch only**；client：merge 保留未提及 peer；**connection deep merge**  
- handler：409 与 503 `node_offline`；role 错误；live redact；`applied` 为再 GET 投影；audit summary 键约束  

### 7.2 SPA

- 表单 ↔ JSON 同步；非法 JSON 不可切表单  
- 脏离开确认；offline / 409 / 503 禁用或提示  
- Ops 不再含 ACL 编辑；Config 含 ACL/peers 表单与 restart 旁路只读  
- 成功保存展示 ignored_fields 并用 `applied` 刷新  

### 7.3 手工冒烟

1. 在线 server 改 ACL → 节点生效 → revision 可见；夹带非白名单字段仅 ignored  
2. 在线 client 只改 rate → 其他 peer 与 encryption 仍在  
3. JSON 含密钥 / enroll_secret → 400  
4. 离线或停 Agent → 409 或 503  

## 8. 成功标准

1. 操作员可在 Config 用表单或 JSON 修改在线 client/server 的白名单配置并成功下发。  
2. 非白名单字段被忽略并可见提示；密钥字段被拒绝。  
3. 每次成功下发可在 revision 与 audit 中追溯（无密钥明文）。  
4. Ops 不再承担 ACL 配置编辑；运行态操作仍可用。  
5. 不回归既有 enroll / WS / proxy 运维路径。  

## 9. 相关文档

- `docs/superpowers/specs/2026-08-08-raypx2-center-console-design.md`  
- raypx2：`docs/admin-api/interface.md`、`docs/config_guide_cn.md`、`docs/center-agent_cn.md`  
