import { useEffect, useState } from "react";
import { toast } from "sonner";
import { proxyNode } from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { errorMessage, isObject, lines, readStringArray, type JsonObject } from "@/lib/node-utils";

export function AclTab({ node }: { node: CenterNode }) {
  const [allow, setAllow] = useState("");
  const [deny, setDeny] = useState("");
  const [denied, setDenied] = useState(0);
  const [status, setStatus] = useState("Waiting for refresh");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    if (node.role !== "server") return;
    setLoading(true);
    setError("");
    try {
      if (!node.online) {
        setStatus("Node offline");
        return;
      }
      const [config, metricsRaw] = await Promise.all([
        proxyNode<JsonObject>(node.node_key, "GET", "/api/v1/server/config"),
        proxyNode<JsonObject>(node.node_key, "GET", "/api/v1/metrics").catch((): JsonObject => ({})),
      ]);
      const metrics = metricsRaw;
      setAllow(readStringArray(config.allow_targets).join("\n"));
      setDeny(readStringArray(config.deny_targets).join("\n"));
      setDenied(typeof metrics.acl_denied === "number" ? metrics.acl_denied : 0);
      const path = typeof config.config_path === "string" ? config.config_path : "runtime state";
      setStatus(`loaded from ${path}`);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online, node.role]);

  async function save() {
    if (!node.online || node.role !== "server") return;
    setSaving(true);
    setError("");
    setStatus("saving");
    try {
      const payload = {
        allow_targets: lines(allow),
        deny_targets: lines(deny),
      };
      const config = await proxyNode<JsonObject>(
        node.node_key,
        "PATCH",
        "/api/v1/server/config",
        payload,
      );
      if (isObject(config)) {
        setAllow(readStringArray(config.allow_targets).join("\n"));
        setDeny(readStringArray(config.deny_targets).join("\n"));
      }
      setStatus("saved");
      toast.success("ACL saved");
    } catch (cause) {
      setError(errorMessage(cause));
      setStatus("save failed");
    } finally {
      setSaving(false);
    }
  }

  if (node.role !== "server") {
    return <p className="text-sm text-muted-foreground">ACL editing is only available for server nodes.</p>;
  }

  const allowList = lines(allow);
  const denyList = lines(deny);

  return (
    <div className="space-y-4">
      {!node.online && (
        <Alert>
          <AlertDescription>ACL is read-only while this node is offline.</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Edit allow_targets and deny_targets, then save to apply new ACL rules to subsequent tunnels.
        </p>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            Refresh
          </Button>
          <Button size="sm" disabled={!node.online || saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save ACL"}
          </Button>
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><CardTitle>allow_targets</CardTitle></CardHeader>
          <CardContent>
            <Label htmlFor="allow-targets" className="sr-only">Allow targets</Label>
            <Textarea
              id="allow-targets"
              className="min-h-40 font-mono text-xs"
              value={allow}
              readOnly={!node.online}
              onChange={(event) => setAllow(event.target.value)}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>deny_targets</CardTitle></CardHeader>
          <CardContent>
            <Label htmlFor="deny-targets" className="sr-only">Deny targets</Label>
            <Textarea
              id="deny-targets"
              className="min-h-40 font-mono text-xs"
              value={deny}
              readOnly={!node.online}
              onChange={(event) => setDeny(event.target.value)}
            />
          </CardContent>
        </Card>
      </div>
      <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
        <DataTableShell>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>type</TableHead>
                <TableHead>targets</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>allow</TableCell>
                <TableCell>{allowList.join(", ") || "—"}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>deny</TableCell>
                <TableCell>{denyList.join(", ") || "—"}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </DataTableShell>
        <Card>
          <CardHeader><CardTitle>Status</CardTitle></CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div>
              <div className="text-muted-foreground">acl_denied</div>
              <div className="text-2xl font-semibold">{denied}</div>
            </div>
            <p className="text-muted-foreground">{status}</p>
            <p className="text-xs text-muted-foreground">
              One CIDR per line, or paste a comma-separated list. An empty allow_targets list rejects all regular targets.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
