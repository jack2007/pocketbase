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
import { buildPeerSavePayload, emptyPeerForm, peerToForm } from "./peer-form-helpers";

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
    setForm(peerToForm(peer));
    setEditorOpen(true);
  }

  async function savePeer() {
    if (!node.online || node.role !== "client") return;
    setWriting(true);
    setError("");
    try {
      const payload = buildPeerSavePayload(form);
      if (!payload.peer_id) throw new Error("peer_id is required.");
      if (editing) {
        await proxyNode(
          node.node_key,
          "PUT",
          `/api/v1/peers/${encodeURIComponent(String(payload.peer_id))}`,
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
                      <TableHead>socks_listen</TableHead>
                      <TableHead>http_listen</TableHead>
                      <TableHead>connection_count</TableHead>
                      <TableHead>connected_connections</TableHead>
                      <TableHead>active_streams</TableHead>
                      <TableHead>total_streams</TableHead>
                      <TableHead>reconnects</TableHead>
                      <TableHead>last_error</TableHead>
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
                    <TableCell colSpan={node.role === "client" ? 13 : 5} className="h-20 text-center text-muted-foreground">
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
                      <TableCell>{display(peer.socks_listen)}</TableCell>
                      <TableCell>{display(peer.http_listen)}</TableCell>
                      <TableCell>{display(peer.connection_count)}</TableCell>
                      <TableCell>{display(peer.connected_connections)}</TableCell>
                      <TableCell>{display(peer.active_streams)}</TableCell>
                      <TableCell>{display(peer.total_streams)}</TableCell>
                      <TableCell>{display(peer.reconnects)}</TableCell>
                      <TableCell>{display(peer.last_error)}</TableCell>
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
            <Field label="peer_id" htmlFor="peer-id">
              <Input
                id="peer-id"
                value={form.peer_id}
                disabled={Boolean(editing)}
                onChange={(event) => setForm({ ...form, peer_id: event.target.value })}
              />
            </Field>
            <Field label="quic_peer addresses" htmlFor="peer-quic-peer">
              <Input
                id="peer-quic-peer"
                value={form.quic_peer}
                onChange={(event) => setForm({ ...form, quic_peer: event.target.value })}
              />
            </Field>
            <Field label="Connections" htmlFor="peer-connections">
              <Input
                id="peer-connections"
                type="number"
                min={1}
                max={128}
                value={form.connections}
                onChange={(event) => setForm({ ...form, connections: event.target.value })}
              />
            </Field>
            <Field label="socks_listen" htmlFor="peer-socks-listen">
              <Input
                id="peer-socks-listen"
                value={form.socks_listen}
                onChange={(event) => setForm({ ...form, socks_listen: event.target.value })}
              />
            </Field>
            <Field label="http_listen" htmlFor="peer-http-listen">
              <Input
                id="peer-http-listen"
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
            <Field label="Desired encryption" htmlFor="peer-encryption">
              <select
                id="peer-encryption"
                className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm"
                value={form.encryption}
                onChange={(event) => setForm({ ...form, encryption: event.target.value })}
              >
                <option value="enabled">enabled</option>
                <option value="disabled">disabled</option>
              </select>
            </Field>
            <Field label="Desired compression mode" htmlFor="peer-compression-mode">
              <select
                id="peer-compression-mode"
                className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm"
                value={form.compression_mode}
                onChange={(event) => setForm({ ...form, compression_mode: event.target.value })}
              >
                <option value="enabled">enabled</option>
                <option value="disabled">disabled</option>
              </select>
            </Field>
            <Field label="Desired compression level" htmlFor="peer-compression-level">
              <Input
                id="peer-compression-level"
                type="number"
                min={1}
                max={22}
                step={1}
                value={form.compression_level}
                onChange={(event) => setForm({ ...form, compression_level: event.target.value })}
              />
            </Field>
            <Field label="Applied encryption" htmlFor="peer-applied-encryption">
              <output id="peer-applied-encryption" className="text-sm">
                {form.applied_encryption}
              </output>
            </Field>
            <Field label="Applied compression mode" htmlFor="peer-applied-compression-mode">
              <output id="peer-applied-compression-mode" className="text-sm">
                {form.applied_compression_mode}
              </output>
            </Field>
            <Field label="Applied compression level" htmlFor="peer-applied-compression-level">
              <output id="peer-applied-compression-level" className="text-sm">
                {form.applied_compression_level}
              </output>
            </Field>
            <Field label="Restart required" htmlFor="peer-restart-required">
              <output id="peer-restart-required" className="text-sm">
                {String(form.restart_required)}
              </output>
            </Field>
            <Field label="Path settings" htmlFor="peer-paths">
              <Textarea
                id="peer-paths"
                className="min-h-24 font-mono text-xs"
                value={form.paths}
                onChange={(event) => setForm({ ...form, paths: event.target.value })}
              />
            </Field>
            <Field label="Port forwards" htmlFor="peer-port-forwards">
              <Textarea
                id="peer-port-forwards"
                className="min-h-24 font-mono text-xs"
                value={form.port_forwards}
                onChange={(event) => setForm({ ...form, port_forwards: event.target.value })}
              />
            </Field>
          </div>
          <p className="px-4 text-sm text-muted-foreground">
            Provide either quic_peer addresses or a paths array. Connection settings on a newly
            created peer apply immediately. Editing connection settings on an existing peer
            updates desired values and requires a client process restart.
          </p>
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

function Field({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}
