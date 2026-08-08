import { useEffect, useState } from "react";
import {
  listAuditLogs,
  listConfigRevisions,
  proxyNode,
  type AuditLog,
  type ConfigRevision,
} from "../api";
import type { CenterNode } from "./Nodes";

type Tab = "overview" | "ops" | "config" | "audit";
type JsonObject = Record<string, unknown>;

interface NodeDetailProps {
  node: CenterNode;
  onBack: () => void;
}

export function NodeDetail({ node, onBack }: NodeDetailProps) {
  const [tab, setTab] = useState<Tab>("overview");

  return (
    <section>
      <button className="back-button" onClick={onBack}>← Back to nodes</button>
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
        {(["overview", "ops", "config", "audit"] as Tab[]).map((item) => (
          <button
            key={item}
            role="tab"
            aria-selected={tab === item}
            className={tab === item ? "active" : ""}
            onClick={() => setTab(item)}
          >
            {title(item)}
          </button>
        ))}
      </div>

      {tab === "overview" && <NodeOverview node={node} />}
      {tab === "ops" && <NodeOps node={node} />}
      {tab === "config" && <NodeConfig node={node} />}
      {tab === "audit" && <NodeAudit node={node} />}
    </section>
  );
}

function NodeConfig({ node }: { node: CenterNode }) {
  const [live, setLive] = useState<unknown>();
  const [revisions, setRevisions] = useState<ConfigRevision[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const history = await listConfigRevisions(node.id);
      setRevisions(history);
      if (node.online && (node.role === "client" || node.role === "server")) {
        setLive(await proxyNode(
          node.node_key,
          "GET",
          node.role === "server" ? "/api/v1/server/config" : "/api/v1/config",
        ));
      } else {
        setLive(undefined);
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [node.id, node.online]);

  return (
    <div className="ops-stack">
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="panel ops-panel">
        <PanelHeading title="Live configuration" onRefresh={() => void load()} loading={loading} />
        {!node.online
          ? <p className="muted">Node is offline; showing revision history only.</p>
          : <JsonView value={live} empty={loading ? "Loading…" : "No configuration returned."} />}
      </div>
      <div className="table-shell">
        <table>
          <thead><tr><th>Time</th><th>Kind</th><th>Source</th><th>Summary</th><th>Content</th></tr></thead>
          <tbody>
            {revisions.length === 0 && <EmptyRow columns={5} loading={loading} />}
            {revisions.map((revision) => (
              <tr key={revision.id}>
                <td>{formatDate(revision.created)}</td>
                <td><span className="tag">{revision.kind}</span></td>
                <td>{revision.source}</td>
                <td>{revision.diff_summary || "—"}</td>
                <td><details><summary>View JSON</summary><JsonView value={revision.content} /></details></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
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
  const [serverConfig, setServerConfig] = useState<JsonObject>({});
  const [allowTargets, setAllowTargets] = useState("");
  const [denyTargets, setDenyTargets] = useState("");
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      if (node.role === "server") {
        const [healthResult, configResult, connectionsResult] = await Promise.all([
          proxyNode(node.node_key, "GET", "/api/v1/health"),
          proxyNode<JsonObject>(node.node_key, "GET", "/api/v1/server/config"),
          proxyNode(node.node_key, "GET", "/api/v1/server/connections"),
        ]);
        setHealth(healthResult);
        setServerConfig(configResult);
        setAllowTargets(readTargets(configResult, "allow_targets").join("\n"));
        setDenyTargets(readTargets(configResult, "deny_targets").join("\n"));
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

  async function saveAcl() {
    if (!node.online) return;
    setWriting(true);
    setError("");
    try {
      const updated = await proxyNode<JsonObject>(node.node_key, "PATCH", "/api/v1/server/config", {
        allow_targets: lines(allowTargets),
        deny_targets: lines(denyTargets),
      });
      setServerConfig(updated);
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
        <>
          <div className="panel ops-panel">
            <PanelHeading title="Server ACL" />
            <div className="acl-grid">
              <label>Allow targets<textarea value={allowTargets} onChange={(event) => setAllowTargets(event.target.value)} /></label>
              <label>Deny targets<textarea value={denyTargets} onChange={(event) => setDenyTargets(event.target.value)} /></label>
            </div>
            <div className="acl-actions">
              <span className="muted">One CIDR or target per line.</span>
              <button className="button" disabled={!node.online || writing || loading} onClick={() => void saveAcl()}>
                {writing ? "Saving…" : "Save ACL"}
              </button>
            </div>
            {Object.keys(serverConfig).length > 0 && <details><summary>Raw server config</summary><JsonView value={serverConfig} /></details>}
          </div>
          <ConnectionsTable connections={connections} loading={loading} />
        </>
      )}
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

function readTargets(config: JsonObject, key: string): string[] {
  const direct = config[key];
  if (Array.isArray(direct)) return direct.filter((value): value is string => typeof value === "string");
  const acl = isObject(config.acl) ? config.acl[key] : undefined;
  return Array.isArray(acl) ? acl.filter((value): value is string => typeof value === "string") : [];
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

function formatDate(value?: string) {
  if (!value) return "Never";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

function title(tab: Tab) {
  return tab.charAt(0).toUpperCase() + tab.slice(1);
}
