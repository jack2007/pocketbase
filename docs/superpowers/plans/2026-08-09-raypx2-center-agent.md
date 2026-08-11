# raypx2 Center Agent Follow-up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **执行归属（2026-08-10）：** Agent 后续开发以 **raypx2 仓库副本**为准：`/home/jack/src/raypx2/docs/superpowers/plans/2026-08-09-raypx2-center-agent.md`。本文件为 pocketbase 侧归档镜像，请勿在 pocketbase 工作区继续推进 Agent 任务。

**Goal:** 在 raypx2 内嵌出站 Center Agent，与已交付的 `apps/raypx2-center` 完成真实节点 E2E（enroll → WSS → summary → Admin proxy），并补齐 Agent 侧文档。

**Architecture:** Agent 进程内嵌、出站连接中心；`enroll` HTTP 换票后以 Bearer 连 `WSS /api/agent/ws`；应用层 JSON 帧；`http_proxy_req` 仅转发到本机 `admin_base_url` 且 path 前缀 `/api/v1/`。TLS/WSS 使用已接入的 **cpp-httplib + bundled quictls**（禁止系统 libssl）。

**Tech Stack:** C++17、nlohmann/json、`httplib::Client` / `httplib::ws::WebSocketClient`、`CPPHTTPLIB_OPENSSL_SUPPORT` + `tcpquic_quictls*`（Win/Linux/macOS）；中心侧 PocketBase 已就绪，本计划以 raypx2 为主。

**Spec:** [docs/superpowers/specs/2026-08-08-raypx2-center-console-design.md](../specs/2026-08-08-raypx2-center-console-design.md)  
**Parent plan:** [2026-08-08-raypx2-center-console.md](./2026-08-08-raypx2-center-console.md)（Tasks 1–4、7–10 中心侧已交付）

## Global Constraints

- 跨仓库：Agent / 配置 / Agent 文档在 `/home/jack/src/raypx2`；中心仅在 E2E/文档交叉引用时改动。
- `center.enabled=false`（默认）时行为与现网一致。
- WebSocket：**锁定** `cpp-httplib` `ws::WebSocketClient`；HTTPS/WSS 仅 bundled quictls（已有 CMake：`tcpquic_quictls_headers` / `tcpquic_quictls`）。
- Admin Bearer **永不**写入上行帧、审计或修订正文。
- Proxy path 仅允许 `/api/v1/` 前缀；单帧上限约 1 MiB；同节点单活跃 WS（中心侧已 enforce）。
- MVP 不做 yamux、不做时序采样、不做多角色 RBAC。
- 静态 `libmsquic.a` 已含 quictls 时，主程序勿再二次链接 `libssl`/`libcrypto`；无 msquic 的测试目标链 `tcpquic_quictls`。

## Status Snapshot（截至整理日）

| 区域 | 状态 |
|---|---|
| 中心 Tasks 1–4、7–10（集合、enroll/WS/hub、proxy、SPA、Ops、模板/apply、rotate/revoke） | **已完成**（pocketbase `apps/raypx2-center`） |
| raypx2：httplib OPENSSL + quictls CMake | **已完成**（待在 raypx2 仓库单独提交） |
| raypx2：`center` 配置解析（原 Task 5） | **未做** → 本计划 Task A |
| raypx2：内嵌 Agent（原 Task 6） | **未做** → 本计划 Task B |
| 真节点 E2E + Agent 文档（原 Task 7/10 残余） | **未做** → 本计划 Task C |

## File Structure（raypx2）

| Path | Responsibility |
|---|---|
| `src/config/config.h` / `config.cpp` | 解析 `center` 块 → `TqCenterConfig` |
| `src/unittest/config_router_test.cpp`（或现有 config 测） | center 解析单测 |
| `docs/config_guide_cn.md` | `center` 配置一小节 |
| `src/runtime/center_agent_protocol.h` | 帧 type 常量（与中心 `protocol.Frame` 对齐） |
| `src/runtime/center_agent.h` / `center_agent.cpp` | enroll、WSS、心跳、summary、proxy、重连 |
| `src/unittest/center_agent_test.cpp` | path 白名单、帧编解码、（可选）mock WS |
| `src/main.cpp`（或 Admin 启动路径） | `center.enabled` 时 Start/Stop Agent |
| `src/CMakeLists.txt` | 新源文件 + 测试目标；Agent 目标链 `cpp_httplib`（及必要时 `tcpquic_quictls`） |
| `docs/center-agent_cn.md` | 运行手册：配置样例、与中心对接、排障 |
| `build.md` | 已记 httplib+quictls；Agent 落地后补一句运行依赖即可 |

---

### Task A: `center` 配置解析

**Files:**
- Modify: `src/config/config.h`、`src/config/config.cpp`
- Modify: 现有 config unittest
- Modify: `docs/config_guide_cn.md`

**Interfaces:**

```cpp
struct TqCenterConfig {
    bool Enabled{false};
    std::string Url;                 // https://center.example （开发可用 http/ws）
    std::string NodeKey;
    std::string EnrollSecretFile;
    std::string AdminBaseUrl;        // 默认由 AdminListen 推导，如 http://127.0.0.1:2345
};
// TqConfig 内: TqCenterConfig Center;
```

- JSON：`center.enabled`、`center.url`、`center.node_key`、`center.enroll_secret_file`、`center.admin_base_url`
- 省略整个 `center` ≡ `Enabled=false`

- [ ] **Step 1: Write failing config parse test**（有 center 块填字段；无块 Enabled=false）

- [ ] **Step 2: Run config test target — expect FAIL**

- [ ] **Step 3: Implement parse + `docs/config_guide_cn.md` 一小节**

- [ ] **Step 4: Run tests — PASS**

- [ ] **Step 5: Commit（raypx2）**

```bash
cd /home/jack/src/raypx2
git add src/config docs/config_guide_cn.md src/unittest
git commit -m "feat(config): add optional center agent settings"
```

---

### Task B: 内嵌 Center Agent（enroll + WSS + proxy）

**Files:**
- Create: `src/runtime/center_agent_protocol.h`
- Create: `src/runtime/center_agent.h`、`center_agent.cpp`
- Create: `src/unittest/center_agent_test.cpp`
- Modify: Admin 启动后调用 Start；进程退出 Stop
- Modify: `src/CMakeLists.txt`

**Interfaces:**
- `bool TqCenterAgentStart(const TqConfig&, /* admin token provider */, std::string& err)`
- `void TqCenterAgentStop()`
- 行为顺序：
  1. 读 `enroll_secret_file`
  2. `httplib::Client`/`SSLClient`：`POST {url}/api/agent/enroll`
  3. `ws::WebSocketClient`：`ws`/`wss` + `Authorization: Bearer <session_token>` → `/api/agent/ws`
  4. 独立线程阻塞 `read()`；处理 `welcome` / `ping`→`pong` / `http_proxy_req`→本地 Admin→`http_proxy_res`
  5. 定时上行 `status_summary`（降维，复用现有 health/metrics）
  6. 断线指数退避 1s→30s；session 未过期可直连 WS，否则 re-enroll
- `TqCenterAgentIsAllowedAdminPath`：必须含 `/api/v1/` 前缀（拒绝 `/console/` 等）
- TLS：`set_ca_cert_path` / 校验策略与产品既有 PEM/CA 习惯一致；**不**引入系统 OpenSSL
- 实现时优先 Text 帧 `send(std::string)`，与中心 `wsjson` 对齐

- [ ] **Step 1: Failing unit tests** — `IsAllowedAdminPath` + 最小帧 parse/serialize

```cpp
TEST(CenterAgent, RejectsNonV1Path) {
  EXPECT_FALSE(TqCenterAgentIsAllowedAdminPath("/console/"));
  EXPECT_TRUE(TqCenterAgentIsAllowedAdminPath("/api/v1/health"));
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement Agent**（httplib HTTPS + WSS；Admin token 仅本地 HTTP 注入）

- [ ] **Step 4: Unit tests PASS；手动 `ws://` 连本地 center 冒烟（可选）**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(runtime): embed outbound center agent"
```

---

### Task C: 真节点 E2E + Agent 文档

**Files:**
- Create: `docs/center-agent_cn.md`（链到 pocketbase 中心 spec + 配置样例 + 排障）
- Modify: `apps/raypx2-center/README.md` 仅在需要时补充「真 Agent」联调一节（pocketbase 仓）
- 可选：raypx2 侧轻量 system test 脚本（本地 center + 单节点）

**E2E checklist:**
1. 启动 `raypx2-center`，创建 superuser与 node，保存一次 `enroll_secret`
2. raypx2 配置 `center.enabled=true` + secret 文件 + `node_key` + center URL
3. 启动 raypx2 → SPA Nodes：`online=true`，`last_seen` / summary 更新
4. NodeDetail Ops：至少 1 次 GET（如 health）+ 1 次写，确认中心 `audit_logs`
5. `rotate-enroll` / `revoke`：旧连接断开、旧 secret 无法 re-enroll

- [ ] **Step 1: Write `docs/center-agent_cn.md`**

- [ ] **Step 2: Run E2E checklist against local center（开发可用 `ws://`/`http://`；记录生产须 `wss`/`https`）**

- [ ] **Step 3: Fix gaps found in Agent or docs**

- [ ] **Step 4: Commit raypx2（及如有的 pocketbase README 补丁）**

```bash
# raypx2
git add docs/center-agent_cn.md
git commit -m "docs: add center agent runbook"

# pocketbase（若改了 README）
git add apps/raypx2-center/README.md
git commit -m "docs(center): note real agent E2E wiring"
```

---

## Prerequisite Commit（若尚未提交）

将已落地的 httplib+quictls CMake 变更在 raypx2 **单独提交**，避免与 Task A/B 混在一起：

```bash
cd /home/jack/src/raypx2
git add src/CMakeLists.txt build.md \
  scripts/check-system-capability-api.py \
  tests/scripts/test_check_system_capability_api.py
git commit -m "build: enable cpp-httplib OpenSSL via bundled quictls"
```

---

## Milestone Exit Criteria

| Milestone | Done when |
|---|---|
| A | `center` 可解析；默认关闭；config 测绿 |
| B | Agent 单测绿；进程可 enroll + 维持 WS + 本地 proxy |
| C | SPA 见真实 online/summary；≥1 读 ≥1 写经中心；文档齐全 |

## Suggested Order

1. 提交 quictls/httplib 前置（若未提交）  
2. Task A → Task B → Task C  
3. 不回头大改中心协议；若帧/路径有歧义，以中心已实现的 `protocol.Frame` 与 `/api/agent/*` 为准  

## Self-Review Notes

1. 中心侧协议与 API 已实现，本计划不重复 Tasks 1–4/8–10。  
2. WSS 库与 TLS 选型已锁定，Task B 不再做选型分叉。  
3. E2E 依赖双仓同时可用；开发机可先 `http`/`ws`，生产文档必须写清 `https`/`wss`。  
4. `AdminBaseUrl` 默认推导逻辑在 Task A 定契约，Task B 只消费。  
