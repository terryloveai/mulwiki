"use client";

import { use } from "react";
import { WikiIndexPage } from "@mulwiki/views/wiki";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <WikiIndexPage workspaceSlug={workspaceSlug} />;
}
