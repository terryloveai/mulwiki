"use client";

import { use } from "react";
import { redirect } from "next/navigation";

export default function CreateAgentRedirect({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  redirect(`/${workspaceSlug}/agents?new=true`);
}
