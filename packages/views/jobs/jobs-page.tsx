"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import { agentListOptions } from "@mulwiki/core/agents/queries";
import { jobKeys, jobListOptions } from "@mulwiki/core/jobs/queries";
import {
  schemaListOptions,
  sourceListOptions,
  workspaceDetailOptions,
} from "@mulwiki/core/workspace/queries";
import { Button } from "@mulwiki/ui/components/Button";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { Cpu, Database, Layers, Play } from "lucide-react";
import { JobList, StatusFilterBar } from "./job-components";

export function JobsPage({ workspaceSlug }: { workspaceSlug: string }) {
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

      <StatusFilterBar
        jobs={jobs ?? []}
        statusFilter={statusFilter}
        counts={counts}
        onChange={setStatusFilter}
      />

      {/* ── Job list ── */}
      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="h-6 w-6 text-muted-foreground" />
        </div>
      ) : (
        <JobList
          jobs={filteredJobs}
          expandedJob={expandedJob}
          statusFilter={statusFilter}
          workspaceSlug={workspaceSlug}
          onToggle={toggleJob}
        />
      )}
    </div>
  );
}
