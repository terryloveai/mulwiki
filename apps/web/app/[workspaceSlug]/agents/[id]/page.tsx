"use client";

import { use } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { runtimeListOptions } from "@mulwiki/core/agents/queries";
import { AgentDetailPanel } from "@mulwiki/views/agents/agent-detail-panel";
import { Button } from "@mulwiki/ui/components/Button";
import { ArrowLeft } from "lucide-react";

export default function AgentDetailRoute({
  params,
}: {
  params: Promise<{ workspaceSlug: string; id: string }>;
}) {
  const { workspaceSlug, id } = use(params);
  const router = useRouter();
  const { data: runtimes } = useQuery(runtimeListOptions(workspaceSlug));

  return (
    <div>
      <Link href={`/${workspaceSlug}/agents`}>
        <Button variant="ghost" size="sm" className="mb-2">
          <ArrowLeft className="h-4 w-4" /> Agents
        </Button>
      </Link>
      <AgentDetailPanel
        workspaceSlug={workspaceSlug}
        agentId={id}
        runtimes={runtimes ?? []}
        onArchive={() => router.push(`/${workspaceSlug}/agents`)}
      />
    </div>
  );
}
