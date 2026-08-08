import { useCallback, useEffect, useState } from "react";
import { createNode, listNodes, login, logout, pb, type CreateNodeInput } from "./api";
import { Login } from "./pages/Login";
import { NodeDetail } from "./pages/NodeDetail";
import { Nodes, type CenterNode } from "./pages/Nodes";
import { Overview } from "./pages/Overview";

type Page = "overview" | "nodes";

export default function App() {
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const [page, setPage] = useState<Page>("overview");
  const [nodes, setNodes] = useState<CenterNode[]>([]);
  const [selectedNode, setSelectedNode] = useState<CenterNode>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => pb.authStore.onChange(() => setAuthenticated(pb.authStore.isValid)), []);

  const refresh = useCallback(async () => {
    if (!pb.authStore.isValid) return;
    setLoading(true);
    setError("");
    try {
      setNodes(await listNodes());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to load nodes.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (authenticated) void refresh();
  }, [authenticated, refresh]);

  async function handleCreate(input: CreateNodeInput) {
    const result = await createNode(input);
    await refresh();
    return result;
  }

  if (!authenticated) {
    return <Login onLogin={login} />;
  }

  return (
    <div className="app-shell">
      <aside>
        <div>
          <div className="brand">
            <div className="brand-mark small">R2</div>
            <div><strong>raypx2</strong><span>center</span></div>
          </div>
          <nav aria-label="Primary navigation">
            <button className={page === "overview" ? "active" : ""} onClick={() => setPage("overview")}>
              <span>◫</span> Overview
            </button>
            <button className={page === "nodes" ? "active" : ""} onClick={() => { setPage("nodes"); setSelectedNode(undefined); }}>
              <span>⌘</span> Nodes
            </button>
          </nav>
        </div>
        <button className="sign-out" onClick={logout}>Sign out</button>
      </aside>
      <main className="content">
        {error && <div className="alert">{error}</div>}
        {page === "overview" ? (
          <Overview nodes={nodes} />
        ) : selectedNode ? (
          <NodeDetail
            node={nodes.find((node) => node.id === selectedNode.id) ?? selectedNode}
            onBack={() => setSelectedNode(undefined)}
          />
        ) : (
          <Nodes
            nodes={nodes}
            loading={loading}
            onRefresh={refresh}
            onCreate={handleCreate}
            onSelect={setSelectedNode}
          />
        )}
      </main>
    </div>
  );
}
