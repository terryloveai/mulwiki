"use client";

import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "../api/queries";
import { agentKeys } from "../agents/queries";
import { jobKeys } from "../jobs/queries";
import { workspaceKeys } from "../workspace/queries";
import { useRealtimeClient } from "./provider";
import type { RealtimeClient, RealtimeEvent } from "./ws-client";

type EventPayload = Record<string, unknown>;

function payloadRecord(event: RealtimeEvent): EventPayload {
  return event.payload && typeof event.payload === "object" && !Array.isArray(event.payload)
    ? (event.payload as EventPayload)
    : {};
}

function stringField(event: RealtimeEvent, key: keyof RealtimeEvent | string) {
  const direct = event[key as keyof RealtimeEvent];
  if (typeof direct === "string" && direct) return direct;

  const payload = payloadRecord(event);
  const value = payload[key];
  return typeof value === "string" && value ? value : undefined;
}

function invalidate(queryClient: QueryClient, queryKey: readonly unknown[]) {
  queryClient.invalidateQueries({ queryKey });
}

export function invalidateQueriesForRealtimeEvent(
  queryClient: QueryClient,
  workspaceKey: string,
  event: RealtimeEvent,
) {
  if (!workspaceKey || !event.type) return;

  const [scope] = event.type.split(".");
  const agentId = stringField(event, "agent_id") ?? stringField(event, "agentId");
  const taskId = stringField(event, "task_id") ?? stringField(event, "taskId");
  const jobId = stringField(event, "job_id") ?? stringField(event, "jobId");
  const schemaId = stringField(event, "schema_id") ?? stringField(event, "schemaId");
  const sourcePath = stringField(event, "source_path") ?? stringField(event, "sourcePath") ?? stringField(event, "path");
  const wikiPath = stringField(event, "wiki_path") ?? stringField(event, "wikiPath") ?? stringField(event, "path");

  switch (scope) {
    case "task":
      invalidate(queryClient, jobKeys.all(workspaceKey));
      invalidate(queryClient, agentKeys.all(workspaceKey));
      if (agentId) invalidate(queryClient, agentKeys.tasks(workspaceKey, agentId));
      if (agentId && taskId) invalidate(queryClient, agentKeys.task(workspaceKey, agentId, taskId));
      if (jobId) {
        invalidate(queryClient, jobKeys.detail(workspaceKey, jobId));
        invalidate(queryClient, jobKeys.logs(workspaceKey, jobId));
      }
      if (taskId) invalidate(queryClient, jobKeys.taskMessages(taskId));
      break;

    case "daemon":
      invalidate(queryClient, queryKeys.daemons());
      invalidate(queryClient, agentKeys.runtimes(workspaceKey));
      break;

    case "agent":
      invalidate(queryClient, agentKeys.list(workspaceKey));
      if (agentId) invalidate(queryClient, agentKeys.detail(workspaceKey, agentId));
      break;

    case "schema":
      invalidate(queryClient, workspaceKeys.schemaList(workspaceKey));
      invalidate(queryClient, ["schemas", workspaceKey]);
      if (schemaId) {
        invalidate(queryClient, workspaceKeys.schemaDetail(workspaceKey, schemaId));
        invalidate(queryClient, ["schemas", workspaceKey, schemaId]);
      }
      break;

    case "source":
      invalidate(queryClient, workspaceKeys.sourceList(workspaceKey));
      invalidate(queryClient, ["sources", workspaceKey]);
      if (sourcePath) {
        invalidate(queryClient, workspaceKeys.sourceDetail(workspaceKey, sourcePath));
        invalidate(queryClient, ["source-content", workspaceKey, sourcePath]);
      }
      break;

    case "wiki":
      invalidate(queryClient, workspaceKeys.wikiList(workspaceKey));
      invalidate(queryClient, ["wiki", workspaceKey]);
      if (wikiPath) {
        invalidate(queryClient, workspaceKeys.wikiDetail(workspaceKey, wikiPath));
        invalidate(queryClient, workspaceKeys.wikiBacklinks(workspaceKey, wikiPath));
        invalidate(queryClient, ["wiki", workspaceKey, wikiPath]);
        invalidate(queryClient, ["wiki-backlinks", workspaceKey, wikiPath]);
      }
      break;
  }
}

export function useRealtimeSync(workspaceKey: string, clientOverride?: RealtimeClient | null) {
  const queryClient = useQueryClient();
  const contextClient = useRealtimeClient();
  const client = clientOverride === undefined ? contextClient : clientOverride;

  useEffect(() => {
    if (!client || !workspaceKey) return;
    return client.onAny((event) => {
      invalidateQueriesForRealtimeEvent(queryClient, workspaceKey, event);
    });
  }, [client, queryClient, workspaceKey]);
}
