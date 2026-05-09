import type { ComponentType, ReactNode } from "react";
import { RealtimeClient, type RealtimeEvent } from "./ws-client";
import { RealtimeProvider } from "./provider";
import { useRealtimeSync } from "./use-realtime-sync";

const event: RealtimeEvent = {
  type: "task.completed",
  workspace_id: "workspace-id",
  agent_id: "agent-id",
  task_id: "task-id",
  payload: {},
};

const client = new RealtimeClient({ workspace: "workspace-id" });
client.on("task.completed", (_payload) => {});
client.onAny((_event) => {});
client.close();

function Harness({ children }: { children: ReactNode }) {
  useRealtimeSync("workspace-slug");
  return <RealtimeProvider workspace="workspace-id" workspaceKey="workspace-slug">{children}</RealtimeProvider>;
}

const provider: ComponentType<{ children: ReactNode }> = Harness;
void provider;
