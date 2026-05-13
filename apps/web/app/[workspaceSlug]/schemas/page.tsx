"use client";

import { use } from "react";
import { SchemasPage } from "@mulwiki/views/schemas";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <SchemasPage workspaceSlug={workspaceSlug} />;
}
