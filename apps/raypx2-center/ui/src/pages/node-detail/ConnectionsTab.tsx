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
import { Label } from "@/components/ui/label";
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

export function ConnectionsTab({ node }: { node: CenterNode }) {
  const [connections, setConnections] = useState<JsonObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<JsonObject>();
  const [minRate, setMinRate] = useState("0");
  const [maxRate, setMaxRate] = useState("0");

  async function load() {
    setLoading(true);
    setError("");
    try {
      if (!node.online) {
        setConnections([]);
        return;
      }
      if (node.role === "server") {
        const result = await proxyNode(node.node_key, "GET", "/api/v1/server/connections");
        setConnections(itemsFrom(result, "connections"));
      } else if (node.role === "client") {
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
        setConnections(groups.flat());
      } else {
        setConnections([]);
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online, node.role]);

  function openDetail(connection: JsonObject) {
    setSelected(connection);
    setMinRate(String(connection.min_send_rate_kbps ?? connection.client_min_send_rate_kbps ?? 0));
    setMaxRate(String(connection.max_send_rate_kbps ?? connection.client_max_send_rate_kbps ?? 0));
  }

  async function saveRates() {
    if (!selected || !node.online) return;
    const connectionId = itemId(selected);
    const peerId = typeof selected.peer_id === "string" ? selected.peer_id : "";
    if (!connectionId) return;
    setWriting(true);
    setError("");
    try {
      if (!/^\d+$/.test(minRate.trim()) || !/^\d+$/.test(maxRate.trim())) {
        throw new Error("Send rates must be non-negative integers.");
      }
      const bounds = {
        min_send_rate_kbps: Number(minRate),
        max_send_rate_kbps: Number(maxRate),
      };
      if (bounds.min_send_rate_kbps !== 0 && bounds.max_send_rate_kbps !== 0
        && bounds.min_send_rate_kbps > bounds.max_send_rate_kbps) {
        throw new Error("Minimum send rate cannot exceed maximum.");
      }
      if (node.role === "server") {
        await proxyNode(
          node.node_key,
          "PATCH",
          `/api/v1/server/connections/${encodeURIComponent(connectionId)}`,
          bounds,
        );
      } else {
        if (!peerId) throw new Error("Missing peer_id for connection.");
        await proxyNode(
          node.node_key,
          "PATCH",
          `/api/v1/peers/${encodeURIComponent(peerId)}/connections/${encodeURIComponent(connectionId)}`,
          bounds,
        );
      }
      toast.success("Send rates updated");
      setSelected(undefined);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setWriting(false);
    }
  }

  if (node.role !== "client" && node.role !== "server") {
    return <p className="text-sm text-muted-foreground">Connections are not available for this node role.</p>;
  }

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
                  <TableHead>connection_id</TableHead>
                  <TableHead>peer</TableHead>
                  <TableHead>state</TableHead>
                  <TableHead>remote</TableHead>
                  <TableHead>min_kbps</TableHead>
                  <TableHead>max_kbps</TableHead>
                  <TableHead>actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {connections.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={7} className="h-20 text-center text-muted-foreground">
                      {loading ? "Loading…" : "No items returned."}
                    </TableCell>
                  </TableRow>
                )}
                {connections.map((connection, index) => (
                  <TableRow key={itemId(connection) || index}>
                    <TableCell className="font-medium">{itemId(connection) || `Connection ${index + 1}`}</TableCell>
                    <TableCell>{display(connection.peer_id ?? connection.peer ?? connection.client_name)}</TableCell>
                    <TableCell>{display(connection.state ?? connection.status)}</TableCell>
                    <TableCell>{display(connection.remote_address ?? connection.remote)}</TableCell>
                    <TableCell>{display(connection.min_send_rate_kbps ?? connection.client_min_send_rate_kbps)}</TableCell>
                    <TableCell>{display(connection.max_send_rate_kbps ?? connection.client_max_send_rate_kbps)}</TableCell>
                    <TableCell>
                      <Button variant="outline" size="sm" onClick={() => openDetail(connection)}>
                        Detail
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataTableShell>
        </CardContent>
      </Card>

      <Sheet open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(undefined)}>
        <SheetContent className="overflow-y-auto sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>Connection detail</SheetTitle>
          </SheetHeader>
          <div className="space-y-4 px-4">
            <JsonView value={selected} />
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="min-rate">min_send_rate_kbps</Label>
                <Input
                  id="min-rate"
                  value={minRate}
                  disabled={!node.online}
                  onChange={(event) => setMinRate(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="max-rate">max_send_rate_kbps</Label>
                <Input
                  id="max-rate"
                  value={maxRate}
                  disabled={!node.online}
                  onChange={(event) => setMaxRate(event.target.value)}
                />
              </div>
            </div>
          </div>
          <SheetFooter>
            <Button variant="outline" onClick={() => setSelected(undefined)}>Close</Button>
            <Button disabled={!node.online || writing} onClick={() => void saveRates()}>
              {writing ? "Saving…" : "Save rates"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
