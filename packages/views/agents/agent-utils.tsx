import type { Agent, AgentTask } from "@mulwiki/core/types";
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  Code,
  FileText,
  ListChecks,
  Loader2,
  Puzzle,
  Settings,
  Wrench,
  XCircle,
  type LucideIcon,
} from "lucide-react";

export type AgentTab = "instructions" | "skills" | "tasks" | "env" | "custom_args" | "settings";

export const AGENT_TABS: { key: AgentTab; label: string; icon: LucideIcon }[] = [
  { key: "instructions", label: "Instructions", icon: FileText },
  { key: "skills", label: "Skills", icon: Puzzle },
  { key: "tasks", label: "Tasks", icon: ListChecks },
  { key: "env", label: "Environment", icon: Wrench },
  { key: "custom_args", label: "Custom Args", icon: Code },
  { key: "settings", label: "Settings", icon: Settings },
];

export function statusColor(agent: Pick<Agent, "status">) {
  if (agent.status === "archived") return "bg-warning";
  if (agent.status === "online") return "bg-success";
  return "bg-muted-foreground/40";
}

export function fmtDateTime(iso: string) {
  return new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function taskStatusIcon(status: AgentTask["status"]) {
  switch (status) {
    case "completed":
      return <CheckCircle2 className="h-4 w-4 text-success" />;
    case "failed":
      return <XCircle className="h-4 w-4 text-destructive" />;
    case "running":
    case "started":
    case "dispatched":
      return <Loader2 className="h-4 w-4 animate-spin text-accent" />;
    case "cancelled":
      return <AlertCircle className="h-4 w-4 text-warning" />;
    default:
      return <Clock className="h-4 w-4 text-muted-foreground" />;
  }
}
