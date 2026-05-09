import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { workspaceKeys } from "../workspace/queries";

export const jobKeys = {
  all: (workspace: string) => [...workspaceKeys.detail(workspace), "jobs"] as const,
  list: (workspace: string) => [...jobKeys.all(workspace), "list"] as const,
  detail: (workspace: string, jobId: string) => [...jobKeys.all(workspace), jobId] as const,
  taskMessages: (taskId: string) => ["tasks", taskId, "messages"] as const,
};

export function jobListOptions(workspace: string) {
  return queryOptions({
    queryKey: jobKeys.list(workspace),
    queryFn: () => api.listJobs(workspace),
    enabled: !!workspace,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      return data.some((job) => job.status === "running" || job.status === "pending")
        ? 2000
        : false;
    },
  });
}

export function jobDetailOptions(workspace: string, jobId: string) {
  return queryOptions({
    queryKey: jobKeys.detail(workspace, jobId),
    queryFn: () => api.getJob(workspace, jobId),
    enabled: !!workspace && !!jobId,
  });
}

export function taskMessagesOptions(taskId: string, since = 0, refetchInterval: number | false = false) {
  return queryOptions({
    queryKey: jobKeys.taskMessages(taskId),
    queryFn: () => api.listTaskMessages(taskId, since),
    select: (data) => data.messages,
    enabled: !!taskId,
    refetchInterval,
  });
}
