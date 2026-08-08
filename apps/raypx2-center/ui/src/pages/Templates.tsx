import { useEffect, useState } from "react";
import {
  createTemplate,
  deleteTemplate,
  listTemplates,
  updateTemplate,
  type ConfigTemplate,
} from "../api";

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
      edit();
      await refresh();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setSaving(false);
    }
  }

  async function remove(template: ConfigTemplate) {
    if (!window.confirm(`Delete template "${template.name}"?`)) return;
    try {
      await deleteTemplate(template.id);
      await refresh();
    } catch (cause) {
      setError(message(cause));
    }
  }

  return (
    <section>
      <div className="page-heading">
        <div><p className="eyebrow">Configuration</p><h2>Templates</h2></div>
        <button className="button secondary" onClick={() => edit()}>New template</button>
      </div>
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="config-layout">
        <div className="table-shell">
          <table>
            <thead><tr><th>Name</th><th>Role</th><th>Version</th><th>Actions</th></tr></thead>
            <tbody>
              {templates.length === 0 && <tr><td className="empty" colSpan={4}>No templates yet.</td></tr>}
              {templates.map((template) => (
                <tr key={template.id}>
                  <td className="strong">{template.name}</td>
                  <td><span className="tag">{template.target_role}</span></td>
                  <td>v{template.version}</td>
                  <td>
                    <button className="button secondary small" onClick={() => edit(template)}>Edit</button>{" "}
                    <button className="button secondary small" onClick={() => void remove(template)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="panel template-editor">
          <h3>{editing ? `Edit ${editing.name}` : "New template"}</h3>
          <label>Name<input value={name} onChange={(event) => setName(event.target.value)} /></label>
          <label>Target role<select value={role} onChange={(event) => setRole(event.target.value as "client" | "server")}>
            <option value="server">Server</option><option value="client">Client</option>
          </select></label>
          <label>Template body<textarea className="code-input" value={body} onChange={(event) => setBody(event.target.value)} /></label>
          <label>Notes<textarea value={notes} onChange={(event) => setNotes(event.target.value)} /></label>
          <button className="button" disabled={saving || !name.trim()} onClick={() => void save()}>
            {saving ? "Saving…" : editing ? "Save new version" : "Create template"}
          </button>
        </div>
      </div>
    </section>
  );
}

function message(cause: unknown) {
  return cause instanceof Error ? cause.message : "Operation failed.";
}
