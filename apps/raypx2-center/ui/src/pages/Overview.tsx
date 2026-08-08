import type { CenterNode } from "./Nodes";

export function Overview({ nodes }: { nodes: CenterNode[] }) {
  const online = nodes.filter((node) => node.online).length;
  const offline = nodes.length - online;

  return (
    <section>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Control plane</p>
          <h2>Overview</h2>
          <p className="muted">Current fleet connectivity at a glance.</p>
        </div>
      </div>
      <div className="stats">
        <article className="stat-card accent">
          <span>Online</span>
          <strong>{online}</strong>
          <small>Connected nodes</small>
        </article>
        <article className="stat-card">
          <span>Total</span>
          <strong>{nodes.length}</strong>
          <small>Enrolled nodes</small>
        </article>
        <article className="stat-card">
          <span>Offline</span>
          <strong>{offline}</strong>
          <small>Awaiting connection</small>
        </article>
      </div>
      <article className="panel">
        <div>
          <p className="eyebrow">Milestone 1</p>
          <h3>Center management is ready</h3>
        </div>
        <p className="muted">
          Create enrollment credentials and inspect node connectivity. Live agent workflows are deferred.
        </p>
      </article>
    </section>
  );
}
