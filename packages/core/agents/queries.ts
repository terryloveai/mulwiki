import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { workspaceKeys } from "../workspace/queries";

export const agentKeys = {
  all: (workspace: string) => [...workspaceKeys.detail(workspace), "agents"] as const,
  list: (workspace: string) => [...agentKeys.all(workspace), "list"] as const,
  detail: (workspace: string, agentId: string) => [...agentKeys.all(workspace), agentId] as const,
  tasks: (workspace: string, agentId: string) => [...agentKeys.detail(workspace, agentId), "tasks"] as const,
  task: (workspace: string, agentId: string, taskId: string) => [...agentKeys.tasks(workspace, agentId), taskId] as const,
  runtimes: (workspace: string) => [...agentKeys.all(workspace), "runtimes"] as const,
  runtime: (workspace: string, runtimeId: string) => [...agentKeys.runtimes(workspace), runtimeId] as const,
  skills: (workspace: string) => [...agentKeys.all(workspace), "skills"] as const,
};

export function agentListOptions(workspace: string) {
  return queryOptions({
    queryKey: agentKeys.list(workspace),
    queryFn: () => api.listAgents(workspace),
    select: (data) => data.agents,
    enabled: !!workspace,
    refetchInterval: 10_000,
  });
}

export function agentDetailOptions(workspace: string, agentId: string) {
  return queryOptions({
    queryKey: agentKeys.detail(workspace, agentId),
    queryFn: () => api.getAgent(workspace, agentId),
    select: (data) => data.agent,
    enabled: !!workspace && !!agentId,
  });
}

export function runtimeListOptions(workspace: string) {
  return queryOptions({
    queryKey: agentKeys.runtimes(workspace),
    queryFn: () => api.listRuntimes(workspace),
    select: (data) => data.runtimes,
    enabled: !!workspace,
    refetchInterval: 15_000,
  });
}

export function runtimeDetailOptions(workspace: string, runtimeId: string) {
  return queryOptions({
    queryKey: agentKeys.runtime(workspace, runtimeId),
    queryFn: () => api.getRuntime(workspace, runtimeId),
    select: (data) => data.runtime,
    enabled: !!workspace && !!runtimeId,
  });
}

export function skillListOptions(workspace: string) {
  return queryOptions({
    queryKey: agentKeys.skills(workspace),
    queryFn: () => api.listSkills(workspace),
    select: (data) => data.skills,
    enabled: !!workspace,
  });
}

export function agentTasksOptions(workspace: string, agentId: string) {
  return queryOptions({
    queryKey: agentKeys.tasks(workspace, agentId),
    queryFn: () => api.listAgentTasks(workspace, agentId),
    select: (data) => data.tasks,
    enabled: !!workspace && !!agentId,
  });
}

export function agentTaskOptions(workspace: string, agentId: string, taskId: string) {
  return queryOptions({
    queryKey: agentKeys.task(workspace, agentId, taskId),
    queryFn: () => api.getAgentTask(workspace, agentId, taskId),
    select: (data) => data.task,
    enabled: !!workspace && !!agentId && !!taskId,
  });
}
