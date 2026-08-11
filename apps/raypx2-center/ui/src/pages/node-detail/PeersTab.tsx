import { useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { deleteNodePeer, proxyNode } from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { display, errorMessage, itemId, itemsFrom, type JsonObject } from "@/lib/node-utils";

export function PeersTab({ node }: { node: CenterNode }) {
  const [peers, setPeers] = useState<JsonObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState("");
  const [pendingDelete, setPendingDelete] = useState<JsonObject>();
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<JsonObject>();
  const [form, setForm] = useState(emptyPeerForm());

  async function load() {
    setLoading(true);
    setError("");
    try {
      if (!node.online) {
        setPeers([]);
        return;
      }
      if (node.role === "client") {
        const result = await proxyNode(node.node_key, "GET", "/api/v1/peers");
        setPeers(itemsFrom(result, "peers"));
      } else if (node.role === "server") {
        const result = await proxyNode(node.node_key, "GET", "/api/v1/server/peers").catch(async () => {
          const connections = await proxyNode(node.node_key, "GET", "/api/v1/server/connections");
          const rows = itemsFrom(connections, "connections");
          const byPeer = new Map<string, JsonObject>();
          for (const row of rows) {
            const peer = display(row.peer ?? row.peer_id);
            const current = byPeer.get(peer) ?? {
              peer,
              remote_address: row.remote_address,
              connections: 0,
              active_streams: 0,
              active_tunnels: 0,
            };
            current.connections = Number(current.connections) + 1;
            current.active_streams = Number(current.active_streams) + (typeof row.active_streams === "number" ? row.active_streams : 0);
            current.active_tunnels = Number(current.active_tunnels) + (typeof row.active_tunnels === "number" ? row.active_tunnels : 0);
            byPeer.set(peer, current);
          }
          return { peers: [...byPeer.values()] };
        });
        setPeers(itemsFrom(result, "peers"));
      } else {
        setPeers([]);
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online, node.role]);

  async function togglePeer(peer: JsonObject) {
    const peerId = itemId(peer);
    if (!peerId || !node.online) return;
    const action = peer.enabled === false || peer.state === "disabled" ? "enable" : "disable";
    setWriting(true);
    setError("");
    try {
      await proxyNode(node.node_key, "POST", `/api/v1/peers/${encodeURIComponent(peerId)}:${action}`);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setWriting(false);
    }
  }

  async function confirmDelete() {
    const peerId = pendingDelete ? itemId(pendingDelete) : "";
    if (!peerId || !node.online) return;
    setWriting(true);
    setError("");
    try {
      await deleteNodePeer(node.node_key, peerId);
      toast.success(`Deleted peer ${peerId}`);
      setPendingDelete(undefined);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setWriting(false);
    }
  }

  function openCreate() {
    setEditing(undefined);
    setForm(emptyPeerForm());
    setEditorOpen(true);
  }

  function openEdit(peer: JsonObject) {
    setEditing(peer);
    setForm({
      peer_id: String(peer.peer_id ?? ""),
      quic_peer: String(peer.quic_peer ?? peer.address ?? ""),
      connections: String(peer.quic_connections ?? peer.connections ?? 1),
      socks_listen: String(peer.socks_listen ?? "127.0.0.1:1080"),
      http_listen: String(peer.http_listen ?? "127.0.0.1:8080"),
      enabled: peer.enabled !== false && peer.state !== "disabled",
      paths: JSON.stringify(peer.paths ?? [], null, 2),
      port_forwards: JSON.stringify(peer.port_forwards ?? [], null, 2),
    });
    setEditorOpen(true);
  }

  async function savePeer() {
    if (!node.online || node.role !== "client") return;
    setWriting(true);
    setError("");
    try {
      const payload = {
        peer_id: form.peer_id.trim(),
        quic_peer: form.quic_peer.trim(),
        quic_connections: Number(form.connections) || 1,
        socks_listen: form.socks_listen.trim(),
        http_listen: form.http_listen.trim(),
        enabled: form.enabled,
        paths: JSON.parse(form.paths || "[]"),
        port_forwards: JSON.parse(form.port_forwards || "[]"),
      };
      if (!payload.peer_id) throw new Error("peer_id is required.");
      if (editing) {
        await proxyNode(
          node.node_key,
          "PUT",
          `/api/v1/peers/${encodeURIComponent(payload.peer_id)}`,
          payload,
        );
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

  if (node.role !== "client" && node.role !== "server") {
    return <p className="text-sm text-muted-foreground">Peers are not available for this node role.</p>;
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
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle>{node.role === "client" ? "Client peers" : "Server peers"}</CardTitle>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
              Refresh
            </Button>
            {node.role === "client" && (
              <Button size="sm" disabled={!node.online} onClick={openCreate}>
                Create Peer
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="px-0 pb-0">
          <DataTableShell>
            <Table>
              <TableHeader>
                <TableRow>
                  {node.role === "client" ? (
                    <>
                      <TableHead>peer_id</TableHead>
                      <TableHead>state</TableHead>
                      <TableHead>enabled</TableHead>
                      <TableHead>quic_peer</TableHead>
                      <TableHead>actions</TableHead>
                    </>
                  ) : (
                    <>
                      <TableHead>peer</TableHead>
                      <TableHead>remote_address</TableHead>
                      <TableHead>connections</TableHead>
                      <TableHead>active_streams</TableHead>
                      <TableHead>active_tunnels</TableHead>
                    </>
                  )}
                </TableRow>
              </TableHeader>
              <TableBody>
                {peers.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="h-20 text-center text-muted-foreground">
                      {loading ? "Loading…" : "No items returned."}
                    </TableCell>
                  </TableRow>
                )}
                {node.role === "client"
                  ? peers.map((peer, index) => (
                    <TableRow key={itemId(peer) || index}>
                      <TableCell className="font-medium">{itemId(peer) || `Peer ${index + 1}`}</TableCell>
                      <TableCell>{display(peer.state)}</TableCell>
                      <TableCell>{display(peer.enabled)}</TableCell>
                      <TableCell>{display(peer.quic_peer ?? peer.address ?? peer.endpoint)}</TableCell>
                      <TableCell className="space-x-2">
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!node.online || writing || !itemId(peer)}
                          onClick={() => openEdit(peer)}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!node.online || writing || !itemId(peer)}
                          onClick={() => void togglePeer(peer)}
                        >
                          {peer.enabled === false || peer.state === "disabled" ? "Enable" : "Disable"}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!node.online || writing || !itemId(peer)}
                          onClick={() => setPendingDelete(peer)}
                        >
                          Delete
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                  : peers.map((peer, index) => (
                    <TableRow key={itemId(peer) || index}>
                      <TableCell className="font-medium">{display(peer.peer ?? peer.peer_id)}</TableCell>
                      <TableCell>{display(peer.remote_address)}</TableCell>
                      <TableCell>{display(peer.connections)}</TableCell>
                      <TableCell>{display(peer.active_streams)}</TableCell>
                      <TableCell>{display(peer.active_tunnels)}</TableCell>
                    </TableRow>
                  ))}
              </TableBody>
            </Table>
          </DataTableShell>
        </CardContent>
      </Card>

      <Sheet open={editorOpen} onOpenChange={setEditorOpen}>
        <SheetContent className="overflow-y-auto sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>{editing ? "Edit Peer" : "Create Peer"}</SheetTitle>
          </SheetHeader>
          <div className="grid gap-4 px-4 py-2">
            <Field label="peer_id">
              <Input
                value={form.peer_id}
                disabled={Boolean(editing)}
                onChange={(event) => setForm({ ...form, peer_id: event.target.value })}
              />
            </Field>
            <Field label="quic_peer addresses">
              <Input
                value={form.quic_peer}
                onChange={(event) => setForm({ ...form, quic_peer: event.target.value })}
              />
            </Field>
            <Field label="Connections">
              <Input
                type="number"
                min={1}
                max={128}
                value={form.connections}
                onChange={(event) => setForm({ ...form, connections: event.target.value })}
              />
            </Field>
            <Field label="socks_listen">
              <Input
                value={form.socks_listen}
                onChange={(event) => setForm({ ...form, socks_listen: event.target.value })}
              />
            </Field>
            <Field label="http_listen">
              <Input
                value={form.http_listen}
                onChange={(event) => setForm({ ...form, http_listen: event.target.value })}
              />
            </Field>
            <div className="flex items-center justify-between rounded-md border p-3">
              <Label htmlFor="peer-enabled">Peer enabled</Label>
              <Switch
                id="peer-enabled"
                checked={form.enabled}
                onCheckedChange={(checked) => setForm({ ...form, enabled: checked })}
              />
            </div>
            <Field label="Path settings">
              <Textarea
                className="min-h-24 font-mono text-xs"
                value={form.paths}
                onChange={(event) => setForm({ ...form, paths: event.target.value })}
              />
            </Field>
            <Field label="Port forwards">
              <Textarea
                className="min-h-24 font-mono text-xs"
                value={form.port_forwards}
                onChange={(event) => setForm({ ...form, port_forwards: event.target.value })}
              />
            </Field>
          </div>
          <SheetFooter>
            <Button variant="outline" onClick={() => setEditorOpen(false)}>Cancel</Button>
            <Button disabled={writing} onClick={() => void savePeer()}>
              {writing ? "Saving…" : "Create / Save"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => !open && setPendingDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete peer?</AlertDialogTitle>
            <AlertDialogDescription>
              Delete peer &quot;{pendingDelete ? itemId(pendingDelete) : ""}&quot; from this node? This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void confirmDelete()}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function emptyPeerForm() {
  return {
    peer_id: "",
    quic_peer: "",
    connections: "1",
    socks_listen: "127.0.0.1:1080",
    http_listen: "127.0.0.1:8080",
    enabled: true,
    paths: "[]",
    port_forwards: "[]",
  };
}
