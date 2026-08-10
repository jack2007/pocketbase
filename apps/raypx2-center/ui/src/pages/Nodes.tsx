import { useState, type FormEvent } from "react";
import type { CreateNodeInput, CreateNodeResult } from "../api";

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
  const [error, setError] = useState("");
  const [secret, setSecret] = useState("");

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
        role: String(data.get("role") ?? "unknown") as CreateNodeInput["role"],
      });
      setSecret(result.enroll_secret);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to create node.");
    } finally {
      setSubmitting(false);
    }
  }

  async function remove(node: CenterNode) {
    if (!onDelete) return;
    const label = node.name || node.node_key;
    if (!window.confirm(`Delete node "${label}"? This cannot be undone.`)) return;
    setDeletingKey(node.node_key);
    setError("");
    try {
      await onDelete(node);
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
  }

  return (
    <section>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Fleet</p>
          <h2>Nodes</h2>
          <p className="muted">Manage enrolled raypx2 instances.</p>
        </div>
        <div className="actions">
          <button className="button secondary" onClick={onRefresh} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
          {onCreate && (
            <button className="button" onClick={() => setDialogOpen(true)}>
              Create node
            </button>
          )}
        </div>
      </div>

      {error && !dialogOpen && <div className="alert" role="alert">{error}</div>}

      <div className="table-shell">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Node key</th>
              <th>Role</th>
              <th>Status</th>
              <th>Health</th>
              <th>Last seen</th>
              {onDelete && <th>Actions</th>}
            </tr>
          </thead>
          <tbody>
            {!loading && nodes.length === 0 && (
              <tr>
                <td colSpan={onDelete ? 7 : 6} className="empty">No nodes yet. Create one to get started.</td>
              </tr>
            )}
            {nodes.map((node) => (
              <tr key={node.id}>
                <td className="strong">
                  {onSelect
                    ? <button className="node-link" onClick={() => onSelect(node)}>{node.name || "Unnamed node"}</button>
                    : node.name || "Unnamed node"}
                </td>
                <td><code>{node.node_key}</code></td>
                <td><span className="tag">{node.role}</span></td>
                <td>
                  <span className={`status ${node.online ? "online" : "offline"}`}>
                    <span className="status-dot" />
                    {node.online ? "Online" : "Offline"}
                  </span>
                </td>
                <td>{node.health_status || "—"}</td>
                <td>{formatDate(node.last_seen_at)}</td>
                {onDelete && (
                  <td>
                    <button
                      className="button danger small"
                      disabled={deletingKey === node.node_key}
                      onClick={() => void remove(node)}
                    >
                      {deletingKey === node.node_key ? "Deleting…" : "Delete"}
                    </button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {dialogOpen && (
        <div className="dialog-backdrop" role="presentation">
          <div className="dialog" role="dialog" aria-modal="true" aria-labelledby="create-node-title">
            {secret ? (
              <>
                <p className="eyebrow">Node created</p>
                <h3 id="create-node-title">Save the enrollment secret</h3>
                <p className="muted">This secret is shown only once. Store it before closing.</p>
                <div className="secret"><code>{secret}</code></div>
                <button className="button full" onClick={() => navigator.clipboard?.writeText(secret)}>
                  Copy secret
                </button>
                <button className="button secondary full" onClick={closeDialog}>Done</button>
              </>
            ) : (
              <form onSubmit={submit}>
                <p className="eyebrow">Enrollment</p>
                <h3 id="create-node-title">Create node</h3>
                <label>
                  Name
                  <input name="name" placeholder="Singapore edge" autoFocus />
                </label>
                <label>
                  Node key <span className="optional">(optional)</span>
                  <input name="node_key" placeholder="Generated when empty" />
                </label>
                <label>
                  Role
                  <select name="role" defaultValue="unknown">
                    <option value="unknown">Unknown</option>
                    <option value="client">Client</option>
                    <option value="server">Server</option>
                  </select>
                </label>
                {error && <p className="form-error">{error}</p>}
                <div className="dialog-actions">
                  <button type="button" className="button secondary" onClick={closeDialog}>Cancel</button>
                  <button className="button" disabled={submitting}>
                    {submitting ? "Creating…" : "Create node"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </section>
  );
}

function formatDate(value?: string) {
  if (!value) return "Never";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}
