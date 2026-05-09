"use client";

import { use, useState, useCallback, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  useCreateAgent,
  useUpdateAgent,
  useArchiveAgent,
  useRestoreAgent,
  useAssignSkill,
  useUnassignSkill,
} from "@mulwiki/core/hooks";
import {
  agentDetailOptions,
  agentListOptions,
  agentTasksOptions,
  runtimeListOptions,
  skillListOptions,
} from "@mulwiki/core/agents/queries";
import type { Agent, AgentTask } from "@mulwiki/core/types";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  Bot,
  Cpu,
  FileText,
  Puzzle,
  ListChecks,
  Wrench,
  Settings,
  Code,
  Plus,
  Trash2,
  Edit3,
  Archive,
  RotateCcw,
  Eye,
  EyeOff,
  Search,
  Clock,
  AlertCircle,
  CheckCircle2,
  Loader2,
  XCircle,
} from "lucide-react";

/* ── types ── */

type Tab = "instructions" | "skills" | "tasks" | "env" | "custom_args" | "settings";
const TABS: { key: Tab; label: string; icon: typeof FileText }[] = [
  { key: "instructions", label: "Instructions", icon: FileText },
  { key: "skills", label: "Skills", icon: Puzzle },
  { key: "tasks", label: "Tasks", icon: ListChecks },
  { key: "env", label: "Environment", icon: Wrench },
  { key: "custom_args", label: "Custom Args", icon: Code },
  { key: "settings", label: "Settings", icon: Settings },
];

function statusColor(agent: Agent) {
  if (agent.status === "archived") return "bg-warning";
  if (agent.status === "online") return "bg-success";
  return "bg-muted-foreground/40";
}

function fmtDateTime(iso: string) {
  return new Date(iso).toLocaleString("en-US", {
    month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit",
  });
}

/* ── tab helpers ── */

function taskStatusIcon(status: AgentTask["status"]) {
  switch (status) {
    case "completed": return <CheckCircle2 className="h-4 w-4 text-success" />;
    case "failed":    return <XCircle className="h-4 w-4 text-destructive" />;
    case "running":
    case "started":
    case "dispatched": return <Loader2 className="h-4 w-4 animate-spin text-accent" />;
    case "cancelled": return <AlertCircle className="h-4 w-4 text-warning" />;
    default:           return <Clock className="h-4 w-4 text-muted-foreground" />;
  }
}

/* ══════════════════════════════════════════════════════════
   Agents Page — Multica-style split layout
   ══════════════════════════════════════════════════════════ */

export default function AgentsPage({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const router = useRouter();
  const sp = useSearchParams();

  const { data: agents, isLoading: agLoading, refetch: refetchAgents } = useQuery(agentListOptions(workspaceSlug));
  const { data: runtimes } = useQuery(runtimeListOptions(workspaceSlug));

  // selection driven by searchParams
  const idFromUrl = sp.get("id") ?? "";
  const creating = sp.get("new") === "true";
  const [selectedId, setSelectedId] = useState(idFromUrl);
  const [search, setSearch] = useState("");

  // sync URL when selection changes internally
  const selectAgent = useCallback((id: string) => {
    setSelectedId(id);
    router.replace(`/${workspaceSlug}/agents?id=${id}`, { scroll: false });
  }, [router, workspaceSlug]);

  const startCreate = useCallback(() => {
    setSelectedId("");
    router.replace(`/${workspaceSlug}/agents?new=true`, { scroll: false });
  }, [router, workspaceSlug]);

  const clearSelection = useCallback(() => {
    setSelectedId("");
    router.replace(`/${workspaceSlug}/agents`, { scroll: false });
  }, [router, workspaceSlug]);

  useEffect(() => {
    if (creating) setSelectedId("");
    else if (idFromUrl) setSelectedId(idFromUrl);
    else if (!sp.has("id") && !sp.has("new")) setSelectedId("");
  }, [idFromUrl, creating, sp]);

  const filtered = (agents ?? []).filter((a) =>
    !search || a.name.toLowerCase().includes(search.toLowerCase()),
  );

  const runtimeMap = new Map((runtimes ?? []).map((r) => [r.id, r]));

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* ── Middle Panel: Agent List ── */}
      <div className="flex w-72 flex-shrink-0 flex-col border-r border-border bg-card">
        {/* header */}
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">
            Agents
            {filtered.length > 0 && (
              <span className="ml-1.5 text-muted-foreground">{filtered.length}</span>
            )}
          </h2>
          <Button size="icon" variant="ghost" onClick={startCreate} title="New Agent">
            <Plus className="h-4 w-4" />
          </Button>
        </div>

        {/* search */}
        <div className="border-b border-border px-3 py-2">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search..."
              className="h-7 pl-7 text-xs"
            />
          </div>
        </div>

        {/* list */}
        <div className="flex-1 overflow-y-auto">
          {agLoading ? (
            <div className="flex justify-center py-12">
              <Spinner className="h-5 w-5 text-muted-foreground" />
            </div>
          ) : filtered.length === 0 ? (
            <div className="px-4 py-8 text-center text-xs text-muted-foreground">
              {search ? "No agents match your search." : "No agents yet."}
            </div>
          ) : (
            filtered.map((agent) => {
              const isSelected = agent.id === selectedId;
              const rt = runtimeMap.get(agent.runtime_id);
              return (
                <button
                  key={agent.id}
                  onClick={() => selectAgent(agent.id)}
                  className={`flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40 ${
                    isSelected ? "bg-accent/40" : ""
                  }`}
                >
                  <Bot className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-foreground">
                      {agent.name}
                    </div>
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <span className={`inline-block h-2 w-2 rounded-full ${statusColor(agent)}`} />
                      {agent.status === "archived" ? "Archived" : "Idle"}
                      {rt && <span className="text-muted-foreground/60">· {rt.backend}</span>}
                    </div>
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>

      {/* ── Right Panel ── */}
      <div className="flex-1 overflow-y-auto bg-background">
        {creating ? (
          <AgentCreatePanel
            workspaceSlug={workspaceSlug}
            runtimes={runtimes ?? []}
            onDone={(agentId) => {
              selectAgent(agentId);
              refetchAgents();
            }}
            onCancel={() => clearSelection()}
          />
        ) : selectedId ? (
          <AgentDetailPanel
            workspaceSlug={workspaceSlug}
            agentId={selectedId}
            runtimes={runtimes ?? []}
            onArchive={() => { clearSelection(); refetchAgents(); }}
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <div className="text-center text-muted-foreground">
              <Bot className="mx-auto mb-3 h-10 w-10 opacity-20" />
              <p className="text-sm">Select an agent or create a new one.</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* ══════════════════════════════════════════════════════════
   Agent Detail Panel (Right Panel)
   ══════════════════════════════════════════════════════════ */

function AgentDetailPanel({
  workspaceSlug,
  agentId,
  runtimes,
  onArchive,
}: {
  workspaceSlug: string;
  agentId: string;
  runtimes: import("@mulwiki/core/types").AgentRuntime[];
  onArchive: () => void;
}) {
  const { data: agent, isLoading } = useQuery(agentDetailOptions(workspaceSlug, agentId));
  const { data: skills } = useQuery(skillListOptions(workspaceSlug));
  const { data: tasks } = useQuery(agentTasksOptions(workspaceSlug, agentId));
  const updateAgent = useUpdateAgent(workspaceSlug);
  const archiveAgent = useArchiveAgent(workspaceSlug);
  const restoreAgent = useRestoreAgent(workspaceSlug);
  const assignSkill = useAssignSkill(workspaceSlug);
  const unassignSkill = useUnassignSkill(workspaceSlug);

  const [tab, setTab] = useState<Tab>("instructions");

  if (isLoading || !agent) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="h-5 w-5 text-muted-foreground" />
      </div>
    );
  }
  if (agent.status === "archived") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4">
        <Archive className="h-10 w-10 text-muted-foreground/30" />
        <p className="text-sm text-muted-foreground">This agent has been archived.</p>
        <Button
          variant="outline"
          onClick={() => restoreAgent.mutate(agentId)}
        >
          <RotateCcw className="h-4 w-4" /> Restore Agent
        </Button>
      </div>
    );
  }

  const hasRuntime = runtimes.find((r) => r.id === agent.runtime_id);

  return (
    <div className="mx-auto max-w-2xl px-6 py-6">
      {/* Header bar */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Bot className="h-5 w-5 text-muted-foreground" />
          <div>
            <h1 className="text-lg font-semibold text-foreground">{agent.name}</h1>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className={`inline-block h-2 w-2 rounded-full ${statusColor(agent)}`} />
              {(agent as Agent).status === "archived" ? "Archived" : "Idle"}
              {hasRuntime && <span>· {hasRuntime.name} ({hasRuntime.hostname || "unknown"})</span>}
            </div>
          </div>
        </div>
        <Button
          variant="destructive"
          size="sm"
          onClick={() => {
            if (confirm(`Archive agent "${agent.name}"?`)) {
              archiveAgent.mutate(agentId, {
                onSuccess: () => onArchive(),
              });
            }
          }}
        >
          <Archive className="h-4 w-4" /> Archive
        </Button>
      </div>

      {/* Underline tab bar */}
      <div className="mb-6 border-b border-border">
        <nav className="-mb-px flex gap-0">
          {TABS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              onClick={() => setTab(key)}
              className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium transition-colors border-b-2 ${
                tab === key
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground hover:border-muted-foreground/30"
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
              {label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      {tab === "instructions" && (
        <InstructionsTab
          agent={agent}
          updateAgent={updateAgent}
        />
      )}
      {tab === "skills" && (
        <SkillsTab
          agent={agent}
          skills={skills ?? []}
          assignSkill={assignSkill}
          unassignSkill={unassignSkill}
        />
      )}
      {tab === "tasks" && (
        <TasksTab tasks={tasks ?? []} />
      )}
      {tab === "env" && (
        <EnvTab
          agent={agent}
          updateAgent={updateAgent}
        />
      )}
      {tab === "custom_args" && (
        <CustomArgsTab
          agent={agent}
          updateAgent={updateAgent}
        />
      )}
      {tab === "settings" && (
        <SettingsTab
          agent={agent}
          updateAgent={updateAgent}
          workspaceSlug={workspaceSlug}
        />
      )}
    </div>
  );
}

/* ── Instructions Tab ── */

function InstructionsTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const [text, setText] = useState(agent.instructions ?? "");
  const [saving, setSaving] = useState(false);
  const dirty = text !== (agent.instructions ?? "");

  useEffect(() => { setText(agent.instructions ?? ""); }, [agent.instructions]);

  const handleSave = () => {
    setSaving(true);
    updateAgent.mutate(
      { id: agent.id, instructions: text },
      { onSettled: () => setSaving(false) },
    );
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Agent Instructions</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Define this agent&rsquo;s identity and working style. These instructions are injected into
        the agent&rsquo;s context for every task.
      </p>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={12}
        className="w-full rounded-lg border border-border bg-transparent px-3 py-2.5 text-sm font-mono shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-y"
        placeholder="You are a planning agent..."
      />
      <div className="mt-2 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{text.length} characters</span>
        <Button size="sm" disabled={!dirty || saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}

/* ── Skills Tab ── */

function SkillsTab({
  agent,
  skills,
  assignSkill,
  unassignSkill,
}: {
  agent: Agent;
  skills: import("@mulwiki/core/types").AgentSkill[];
  assignSkill: ReturnType<typeof useAssignSkill>;
  unassignSkill: ReturnType<typeof useUnassignSkill>;
}) {
  const assigned = new Set(agent.skills?.map((s) => s.id) ?? []);
  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Skills</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Attach skills that extend the agent&rsquo;s capabilities.
      </p>
      {skills.length === 0 ? (
        <p className="text-sm text-muted-foreground">No skills defined in this workspace.</p>
      ) : (
        <div className="space-y-1.5">
          {skills.map((sk) => {
            const has = assigned.has(sk.id);
            return (
              <div
                key={sk.id}
                className="flex items-center justify-between rounded-md border border-border px-3 py-2"
              >
                <div>
                  <div className="text-sm font-medium text-foreground">{sk.name}</div>
                  {sk.description && (
                    <div className="text-xs text-muted-foreground">{sk.description}</div>
                  )}
                </div>
                <Button
                  size="sm"
                  variant={has ? "outline" : "default"}
                  onClick={() => {
                    if (has) unassignSkill.mutate({ agentId: agent.id, skillId: sk.id });
                    else assignSkill.mutate({ agentId: agent.id, skillId: sk.id });
                  }}
                >
                  {has ? "Remove" : "Attach"}
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ── Tasks Tab ── */

function TasksTab({ tasks }: { tasks: AgentTask[] }) {
  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Task History</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Recent tasks executed by this agent.
      </p>
      {tasks.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          <ListChecks className="mx-auto mb-2 h-8 w-8 opacity-20" />
          No tasks yet.
        </div>
      ) : (
        <div className="space-y-1.5">
          {tasks.map((t) => (
            <div
              key={t.id}
              className="flex items-center gap-3 rounded-md border border-border px-3 py-2"
            >
              {taskStatusIcon(t.status)}
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-foreground">{t.schema_id || t.source_path || t.id}</div>
                <div className="text-xs text-muted-foreground">{fmtDateTime(t.created_at)}</div>
              </div>
              <Badge variant={t.status === "completed" ? "success" : t.status === "failed" ? "destructive" : "secondary"}>
                {t.status}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* ── Environment Tab ── */

function EnvTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const initialEnv = agent.custom_env && typeof agent.custom_env === "object"
    ? Object.entries(agent.custom_env as Record<string, string>)
    : [];
  const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>(
    initialEnv.length > 0 ? initialEnv.map(([k, v]) => ({ key: k, value: v })) : [{ key: "", value: "" }],
  );
  const [showValues, setShowValues] = useState<Record<number, boolean>>({});
  const [saving, setSaving] = useState(false);

  const addRow = () => setEnvVars((p) => [...p, { key: "", value: "" }]);
  const removeRow = (i: number) => setEnvVars((p) => p.filter((_, idx) => idx !== i));

  const handleSave = () => {
    const env: Record<string, string> = {};
    for (const row of envVars) {
      if (row.key.trim()) env[row.key.trim()] = row.value;
    }
    setSaving(true);
    updateAgent.mutate(
      { id: agent.id, custom_env: env },
      { onSettled: () => setSaving(false) },
    );
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Environment Variables</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Variables injected into the agent&rsquo;s execution environment.
      </p>
      <div className="space-y-2">
        {envVars.map((row, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              value={row.key}
              onChange={(e) =>
                setEnvVars((p) => p.map((r, idx) => idx === i ? { ...r, key: e.target.value } : r))
              }
              placeholder="KEY"
              className="flex-1 font-mono text-xs"
            />
            <div className="relative flex-1">
              <Input
                type={showValues[i] ? "text" : "password"}
                value={row.value}
                onChange={(e) =>
                  setEnvVars((p) => p.map((r, idx) => idx === i ? { ...r, value: e.target.value } : r))
                }
                placeholder="value"
                className="flex-1 pr-8 font-mono text-xs"
              />
              <button
                type="button"
                onClick={() => setShowValues((p) => ({ ...p, [i]: !p[i] }))}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                {showValues[i] ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </button>
            </div>
            <Button variant="ghost" size="icon" onClick={() => removeRow(i)} disabled={envVars.length <= 1}>
              <Trash2 className="h-4 w-4 text-muted-foreground" />
            </Button>
          </div>
        ))}
        <Button variant="outline" size="sm" onClick={addRow}>
          <Plus className="h-3.5 w-3.5" /> Add Variable
        </Button>
      </div>
      <div className="mt-4">
        <Button size="sm" disabled={saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}

/* ── Custom Args Tab ── */

function CustomArgsTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const initial = Array.isArray(agent.custom_args)
    ? (agent.custom_args as string[]).join(", ")
    : (agent.custom_args as string) ?? "";
  const [args, setArgs] = useState(initial);
  const [saving, setSaving] = useState(false);

  const handleSave = () => {
    const parsed = args.split(/[,;\n]/).map((s) => s.trim()).filter(Boolean);
    setSaving(true);
    updateAgent.mutate(
      { id: agent.id, custom_args: parsed },
      { onSettled: () => setSaving(false) },
    );
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">CLI Arguments</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Extra arguments passed to the runtime CLI when executing tasks.
      </p>
      <Input
        value={args}
        onChange={(e) => setArgs(e.target.value)}
        placeholder="--verbose, --timeout=300"
      />
      <div className="mt-4">
        <Button size="sm" disabled={saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}

/* ── Settings Tab ── */

function SettingsTab({
  agent,
  updateAgent,
  workspaceSlug,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
  workspaceSlug: string;
}) {
  const [model, setModel] = useState(agent.model ?? "");
  const [maxConcurrent, setMaxConcurrent] = useState(agent.max_concurrent_tasks ?? 6);
  const [visibility, setVisibility] = useState<"private" | "public">(agent.visibility ?? "private");
  const [saving, setSaving] = useState(false);

  const handleSave = () => {
    setSaving(true);
    updateAgent.mutate(
      {
        id: agent.id,
        model: model.trim() || undefined,
        max_concurrent_tasks: maxConcurrent,
        visibility,
      },
      { onSettled: () => setSaving(false) },
    );
  };

  return (
    <div className="space-y-6">
      <div>
        <div className="mb-1 text-sm font-medium text-foreground">Model & Concurrency</div>
        <p className="mb-3 text-xs text-muted-foreground">
          Override the default model and task concurrency for this agent.
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Model</label>
            <Input
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="e.g. claude-sonnet-4-20250514"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Max Concurrent Tasks</label>
            <Input
              type="number"
              min={1}
              max={50}
              value={maxConcurrent}
              onChange={(e) => setMaxConcurrent(Number(e.target.value))}
            />
          </div>
        </div>
      </div>

      <div>
        <div className="mb-1 text-sm font-medium text-foreground">Visibility</div>
        <p className="mb-3 text-xs text-muted-foreground">
          Public agents can be discovered by other workspace members.
        </p>
        <div className="flex gap-3">
          {(["private", "public"] as const).map((v) => (
            <button
              key={v}
              type="button"
              onClick={() => setVisibility(v)}
              className={`rounded-md border px-3 py-1.5 text-sm font-medium capitalize transition-colors ${
                visibility === v
                  ? "border-foreground bg-foreground/5 text-foreground"
                  : "border-border text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground"
              }`}
            >
              {v}
            </button>
          ))}
        </div>
      </div>

      <Button size="sm" disabled={saving} onClick={handleSave}>
        {saving ? "Saving..." : "Save"}
      </Button>
    </div>
  );
}

/* ══════════════════════════════════════════════════════════
   Agent Create Panel (Right Panel, inline)
   ══════════════════════════════════════════════════════════ */

function AgentCreatePanel({
  workspaceSlug,
  runtimes,
  onDone,
  onCancel,
}: {
  workspaceSlug: string;
  runtimes: import("@mulwiki/core/types").AgentRuntime[];
  onDone: (agentId: string) => void;
  onCancel: () => void;
}) {
  const createAgent = useCreateAgent(workspaceSlug);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [runtimeId, setRuntimeId] = useState("");
  const [instructions, setInstructions] = useState("");
  const [model, setModel] = useState("");

  const valid = name.trim() && runtimeId;

  const handleSubmit = () => {
    if (!valid) return;
    createAgent.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        instructions: instructions.trim(),
        runtime_id: runtimeId,
        runtime_config: "{}",
        mcp_config: "{}",
        visibility: "private",
        max_concurrent_tasks: 6,
        model: model.trim() || undefined,
      },
      {
        onSuccess: (data) => {
          onDone(data.agent.id);
        },
      },
    );
  };

  return (
    <div className="mx-auto max-w-2xl px-6 py-6">
      <div className="mb-6 flex items-center gap-3">
        <Bot className="h-5 w-5 text-muted-foreground" />
        <h1 className="text-lg font-semibold text-foreground">New Agent</h1>
      </div>

      <div className="space-y-5">
        {/* Name */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">
              Name <span className="text-destructive">*</span>
            </label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Agent name" autoFocus />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Description</label>
            <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What this agent does" />
          </div>
        </div>

        {/* Runtime */}
        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">
            Runtime <span className="text-destructive">*</span>
          </label>
          {runtimes.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No runtimes available. Start a daemon first.
            </p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {runtimes.map((rt) => (
                <button
                  key={rt.id}
                  type="button"
                  onClick={() => setRuntimeId(rt.id)}
                  className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors ${
                    runtimeId === rt.id
                      ? "border-foreground bg-foreground/5"
                      : "border-border hover:border-muted-foreground/30"
                  }`}
                >
                  <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-foreground">{rt.name}</span>
                  <span className="text-xs text-muted-foreground">{rt.backend}</span>
                </button>
              ))}
            </div>
          )}
          {!runtimeId && runtimes.length > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">Select a runtime for this agent.</p>
          )}
        </div>

        {/* Instructions */}
        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">Instructions (optional)</label>
          <textarea
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            rows={4}
            placeholder="Define this agent's behavior and role…"
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-y"
          />
        </div>

        {/* Model */}
        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">Model (optional)</label>
          <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="Leave blank for runtime default" />
        </div>
      </div>

      {/* Actions */}
      <div className="mt-6 flex items-center justify-between border-t border-border pt-4">
        <span className="text-xs text-muted-foreground">Skills & settings can be configured after creation.</span>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onCancel}>Cancel</Button>
          <Button disabled={!valid || createAgent.isPending} onClick={handleSubmit}>
            {createAgent.isPending ? "Creating..." : "Create Agent"}
          </Button>
        </div>
      </div>
    </div>
  );
}
