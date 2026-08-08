# raypx2 中心管理控制台设计

**日期：** 2026-08-08  
**状态：** 已批准  
**范围仓库：**

- 中心应用：`/home/jack/src/pocketbase`（PocketBase 作 Go 框架，新增 `apps/raypx2-center`）
- Agent：`/home/jack/src/raypx2`（进程内嵌出站 Agent）

## 1. 背景与目标

raypx2 已具备单机 Admin HTTP API（`/api/v1/*`）与单机 Admin Console（`/console/`）。默认 Admin 监听 `127.0.0.1`，无法在多机场景下统一运维。

本设计在 PocketBase 之上构建**舰队中心控制台**：

- **主目标（B）：** 在一个 SPA 内对多台 client/server 做统一运维（透传现有 Admin 能力）。
- **轻量 C：** 节点登记、期望/实际配置修订、配置模板应用到多节点；**不做**持续声明式调和。
- **连接模型：** 节点**出站**连接中心；Admin 继续只绑 loopback。
- **历史（MVP）：** 仅 P0——当前态摘要 + 配置修订史 + 审计；**不做**时序采样曲线。

### 1.1 非目标

- 不修改 PocketBase 核心 upstream 行为。
- 不做多操作员 RBAC（仅 superuser）。
- 不做节点级时序指标库、connection/tunnel 明细史。
- 不经中心转发用户业务流量（QUIC/TCP 数据面仍在节点之间）。
- 不 iframe 复用单机 `/console/`。

### 1.2 约束摘要

| 项 | 决定 |
|---|---|
| 产品方向 | B 为主 + 轻量 C（MVP 含模板多节点应用） |
| 连接 | 出站 Agent |
| Agent 形态 | 内嵌 raypx2 |
| 中心形态 | PocketBase 作框架，自建应用 |
| 登记 | **每节点独立** enroll secret |
| 规模 | 20–100 节点 |
| UI | 独立 SPA |
| 操作员 | 单一管理员（superuser） |
| 历史 | P0 only |
| 认证 | 两阶段：enroll → session → WS |

## 2. 架构与模块边界

```text
[SPA] → [PocketBase App: 认证/集合/代理 API]
              ↑ 持久 WebSocket（每节点一条）
[raypx2 内嵌 Agent] → 本地 loopback Admin API (/api/v1/*)
```

| 模块 | 职责 | 不负责 |
|---|---|---|
| PocketBase App | superuser 登录、集合、Realtime、自定义路由 | 改 PB 核心 |
| Agent Hub | 登记会话、按 `node_key` 路由、反向隧道 inflight | 解释 Admin 业务语义 |
| Admin Proxy | 将中心请求映射为「节点 + Admin 路径」 | 重写 Admin |
| Config Control | 期望/模板、修订、apply job | GitOps 持续收敛 |
| Audit | 操作摘要 | 存密钥 |
| raypx2 Agent | 出站、隧道转发、上报身份/摘要 | 当配置中心 |

**数据权威**

- 运行态：各 raypx2 进程（经隧道实时读）。
- 中心：节点登记、enroll、期望配置/模板、应用任务、审计、当前摘要与配置修订。

**安全边界**

- 公网只暴露中心；节点 Admin 默认 loopback。
- 库内只存 enroll/session 的哈希。
- 本地 Admin Bearer 永不离开节点。

## 3. 参考项目（架构样本，非整仓依赖）

| 项目 | 借鉴 |
|---|---|
| fosrl/pangolin + fosrl/newt | 每站点独立凭证、两阶段认证、WS 控制通道 |
| wenisch-tech/proxera | 出站 WebSocket 反向 HTTP 隧道、登记位 |
| PipeOpsHQ/pipeops-k8-agent | 出站反向访问本地 API、Yamux-over-WS 思路 |
| pockethost/pockethost | PocketBase mothership 应用组织 |
| fatedier/frp | 单机 Admin 边界；不作多节点中心 |

## 4. 两阶段认证（enroll → session → WS）

1. 运维在中心为**每台**节点生成 `node_key` + `enroll_secret`（明文仅展示一次；库存 hash）。
2. Agent：`POST /api/agent/enroll` 提交 `node_key` + `enroll_secret` 及角色/版本/主机名。
3. 中心校验后签发短期 `session_token`（服务端 session 表，TTL 默认 30 分钟），返回 `ws_url`。
4. Agent 用 `Authorization: Bearer <session_token>` 连接 `WSS /api/agent/ws`。
5. 同 `node_key` 新连接成功后踢掉旧连接并 revoke 旧 session（防双开幽灵在线）。
6. `POST /api/agent/session/refresh` 在过期前续期。

长期密钥不出现在 WS 帧中；吊销 enroll 后拒绝换票并断开现有连接。

## 5. 数据模型（PocketBase 集合）

### 5.1 `nodes`

`node_key`（unique）、`name`、`role`（client/server/unknown）、`enroll_secret_hash`、`enroll_status`（active/revoked）、`labels`（json）、`hostname`、`agent_version`、`last_seen_at`、`online`、`created_by`。

### 5.2 `agent_sessions`

`node`（relation）、`token_hash`（unique）、`expires_at`、`revoked_at`、`client_info`（json）。

### 5.3 `node_status`（每节点一行，覆盖更新）

`node`（unique relation）、`health_status`、`uptime_seconds`、`last_error`、`summary`（json 降维摘要）、`config_hash`、`fetched_at`。

`summary` 含连接/peer/tunnel/relay 聚合字段及 peer 短列表；不含完整 tunnel/relay 明细。

### 5.4 `config_revisions`

`node`、`kind`（actual/desired）、`source`（pull/template_apply/manual_edit/proxy_write）、`content_hash`、`content`（脱敏 json）、`diff_summary`、`actor`。

### 5.5 `config_templates`

`name`、`target_role`、`body`（json）、`version`、`notes`。

### 5.6 `apply_jobs` / `apply_job_targets`

Job：`template`、`template_version`、`status`、`created_by`、时间戳。  
Target：`job`、`node`、`status`、`error`、`result_revision`。

### 5.7 `audit_logs`

`actor`、`action`、`node`（optional）、`request_summary`（json，无密钥）、`ip`。

### 5.8 MVP 不做的集合

`node_metrics_samples`、`peer_metrics_samples`、`runtime_events`。

### 5.9 访问控制

业务集合对匿名关闭。Agent **不**用 PB 用户 JWT 访问集合，只走 `/api/agent/*`。SPA 使用 superuser。

## 6. Agent 协议

### 6.1 HTTP

- `POST /api/agent/enroll` — 换票。
- `POST /api/agent/session/refresh` — Bearer session 续期。
- `GET|WSS /api/agent/ws` — Hub（Header Bearer）。

### 6.2 帧格式

```json
{"type":"...","id":"<uuid>","ts":"...","payload":{}}
```

类型：`welcome`、`ping`/`pong`、`status_summary`、`config_snapshot`、`http_proxy_req`、`http_proxy_res`、`error`、`bye`。

未知 `type` 不得导致断连。单帧上限约 1 MiB。

### 6.3 反向 Admin 隧道

Hub → Agent：`http_proxy_req{method,path,headers,body_b64,timeout_ms}`。  
Agent → 仅 `admin_base_url` + path 前缀 `/api/v1/`；注入本地 Admin token；响应 `http_proxy_res`。  
中心 SPA API：`POST /api/center/nodes/{node_key}/proxy`（superuser）。

每节点 proxy inflight 上限 8。心跳间隔默认 15s；摘要间隔默认 30s；`3 * heartbeat` 无消息标 offline。

### 6.4 raypx2 配置增量

```json
{
  "center": {
    "enabled": true,
    "url": "https://center.example",
    "node_key": "node-prod-client-01",
    "enroll_secret_file": "/var/lib/raypx2/center-enroll.json",
    "admin_base_url": "http://127.0.0.1:2345"
  }
}
```

`center.enabled=false` 时行为与现网一致。

## 7. 中心应用与 SPA

### 7.1 目录

```text
apps/raypx2-center/
  main.go
  internal/agenthub|agentapi|centerapi|collections|audit|apply|configmerge/
  ui/   # Vite SPA，构建后 embed
```

运行：`go run ./apps/raypx2-center serve`。

### 7.2 中心路由

| 前缀 | 认证 |
|---|---|
| `/api/agent/*` | enroll 或 session |
| `/api/center/nodes`、`.../proxy`、templates、apply-jobs、audit | superuser |
| `/app/*` 或 `/` | SPA |

### 7.3 SPA 导航

登录 → Overview → Nodes → Node Detail（概览 / 运维代理 / 配置 / 审计）→ Templates → Apply Jobs → Audit。

运维页 MVP 优先级：Health/Metrics → Client Peers/Connections → Server ACL/Connections → Tunnels → Relay 明细后置。

列表用中心库（可选 Realtime）；细操走 proxy。

## 8. 配置模板应用

执行器对每个 target：offline 则 failed/skipped → proxy 拉实际配置 → 按角色白名单 merge → proxy 写回 → 写 `config_revisions`。  
禁止模板包含 TLS 私钥或 admin token。  
失败不自动风暴重试；可手动重跑 job。

## 9. 错误处理

- Enroll 对外统一 `invalid credentials`；审计区分原因。
- 代理错误区分 `node_offline`、`tunnel_timeout`、`admin_unreachable` 与透传 Admin 状态码。
- Apply job 支持 `partial`。
- MVP 不保证跨节点事务、断线操作排队。

## 10. 测试策略

- 中心：哈希/session/merge/白名单单元测试；enroll→WS→proxy 集成测试。
- Agent：帧编解码、重连、路径白名单；与本地 center 的系统测试。
- SPA：登录、列表、offline 禁用写、错误展示；冒烟创建节点→上线→模板应用。

## 11. 里程碑

| 里程碑 | 交付 |
|---|---|
| M1 骨架 | 集合、enroll/session/WS、nodes/status、SPA 登录与节点列表 |
| M2 隧道运维 | proxy + 运维页 + 审计 |
| M3 配置与模板 | revisions、templates、apply_jobs |
| M4 加固 | refresh、踢旧、吊销/轮换、HTTPS 文档、集成套件 |

## 12. 第三方开源组件分析

### 12.1 整用（A）

- **PocketBase**：中心后端骨架、superuser、SQLite 集合、Realtime、embed。

### 12.2 库用（B）

| 能力 | 选型 |
|---|---|
| WebSocket | `github.com/coder/websocket`（或与生态一致的等价库） |
| 多路复用 | MVP 用 JSON 帧 + `id`；后续可选 `hashicorp/yamux` |
| 哈希 | `golang.org/x/crypto` argon2id 或 bcrypt（实现时锁定一种） |
| 配置合并机械层 | JSON Merge Patch（如 `evanphx/json-patch`） |
| SPA | Vite + TypeScript + PocketBase JS SDK |
| Embed | Go `embed` |
| Agent JSON | 复用 nlohmann/json |
| Agent HTTPS/WSS | 优先复用 raypx2 现有网络/TLS 栈 |

### 12.3 参考不引用（C）

Pangolin/Newt、Proxera、PipeOps agent、PocketHost、frp——只借模式，不当运行时依赖。

### 12.4 MVP 不用（D）

Prometheus/VictoriaMetrics、Redis/NATS/Asynq、Keycloak/OIDC、整仓 frp/rathole/Pangolin 当隧道。

### 12.5 必须自研

协议状态机、Hub 路由、Admin 路径白名单与审计脱敏、raypx2 配置合并语义、内嵌 Agent、舰队 SPA 页面。

## 13. 成功标准（MVP）

1. 每节点独立 enroll，两阶段后维持出站 WS。  
2. SPA 查看 20–100 规模节点在线与当前摘要（无时序曲线）。  
3. 对在线节点透传 Admin 运维读写。  
4. 配置修订可查；模板可应用到多节点。  
5. 关键操作可审计；秘密材料不落明文。  
6. `center.enabled=false` 时 raypx2 与现网一致。

## 14. 相关文档

- raypx2 Admin API：`/home/jack/src/raypx2/docs/admin-api/interface.md`
- raypx2 配置：`/home/jack/src/raypx2/docs/config_guide_cn.md`
