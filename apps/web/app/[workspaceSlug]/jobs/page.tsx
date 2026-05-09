"use client";

import { use, useState, useRef, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import { agentListOptions, agentTasksOptions } from "@mulwiki/core/agents/queries";
import { jobKeys, jobListOptions, taskMessagesOptions } from "@mulwiki/core/jobs/queries";
import {
  schemaListOptions,
  sourceListOptions,
  workspaceDetailOptions,
} from "@mulwiki/core/workspace/queries";
import { Button } from "@mulwiki/ui/components/Button";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  Play, Clock, CheckCircle, XCircle, AlertTriangle,
  RefreshCw, FileText, ChevronDown, ChevronRight,
  Server, Cpu, ChevronUp, Layers, Database,
} from "lucide-react";

// --- Constants ---

const statusConfig: Record<string, { variant: "success" | "warning" | "destructive" | "outline" | "brand"; icon: React.ComponentType<{ className?: string }>; label: string }> = {
  pending:     { variant: "outline",     icon: Clock,        label: "Pending" },
  running:     { variant: "brand",       icon: RefreshCw,    label: "Running" },
  completed:   { variant: "success",     icon: CheckCircle,  label: "Completed" },
  failed:      { variant: "destructive", icon: XCircle,      label: "Failed" },
};

const taskStatusConfig: Record<string, { variant: "success" | "warning" | "destructive" | "outline" | "brand"; label: string }> = {
  queued:     { variant: "outline",     label: "Queued" },
  dispatched: { variant: "warning",     label: "Dispatched" },
  started:    { variant: "brand",       label: "Started" },
  running:    { variant: "brand",       label: "Running" },
  completed:  { variant: "success",     label: "Done" },
  failed:     { variant: "destructive", label: "Failed" },
  cancelled:  { variant: "outline",     label: "Cancelled" },
};

// --- Helpers ---

function fmtDateTime(s: string): string {
  try { return new Date(s).toLocaleString(); } catch { return s; }
}

function fmtShort(s: string): string {
  try {
    return new Date(s).toLocaleString(undefined, {
      month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit",
    });
  } catch { return s; }
}

function progressGradient(pct: number): string {
  if (pct < 20) return "from-brand/40 to-brand";
  if (pct < 60) return "from-brand/60 to-brand";
  return "from-brand/80 to-brand";
}

export default function JobsPage({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const queryClient = useQueryClient();

  const [expandedJob, setExpandedJob] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string | null>(null);

  // Queries
  const { data: sources } = useQuery(sourceListOptions(workspaceSlug));
  const { data: schemas } = useQuery(schemaListOptions(workspaceSlug));
  const { data: workspace } = useQuery(workspaceDetailOptions(workspaceSlug));
  const { data: agents } = useQuery(agentListOptions(workspaceSlug));

  const availableAgents = agents ?? [];
  const activeSchema = schemas?.find((s) => s.id === workspace?.active_schema_id) ?? schemas?.[0];
  const agent = availableAgents[0];

  const { data: jobs, isLoading } = useQuery(jobListOptions(workspaceSlug));

  // Create mutation — one button, no selectors
  const createMutation = useMutation({
    mutationFn: () => {
      if (!activeSchema) throw new Error("No active schema");
      if (!agent) throw new Error("No agent available");
      const sourcePaths = sources?.map((s) => s.path) ?? [];
      return api.createJob(workspaceSlug, {
        source_paths: sourcePaths,
        schema_id: activeSchema.id,
        agent_id: agent.id,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: jobKeys.list(workspaceSlug) });
    },
  });

  const toggleJob = (jobId: string) =>
    setExpandedJob((prev) => (prev === jobId ? null : jobId));

  // Filter jobs
  const filteredJobs = jobs?.filter((j) => !statusFilter || j.status === statusFilter) ?? [];

  // Status counts
  const counts = jobs?.reduce(
    (acc, j) => ({ ...acc, [j.status]: (acc[j.status] || 0) + 1 }),
    {} as Record<string, number>,
  ) ?? {};

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Jobs</h1>
          <p className="mt-1 text-muted-foreground">
            Run the active schema pipeline on all workspace sources.
          </p>
        </div>
      </div>

      {/* ── Pipeline bar ── */}
      <div className="mb-8 rounded-lg border border-border bg-card p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-6 text-sm">
            <div className="flex items-center gap-2">
              <Layers className="h-4 w-4 text-muted-foreground" />
              <span className="text-muted-foreground">Schema</span>
              <span className="font-medium text-foreground">
                {activeSchema ? `${activeSchema.name} (v${activeSchema.version})` : "None"}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Database className="h-4 w-4 text-muted-foreground" />
              <span className="text-muted-foreground">Sources</span>
              <span className="font-medium text-foreground">{sources?.length ?? 0} files</span>
            </div>
            <div className="flex items-center gap-2">
              <Cpu className="h-4 w-4 text-muted-foreground" />
              <span className="text-muted-foreground">Agent</span>
              <span className="font-medium text-foreground">
                {agent ? `${agent.name}${agent.model ? ` (${agent.model})` : ""}` : "None"}
              </span>
            </div>
          </div>
          <Button
            onClick={() => createMutation.mutate()}
            variant="brand"
            disabled={createMutation.isPending || !activeSchema || !agent}
            className="shrink-0"
          >
            {createMutation.isPending ? (
              <Spinner className="h-4 w-4" />
            ) : (
              <>
                <Play className="h-4 w-4" />
                Run Pipeline
              </>
            )}
          </Button>
        </div>
        {createMutation.isError && (
          <p className="mt-3 text-sm text-destructive">
            {(createMutation.error as Error).message}
          </p>
        )}
      </div>

      {/* ── Status filter bar ── */}
      {jobs && jobs.length > 0 && (
        <div className="mb-4 flex flex-wrap gap-2">
          <button
            onClick={() => setStatusFilter(null)}
            className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
              statusFilter === null
                ? "bg-brand text-white"
                : "bg-secondary text-muted-foreground hover:bg-accent"
            }`}
          >
            All ({jobs.length})
          </button>
          {(["pending", "running", "completed", "failed"] as const).map((s) => {
            const cfg = statusConfig[s];
            const c = counts[s] || 0;
            if (c === 0 && statusFilter !== s) return null;
            return (
              <button
                key={s}
                onClick={() => setStatusFilter(s === statusFilter ? null : s)}
                className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                  statusFilter === s
                    ? "bg-brand text-white"
                    : "bg-secondary text-muted-foreground hover:bg-accent"
                }`}
              >
                {s} ({c})
              </button>
            );
          })}
        </div>
      )}

      {/* ── Job list ── */}
      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="h-6 w-6 text-muted-foreground" />
        </div>
      ) : filteredJobs.length > 0 ? (
        <ul className="space-y-2">
          {filteredJobs.map((job) => {
            const cfg = statusConfig[job.status] ?? {
              variant: "outline" as const,
              icon: Clock,
              label: job.status,
            };
            const isRunning = job.status === "running";
            const isPending = job.status === "pending";
            const isExpanded = expandedJob === job.id;

            return (
              <li key={job.id}>
                <button
                  onClick={() => toggleJob(job.id)}
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
                      <span className="font-medium text-foreground">
                        Job
                      </span>
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
                    {/* Progress bar */}
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

                {/* ── Expanded detail panel ── */}
                {isExpanded && (
                  <div className="rounded-b-lg border-x border-b border-border bg-card p-4 space-y-3">
                    {/* Metadata */}
                    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 text-xs">
                      <div>
                        <span className="text-muted-foreground">Schema</span>
                        <p className="font-medium text-foreground truncate">{job.schema_id || "—"}</p>
                      </div>
                      <div>
                        <span className="text-muted-foreground">Source</span>
                        <p className="font-medium text-foreground truncate">{job.source_path || "—"}</p>
                      </div>
                      <div>
                        <span className="text-muted-foreground">Agent</span>
                        <p className="font-medium text-foreground truncate">{job.agent_id ? job.agent_id.slice(0, 12) : "—"}</p>
                      </div>
                    </div>

                    {/* Multi-source paths */}
                    {job.source_paths && job.source_paths.length > 1 && (
                      <div className="text-xs">
                        <span className="text-muted-foreground">All sources ({job.source_paths.length}):</span>
                        <ul className="mt-1 list-disc pl-4 space-y-0.5 text-foreground">
                          {job.source_paths.map((p) => (
                            <li key={p} className="truncate">{p}</li>
                          ))}
                        </ul>
                      </div>
                    )}

                    {/* Error details */}
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

                    {/* Live log stream (SSE) */}
                    {isRunning && (
                      <JobLogStream workspaceSlug={workspaceSlug} jobId={job.id} />
                    )}

                    {/* Task timeline placeholder — fetched from agent tasks */}
                    {job.agent_id && (
                      <AgentTaskTimeline workspaceSlug={workspaceSlug} agentId={job.agent_id} />
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      ) : (
        <div className="py-16 text-center text-muted-foreground">
          {statusFilter
            ? `No ${statusFilter} jobs.`
            : "No jobs yet. Click Run Pipeline to start."}
        </div>
      )}
    </div>
  );
}

// ── Live SSE log stream ──
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

    const handleStatus = (e: MessageEvent) => {
      setLogs((prev) => [...prev, { event: "status", data: e.data }]);
    };
    const handleError = (e: MessageEvent) => {
      setLogs((prev) => [...prev, { event: "error", data: e.data }]);
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
      <div className="flex items-center gap-2 mb-1.5">
        <FileText className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">
          Live Log {done && "(complete)"}
        </span>
      </div>
      <div className="max-h-48 overflow-y-auto rounded-md bg-muted p-3 font-mono text-xs">
        {logs.map((entry, i) => {
          const data = tryParse(entry.data);
          const text = typeof data === "object" ? JSON.stringify(data, null, 2) : entry.data;
          return (
            <div
              key={i}
              className={`whitespace-pre-wrap ${
                entry.event === "error"
                  ? "text-destructive"
                  : "text-foreground/80"
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

// ── Agent task timeline ──
function AgentTaskTimeline({ workspaceSlug, agentId }: { workspaceSlug: string; agentId: string }) {
  const [expandedTask, setExpandedTask] = useState<string | null>(null);
  const { data: tasks } = useQuery(agentTasksOptions(workspaceSlug, agentId));

  const agentTasks = tasks ?? [];
  if (agentTasks.length === 0) return null;

  return (
    <div>
      <div className="flex items-center gap-2 mb-1.5">
        <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">
          Agent Tasks ({agentTasks.length})
        </span>
      </div>
      <div className="max-h-40 overflow-y-auto space-y-1">
        {agentTasks.map((t) => {
          const cfg = taskStatusConfig[t.status] ?? { variant: "outline" as const, label: t.status };
          const isExpanded = expandedTask === t.id;
          return (
            <div
              key={t.id}
              className="rounded bg-muted/50 text-xs"
            >
              <button
                type="button"
                onClick={() => setExpandedTask(isExpanded ? null : t.id)}
                className="flex w-full items-center gap-2 px-2 py-1 text-left"
              >
                <Badge variant={cfg.variant} className="text-[10px] px-1.5 py-0">
                  {cfg.label}
                </Badge>
                <span className="text-muted-foreground flex-1 truncate">
                  {t.source_path?.slice(0, 30) || t.id.slice(0, 8)}
                </span>
                {t.session_id && (
                  <span className="text-muted-foreground truncate max-w-[80px]">
                    {t.session_id.slice(0, 8)}
                  </span>
                )}
                {t.error && (
                  <span className="text-destructive truncate max-w-[120px]" title={t.error}>
                    {t.error.slice(0, 40)}
                  </span>
                )}
                <span className="text-muted-foreground shrink-0">
                  {fmtShort(t.created_at)}
                </span>
                {isExpanded ? (
                  <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                )}
              </button>
              {isExpanded && (
                <div className="border-t border-border/60 px-2 py-2">
                  <TaskMessagePreview taskId={t.id} live={t.status === "running" || t.status === "dispatched"} />
                </div>
              )}
            </div>
          );
        })}
      </div>
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

// ── Helper ──
function tryParse(s: string): unknown {
  try { return JSON.parse(s); } catch { return s; }
}
