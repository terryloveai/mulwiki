import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import {
  agentDetailOptions,
  agentKeys,
  agentListOptions,
  agentTaskOptions,
  agentTasksOptions,
  runtimeDetailOptions,
  runtimeListOptions,
  skillListOptions,
} from "../agents/queries";
import { useRealtimeSync } from "../realtime/use-realtime-sync";
import { RealtimeClient } from "../realtime/ws-client";
import type {
  CreateRuntimeRequest,
  UpdateRuntimeRequest,
  CreateAgentRequest,
  UpdateAgentRequest,
  CreateSkillRequest,
  UpdateSkillRequest,
} from "../types";

/* ── Daemon ── */

export function useDaemons(ws: string) {
  return useQuery({
    queryKey: ["daemons", ws],
    queryFn: () => api.listDaemons(ws),
    select: (data) => data.daemons,
    refetchInterval: 10_000,
  });
}

export function useStopDaemon(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.stopDaemon(ws, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["daemons", ws] });
    },
  });
}

export function useStartDaemon(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (opts?: { serverUrl?: string }) => api.startDaemon(ws, opts?.serverUrl),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["daemons", ws] });
      qc.invalidateQueries({ queryKey: agentKeys.runtimes(ws) });
    },
  });
}

export function useDaemonLogs(ws: string, id: string | null) {
  return useQuery({
    queryKey: ["daemons", ws, id, "logs"],
    queryFn: () => api.getDaemonLogs(ws, id!, 50),
    enabled: !!id,
    refetchInterval: 5_000,
  });
}

/* ── Runtimes ── */

export function useRuntimes(ws: string) {
  return useQuery(runtimeListOptions(ws));
}

export function useRuntime(ws: string, runtimeId: string) {
  return useQuery(runtimeDetailOptions(ws, runtimeId));
}

export function useCreateRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateRuntimeRequest) => api.createRuntime(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.runtimes(ws) });
    },
  });
}

export function useUpdateRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateRuntimeRequest & { id: string }) =>
      api.updateRuntime(ws, id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.runtimes(ws) });
    },
  });
}

export function useDeleteRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (runtimeId: string) => api.deleteRuntime(ws, runtimeId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.runtimes(ws) });
    },
  });
}

/* ── Agents ── */

export function useAgents(ws: string) {
  return useQuery(agentListOptions(ws));
}

export function useAgent(ws: string, id: string) {
  return useQuery(agentDetailOptions(ws, id));
}

export function useCreateAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateAgentRequest) => api.createAgent(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.list(ws) });
    },
  });
}

export function useUpdateAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateAgentRequest & { id: string }) =>
      api.updateAgent(ws, id, data),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentKeys.list(ws) });
      qc.invalidateQueries({ queryKey: agentKeys.detail(ws, vars.id) });
    },
  });
}

export function useArchiveAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) => api.archiveAgent(ws, agentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.list(ws) });
    },
  });
}

export function useRestoreAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) => api.restoreAgent(ws, agentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.list(ws) });
    },
  });
}

/* ── Skills ── */

export function useSkills(ws: string) {
  return useQuery(skillListOptions(ws));
}

export function useCreateSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateSkillRequest) => api.createSkill(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.skills(ws) });
    },
  });
}

export function useUpdateSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateSkillRequest & { id: string }) =>
      api.updateSkill(ws, id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.skills(ws) });
    },
  });
}

export function useDeleteSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (skillId: string) => api.deleteSkill(ws, skillId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentKeys.skills(ws) });
    },
  });
}

export function useAssignSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, skillId }: { agentId: string; skillId: string }) =>
      api.assignSkill(ws, agentId, skillId),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentKeys.detail(ws, vars.agentId) });
    },
  });
}

export function useUnassignSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, skillId }: { agentId: string; skillId: string }) =>
      api.unassignSkill(ws, agentId, skillId),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentKeys.detail(ws, vars.agentId) });
    },
  });
}

/* ── Tasks ── */

export function useAgentTasks(ws: string, agentId: string) {
  return useQuery(agentTasksOptions(ws, agentId));
}

export function useAgentTask(ws: string, agentId: string, taskId: string) {
  return useQuery(agentTaskOptions(ws, agentId, taskId));
}

export { useRealtimeSync } from "../realtime/use-realtime-sync";
export { useRealtimeClient } from "../realtime/provider";

export function useWorkspaceRealtime(workspaceId?: string | null, workspaceKey = workspaceId ?? "") {
  const [client, setClient] = useState<RealtimeClient | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      setClient(null);
      return;
    }

    const nextClient = new RealtimeClient({ workspace: workspaceId });
    setClient(nextClient);
    return () => {
      nextClient.close();
    };
  }, [workspaceId]);

  useRealtimeSync(workspaceKey, client);
}
