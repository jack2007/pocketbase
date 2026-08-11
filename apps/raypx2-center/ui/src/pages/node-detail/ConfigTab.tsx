import { useEffect, useState } from "react";
import {
  getNodeConfig,
  putNodeConfig,
  type ConfigRevision,
} from "../../api";
import type { CenterNode } from "../Nodes";
import { DataTableShell } from "@/components/shared/DataTableShell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
import { errorMessage, formatDate, isObject, itemId, type JsonObject } from "@/lib/node-utils";
import {
  addDraftPeer,
  addPeerPortForward,
  connectionMetadata,
  displayInput,
  isNodeOfflineError,
  jsonSnapshot,
  parseJsonObject,
  readPath,
  refreshWarningMessage,
  removeDraftPeer,
  removePeerPortForward,
  setPath,
  stableStringify,
  stripDraftPeerMarkers,
  updatePeer,
  updatePeerPortForward,
  validateConfig,
} from "./config-helpers";

export function ConfigTab({
  node,
  onDirtyChange,
}: {
  node: CenterNode;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [mode, setMode] = useState<"form" | "json">("form");
  const [draft, setDraft] = useState<JsonObject>({});
  const [jsonText, setJsonText] = useState("{}");
  const [baseline, setBaseline] = useState("");
  const [role, setRole] = useState<string>(node.role);
  const [online, setOnline] = useState(node.online);
  const [liveMeta, setLiveMeta] = useState<JsonObject>({});
  const [revisions, setRevisions] = useState<ConfigRevision[]>([]);
  const [ignored, setIgnored] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const currentSnapshot = mode === "json" ? jsonSnapshot(jsonText) : stableStringify(draft);
  const dirty = baseline !== "" && currentSnapshot !== baseline;
  const readOnly = !online || (role !== "client" && role !== "server");

  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);

  async function load() {
    setLoading(true);
    setError("");
    try {
      const response = await getNodeConfig(node.node_key);
      const nextDraft = response.editor_draft || {};
      setDraft(nextDraft);
      setJsonText(JSON.stringify(nextDraft, null, 2));
      setBaseline(stableStringify(nextDraft));
      setRole(response.role);
      setOnline(response.online);
      setLiveMeta(connectionMetadata(response));
      setRevisions(response.recent_revisions || []);
      setIgnored([]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.node_key]);

  function switchMode(nextMode: "form" | "json") {
    if (nextMode === mode) return;
    setError("");
    if (nextMode === "json") {
      setJsonText(JSON.stringify(draft, null, 2));
      setMode("json");
      return;
    }
    const parsed = parseJsonObject(jsonText);
    if (!parsed) {
      setError("Configuration must be a valid JSON object before switching to Form.");
      return;
    }
    setDraft(parsed);
    setMode("form");
  }

  function commitApplied(result: { applied: JsonObject; ignored_fields?: string[] }) {
    const applied = result.applied;
    setDraft(applied);
    setJsonText(JSON.stringify(applied, null, 2));
    setBaseline(stableStringify(applied));
    setIgnored(result.ignored_fields || []);
  }

  async function save() {
    let content = draft;
    if (mode === "json") {
      const parsed = parseJsonObject(jsonText);
      if (!parsed) {
        setError("Configuration must be a valid JSON object before saving.");
        return;
      }
      content = parsed;
      setDraft(parsed);
    }
    content = stripDraftPeerMarkers(content);
    const validationError = validateConfig(role, content);
    if (validationError) {
      setError(validationError);
      return;
    }
    if (readOnly) return;
    setSaving(true);
    setError("");
    setIgnored([]);
    try {
      const result = await putNodeConfig(node.node_key, content);
      commitApplied(result);
      try {
        const response = await getNodeConfig(node.node_key);
        const nextDraft = response.editor_draft || {};
        setDraft(nextDraft);
        setJsonText(JSON.stringify(nextDraft, null, 2));
        setBaseline(stableStringify(nextDraft));
        setRole(response.role);
        setOnline(response.online);
        setLiveMeta(connectionMetadata(response));
        setRevisions(response.recent_revisions || []);
      } catch (refreshCause) {
        setError(refreshWarningMessage(refreshCause));
      }
    } catch (cause) {
      if (isNodeOfflineError(cause)) {
        setOnline(false);
        setError("Node is offline. Your unsaved configuration draft has been preserved.");
      } else {
        setError(errorMessage(cause));
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {!online && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-200">
          Configuration is read-only while this node is offline.
        </div>
      )}
      {online && role !== "client" && role !== "server" && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-200">
          Configuration is read-only for unsupported node role “{role}”.
        </div>
      )}
      {ignored.length > 0 && (
        <div className="rounded-md border bg-muted/40 px-3 py-2 text-sm">
          <strong>Ignored fields:</strong>{" "}
          {ignored.map((field) => <code key={field} className="mx-1">{field}</code>)}
        </div>
      )}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle>Configuration editor</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex gap-1" aria-label="Editor mode">
              <Button
                variant="outline"
                size="sm"
                aria-pressed={mode === "form"}
                onClick={() => switchMode("form")}
              >
                Form
              </Button>
              <Button
                variant="outline"
                size="sm"
                aria-pressed={mode === "json"}
                onClick={() => switchMode("json")}
              >
                JSON
              </Button>
            </div>
            <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
              Refresh
            </Button>
            <Button disabled={readOnly || loading || saving} onClick={() => void save()}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : mode === "json" ? (
            <div className="space-y-2">
              <Label htmlFor="config-json">JSON configuration</Label>
              <Textarea
                id="config-json"
                className="min-h-80 font-mono text-xs"
                value={jsonText}
                readOnly={readOnly}
                onChange={(event) => setJsonText(event.target.value)}
              />
            </div>
          ) : (
            <ConfigForm role={role} draft={draft} readOnly={readOnly} onChange={setDraft} />
          )}
          {Object.keys(liveMeta).length > 0 && (
            <div className="flex flex-wrap justify-between gap-2 border-t pt-4 text-sm">
              <strong>Restart required: {liveMeta.restart_required === true ? "Yes" : "No"}</strong>
              <span className="text-muted-foreground">
                Pending fields:{" "}
                {(Array.isArray(liveMeta.pending_fields)
                  ? liveMeta.pending_fields.filter((item): item is string => typeof item === "string")
                  : []
                ).join(", ") || "None"}
              </span>
            </div>
          )}
        </CardContent>
      </Card>
      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Kind</TableHead>
              <TableHead>Source</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {revisions.length === 0 && (
              <TableRow>
                <TableCell colSpan={3} className="h-16 text-center text-muted-foreground">
                  {loading ? "Loading…" : "No items returned."}
                </TableCell>
              </TableRow>
            )}
            {revisions.map((revision) => (
              <TableRow key={revision.id}>
                <TableCell>{formatDate(revision.created)}</TableCell>
                <TableCell><Badge variant="secondary">{revision.kind}</Badge></TableCell>
                <TableCell>{revision.source}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataTableShell>
    </div>
  );
}

function ConfigForm({
  role,
  draft,
  readOnly,
  onChange,
}: {
  role: string;
  draft: JsonObject;
  readOnly: boolean;
  onChange: (draft: JsonObject) => void;
}) {
  if (role === "server") {
    return (
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Server ACL targets are edited on the ACL tab. Use Form for live-safe connection fields, or JSON for the full draft.
        </p>
        <div className="space-y-2">
          <Label htmlFor="compression-level">Compression level</Label>
          <Input
            id="compression-level"
            type="number"
            min={1}
            max={22}
            step={1}
            value={displayInput(readPath(draft, ["connection", "compression", "level"]))}
            readOnly={readOnly}
            onChange={(event) => onChange(setPath(
              draft,
              ["connection", "compression", "level"],
              event.target.value === "" ? "" : Number(event.target.value),
            ))}
          />
        </div>
      </div>
    );
  }

  if (role === "client") {
    const peers = Array.isArray(draft.peers) ? draft.peers.filter(isObject) : [];
    return (
      <div className="space-y-4">
        {peers.length === 0 && (
          <p className="text-sm text-muted-foreground">No peers in the draft. Use Add peer to create one.</p>
        )}
        {peers.map((peer, index) => {
          const isDraftNew = peer._draft_new === true;
          return (
            <fieldset key={itemId(peer) || `draft-${index}`} className="grid gap-3 rounded-md border p-4 sm:grid-cols-2">
              <legend className="px-1 text-sm font-semibold">Peer {index + 1}</legend>
              <div className="space-y-2">
                <Label htmlFor={`peer-id-${index}`}>Peer ID</Label>
                <Input
                  id={`peer-id-${index}`}
                  value={displayInput(peer.peer_id)}
                  readOnly={readOnly || !isDraftNew}
                  onChange={(event) => onChange(updatePeer(draft, index, "peer_id", event.target.value))}
                />
              </div>
              {[
                ["client_name", "Client name"],
                ["quic_peer", "QUIC peer"],
                ["socks_listen", "SOCKS listen"],
                ["http_listen", "HTTP listen"],
              ].map(([key, label]) => (
                <div className="space-y-2" key={key}>
                  <Label htmlFor={`${key}-${index}`}>{label}</Label>
                  <Input
                    id={`${key}-${index}`}
                    value={displayInput(peer[key])}
                    readOnly={readOnly}
                    onChange={(event) => onChange(updatePeer(draft, index, key, event.target.value))}
                  />
                </div>
              ))}
              <div className="space-y-2">
                <Label htmlFor={`quic-connections-${index}`}>QUIC connections</Label>
                <Input
                  id={`quic-connections-${index}`}
                  type="number"
                  min={0}
                  step={1}
                  value={displayInput(peer.quic_connections)}
                  readOnly={readOnly}
                  onChange={(event) => onChange(updatePeer(
                    draft,
                    index,
                    "quic_connections",
                    event.target.value === "" ? "" : Number(event.target.value),
                  ))}
                />
              </div>
              <div className="space-y-3 sm:col-span-2">
                <strong className="text-sm">Port forwards</strong>
                {(Array.isArray(peer.port_forwards) ? peer.port_forwards.filter(isObject) : []).map((forward, forwardIndex) => (
                  <fieldset key={forwardIndex} className="grid gap-3 rounded-md border p-3 sm:grid-cols-2">
                    <legend className="px-1 text-xs font-medium">Port forward {forwardIndex + 1}</legend>
                    <div className="space-y-2">
                      <Label htmlFor={`pf-listen-${index}-${forwardIndex}`}>
                        Port forward {forwardIndex + 1} listen
                      </Label>
                      <Input
                        id={`pf-listen-${index}-${forwardIndex}`}
                        value={displayInput(forward.listen)}
                        readOnly={readOnly}
                        onChange={(event) => onChange(updatePeerPortForward(
                          draft, index, forwardIndex, "listen", event.target.value,
                        ))}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`pf-target-${index}-${forwardIndex}`}>
                        Port forward {forwardIndex + 1} target
                      </Label>
                      <Input
                        id={`pf-target-${index}-${forwardIndex}`}
                        value={displayInput(forward.target)}
                        readOnly={readOnly}
                        onChange={(event) => onChange(updatePeerPortForward(
                          draft, index, forwardIndex, "target", event.target.value,
                        ))}
                      />
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={readOnly}
                      onClick={() => onChange(removePeerPortForward(draft, index, forwardIndex))}
                    >
                      Remove port forward {forwardIndex + 1}
                    </Button>
                  </fieldset>
                ))}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={readOnly}
                  onClick={() => onChange(addPeerPortForward(draft, index))}
                >
                  Add port forward
                </Button>
              </div>
              <label className="flex items-center gap-2 text-sm sm:col-span-2">
                <input
                  type="checkbox"
                  checked={peer.enabled !== false}
                  disabled={readOnly}
                  onChange={(event) => onChange(updatePeer(draft, index, "enabled", event.target.checked))}
                />
                Enabled
              </label>
              {isDraftNew && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={readOnly}
                  onClick={() => onChange(removeDraftPeer(draft, index))}
                >
                  Remove from draft
                </Button>
              )}
            </fieldset>
          );
        })}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={readOnly}
          onClick={() => onChange(addDraftPeer(draft))}
        >
          Add peer
        </Button>
        <p className="text-sm text-muted-foreground">
          To delete a saved peer from the node, use the Peers tab.
        </p>
      </div>
    );
  }

  return <p className="text-sm text-muted-foreground">No editable fields are available for this node role.</p>;
}
