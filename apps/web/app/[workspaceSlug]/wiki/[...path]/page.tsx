"use client";

import { use } from "react";
import { WikiDetailPage } from "@mulwiki/views/wiki";

export default function Page({
  params,
}: {
  params: Promise<{ workspaceSlug: string; path: string[] }>;
}) {
  const { workspaceSlug, path } = use(params);
  return <WikiDetailPage workspaceSlug={workspaceSlug} path={path} />;
}
