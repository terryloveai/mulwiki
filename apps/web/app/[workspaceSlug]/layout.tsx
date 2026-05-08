"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import { workspaceQueries } from "@mulwiki/core/queries";
import { useWorkspaceRealtime } from "@mulwiki/core/hooks";
import { Sidebar } from "@mulwiki/ui/components/Sidebar";

export default function WorkspaceLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const { data: workspace } = useQuery(workspaceQueries.detail(workspaceSlug));
  useWorkspaceRealtime(workspace?.id);

  return (
    <div className="flex h-svh">
      <Sidebar workspaceSlug={workspaceSlug} />
      <div className="ml-64 flex flex-1 flex-col overflow-hidden">
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
