import { useCallback, useEffect, useState } from "react";
import { createNode, deleteNode, listNodes, login, logout, pb, type CreateNodeInput } from "./api";
import { AppShell, type FleetPage } from "./components/layout/AppShell";
import { Toaster } from "./components/ui/sonner";
import { TooltipProvider } from "./components/ui/tooltip";
import { ApplyJobs } from "./pages/ApplyJobs";
import { Login } from "./pages/Login";
import { NodeDetail } from "./pages/node-detail/NodeDetail";
import { Nodes, type CenterNode } from "./pages/Nodes";
import { Overview } from "./pages/Overview";
import { Templates } from "./pages/Templates";

const REFRESH_INTERVAL_MS = 10_000;

export default function App() {
  const [authenticated, setAuthenticated] = useState(pb.authStore.isValid);
  const [page, setPage] = useState<FleetPage>("overview");
  const [nodes, setNodes] = useState<CenterNode[]>([]);
  const [selectedNode, setSelectedNode] = useState<CenterNode>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [refreshPaused, setRefreshPaused] = useState(false);

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
    if (!authenticated || refreshPaused) return;
    void refresh();
    const timer = window.setInterval(() => void refresh({ quiet: true }), REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [authenticated, refresh, refreshPaused]);

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
    return (
      <TooltipProvider>
        <Login onLogin={login} />
        <Toaster />
      </TooltipProvider>
    );
  }

  const detailNode = selectedNode
    ? nodes.find((node) => node.id === selectedNode.id) ?? selectedNode
    : undefined;

  const onlineCount = nodes.filter((node) => node.online).length;

  return (
    <TooltipProvider>
      <AppShell
        page={page}
        onNavigate={(next) => {
          setPage(next);
          setSelectedNode(undefined);
        }}
        onlineCount={onlineCount}
        totalCount={nodes.length}
        refreshPaused={refreshPaused}
        onToggleRefresh={() => setRefreshPaused((value) => !value)}
        onRefreshNow={() => void refresh()}
        refreshing={loading}
        error={error}
        onSignOut={logout}
      >
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
      </AppShell>
      <Toaster />
    </TooltipProvider>
  );
}
