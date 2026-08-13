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
    const connectionId = typeof connection.connection_id === "string" ? connection.connection_id : "";
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
    const connectionId = typeof connection.connection_id === "string" ? connection.connection_id : "";
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
    const connectionId = typeof connection.connection_id === "string" ? connection.connection_id : "";
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
                  const connectionId = isClient
                    ? (typeof connection.connection_id === "string" ? connection.connection_id : "")
                    : itemId(connection);
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
