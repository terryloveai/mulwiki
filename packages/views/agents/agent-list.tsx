"use client";

import { useMemo, useState } from "react";
import type { Agent, AgentRuntime } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { Bot, Plus, Search } from "lucide-react";
import { statusColor } from "./agent-utils";

export function AgentList({
  agents,
  runtimes,
  isLoading,
  selectedAgentId,
  onSelectAgent,
  onCreate,
}: {
  agents: Agent[];
  runtimes: AgentRuntime[];
  isLoading: boolean;
  selectedAgentId: string;
  onSelectAgent: (agentId: string) => void;
  onCreate: () => void;
}) {
  const [search, setSearch] = useState("");
  const filtered = useMemo(
    () => agents.filter((a) => !search || a.name.toLowerCase().includes(search.toLowerCase())),
    [agents, search],
  );
  const runtimeMap = useMemo(() => new Map(runtimes.map((runtime) => [runtime.id, runtime])), [runtimes]);

  return (
    <div className="flex w-72 flex-shrink-0 flex-col border-r border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-foreground">
          Agents
          {filtered.length > 0 && <span className="ml-1.5 text-muted-foreground">{filtered.length}</span>}
        </h2>
        <Button size="icon" variant="ghost" onClick={onCreate} title="New Agent">
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      <div className="border-b border-border px-3 py-2">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search..."
            className="h-7 pl-7 text-xs"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="flex justify-center py-12">
            <Spinner className="h-5 w-5 text-muted-foreground" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-4 py-8 text-center text-xs text-muted-foreground">
            {search ? "No agents match your search." : "No agents yet."}
          </div>
        ) : (
          filtered.map((agent) => {
            const isSelected = agent.id === selectedAgentId;
            const runtime = runtimeMap.get(agent.runtime_id);

            return (
              <button
                key={agent.id}
                type="button"
                onClick={() => onSelectAgent(agent.id)}
                className={`flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40 ${
                  isSelected ? "bg-accent/40" : ""
                }`}
              >
                <Bot className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium text-foreground">{agent.name}</div>
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className={`inline-block h-2 w-2 rounded-full ${statusColor(agent)}`} />
                    {agent.status === "archived" ? "Archived" : "Idle"}
                    {runtime && <span className="text-muted-foreground/60">· {runtime.backend}</span>}
                  </div>
                </div>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}
