import { useEffect, useState } from "react";
import { toast } from "sonner";
import {
  createTemplate,
  deleteTemplate,
  listTemplates,
  updateTemplate,
  type ConfigTemplate,
} from "../api";
import { PageHeader } from "@/components/shared/PageHeader";
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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Textarea } from "@/components/ui/textarea";

const serverExample = JSON.stringify({
  allow_targets: ["10.0.0.0/8"],
  deny_targets: ["169.254.0.0/16"],
}, null, 2);

export function Templates() {
  const [templates, setTemplates] = useState<ConfigTemplate[]>([]);
  const [editing, setEditing] = useState<ConfigTemplate>();
  const [name, setName] = useState("");
  const [role, setRole] = useState<"client" | "server">("server");
  const [body, setBody] = useState(serverExample);
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<ConfigTemplate>();

  async function refresh() {
    try {
      setTemplates(await listTemplates());
    } catch (cause) {
      setError(message(cause));
    }
  }

  useEffect(() => { void refresh(); }, []);

  function edit(template?: ConfigTemplate) {
    setEditing(template);
    setName(template?.name ?? "");
    setRole(template?.target_role ?? "server");
    setBody(JSON.stringify(template?.body ?? JSON.parse(serverExample), null, 2));
    setNotes(template?.notes ?? "");
    setError("");
  }

  async function save() {
    setSaving(true);
    setError("");
    try {
      const parsed = JSON.parse(body) as Record<string, unknown>;
      const input = { name, target_role: role, body: parsed, notes };
      if (editing) await updateTemplate(editing.id, input);
      else await createTemplate(input);
      toast.success(editing ? "Template version saved" : "Template created");
      edit();
      await refresh();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setSaving(false);
    }
  }

  async function confirmDelete() {
    if (!pendingDelete) return;
    try {
      await deleteTemplate(pendingDelete.id);
      toast.success("Template deleted");
      setPendingDelete(undefined);
      await refresh();
    } catch (cause) {
      setError(message(cause));
    }
  }

  return (
    <section>
      <PageHeader
        eyebrow="Configuration"
        title="Templates"
        actions={<Button variant="outline" onClick={() => edit()}>New template</Button>}
      />
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
        <DataTableShell>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Version</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {templates.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                    No templates yet.
                  </TableCell>
                </TableRow>
              )}
              {templates.map((template) => (
                <TableRow key={template.id}>
                  <TableCell className="font-medium">{template.name}</TableCell>
                  <TableCell><Badge variant="secondary">{template.target_role}</Badge></TableCell>
                  <TableCell>v{template.version}</TableCell>
                  <TableCell className="space-x-2">
                    <Button variant="outline" size="sm" onClick={() => edit(template)}>Edit</Button>
                    <Button variant="outline" size="sm" onClick={() => setPendingDelete(template)}>Delete</Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataTableShell>
        <Card>
          <CardHeader>
            <CardTitle>{editing ? `Edit ${editing.name}` : "New template"}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="template-name">Name</Label>
              <Input id="template-name" value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>Target role</Label>
              <Select value={role} onValueChange={(value) => setRole(value as "client" | "server")}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="server">Server</SelectItem>
                  <SelectItem value="client">Client</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="template-body">Template body</Label>
              <Textarea
                id="template-body"
                className="min-h-48 font-mono text-xs"
                value={body}
                onChange={(event) => setBody(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="template-notes">Notes</Label>
              <Textarea id="template-notes" value={notes} onChange={(event) => setNotes(event.target.value)} />
            </div>
            <Button disabled={saving || !name.trim()} onClick={() => void save()}>
              {saving ? "Saving…" : editing ? "Save new version" : "Create template"}
            </Button>
          </CardContent>
        </Card>
      </div>

      <AlertDialog open={Boolean(pendingDelete)} onOpenChange={(open) => !open && setPendingDelete(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete template?</AlertDialogTitle>
            <AlertDialogDescription>
              Delete template &quot;{pendingDelete?.name}&quot;?
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

function message(cause: unknown) {
  return cause instanceof Error ? cause.message : "Operation failed.";
}
