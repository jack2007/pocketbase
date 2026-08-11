import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { createApplyJob, listApplyJobs, listTemplates, type ApplyJob, type ConfigTemplate } from "../api";
import type { CenterNode } from "./Nodes";
import { PageHeader } from "@/components/shared/PageHeader";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

export function ApplyJobs({ nodes }: { nodes: CenterNode[] }) {
  const [jobs, setJobs] = useState<ApplyJob[]>([]);
  const [templates, setTemplates] = useState<ConfigTemplate[]>([]);
  const [templateId, setTemplateId] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);
  const template = templates.find((item) => item.id === templateId);
  const eligible = useMemo(
    () => nodes.filter((node) => !template || node.role === template.target_role),
    [nodes, template],
  );

  async function refresh() {
    try {
      const [nextJobs, nextTemplates] = await Promise.all([listApplyJobs(), listTemplates()]);
      setJobs(nextJobs);
      setTemplates(nextTemplates);
      setTemplateId((current) => current || nextTemplates[0]?.id || "");
    } catch (cause) {
      setError(message(cause));
    }
  }
  useEffect(() => { void refresh(); }, []);
  useEffect(() => { setSelected([]); }, [templateId]);

  async function start() {
    setStarting(true);
    setError("");
    try {
      await createApplyJob(templateId, selected);
      setSelected([]);
      toast.success("Apply job started");
      await refresh();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setStarting(false);
    }
  }

  function toggle(nodeId: string) {
    setSelected((current) => current.includes(nodeId)
      ? current.filter((id) => id !== nodeId)
      : [...current, nodeId]);
  }

  return (
    <section>
      <PageHeader
        eyebrow="Configuration"
        title="Apply jobs"
        actions={<Button variant="outline" onClick={() => void refresh()}>Refresh</Button>}
      />
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Card className="mb-4">
        <CardHeader>
          <CardTitle>Apply a template</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>Template</Label>
            <Select value={templateId} onValueChange={setTemplateId}>
              <SelectTrigger>
                <SelectValue placeholder="Select a template" />
              </SelectTrigger>
              <SelectContent>
                {templates.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.name} · {item.target_role} · v{item.version}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="max-h-64 space-y-2 overflow-auto">
            {eligible.map((node) => (
              <label
                key={node.id}
                className="flex cursor-pointer items-center gap-3 rounded-md border p-3"
              >
                <input
                  type="checkbox"
                  className="size-4"
                  checked={selected.includes(node.id)}
                  onChange={() => toggle(node.id)}
                />
                <span className="flex-1 text-sm font-medium">{node.name || node.node_key}</span>
                <StatusBadge online={node.online} />
              </label>
            ))}
            {template && eligible.length === 0 && (
              <p className="text-sm text-muted-foreground">No matching {template.target_role} nodes.</p>
            )}
          </div>
          <Button disabled={starting || !templateId || selected.length === 0} onClick={() => void start()}>
            {starting ? "Starting…" : `Apply to ${selected.length} node${selected.length === 1 ? "" : "s"}`}
          </Button>
        </CardContent>
      </Card>
      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Job</TableHead>
              <TableHead>Template</TableHead>
              <TableHead>Version</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Targets</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {jobs.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  No apply jobs yet.
                </TableCell>
              </TableRow>
            )}
            {jobs.map((job) => (
              <TableRow key={job.id}>
                <TableCell><code className="text-xs">{job.id}</code></TableCell>
                <TableCell>{templates.find((item) => item.id === job.template)?.name ?? job.template}</TableCell>
                <TableCell>v{job.template_version}</TableCell>
                <TableCell><Badge variant="secondary">{job.status}</Badge></TableCell>
                <TableCell className="text-sm">
                  {job.targets.map((target) => (
                    <div key={target.id}>
                      {nodeName(nodes, target.node)}: {target.status}
                      {target.error ? ` (${target.error})` : ""}
                    </div>
                  ))}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableShell>
    </section>
  );
}

function nodeName(nodes: CenterNode[], id: string) {
  const node = nodes.find((item) => item.id === id);
  return node?.name || node?.node_key || id;
}

function message(cause: unknown) {
  return cause instanceof Error ? cause.message : "Operation failed.";
}
