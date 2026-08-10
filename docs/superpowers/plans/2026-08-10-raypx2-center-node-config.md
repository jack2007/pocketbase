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
- Server Admin PATCH **当前**仅接受：`allow_targets`、`deny_targets`、`connection.compression.level`（以 raypx2 `TqParseServerConfigPatch` 为准）。**`min/max_send_rate_kbps` 必须 trim 进 ignored，不得 PATCH。**
- **Server `PATCH` body = `TrimForRole` 的 patch only**；`MergeServerConfig` 只用于 revision / 内部 desired，不得把 merge 整包发给 Admin。
- **Client `PUT` body = `MergeClientPeers` 后的整包 desired**；peer `connection`（及嵌套 `compression`）必须 deep merge。
- Client peer `connection.min/max_send_rate_kbps` 是 Admin startup-only：Config trim 为 ignored，Apply/template merge 拒绝。
- Client：未提交的既有 peer **保留**；本阶段不经 Config 删除 peer；`{peers:[]}` → `empty_config_update`。
- `node_offline`：库内预检 **409**；hub `ErrNodeOffline` **503**；SPA 两者同等处理。
- PUT 成功响应的 `applied` = 下发后再 GET 的 `editor_draft`（脱敏）。
- 本地 Admin Bearer / enroll 明文永不进 revision 正文或审计全文；`rejectSecrets` 含 `enroll_secret*`。
- SPA 入口 `/app/`；操作员仅为 superuser。
- Client merge 白名单会被 Apply 继承；startup-only rate 不在白名单内。Server 模板仍用 `MergeServerACL` 仅 ACL，除非另开任务。

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

> 2026-08-10 smoke 修订：下方早期 rate 测试示例由全局约束取代；client rate 必须 Trim ignored / Merge reject。

**Files:**
- Modify: `apps/raypx2-center/internal/configmerge/merge.go`
- Modify: `apps/raypx2-center/internal/configmerge/merge_test.go`

**Interfaces:**
- Consumes: 现有 `MergeServerACL`、`MergeClientPeers`、`rejectSecrets`
- Produces:
  - `func TrimForRole(role string, content map[string]any) (patch map[string]any, ignored []string, err error)`
  - `func EditorDraft(role string, live map[string]any) (map[string]any, error)`
  - `func NormalizeClientPeers(content map[string]any) (map[string]any, error)` — `id`→`peer_id`，`proto_peer`→`quic_peer`，`proto_connections`→`quic_connections`
  - peer `connection.min/max_send_rate_kbps` 不进入白名单（Trim ignored；Merge reject）
  - **`MergeClientPeers`：对 `connection` / `compression` deep merge**（部分更新不得整键覆盖）
  - 扩展 server：新增 `MergeServerConfig` 允许 `connection.compression.level`；`MergeServerACL` 保持仅 ACL（Apply 继续用）
  - 扩展 `rejectSecrets`：拒绝 `enroll_secret`、`enroll_secret_file`、以及键路径含 `enroll_secret` 的对象
  - `WritablePaths(role string) []string`
  - `func Redact(value any) any`（导出或包内可测；规则对齐 apply `redact`：token/password/secret/`tls.key` → `[REDACTED]`）

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
	for name, content := range map[string]map[string]any{
		"tls key": {
			"allow_targets": []any{"10.0.0.0/8"},
			"tls":           map[string]any{"key": "SECRET"},
		},
		"enroll_secret": {
			"peers": []any{map[string]any{
				"peer_id":       "peer-a",
				"enroll_secret": "SECRET",
			}},
		},
		"center enroll file": {
			"center": map[string]any{"enroll_secret_file": "/x"},
		},
	} {
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			role := "server"
			if _, ok := content["peers"]; ok {
				role = "client"
			}
			if _, ok := content["center"]; ok {
				role = "client"
				content = map[string]any{
					"peers":  []any{map[string]any{"peer_id": "a"}},
					"center": content["center"],
				}
			}
			_, _, err := TrimForRole(role, content)
			if err == nil {
				t.Fatal("expected secret rejection")
			}
		})
	}
}

func TestMergeClientPeersPartialConnectionPreservesEncryption(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"peers": []any{
		map[string]any{
			"peer_id":  "peer-a",
			"quic_peer": "old:443",
			"connection": map[string]any{
				"encryption": "enabled",
				"compression": map[string]any{"mode": "disabled", "level": float64(3)},
				"max_send_rate_kbps": float64(0),
			},
		},
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
	conn := merged["peers"].([]any)[0].(map[string]any)["connection"].(map[string]any)
	if conn["encryption"] != "enabled" {
		t.Fatalf("encryption wiped: %#v", conn)
	}
	comp := conn["compression"].(map[string]any)
	if comp["mode"] != "disabled" || comp["level"] != float64(3) {
		t.Fatalf("compression wiped: %#v", conn)
	}
	if conn["max_send_rate_kbps"] != float64(2000) || conn["min_send_rate_kbps"] != float64(1000) {
		t.Fatalf("rates missing: %#v", conn)
	}
}

func TestRedactMasksSecrets(t *testing.T) {
	t.Parallel()
	out := Redact(map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"tls":           map[string]any{"key": "SECRET", "ca": "certs/ca.crt"},
		"admin_token":   "SECRET",
	}).(map[string]any)
	if out["admin_token"] != "[REDACTED]" {
		t.Fatalf("%#v", out)
	}
	tls := out["tls"].(map[string]any)
	if tls["key"] != "[REDACTED]" || tls["ca"] != "certs/ca.crt" {
		t.Fatalf("%#v", tls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/configmerge/ -count=1`  
Expected: FAIL（`TrimForRole` / `EditorDraft` / `Redact` 未定义，或 shallow merge / 缺少 enroll 拒绝）

- [ ] **Step 3: Implement**

在 `merge.go`：

1. `validatePeer` 的 `connection` 仅允许运行时可写字段；`min/max_send_rate_kbps` 作为 startup-only 字段拒绝。
2. **Deep merge：** upsert 时若 key 为 `connection`（或嵌套 `compression`），递归合并 map，而非 `peer[key]=value` 整键覆盖。
3. 新增 `MergeServerConfig(actual, body)`，允许 `allow_targets`、`deny_targets`、`connection.compression.level`；`MergeServerACL` 保持仅 ACL，供 Apply 使用。
4. 实现 `NormalizeClientPeers`：peers 数组做键别名映射。
5. 实现 `TrimForRole`：先 `rejectSecrets`；再按角色提取白名单；server 与 client peer 均将 `connection.max/min_send_rate_kbps` 记入 ignored；client 顶层只保留归一后的 `peers`；`{peers:[]}` 视为空 patch。
6. 扩展 `rejectSecrets`：显式拒绝 `enroll_secret`、`enroll_secret_file`；键名归一后含 `enroll_secret` 的也拒绝。
7. 实现 `EditorDraft`：server ACL + desired compression level；client peers Normalize。
8. 实现可导出 `Redact`（或 `configmerge.RedactForStorage`），规则对齐 apply。
9. `WritablePaths` 返回稳定列表。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/configmerge/ -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/internal/configmerge/merge.go apps/raypx2-center/internal/configmerge/merge_test.go
git commit -m "$(cat <<'EOF'
feat(center): extend configmerge for node config editing

Add role trim/editor draft helpers, peer deep-merge, enroll secret
rejection, and redact aligned with Admin writable surfaces.
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

1. `PUT` 库内 offline → `409` + `node_offline`（不调用 hub）
2. `PUT` 库内 online 但 hub 返回 `ErrNodeOffline` → `503` + `node_offline`
3. `PUT` server 成功：stub GET 返回含 `listen` 的 live；body 含 `allow_targets` + 多余 `listen` + `connection.max_send_rate_kbps`；断言 hub **PATCH** path 正确，且 **body 仅含白名单字段**（无 `listen`、无 rate、无整包 startup）；响应含 `ignored_fields`；第二次 GET stub 用于 `applied`；DB 有 `desired/manual_edit`；audit `node.config.update` 的 summary 含 `content_hash` / `ignored_fields` / `admin_status` / `role`，**不含**完整 content
4. `PUT` client：GET 含两个 peers（peer-a 带 encryption）；content 改 peer-a writable field 并夹带 rates；断言 rates ignored、PUT body 仍含 peer-b，且 peer-a `encryption` 仍在
5. `PUT` 含 `tls.key` 或 `enroll_secret` → `400 secret_field_forbidden`（不调用 hub）
6. `PUT` 裁剪后空或 `{peers:[]}` → `400 empty_config_update`
7. `GET` online：`live` 已 redact（`admin_token`/`tls.key` → `[REDACTED]`）；含 `editor_draft`、`writable_paths`
8. `GET` offline：`live` null，`editor_draft` `{}`，仍 200
9. `GET` `role=unknown` online：可有 live，PUT 仍 400 `unsupported_node_role`

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jack/src/pocketbase && go test ./apps/raypx2-center/internal/centerapi/ -count=1 -run Config`  
Expected: FAIL（handler 不存在）

- [ ] **Step 3: Implement `config.go`**

PUT 核心顺序（与设计一致）：

1. 节点 / role / online(409)  
2. bind `content` → `TrimForRole`（内含 rejectSecrets）→ empty 检查  
3. proxy GET actual → `configmerge.Redact` → `saveConfigRevision(..., "actual", "pull")`  
4. server：`desired = MergeServerConfig(actual, patch)`；**`proxy PATCH` body = `patch`**  
   client：`desired = MergeClientPeers(actual, patch)`；**`proxy PUT` body = `desired`**  
5. hub `ErrNodeOffline` → **503** `node_offline`  
6. Admin 非 2xx → `admin_rejected`（`admin_status`、脱敏 `admin_body`）；不写 desired  
7. `saveConfigRevision(..., "desired", "manual_edit")`  
8. 再 GET → `applied = EditorDraft(role, redactedLive)`  
9. `audit.RecordManagement` summary **仅**：`node_key`、`role`、`content_hash`、`ignored_fields`、`admin_status`

GET：offline 跳过 proxy；online GET + Redact(`live`) + `EditorDraft`；`recent_revisions` limit 20。

`main.go` 挂载：

```go
center.GET("/nodes/{node_key}/config", centerAPI.HandleGetNodeConfig)
center.PUT("/nodes/{node_key}/config", centerAPI.HandlePutNodeConfig)
```

允许从 `apply/runner.go` **复制**私有 `proxyJSON` 辅助到 `centerapi`，脱敏优先调用 `configmerge.Redact`。

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
    live: { allow_targets: ["10.0.0.0/8"], deny_targets: [], connection_config: { restart_required: true, pending_fields: ["connection.compression.level"] } },
    editor_draft: { allow_targets: ["10.0.0.0/8"], deny_targets: [] },
    writable_paths: ["allow_targets", "deny_targets", "connection.compression.level"],
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
  expect(await screen.findByText(/restart/i)).toBeInTheDocument();
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

it("treats 503 node_offline like offline on save", async () => {
  vi.spyOn(api, "getNodeConfig").mockResolvedValue({ /* online server draft */ });
  vi.spyOn(api, "putNodeConfig").mockRejectedValue(Object.assign(new Error("node_offline"), { status: 503, data: { code: "node_offline" } }));
  // save → show offline message; draft preserved
});

it("confirms before leaving Config tab when dirty", async () => {
  // load, edit allow textarea, click Ops → expect confirm; cancel stays on Config
});
```

- [ ] **Step 2: Run UI tests to verify they fail**

Run: `cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test`  
Expected: FAIL（`getNodeConfig` 不存在或旧 Config 无 Save）

- [ ] **Step 3: Implement API + Config UI**

`api.ts` 增加（类型同前；确保 `centerRequest` 把 HTTP status / `code` 暴露给调用方以便区分 409/503）：

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

重写 `NodeConfig`（**本任务不要删除 Ops ACL**——留给 Task 4，避免冲突）：

- 状态：`mode`、`draft`、`jsonText`、`baseline`（JSON.stringify 规范化）、`ignored`、`error`、`saving`、`liveMeta`（restart_required 等）
- Refresh → `getNodeConfig`；`draft = editor_draft`；`baseline = stableStringify(draft)`
- dirty：`stableStringify(draft) !== baseline`；切换到 Overview/Ops/返回时若 dirty → `window.confirm`；取消则留在 Config
- Server 表单：allow/deny；`connection.compression.level`；旁路只读 `restart_required` / `pending_fields`
- Client 表单：peers 字段；**不提供删除 peer**
- JSON textarea；切回 Form 时 parse 失败则禁止切换
- Save → `putNodeConfig`；用 **`applied`** 刷新 draft+baseline；展示 ignored；处理 409 **与** 503 `node_offline`
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

Wire Config tab to the center config API with dual-mode editing,
dirty-leave confirm, and ignored-field feedback after save.
EOF
)"
```

---

### Task 4: Ops 移除 ACL；构建 dist；README

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`（**仅** Ops ACL 移除；Config 编辑器已在 Task 3）
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

在具备在线 client/server 的环境执行，并在会话记录结果（通过/失败与现象）。

- [ ] **Step 1:** 启动中心与在线节点（沿用既有 e2e 拓扑或本机 client）
- [ ] **Step 2:** Server Config 改 ACL → Save → Admin GET 确认 → revision `manual_edit`；JSON 夹带 `listen` 时出现 ignored 提示且节点未写入 `listen`
- [ ] **Step 3:** Client 只改某一 peer 的 writable field（如 `socks_listen` 或 `enabled`）→ 其他 peer 与 connection 配置仍在；夹带 rate 时报告 ignored
- [ ] **Step 4:** JSON 含 `tls.key` 或 `enroll_secret` → `400 secret_field_forbidden`
- [ ] **Step 5:** 库内标记 offline 或停 Agent → Save → `409` 或 `503` + `node_offline`
- [ ] **Step 6:** 将上述 5 步结果追加到 `apps/raypx2-center/README.md`「Node configuration」小节下的短清单（或独立 smoke 笔记），并 commit：

```bash
git add apps/raypx2-center/README.md
git commit -m "docs(center): record node config manual smoke results"
```

---

## Spec coverage self-check

| Spec 要求 | Task |
|---|---|
| 中心 GET/PUT config API | Task 2 |
| 白名单裁剪 + ignored_fields | Task 1–2 |
| Server PATCH body = patch only | Task 2 |
| Client PUT = merged desired；connection deep merge | Task 1–2 |
| 密钥拒绝（含 enroll_secret） | Task 1–2 |
| live / revision redact | Task 1–2 |
| `applied` = 再 GET 的 editor_draft | Task 2–3 |
| editor_draft 投影 | Task 1–2 |
| 表单 + JSON 双模式 | Task 3 |
| 脏检查 / 离开确认 | Task 3 |
| 409 与 503 `node_offline` | Task 2–3、5 |
| Ops ACL 迁出 | Task 4 |
| revision + audit 固定 summary 键 | Task 2 |
| 不删 peer；空 peers → empty update | Task 1–3 |
| README / 冒烟记录 | Task 4–5 |
| 不改 raypx2 Admin；server 无速率表单 | 全任务 |

## Placeholder scan

无 TBD/TODO。Server 速率字段按现网 Admin **排除**并写入 ignored。Task 5 Step 6 为固定「记录结果并 commit」动作。
