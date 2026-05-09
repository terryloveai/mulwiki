"use client";

import { use, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AgentsPage } from "@mulwiki/views/agents/agents-page";

export default function AgentsRoute({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const selectedAgentId = searchParams.get("id") ?? undefined;
  const creating = searchParams.get("new") === "true";

  const replaceAgentsUrl = useCallback(
    (suffix = "") => {
      router.replace(`/${workspaceSlug}/agents${suffix}`, { scroll: false });
    },
    [router, workspaceSlug],
  );

  return (
    <AgentsPage
      workspaceSlug={workspaceSlug}
      selectedAgentId={selectedAgentId}
      creating={creating}
      onSelectAgent={(agentId) => replaceAgentsUrl(`?id=${encodeURIComponent(agentId)}`)}
      onStartCreate={() => replaceAgentsUrl("?new=true")}
      onClearSelection={() => replaceAgentsUrl()}
    />
  );
}
