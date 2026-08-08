# raypx2 Center Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 PocketBase 应用中交付多 raypx2 节点的中心管理控制台（出站 Agent、两阶段认证、Admin 反向隧道、配置模板应用到多节点），并在 raypx2 内嵌最小可用 Agent。

**Architecture:** 中心为 `apps/raypx2-center`（`pocketbase.New()` + 自定义路由 + SQLite 集合 + embed SPA）。节点内嵌 Agent 出站 `enroll → session → WSS`，经 JSON 帧把中心代理请求转到本机 `127.0.0.1` Admin `/api/v1/*`。P0 只持久化当前摘要、配置修订与审计。

**Tech Stack:** Go 1.25 + PocketBase（本仓库）、`coder/websocket`、`golang.org/x/crypto/bcrypt`、JSON Merge Patch（`evanphx/json-patch`）、Vite + TypeScript + React + `pocketbase` JS SDK；raypx2 侧 C++17、nlohmann/json、复用现有 HTTP/TLS 栈。

**Spec:** [docs/superpowers/specs/2026-08-08-raypx2-center-console-design.md](../specs/2026-08-08-raypx2-center-console-design.md)

## Global Constraints

- 不修改 PocketBase 核心包行为；应用代码只放在 `apps/raypx2-center/`。
- 每节点独立 enroll secret；库内只存 bcrypt hash；明文只在创建接口返回一次。
- Agent 只允许代理 path 前缀 `/api/v1/` 到配置的 loopback Admin。
- 本地 Admin Bearer 永不上传中心、不进审计/修订正文。
- MVP 不做时序采样集合、不做多角色 RBAC、不上 yamux（JSON 帧 + `id`）。
- Secret 哈希锁定 **bcrypt**；session TTL 默认 **30 分钟**；同 `node_key` 单活跃 WS。
- SPA 框架锁定 **Vite + TypeScript + React**；产品入口 `/app/`。
- `center.enabled=false`（默认）时 raypx2 行为与现网一致。
- 跨仓库：中心改动在 `/home/jack/src/pocketbase`；Agent 改动在 `/home/jack/src/raypx2`。

## File Structure

### PocketBase 仓库（中心）

| Path | Responsibility |
|---|---|
| `apps/raypx2-center/main.go` | 启动 PB、挂载路由、embed UI |
| `apps/raypx2-center/internal/collections/ensure.go` | 启动时确保集合与字段/规则 |
| `apps/raypx2-center/internal/crypto/secret.go` | 生成 secret、bcrypt hash/verify |
| `apps/raypx2-center/internal/agentapi/enroll.go` | enroll + refresh HTTP |
| `apps/raypx2-center/internal/agenthub/hub.go` | 连接表、踢旧、收发帧 |
| `apps/raypx2-center/internal/agenthub/proxy.go` | 等待 `http_proxy_res` |
| `apps/raypx2-center/internal/agentapi/ws.go` | WS upgrade + session 校验 |
| `apps/raypx2-center/internal/centerapi/nodes.go` | 节点 CRUD、一次返回 enroll 明文 |
| `apps/raypx2-center/internal/centerapi/proxy.go` | superuser → Hub 代理 |
| `apps/raypx2-center/internal/centerapi/templates.go` | 模板 CRUD |
| `apps/raypx2-center/internal/apply/runner.go` | apply job 执行 |
| `apps/raypx2-center/internal/configmerge/merge.go` | 白名单 + merge-patch |
| `apps/raypx2-center/internal/audit/audit.go` | 写 `audit_logs` |
| `apps/raypx2-center/internal/protocol/frame.go` | 帧类型与编解码 |
| `apps/raypx2-center/ui/` | React SPA |
| `apps/raypx2-center/internal/.../*_test.go` | 单元/集成测试 |

### raypx2 仓库（Agent）

| Path | Responsibility |
|---|---|
| `src/config/config.h` / `config.cpp` | 解析 `center` 块 |
| `src/runtime/center_agent.h` / `center_agent.cpp` | enroll、WS、心跳、summary、proxy |
| `src/runtime/center_agent_protocol.h` | 帧常量（与中心对齐） |
| `src/main.cpp`（或等价启动路径） | `center.enabled` 时启动 Agent |
| `docs/admin-api/` 或 `docs/center-agent_cn.md` | 简短协议说明（链到中心 spec） |
| 对应 unittest | 帧/白名单/重连 |

---

### Task 1: 中心应用骨架与集合确保

**Files:**
- Create: `apps/raypx2-center/main.go`
- Create: `apps/raypx2-center/internal/collections/ensure.go`
- Create: `apps/raypx2-center/internal/collections/ensure_test.go`
- Test: `apps/raypx2-center/internal/collections/ensure_test.go`

**Interfaces:**
- Consumes: `pocketbase.New()`, `OnServe` / app bootstrap hooks（按当前 PB API）
- Produces: `EnsureCollections(app core.App) error` — 创建 `nodes`、`agent_sessions`、`node_status`、`config_revisions`、`config_templates`、`apply_jobs`、`apply_job_targets`、`audit_logs`；匿名规则全拒

- [ ] **Step 1: Write the failing test**

```go
func TestEnsureCollectionsCreatesNodes(t *testing.T) {
    app, _ := tests.NewTestApp() // 使用仓库现有 test helper；若无则用临时 dataDir 启动
    defer app.Cleanup()
    if err := collections.EnsureCollections(app); err != nil {
        t.Fatal(err)
    }
    if _, err := app.FindCollectionByNameOrId("nodes"); err != nil {
        t.Fatalf("nodes missing: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/collections/ -count=1`  
Expected: FAIL（包或函数不存在）

- [ ] **Step 3: Write minimal implementation**

实现 `EnsureCollections`：按 spec §5 创建各集合字段；所有 list/view/create/update/delete 规则设为仅超级用户（或空规则 + 仅走自定义 API）。`main.go` 调用 `pocketbase.New()`，在 serve 前 `EnsureCollections`。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/raypx2-center/internal/collections/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/main.go apps/raypx2-center/internal/collections
git commit -m "feat(center): bootstrap raypx2-center app and collections"
```

---

### Task 2: Secret 哈希与 enroll/session 核心

**Files:**
- Create: `apps/raypx2-center/internal/crypto/secret.go`
- Create: `apps/raypx2-center/internal/crypto/secret_test.go`
- Create: `apps/raypx2-center/internal/agentapi/enroll.go`
- Create: `apps/raypx2-center/internal/agentapi/enroll_test.go`
- Create: `apps/raypx2-center/internal/audit/audit.go`
- Modify: `apps/raypx2-center/main.go` — 注册 `POST /api/agent/enroll`、`POST /api/agent/session/refresh`

**Interfaces:**
- Produces:
  - `GenerateEnrollSecret() (plaintext string, hash string, err error)`
  - `VerifySecret(hash, plaintext string) bool`
  - `HashToken(plaintext string) (string, error)`
  - `HandleEnroll(e *core.RequestEvent) error` — 校验 node、写 `agent_sessions`、审计
  - `HandleRefresh(e *core.RequestEvent) error`

- [ ] **Step 1: Write failing tests for bcrypt generate/verify and enroll happy path**

```go
func TestGenerateAndVerifyEnrollSecret(t *testing.T) {
    plain, hash, err := crypto.GenerateEnrollSecret()
    if err != nil || plain == "" || hash == "" {
        t.Fatalf("gen: %v", err)
    }
    if !crypto.VerifySecret(hash, plain) {
        t.Fatal("verify failed")
    }
    if crypto.VerifySecret(hash, "wrong") {
        t.Fatal("expected reject")
    }
}
```

- [ ] **Step 2: Run tests — expect FAIL**

Run: `go test ./apps/raypx2-center/internal/crypto/ ./apps/raypx2-center/internal/agentapi/ -count=1`

- [ ] **Step 3: Implement crypto + enroll/refresh**

`GenerateEnrollSecret`：32 字节 `crypto/rand` → base64.RawURLEncoding；`bcrypt.GenerateFromPassword` cost 默认。  
Enroll：查 `nodes` by `node_key`；`enroll_status==active`；verify hash；创建 session（随机 32 字节 token，存 hash，`expires_at=now+30m`）；更新 hostname/version/role；审计 `agent.enroll`；对外错误统一 `{"message":"invalid credentials"}`。  
Refresh：Bearer 查 session 未过期未吊销 → 延期或轮换 token。

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/internal/crypto apps/raypx2-center/internal/agentapi apps/raypx2-center/internal/audit apps/raypx2-center/main.go
git commit -m "feat(center): agent enroll and session refresh"
```

---

### Task 3: 协议帧与 Agent Hub（WS + 踢旧）

**Files:**
- Create: `apps/raypx2-center/internal/protocol/frame.go`
- Create: `apps/raypx2-center/internal/protocol/frame_test.go`
- Create: `apps/raypx2-center/internal/agenthub/hub.go`
- Create: `apps/raypx2-center/internal/agenthub/hub_test.go`
- Create: `apps/raypx2-center/internal/agentapi/ws.go`
- Modify: `apps/raypx2-center/main.go` — 注册 `/api/agent/ws`

**Interfaces:**
- Produces:
  - `type Frame struct { Type string; ID string; TS string; Payload json.RawMessage }`
  - `Hub.Register(nodeKey string, conn Conn) (replaced bool)`
  - `Hub.Unregister(nodeKey, connID string)`
  - `Hub.Send(nodeKey string, frame Frame) error`
  - `Hub.RequestProxy(ctx, nodeKey, ProxyRequest) (ProxyResponse, error)`
  - `HandleWS(e *core.RequestEvent) error`

- [ ] **Step 1: Write failing tests** — frame marshal round-trip；Register 第二次踢掉第一次（用 fake Conn）

```go
func TestHubReplaceConnection(t *testing.T) {
    h := agenthub.New()
    a := &fakeConn{id: "a"}
    b := &fakeConn{id: "b"}
    h.Register("n1", a)
    h.Register("n1", b)
    if !a.closed {
        t.Fatal("expected old conn closed")
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — `coder/websocket` accept；校验 Bearer session；发 `welcome`；读循环处理 `ping`→`pong`、`status_summary`→更新 `nodes`/`node_status`、`config_snapshot`→`config_revisions`；写 path 用 app DAO。心跳超时：`3*15s` 无消息 Unregister + `online=false`。

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(center): agent websocket hub with single-connection replace"
```

---

### Task 4: 中心节点 API + 隧道 proxy API

**Files:**
- Create: `apps/raypx2-center/internal/centerapi/nodes.go`
- Create: `apps/raypx2-center/internal/centerapi/proxy.go`
- Create: `apps/raypx2-center/internal/centerapi/auth.go` — require superuser
- Create: `apps/raypx2-center/internal/agenthub/proxy.go`
- Create: `apps/raypx2-center/internal/centerapi/proxy_test.go`
- Modify: `apps/raypx2-center/main.go`

**Interfaces:**
- `POST /api/center/nodes` → `{node, enroll_secret}`（secret 仅此响应）
- `GET /api/center/nodes`
- `POST /api/center/nodes/{node_key}/proxy` body `{method,path,headers,body}` → Admin 状态与 JSON
- Hub inflight per node ≤ 8；超时默认 10s
- 审计 `proxy.request`：method/path/status/latency，无 body 密钥

- [ ] **Step 1: Write integration-style test with fake Agent conn that replies `http_proxy_res`**

```go
func TestProxyRoundTrip(t *testing.T) {
    // hub + fake conn echoes 200 {"status":"healthy"} for GET /api/v1/health
    // call centerapi proxy handler / Hub.RequestProxy
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement nodes CRUD + proxy** — path 必须以 `/api/v1/` 开头否则 400；offline → 503 `node_offline`

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(center): node management and admin proxy API"
```

---

### Task 5: raypx2 `center` 配置解析

**Files:**
- Modify: `/home/jack/src/raypx2/src/config/config.h`
- Modify: `/home/jack/src/raypx2/src/config/config.cpp`
- Create/Modify: 对应 unittest（现有 config 测试文件）

**Interfaces:**
- `TqConfig` 增加：

```cpp
struct TqCenterConfig {
    bool Enabled{false};
    std::string Url;
    std::string NodeKey;
    std::string EnrollSecretFile;
    std::string AdminBaseUrl; // 默认由 AdminListen 推导
};
TqCenterConfig Center;
```

- JSON 键：`center.enabled`、`center.url`、`center.node_key`、`center.enroll_secret_file`、`center.admin_base_url`
- 省略 `center` ≡ disabled

- [ ] **Step 1: Write failing config parse test** — 含 center 块的 JSON 填入字段；无块时 Enabled=false

- [ ] **Step 2: Run existing config test target — expect FAIL**

- [ ] **Step 3: Implement parse + 文档片段写入 `docs/config_guide_cn.md` 一小节**

- [ ] **Step 4: Run tests — PASS**

- [ ] **Step 5: Commit（raypx2 仓库）**

```bash
cd /home/jack/src/raypx2
git add src/config docs/config_guide_cn.md
git commit -m "feat(config): add optional center agent settings"
```

---

### Task 6: raypx2 内嵌 Center Agent（enroll + WS + proxy）

**Files:**
- Create: `/home/jack/src/raypx2/src/runtime/center_agent_protocol.h`
- Create: `/home/jack/src/raypx2/src/runtime/center_agent.h`
- Create: `/home/jack/src/raypx2/src/runtime/center_agent.cpp`
- Create: unittest for path allowlist + frame parse
- Modify: `src/main.cpp`（或 runtime 启动）在 Admin 启动后 `TqCenterAgentStart` 
- Modify: `src/CMakeLists.txt` 纳入新源文件

**Interfaces:**
- `bool TqCenterAgentStart(const TqConfig&, /* admin token provider */, std::string& err)`
- `void TqCenterAgentStop()`
- 行为：读 enroll secret 文件 → POST `{center.url}/api/agent/enroll` → WSS → 心跳 → 处理 `http_proxy_req` → 本地 HTTP 到 `AdminBaseUrl`（注入 Admin Bearer）→ `http_proxy_res`
- 拒绝 path 不含 `/api/v1/` 前缀
- 重连：指数退避 1s→30s；session 有效直连 WS 否则 re-enroll
- `status_summary`：组装 health/metrics 降维字段（复用现有 JSON API 或内部 snapshot）

- [ ] **Step 1: Write failing unit tests** for `IsAllowedAdminPath` and frame parse

```cpp
TEST(CenterAgent, RejectsNonV1Path) {
  EXPECT_FALSE(TqCenterAgentIsAllowedAdminPath("/console/"));
  EXPECT_TRUE(TqCenterAgentIsAllowedAdminPath("/api/v1/health"));
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement Agent** — HTTPS/WSS 优先用项目已有客户端能力；若缺失，最小引入与现有 TLS 策略一致的依赖（在 `build.md` 记录）。**不得**把 Admin token 写入任何上行帧。

- [ ] **Step 4: Run unit tests — PASS**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(runtime): embed outbound center agent"
```

---

### Task 7: M1 端到端接线与 SPA 骨架

**Files:**
- Create: `apps/raypx2-center/ui/`（Vite React TS）
- Create: `apps/raypx2-center/ui/src/pages/Login.tsx`、`Nodes.tsx`、`Overview.tsx`
- Create: `apps/raypx2-center/ui/src/api.ts` — PB 登录 + `/api/center/*`
- Modify: `apps/raypx2-center/main.go` — embed `ui/dist` 到 `/app/`
- Create: `apps/raypx2-center/README.md` — 启动步骤

**Interfaces:**
- SPA：`pb.admins.authWithPassword`（或当前 PB 超级用户登录 API）后调用 center API
- 页面：登录、Overview（online 计数）、Nodes 表（online/health/role/last_seen）、创建节点对话框（展示一次 enroll_secret）

- [ ] **Step 1: Scaffold Vite app and a smoke test**（若用 Playwright/ vitest，至少保证 `Nodes` 在 mock 数据下渲染行）

- [ ] **Step 2: Build UI and verify embed serves `/app/`**

Run: `cd apps/raypx2-center/ui && npm i && npm run build`  
Run: `go run ./apps/raypx2-center serve --http=127.0.0.1:8090`  
Expected: `GET /app/` → 200

- [ ] **Step 3: Manual E2E checklist**

1. 创建 PB superuser  
2. SPA 登录 → 创建 node → 复制 enroll_secret 到 raypx2 `enroll_secret_file`  
3. 启动 raypx2（client 或 server）`center.enabled=true`  
4. Nodes 页显示 online=true，last_seen 更新  

- [ ] **Step 4: Commit both repos as appropriate**

```bash
# pocketbase
git add apps/raypx2-center && git commit -m "feat(center): SPA skeleton with overview and nodes"
```

---

### Task 8: M2 运维页 + 审计展示

**Files:**
- Create: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`（tabs: Overview / Ops / Audit）
- Create: Ops 子页：Health、Peers、Connections、ServerACL（按 role 切换）
- Modify: `internal/audit` 若需 list API；或 SPA 直接读集合（superuser）
- Test: `tools` 或 `ui` 契约测试 — offline 时写按钮 disabled

**Interfaces:**
- Ops 所有读写经 `POST /api/center/nodes/{key}/proxy`
- Client：`GET/PATCH /api/v1/peers...`、`.../connections...`
- Server：`GET/PATCH /api/v1/server/config`、`GET /api/v1/server/connections`
- 错误展示：HTTP 状态 + JSON `message`/`code`

- [ ] **Step 1: Implement proxy client helper + NodeDetail Ops for health GET**

- [ ] **Step 2: Add peers list + one write**（如 disable peer 或 ACL patch）并确认 `audit_logs` 新增行

- [ ] **Step 3: Manual verify against live raypx2**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(center): node ops pages via admin proxy and audit view"
```

---

### Task 9: M3 配置修订、模板与 apply jobs

**Files:**
- Create: `apps/raypx2-center/internal/configmerge/merge.go` + `_test.go`
- Create: `apps/raypx2-center/internal/centerapi/templates.go`
- Create: `apps/raypx2-center/internal/apply/runner.go` + `_test.go`
- Create: UI pages `Templates.tsx`、`ApplyJobs.tsx`、NodeDetail Config tab

**Interfaces:**
- `MergeServerACL(actual, templateBody) (merged, error)` — 仅 `allow_targets`/`deny_targets` 等白名单
- `MergeClientPeers(actual, templateBody) (merged, error)` — 按 `peer_id` upsert；禁 tls key/token 字段
- `StartApplyJob(app, jobID) error` — 异步逐 target；终态 completed/partial/failed
- 成功写 `config_revisions`（actual + desired 视情况）

- [ ] **Step 1: Failing tests for merge whitelist rejecting `tls.key`**

```go
func TestMergeRejectsTLSKey(t *testing.T) {
    _, err := configmerge.MergeClientPeers(actual, map[string]any{"tls": map[string]any{"key": "SECRET"}})
    if err == nil {
        t.Fatal("expected error")
    }
}
```

- [ ] **Step 2: Implement merge + runner + APIs + UI**

- [ ] **Step 3: E2E** — 两节点（可用一个真节点 + 一个 mock hub conn）apply → 状态可见

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(center): config templates and multi-node apply jobs"
```

---

### Task 10: M4 加固与文档

**Files:**
- Modify: enroll/session/hub — 吊销 enroll、轮换 secret API、`bye{reason}` 
- Create: `apps/raypx2-center/README.md` 生产 HTTPS/`wss` 说明
- Create: `/home/jack/src/raypx2/docs/center-agent_cn.md` — 链到中心 spec + 配置样例
- Create: `apps/raypx2-center/internal/agentapi/enroll_ws_integration_test.go`

**Interfaces:**
- `POST /api/center/nodes/{key}/rotate-enroll` → 新 secret 一次返回；旧 hash 失效；踢 WS
- `POST /api/center/nodes/{key}/revoke` → `enroll_status=revoked`

- [ ] **Step 1: Integration test** — revoke 后 enroll 403/401；旧 WS 断开

- [ ] **Step 2: Implement rotate/revoke + refresh 路径稳态**

- [ ] **Step 3: Document runbooks（中心 README + raypx2 center-agent 文档）**

- [ ] **Step 4: Run full center `go test ./apps/raypx2-center/...` and raypx2 agent unit tests**

- [ ] **Step 5: Commit both repos**

```bash
git commit -am "feat(center): enroll rotate/revoke hardening and docs"
```

---

## Milestone Exit Criteria

| Milestone | Done when |
|---|---|
| M1 (Tasks 1–7) | 真 Agent 上线；SPA 见 online 与 summary |
| M2 (Task 8) | 经中心完成 ≥1 读 ≥1 写 Admin，有审计 |
| M3 (Task 9) | 模板应用到 ≥2 targets，分目标状态可见 |
| M4 (Task 10) | 吊销/轮换有效；文档齐全；测试绿 |

## Self-Review Notes

1. **Spec coverage:** §2–§13 均映射到 Tasks 1–10；P0 历史无时序表；两阶段认证在 Tasks 2–3/6；第三方选型（bcrypt、JSON 帧、无 yamux）写入 Global Constraints。  
2. **Placeholders:** 无 TBD；测试 helper 名称若与仓库不符，实施时改为 `pocketbase/tests` 现有 API。  
3. **Type consistency:** `node_key`、帧 `type`/`id`、proxy path 前缀 `/api/v1/` 全任务一致。  
4. **Multi-repo:** Tasks 5–6、部分 7/10 在 raypx2；其余在 pocketbase。
