import { useCallback, useEffect, useState } from "react";
import { createNode, deleteNode, listNodes, login, logout, pb, type CreateNodeInput } from "./api";
import { Login } from "./pages/Login";
import { ApplyJobs } from "./pages/ApplyJobs";
import { NodeDetail } from "./pages/NodeDetail";
import { Nodes, type CenterNode } from "./pages/Nodes";
import { Overview } from "./pages/Overview";
import { Templates } from "./pages/Templates";

type Page = "overview" | "nodes" | "templates" | "apply-jobs";

const REFRESH_INTERVAL_MS = 10_000;

export default function App() {
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const [page, setPage] = useState<Page>("overview");
  const [nodes, setNodes] = useState<CenterNode[]>([]);
  const [selectedNode, setSelectedNode] = useState<CenterNode>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => pb.authStore.onChange(() => setAuthenticated(pb.authStore.isValid)), []);

  const refresh = useCallback(async (options?: { quiet?: boolean }) => {
    if (!pb.authStore.isValid) return;
    if (!options?.quiet) setLoading(true);
    setError("");
    try {
      const items = await listNodes();
      setNodes(items);
      setSelectedNode((current) => {
        if (!current) return current;
        return items.some((node) => node.id === current.id) ? current : undefined;
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to load nodes.");
    } finally {
      if (!options?.quiet) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!authenticated) return;
    void refresh();
    const timer = window.setInterval(() => void refresh({ quiet: true }), REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [authenticated, refresh]);

  async function handleCreate(input: CreateNodeInput) {
    const result = await createNode(input);
    await refresh({ quiet: true });
    return result;
  }

  async function handleDelete(node: CenterNode) {
    await deleteNode(node.node_key);
    await refresh({ quiet: true });
  }

  if (!authenticated) {
    return <Login onLogin={login} />;
  }

  const detailNode = selectedNode
    ? nodes.find((node) => node.id === selectedNode.id) ?? selectedNode
    : undefined;

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
            <button className={page === "templates" ? "active" : ""} onClick={() => { setPage("templates"); setSelectedNode(undefined); }}>
              <span>◇</span> Templates
            </button>
            <button className={page === "apply-jobs" ? "active" : ""} onClick={() => { setPage("apply-jobs"); setSelectedNode(undefined); }}>
              <span>↯</span> Apply Jobs
            </button>
          </nav>
        </div>
        <button className="sign-out" onClick={logout}>Sign out</button>
      </aside>
      <main className="content">
        {error && <div className="alert">{error}</div>}
        {page === "overview" ? (
          <Overview nodes={nodes} />
        ) : page === "templates" ? (
          <Templates />
        ) : page === "apply-jobs" ? (
          <ApplyJobs nodes={nodes} />
        ) : detailNode ? (
          <NodeDetail
            node={detailNode}
            onBack={() => setSelectedNode(undefined)}
          />
        ) : (
          <Nodes
            nodes={nodes}
            loading={loading}
            onRefresh={() => void refresh()}
            onCreate={handleCreate}
            onDelete={handleDelete}
            onSelect={setSelectedNode}
          />
        )}
      </main>
    </div>
  );
}
