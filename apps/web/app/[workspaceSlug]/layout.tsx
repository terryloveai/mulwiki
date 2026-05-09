"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import { RealtimeProvider } from "@mulwiki/core/realtime/provider";
import { useRealtimeSync } from "@mulwiki/core/realtime/use-realtime-sync";
import { meOptions } from "@mulwiki/core/queries";
import { workspaceDetailOptions } from "@mulwiki/core/workspace/queries";
import { AuthGuard } from "@mulwiki/views/auth/AuthGuard";
import { Sidebar } from "@mulwiki/ui/components/Sidebar";

function WorkspaceRealtimeSync({ workspaceSlug }: { workspaceSlug: string }) {
  useRealtimeSync(workspaceSlug);
  return null;
}

export default function WorkspaceLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);

  return (
    <AuthGuard>
      <WorkspaceShell workspaceSlug={workspaceSlug}>{children}</WorkspaceShell>
    </AuthGuard>
  );
}

function WorkspaceShell({
  children,
  workspaceSlug,
}: {
  children: React.ReactNode;
  workspaceSlug: string;
}) {
  const { data: workspace } = useQuery(workspaceDetailOptions(workspaceSlug));
  const { data: user } = useQuery(meOptions());

  return (
    <RealtimeProvider workspace={workspace?.id ?? null} workspaceKey={workspaceSlug}>
      <WorkspaceRealtimeSync workspaceSlug={workspaceSlug} />
      <div className="flex h-svh">
        <Sidebar workspaceSlug={workspaceSlug} userEmail={user?.email} />
        <div className="ml-64 flex flex-1 flex-col overflow-hidden">
          <main className="flex-1 overflow-y-auto p-6">{children}</main>
        </div>
      </div>
    </RealtimeProvider>
  );
}
