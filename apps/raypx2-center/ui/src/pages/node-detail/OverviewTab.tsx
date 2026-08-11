import { useEffect, useState } from "react";
import { proxyNode } from "../../api";
import type { CenterNode } from "../Nodes";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  display,
  errorMessage,
  formatDate,
  itemsFrom,
  sumField,
  type JsonObject,
} from "@/lib/node-utils";

export function OverviewTab({ node }: { node: CenterNode }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [metrics, setMetrics] = useState({
    peers: 0,
    connections: 0,
    tunnels: 0,
    streams: 0,
    aclDenied: 0,
  });

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError("");
      if (!node.online) {
        setLoading(false);
        return;
      }
      try {
        if (node.role === "client") {
          const [peersResult, tunnelsResult] = await Promise.all([
            proxyNode(node.node_key, "GET", "/api/v1/peers"),
            proxyNode(node.node_key, "GET", "/api/v1/tunnels"),
          ]);
          const peers = itemsFrom(peersResult, "peers");
          const connectionGroups = await Promise.all(peers.map(async (peer) => {
            const peerId = typeof peer.peer_id === "string" ? peer.peer_id : "";
            if (!peerId) return [];
            const result = await proxyNode(
              node.node_key,
              "GET",
              `/api/v1/peers/${encodeURIComponent(peerId)}/connections`,
            );
            return itemsFrom(result, "connections");
          }));
          if (!cancelled) {
            setMetrics({
              peers: peers.length,
              connections: connectionGroups.flat().length || sumField(peers, "connection_count"),
              tunnels: itemsFrom(tunnelsResult, "tunnels").length,
              streams: sumField(peers, "active_streams"),
              aclDenied: 0,
            });
          }
        } else if (node.role === "server") {
          const [peersResult, connectionsResult, tunnelsResult, metricsResult] = await Promise.all([
            proxyNode(node.node_key, "GET", "/api/v1/server/peers").catch(() => ({})),
            proxyNode(node.node_key, "GET", "/api/v1/server/connections"),
            proxyNode(node.node_key, "GET", "/api/v1/server/tunnels"),
            proxyNode<JsonObject>(node.node_key, "GET", "/api/v1/metrics").catch((): JsonObject => ({})),
          ]);
          const peers = itemsFrom(peersResult, "peers");
          const connections = itemsFrom(connectionsResult, "connections");
          if (!cancelled) {
            setMetrics({
              peers: peers.length || new Set(connections.map((item) => display(item.peer))).size,
              connections: connections.length,
              tunnels: itemsFrom(tunnelsResult, "tunnels").length,
              streams: sumField(connections, "active_streams"),
              aclDenied: typeof metricsResult.acl_denied === "number" ? metricsResult.acl_denied : 0,
            });
          }
        }
      } catch (cause) {
        if (!cancelled) setError(errorMessage(cause));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [node.id, node.node_key, node.online, node.role]);

  return (
    <div className="space-y-4">
      {!node.online && (
        <Alert>
          <AlertDescription>Node is offline. Live overview metrics are unavailable.</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Metric title="Role" value={node.role} note="Configured enrollment role" loading={false} />
        <Metric title="Health" value={node.health_status || "Unknown"} note="Last reported health" loading={false} />
        <Metric title="Last seen" value={formatDate(node.last_seen_at)} note="Agent heartbeat" loading={false} />
        {node.role === "server" ? (
          <Metric title="ACL denied" value={metrics.aclDenied} note="Aggregate deny counter" loading={loading && node.online} />
        ) : (
          <Metric title="Active streams" value={metrics.streams} note="Open streams across peers" loading={loading && node.online} />
        )}
      </div>
      {(node.role === "client" || node.role === "server") && (
        <div className="grid gap-4 sm:grid-cols-3">
          <Metric title="Peers" value={metrics.peers} note="Runtime peer inventory" loading={loading && node.online} />
          <Metric title="Connections" value={metrics.connections} note="Observed connections" loading={loading && node.online} />
          <Metric title="Tunnels" value={metrics.tunnels} note="Active tunnel sessions" loading={loading && node.online} />
        </div>
      )}
    </div>
  );
}

function Metric({
  title,
  value,
  note,
  loading,
}: {
  title: string;
  value: string | number;
  note: string;
  loading: boolean;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-2xl tracking-tight">
          {loading ? <Skeleton className="h-8 w-16" /> : value}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-xs text-muted-foreground">{note}</p>
      </CardContent>
    </Card>
  );
}
