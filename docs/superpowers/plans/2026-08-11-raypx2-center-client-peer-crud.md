# raypx2 Center Client Peer CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Config 表单支持手填 peer_id 新增/编辑 client peer；Ops 通过专用 DELETE API 删除 peer（GET→去 peer→PUT，写 revision/audit）。

**Architecture:** `configmerge.RemoveClientPeer` 负责从 actual 去掉指定 peer；`centerapi` 新增 DELETE peers handler，复用 config proxy/revision 路径；SPA Config 增加 Add peer，Ops 增加 Delete。`MergeClientPeers` 保持 upsert-保留。

**Tech Stack:** Go + PocketBase（`apps/raypx2-center`）、既有 `agenthub` proxy、Vite + React + TypeScript + Vitest。

**Spec:** [docs/superpowers/specs/2026-08-11-raypx2-center-client-peer-crud-design.md](../specs/2026-08-11-raypx2-center-client-peer-crud-design.md)

## Global Constraints

- 应用代码只放在 `apps/raypx2-center/`；不修改 PocketBase 核心包行为。
- Config merge **保持** upsert-保留；不经 Config 删除节点上已有 peer。
- 删除仅经 `DELETE /api/center/nodes/{node_key}/peers/{peer_id}`；下发为 Admin `PUT /api/v1/config` 整包。
- desired revision `source=peer_delete`；audit `node.peer.delete`。
- `node_offline`：库内 **409**；hub 竞态 **503**。
- 新增 peer_id 由操作员手填；已存在 peer 的 peer_id 只读。
- SPA 入口 `/app/`；操作员仅为 superuser。

## File Structure

| Path | Responsibility |
|---|---|
| `internal/configmerge/merge.go` | `RemoveClientPeer` |
| `internal/configmerge/merge_test.go` | RemoveClientPeer 单测 |
| `internal/centerapi/peers.go` | `HandleDeleteNodePeer` |
| `internal/centerapi/peers_test.go` | DELETE handler 测试 |
| `internal/audit/audit.go` | `ActionNodePeerDelete` |
| `main.go` | 注册 DELETE 路由 |
| `internal/centerapi/routes_test.go` | 未认证 DELETE 路由 |
| `ui/src/api.ts` | `deleteNodePeer` |
| `ui/src/pages/NodeDetail.tsx` | Config Add peer；Ops Delete |
| `ui/src/pages/NodeDetail.test.tsx` | SPA 测试 |
| `README.md` | 操作说明 |

---

### Task 1: `RemoveClientPeer`

**Files:**
- Modify: `apps/raypx2-center/internal/configmerge/merge.go`
- Modify: `apps/raypx2-center/internal/configmerge/merge_test.go`

- [ ] **Step 1: Write failing tests** for remove success, preserve other peers/fields, missing peer error, empty peer_id
- [ ] **Step 2: Implement `RemoveClientPeer`**
- [ ] **Step 3: Run** `go test ./apps/raypx2-center/internal/configmerge/ -count=1`
- [ ] **Step 4: Commit** `feat(center): add RemoveClientPeer for ops peer delete`

### Task 2: DELETE peer API

**Files:**
- Create: `apps/raypx2-center/internal/centerapi/peers.go`
- Create: `apps/raypx2-center/internal/centerapi/peers_test.go`
- Modify: `apps/raypx2-center/internal/audit/audit.go`
- Modify: `apps/raypx2-center/main.go`
- Modify: `apps/raypx2-center/internal/centerapi/routes_test.go`

- [ ] **Step 1: Write failing handler tests** (success+audit, peer_not_found, unsupported role, 409/503 offline)
- [ ] **Step 2: Implement handler + audit + route**
- [ ] **Step 3: Run** `go test ./apps/raypx2-center/internal/centerapi/ -count=1`
- [ ] **Step 4: Commit** `feat(center): add DELETE node peer API`

### Task 3: Config UI Add peer

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`

- [ ] **Step 1: Write failing SPA tests** (add peer editable peer_id; existing read-only; draft remove; validation)
- [ ] **Step 2: Implement Config form changes + validateConfig**
- [ ] **Step 3: Run** `npm test` in `ui/`
- [ ] **Step 4: Commit** `feat(center): allow adding client peers in Config form`

### Task 4: Ops Delete + README + dist

**Files:**
- Modify: `apps/raypx2-center/ui/src/api.ts`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`
- Modify: `apps/raypx2-center/README.md`
- Modify: `apps/raypx2-center/ui/dist/**`

- [ ] **Step 1: Write failing Ops delete tests**
- [ ] **Step 2: Implement `deleteNodePeer` + Ops Delete button**
- [ ] **Step 3: Update README; `npm test && npm run build`**
- [ ] **Step 4: Commit** `feat(center): delete client peers from Ops and ship UI`
