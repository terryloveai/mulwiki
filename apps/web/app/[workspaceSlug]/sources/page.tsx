"use client";

import { use } from "react";
import { SourcesPage } from "@mulwiki/views/sources";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <SourcesPage workspaceSlug={workspaceSlug} />;
}
