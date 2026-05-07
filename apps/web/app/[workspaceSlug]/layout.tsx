"use client";

import { use } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "@mulwiki/ui/components/Sidebar";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

export default function WorkspaceLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);

  return (
    <QueryClientProvider client={queryClient}>
      <div className="flex h-svh">
        <Sidebar workspaceSlug={workspaceSlug} />
        <div className="ml-64 flex flex-1 flex-col overflow-hidden">
          <main className="flex-1 overflow-y-auto p-6">{children}</main>
        </div>
      </div>
    </QueryClientProvider>
  );
}
