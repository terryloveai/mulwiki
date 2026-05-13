"use client";

import { use } from "react";
import { JobsPage } from "@mulwiki/views/jobs";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <JobsPage workspaceSlug={workspaceSlug} />;
}
