"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@mulwiki/core/query-client";

const queryClient = createQueryClient();

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <main className="flex min-h-svh items-center justify-center bg-background px-6 py-12">
        <div className="w-full max-w-sm rounded-lg border border-border bg-card p-6 shadow-sm">
          <div className="mb-6">
            <div className="text-xl font-semibold text-foreground">Mulwiki</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Structured workspace access
            </div>
          </div>
          {children}
        </div>
      </main>
    </QueryClientProvider>
  );
}
