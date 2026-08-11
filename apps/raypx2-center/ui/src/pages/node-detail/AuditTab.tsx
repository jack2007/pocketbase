import { useEffect, useState } from "react";
import { listAuditLogs, type AuditLog } from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { display, errorMessage, formatDate } from "@/lib/node-utils";

export function AuditTab({ node }: { node: CenterNode }) {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    setLoading(true);
    listAuditLogs(node.id)
      .then(setLogs)
      .catch((cause) => setError(errorMessage(cause)))
      .finally(() => setLoading(false));
  }, [node.id]);

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Request</TableHead>
              <TableHead>IP</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {logs.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="h-20 text-center text-muted-foreground">
                  {loading ? "Loading…" : "No items returned."}
                </TableCell>
              </TableRow>
            )}
            {logs.map((log) => (
              <TableRow key={log.id}>
                <TableCell>{formatDate(log.created)}</TableCell>
                <TableCell><Badge variant="secondary">{log.action}</Badge></TableCell>
                <TableCell><code className="text-xs">{summary(log.request_summary)}</code></TableCell>
                <TableCell>{log.ip || "—"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableShell>
    </div>
  );
}

function summary(value?: Record<string, unknown>): string {
  if (!value) return "—";
  const method = display(value.method);
  const path = display(value.path);
  const status = display(value.status);
  if (method !== "—" || path !== "—") return `${method} ${path} → ${status}`;
  return JSON.stringify(value);
}
