import { useState } from "react";
import { ArrowLeft } from "lucide-react";
import type { CenterNode } from "../Nodes";
import { PageHeader } from "@/components/shared/PageHeader";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AclTab } from "./AclTab";
import { AuditTab } from "./AuditTab";
import { ConnectionsTab } from "./ConnectionsTab";
import { OverviewTab } from "./OverviewTab";
import { PeersTab } from "./PeersTab";
import { TunnelsTab } from "./TunnelsTab";

type Tab =
  | "overview"
  | "peers"
  | "connections"
  | "tunnels"
  | "acl"
  | "audit";

interface NodeDetailProps {
  node: CenterNode;
  onBack: () => void;
}

export function NodeDetail({ node, onBack }: NodeDetailProps) {
  const [tab, setTab] = useState<Tab>("overview");

  const tabs: Tab[] = node.role === "server"
    ? ["overview", "peers", "connections", "tunnels", "acl", "audit"]
    : node.role === "client"
      ? ["overview", "peers", "connections", "tunnels", "audit"]
      : ["overview", "audit"];

  function title(item: Tab) {
    return item === "acl" ? "ACL" : item.charAt(0).toUpperCase() + item.slice(1);
  }

  return (
    <section>
      <Button
        variant="ghost"
        size="sm"
        className="mb-3 -ml-2 text-muted-foreground"
        onClick={onBack}
      >
        <ArrowLeft className="size-4" />
        Back to nodes
      </Button>
      <PageHeader
        eyebrow="Node detail"
        title={node.name || "Unnamed node"}
        description={`${node.node_key} · ${node.role}`}
        actions={<StatusBadge online={node.online} />}
      />

      <div className="mb-4 flex flex-wrap gap-1 border-b" role="tablist" aria-label="Node detail">
        {tabs.map((item) => (
          <button
            key={item}
            type="button"
            role="tab"
            aria-selected={tab === item}
            className={cn(
              "border-b-2 px-3 py-2 text-sm font-semibold transition-colors",
              tab === item
                ? "border-foreground text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
            onClick={() => {
              if (item !== tab) setTab(item);
            }}
          >
            {title(item)}
          </button>
        ))}
      </div>

      {tab === "overview" && <OverviewTab node={node} />}
      {tab === "peers" && <PeersTab node={node} />}
      {tab === "connections" && <ConnectionsTab node={node} />}
      {tab === "tunnels" && <TunnelsTab node={node} />}
      {tab === "acl" && <AclTab node={node} />}
      {tab === "audit" && <AuditTab node={node} />}
    </section>
  );
}
