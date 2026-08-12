# raypx2 Center Client Peers 字段对齐设计

**日期：** 2026-08-12  
**状态：** 已批准  
**实现计划：** [docs/superpowers/plans/2026-08-12-raypx2-center-client-peers-field-parity.md](../plans/2026-08-12-raypx2-center-client-peers-field-parity.md)
**范围仓库：** `/home/jack/src/pocketbase`（`apps/raypx2-center`）  
**对照实现：** `/home/jack/src/raypx2/src/admin_console`（Peers 表单与列表）  
**前置：** Client Peer CRUD（`2026-08-11-raypx2-center-client-peer-crud-design.md`）

## 1. 背景与目标

Center 节点详情的 Client Peers 页已具备增删改查，但相对 raypx2 Admin Console 仍缺少 connection 相关字段，列表运维列也不完整。

**目标：** 仅在 PeersTab 上对齐 Admin Console 的表单字段与列表列；Config 页本轮不动。

### 1.1 决策摘要

| 项 | 决定 |
|---|---|
| 实现位置 | 仅改 `PeersTab`（方案 1：UI-only） |
| Config 页 | 保留，本轮不删除/不隐藏 |
| 后端 | 不改 Center peer API；仍经 `proxyNode` 直连节点 Admin |
| Connections 字段名 | 保存发 `quic_connections`；读取兼容 `proto_connections \|\| quic_connections \|\| connection_count` |
| paths / port_forwards | 保持 JSON textarea |
| Server peers | 本轮不扩展 |

### 1.2 非目标

- 不删除或隐藏 Config tab
- 不新增/修改 Center 后端 peer 路由
- 不做 `proto_connections` 字段重命名对齐（属更完整 parity，留待后续）
- 不把 paths / port_forwards 改成结构化编辑器
- 不改 server peers 只读汇总表

## 2. UI / 字段

### 2.1 Client 列表列

横向滚动表格，列顺序对齐 Admin Console：

`peer_id` · `state` · `enabled` · `quic_peer` · `socks_listen` · `http_listen` · `connection_count` · `connected_connections` · `active_streams` · `total_streams` · `reconnects` · `last_error` · `actions`

`actions` 仍为 Edit / Enable|Disable / Delete。

### 2.2 编辑表单

**可写（已有）：** `peer_id`（编辑时只读）、`quic_peer`、Connections、`socks_listen`、`http_listen`、enabled、paths、port_forwards。

**可写（新增）：**

| 字段 | 控件 | 默认 |
|---|---|---|
| Desired encryption | select：`enabled` / `disabled` | `enabled` |
| Desired compression mode | select：`disabled` / `enabled` | `disabled` |
| Desired compression level | number，1–22 | `1` |

**只读（新增，来自 `peer.connection_config`）：**

| 展示 | 来源 |
|---|---|
| Applied encryption | `connection_config.applied.encryption` |
| Applied compression mode | `connection_config.applied.compression.mode` |
| Applied compression level | `connection_config.applied.compression.level` |
| Restart required | `connection_config.restart_required` |

创建模式：只读区显示默认值（encryption=`enabled`，compression=`disabled`，level=`1`，restart=`false`）。

表单底部增加简短 callout：新建 peer 的 connection 设置立即生效；编辑已有 peer 的 connection 只更新 desired，需重启客户端进程后生效。

### 2.3 保存 payload

```json
{
  "peer_id": "...",
  "quic_peer": "...",
  "quic_connections": 1,
  "socks_listen": "...",
  "http_listen": "...",
  "enabled": true,
  "paths": [],
  "port_forwards": [],
  "connection": {
    "encryption": "enabled",
    "compression": {
      "mode": "disabled",
      "level": 1
    }
  }
}
```

**校验：**

- `peer_id` 非空（已有）
- compression level 为 1–22 整数，否则前端拦截不发请求
- `paths` / `port_forwards` JSON 可解析（已有）

## 3. 数据流

```text
PeersTab
  GET  /api/v1/peers                         → 列表列 + 编辑时读 connection_config
  POST /api/v1/peers                         → 创建（含 connection）
  PUT  /api/v1/peers/{peer_id}               → 更新（含 connection）
  POST /api/v1/peers/{peer_id}:enable|disable
  DELETE /api/center/nodes/{node_key}/peers/{peer_id}
```

全部仍经现有 `proxyNode` / `deleteNodePeer`。Center 无新路由。

### 3.1 读字段约定

| UI | 读取顺序 |
|---|---|
| Connections | `proto_connections \|\| quic_connections \|\| connection_count \|\| 1` |
| Desired connection | `connection_config.desired \|\| peer.connection \|\| defaults` |
| Applied / restart | `connection_config.applied` / `connection_config.restart_required` |

Defaults：

```json
{
  "encryption": "enabled",
  "compression": { "mode": "disabled", "level": 1 }
}
```

### 3.2 错误与可用性

- compression level 非法：前端错误提示，不发请求
- paths / port_forwards JSON 非法：沿用现有 error/toast
- offline / writing / 无 peer_id：按钮仍禁用

## 4. 测试策略

扩展 SPA 测试（`NodeDetail.test.tsx` 或等价 Peers 用例）：

1. 打开 Edit 时 desired / applied / restart_required 正确回填
2. Save 时 PUT body 含 `connection.encryption` 与 `connection.compression`
3. 列表渲染新增列（对 mock peer 断言关键表头/单元格）

不新增 Go 测试（后端无改动）。

## 5. 成功标准

1. 操作员可在 Client Peers 表单编辑并保存 encryption / compression（含 level）。
2. 编辑已有 peer 时可看到 applied* 与 restart_required。
3. Client peers 列表具备与 Admin Console 对齐的运维列。
4. Config 页入口与行为不变。

## 6. 相关文档

- [2026-08-11-raypx2-center-client-peer-crud-design.md](./2026-08-11-raypx2-center-client-peer-crud-design.md)
- [2026-08-10-raypx2-center-node-config-design.md](./2026-08-10-raypx2-center-node-config-design.md)
