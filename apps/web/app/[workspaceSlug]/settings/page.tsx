"use client";

import { use } from "react";
import { SettingsPage } from "@mulwiki/views/settings";

export default function Page({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  return <SettingsPage workspaceSlug={workspaceSlug} />;
}
