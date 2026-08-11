import type { ReactNode } from "react";
import { Card } from "@/components/ui/card";

export function DataTableShell({ children }: { children: ReactNode }) {
  return (
    <Card className="overflow-hidden py-0">
      <div className="overflow-x-auto">{children}</div>
    </Card>
  );
}
