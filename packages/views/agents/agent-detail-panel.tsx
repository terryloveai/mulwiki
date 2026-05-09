"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  agentDetailOptions,
  agentTasksOptions,
  skillListOptions,
} from "@mulwiki/core/agents/queries";
import {
  useArchiveAgent,
  useAssignSkill,
  useRestoreAgent,
  useUnassignSkill,
  useUpdateAgent,
} from "@mulwiki/core/hooks";
import type { AgentRuntime } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { Archive, Bot, RotateCcw } from "lucide-react";
import { AGENT_TABS, type AgentTab, statusColor } from "./agent-utils";
import { CustomArgsTab } from "./tabs/custom-args-tab";
import { EnvTab } from "./tabs/env-tab";
import { InstructionsTab } from "./tabs/instructions-tab";
import { SettingsTab } from "./tabs/settings-tab";
import { SkillsTab } from "./tabs/skills-tab";
import { TasksTab } from "./tabs/tasks-tab";

export function AgentDetailPanel({
  workspaceSlug,
  agentId,
  runtimes,
  onArchive,
}: {
  workspaceSlug: string;
  agentId: string;
  runtimes: AgentRuntime[];
  onArchive?: () => void;
}) {
  const { data: agent, isLoading } = useQuery(agentDetailOptions(workspaceSlug, agentId));
  const { data: skills } = useQuery(skillListOptions(workspaceSlug));
  const { data: tasks } = useQuery(agentTasksOptions(workspaceSlug, agentId));
  const updateAgent = useUpdateAgent(workspaceSlug);
  const archiveAgent = useArchiveAgent(workspaceSlug);
  const restoreAgent = useRestoreAgent(workspaceSlug);
  const assignSkill = useAssignSkill(workspaceSlug);
  const unassignSkill = useUnassignSkill(workspaceSlug);
  const [tab, setTab] = useState<AgentTab>("instructions");

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
        <Button variant="outline" onClick={() => restoreAgent.mutate(agentId)}>
          <RotateCcw className="h-4 w-4" /> Restore Agent
        </Button>
      </div>
    );
  }

  const runtime = runtimes.find((candidate) => candidate.id === agent.runtime_id);

  return (
    <div className="mx-auto max-w-2xl px-6 py-6">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Bot className="h-5 w-5 text-muted-foreground" />
          <div>
            <h1 className="text-lg font-semibold text-foreground">{agent.name}</h1>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className={`inline-block h-2 w-2 rounded-full ${statusColor(agent)}`} />
              {agent.status === "online" ? "Online" : "Idle"}
              {runtime && <span>· {runtime.name} ({runtime.hostname || "unknown"})</span>}
            </div>
          </div>
        </div>
        <Button
          variant="destructive"
          size="sm"
          onClick={() => {
            if (confirm(`Archive agent "${agent.name}"?`)) {
              archiveAgent.mutate(agentId, { onSuccess: () => onArchive?.() });
            }
          }}
        >
          <Archive className="h-4 w-4" /> Archive
        </Button>
      </div>

      <div className="mb-6 border-b border-border">
        <nav className="-mb-px flex gap-0">
          {AGENT_TABS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              type="button"
              onClick={() => setTab(key)}
              className={`flex items-center gap-1.5 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                tab === key
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground"
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
              {label}
            </button>
          ))}
        </nav>
      </div>

      {tab === "instructions" && <InstructionsTab agent={agent} updateAgent={updateAgent} />}
      {tab === "skills" && (
        <SkillsTab
          agent={agent}
          skills={skills ?? []}
          assignSkill={assignSkill}
          unassignSkill={unassignSkill}
        />
      )}
      {tab === "tasks" && <TasksTab tasks={tasks ?? []} />}
      {tab === "env" && <EnvTab agent={agent} updateAgent={updateAgent} />}
      {tab === "custom_args" && <CustomArgsTab agent={agent} updateAgent={updateAgent} />}
      {tab === "settings" && <SettingsTab agent={agent} updateAgent={updateAgent} />}
    </div>
  );
}
