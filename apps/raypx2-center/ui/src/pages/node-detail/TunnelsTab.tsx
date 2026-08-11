import { useEffect, useState } from "react";
import { proxyNode } from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { display, errorMessage, itemId, itemsFrom, type JsonObject } from "@/lib/node-utils";

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

export function TunnelsTab({ node }: { node: CenterNode }) {
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
      const path = node.role === "server" ? "/api/v1/server/tunnels" : "/api/v1/tunnels";
      const result = await proxyNode(node.node_key, "GET", path);
      setTunnels(itemsFrom(result, "tunnels"));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online, node.role]);

  if (!node.online) {
    return (
      <Alert>
        <AlertDescription>Tunnels are unavailable while this node is offline.</AlertDescription>
      </Alert>
    );
  }

  if (!columns) {
    return (
      <Alert>
        <AlertDescription>
          Tunnel inventory is not available for role &quot;{node.role}&quot;.
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Tunnels</CardTitle>
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
                </TableRow>
              </TableHeader>
              <TableBody>
                {tunnels.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={columns.length} className="h-20 text-center text-muted-foreground">
                      {loading ? "Loading…" : "No items returned."}
                    </TableCell>
                  </TableRow>
                )}
                {tunnels.map((tunnel, index) => (
                  <TableRow key={itemId(tunnel) || index}>
                    {columns.map((column) => (
                      <TableCell key={column}>{display(tunnel[column])}</TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataTableShell>
        </CardContent>
      </Card>
    </div>
  );
}
