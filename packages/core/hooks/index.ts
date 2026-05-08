import { useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { queryKeys } from "../api/queries";
import type {
  CreateRuntimeRequest,
  UpdateRuntimeRequest,
  CreateAgentRequest,
  UpdateAgentRequest,
  CreateSkillRequest,
  UpdateSkillRequest,
} from "../types";

function agentsKey(ws: string) {
  return ["agents", ws] as const;
}

function runtimesKey(ws: string) {
  return ["agents", ws, "runtimes"] as const;
}

function agentKey(ws: string, id: string) {
  return ["agents", ws, id] as const;
}

function skillsKey(ws: string) {
  return ["agents", ws, "skills"] as const;
}

function tasksKey(ws: string, agentId: string) {
  return ["agents", ws, agentId, "tasks"] as const;
}

/* ── Daemon ── */

export function useDaemons() {
  return useQuery({
    queryKey: ["daemons"],
    queryFn: () => api.listDaemons(),
    select: (data) => data.daemons,
    refetchInterval: 10_000,
  });
}

export function useStopDaemon() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.stopDaemon(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["daemons"] });
    },
  });
}

export function useStartDaemon(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (opts?: { serverUrl?: string }) => api.startDaemon(ws, opts?.serverUrl),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["daemons"] });
      qc.invalidateQueries({ queryKey: ["agents", ws, "runtimes"] });
    },
  });
}

export function useDaemonLogs(id: string | null) {
  return useQuery({
    queryKey: ["daemons", id, "logs"],
    queryFn: () => api.getDaemonLogs(id!, 50),
    enabled: !!id,
    refetchInterval: 5_000,
  });
}

/* ── Runtimes ── */

export function useRuntimes(ws: string) {
  return useQuery({
    queryKey: runtimesKey(ws),
    queryFn: () => api.listRuntimes(ws),
    select: (data) => data.runtimes,
    refetchInterval: 15_000,
  });
}

export function useRuntime(ws: string, runtimeId: string) {
  return useQuery({
    queryKey: [...runtimesKey(ws), runtimeId],
    queryFn: () => api.getRuntime(ws, runtimeId),
    select: (data) => data.runtime,
  });
}

export function useCreateRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateRuntimeRequest) => api.createRuntime(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: runtimesKey(ws) });
    },
  });
}

export function useUpdateRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateRuntimeRequest & { id: string }) =>
      api.updateRuntime(ws, id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: runtimesKey(ws) });
    },
  });
}

export function useDeleteRuntime(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (runtimeId: string) => api.deleteRuntime(ws, runtimeId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: runtimesKey(ws) });
    },
  });
}

/* ── Agents ── */

export function useAgents(ws: string) {
  return useQuery({
    queryKey: agentsKey(ws),
    queryFn: () => api.listAgents(ws),
    select: (data) => data.agents,
    refetchInterval: 10_000,
  });
}

export function useAgent(ws: string, id: string) {
  return useQuery({
    queryKey: agentKey(ws, id),
    queryFn: () => api.getAgent(ws, id),
    select: (data) => data.agent,
    enabled: !!id,
  });
}

export function useCreateAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateAgentRequest) => api.createAgent(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentsKey(ws) });
    },
  });
}

export function useUpdateAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateAgentRequest & { id: string }) =>
      api.updateAgent(ws, id, data),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentsKey(ws) });
      qc.invalidateQueries({ queryKey: agentKey(ws, vars.id) });
    },
  });
}

export function useArchiveAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) => api.archiveAgent(ws, agentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentsKey(ws) });
    },
  });
}

export function useRestoreAgent(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) => api.restoreAgent(ws, agentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: agentsKey(ws) });
    },
  });
}

/* ── Skills ── */

export function useSkills(ws: string) {
  return useQuery({
    queryKey: skillsKey(ws),
    queryFn: () => api.listSkills(ws),
    select: (data) => data.skills,
  });
}

export function useCreateSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateSkillRequest) => api.createSkill(ws, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillsKey(ws) });
    },
  });
}

export function useUpdateSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: UpdateSkillRequest & { id: string }) =>
      api.updateSkill(ws, id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillsKey(ws) });
    },
  });
}

export function useDeleteSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (skillId: string) => api.deleteSkill(ws, skillId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillsKey(ws) });
    },
  });
}

export function useAssignSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, skillId }: { agentId: string; skillId: string }) =>
      api.assignSkill(ws, agentId, skillId),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentKey(ws, vars.agentId) });
    },
  });
}

export function useUnassignSkill(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, skillId }: { agentId: string; skillId: string }) =>
      api.unassignSkill(ws, agentId, skillId),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: agentKey(ws, vars.agentId) });
    },
  });
}

/* ── Tasks ── */

export function useAgentTasks(ws: string, agentId: string) {
  return useQuery({
    queryKey: tasksKey(ws, agentId),
    queryFn: () => api.listAgentTasks(ws, agentId),
    select: (data) => data.tasks,
    refetchInterval: 10_000,
    enabled: !!agentId,
  });
}

export function useAgentTask(ws: string, agentId: string, taskId: string) {
  return useQuery({
    queryKey: [...tasksKey(ws, agentId), taskId],
    queryFn: () => api.getAgentTask(ws, agentId, taskId),
    select: (data) => data.task,
    enabled: !!agentId && !!taskId,
  });
}

type RealtimeEvent = {
  type?: string;
  workspace_id?: string;
  agent_id?: string;
  task_id?: string;
};

export function useWorkspaceRealtime(workspaceId?: string | null) {
  const qc = useQueryClient();

  useEffect(() => {
    if (!workspaceId || typeof window === "undefined") return;

    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(
      `${scheme}://${window.location.host}/ws?workspace_id=${encodeURIComponent(workspaceId)}`,
    );

    socket.onmessage = (message) => {
      const event = JSON.parse(message.data) as RealtimeEvent;
      if (event.type?.startsWith("task.")) {
        qc.invalidateQueries({ queryKey: ["jobs"] });
        qc.invalidateQueries({ queryKey: ["agents"] });
      }
      if (event.type?.startsWith("daemon.")) {
        qc.invalidateQueries({ queryKey: queryKeys.daemons() });
        qc.invalidateQueries({ queryKey: ["agents"] });
      }
    };

    return () => socket.close();
  }, [qc, workspaceId]);
}
