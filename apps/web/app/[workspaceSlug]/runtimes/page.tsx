"use client";

import { use } from "react";
import { RuntimesPage } from "@mulwiki/views/runtimes";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <RuntimesPage workspaceSlug={workspaceSlug} />;
}
