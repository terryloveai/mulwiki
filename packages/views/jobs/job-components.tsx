"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import type { AgentTask, Job } from "@mulwiki/core/types";
import { agentTasksOptions } from "@mulwiki/core/agents/queries";
import { taskMessagesOptions } from "@mulwiki/core/jobs/queries";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Button } from "@mulwiki/ui/components/Button";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  AlertTriangle,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Clock,
  Cpu,
  FileText,
  RefreshCw,
  Server,
  XCircle,
  type LucideIcon,
} from "lucide-react";

type BadgeVariant = "success" | "warning" | "destructive" | "outline" | "brand";

export const statusConfig: Record<string, { variant: BadgeVariant; icon: LucideIcon; label: string }> = {
  pending: { variant: "outline", icon: Clock, label: "Pending" },
  running: { variant: "brand", icon: RefreshCw, label: "Running" },
  completed: { variant: "success", icon: CheckCircle, label: "Completed" },
  failed: { variant: "destructive", icon: XCircle, label: "Failed" },
};

const taskStatusConfig: Record<string, { variant: BadgeVariant; label: string }> = {
  queued: { variant: "outline", label: "Queued" },
  dispatched: { variant: "warning", label: "Dispatched" },
  started: { variant: "brand", label: "Started" },
  running: { variant: "brand", label: "Running" },
  completed: { variant: "success", label: "Done" },
  failed: { variant: "destructive", label: "Failed" },
  cancelled: { variant: "outline", label: "Cancelled" },
};

export function fmtShort(value: string): string {
  try {
    return new Date(value).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return value;
  }
}

function progressGradient(pct: number): string {
  if (pct < 20) return "from-brand/40 to-brand";
  if (pct < 60) return "from-brand/60 to-brand";
  return "from-brand/80 to-brand";
}

export function StatusFilterBar({
  jobs,
  statusFilter,
  counts,
  onChange,
}: {
  jobs: Job[];
  statusFilter: string | null;
  counts: Record<string, number>;
  onChange: (status: string | null) => void;
}) {
  if (jobs.length === 0) return null;

  return (
    <div className="mb-4 flex flex-wrap gap-2">
      <button
        onClick={() => onChange(null)}
        className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
          statusFilter === null
            ? "bg-brand text-white"
            : "bg-secondary text-muted-foreground hover:bg-accent"
        }`}
      >
        All ({jobs.length})
      </button>
      {(["pending", "running", "completed", "failed"] as const).map((status) => {
        const count = counts[status] || 0;
        if (count === 0 && statusFilter !== status) return null;
        return (
          <button
            key={status}
            onClick={() => onChange(status === statusFilter ? null : status)}
            className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
              statusFilter === status
                ? "bg-brand text-white"
                : "bg-secondary text-muted-foreground hover:bg-accent"
            }`}
          >
            {status} ({count})
          </button>
        );
      })}
    </div>
  );
}

export function JobList({
  jobs,
  expandedJob,
  statusFilter,
  workspaceSlug,
  onToggle,
}: {
  jobs: Job[];
  expandedJob: string | null;
  statusFilter: string | null;
  workspaceSlug: string;
  onToggle: (jobId: string) => void;
}) {
  if (jobs.length === 0) {
    return (
      <div className="py-16 text-center text-muted-foreground">
        {statusFilter ? `No ${statusFilter} jobs.` : "No jobs yet. Click Run Pipeline to start."}
      </div>
    );
  }

  return (
    <ul className="space-y-2">
      {jobs.map((job) => (
        <JobListItem
          key={job.id}
          job={job}
          isExpanded={expandedJob === job.id}
          workspaceSlug={workspaceSlug}
          onToggle={() => onToggle(job.id)}
        />
      ))}
    </ul>
  );
}

function JobListItem({
  job,
  isExpanded,
  workspaceSlug,
  onToggle,
}: {
  job: Job;
  isExpanded: boolean;
  workspaceSlug: string;
  onToggle: () => void;
}) {
  const cfg = statusConfig[job.status] ?? {
    variant: "outline" as const,
    icon: Clock,
    label: job.status,
  };
  const isRunning = job.status === "running";
  const isPending = job.status === "pending";

  return (
    <li>
      <button
        onClick={onToggle}
        className="flex w-full items-center gap-4 rounded-lg border border-border bg-card p-4 text-left transition-colors hover:bg-accent"
      >
        {isRunning ? (
          <RefreshCw className="h-5 w-5 shrink-0 animate-spin text-brand" />
        ) : isPending ? (
          <Clock className="h-5 w-5 shrink-0 text-muted-foreground" />
        ) : (
          <cfg.icon
            className={`h-5 w-5 shrink-0 ${
              job.status === "completed"
                ? "text-green-500"
                : job.status === "failed"
                  ? "text-destructive"
                  : "text-muted-foreground"
            }`}
          />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-medium text-foreground">Job</span>
            {job.agent_id && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <Cpu className="h-3 w-3" />
                Agent
              </span>
            )}
          </div>
          <div className="mt-0.5 text-xs text-muted-foreground">
            {fmtShort(job.created_at)}
            {job.completed_at && ` · Completed ${fmtShort(job.completed_at)}`}
            {job.claimed_by && (
              <span className="ml-2 inline-flex items-center gap-1">
                <Server className="h-3 w-3" />
                {job.claimed_by.slice(0, 8)}
              </span>
            )}
          </div>
          {(isRunning || job.status === "completed") && (
            <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-secondary">
              <div
                className={`h-full rounded-full bg-gradient-to-r ${progressGradient(job.progress)} transition-all duration-700 ease-out`}
                style={{ width: `${job.progress}%` }}
              />
            </div>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={cfg.variant}>
            {isRunning && <RefreshCw className="mr-1 h-3 w-3 animate-spin" />}
            {cfg.label}
            {isRunning && ` ${job.progress}%`}
          </Badge>
          {isExpanded ? (
            <ChevronUp className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )}
        </div>
      </button>

      {isExpanded && (
        <div className="space-y-3 rounded-b-lg border-x border-b border-border bg-card p-4">
          <JobMetadata job={job} />

          {job.error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <div>
                  <p className="font-medium">Error</p>
                  <p className="mt-0.5">{job.error}</p>
                </div>
              </div>
            </div>
          )}

          {isRunning && <JobLogStream workspaceSlug={workspaceSlug} jobId={job.id} />}
          {job.agent_id && <AgentTaskTimeline workspaceSlug={workspaceSlug} agentId={job.agent_id} />}
        </div>
      )}
    </li>
  );
}

function JobMetadata({ job }: { job: Job }) {
  return (
    <>
      <div className="grid grid-cols-2 gap-3 text-xs sm:grid-cols-3">
        <div>
          <span className="text-muted-foreground">Schema</span>
          <p className="truncate font-medium text-foreground">{job.schema_id || "—"}</p>
        </div>
        <div>
          <span className="text-muted-foreground">Source</span>
          <p className="truncate font-medium text-foreground">{job.source_path || "—"}</p>
        </div>
        <div>
          <span className="text-muted-foreground">Agent</span>
          <p className="truncate font-medium text-foreground">{job.agent_id ? job.agent_id.slice(0, 12) : "—"}</p>
        </div>
      </div>

      {job.source_paths && job.source_paths.length > 1 && (
        <div className="text-xs">
          <span className="text-muted-foreground">All sources ({job.source_paths.length}):</span>
          <ul className="mt-1 list-disc space-y-0.5 pl-4 text-foreground">
            {job.source_paths.map((path) => (
              <li key={path} className="truncate">
                {path}
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}

function JobLogStream({ workspaceSlug, jobId }: { workspaceSlug: string; jobId: string }) {
  const [logs, setLogs] = useState<Array<{ event: string; data: string }>>([]);
  const [done, setDone] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let es: EventSource | null = null;
    try {
      es = api.streamJobLogs(workspaceSlug, jobId);
    } catch {
      return;
    }

    const handleStatus = (event: MessageEvent) => {
      setLogs((prev) => [...prev, { event: "status", data: event.data }]);
    };
    const handleError = (event: MessageEvent) => {
      setLogs((prev) => [...prev, { event: "error", data: event.data }]);
    };
    const handleDone = () => {
      setDone(true);
    };

    es.addEventListener("status", handleStatus);
    es.addEventListener("error", handleError);
    es.addEventListener("done", handleDone);

    return () => {
      es?.removeEventListener("status", handleStatus);
      es?.removeEventListener("error", handleError);
      es?.removeEventListener("done", handleDone);
      es?.close();
      setLogs([]);
      setDone(false);
    };
  }, [workspaceSlug, jobId]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  if (logs.length === 0 && !done) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Spinner className="h-3 w-3" />
        Waiting for logs...
      </div>
    );
  }

  return (
    <div>
      <div className="mb-1.5 flex items-center gap-2">
        <FileText className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">
          Live Log {done && "(complete)"}
        </span>
      </div>
      <div className="max-h-48 overflow-y-auto rounded-md bg-muted p-3 font-mono text-xs">
        {logs.map((entry, index) => {
          const data = tryParse(entry.data);
          const text = typeof data === "object" ? JSON.stringify(data, null, 2) : entry.data;
          return (
            <div
              key={index}
              className={`whitespace-pre-wrap ${
                entry.event === "error" ? "text-destructive" : "text-foreground/80"
              }`}
            >
              {entry.event === "status" ? "▸ " : entry.event === "error" ? "✕ " : "✓ "}
              {text}
            </div>
          );
        })}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

function AgentTaskTimeline({ workspaceSlug, agentId }: { workspaceSlug: string; agentId: string }) {
  const [expandedTask, setExpandedTask] = useState<string | null>(null);
  const { data: tasks } = useQuery(agentTasksOptions(workspaceSlug, agentId));

  const agentTasks = tasks ?? [];
  if (agentTasks.length === 0) return null;

  return (
    <div>
      <div className="mb-1.5 flex items-center gap-2">
        <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">
          Agent Tasks ({agentTasks.length})
        </span>
      </div>
      <div className="max-h-40 space-y-1 overflow-y-auto">
        {agentTasks.map((task) => (
          <AgentTaskRow
            key={task.id}
            task={task}
            isExpanded={expandedTask === task.id}
            onToggle={() => setExpandedTask(expandedTask === task.id ? null : task.id)}
          />
        ))}
      </div>
    </div>
  );
}

function AgentTaskRow({
  task,
  isExpanded,
  onToggle,
}: {
  task: AgentTask;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const cfg = taskStatusConfig[task.status] ?? { variant: "outline" as const, label: task.status };

  return (
    <div className="rounded bg-muted/50 text-xs">
      <button type="button" onClick={onToggle} className="flex w-full items-center gap-2 px-2 py-1 text-left">
        <Badge variant={cfg.variant} className="px-1.5 py-0 text-[10px]">
          {cfg.label}
        </Badge>
        <span className="flex-1 truncate text-muted-foreground">
          {task.source_path?.slice(0, 30) || task.id.slice(0, 8)}
        </span>
        {task.session_id && (
          <span className="max-w-[80px] truncate text-muted-foreground">{task.session_id.slice(0, 8)}</span>
        )}
        {task.error && (
          <span className="max-w-[120px] truncate text-destructive" title={task.error}>
            {task.error.slice(0, 40)}
          </span>
        )}
        <span className="shrink-0 text-muted-foreground">{fmtShort(task.created_at)}</span>
        {isExpanded ? (
          <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
        )}
      </button>
      {isExpanded && (
        <div className="border-t border-border/60 px-2 py-2">
          <TaskMessagePreview taskId={task.id} live={task.status === "running" || task.status === "dispatched"} />
        </div>
      )}
    </div>
  );
}

function TaskMessagePreview({ taskId, live }: { taskId: string; live: boolean }) {
  const { data: messages, isLoading } = useQuery(taskMessagesOptions(taskId, 0, live ? 1500 : false));

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground">
        <Spinner className="h-3 w-3" />
        Loading messages
      </div>
    );
  }

  const taskMessages = messages ?? [];
  if (taskMessages.length === 0) {
    return <div className="text-muted-foreground">No messages</div>;
  }

  return (
    <div className="max-h-48 overflow-y-auto rounded bg-background/60 p-2 font-mono text-[11px]">
      {taskMessages.slice(-30).map((msg) => {
        const label = msg.type || msg.role || "message";
        const text = msg.content || msg.output || msg.status || msg.tool || "";
        return (
          <div key={`${msg.task_id}-${msg.seq}`} className="grid grid-cols-[5rem_1fr] gap-2 py-0.5">
            <span className="truncate text-muted-foreground">{label}</span>
            <span className={`whitespace-pre-wrap break-words ${msg.type === "error" ? "text-destructive" : "text-foreground/80"}`}>
              {text}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function tryParse(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}
