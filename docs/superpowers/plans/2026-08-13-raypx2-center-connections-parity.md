# raypx2 Center Connections Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Center 的 Client/Server Connections 页具备与 raypx2 Admin Console 一致的行内速率编辑，并删除重复的 Config UI。

**Architecture:** 抽出 `parseRateBounds` / `serverPeerName` 等到纯函数 helpers。`ConnectionsTab` 改为 Admin 对齐的列 + 行内输入 + Apply（Client 另有 `server-send-rate` 与 JSON Detail）。`NodeDetail` 去掉 Config tab；删除 `ConfigTab` 与前端 `getNodeConfig` / `putNodeConfig`。Connections 仍经现有 `proxyNode`。不改 Go 后端。

**Tech Stack:** Vite + React + TypeScript + Vitest + Testing Library；既有 shadcn Table / Input / Sheet / Alert / Button。

**Spec:** [docs/superpowers/specs/2026-08-13-raypx2-center-connections-parity-design.md](../specs/2026-08-13-raypx2-center-connections-parity-design.md)

## Global Constraints

- 只改 `apps/raypx2-center/ui/`（及本 plan/spec 文档链接）；不改 Go 后端。
- Client Apply client rate：`PATCH /api/v1/peers/{peer_id}/connections/{connection_id}`。
- Client Apply server rate：`PATCH /api/v1/peers/{peer_id}/connections/{connection_id}/server-send-rate`。
- Server Apply：`PATCH /api/v1/server/connections/{connection_id}`。
- PATCH body 均为 `{ min_send_rate_kbps, max_send_rate_kbps }`。
- Client server 速率输入绑定 `effective_server_tx_min_kbps` / `effective_server_tx_max_kbps`。
- 校验文案必须与 spec 一致（见 Task 1 常量）。
- 删除 Config UI；保留 Go `GET/PUT /api/center/nodes/{node_key}/config`。
- 不把 Server compression 迁到 ACL；Server Connections 无 Detail。
- SPA 测试入口：`npm test`（在 `apps/raypx2-center/ui/`）。

## File Structure

| Path | Responsibility |
|---|---|
| `ui/src/pages/node-detail/connection-form-helpers.ts` | 速率校验、server peer 名、streams 读取、行 key |
| `ui/src/pages/node-detail/connection-form-helpers.test.ts` | helpers 单测 |
| `ui/src/pages/node-detail/ConnectionsTab.tsx` | Client/Server 列表、行内 Apply、Client Detail |
| `ui/src/pages/node-detail/NodeDetail.tsx` | 去掉 Config tab 与 dirty 确认 |
| `ui/src/pages/NodeDetail.test.tsx` | Connections 集成测试；删除 Config 用例 |
| `ui/src/api.ts` | 删除 `getNodeConfig` / `putNodeConfig` / `listConfigRevisions` 及相关类型 |
| 删除 `ConfigTab.tsx`、`config-helpers.ts` | Config UI |
| Spec | header 增加 plan 链接 |

---

### Task 1: connection-form helpers

**Files:**
- Create: `apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.ts`
- Create: `apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.test.ts`
- Modify: `docs/superpowers/specs/2026-08-13-raypx2-center-connections-parity-design.md`（header 在 **状态** 行后增加：`**实现计划：** [docs/superpowers/plans/2026-08-13-raypx2-center-connections-parity.md](../plans/2026-08-13-raypx2-center-connections-parity.md)`）

**Interfaces:**
- Consumes: `JsonObject` from `@/lib/node-utils`；`itemId` from `@/lib/node-utils`
- Produces:
  - `RATE_INTEGER_ERROR = "Minimum and maximum send rates must be safe non-negative integers."`
  - `RATE_BOUNDS_ERROR = "Minimum send rate must not exceed maximum send rate when both are non-zero."`
  - `type RateBounds = { min_send_rate_kbps: number; max_send_rate_kbps: number }`
  - `parseRateBounds(minText: string, maxText: string): RateBounds`（失败 throw `Error`）
  - `rateField(value: unknown): string`（有限数字或非空字符串则 `String(value)`，否则 `"0"`）
  - `connectionRowKey(connection: JsonObject): string`（有 `peer_id` 字符串则为 `` `${peer_id}:${itemId}` ``，否则 `itemId`）
  - `serverPeerName(connection: JsonObject): string`
  - `readTotalStreamsOpened(connection: JsonObject): unknown`

- [ ] **Step 1: Write failing helper tests**

Create `apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  RATE_BOUNDS_ERROR,
  RATE_INTEGER_ERROR,
  connectionRowKey,
  parseRateBounds,
  rateField,
  readTotalStreamsOpened,
  serverPeerName,
} from "./connection-form-helpers";

describe("connection-form-helpers", () => {
  it("parses non-negative integer bounds", () => {
    expect(parseRateBounds("0", "0")).toEqual({
      min_send_rate_kbps: 0,
      max_send_rate_kbps: 0,
    });
    expect(parseRateBounds(" 100 ", "200")).toEqual({
      min_send_rate_kbps: 100,
      max_send_rate_kbps: 200,
    });
  });

  it("rejects non-integers", () => {
    expect(() => parseRateBounds("1.5", "2")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("-1", "2")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("abc", "1")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("1", "")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("9007199254740993", "1")).toThrow(RATE_INTEGER_ERROR);
  });

  it("rejects min greater than max when both are non-zero", () => {
    expect(() => parseRateBounds("20", "10")).toThrow(RATE_BOUNDS_ERROR);
  });

  it("allows min greater than max when either bound is zero", () => {
    expect(parseRateBounds("20", "0")).toEqual({
      min_send_rate_kbps: 20,
      max_send_rate_kbps: 0,
    });
    expect(parseRateBounds("0", "10")).toEqual({
      min_send_rate_kbps: 0,
      max_send_rate_kbps: 10,
    });
  });

  it("stringifies rate fields with a zero default", () => {
    expect(rateField(12)).toBe("12");
    expect(rateField("8")).toBe("8");
    expect(rateField(undefined)).toBe("0");
    expect(rateField("")).toBe("0");
  });

  it("builds a row key from peer_id and connection_id", () => {
    expect(connectionRowKey({ peer_id: "peer-a", connection_id: "conn-0" })).toBe("peer-a:conn-0");
    expect(connectionRowKey({ connection_id: "sc1" })).toBe("sc1");
  });

  it("names a server peer from client_name or remote_address", () => {
    expect(serverPeerName({ client_name: "edge-1", remote_address: "10.0.0.1:443" })).toBe("edge-1");
    expect(serverPeerName({ remote_address: "10.0.0.1:443" })).toBe("peer-10.0.0.1:443");
    expect(serverPeerName({})).toBe("peer-unknown");
    expect(serverPeerName({ client_name: "  ", remote_address: "  " })).toBe("peer-unknown");
  });

  it("reads total_streams_opened preferring the explicit field", () => {
    expect(readTotalStreamsOpened({ total_streams_opened: 9, total_streams: 3 })).toBe(9);
    expect(readTotalStreamsOpened({ total_streams: 3 })).toBe(3);
    expect(readTotalStreamsOpened({})).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/node-detail/connection-form-helpers.test.ts
```

Expected: FAIL（模块不存在或导出缺失）

- [ ] **Step 3: Implement helpers**

Create `apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.ts`:

```ts
import { itemId, type JsonObject } from "@/lib/node-utils";

export const RATE_INTEGER_ERROR =
  "Minimum and maximum send rates must be safe non-negative integers.";
export const RATE_BOUNDS_ERROR =
  "Minimum send rate must not exceed maximum send rate when both are non-zero.";

export type RateBounds = {
  min_send_rate_kbps: number;
  max_send_rate_kbps: number;
};

export function parseRateBounds(minText: string, maxText: string): RateBounds {
  const minValue = String(minText).trim();
  const maxValue = String(maxText).trim();
  if (!/^\d+$/.test(minValue) || !/^\d+$/.test(maxValue)) {
    throw new Error(RATE_INTEGER_ERROR);
  }
  const minRate = Number(minValue);
  const maxRate = Number(maxValue);
  if (!Number.isSafeInteger(minRate) || minRate < 0
    || !Number.isSafeInteger(maxRate) || maxRate < 0) {
    throw new Error(RATE_INTEGER_ERROR);
  }
  if (minRate !== 0 && maxRate !== 0 && minRate > maxRate) {
    throw new Error(RATE_BOUNDS_ERROR);
  }
  return { min_send_rate_kbps: minRate, max_send_rate_kbps: maxRate };
}

export function rateField(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  if (typeof value === "string" && value !== "") return value;
  return "0";
}

export function connectionRowKey(connection: JsonObject): string {
  const id = itemId(connection);
  const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
  return peerId ? `${peerId}:${id}` : id;
}

export function serverPeerName(connection: JsonObject): string {
  const clientName = typeof connection.client_name === "string" ? connection.client_name.trim() : "";
  if (clientName) return clientName;
  const remote = typeof connection.remote_address === "string" ? connection.remote_address.trim() : "";
  return `peer-${remote || "unknown"}`;
}

export function readTotalStreamsOpened(connection: JsonObject): unknown {
  return connection.total_streams_opened ?? connection.total_streams;
}
```

In the spec header, immediately after the `**状态：** 已批准` line, insert:

```markdown
**实现计划：** [docs/superpowers/plans/2026-08-13-raypx2-center-connections-parity.md](../plans/2026-08-13-raypx2-center-connections-parity.md)
```

- [ ] **Step 4: Run helper tests**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/node-detail/connection-form-helpers.test.ts
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.ts \
  apps/raypx2-center/ui/src/pages/node-detail/connection-form-helpers.test.ts \
  docs/superpowers/specs/2026-08-13-raypx2-center-connections-parity-design.md \
  docs/superpowers/plans/2026-08-13-raypx2-center-connections-parity.md
git commit -m "$(cat <<'EOF'
feat(center): add connection rate helpers for admin console parity

Extract send-rate parsing and server peer naming so Connections can
apply the same bounds rules as the raypx2 admin console.
EOF
)"
```

---

### Task 2: Connections tab inline editors

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/node-detail/ConnectionsTab.tsx`（整文件替换）
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`（新增 Connections 用例；保留现有 Config 用例到 Task 3）

**Interfaces:**
- Consumes: Task 1 的 `parseRateBounds`、`rateField`、`connectionRowKey`、`serverPeerName`、`readTotalStreamsOpened`、`RateBounds`
- Consumes: `proxyNode(nodeKey, method, path, body?)`；`itemId` / `itemsFrom` / `display` / `errorMessage`
- Produces: `ConnectionsTab` 行内 Apply client rate / Apply server rate / Apply；Client Detail GET JSON Sheet

- [ ] **Step 1: Write failing NodeDetail connection tests**

In `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`, **add** the following tests inside `describe("NodeDetail")`（放在现有 `"loads server connections through the node proxy"` 用例之后）。不要删除 Config 用例。

```tsx
  it("shows admin-aligned client connection columns and applies both rate patches", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a" }] };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections") {
        return {
          connections: [{
            connection_id: "conn-0",
            slot_index: 0,
            generation: 1,
            connected: true,
            retry_scheduled: false,
            state: "ready",
            encryption: "disabled",
            compression_mode: "disabled",
            compression_level: 1,
            path: "default",
            local: "127.0.0.1:1",
            peer: "10.0.0.2:4433",
            active_tunnels: 0,
            last_error: "",
            min_send_rate_kbps: 0,
            max_send_rate_kbps: 0,
            effective_server_tx_min_kbps: 0,
            effective_server_tx_max_kbps: 0,
          }],
        };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections/conn-0") {
        return { connection_id: "conn-0", state: "ready" };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    expect(await screen.findByText("conn-0")).toBeInTheDocument();

    for (const header of [
      "slot_index",
      "generation",
      "connected",
      "retry_scheduled",
      "encryption",
      "compression_mode",
      "compression_level",
      "client_min_send_rate_kbps",
      "server_min_send_rate_kbps",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }

    fireEvent.change(screen.getByLabelText("client_min_send_rate_kbps"), { target: { value: "100" } });
    fireEvent.change(screen.getByLabelText("client_max_send_rate_kbps"), { target: { value: "200" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply client rate" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PATCH",
        "/api/v1/peers/peer-a/connections/conn-0",
        { min_send_rate_kbps: 100, max_send_rate_kbps: 200 },
      );
    });

    fireEvent.change(screen.getByLabelText("server_min_send_rate_kbps"), { target: { value: "30" } });
    fireEvent.change(screen.getByLabelText("server_max_send_rate_kbps"), { target: { value: "40" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply server rate" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PATCH",
        "/api/v1/peers/peer-a/connections/conn-0/server-send-rate",
        { min_send_rate_kbps: 30, max_send_rate_kbps: 40 },
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Detail" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "GET",
        "/api/v1/peers/peer-a/connections/conn-0",
      );
    });
  });

  it("does not patch client rates when bounds are invalid", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a" }] };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections") {
        return {
          connections: [{
            connection_id: "conn-0",
            min_send_rate_kbps: 0,
            max_send_rate_kbps: 0,
          }],
        };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    await screen.findByLabelText("client_min_send_rate_kbps");
    fireEvent.change(screen.getByLabelText("client_min_send_rate_kbps"), { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply client rate" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/safe non-negative integers/i);
    expect(vi.mocked(api.proxyNode).mock.calls.filter((call) => call[1] === "PATCH")).toHaveLength(0);
  });

  it("shows admin-aligned server connection columns and applies send rates", async () => {
    vi.mocked(api.proxyNode).mockResolvedValue({
      connections: [{
        connection_id: "sc1",
        client_name: "",
        remote_address: "192.168.1.9:443",
        state: "ready",
        encryption: "enabled",
        active_streams: 2,
        total_streams: 8,
        active_tunnels: 1,
        last_error: "",
        min_send_rate_kbps: 0,
        max_send_rate_kbps: 0,
      }],
    });
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));

    expect(await screen.findByText("sc1")).toBeInTheDocument();
    expect(screen.getByText("peer-192.168.1.9:443")).toBeInTheDocument();
    expect(screen.getByText("8")).toBeInTheDocument();
    for (const header of [
      "remote_address",
      "encryption",
      "active_streams",
      "total_streams_opened",
      "active_tunnels",
      "min_send_rate_kbps",
      "max_send_rate_kbps",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }
    expect(screen.queryByRole("button", { name: "Detail" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("min_send_rate_kbps"), { target: { value: "50" } });
    fireEvent.change(screen.getByLabelText("max_send_rate_kbps"), { target: { value: "80" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        onlineServer.node_key,
        "PATCH",
        "/api/v1/server/connections/sc1",
        { min_send_rate_kbps: 50, max_send_rate_kbps: 80 },
      );
    });
  });

  it("disables connection writes while the node is offline", async () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    expect(screen.getByText("Writes are disabled while this node is offline.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Apply" })).not.toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });
```

- [ ] **Step 2: Run the new tests to verify they fail**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/NodeDetail.test.tsx
```

Expected: FAIL（缺少新列、Apply client rate / Apply server rate、或 server-send-rate PATCH）

- [ ] **Step 3: Replace ConnectionsTab**

Overwrite `apps/raypx2-center/ui/src/pages/node-detail/ConnectionsTab.tsx` with:

```tsx
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { proxyNode } from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { JsonView } from "@/components/shared/JsonView";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { display, errorMessage, itemId, itemsFrom, type JsonObject } from "@/lib/node-utils";
import {
  connectionRowKey,
  parseRateBounds,
  rateField,
  readTotalStreamsOpened,
  serverPeerName,
  type RateBounds,
} from "./connection-form-helpers";

const CLIENT_COLUMNS = [
  "connection_id",
  "peer_id",
  "slot_index",
  "generation",
  "connected",
  "retry_scheduled",
  "state",
  "encryption",
  "compression_mode",
  "compression_level",
  "path",
  "local",
  "peer",
  "active_tunnels",
  "last_error",
] as const;

const SERVER_COLUMNS = [
  "connection_id",
  "peer",
  "remote_address",
  "state",
  "encryption",
  "active_streams",
  "total_streams_opened",
  "active_tunnels",
  "last_error",
] as const;

type RateDraft = {
  min: string;
  max: string;
  serverMin: string;
  serverMax: string;
};

function draftFrom(connection: JsonObject): RateDraft {
  return {
    min: rateField(connection.min_send_rate_kbps),
    max: rateField(connection.max_send_rate_kbps),
    serverMin: rateField(connection.effective_server_tx_min_kbps),
    serverMax: rateField(connection.effective_server_tx_max_kbps),
  };
}

export function ConnectionsTab({ node }: { node: CenterNode }) {
  const [connections, setConnections] = useState<JsonObject[]>([]);
  const [drafts, setDrafts] = useState<Record<string, RateDraft>>({});
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState("");
  const [detail, setDetail] = useState<JsonObject>();

  const isClient = node.role === "client";
  const isServer = node.role === "server";

  async function load() {
    setLoading(true);
    setError("");
    try {
      if (!node.online) {
        setConnections([]);
        setDrafts({});
        return;
      }
      let rows: JsonObject[] = [];
      if (isServer) {
        const result = await proxyNode(node.node_key, "GET", "/api/v1/server/connections");
        rows = itemsFrom(result, "connections");
      } else if (isClient) {
        const peersResult = await proxyNode(node.node_key, "GET", "/api/v1/peers");
        const peers = itemsFrom(peersResult, "peers");
        const groups = await Promise.all(peers.map(async (peer) => {
          const peerId = itemId(peer);
          if (!peerId) return [];
          const result = await proxyNode(
            node.node_key,
            "GET",
            `/api/v1/peers/${encodeURIComponent(peerId)}/connections`,
          );
          return itemsFrom(result, "connections").map((connection) => ({ ...connection, peer_id: peerId }));
        }));
        rows = groups.flat();
      }
      setConnections(rows);
      setDrafts(Object.fromEntries(rows.map((row) => [connectionRowKey(row), draftFrom(row)])));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online, node.role]);

  function updateDraft(key: string, patch: Partial<RateDraft>) {
    setDrafts((current) => ({
      ...current,
      [key]: { ...(current[key] ?? draftFrom({})), ...patch },
    }));
  }

  async function applyBounds(connection: JsonObject, path: string, bounds: RateBounds) {
    setWriting(true);
    setError("");
    try {
      await proxyNode(node.node_key, "PATCH", path, bounds);
      toast.success("Send rates updated");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setWriting(false);
    }
  }

  function applyClientRate(connection: JsonObject) {
    const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
    const connectionId = itemId(connection);
    if (!peerId || !connectionId) return;
    const draft = drafts[connectionRowKey(connection)] ?? draftFrom(connection);
    try {
      const bounds = parseRateBounds(draft.min, draft.max);
      void applyBounds(
        connection,
        `/api/v1/peers/${encodeURIComponent(peerId)}/connections/${encodeURIComponent(connectionId)}`,
        bounds,
      );
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  function applyClientServerRate(connection: JsonObject) {
    const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
    const connectionId = itemId(connection);
    if (!peerId || !connectionId) return;
    const draft = drafts[connectionRowKey(connection)] ?? draftFrom(connection);
    try {
      const bounds = parseRateBounds(draft.serverMin, draft.serverMax);
      void applyBounds(
        connection,
        `/api/v1/peers/${encodeURIComponent(peerId)}/connections/${encodeURIComponent(connectionId)}/server-send-rate`,
        bounds,
      );
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  function applyServerRate(connection: JsonObject) {
    const connectionId = itemId(connection);
    if (!connectionId) return;
    const draft = drafts[connectionRowKey(connection)] ?? draftFrom(connection);
    try {
      const bounds = parseRateBounds(draft.min, draft.max);
      void applyBounds(
        connection,
        `/api/v1/server/connections/${encodeURIComponent(connectionId)}`,
        bounds,
      );
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  async function openDetail(connection: JsonObject) {
    const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
    const connectionId = itemId(connection);
    if (!peerId || !connectionId || !node.online) return;
    setWriting(true);
    setError("");
    try {
      const result = await proxyNode<JsonObject>(
        node.node_key,
        "GET",
        `/api/v1/peers/${encodeURIComponent(peerId)}/connections/${encodeURIComponent(connectionId)}`,
      );
      setDetail(result);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setWriting(false);
    }
  }

  if (!isClient && !isServer) {
    return <p className="text-sm text-muted-foreground">Connections are not available for this node role.</p>;
  }

  const columns = isClient ? CLIENT_COLUMNS : SERVER_COLUMNS;
  const emptyCols = isClient ? 20 : 12;

  return (
    <div className="space-y-4">
      {!node.online && (
        <Alert>
          <AlertDescription>Writes are disabled while this node is offline.</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Connections</CardTitle>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="px-0 pb-0">
          <DataTableShell>
            <Table>
              <TableHeader>
                <TableRow>
                  {columns.map((column) => (
                    <TableHead key={column}>{column}</TableHead>
                  ))}
                  {isClient ? (
                    <>
                      <TableHead>client_min_send_rate_kbps</TableHead>
                      <TableHead>client_max_send_rate_kbps</TableHead>
                      <TableHead>server_min_send_rate_kbps</TableHead>
                      <TableHead>server_max_send_rate_kbps</TableHead>
                    </>
                  ) : (
                    <>
                      <TableHead>min_send_rate_kbps</TableHead>
                      <TableHead>max_send_rate_kbps</TableHead>
                    </>
                  )}
                  <TableHead>actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {connections.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={emptyCols} className="h-20 text-center text-muted-foreground">
                      {loading ? "Loading…" : "No items returned."}
                    </TableCell>
                  </TableRow>
                )}
                {connections.map((connection, index) => {
                  const key = connectionRowKey(connection) || String(index);
                  const draft = drafts[key] ?? draftFrom(connection);
                  const connectionId = itemId(connection);
                  const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
                  const canWrite = node.online && !writing && Boolean(connectionId) && (!isClient || Boolean(peerId));
                  return (
                    <TableRow key={key}>
                      {columns.map((column) => {
                        let value: unknown = connection[column];
                        if (column === "peer" && isServer) value = serverPeerName(connection);
                        if (column === "total_streams_opened") value = readTotalStreamsOpened(connection);
                        if (column === "connection_id") value = connectionId || `Connection ${index + 1}`;
                        return <TableCell key={column} className={column === "connection_id" ? "font-medium" : undefined}>{display(value)}</TableCell>;
                      })}
                      <TableCell>
                        <Input
                          aria-label={isClient ? "client_min_send_rate_kbps" : "min_send_rate_kbps"}
                          value={draft.min}
                          disabled={!node.online || writing}
                          onChange={(event) => updateDraft(key, { min: event.target.value })}
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          aria-label={isClient ? "client_max_send_rate_kbps" : "max_send_rate_kbps"}
                          value={draft.max}
                          disabled={!node.online || writing}
                          onChange={(event) => updateDraft(key, { max: event.target.value })}
                        />
                      </TableCell>
                      {isClient && (
                        <>
                          <TableCell>
                            <Input
                              aria-label="server_min_send_rate_kbps"
                              value={draft.serverMin}
                              disabled={!node.online || writing}
                              onChange={(event) => updateDraft(key, { serverMin: event.target.value })}
                            />
                          </TableCell>
                          <TableCell>
                            <Input
                              aria-label="server_max_send_rate_kbps"
                              value={draft.serverMax}
                              disabled={!node.online || writing}
                              onChange={(event) => updateDraft(key, { serverMax: event.target.value })}
                            />
                          </TableCell>
                        </>
                      )}
                      <TableCell className="space-x-2 whitespace-nowrap">
                        {isClient && (
                          <Button variant="outline" size="sm" disabled={!canWrite} onClick={() => void openDetail(connection)}>
                            Detail
                          </Button>
                        )}
                        {isClient ? (
                          <>
                            <Button variant="outline" size="sm" disabled={!canWrite} onClick={() => applyClientRate(connection)}>
                              Apply client rate
                            </Button>
                            <Button variant="outline" size="sm" disabled={!canWrite} onClick={() => applyClientServerRate(connection)}>
                              Apply server rate
                            </Button>
                          </>
                        ) : (
                          <Button variant="outline" size="sm" disabled={!canWrite} onClick={() => applyServerRate(connection)}>
                            Apply
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </DataTableShell>
        </CardContent>
      </Card>

      <Sheet open={Boolean(detail)} onOpenChange={(open) => !open && setDetail(undefined)}>
        <SheetContent className="overflow-y-auto sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>Connection detail</SheetTitle>
          </SheetHeader>
          <div className="px-4">
            <JsonView value={detail} />
          </div>
          <SheetFooter>
            <Button variant="outline" onClick={() => setDetail(undefined)}>Close</Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
```

- [ ] **Step 4: Run SPA tests**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test
```

Expected: PASS（含既有 Config/Peers 用例；新 Connections 用例通过）

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/pages/node-detail/ConnectionsTab.tsx \
  apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx
git commit -m "$(cat <<'EOF'
feat(center): align Connections tab with admin console rate editors

Add inline client/server send-rate apply actions and admin-aligned
columns so operators can edit pacing without the Config tab.
EOF
)"
```

---

### Task 3: Remove Config UI

**Files:**
- Delete: `apps/raypx2-center/ui/src/pages/node-detail/ConfigTab.tsx`
- Delete: `apps/raypx2-center/ui/src/pages/node-detail/config-helpers.ts`
- Modify: `apps/raypx2-center/ui/src/pages/node-detail/NodeDetail.tsx`（整文件替换）
- Modify: `apps/raypx2-center/ui/src/api.ts`（删除 Config 前端 API 与类型）
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`（删除 Config 用例、改 HTTP 错误测试、断言无 Config tab）

**Interfaces:**
- Consumes: 无 Config UI 符号
- Produces: NodeDetail tabs 为 client `overview peers connections tunnels audit`；server `overview peers connections tunnels acl audit`；unknown `overview audit`

- [ ] **Step 1: Write failing tests for Config removal**

In `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`:

1. 从 `vi.mock("../api")` 去掉 `getNodeConfig` 与 `putNodeConfig`。
2. 删除 `serverConfig` 常量。
3. `beforeEach` 只保留 `vi.clearAllMocks()`（不要再 `mockResolvedValue` `getNodeConfig`）。
4. **删除** 下列用例整段：
   - `"keeps applied config and shows refresh warning when post-save GET fails"`
   - `"re-fetches config metadata and revisions after saving"`
   - `"blocks switching to Form when JSON is invalid"`
   - `"disables config save when offline"`
   - `"constrains server compression level and blocks invalid saves"`
   - `"edits client port forwards and QUIC connections while keeping peer ID read-only"`
   - `"adds a draft peer with editable peer ID and can remove it before save"`
   - `"rejects saving a draft peer without peer ID"`
   - `"removes only draft peers from the Config form"`
   - `"treats a 503 node_offline save response as offline and preserves the draft"`
   - `"confirms before leaving Config when the draft is dirty"`
   - `"does not confirm when clicking the active Config tab"`
5. 在 `describe("NodeDetail")` 内新增：

```tsx
  it("does not show a Config tab for client, server, or unknown nodes", () => {
    const { unmount } = render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    unmount();

    const client: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    const clientView = render(<NodeDetail node={client} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    clientView.unmount();

    const unknown: CenterNode = {
      id: "node-u1",
      node_key: "unknown-1",
      name: "Unknown node",
      role: "unknown",
      online: true,
    };
    render(<NodeDetail node={unknown} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Audit" })).toBeInTheDocument();
  });
```

6. 将文件末尾 `describe("center config API")` **整段替换为**：

```tsx
describe("center proxy API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("surfaces HTTP status and error code", async () => {
    const actual = await vi.importActual<typeof import("../api")>("../api");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "node_offline", message: "node_offline" }),
      { status: 503, headers: { "Content-Type": "application/json" } },
    )));

    await expect(actual.proxyNode("node/one", "GET", "/api/v1/server/connections")).rejects.toMatchObject({
      status: 503,
      code: "node_offline",
      data: { code: "node_offline" },
    });
  });
});
```

- [ ] **Step 2: Run tests to verify Config assertions fail**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- src/pages/NodeDetail.test.tsx
```

Expected: FAIL（Client/Server 仍渲染 Config tab，或 `getNodeConfig` mock 已删导致残留用例崩溃）。若你已先删完 Config 用例，则失败点应是 `"does not show a Config tab..."` 仍能找到 Config tab。

- [ ] **Step 3: Remove Config UI and frontend config API**

Overwrite `apps/raypx2-center/ui/src/pages/node-detail/NodeDetail.tsx` with:

```tsx
import { useState } from "react";
import { ArrowLeft } from "lucide-react";
import type { CenterNode } from "../Nodes";
import { PageHeader } from "@/components/shared/PageHeader";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AclTab } from "./AclTab";
import { AuditTab } from "./AuditTab";
import { ConnectionsTab } from "./ConnectionsTab";
import { OverviewTab } from "./OverviewTab";
import { PeersTab } from "./PeersTab";
import { TunnelsTab } from "./TunnelsTab";

type Tab =
  | "overview"
  | "peers"
  | "connections"
  | "tunnels"
  | "acl"
  | "audit";

interface NodeDetailProps {
  node: CenterNode;
  onBack: () => void;
}

export function NodeDetail({ node, onBack }: NodeDetailProps) {
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: Tab[] = node.role === "server"
    ? ["overview", "peers", "connections", "tunnels", "acl", "audit"]
    : node.role === "client"
      ? ["overview", "peers", "connections", "tunnels", "audit"]
      : ["overview", "audit"];

  function title(item: Tab) {
    return item === "acl" ? "ACL" : item.charAt(0).toUpperCase() + item.slice(1);
  }

  return (
    <section>
      <Button
        variant="ghost"
        size="sm"
        className="mb-3 -ml-2 text-muted-foreground"
        onClick={onBack}
      >
        <ArrowLeft className="size-4" />
        Back to nodes
      </Button>
      <PageHeader
        eyebrow="Node detail"
        title={node.name || "Unnamed node"}
        description={`${node.node_key} · ${node.role}`}
        actions={<StatusBadge online={node.online} />}
      />

      <div className="mb-4 flex flex-wrap gap-1 border-b" role="tablist" aria-label="Node detail">
        {tabs.map((item) => (
          <button
            key={item}
            type="button"
            role="tab"
            aria-selected={tab === item}
            className={cn(
              "border-b-2 px-3 py-2 text-sm font-semibold transition-colors",
              tab === item
                ? "border-foreground text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
            onClick={() => {
              if (item !== tab) setTab(item);
            }}
          >
            {title(item)}
          </button>
        ))}
      </div>

      {tab === "overview" && <OverviewTab node={node} />}
      {tab === "peers" && <PeersTab node={node} />}
      {tab === "connections" && <ConnectionsTab node={node} />}
      {tab === "tunnels" && <TunnelsTab node={node} />}
      {tab === "acl" && <AclTab node={node} />}
      {tab === "audit" && <AuditTab node={node} />}
    </section>
  );
}
```

Delete these files:

```bash
rm apps/raypx2-center/ui/src/pages/node-detail/ConfigTab.tsx \
  apps/raypx2-center/ui/src/pages/node-detail/config-helpers.ts
```

In `apps/raypx2-center/ui/src/api.ts`, delete the following blocks entirely（不要留下未使用导出）：

- `export interface ConfigRevision { ... }`
- `export interface NodeConfigResponse { ... }`
- `export interface NodeConfigUpdateResult { ... }`
- `export async function listConfigRevisions(...) { ... }`
- `export function getNodeConfig(...) { ... }`
- `export function putNodeConfig(...) { ... }`

`createApplyJob` 之后应直接是 `export interface DeleteNodePeerResult`。

- [ ] **Step 4: Run SPA tests**

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test
```

Expected: PASS。`tsc -b` 不报对已删符号的引用。可用：

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npx tsc -b --pretty false
```

Expected: 无错误。

- [ ] **Step 5: Commit**

```bash
git add apps/raypx2-center/ui/src/pages/node-detail/NodeDetail.tsx \
  apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx \
  apps/raypx2-center/ui/src/api.ts
git rm apps/raypx2-center/ui/src/pages/node-detail/ConfigTab.tsx \
  apps/raypx2-center/ui/src/pages/node-detail/config-helpers.ts
git commit -m "$(cat <<'EOF'
feat(center): remove duplicate node Config tab

Drop the Config UI now that Peers, ACL, and Connections cover the
editable fields, while leaving the Go config API for apply jobs.
EOF
)"
```

---

### Task 4: Build UI artifact (if dist is shipped)

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
chore(center): rebuild UI after Connections parity and Config removal
EOF
)"
```

---

## Spec coverage checklist

| Spec 要求 | Task |
|---|---|
| Client 只读列对齐 Admin | Task 2 |
| Client 行内 client/server 速率 + 三个操作 | Task 2 |
| Client Detail GET JSON | Task 2 |
| Server 列、`peer` 命名、`total_streams_opened` | Task 1 + 2 |
| Server 行内 Apply，无 Detail | Task 2 |
| `parseRateBounds` 规则与文案 | Task 1 + 2 |
| offline 禁用写 / 不发 proxy | Task 2 |
| 删除 Config tab（含 unknown） | Task 3 |
| 删除 ConfigTab / config-helpers / 前端 config API | Task 3 |
| 保留 Go config API | 无 Go 文件改动 |
| Server compression 不迁 ACL | Task 3 不改 AclTab |
| SPA 测试 | Task 1–3 |
| HTTP 错误暴露改绑 `proxyNode` | Task 3 |

## Self-review notes

- 无 TBD/TODO 占位。
- Helpers 与 ConnectionsTab / 测试共用同一导出名：`parseRateBounds`、`RATE_INTEGER_ERROR`、`serverPeerName`、`readTotalStreamsOpened`、`connectionRowKey`、`rateField`。
- PATCH 路径与 body 在 Task 2 测试与实现中一致。
- 不引入 Center 后端变更，符合 spec 非目标。
