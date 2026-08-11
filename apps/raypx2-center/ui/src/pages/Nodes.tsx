import { useRef, useState, type FormEvent } from "react";
import { toast } from "sonner";
import type { CreateNodeInput, CreateNodeResult } from "../api";
import { PageHeader } from "@/components/shared/PageHeader";
import { StatusBadge } from "@/components/shared/StatusBadge";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatDate } from "@/lib/node-utils";
import { cn } from "@/lib/utils";

export interface CenterNode {
  id: string;
  node_key: string;
  name: string;
  role: "client" | "server" | "unknown";
  online: boolean;
  last_seen_at?: string;
  health_status?: string;
}

interface NodesProps {
  nodes: CenterNode[];
  loading: boolean;
  onRefresh: () => void;
  onCreate?: (input: CreateNodeInput) => Promise<CreateNodeResult>;
  onDelete?: (node: CenterNode) => Promise<void>;
  onSelect?: (node: CenterNode) => void;
}

export function Nodes({ nodes, loading, onRefresh, onCreate, onDelete, onSelect }: NodesProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deletingKey, setDeletingKey] = useState("");
  const [pendingDelete, setPendingDelete] = useState<CenterNode>();
  const [error, setError] = useState("");
  const [secret, setSecret] = useState("");
  const [role, setRole] = useState<CreateNodeInput["role"]>("unknown");
  const secretInputRef = useRef<HTMLTextAreaElement>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!onCreate) return;
    setSubmitting(true);
    setError("");
    const data = new FormData(event.currentTarget);
    try {
      const result = await onCreate({
        node_key: String(data.get("node_key") ?? "").trim(),
        name: String(data.get("name") ?? "").trim(),
        role,
      });
      setSecret(result.enroll_secret);
      toast.success("Node created");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to create node.");
    } finally {
      setSubmitting(false);
    }
  }

  async function confirmDelete() {
    if (!onDelete || !pendingDelete) return;
    setDeletingKey(pendingDelete.node_key);
    setError("");
    try {
      await onDelete(pendingDelete);
      toast.success("Node deleted");
      setPendingDelete(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to delete node.");
    } finally {
      setDeletingKey("");
    }
  }

  function closeDialog() {
    setDialogOpen(false);
    setSecret("");
    setError("");
    setRole("unknown");
  }

  async function copySecret() {
    const value = secretInputRef.current?.value || secret;
    if (!value) return;

    try {
      if (window.isSecureContext && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const input = secretInputRef.current;
        if (!input) throw new Error("secret input missing");
        input.focus();
        input.select();
        input.setSelectionRange(0, input.value.length);
        const ok = document.execCommand("copy");
        input.setSelectionRange(0, 0);
        if (!ok) throw new Error("copy failed");
      }
      toast.success("Secret copied");
    } catch {
      toast.error("Unable to copy secret");
    }
  }

  return (
    <section>
      <PageHeader
        eyebrow="Fleet"
        title="Nodes"
        description="Manage enrolled raypx2 instances."
        actions={
          <>
            <Button variant="outline" onClick={onRefresh} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
            {onCreate && <Button onClick={() => setDialogOpen(true)}>Create node</Button>}
          </>
        }
      />

      {error && !dialogOpen && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Node key</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Health</TableHead>
              <TableHead>Last seen</TableHead>
              {onDelete && <TableHead>Actions</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {!loading && nodes.length === 0 && (
              <TableRow>
                <TableCell colSpan={onDelete ? 7 : 6} className="h-24 text-center text-muted-foreground">
                  No nodes yet. Create one to get started.
                </TableCell>
              </TableRow>
            )}
            {nodes.map((node) => (
              <TableRow key={node.id}>
                <TableCell className="font-medium">
                  {onSelect ? (
                    <button
                      type="button"
                      className="underline decoration-muted-foreground/50 underline-offset-4 hover:text-foreground"
                      onClick={() => onSelect(node)}
                    >
                      {node.name || "Unnamed node"}
                    </button>
                  ) : (
                    node.name || "Unnamed node"
                  )}
                </TableCell>
                <TableCell><code className="text-xs">{node.node_key}</code></TableCell>
                <TableCell><Badge variant="secondary">{node.role}</Badge></TableCell>
                <TableCell><StatusBadge online={node.online} /></TableCell>
                <TableCell>{node.health_status || "—"}</TableCell>
                <TableCell>{formatDate(node.last_seen_at)}</TableCell>
                {onDelete && (
                  <TableCell>
                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={deletingKey === node.node_key}
                      onClick={() => setPendingDelete(node)}
                    >
                      {deletingKey === node.node_key ? "Deleting…" : "Delete"}
                    </Button>
                  </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableShell>

      <Dialog open={dialogOpen} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent>
          {secret ? (
            <>
              <DialogHeader>
                <DialogTitle>Save the enrollment secret</DialogTitle>
                <DialogDescription>
                  This secret is shown only once. Store it before closing.
                </DialogDescription>
              </DialogHeader>
              <textarea
                ref={secretInputRef}
                readOnly
                value={secret}
                rows={3}
                aria-label="Enrollment secret"
                onFocus={(event) => event.currentTarget.select()}
                className={cn(
                  "w-full resize-none rounded-md border px-3 py-2 font-mono text-sm break-all outline-none",
                  "border-emerald-500/30 bg-emerald-500/10",
                  "focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
                )}
              />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => void copySecret()}>
                  Copy secret
                </Button>
                <Button type="button" onClick={closeDialog}>Done</Button>
              </DialogFooter>
            </>
          ) : (
            <form onSubmit={submit}>
              <DialogHeader>
                <DialogTitle>Create node</DialogTitle>
                <DialogDescription>Issue enrollment credentials for a new agent.</DialogDescription>
              </DialogHeader>
              <div className="mt-4 space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="name">Name</Label>
                  <Input id="name" name="name" placeholder="Peer name" autoFocus />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="node_key">Node key <span className="text-muted-foreground">(optional)</span></Label>
                  <Input id="node_key" name="node_key" placeholder="Generated when empty" />
                </div>
                <div className="space-y-2">
                  <Label>Role</Label>
                  <Select value={role} onValueChange={(value) => setRole(value as CreateNodeInput["role"])}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="unknown">Unknown</SelectItem>
                      <SelectItem value="client">Client</SelectItem>
                      <SelectItem value="server">Server</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {error && (
                  <Alert variant="destructive">
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                )}
              </div>
              <DialogFooter className="mt-6">
                <Button type="button" variant="outline" onClick={closeDialog}>Cancel</Button>
                <Button disabled={submitting}>{submitting ? "Creating…" : "Confirm"}</Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => !open && setPendingDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete node?</AlertDialogTitle>
            <AlertDialogDescription>
              Delete node &quot;{pendingDelete?.name || pendingDelete?.node_key}&quot;? This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void confirmDelete()}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
