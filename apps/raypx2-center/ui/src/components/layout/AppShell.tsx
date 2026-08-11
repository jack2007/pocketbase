import {
  Activity,
  LayoutDashboard,
  Menu,
  Moon,
  Pause,
  Play,
  RefreshCw,
  Server,
  Sun,
  FileStack,
  Workflow,
} from "lucide-react";
import { useTheme } from "next-themes";
import { useState, type ReactNode } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

export type FleetPage = "overview" | "nodes" | "templates" | "apply-jobs";

const NAV: { id: FleetPage; label: string; icon: typeof LayoutDashboard }[] = [
  { id: "overview", label: "Overview", icon: LayoutDashboard },
  { id: "nodes", label: "Nodes", icon: Server },
  { id: "templates", label: "Templates", icon: FileStack },
  { id: "apply-jobs", label: "Apply Jobs", icon: Workflow },
];

interface AppShellProps {
  page: FleetPage;
  onNavigate: (page: FleetPage) => void;
  onlineCount: number;
  totalCount: number;
  refreshPaused: boolean;
  onToggleRefresh: () => void;
  onRefreshNow: () => void;
  refreshing?: boolean;
  error?: string;
  onSignOut: () => void;
  children: ReactNode;
}

export function AppShell({
  page,
  onNavigate,
  onlineCount,
  totalCount,
  refreshPaused,
  onToggleRefresh,
  onRefreshNow,
  refreshing,
  error,
  onSignOut,
  children,
}: AppShellProps) {
  const [mobileOpen, setMobileOpen] = useState(false);
  const { theme, setTheme } = useTheme();

  function navigate(next: FleetPage) {
    onNavigate(next);
    setMobileOpen(false);
  }

  const nav = (
    <nav className="flex flex-col gap-1" aria-label="Primary navigation">
      {NAV.map((item) => {
        const Icon = item.icon;
        const active = page === item.id;
        return (
          <Button
            key={item.id}
            variant={active ? "secondary" : "ghost"}
            className={cn("justify-start gap-2", active && "font-semibold")}
            onClick={() => navigate(item.id)}
          >
            <Icon className="size-4" />
            {item.label}
          </Button>
        );
      })}
    </nav>
  );

  return (
    <div className="flex min-h-svh bg-background">
      <aside className="sticky top-0 hidden h-svh w-56 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground md:flex">
        <div className="flex items-center gap-2 border-b px-4 py-4">
          <div className="flex size-8 items-center justify-center rounded-md bg-primary text-xs font-bold text-primary-foreground">
            R2
          </div>
          <div>
            <div className="text-sm font-semibold leading-none">raypx2</div>
            <div className="mt-1 font-mono text-[10px] text-muted-foreground">center</div>
          </div>
        </div>
        <div className="flex-1 p-3">{nav}</div>
        <div className="border-t p-3">
          <Button variant="ghost" className="w-full justify-start" onClick={onSignOut}>
            Sign out
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b bg-background/95 px-4 py-3 backdrop-blur">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="outline"
              size="icon"
              className="md:hidden"
              onClick={() => setMobileOpen(true)}
              aria-label="Open navigation"
            >
              <Menu className="size-4" />
            </Button>
            <Badge variant="outline" className="gap-1.5">
              <Activity className="size-3" />
              {onlineCount}/{totalCount} online
            </Badge>
            <Badge variant="secondary" className="hidden sm:inline-flex">
              refresh {refreshPaused ? "paused" : "10s"}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <Select value={theme ?? "light"} onValueChange={(value) => setTheme(value)}>
              <SelectTrigger className="w-[120px]" aria-label="Theme">
                <SelectValue placeholder="Theme" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">System</SelectItem>
                <SelectItem value="light">
                  <span className="flex items-center gap-2"><Sun className="size-3.5" /> Light</span>
                </SelectItem>
                <SelectItem value="dark">
                  <span className="flex items-center gap-2"><Moon className="size-3.5" /> Dark</span>
                </SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="icon" onClick={onToggleRefresh} aria-label={refreshPaused ? "Resume refresh" : "Pause refresh"}>
              {refreshPaused ? <Play className="size-4" /> : <Pause className="size-4" />}
            </Button>
            <Button variant="outline" size="icon" onClick={onRefreshNow} disabled={refreshing} aria-label="Refresh now">
              <RefreshCw className={cn("size-4", refreshing && "animate-spin")} />
            </Button>
            <Button variant="outline" className="md:hidden" onClick={onSignOut}>
              Sign out
            </Button>
          </div>
        </header>

        <main className="flex-1 p-4 sm:p-6 lg:p-8">
          {error && (
            <Alert variant="destructive" className="mb-4">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {children}
        </main>
      </div>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-64 p-0">
          <SheetHeader className="border-b px-4 py-4 text-left">
            <SheetTitle>raypx2 center</SheetTitle>
          </SheetHeader>
          <div className="p-3">{nav}</div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
