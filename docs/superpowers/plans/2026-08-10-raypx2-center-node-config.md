# raypx2 Center Single-Node Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让中心 SPA 对单个在线节点用表单或 JSON 编辑配置，经 `GET/PUT /api/center/nodes/{node_key}/config` 白名单裁剪后直写节点 Admin，并记录 revision 与审计。

**Architecture:** 扩展 `configmerge` 提供裁剪/投影/client upsert；`centerapi` 新增 config handler，经既有 hub proxy 拉写 Admin；SPA Config 页双模式编辑，Ops 去掉 ACL 编辑。不改 raypx2 Admin 协议。

**Tech Stack:** Go + PocketBase（`apps/raypx2-center`）、既有 `agenthub` proxy、Vite + React + TypeScript + Vitest。

**Spec:** [docs/superpowers/specs/2026-08-10-raypx2-center-node-config-design.md](../specs/2026-08-10-raypx2-center-node-config-design.md)

## Global Constraints

- 应用代码只放在 `apps/raypx2-center/`；不修改 PocketBase 核心包行为。
- 保存 = 中心 API 直写在线节点；成功后写 `config_revisions`（`actual/pull` + `desired/manual_edit`）。
- 下发前白名单裁剪；密钥字段整请求拒绝；`ignored_fields` 提示。
- Server Admin PATCH **当前**仅接受：`allow_targets`、`deny_targets`、`connection.compression.level`（以 raypx2 `TqParseServerConfigPatch` 为准；不要假装可写 `min/max_send_rate_kbps`）。
- Client：`PUT /api/v1/config` 整包；未提交的既有 peer **保留**；本阶段不经 Config 删除 peer。
- 本地 Admin Bearer / enroll 明文永不进 revision 正文或审计全文。
- SPA 入口 `/app/`；操作员仅为 superuser。

## File Structure

| Path | Responsibility |
|---|---|
| `apps/raypx2-center/internal/configmerge/merge.go` | 扩展白名单、`TrimForRole`、`EditorDraft`、peer 字段归一、server connection 白名单 |
| `apps/raypx2-center/internal/configmerge/merge_test.go` | 上述单元测试 |
| `apps/raypx2-center/internal/centerapi/config.go` | `HandleGetNodeConfig` / `HandlePutNodeConfig` |
| `apps/raypx2-center/internal/centerapi/config_test.go` | handler 集成测试（mock hub） |
| `apps/raypx2-center/internal/audit/audit.go` | 增加 `ActionNodeConfigUpdate` |
| `apps/raypx2-center/main.go` | 挂载 GET/PUT config 路由 |
| `apps/raypx2-center/ui/src/api.ts` | `getNodeConfig` / `putNodeConfig` |
| `apps/raypx2-center/ui/src/pages/NodeDetail.tsx` | Config 双模式编辑器；Ops 移除 ACL |
| `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx` | SPA 测试 |
| `apps/raypx2-center/README.md` | 简短操作说明 |

---

### Task 1: 扩展 `configmerge`（裁剪、投影、速率字段）

**Files:**
- Modify: `apps/raypx2-center/internal/configmerge/merge.go`
- Modify: `apps/raypx2-center/internal/configmerge/merge_test.go`

**Interfaces:**
- Consumes: 现有 `MergeServerACL`、`MergeClientPeers`、`rejectSecrets`
- Produces:
  - `func TrimForRole(role string, content map[string]any) (patch map[string]any, ignored []string, err error)`
  - `func EditorDraft(role string, live map[string]any) (map[string]any, error)`
  - `func NormalizeClientPeers(content map[string]any) (map[string]any, error)` — `id`→`peer_id`，`proto_peer`→`quic_peer`，`proto_connections`→`quic_connections`
  - 扩展 peer `connection` 白名单：允许 `min_send_rate_kbps`、`max_send_rate_kbps`（非负整数）
  - 扩展 server：新增 `MergeServerConfig` 允许 `connection.compression.level`；`MergeServerACL` 保持可用（委托或并行）
  - `WritablePaths(role string) []string`

- [ ] **Step 1: Write the failing tests**

在 `merge_test.go` 追加：

```go
func TestTrimForRoleServerKeepsACLAndCompressionIgnoresRest(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"deny_targets":  []any{},
		"connection": map[string]any{
			"compression":       map[string]any{"level": float64(5)},
			"max_send_rate_kbps": float64(100000), // Admin PATCH 当前不接受 → ignored
		},
		"listen": ":443",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := patch["allow_targets"]; !ok {
		t.Fatalf("patch=%#v", patch)
	}
	conn := patch["connection"].(map[string]any)
	if _, ok := conn["compression"]; !ok {
		t.Fatalf("expected compression kept: %#v", conn)
	}
	if _, ok := conn["max_send_rate_kbps"]; ok {
		t.Fatalf("max_send_rate must be ignored for server trim: %#v", conn)
	}
	joined := strings.Join(ignored, ",")
	if !strings.Contains(joined, "listen") || !strings.Contains(joined, "max_send_rate_kbps") {
		t.Fatalf("ignored=%v", ignored)
	}
}

func TestTrimForRoleClientPeersPreservesRateFields(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"id":         "peer-a",
			"proto_peer": "10.0.0.2:4433",
			"connection": map[string]any{
				"min_send_rate_kbps": float64(1000),
				"max_send_rate_kbps": float64(50000),
			},
		}},
		"tls": map[string]any{"ca": "certs/ca.crt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	peers := patch["peers"].([]any)
	peer := peers[0].(map[string]any)
	if peer["peer_id"] != "peer-a" || peer["quic_peer"] != "10.0.0.2:4433" {
		t.Fatalf("normalize failed: %#v", peer)
	}
	conn := peer["connection"].(map[string]any)
	if conn["min_send_rate_kbps"] != float64(1000) {
		t.Fatalf("rates missing: %#v", conn)
	}
	if !strings.Contains(strings.Join(ignored, ","), "tls") {
		t.Fatalf("ignored=%v", ignored)
	}
}

func TestEditorDraftServerFlattensDesiredConnection(t *testing.T) {
	t.Parallel()
	draft, err := EditorDraft("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"deny_targets":  []any{},
		"connection_config": map[string]any{
			"desired": map[string]any{
				"compression":       map[string]any{"level": float64(3)},
				"max_send_rate_kbps": float64(0),
			},
			"restart_required": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft["allow_targets"] == nil {
		t.Fatal("missing allow_targets")
	}
	conn := draft["connection"].(map[string]any)
	comp := conn["compression"].(map[string]any)
	if comp["level"] != float64(3) {
		t.Fatalf("draft connection=%#v", conn)
	}
	if _, ok := draft["connection_config"]; ok {
		t.Fatal("connection_config must not appear in editor draft")
	}
}

func TestMergeClientPeersAllowsSendRateBounds(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"peers": []any{
		map[string]any{"peer_id": "peer-a", "quic_peer": "old:443"},
	}}
	merged, err := MergeClientPeers(actual, map[string]any{
		"peers": []any{map[string]any{
			"peer_id": "peer-a",
			"connection": map[string]any{
				"min_send_rate_kbps": float64(1000),
				"max_send_rate_kbps": float64(2000),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	peer := merged["peers"].([]any)[0].(map[string]any)
	conn := peer["connection"].(map[string]any)
	if conn["max_send_rate_kbps"] != float64(2000) {
		t.Fatalf("%#v", conn)
	}
}

func TestTrimForRoleRejectsSecrets(t *testing.T) {
	t.Parallel()
	_, _, err := TrimForRole("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"tls":           map[string]any{"key": "SECRET"},
	})
	if err == nil {
		t.Fatal("expected secret rejection")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/configmerge/ -count=1`  
Expected: FAIL（`TrimForRole` / `EditorDraft` 未定义，或 `MergeClientPeers` 拒绝 rate 字段）

- [ ] **Step 3: Implement**

在 `merge.go`：

1. 扩展 `validatePeer`：`connection` 允许 `encryption`、`compression`、`min_send_rate_kbps`、`max_send_rate_kbps`；数值须为 JSON number 且 ≥ 0。
2. 新增 `MergeServerConfig(actual, body)`，允许 `allow_targets`、`deny_targets`、`connection.compression.level`；`MergeServerACL` 保持对外签名并继续可用于仅 ACL 模板。
3. 实现 `NormalizeClientPeers`：peers 数组做键别名映射。
4. 实现 `TrimForRole`：先 `rejectSecrets`；再按角色提取白名单子树；非白名单路径记入 `ignored`（点分路径，如 `connection.max_send_rate_kbps`、`tls`）。client 顶层只保留归一后的 `peers`。
5. 实现 `EditorDraft`：server 从 live 取 ACL + `connection_config.desired.compression` 写入 `connection.compression`；client 从 live 取 `peers` 并 Normalize。
6. `WritablePaths` 返回稳定字符串列表供 GET 响应。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/configmerge/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/internal/configmerge/merge.go apps/raypx2-center/internal/configmerge/merge_test.go
git commit -m "$(cat <<'EOF'
feat(center): extend configmerge for node config editing

Add role trim/editor draft helpers, peer field aliases, and client
send-rate whitelist fields aligned with Admin writable surfaces.
EOF
)"
```

---

### Task 2: `GET/PUT` node config API + 审计常量

**Files:**
- Create: `apps/raypx2-center/internal/centerapi/config.go`
- Create: `apps/raypx2-center/internal/centerapi/config_test.go`
- Modify: `apps/raypx2-center/internal/audit/audit.go`
- Modify: `apps/raypx2-center/main.go`
- Modify: `apps/raypx2-center/internal/centerapi/auth.go`（`hub` 改为 `ProxyRequester` 接口以便测试）
- Modify: `apps/raypx2-center/internal/centerapi/routes_test.go`（补鉴权用例）

**Interfaces:**
- Consumes: `api.hub.RequestProxy`、`configmerge.TrimForRole|EditorDraft|MergeServerConfig|MergeClientPeers`、`audit.RecordManagement`
- Produces:
  - `(api *API) HandleGetNodeConfig(e *core.RequestEvent) error`
  - `(api *API) HandlePutNodeConfig(e *core.RequestEvent) error`
  - `audit.ActionNodeConfigUpdate = "node.config.update"`

- [ ] **Step 1: Write failing handler tests**

参考 `proxy_test.go`。将 `API.hub` 改为接口：

```go
type ProxyRequester interface {
	RequestProxy(context.Context, string, agenthub.ProxyRequest) (agenthub.ProxyResponse, error)
}
```

Mock hub 最小形状：

```go
type configHub struct {
	online bool
	gets   []agenthub.ProxyResponse
	puts   []agenthub.ProxyRequest
}

func (h *configHub) RequestProxy(ctx context.Context, nodeKey string, req agenthub.ProxyRequest) (agenthub.ProxyResponse, error) {
	if !h.online {
		return agenthub.ProxyResponse{}, agenthub.ErrNodeOffline
	}
	if req.Method == http.MethodGet {
		if len(h.gets) == 0 {
			return agenthub.ProxyResponse{}, errors.New("no get stub")
		}
		resp := h.gets[0]
		h.gets = h.gets[1:]
		return resp, nil
	}
	h.puts = append(h.puts, req)
	body, _ := json.Marshal(map[string]any{"ok": true})
	return agenthub.ProxyResponse{Status: 200, BodyB64: base64.StdEncoding.EncodeToString(body)}, nil
}
```

必测用例：

1. `PUT` offline → `409` + `node_offline`
2. `PUT` server 成功：stub GET 返回 ACL live；body 含 `allow_targets` + 多余 `listen`；断言 hub 收到 `PATCH /api/v1/server/config`；响应含 `ignored_fields`；DB 有 `desired/manual_edit`；audit `node.config.update`
3. `PUT` client：GET 含两个 peers；content 只改 peer-a；断言 PUT body 仍含 peer-b
4. `PUT` 含 `tls.key` → `400 secret_field_forbidden`
5. `PUT` 裁剪后空 → `400 empty_config_update`
6. `GET` online 返回 `live`、`editor_draft`、`writable_paths`
7. `GET` offline：`live` null，仍 200

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/centerapi/ -count=1 -run Config`  
Expected: FAIL（handler 不存在）

- [ ] **Step 3: Implement `config.go`**

PUT 核心顺序：鉴权节点 → role 校验 → online 校验 → bind `content` → `TrimForRole` → empty 检查 → proxy GET actual → `saveConfigRevision(..., "actual", "pull")` → `MergeServerConfig` 或 `MergeClientPeers` → proxy PATCH/PUT → 非 2xx 返回 `admin_rejected`（含 `admin_status`、脱敏 `admin_body`）→ `desired/manual_edit` revision → `audit.RecordManagement(..., ActionNodeConfigUpdate, ...)`。

GET：offline 跳过 proxy；online GET + `EditorDraft`；`recent_revisions` limit 20。

`main.go` 挂载：

```go
center.GET("/nodes/{node_key}/config", centerAPI.HandleGetNodeConfig)
center.PUT("/nodes/{node_key}/config", centerAPI.HandlePutNodeConfig)
```

允许从 `apply/runner.go` **复制**私有 `proxyJSON`/redact 辅助到 `centerapi`，避免本任务大重构。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/centerapi/ ./apps/raypx2-center/internal/configmerge/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/internal/centerapi/config.go \
  apps/raypx2-center/internal/centerapi/config_test.go \
  apps/raypx2-center/internal/centerapi/auth.go \
  apps/raypx2-center/internal/centerapi/routes_test.go \
  apps/raypx2-center/internal/audit/audit.go \
  apps/raypx2-center/main.go
git commit -m "$(cat <<'EOF'
feat(center): add node config GET/PUT API

Expose whitelist-trimmed single-node config reads/writes over the agent
proxy with revisions and management audit events.
EOF
)"
```

---

### Task 3: SPA API 客户端 + Config 双模式编辑器

**Files:**
- Modify: `apps/raypx2-center/ui/src/api.ts`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`
- Modify: `apps/raypx2-center/ui/src/styles.css`（仅当缺表单样式时）

**Interfaces:**
- Consumes: `GET/PUT /api/center/nodes/{node_key}/config`
- Produces:
  - `getNodeConfig(nodeKey): Promise<NodeConfigResponse>`
  - `putNodeConfig(nodeKey, content): Promise<NodeConfigUpdateResult>`
  - `NodeConfig`：表单 | JSON、保存、ignored 提示、revision 表

- [ ] **Step 1: Write failing UI tests**

```tsx
it("loads config editor draft and saves via putNodeConfig", async () => {
  vi.spyOn(api, "getNodeConfig").mockResolvedValue({
    node_key: "n1",
    role: "server",
    online: true,
    live: { allow_targets: ["10.0.0.0/8"], deny_targets: [] },
    editor_draft: { allow_targets: ["10.0.0.0/8"], deny_targets: [] },
    writable_paths: ["allow_targets", "deny_targets"],
    recent_revisions: [],
  });
  const put = vi.spyOn(api, "putNodeConfig").mockResolvedValue({
    applied: { allow_targets: ["127.0.0.0/8"], deny_targets: [] },
    ignored_fields: ["listen"],
    revision_id: "rev1",
    admin_status: 200,
  });
  render(<NodeDetail node={onlineServerNode} onBack={() => {}} />);
  await userEvent.click(screen.getByRole("button", { name: /config/i }));
  await userEvent.click(screen.getByRole("button", { name: /save/i }));
  expect(put).toHaveBeenCalled();
  expect(await screen.findByText(/ignored/i)).toBeInTheDocument();
});

it("blocks switching to form when JSON is invalid", async () => {
  // load config, switch to JSON, type "{", click Form → expect error, remain on JSON
});

it("disables save when offline", async () => {
  // offline node → Save disabled
});
```

- [ ] **Step 2: Run UI tests to verify they fail**

Run: `cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test`  
Expected: FAIL（`getNodeConfig` 不存在或旧 Config 无 Save）

- [ ] **Step 3: Implement API + Config UI**

`api.ts` 增加：

```ts
export interface NodeConfigResponse {
  node_key: string;
  role: string;
  online: boolean;
  live: Record<string, unknown> | null;
  editor_draft: Record<string, unknown>;
  writable_paths: string[];
  recent_revisions: ConfigRevision[];
}

export interface NodeConfigUpdateResult {
  applied: Record<string, unknown>;
  ignored_fields: string[];
  revision_id: string;
  admin_status: number;
}

export function getNodeConfig(nodeKey: string) {
  return centerRequest<NodeConfigResponse>(
    `/api/center/nodes/${encodeURIComponent(nodeKey)}/config`,
  );
}

export function putNodeConfig(nodeKey: string, content: Record<string, unknown>) {
  return centerRequest<NodeConfigUpdateResult>(
    `/api/center/nodes/${encodeURIComponent(nodeKey)}/config`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
    },
  );
}
```

重写 `NodeConfig`：

- 状态：`mode`、`draft`、`jsonText`、`ignored`、`error`、`saving`
- Refresh → `getNodeConfig`；绑定 `editor_draft`
- Server 表单：allow/deny 多行；`connection.compression.level` number
- Client 表单：peers 字段（标识、地址、listens、enabled、rates、port_forwards）；**不提供删除 peer**
- JSON textarea；切回 Form 时 `JSON.parse` 失败则禁止切换
- Save → `putNodeConfig`；展示 `ignored_fields`；刷新 revisions
- offline / 非 client|server：只读

- [ ] **Step 4: Run UI tests**

Run: `cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/api.ts \
  apps/raypx2-center/ui/src/pages/NodeDetail.tsx \
  apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx \
  apps/raypx2-center/ui/src/styles.css
git commit -m "$(cat <<'EOF'
feat(center): add form/JSON node config editor

Wire Config tab to the center config API with dual-mode editing and
ignored-field feedback after save.
EOF
)"
```

---

### Task 4: Ops 移除 ACL；构建 dist；README

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`
- Modify: `apps/raypx2-center/ui/dist/**`（`npm run build`）
- Modify: `apps/raypx2-center/README.md`

**Interfaces:**
- Consumes: Task 3 Config 编辑器
- Produces: Ops 无 ACL；README 说明；更新 embed `ui/dist`

- [ ] **Step 1: Write failing test for Ops ACL removal**

```tsx
it("does not show Server ACL editor on Ops tab", async () => {
  render(<NodeDetail node={onlineServerNode} onBack={() => {}} />);
  expect(screen.queryByRole("button", { name: /save acl/i })).not.toBeInTheDocument();
  expect(screen.queryByLabelText(/allow targets/i)).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- -t "does not show Server ACL"`  
Expected: FAIL（若 ACL 仍在）

- [ ] **Step 3: Remove Ops ACL UI**

删除 `allowTargets` / `denyTargets` / `saveAcl` / Server ACL panel；保留 health 与 connections。更新旧 ACL proxy 断言。

- [ ] **Step 4: Build UI + README**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test && npm run build
```

README 追加：

```markdown
## Node configuration

Open a node → **Config**. Edit via **Form** or **JSON**, then Save.
The center whitelists writable Admin fields, writes the node over the
agent tunnel, and stores a `manual_edit` revision. Offline nodes are
read-only. Peer deletion is not supported on this page.
```

- [ ] **Step 5: Run full相关测试**

```bash
cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/... -count=1
cd apps/raypx2-center/ui && npm test
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add apps/raypx2-center/ui apps/raypx2-center/README.md
git commit -m "$(cat <<'EOF'
feat(center): move ACL editing to Config and ship UI dist

Drop duplicate Server ACL controls from Ops, document the Config
workflow, and refresh the embedded SPA build.
EOF
)"
```

---

### Task 5: 手工冒烟清单

- [ ] **Step 1:** 启动中心与在线节点（沿用既有 e2e 拓扑或本机 client）
- [ ] **Step 2:** Server Config 改 ACL → Save → Admin GET 确认 → revision `manual_edit`
- [ ] **Step 3:** Client 改某一 peer → 其他 peer 仍在
- [ ] **Step 4:** JSON 含 `tls.key` → `400 secret_field_forbidden`
- [ ] **Step 5:** 停 Agent → Save → `409 node_offline`
- [ ] **Step 6:** 若补充了 README 冒烟步骤则单独 docs commit；否则在会话记录结果即可

---

## Spec coverage self-check

| Spec 要求 | Task |
|---|---|
| 中心 GET/PUT config API | Task 2 |
| 白名单裁剪 + ignored_fields | Task 1–2 |
| 密钥拒绝 | Task 1–2 |
| server PATCH / client merge-PUT | Task 2 |
| editor_draft 投影 | Task 1–2 |
| 表单 + JSON 双模式 | Task 3 |
| Ops ACL 迁出 | Task 4 |
| revision + audit | Task 2 |
| 不删 peer | Task 3 |
| README / 冒烟 | Task 4–5 |
| 不改 raypx2 Admin | 全任务；server 白名单对齐现网 PATCH |

## Placeholder scan

无 TBD/TODO。Server `min/max_send_rate_kbps` 按现网 Admin **排除**（符合 spec「若已支持则纳入」）。
