# raypx2 Center Client Peers Field Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Center Client Peers 页的表单与列表字段对齐 raypx2 Admin Console（connection desired/applied/restart + 运维列），Config 页不动。

**Architecture:** 在 `PeersTab` 旁新增纯函数 helpers（读 connection_config、校验 compression level、组装 peer payload）。PeersTab 扩展 Sheet 表单与 client 表格列；仍经现有 `proxyNode` 调用节点 `/api/v1/peers`。无 Center 后端改动。

**Tech Stack:** Vite + React + TypeScript + Vitest + Testing Library；既有 shadcn UI（Input/Label/Switch/Sheet/Table）；Select 用原生 `<select>`（便于测试，语义同 Admin）。

**Spec:** [docs/superpowers/specs/2026-08-12-raypx2-center-client-peers-field-parity-design.md](../specs/2026-08-12-raypx2-center-client-peers-field-parity-design.md)

## Global Constraints

- 只改 `apps/raypx2-center/ui/`（及本 plan/spec 文档链接）；不改 Go 后端、不改 Config tab。
- 保存发 `quic_connections` + `connection`；读取兼容 `proto_connections || quic_connections || connection_count`。
- Desired 来自 `connection_config.desired || peer.connection || defaults`；applied/restart 来自 `connection_config`。
- paths / port_forwards 保持 JSON textarea。
- Server peers 列表列不变。
- SPA 测试入口：`npm test`（在 `apps/raypx2-center/ui/`）。

## File Structure

| Path | Responsibility |
|---|---|
| `ui/src/pages/node-detail/peer-form-helpers.ts` | connection 读/写、校验、defaults、payload 组装 |
| `ui/src/pages/node-detail/peer-form-helpers.test.ts` | helpers 单测 |
| `ui/src/pages/node-detail/PeersTab.tsx` | 表单字段 + client 列表列 + callout |
| `ui/src/pages/NodeDetail.test.tsx` | Peers 集成测试（回填、PUT body、列表列） |
| Spec | 补上 plan 链接 |

---

### Task 1: peer-form helpers

**Files:**
- Create: `apps/raypx2-center/ui/src/pages/node-detail/peer-form-helpers.ts`
- Create: `apps/raypx2-center/ui/src/pages/node-detail/peer-form-helpers.test.ts`
- Modify: `docs/superpowers/specs/2026-08-12-raypx2-center-client-peers-field-parity-design.md`（header 增加 plan 链接）

**Interfaces:**
- Consumes: `JsonObject` from `@/lib/node-utils`（或本地 `Record<string, unknown>`）
- Produces:
  - `DEFAULT_CONNECTION: { encryption: "enabled"; compression: { mode: "disabled"; level: 1 } }`
  - `readPeerConnections(peer: JsonObject): number`
  - `readDesiredConnection(peer: JsonObject): { encryption: string; compression: { mode: string; level: number } }`
  - `readAppliedConnection(peer: JsonObject): { encryption: string; compression: { mode: string; level: number } }`
  - `readRestartRequired(peer: JsonObject): boolean`
  - `validateCompressionLevel(value: string | number): string | null`（null=ok，否则错误文案）
  - `buildPeerSavePayload(form: PeerFormState): JsonObject`
  - `type PeerFormState`（表单状态形状）
  - `emptyPeerForm(): PeerFormState`
  - `peerToForm(peer: JsonObject): PeerFormState`

- [ ] **Step 1: Write failing helper tests**

Create `peer-form-helpers.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  buildPeerSavePayload,
  emptyPeerForm,
  peerToForm,
  readAppliedConnection,
  readDesiredConnection,
  readPeerConnections,
  readRestartRequired,
  validateCompressionLevel,
} from "./peer-form-helpers";

describe("peer-form-helpers", () => {
  it("reads connections preferring proto_connections", () => {
    expect(readPeerConnections({
      proto_connections: 4,
      quic_connections: 2,
      connection_count: 8,
    })).toBe(4);
    expect(readPeerConnections({ quic_connections: 2 })).toBe(2);
    expect(readPeerConnections({})).toBe(1);
  });

  it("reads desired from connection_config.desired then peer.connection", () => {
    expect(readDesiredConnection({
      connection_config: {
        desired: { encryption: "disabled", compression: { mode: "enabled", level: 5 } },
      },
    }).encryption).toBe("disabled");
    expect(readDesiredConnection({
      connection: { encryption: "disabled", compression: { mode: "enabled", level: 3 } },
    }).compression.level).toBe(3);
    expect(readDesiredConnection({}).encryption).toBe("enabled");
  });

  it("reads applied and restart_required from connection_config", () => {
    const peer = {
      connection_config: {
        applied: { encryption: "disabled", compression: { mode: "enabled", level: 7 } },
        restart_required: true,
      },
    };
    expect(readAppliedConnection(peer).compression.level).toBe(7);
    expect(readRestartRequired(peer)).toBe(true);
    expect(readRestartRequired({})).toBe(false);
  });

  it("validates compression level 1-22 integer", () => {
    expect(validateCompressionLevel(1)).toBeNull();
    expect(validateCompressionLevel("22")).toBeNull();
    expect(validateCompressionLevel(0)).toMatch(/1 to 22/i);
    expect(validateCompressionLevel(1.5)).toMatch(/1 to 22/i);
  });

  it("builds save payload with connection object", () => {
    const form = {
      ...emptyPeerForm(),
      peer_id: "peer-a",
      quic_peer: "edge:443",
      connections: "3",
      encryption: "disabled",
      compression_mode: "enabled",
      compression_level: "5",
      paths: '[{"name":"p1"}]',
      port_forwards: "[]",
    };
    expect(buildPeerSavePayload(form)).toEqual({
      peer_id: "peer-a",
      quic_peer: "edge:443",
      quic_connections: 3,
      socks_listen: "127.0.0.1:1080",
      http_listen: "127.0.0.1:8080",
      enabled: true,
      paths: [{ name: "p1" }],
      port_forwards: [],
      connection: {
        encryption: "disabled",
        compression: { mode: "enabled", level: 5 },
      },
    });
  });

  it("maps peer list row into form state including applied readouts", () => {
    const form = peerToForm({
      peer_id: "peer-a",
      proto_connections: 2,
      socks_listen: "127.0.0.1:1081",
      http_listen: "127.0.0.1:8081",
      enabled: false,
      paths: [{ name: "x" }],
      port_forwards: [{ listen: ":9", target: "1:2" }],
      connection_config: {
        desired: { encryption: "disabled", compression: { mode: "enabled", level: 4 } },
        applied: { encryption: "enabled", compression: { mode: "disabled", level: 1 } },
        restart_required: true,
      },
    });
    expect(form.peer_id).toBe("peer-a");
    expect(form.connections).toBe("2");
    expect(form.encryption).toBe("disabled");
    expect(form.compression_mode).toBe("enabled");
    expect(form.compression_level).toBe("4");
    expect(form.applied_encryption).toBe("enabled");
    expect(form.applied_compression_mode).toBe("disabled");
    expect(form.applied_compression_level).toBe("1");
    expect(form.restart_required).toBe(true);
    expect(form.enabled).toBe(false);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/node-detail/peer-form-helpers.test.ts
```

Expected: FAIL（module not found / exports missing）

- [ ] **Step 3: Implement helpers**

Create `peer-form-helpers.ts`:

```ts
import type { JsonObject } from "@/lib/node-utils";

export type PeerConnection = {
  encryption: string;
  compression: { mode: string; level: number };
};

export const DEFAULT_CONNECTION: PeerConnection = {
  encryption: "enabled",
  compression: { mode: "disabled", level: 1 },
};

export type PeerFormState = {
  peer_id: string;
  quic_peer: string;
  connections: string;
  socks_listen: string;
  http_listen: string;
  enabled: boolean;
  encryption: string;
  compression_mode: string;
  compression_level: string;
  applied_encryption: string;
  applied_compression_mode: string;
  applied_compression_level: string;
  restart_required: boolean;
  paths: string;
  port_forwards: string;
};

function asObject(value: unknown): JsonObject | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as JsonObject)
    : undefined;
}

function normalizeConnection(raw: unknown): PeerConnection {
  const obj = asObject(raw) ?? {};
  const compression = asObject(obj.compression) ?? {};
  const level = Number(compression.level);
  return {
    encryption: obj.encryption === "disabled" ? "disabled" : "enabled",
    compression: {
      mode: compression.mode === "enabled" ? "enabled" : "disabled",
      level: Number.isFinite(level) && level > 0 ? level : 1,
    },
  };
}

export function readPeerConnections(peer: JsonObject): number {
  const value = peer.proto_connections ?? peer.quic_connections ?? peer.connection_count ?? 1;
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : 1;
}

export function readDesiredConnection(peer: JsonObject): PeerConnection {
  const config = asObject(peer.connection_config);
  const desired = config ? config.desired : undefined;
  if (desired !== undefined) return normalizeConnection(desired);
  if (peer.connection !== undefined) return normalizeConnection(peer.connection);
  return { ...DEFAULT_CONNECTION, compression: { ...DEFAULT_CONNECTION.compression } };
}

export function readAppliedConnection(peer: JsonObject): PeerConnection {
  const config = asObject(peer.connection_config);
  if (config?.applied !== undefined) return normalizeConnection(config.applied);
  return { ...DEFAULT_CONNECTION, compression: { ...DEFAULT_CONNECTION.compression } };
}

export function readRestartRequired(peer: JsonObject): boolean {
  const config = asObject(peer.connection_config);
  return config?.restart_required === true;
}

export function validateCompressionLevel(value: string | number): string | null {
  const n = typeof value === "number" ? value : Number(value);
  if (!Number.isInteger(n) || n < 1 || n > 22) {
    return "Compression level must be an integer from 1 to 22.";
  }
  return null;
}

export function emptyPeerForm(): PeerFormState {
  return {
    peer_id: "",
    quic_peer: "",
    connections: "1",
    socks_listen: "127.0.0.1:1080",
    http_listen: "127.0.0.1:8080",
    enabled: true,
    encryption: DEFAULT_CONNECTION.encryption,
    compression_mode: DEFAULT_CONNECTION.compression.mode,
    compression_level: String(DEFAULT_CONNECTION.compression.level),
    applied_encryption: DEFAULT_CONNECTION.encryption,
    applied_compression_mode: DEFAULT_CONNECTION.compression.mode,
    applied_compression_level: String(DEFAULT_CONNECTION.compression.level),
    restart_required: false,
    paths: "[]",
    port_forwards: "[]",
  };
}

export function peerToForm(peer: JsonObject): PeerFormState {
  const desired = readDesiredConnection(peer);
  const applied = readAppliedConnection(peer);
  return {
    peer_id: String(peer.peer_id ?? ""),
    quic_peer: String(peer.quic_peer ?? peer.address ?? ""),
    connections: String(readPeerConnections(peer)),
    socks_listen: String(peer.socks_listen ?? "127.0.0.1:1080"),
    http_listen: String(peer.http_listen ?? "127.0.0.1:8080"),
    enabled: peer.enabled !== false && peer.state !== "disabled",
    encryption: desired.encryption,
    compression_mode: desired.compression.mode,
    compression_level: String(desired.compression.level),
    applied_encryption: applied.encryption,
    applied_compression_mode: applied.compression.mode,
    applied_compression_level: String(applied.compression.level),
    restart_required: readRestartRequired(peer),
    paths: JSON.stringify(peer.paths ?? [], null, 2),
    port_forwards: JSON.stringify(peer.port_forwards ?? [], null, 2),
  };
}

export function buildPeerSavePayload(form: PeerFormState): JsonObject {
  const levelError = validateCompressionLevel(form.compression_level);
  if (levelError) throw new Error(levelError);
  return {
    peer_id: form.peer_id.trim(),
    quic_peer: form.quic_peer.trim(),
    quic_connections: Number(form.connections) || 1,
    socks_listen: form.socks_listen.trim(),
    http_listen: form.http_listen.trim(),
    enabled: form.enabled,
    paths: JSON.parse(form.paths || "[]"),
    port_forwards: JSON.parse(form.port_forwards || "[]"),
    connection: {
      encryption: form.encryption === "disabled" ? "disabled" : "enabled",
      compression: {
        mode: form.compression_mode === "enabled" ? "enabled" : "disabled",
        level: Number(form.compression_level),
      },
    },
  };
}
```

Also add to the design spec header (after **状态** line):

```markdown
**实现计划：** [docs/superpowers/plans/2026-08-12-raypx2-center-client-peers-field-parity.md](../plans/2026-08-12-raypx2-center-client-peers-field-parity.md)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/node-detail/peer-form-helpers.test.ts
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/pages/node-detail/peer-form-helpers.ts \
  apps/raypx2-center/ui/src/pages/node-detail/peer-form-helpers.test.ts \
  docs/superpowers/specs/2026-08-12-raypx2-center-client-peers-field-parity-design.md \
  docs/superpowers/plans/2026-08-12-raypx2-center-client-peers-field-parity.md
git commit -m "$(cat <<'EOF'
feat(center): add peer form helpers for connection field parity

Extract connection_config read/write helpers so PeersTab can align
with the raypx2 admin console payload shape.
EOF
)"
```

---

### Task 2: PeersTab form + table + SPA tests

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/node-detail/PeersTab.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`

**Interfaces:**
- Consumes: helpers from Task 1 (`emptyPeerForm`, `peerToForm`, `buildPeerSavePayload`, `validateCompressionLevel`)
- Produces: Client Peers UI with expanded columns and connection fields; no new exported API

- [ ] **Step 1: Write failing SPA tests**

Append to `NodeDetail.test.tsx` inside `describe("NodeDetail", ...)`:

```ts
  it("shows admin-aligned client peer columns", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return {
          peers: [{
            peer_id: "peer-a",
            state: "connected",
            enabled: true,
            quic_peer: "edge:443",
            socks_listen: "127.0.0.1:1080",
            http_listen: "127.0.0.1:8080",
            connection_count: 2,
            connected_connections: 1,
            active_streams: 3,
            total_streams: 9,
            reconnects: 4,
            last_error: "timeout",
          }],
        };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    expect(await screen.findByText("peer-a")).toBeInTheDocument();

    for (const header of [
      "socks_listen",
      "http_listen",
      "connection_count",
      "connected_connections",
      "active_streams",
      "total_streams",
      "reconnects",
      "last_error",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }
    expect(screen.getByText("timeout")).toBeInTheDocument();
    expect(screen.getByText("127.0.0.1:1080")).toBeInTheDocument();
  });

  it("fills connection fields when editing a peer and saves connection payload", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path, body) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return {
          peers: [{
            peer_id: "peer-a",
            state: "connected",
            enabled: true,
            quic_peer: "edge:443",
            socks_listen: "127.0.0.1:1080",
            http_listen: "127.0.0.1:8080",
            proto_connections: 2,
            paths: [],
            port_forwards: [],
            connection_config: {
              desired: { encryption: "disabled", compression: { mode: "enabled", level: 5 } },
              applied: { encryption: "enabled", compression: { mode: "disabled", level: 1 } },
              restart_required: true,
            },
          }],
        };
      }
      if (method === "PUT" && path === "/api/v1/peers/peer-a") {
        return body ?? {};
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    await screen.findByText("peer-a");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(screen.getByLabelText("Desired encryption")).toHaveValue("disabled");
    expect(screen.getByLabelText("Desired compression mode")).toHaveValue("enabled");
    expect(screen.getByLabelText("Desired compression level")).toHaveValue(5);
    expect(screen.getByLabelText("Applied encryption")).toHaveTextContent("enabled");
    expect(screen.getByLabelText("Applied compression mode")).toHaveTextContent("disabled");
    expect(screen.getByLabelText("Applied compression level")).toHaveTextContent("1");
    expect(screen.getByLabelText("Restart required")).toHaveTextContent("true");

    fireEvent.change(screen.getByLabelText("Desired compression level"), { target: { value: "6" } });
    fireEvent.click(screen.getByRole("button", { name: "Create / Save" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PUT",
        "/api/v1/peers/peer-a",
        expect.objectContaining({
          peer_id: "peer-a",
          quic_connections: 2,
          connection: {
            encryption: "disabled",
            compression: { mode: "enabled", level: 6 },
          },
        }),
      );
    });
  });

  it("rejects invalid compression level before calling the peer API", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return { peers: [] };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    fireEvent.click(await screen.findByRole("button", { name: "Create Peer" }));
    fireEvent.change(screen.getByLabelText("peer_id"), { target: { value: "peer-new" } });
    fireEvent.change(screen.getByLabelText("Desired compression level"), { target: { value: "99" } });
    fireEvent.click(screen.getByRole("button", { name: "Create / Save" }));

    expect(await screen.findByText(/Compression level must be an integer from 1 to 22/i)).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalledWith(
      "client-1",
      "POST",
      "/api/v1/peers",
      expect.anything(),
    );
  });
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/NodeDetail.test.tsx
```

Expected: FAIL on missing columns / labels / connection payload

- [ ] **Step 3: Update PeersTab**

In `PeersTab.tsx`:

1. Import helpers; remove local `emptyPeerForm` if duplicated.
2. Replace `openEdit` body with `setForm(peerToForm(peer))`.
3. In `savePeer`, use `buildPeerSavePayload(form)`（catch validation / JSON errors into `setError`）.
4. Extend `Field` to take `htmlFor` and set `Label htmlFor={htmlFor}`; give every input/select/textarea a matching `id`.
5. Client table headers/cells: add the columns from the spec (keep `actions`).
6. Wrap table in existing `DataTableShell`（已有横向滚动）。
7. Sheet form: after enabled switch, add:

```tsx
<Field label="Desired encryption" htmlFor="peer-encryption">
  <select
    id="peer-encryption"
    className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm"
    value={form.encryption}
    onChange={(event) => setForm({ ...form, encryption: event.target.value })}
  >
    <option value="enabled">enabled</option>
    <option value="disabled">disabled</option>
  </select>
</Field>
{/* same pattern for Desired compression mode */}
{/* number input for Desired compression level min=1 max=22 */}
{/* read-only spans with aria-labelledby or Label htmlFor + id on span for Applied* and Restart required */}
```

For read-only fields use:

```tsx
<div className="space-y-2">
  <Label htmlFor="peer-applied-encryption">Applied encryption</Label>
  <p id="peer-applied-encryption" className="text-sm">{form.applied_encryption}</p>
</div>
```

（`getByLabelText` 对关联 label 的元素有效；若 `<p>` 不行则用 `role="status"` 的 `<output>` / 只读 `<Input readOnly>`。）优先用只读 `Input`：

```tsx
<Input id="peer-applied-encryption" value={form.applied_encryption} readOnly />
```

Restart required 显示 `"true"` / `"false"` 字符串以匹配测试。

8. Callout（表单底部、SheetFooter 前）：

```tsx
<p className="px-4 text-sm text-muted-foreground">
  Provide either quic_peer addresses or a paths array. Connection settings on a newly
  created peer apply immediately. Editing connection settings on an existing peer
  updates desired values and requires a client process restart.
</p>
```

9. Ensure existing labels used in tests get `htmlFor`/`id`：`peer_id`、`quic_peer addresses`、`Connections`、`socks_listen`、`http_listen`、`Path settings`、`Port forwards`。

`savePeer` 核心：

```ts
async function savePeer() {
  if (!node.online || node.role !== "client") return;
  setWriting(true);
  setError("");
  try {
    const payload = buildPeerSavePayload(form);
    if (!payload.peer_id) throw new Error("peer_id is required.");
    if (editing) {
      await proxyNode(node.node_key, "PUT", `/api/v1/peers/${encodeURIComponent(String(payload.peer_id))}`, payload);
    } else {
      await proxyNode(node.node_key, "POST", "/api/v1/peers", payload);
    }
    toast.success(editing ? "Peer updated" : "Peer created");
    setEditorOpen(false);
    await load();
  } catch (cause) {
    setError(errorMessage(cause));
  } finally {
    setWriting(false);
  }
}
```

Client table cell mapping（节选）：

```tsx
<TableCell>{display(peer.socks_listen)}</TableCell>
<TableCell>{display(peer.http_listen)}</TableCell>
<TableCell>{display(peer.connection_count)}</TableCell>
<TableCell>{display(peer.connected_connections)}</TableCell>
<TableCell>{display(peer.active_streams)}</TableCell>
<TableCell>{display(peer.total_streams)}</TableCell>
<TableCell>{display(peer.reconnects)}</TableCell>
<TableCell>{display(peer.last_error)}</TableCell>
```

Empty-state `colSpan` 改为 `13`（client）/ `5`（server）。

- [ ] **Step 4: Run SPA tests**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test
```

Expected: PASS（含既有 Config/Peers/delete 用例不回归）

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/pages/node-detail/PeersTab.tsx \
  apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx
git commit -m "$(cat <<'EOF'
feat(center): align client Peers form and table with admin console

Add connection desired/applied fields to the peer editor and expand
the client peers table with ops columns used by raypx2 admin.
EOF
)"
```

---

### Task 3: Build UI artifact (if dist is shipped)

**Files:**
- Modify: `apps/raypx2-center/ui/dist/**`（仅当仓库惯例提交 dist；若 `.gitignore` 忽略则跳过本 task）

- [ ] **Step 1: Check whether dist is tracked**

```bash
cd /home/jack/src/pocketbase && git check-ignore -v apps/raypx2-center/ui/dist/index.html || git ls-files apps/raypx2-center/ui/dist | head
```

若被 ignore 且未跟踪：跳过 Step 2–3，直接结束。

- [ ] **Step 2: Build**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm run build
```

Expected: build succeeds

- [ ] **Step 3: Commit dist if tracked**

```bash
git add apps/raypx2-center/ui/dist
git commit -m "$(cat <<'EOF'
chore(center): rebuild UI after client Peers field parity
EOF
)"
```

---

## Spec coverage checklist

| Spec 要求 | Task |
|---|---|
| 表单 desired encryption/compression/level | Task 2 |
| 表单 applied* / restart_required 只读 | Task 2 |
| 列表运维列 | Task 2 |
| payload 含 `connection` + `quic_connections` | Task 1 + 2 |
| 读字段兼容 proto/quic/connection_count | Task 1 |
| compression level 1–22 校验 | Task 1 + 2 |
| callout 文案 | Task 2 |
| Config 不动 | 无 Config 文件改动 |
| Server peers 不变 | Task 2 仅扩 client 分支 |
| SPA 测试 | Task 1 + 2 |

## Self-review notes

- 无 TBD/TODO 占位。
- Helpers 与 PeersTab / 测试使用同一字段名（`encryption`、`compression_mode`、`compression_level`、`applied_*`、`restart_required`）。
- 不引入 Center 后端变更，符合 spec 非目标。
