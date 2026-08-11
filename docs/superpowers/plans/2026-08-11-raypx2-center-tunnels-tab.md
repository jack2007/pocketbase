# raypx2 Center Tunnels Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Center 节点详情增加只读 Tunnels Tab，经既有 Admin proxy 实时拉取并展示与本地 Admin Console 对齐的 tunnel 列表。

**Architecture:** SPA 新增 `tunnels` Tab 与 `NodeTunnels` 组件；**仅在 `tab === "tunnels"` 时挂载**。进入 Tab / Refresh 时按 role 调用 `proxyNode` → `GET /api/v1/tunnels`（client）或 `GET /api/v1/server/tunnels`（server）。不改 Agent 协议、不落库、无新 Center 路由、不改 `styles.css`（已有横向滚动）。

**Tech Stack:** Vite + React + TypeScript + Vitest；既有 `proxyNode` / PocketBase center proxy。

**Spec:** [docs/superpowers/specs/2026-08-11-raypx2-center-tunnels-tab-design.md](../specs/2026-08-11-raypx2-center-tunnels-tab-design.md)

## Global Constraints

- 只读：无 Abort / Drain 按钮或写 API 调用。
- 数据路径仅实时代理；禁止改 `status_summary` 或新增 PocketBase 集合。
- 列集合必须与本地 Admin Console 一致（见 spec §4.3）。
- `NodeTunnels` 仅条件渲染挂载；禁止在 `NodeDetail` 根部常驻。
- 离线 / unknown role：零 `proxyNode` 调用。
- 请求失败时保留上一次成功行（与 Ops 一致），只 `setError`。
- 应用代码只改 `apps/raypx2-center/`；不修改 PocketBase 核心包；不改 raypx2。
- 默认不改 `styles.css`（`.table-shell { overflow-x: auto; }` 已存在）。
- 提交前须 `npm test` 通过；Task 2 须 `npm run build`。

---

## File Structure

| File | Responsibility |
|---|---|
| `apps/raypx2-center/ui/src/pages/NodeDetail.tsx` | Tab 类型扩展、`NodeTunnels` 组件、列常量与加载逻辑 |
| `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx` | Tunnels Tab 行为测试 |
| `apps/raypx2-center/ui/dist/**` | `npm run build` 产物（embed） |
| `apps/raypx2-center/README.md` | 一行说明 Tunnels Tab |

---

### Task 1: Tunnels Tab SPA（TDD）

**Files:**
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`
- Modify: `apps/raypx2-center/ui/src/pages/NodeDetail.tsx`
- Test: `apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx`

**Interfaces:**
- Consumes: `proxyNode(nodeKey, method, path)` from `../api`；`CenterNode`；现有 helpers `itemsFrom`, `display`, `errorMessage`, `PanelHeading`, `EmptyRow`
- Produces: Tab `"tunnels"`；组件 `NodeTunnels({ node })`；`CLIENT_TUNNEL_COLUMNS` / `SERVER_TUNNEL_COLUMNS`

- [ ] **Step 1: Write the failing tests**

在 `NodeDetail.test.tsx` 的 `describe("NodeDetail")` 内追加：

```tsx
  it("shows a Tunnels tab", () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    expect(screen.getByRole("tab", { name: "Tunnels" })).toBeInTheDocument();
  });

  it("loads client tunnels through the node proxy with client columns", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockResolvedValue({
      tunnels: [
        {
          tunnel_id: "t-1",
          peer_id: "p1",
          connection_id: "c1",
          target: "10.0.0.1:443",
          state: "open",
          role: "client",
          ingress: "socks",
          compress: "disabled",
          created_at: "2026-08-11T00:00:00Z",
          duration_ms: 1000,
          tcp_read_bytes: 10,
          tcp_write_bytes: 20,
          pending_bytes: 0,
          relay_backend: "linux",
          worker_index: 1,
          last_error: "",
        },
      ],
    });
    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/tunnels");
    });
    expect(api.proxyNode).not.toHaveBeenCalledWith(
      node.node_key,
      "GET",
      "/api/v1/server/tunnels",
    );
    expect(screen.getByText("t-1")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "tunnel_id" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "tcp_read_bytes" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /abort/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /drain/i })).not.toBeInTheDocument();
  });

  it("loads server tunnels through the node proxy with server columns", async () => {
    vi.mocked(api.proxyNode).mockResolvedValue({
      tunnels: [
        {
          tunnel_id: "st-1",
          peer_id: "sp1",
          connection_id: "sc1",
          state: "open",
          target: "192.168.1.1:80",
          role: "server",
          duration_ms: 500,
          active: true,
        },
      ],
    });
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        onlineServer.node_key,
        "GET",
        "/api/v1/server/tunnels",
      );
    });
    expect(screen.getByText("st-1")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "active" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "tcp_read_bytes" })).not.toBeInTheDocument();
  });

  it("does not proxy tunnels when the node is offline", async () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    expect(
      await screen.findByText("Tunnels are unavailable while this node is offline."),
    ).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });

  it("does not proxy tunnels for unknown role", async () => {
    const node: CenterNode = {
      id: "node-u1",
      node_key: "unknown-1",
      name: "Unknown node",
      role: "unknown",
      online: true,
    };
    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    expect(
      await screen.findByText('Tunnel inventory is not available for role "unknown".'),
    ).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test -- -t "Tunnels|tunnels|unknown role"
```

Expected: FAIL（无 Tunnels tab / 组件未实现）

- [ ] **Step 3: Implement Tab wiring + NodeTunnels**

在 `NodeDetail.tsx`：

1. 扩展 Tab 类型与渲染：

```tsx
type Tab = "overview" | "ops" | "tunnels" | "config" | "audit";
```

将 tabs 数组改为：

```tsx
{(["overview", "ops", "tunnels", "config", "audit"] as Tab[]).map((item) => (
```

在 tab 内容区增加（必须保持条件渲染，不可改为常驻）：

```tsx
{tab === "tunnels" && <NodeTunnels node={node} />}
```

（`title()` 已将首字母大写，tab 文案自动为 `Tunnels`。）

2. 在 `NodeOps` 附近加入列常量与组件（完整实现）：

```tsx
const CLIENT_TUNNEL_COLUMNS = [
  "tunnel_id",
  "peer_id",
  "connection_id",
  "target",
  "state",
  "role",
  "ingress",
  "compress",
  "created_at",
  "duration_ms",
  "tcp_read_bytes",
  "tcp_write_bytes",
  "pending_bytes",
  "relay_backend",
  "worker_index",
  "last_error",
] as const;

const SERVER_TUNNEL_COLUMNS = [
  "tunnel_id",
  "peer_id",
  "connection_id",
  "state",
  "target",
  "role",
  "duration_ms",
  "active",
] as const;

function NodeTunnels({ node }: { node: CenterNode }) {
  const [tunnels, setTunnels] = useState<JsonObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const columns =
    node.role === "client"
      ? CLIENT_TUNNEL_COLUMNS
      : node.role === "server"
        ? SERVER_TUNNEL_COLUMNS
        : null;

  async function load() {
    if (!node.online || !columns) {
      setTunnels([]);
      setError("");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const path =
        node.role === "server" ? "/api/v1/server/tunnels" : "/api/v1/tunnels";
      const result = await proxyNode(node.node_key, "GET", path);
      setTunnels(itemsFrom(result, "tunnels"));
    } catch (cause) {
      setError(errorMessage(cause));
      // Keep previous tunnels on failure (same as Ops keeping prior health/peers).
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [node.id, node.role, node.online]);

  if (!node.online) {
    return (
      <div className="ops-stack">
        <div className="offline-notice">Tunnels are unavailable while this node is offline.</div>
      </div>
    );
  }

  if (!columns) {
    return (
      <div className="ops-stack">
        <p className="muted">Tunnel inventory is not available for role "{node.role}".</p>
      </div>
    );
  }

  return (
    <div className="ops-stack">
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="panel ops-panel">
        <PanelHeading title="Tunnels" onRefresh={() => void load()} loading={loading} />
        <div className="table-shell compact">
          <table>
            <thead>
              <tr>
                {columns.map((column) => (
                  <th key={column}>{column}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {tunnels.length === 0 && (
                <EmptyRow columns={columns.length} loading={loading} />
              )}
              {tunnels.map((tunnel, index) => {
                const key =
                  typeof tunnel.tunnel_id === "string" && tunnel.tunnel_id
                    ? tunnel.tunnel_id
                    : String(index);
                return (
                  <tr key={key}>
                    {columns.map((column) => (
                      <td key={column} className={column === "tunnel_id" ? "strong" : undefined}>
                        {display(tunnel[column])}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
```

不要修改 `styles.css`。

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test
```

Expected: PASS（全部 NodeDetail 测试，含新增 Tunnels 用例）

- [ ] **Step 5: Commit**

```bash
cd /home/jack/src/pocketbase
git add apps/raypx2-center/ui/src/pages/NodeDetail.tsx \
  apps/raypx2-center/ui/src/pages/NodeDetail.test.tsx
git commit -m "$(cat <<'EOF'
feat(center): add read-only Tunnels tab via Admin proxy

Show client/server tunnel inventory on node detail using existing proxy paths.
EOF
)"
```

---

### Task 2: Embed 构建与 README

**Files:**
- Modify: `apps/raypx2-center/ui/dist/**`（由 build 生成）
- Modify: `apps/raypx2-center/README.md`

**Interfaces:**
- Consumes: Task 1 已合并的 SPA 源码
- Produces: embed 可用的 `ui/dist`；README 中 Tunnels 说明一句

- [ ] **Step 1: Build SPA**

Run:

```bash
cd /home/jack/src/pocketbase/apps/raypx2-center/ui && npm test && npm run build
```

Expected: tests PASS；`ui/dist/` 更新

- [ ] **Step 2: README 一句**

在 `apps/raypx2-center/README.md`「The Nodes console auto-refreshes…」段落后追加：

```markdown
Node detail includes a read-only **Tunnels** tab that proxies
`GET /api/v1/tunnels` (client) or `GET /api/v1/server/tunnels` (server) while
the node is online.
```

- [ ] **Step 3: Commit**

```bash
cd /home/jack/src/pocketbase
git add apps/raypx2-center/ui/dist apps/raypx2-center/README.md
git commit -m "$(cat <<'EOF'
chore(center): rebuild UI and document Tunnels tab

EOF
)"
```

- [ ] **Step 4: Manual smoke（可选，有运行中的 center+agent 时）**

1. `go run ./apps/raypx2-center serve --http=127.0.0.1:8090`
2. 登录 SPA → 打开在线 client/server 节点 → **Tunnels**
3. 确认列表列与本地 Console 一致；Refresh 重新拉取；离线节点仅提示；unknown role 仅提示

---

## Spec coverage (self-review)

| Spec 要求 | Task |
|---|---|
| 独立 Tunnels Tab + 条件挂载 | Task 1 |
| 实时代理 client/server path | Task 1 |
| 列对齐本地 Console | Task 1 常量 |
| 只读、无 abort/drain | Task 1 测试断言 |
| 离线不 proxy + 精确文案 | Task 1 测试 |
| unknown role 不 proxy | Task 1 测试 |
| 失败保留上次数据 | Task 1 实现注释 / catch 分支 |
| 无新 Center 路由 / 无 Agent / 无 styles 变更 | Global Constraints |
| README + embed dist | Task 2 |
