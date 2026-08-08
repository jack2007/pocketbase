import { useEffect, useMemo, useState } from "react";
import { createApplyJob, listApplyJobs, listTemplates, type ApplyJob, type ConfigTemplate } from "../api";
import type { CenterNode } from "./Nodes";

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
      <div className="page-heading">
        <div><p className="eyebrow">Configuration</p><h2>Apply jobs</h2></div>
        <button className="button secondary" onClick={() => void refresh()}>Refresh</button>
      </div>
      {error && <div className="alert" role="alert">{error}</div>}
      <div className="panel apply-composer">
        <h3>Apply a template</h3>
        <label>Template<select value={templateId} onChange={(event) => setTemplateId(event.target.value)}>
          <option value="">Select a template</option>
          {templates.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.target_role} · v{item.version}</option>)}
        </select></label>
        <div className="target-picker">
          {eligible.map((node) => (
            <label key={node.id} className="target-option">
              <input type="checkbox" checked={selected.includes(node.id)} onChange={() => toggle(node.id)} />
              <span>{node.name || node.node_key}</span>
              <span className={`status ${node.online ? "online" : "offline"}`}>{node.online ? "Online" : "Offline"}</span>
            </label>
          ))}
          {template && eligible.length === 0 && <p className="muted">No matching {template.target_role} nodes.</p>}
        </div>
        <button className="button" disabled={starting || !templateId || selected.length === 0} onClick={() => void start()}>
          {starting ? "Starting…" : `Apply to ${selected.length} node${selected.length === 1 ? "" : "s"}`}
        </button>
      </div>
      <div className="table-shell">
        <table>
          <thead><tr><th>Job</th><th>Template</th><th>Version</th><th>Status</th><th>Targets</th></tr></thead>
          <tbody>
            {jobs.length === 0 && <tr><td className="empty" colSpan={5}>No apply jobs yet.</td></tr>}
            {jobs.map((job) => (
              <tr key={job.id}>
                <td><code>{job.id}</code></td>
                <td>{templates.find((item) => item.id === job.template)?.name ?? job.template}</td>
                <td>v{job.template_version}</td>
                <td><span className="tag">{job.status}</span></td>
                <td>{job.targets.map((target) => (
                  <div key={target.id}>{nodeName(nodes, target.node)}: {target.status}{target.error ? ` (${target.error})` : ""}</div>
                ))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
