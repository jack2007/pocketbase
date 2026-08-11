import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/shared/PageHeader";
import type { CenterNode } from "./Nodes";

export function Overview({ nodes }: { nodes: CenterNode[] }) {
  const online = nodes.filter((node) => node.online).length;
  const offline = nodes.length - online;
  const clients = nodes.filter((node) => node.role === "client").length;
  const servers = nodes.filter((node) => node.role === "server").length;

  return (
    <section>
      <PageHeader
        eyebrow="Control plane"
        title="Overview"
        description="Fleet connectivity and enrollment at a glance."
      />
      <div className="mb-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Metric title="Online" value={online} note="Connected agents" />
        <Metric title="Total" value={nodes.length} note="Enrolled nodes" />
        <Metric title="Offline" value={offline} note="Awaiting connection" />
        <Metric title="Roles" value={`${clients}/${servers}`} note="Clients / servers" />
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Fleet summary</CardTitle>
          <CardDescription>
            Open Nodes to inspect a host. Node detail mirrors the local admin console
            (peers, connections, tunnels, ACL, config).
          </CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          Auto-refresh polls node status every 10 seconds while the console is open.
        </CardContent>
      </Card>
    </section>
  );
}

function Metric({ title, value, note }: { title: string; value: string | number; note: string }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-3xl tracking-tight">{value}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-xs text-muted-foreground">{note}</p>
      </CardContent>
    </Card>
  );
}
