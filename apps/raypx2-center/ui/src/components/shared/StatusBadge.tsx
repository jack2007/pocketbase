import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export function StatusBadge({
  online,
  className,
}: {
  online: boolean;
  className?: string;
}) {
  return (
    <Badge
      variant="outline"
      className={cn(
        "gap-1.5 font-medium",
        online
          ? "border-emerald-500/30 text-emerald-700 dark:text-emerald-400"
          : "text-muted-foreground",
        className,
      )}
    >
      <span
        className={cn(
          "size-1.5 rounded-full",
          online ? "bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.2)]" : "bg-muted-foreground/50",
        )}
      />
      {online ? "Online" : "Offline"}
    </Badge>
  );
}
