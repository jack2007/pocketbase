import { useEffect, useState } from "react";
import {
  getNodeConfig,
  listAuditLogs,
  putNodeConfig,
  proxyNode,
  type AuditLog,
  type ConfigRevision,
  type NodeConfigResponse,
} from "../api";
import type { CenterNode } from "./Nodes";

type Tab = "overview" | "ops" | "tunnels" | "config" | "audit";
type JsonObject = Record<string, unknown>;

interface NodeDetailProps {
  node: CenterNode;
  onBack: () => void;
}

export function NodeDetail({ node, onBack }: NodeDetailProps) {
  const [tab, setTab] = useState<Tab>("overview");
  const [configDirty, setConfigDirty] = useState(false);

  function leaveConfig(action: () => void) {
    if (tab === "config" && configDirty && !window.confirm("Discard unsaved configuration changes?")) return;
    action();
  }

  return (
    <section>
      <button className="back-button" onClick={() => leaveConfig(onBack)}>← Back to nodes</button>
      <div className="page-heading node-heading">
        <div>
          <p className="eyebrow">Node detail</p>
          <h2>{node.name || "Unnamed node"}</h2>
          <p className="muted"><code>{node.node_key}</code> · {node.role}</p>
        </div>
        <span className={`status ${node.online ? "online" : "offline"}`}>
          <span className="status-dot" />
          {node.online ? "Online" : "Offline"}
        </span>
      </div>

      <div className="tabs" role="tablist" aria-label="Node detail">
        {(["overview", "ops", "tunnels", "config", "audit"] as Tab[]).map((item) => (
          <button
            key={item}
            role="tab"
            aria-selected={tab === item}
            className={tab === item ? "active" : ""}
            onClick={() => {
              if (item !== tab) leaveConfig(() => setTab(item));
            }}
          >
            {title(item)}
          </button>
        ))}
      </div>

      {tab === "overview" && <NodeOverview node={node} />}
      {tab === "ops" && <NodeOps node={node} />}
      {tab === "tunnels" && <NodeTunnels node={node} />}
      {tab === "config" && <NodeConfig node={node} onDirtyChange={setConfigDirty} />}
      {tab === "audit" && <NodeAudit node={node} />}
    </section>
  );
}

function NodeConfig({
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
    <div className="ops-stack">
      {error && <div className="alert" role="alert">{error}</div>}
      {!online && <div className="offline-notice">Configuration is read-only while this node is offline.</div>}
      {online && role !== "client" && role !== "server" && (
        <div className="offline-notice">Configuration is read-only for unsupported node role “{role}”.</div>
      )}
      {ignored.length > 0 && (
        <div className="offline-notice">
          <strong>Ignored fields:</strong> {ignored.map((field) => <code key={field}>{field}</code>)}
        </div>
      )}
      <div className="panel ops-panel">
        <PanelHeading title="Configuration editor" onRefresh={() => void load()} loading={loading} />
        <div className="config-toolbar">
          <div className="mode-switch" aria-label="Editor mode">
            <button
              className="button secondary small"
              aria-pressed={mode === "form"}
              onClick={() => switchMode("form")}
            >
              Form
            </button>
            <button
              className="button secondary small"
              aria-pressed={mode === "json"}
              onClick={() => switchMode("json")}
            >
              JSON
            </button>
          </div>
          <button className="button" disabled={readOnly || loading || saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
        {loading
          ? <p className="muted">Loading…</p>
          : mode === "json"
            ? (
              <label className="config-json-label">
                JSON configuration
                <textarea
                  className="code-input config-json"
                  value={jsonText}
                  readOnly={readOnly}
                  onChange={(event) => setJsonText(event.target.value)}
                />
              </label>
            )
            : (
              <ConfigForm
                role={role}
                draft={draft}
                readOnly={readOnly}
                onChange={setDraft}
              />
            )}
        {Object.keys(liveMeta).length > 0 && (
          <div className="config-live-meta">
            <strong>Restart required: {liveMeta.restart_required === true ? "Yes" : "No"}</strong>
            <span className="muted">
              Pending fields: {readStringArray(liveMeta.pending_fields).join(", ") || "None"}
            </span>
          </div>
        )}
      </div>
      <div className="table-shell">
        <table>
          <thead><tr><th>Time</th><th>Kind</th><th>Source</th></tr></thead>
          <tbody>
            {revisions.length === 0 && <EmptyRow columns={3} loading={loading} />}
            {revisions.map((revision) => (
              <tr key={revision.id}>
                <td>{formatDate(revision.created)}</td>
                <td><span className="tag">{revision.kind}</span></td>
                <td>{revision.source}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
      <div className="config-form">
        <div className="acl-grid">
          <label>
            Allow targets
            <textarea
              value={readStringArray(draft.allow_targets).join("\n")}
              readOnly={readOnly}
              onChange={(event) => onChange({ ...draft, allow_targets: lines(event.target.value) })}
            />
          </label>
          <label>
            Deny targets
            <textarea
              value={readStringArray(draft.deny_targets).join("\n")}
              readOnly={readOnly}
              onChange={(event) => onChange({ ...draft, deny_targets: lines(event.target.value) })}
            />
          </label>
        </div>
        <label>
          Compression level
          <input
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
        </label>
      </div>
    );
  }

  if (role === "client") {
    const peers = Array.isArray(draft.peers) ? draft.peers.filter(isObject) : [];
    return (
      <div className="config-form peer-list">
        {peers.length === 0 && <p className="muted">No peers in the draft. Use Add peer to create one.</p>}
        {peers.map((peer, index) => {
          const isDraftNew = peer._draft_new === true;
          return (
            <fieldset className="peer-editor" key={itemId(peer) || `draft-${index}`}>
              <legend>Peer {index + 1}</legend>
              <label>
                Peer ID
                <input
                  value={displayInput(peer.peer_id)}
                  readOnly={readOnly || !isDraftNew}
                  onChange={(event) => onChange(updatePeer(draft, index, "peer_id", event.target.value))}
                />
              </label>
              {[
                ["client_name", "Client name"],
                ["quic_peer", "QUIC peer"],
                ["socks_listen", "SOCKS listen"],
                ["http_listen", "HTTP listen"],
              ].map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    value={displayInput(peer[key])}
                    readOnly={readOnly}
                    onChange={(event) => onChange(updatePeer(draft, index, key, event.target.value))}
                  />
                </label>
              ))}
              <label>
                QUIC connections
                <input
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
              </label>
              <div className="port-forward-list">
                <strong>Port forwards</strong>
                {(Array.isArray(peer.port_forwards) ? peer.port_forwards.filter(isObject) : []).map((forward, forwardIndex) => (
                  <fieldset key={forwardIndex}>
                    <legend>Port forward {forwardIndex + 1}</legend>
                    <label>
                      Port forward {forwardIndex + 1} listen
                      <input
                        value={displayInput(forward.listen)}
                        readOnly={readOnly}
                        onChange={(event) => onChange(updatePeerPortForward(
                          draft,
                          index,
                          forwardIndex,
                          "listen",
                          event.target.value,
                        ))}
                      />
                    </label>
                    <label>
                      Port forward {forwardIndex + 1} target
                      <input
                        value={displayInput(forward.target)}
                        readOnly={readOnly}
                        onChange={(event) => onChange(updatePeerPortForward(
                          draft,
                          index,
                          forwardIndex,
                          "target",
                          event.target.value,
                        ))}
                      />
                    </label>
                    <button
                      type="button"
                      className="button secondary small"
                      disabled={readOnly}
                      onClick={() => onChange(removePeerPortForward(draft, index, forwardIndex))}
                    >
                      Remove port forward {forwardIndex + 1}
                    </button>
                  </fieldset>
                ))}
                <button
                  type="button"
                  className="button secondary small"
                  disabled={readOnly}
                  onClick={() => onChange(addPeerPortForward(draft, index))}
                >
                  Add port forward
                </button>
              </div>
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={peer.enabled !== false}
                  disabled={readOnly}
                  onChange={(event) => onChange(updatePeer(draft, index, "enabled", event.target.checked))}
                />
                Enabled
              </label>
              {isDraftNew && (
                <button
                  type="button"
                  className="button secondary small"
                  disabled={readOnly}
                  onClick={() => onChange(removeDraftPeer(draft, index))}
                >
                  Remove from draft
                </button>
              )}
            </fieldset>
          );
        })}
        <button
          type="button"
          className="button secondary small"
          disabled={readOnly}
          onClick={() => onChange(addDraftPeer(draft))}
        >
          Add peer
        </button>
        <p className="muted">To delete a saved peer from the node, use the Ops tab.</p>
      </div>
    );
  }

  return <p className="muted">No editable fields are available for this node role.</p>;
}

function NodeOverview({ node }: { node: CenterNode }) {
  return (
    <div className="stats detail-stats">
      <Summary label="Role" value={node.role} />
      <Summary label="Health" value={node.health_status || "Unknown"} />
      <Summary label="Last seen" value={formatDate(node.last_seen_at)} />
    </div>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return <div className="stat-card"><span>{label}</span><strong className="summary-value">{value}</strong></div>;
}

function NodeOps({ node }: { node: CenterNode }) {
  const [health, setHealth] = useState<unknown>();
  const [peers, setPeers] = useState<JsonObject[]>([]);
  const [connections, setConnections] = useState<JsonObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      if (node.role === "server") {
        const [healthResult, connectionsResult] = await Promise.all([
          proxyNode(node.node_key, "GET", "/api/v1/health"),
          proxyNode(node.node_key, "GET", "/api/v1/server/connections"),
        ]);
        setHealth(healthResult);
        setConnections(itemsFrom(connectionsResult, "connections"));
      } else if (node.role === "client") {
        const [healthResult, peersResult] = await Promise.all([
          proxyNode(node.node_key, "GET", "/api/v1/health"),
          proxyNode(node.node_key, "GET", "/api/v1/peers"),
        ]);
        const nextPeers = itemsFrom(peersResult, "peers");
        const connectionGroups = await Promise.all(nextPeers.map(async (peer) => {
          const peerId = itemId(peer);
          if (!peerId) return [];
          const result = await proxyNode(
            node.node_key,
            "GET",
            `/api/v1/peers/${encodeURIComponent(peerId)}/connections`,
          );
          return itemsFrom(result, "connections").map((connection) => ({ ...connection, peer_id: peerId }));
        }));
        setHealth(healthResult);
        setPeers(nextPeers);
        setConnections(connectionGroups.flat());
      } else {
        setHealth(await proxyNode(node.node_key, "GET", "/api/v1/health"));
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [node.id]);

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

  return (
    <div className="ops-stack">
      {!node.online && <div className="offline-notice">Writes are disabled while this node is offline.</div>}
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="panel ops-panel">
        <PanelHeading title="Health" onRefresh={() => void load()} loading={loading} />
        <JsonView value={health} empty={loading ? "Loading…" : "No health response."} />
      </div>

      {node.role === "client" && (
        <>
          <div className="panel ops-panel">
            <PanelHeading title="Client peers" />
            <div className="table-shell compact">
              <table>
                <thead><tr><th>Peer</th><th>State</th><th>Target</th><th>Action</th></tr></thead>
                <tbody>
                  {peers.length === 0 && <EmptyRow columns={4} loading={loading} />}
                  {peers.map((peer, index) => (
                    <tr key={itemId(peer) || index}>
                      <td className="strong">{itemId(peer) || `Peer ${index + 1}`}</td>
                      <td>{display(peer.state ?? peer.enabled)}</td>
                      <td>{display(peer.address ?? peer.endpoint ?? peer.host)}</td>
                      <td>
                        <button
                          className="button secondary small"
                          disabled={!node.online || writing || !itemId(peer)}
                          onClick={() => void togglePeer(peer)}
                        >
                          {peer.enabled === false || peer.state === "disabled" ? "Enable" : "Disable"}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <ConnectionsTable connections={connections} loading={loading} />
        </>
      )}

      {node.role === "server" && (
        <ConnectionsTable connections={connections} loading={loading} />
      )}
    </div>
  );
}

const CLIENT_TUNNEL_COLUMNS = [
  "tunnel_id",
  "peer_id",
  "connection_id",
  "target",
  "state",
  "role",
  "ingress",
  "compress",
  "created_at",
  "duration_ms",
  "tcp_read_bytes",
  "tcp_write_bytes",
  "pending_bytes",
  "relay_backend",
  "worker_index",
  "last_error",
] as const;

const SERVER_TUNNEL_COLUMNS = [
  "tunnel_id",
  "peer_id",
  "connection_id",
  "state",
  "target",
  "role",
  "duration_ms",
  "active",
] as const;

function NodeTunnels({ node }: { node: CenterNode }) {
  const [tunnels, setTunnels] = useState<JsonObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const columns =
    node.role === "client"
      ? CLIENT_TUNNEL_COLUMNS
      : node.role === "server"
        ? SERVER_TUNNEL_COLUMNS
        : null;

  async function load() {
    if (!node.online || !columns) {
      setTunnels([]);
      setError("");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const path =
        node.role === "server" ? "/api/v1/server/tunnels" : "/api/v1/tunnels";
      const result = await proxyNode(node.node_key, "GET", path);
      setTunnels(itemsFrom(result, "tunnels"));
    } catch (cause) {
      setError(errorMessage(cause));
      // Keep previous tunnels on failure (same as Ops keeping prior health/peers).
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [node.id, node.role, node.online]);

  if (!node.online) {
    return (
      <div className="ops-stack">
        <div className="offline-notice">Tunnels are unavailable while this node is offline.</div>
      </div>
    );
  }

  if (!columns) {
    return (
      <div className="ops-stack">
        <p className="muted">Tunnel inventory is not available for role "{node.role}".</p>
      </div>
    );
  }

  return (
    <div className="ops-stack">
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="panel ops-panel">
        <PanelHeading title="Tunnels" onRefresh={() => void load()} loading={loading} />
        <div className="table-shell compact">
          <table>
            <thead>
              <tr>
                {columns.map((column) => (
                  <th key={column}>{column}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {tunnels.length === 0 && (
                <EmptyRow columns={columns.length} loading={loading} />
              )}
              {tunnels.map((tunnel, index) => {
                const key =
                  typeof tunnel.tunnel_id === "string" && tunnel.tunnel_id
                    ? tunnel.tunnel_id
                    : String(index);
                return (
                  <tr key={key}>
                    {columns.map((column) => (
                      <td key={column} className={column === "tunnel_id" ? "strong" : undefined}>
                        {display(tunnel[column])}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function PanelHeading({ title: heading, onRefresh, loading }: { title: string; onRefresh?: () => void; loading?: boolean }) {
  return (
    <div className="panel-heading">
      <h3>{heading}</h3>
      {onRefresh && <button className="button secondary small" disabled={loading} onClick={onRefresh}>Refresh</button>}
    </div>
  );
}

function ConnectionsTable({ connections, loading }: { connections: JsonObject[]; loading: boolean }) {
  return (
    <div className="panel ops-panel">
      <PanelHeading title="Connections" />
      <div className="table-shell compact">
        <table>
          <thead><tr><th>Connection</th><th>Peer / client</th><th>State</th><th>Remote</th></tr></thead>
          <tbody>
            {connections.length === 0 && <EmptyRow columns={4} loading={loading} />}
            {connections.map((connection, index) => (
              <tr key={itemId(connection) || index}>
                <td className="strong">{itemId(connection) || `Connection ${index + 1}`}</td>
                <td>{display(connection.peer_id ?? connection.client_name)}</td>
                <td>{display(connection.state ?? connection.status)}</td>
                <td>{display(connection.remote_address ?? connection.remote)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function EmptyRow({ columns, loading }: { columns: number; loading: boolean }) {
  return <tr><td className="empty" colSpan={columns}>{loading ? "Loading…" : "No items returned."}</td></tr>;
}

function NodeAudit({ node }: { node: CenterNode }) {
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
    <>
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="table-shell">
        <table>
          <thead><tr><th>Time</th><th>Action</th><th>Request</th><th>IP</th></tr></thead>
          <tbody>
            {logs.length === 0 && <EmptyRow columns={4} loading={loading} />}
            {logs.map((log) => (
              <tr key={log.id}>
                <td>{formatDate(log.created)}</td>
                <td><span className="tag">{log.action}</span></td>
                <td><code>{summary(log.request_summary)}</code></td>
                <td>{log.ip || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function JsonView({ value, empty = "No data." }: { value: unknown; empty?: string }) {
  if (value === undefined) return <p className="muted">{empty}</p>;
  return <pre className="json-view">{JSON.stringify(value, null, 2)}</pre>;
}

function connectionMetadata(response: NodeConfigResponse): JsonObject {
  if (!response.live || !isObject(response.live.connection_config)) return {};
  return response.live.connection_config;
}

function parseJsonObject(value: string): JsonObject | null {
  try {
    const parsed: unknown = JSON.parse(value);
    return isObject(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function jsonSnapshot(value: string): string {
  const parsed = parseJsonObject(value);
  return parsed ? stableStringify(parsed) : `invalid:${value}`;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(sortJson(value));
}

function sortJson(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortJson);
  if (!isObject(value)) return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .filter((key) => value[key] !== undefined)
      .map((key) => [key, sortJson(value[key])]),
  );
}

function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function readPath(value: JsonObject, path: string[]): unknown {
  let current: unknown = value;
  for (const key of path) {
    if (!isObject(current)) return undefined;
    current = current[key];
  }
  return current;
}

function setPath(value: JsonObject, path: string[], next: unknown): JsonObject {
  const [key, ...rest] = path;
  if (!key) return value;
  return {
    ...value,
    [key]: rest.length === 0
      ? next
      : setPath(isObject(value[key]) ? value[key] : {}, rest, next),
  };
}

function updatePeer(draft: JsonObject, index: number, key: string, value: unknown): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[index]) ? peers[index] : {};
  peers[index] = { ...peer, [key]: value };
  return { ...draft, peers };
}

function addDraftPeer(draft: JsonObject): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  peers.push({
    _draft_new: true,
    peer_id: "",
    client_name: "",
    quic_peer: "",
    socks_listen: "",
    http_listen: "",
    quic_connections: 1,
    port_forwards: [],
    enabled: true,
  });
  return { ...draft, peers };
}

function removeDraftPeer(draft: JsonObject, index: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[index]) ? peers[index] : null;
  if (!peer || peer._draft_new !== true) return draft;
  peers.splice(index, 1);
  return { ...draft, peers };
}

function stripDraftPeerMarkers(content: JsonObject): JsonObject {
  if (!Array.isArray(content.peers)) return content;
  return {
    ...content,
    peers: content.peers.map((raw) => {
      if (!isObject(raw)) return raw;
      const { _draft_new: _ignored, ...peer } = raw;
      return peer;
    }),
  };
}

function updatePeerPortForward(
  draft: JsonObject,
  peerIndex: number,
  forwardIndex: number,
  key: "listen" | "target",
  value: string,
): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  const forward = isObject(forwards[forwardIndex]) ? forwards[forwardIndex] : {};
  forwards[forwardIndex] = { ...forward, [key]: value };
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

function addPeerPortForward(draft: JsonObject, peerIndex: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  forwards.push({ listen: "", target: "" });
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

function removePeerPortForward(draft: JsonObject, peerIndex: number, forwardIndex: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  forwards.splice(forwardIndex, 1);
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

function validateConfig(role: string, draft: JsonObject): string {
  if (role === "server") {
    const level = readPath(draft, ["connection", "compression", "level"]);
    if (level === undefined) return "";
    if (typeof level !== "number" || !Number.isInteger(level) || level < 1 || level > 22) {
      return "Compression level must be an integer between 1 and 22.";
    }
    return "";
  }
  if (role !== "client") return "";
  const peers = Array.isArray(draft.peers) ? draft.peers.filter(isObject) : [];
  const seen = new Set<string>();
  for (const peer of peers) {
    const peerId = typeof peer.peer_id === "string" ? peer.peer_id.trim() : "";
    if (!peerId) return "Each peer requires a non-empty Peer ID.";
    if (seen.has(peerId)) return `Duplicate Peer ID “${peerId}”.`;
    seen.add(peerId);
  }
  return "";
}

function displayInput(value: unknown): string | number {
  return typeof value === "string" || typeof value === "number" ? value : "";
}

function isNodeOfflineError(cause: unknown): boolean {
  if (!isObject(cause)) return false;
  const status = cause.status;
  const dataCode = isObject(cause.data) ? cause.data.code : undefined;
  const code = cause.code ?? dataCode;
  return code === "node_offline" && (status === 409 || status === 503);
}

function itemsFrom(value: unknown, key: string): JsonObject[] {
  if (Array.isArray(value)) return value.filter(isObject);
  if (!isObject(value)) return [];
  const candidate = value[key] ?? value.items;
  return Array.isArray(candidate) ? candidate.filter(isObject) : [];
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function itemId(item: JsonObject): string {
  const value = item.id ?? item.peer_id ?? item.connection_id;
  return typeof value === "string" ? value : "";
}

function lines(value: string): string[] {
  return value.split("\n").map((item) => item.trim()).filter(Boolean);
}

function summary(value?: Record<string, unknown>): string {
  if (!value) return "—";
  const method = display(value.method);
  const path = display(value.path);
  const status = display(value.status);
  if (method !== "—" || path !== "—") return `${method} ${path} → ${status}`;
  return JSON.stringify(value);
}

function display(value: unknown): string {
  if (value === undefined || value === null || value === "") return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : "Operation failed.";
}

function refreshWarningMessage(cause: unknown): string {
  const detail = cause instanceof Error ? cause.message : "Unknown error";
  return `Configuration saved, but metadata could not be refreshed (${detail}). Use Refresh to reload.`;
}

function formatDate(value?: string) {
  if (!value) return "Never";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

function title(tab: Tab) {
  return tab.charAt(0).toUpperCase() + tab.slice(1);
}
