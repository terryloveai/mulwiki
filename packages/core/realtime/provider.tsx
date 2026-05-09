"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { RealtimeClient } from "./ws-client";

const RealtimeContext = createContext<RealtimeClient | null>(null);

export function RealtimeProvider({
  workspace,
  workspaceKey,
  agentId,
  children,
}: {
  workspace?: string | null;
  workspaceKey?: string;
  agentId?: string;
  children: ReactNode;
}) {
  const [client, setClient] = useState<RealtimeClient | null>(null);

  useEffect(() => {
    if (!workspace) {
      setClient(null);
      return;
    }

    const nextClient = new RealtimeClient({ workspace, agentId });
    setClient(nextClient);

    return () => {
      nextClient.close();
    };
  }, [workspace, agentId]);

  void workspaceKey;

  return <RealtimeContext.Provider value={client}>{children}</RealtimeContext.Provider>;
}

export function useRealtimeClient() {
  return useContext(RealtimeContext);
}
