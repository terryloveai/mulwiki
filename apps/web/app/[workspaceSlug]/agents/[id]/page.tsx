"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  useUpdateAgent,
  useArchiveAgent,
  useRestoreAgent,
  useAssignSkill,
  useUnassignSkill,
} from "@mulwiki/core/hooks";
import {
  agentDetailOptions,
  agentTasksOptions,
  runtimeListOptions,
  skillListOptions,
} from "@mulwiki/core/agents/queries";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  Cpu,
  FileText,
  Puzzle,
  ListChecks,
  Wrench,
  Settings,
  ArrowLeft,
  Edit3,
  Archive,
  RotateCcw,
  Plus,
  Trash2,
  Eye,
  EyeOff,
  ChevronDown,
  ChevronRight,
  Clock,
  AlertCircle,
  CheckCircle2,
  Loader2,
  XCircle,
} from "lucide-react";
import type { AgentTask } from "@mulwiki/core/types";

type Section = "runtime" | "instructions" | "skills" | "tasks" | "env" | "settings";

/* ── lightweight date formatter ── */
function fmtDate(iso: string, opts?: Intl.DateTimeFormatOptions): string {
  return new Date(iso).toLocaleString(
    "en-US",
    opts ?? { month: "short", day: "numeric", year: "numeric", hour: "2-digit", minute: "2-digit" },
  );
}

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

const SECTION_LABELS: { key: Section; label: string; icon: typeof Cpu }[] = [
  { key: "runtime", label: "Runtime", icon: Cpu },
  { key: "instructions", label: "Instructions", icon: FileText },
  { key: "skills", label: "Skills", icon: Puzzle },
  { key: "tasks", label: "Tasks", icon: ListChecks },
  { key: "env", label: "Environment", icon: Wrench },
  { key: "settings", label: "Settings", icon: Settings },
];

const TASK_STATUS_ICONS: Record<AgentTask["status"], typeof CheckCircle2> = {
  queued: Clock,
  dispatched: Loader2,
  started: Loader2,
  running: Loader2,
  completed: CheckCircle2,
  failed: XCircle,
  cancelled: AlertCircle,
};

const TASK_STATUS_COLORS: Record<AgentTask["status"], string> = {
  queued: "text-muted-foreground",
  dispatched: "text-warning",
  started: "text-brand",
  running: "text-info animate-spin",
  completed: "text-success",
  failed: "text-destructive",
  cancelled: "text-warning",
};

export default function AgentDetailPage({
  params,
}: {
  params: Promise<{ workspaceSlug: string; id: string }>;
}) {
  const { workspaceSlug, id } = use(params);
  const router = useRouter();
  const [activeSection, setActiveSection] = useState<Section>("runtime");
  const [editing, setEditing] = useState(false);

  const { data: agent, isLoading: agLoading } = useQuery(agentDetailOptions(workspaceSlug, id));
  const { data: runtimes } = useQuery(runtimeListOptions(workspaceSlug));
  const { data: skills } = useQuery(skillListOptions(workspaceSlug));
  const updateAgent = useUpdateAgent(workspaceSlug);
  const archiveAgent = useArchiveAgent(workspaceSlug);
  const restoreAgent = useRestoreAgent(workspaceSlug);
  const assignSkill = useAssignSkill(workspaceSlug);
  const unassignSkill = useUnassignSkill(workspaceSlug);

  const isArchived = agent?.status === "archived";

  // Edit form state
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [editRuntimeId, setEditRuntimeId] = useState("");
  const [editInstructions, setEditInstructions] = useState("");
  const [editSelectedSkills, setEditSelectedSkills] = useState<Set<string>>(
    new Set(),
  );
  const [editEnvVars, setEditEnvVars] = useState<{ key: string; value: string }[]>([]);
  const [editCustomArgs, setEditCustomArgs] = useState("");
  const [editModel, setEditModel] = useState("");
  const [editMaxConcurrent, setEditMaxConcurrent] = useState(6);
  const [editVisibility, setEditVisibility] = useState<"private" | "public">(
    "private",
  );
  const [editShowValues, setEditShowValues] = useState<Record<number, boolean>>(
    {},
  );

  const enterEdit = () => {
    if (!agent) return;
    setEditName(agent.name);
    setEditDescription(agent.description ?? "");
    setEditRuntimeId(agent.runtime_id);
    setEditInstructions(agent.instructions ?? "");
    setEditSelectedSkills(new Set(agent.skills.map((s) => s.id)));
    const envEntries = Object.entries(agent.custom_env ?? {});
    setEditEnvVars(
      envEntries.length > 0
        ? envEntries.map(([k, v]) => ({ key: k, value: v }))
        : [{ key: "", value: "" }],
    );
    setEditCustomArgs((agent.custom_args ?? []).join(", "));
    setEditModel(agent.model ?? "");
    setEditMaxConcurrent(agent.max_concurrent_tasks ?? 6);
    setEditVisibility(agent.visibility ?? "private");
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
  };

  const handleSave = () => {
    if (!agent || !editName.trim() || !editRuntimeId) return;

    const custom_env: Record<string, string> = {};
    for (const row of editEnvVars) {
      if (row.key.trim()) {
        custom_env[row.key.trim()] = row.value;
      }
    }

    const custom_args = editCustomArgs
      .split(/[,;\n]/)
      .map((s) => s.trim())
      .filter(Boolean);

    updateAgent.mutate(
      {
        id: agent.id,
        name: editName.trim(),
        description: editDescription.trim(),
        instructions: editInstructions.trim(),
        runtime_id: editRuntimeId,
        custom_env,
        custom_args,
        visibility: editVisibility,
        max_concurrent_tasks: editMaxConcurrent,
        model: editModel.trim(),
      },
      {
        onSuccess: () => {
          setEditing(false);
        },
      },
    );
  };

  const baseUrl = `/${workspaceSlug}/agents`;
  const selectedRuntime = runtimes?.find(
    (r) => r.id === (editing ? editRuntimeId : agent?.runtime_id),
  );

  if (agLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner className="h-6 w-6 text-muted-foreground" />
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="py-16 text-center text-muted-foreground">
        Agent not found.
      </div>
    );
  }

  const online = agent.status === "online";

  return (
    <div>
      {/* Header */}
      <div className="mb-6 flex items-center gap-3">
        <Link href={baseUrl}>
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            {editing ? (
              <Input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="text-2xl font-bold h-auto py-1 w-96"
              />
            ) : (
              <h1 className="text-2xl font-bold text-foreground truncate">
                {agent.name}
              </h1>
            )}
            <Badge
              variant={
                isArchived
                  ? "warning"
                  : online
                  ? "success"
                  : "secondary"
              }
            >
              {agent.status}
            </Badge>
            <Badge
              variant={agent.visibility === "public" ? "brand" : "outline"}
            >
              {agent.visibility}
            </Badge>
          </div>
          {editing ? (
            <Input
              value={editDescription}
              onChange={(e) => setEditDescription(e.target.value)}
              placeholder="Description"
              className="mt-1 text-sm h-auto py-1 w-96"
            />
          ) : (
            <p className="mt-1 text-muted-foreground">
              {agent.description || "No description"}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2">
          {editing ? (
            <>
              <Button variant="outline" onClick={cancelEdit}>
                Cancel
              </Button>
              <Button
                onClick={handleSave}
                disabled={updateAgent.isPending}
              >
                {updateAgent.isPending ? (
                  <Spinner className="h-4 w-4" />
                ) : (
                  "Save"
                )}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" size="sm" onClick={enterEdit}>
                <Edit3 className="h-4 w-4" /> Edit
              </Button>
              {isArchived ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    restoreAgent.mutate(agent.id, {
                      onSuccess: () =>
                        router.push(`/${workspaceSlug}/agents`),
                    })
                  }
                >
                  <RotateCcw className="h-4 w-4" /> Restore
                </Button>
              ) : (
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => {
                    if (confirm(`Archive agent "${agent.name}"?`)) {
                      archiveAgent.mutate(agent.id, {
                        onSuccess: () =>
                          router.push(`/${workspaceSlug}/agents`),
                      });
                    }
                  }}
                >
                  <Archive className="h-4 w-4" /> Archive
                </Button>
              )}
            </>
          )}
        </div>
      </div>

      {/* Meta */}
      <div className="mb-6 flex flex-wrap gap-3 text-sm text-muted-foreground">
        <span>
          Created:{" "}
          {fmtDate(agent.created_at)}
        </span>
        <span>
          Updated:{" "}
          {fmtDate(agent.updated_at)}
        </span>
        {selectedRuntime && (
          <span>
            Runtime: {selectedRuntime.name} ({selectedRuntime.backend})
          </span>
        )}
        <span>Model: {editing ? editModel || "default" : agent.model || "default"}</span>
        {agent.archived_at && (
          <span>Archived: {fmtDate(agent.archived_at)}</span>
        )}
      </div>

      {/* Section tabs */}
      <div className="mb-6 flex flex-wrap gap-1 rounded-lg border border-border bg-muted/50 p-1">
        {SECTION_LABELS.map(({ key, label, icon: Icon }) => (
          <button
            key={key}
            onClick={() => setActiveSection(key)}
            className={`flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors ${
              activeSection === key
                ? "bg-card text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </div>

      {/* Section 1: Runtime Binding */}
      {activeSection === "runtime" && (
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-base font-semibold text-foreground">
            Runtime Binding
          </h3>
          {editing ? (
            <div>
              <select
                value={editRuntimeId}
                onChange={(e) => setEditRuntimeId(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                <option value="">-- Choose a runtime --</option>
                {runtimes?.map((rt) => (
                  <option key={rt.id} value={rt.id}>
                    {rt.name} ({rt.backend})
                  </option>
                ))}
              </select>
            </div>
          ) : (
            <div className="text-sm text-foreground">
              {selectedRuntime ? (
                <div className="flex items-center gap-3">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">{selectedRuntime.name}</span>
                  <span className="text-muted-foreground">
                    {selectedRuntime.backend}
                  </span>
                  <span
                    className={`inline-block h-2 w-2 rounded-full ${
                      selectedRuntime.status === "online"
                        ? "bg-success"
                        : "bg-muted-foreground/40"
                    }`}
                  />
                  <span className="text-xs text-muted-foreground">
                    {selectedRuntime.status}
                  </span>
                </div>
              ) : (
                <span className="text-muted-foreground">No runtime bound</span>
              )}
            </div>
          )}
        </div>
      )}

      {/* Section 2: Instructions */}
      {activeSection === "instructions" && (
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-base font-semibold text-foreground">
            Instructions
          </h3>
          {editing ? (
            <div>
              <textarea
                value={editInstructions}
                onChange={(e) => setEditInstructions(e.target.value)}
                rows={14}
                className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-y"
              />
              <div className="mt-1 text-xs text-muted-foreground">
                {editInstructions.length} characters
              </div>
            </div>
          ) : agent.instructions ? (
            <pre className="whitespace-pre-wrap font-sans text-sm text-foreground">
              {agent.instructions}
            </pre>
          ) : (
            <div className="text-sm text-muted-foreground">
              No instructions defined.
            </div>
          )}
        </div>
      )}

      {/* Section 3: Skills */}
      {activeSection === "skills" && (
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-base font-semibold text-foreground">
            Skills
          </h3>
          {editing ? (
            <div className="space-y-2">
              {skills?.map((sk) => {
                const isSelected = editSelectedSkills.has(sk.id);
                return (
                  <label
                    key={sk.id}
                    className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-3 hover:bg-accent/30 transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => {
                        setEditSelectedSkills((prev) => {
                          const next = new Set(prev);
                          if (isSelected) {
                            next.delete(sk.id);
                            unassignSkill.mutate({
                              agentId: agent.id,
                              skillId: sk.id,
                            });
                          } else {
                            next.add(sk.id);
                            assignSkill.mutate({
                              agentId: agent.id,
                              skillId: sk.id,
                            });
                          }
                          return next;
                        });
                      }}
                      className="mt-0.5 h-4 w-4 rounded border-input"
                    />
                    <div>
                      <div className="text-sm font-medium text-foreground">
                        {sk.name}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {sk.description || "No description"}
                      </div>
                    </div>
                  </label>
                );
              })}
              {editSelectedSkills.size > 0 && (
                <div className="mt-3 text-sm text-muted-foreground">
                  {editSelectedSkills.size} skill
                  {editSelectedSkills.size > 1 ? "s" : ""} selected
                </div>
              )}
            </div>
          ) : agent.skills.length > 0 ? (
            <ul className="space-y-2">
              {agent.skills.map((sk) => (
                <li key={sk.id} className="flex items-start gap-3 rounded-md border border-border p-3">
                  <Puzzle className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground" />
                  <div>
                    <div className="text-sm font-medium text-foreground">
                      {sk.name}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {sk.description || "No description"}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <div className="text-sm text-muted-foreground">
              No skills assigned.
            </div>
          )}
        </div>
      )}

      {/* Section 4: Tasks */}
      {activeSection === "tasks" && (
        <TasksSection workspaceSlug={workspaceSlug} agentId={agent.id} />
      )}

      {/* Section 5: Environment */}
      {activeSection === "env" && (
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-base font-semibold text-foreground">
            Environment Variables
          </h3>
          {editing ? (
            <div className="space-y-3">
              {editEnvVars.map((row, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <Input
                    value={row.key}
                    onChange={(e) =>
                      setEditEnvVars((prev) =>
                        prev.map((r, i) =>
                          i === idx ? { ...r, key: e.target.value } : r,
                        ),
                      )
                    }
                    placeholder="KEY"
                    className="flex-1 font-mono text-xs"
                  />
                  <div className="relative flex-1">
                    <Input
                      type={editShowValues[idx] ? "text" : "password"}
                      value={row.value}
                      onChange={(e) =>
                        setEditEnvVars((prev) =>
                          prev.map((r, i) =>
                            i === idx ? { ...r, value: e.target.value } : r,
                          ),
                        )
                      }
                      placeholder="value"
                      className="flex-1 pr-8 font-mono text-xs"
                    />
                    <button
                      type="button"
                      onClick={() =>
                        setEditShowValues((prev) => ({
                          ...prev,
                          [idx]: !prev[idx],
                        }))
                      }
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      {editShowValues[idx] ? (
                        <EyeOff className="h-3.5 w-3.5" />
                      ) : (
                        <Eye className="h-3.5 w-3.5" />
                      )}
                    </button>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() =>
                      setEditEnvVars((prev) => prev.filter((_, i) => i !== idx))
                    }
                    disabled={editEnvVars.length <= 1}
                  >
                    <Trash2 className="h-4 w-4 text-muted-foreground" />
                  </Button>
                </div>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  setEditEnvVars((prev) => [...prev, { key: "", value: "" }])
                }
              >
                <Plus className="h-4 w-4" /> Add Variable
              </Button>
            </div>
          ) : Object.keys(agent.custom_env ?? {}).length > 0 ? (
            <div className="space-y-2">
              {Object.entries(agent.custom_env).map(([key, value]) => (
                <div
                  key={key}
                  className="flex items-center gap-2 rounded-md bg-muted/30 px-3 py-1.5"
                >
                  <code className="text-xs font-semibold text-foreground">
                    {key}
                  </code>
                  <span className="text-xs text-muted-foreground">=</span>
                  <code className="text-xs text-muted-foreground">
                    {"•".repeat(Math.min(value.length, 24))}
                  </code>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm text-muted-foreground">
              No environment variables set.
            </div>
          )}
        </div>
      )}

      {/* Section 6: Settings */}
      {activeSection === "settings" && (
        <div className="space-y-6">
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-base font-semibold text-foreground">
              Custom Args
            </h3>
            {editing ? (
              <div>
                <Input
                  value={editCustomArgs}
                  onChange={(e) => setEditCustomArgs(e.target.value)}
                  placeholder="--verbose, --timeout=300"
                />
                <div className="mt-1 text-xs text-muted-foreground">
                  Comma-separated arguments passed to the runtime CLI.
                </div>
              </div>
            ) : (agent.custom_args?.length ?? 0) > 0 ? (
              <div className="flex flex-wrap gap-1">
                {agent.custom_args.map((arg, i) => (
                  <code
                    key={i}
                    className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono text-foreground"
                  >
                    {arg}
                  </code>
                ))}
              </div>
            ) : (
              <div className="text-sm text-muted-foreground">
                No custom args.
              </div>
            )}
          </div>

          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-base font-semibold text-foreground">
              Model & Concurrency
            </h3>
            {editing ? (
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    Model
                  </label>
                  <Input
                    value={editModel}
                    onChange={(e) => setEditModel(e.target.value)}
                    placeholder="e.g. claude-sonnet-4-20250514"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    Max Concurrent Tasks
                  </label>
                  <Input
                    type="number"
                    min={1}
                    max={50}
                    value={editMaxConcurrent}
                    onChange={(e) =>
                      setEditMaxConcurrent(Number(e.target.value))
                    }
                  />
                </div>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <div className="text-xs text-muted-foreground">Model</div>
                  <div className="text-sm font-medium text-foreground">
                    {agent.model || "default"}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">
                    Max Concurrent Tasks
                  </div>
                  <div className="text-sm font-medium text-foreground">
                    {agent.max_concurrent_tasks ?? 6}
                  </div>
                </div>
              </div>
            )}
          </div>

          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-base font-semibold text-foreground">
              Visibility
            </h3>
            {editing ? (
              <div className="flex gap-3">
                {(["private", "public"] as const).map((v) => (
                  <button
                    key={v}
                    type="button"
                    onClick={() => setEditVisibility(v)}
                    className={`rounded-md border px-4 py-2 text-sm font-medium capitalize transition-colors ${
                      editVisibility === v
                        ? "border-brand bg-brand/10 text-brand"
                        : "border-border text-muted-foreground hover:border-input hover:text-foreground"
                    }`}
                  >
                    {v}
                  </button>
                ))}
              </div>
            ) : (
              <Badge
                variant={
                  agent.visibility === "public" ? "brand" : "outline"
                }
                className="text-sm px-3 py-1"
              >
                {agent.visibility}
              </Badge>
            )}
          </div>

          {/* Runtime config & MCP config (read-only) */}
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-base font-semibold text-foreground">
              Advanced
            </h3>
            <div className="space-y-3 text-sm">
              <div>
                <div className="text-xs text-muted-foreground">
                  Runtime Config
                </div>
                <code className="text-xs text-foreground">
                  {agent.runtime_config || "{}"}
                </code>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">
                  MCP Config
                </div>
                <code className="text-xs text-foreground">
                  {agent.mcp_config || "{}"}
                </code>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">
                  Runtime Mode
                </div>
                <code className="text-xs text-foreground">
                  {agent.runtime_mode || "default"}
                </code>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ── Tasks sub-section (read-only paginated task list) ── */

function TasksSection({
  workspaceSlug,
  agentId,
}: {
  workspaceSlug: string;
  agentId: string;
}) {
  const { data: tasks, isLoading } = useQuery(agentTasksOptions(workspaceSlug, agentId));
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [page, setPage] = useState(0);
  const perPage = 20;

  const toggle = (taskId: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      next.has(taskId) ? next.delete(taskId) : next.add(taskId);
      return next;
    });
  };

  const sorted = tasks
    ? [...tasks].sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      )
    : [];

  const total = sorted.length;
  const totalPages = Math.ceil(total / perPage);
  const paged = sorted.slice(page * perPage, (page + 1) * perPage);

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-base font-semibold text-foreground">
        Tasks {tasks ? `(${tasks.length})` : ""}
      </h3>
      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="h-6 w-6 text-muted-foreground" />
        </div>
      ) : sorted.length === 0 ? (
        <div className="py-8 text-center text-muted-foreground">
          <ListChecks className="mx-auto mb-2 h-8 w-8 opacity-30" />
          No tasks executed yet.
        </div>
      ) : (
        <div className="space-y-2">
          {paged.map((task) => {
            const StatusIcon = TASK_STATUS_ICONS[task.status];
            const statusColor = TASK_STATUS_COLORS[task.status];
            const isExpanded = expanded.has(task.id);

            return (
              <div
                key={task.id}
                className="rounded-md border border-border"
              >
                <button
                  onClick={() => toggle(task.id)}
                  className="flex w-full items-center gap-3 p-3 text-left"
                >
                  <StatusIcon
                    className={`h-4 w-4 flex-shrink-0 ${statusColor}`}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-baseline gap-2">
                      <span className="text-sm font-medium text-foreground">
                        {task.id.slice(0, 8)}...
                      </span>
                      <Badge
                        variant={
                          task.status === "completed"
                            ? "success"
                            : task.status === "failed"
                            ? "destructive"
                            : task.status === "running" || task.status === "started" || task.status === "dispatched"
                            ? "brand"
                            : "secondary"
                        }
                        className="text-[10px]"
                      >
                        {task.status}
                      </Badge>
                      {task.priority > 0 && (
                        <span className="text-[10px] text-muted-foreground">
                          P{task.priority}
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 flex gap-3 text-xs text-muted-foreground">
                      {task.source_path && (
                        <span>Source: {task.source_path.slice(0, 8)}...</span>
                      )}
                      {task.schema_id && (
                        <span>Schema: {task.schema_id.slice(0, 8)}...</span>
                      )}
                      <span>
                        {fmtDate(task.created_at, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
                      </span>
                      <span>
                        Attempt {task.attempt}/{task.max_attempts}
                      </span>
                    </div>
                    {task.status === "completed" && task.result && (
                      <div className="mt-1 text-xs text-muted-foreground line-clamp-2">
                        {task.result.slice(0, 200)}
                      </div>
                    )}
                    {task.status === "failed" && task.error && (
                      <div className="mt-1 text-xs text-destructive line-clamp-1">
                        {task.error}
                      </div>
                    )}
                  </div>
                  {isExpanded ? (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="h-4 w-4 text-muted-foreground" />
                  )}
                </button>

                {isExpanded && (
                  <div className="border-t border-border px-4 py-3">
                    <div className="space-y-2 text-xs">
                      <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                        <div>
                          <span className="text-muted-foreground">ID: </span>
                          <code className="text-foreground">{task.id}</code>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Agent: </span>
                          <code className="text-foreground">
                            {task.agent_id}
                          </code>
                        </div>
                        <div>
                          <span className="text-muted-foreground">
                            Runtime:{" "}
                          </span>
                          <code className="text-foreground">
                            {task.runtime_id}
                          </code>
                        </div>
                        <div>
                          <span className="text-muted-foreground">
                            Priority:{" "}
                          </span>
                          <span className="text-foreground">
                            {task.priority}
                          </span>
                        </div>
                        {task.dispatched_at && (
                          <div>
                            <span className="text-muted-foreground">
                              Dispatched:{" "}
                            </span>
                            <span className="text-foreground">
                              {fmtTime(task.dispatched_at)}
                            </span>
                          </div>
                        )}
                        {task.started_at && (
                          <div>
                            <span className="text-muted-foreground">
                              Started:{" "}
                            </span>
                            <span className="text-foreground">
                              {fmtTime(task.started_at)}
                            </span>
                          </div>
                        )}
                        {task.completed_at && (
                          <div>
                            <span className="text-muted-foreground">
                              Completed:{" "}
                            </span>
                            <span className="text-foreground">
                              {fmtTime(task.completed_at)}
                            </span>
                          </div>
                        )}
                      </div>
                      {task.result && (
                        <div>
                          <div className="mb-1 font-medium text-foreground">
                            Result:
                          </div>
                          <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-muted/50 p-2 text-xs text-foreground">
                            {task.result}
                          </pre>
                        </div>
                      )}
                      {task.error && (
                        <div>
                          <div className="mb-1 font-medium text-destructive">
                            Error:
                          </div>
                          <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-destructive/5 p-2 text-xs text-destructive">
                            {task.error}
                          </pre>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between pt-3">
              <Button
                variant="outline"
                size="sm"
                disabled={page === 0}
                onClick={() => setPage((p) => p - 1)}
              >
                Previous
              </Button>
              <span className="text-xs text-muted-foreground">
                Page {page + 1} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages - 1}
                onClick={() => setPage((p) => p + 1)}
              >
                Next
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
