"use client";

import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { agentListOptions, runtimeListOptions } from "@mulwiki/core/agents/queries";
import { Bot } from "lucide-react";
import { AgentCreatePanel } from "./agent-create-panel";
import { AgentDetailPanel } from "./agent-detail-panel";
import { AgentList } from "./agent-list";

export function AgentsPage({
  workspaceSlug,
  selectedAgentId,
  creating = false,
  onSelectAgent,
  onStartCreate,
  onClearSelection,
}: {
  workspaceSlug: string;
  selectedAgentId?: string;
  creating?: boolean;
  onSelectAgent?: (agentId: string) => void;
  onStartCreate?: () => void;
  onClearSelection?: () => void;
}) {
  const { data: agents, isLoading: agentsLoading, refetch: refetchAgents } = useQuery(agentListOptions(workspaceSlug));
  const { data: runtimes } = useQuery(runtimeListOptions(workspaceSlug));
  const [internalSelectedId, setInternalSelectedId] = useState(selectedAgentId ?? "");

  useEffect(() => {
    setInternalSelectedId(selectedAgentId ?? "");
  }, [selectedAgentId]);

  const activeSelectedId = creating ? "" : internalSelectedId;

  const selectAgent = useCallback(
    (agentId: string) => {
      setInternalSelectedId(agentId);
      onSelectAgent?.(agentId);
    },
    [onSelectAgent],
  );

  const startCreate = useCallback(() => {
    setInternalSelectedId("");
    onStartCreate?.();
  }, [onStartCreate]);

  const clearSelection = useCallback(() => {
    setInternalSelectedId("");
    onClearSelection?.();
  }, [onClearSelection]);

  return (
    <div className="flex flex-1 overflow-hidden">
      <AgentList
        agents={agents ?? []}
        runtimes={runtimes ?? []}
        isLoading={agentsLoading}
        selectedAgentId={activeSelectedId}
        onSelectAgent={selectAgent}
        onCreate={startCreate}
      />

      <div className="flex-1 overflow-y-auto bg-background">
        {creating ? (
          <AgentCreatePanel
            workspaceSlug={workspaceSlug}
            runtimes={runtimes ?? []}
            onDone={(agentId) => {
              selectAgent(agentId);
              refetchAgents();
            }}
            onCancel={clearSelection}
          />
        ) : activeSelectedId ? (
          <AgentDetailPanel
            workspaceSlug={workspaceSlug}
            agentId={activeSelectedId}
            runtimes={runtimes ?? []}
            onArchive={() => {
              clearSelection();
              refetchAgents();
            }}
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
