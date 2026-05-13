"use client";

import { Suspense } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@mulwiki/core/query-client";
import { ThemeProvider } from "@mulwiki/ui/hooks/useTheme";
import { WebNavigationProvider } from "../platform/navigation";

const queryClient = createQueryClient();

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <Suspense>
          <WebNavigationProvider>{children}</WebNavigationProvider>
        </Suspense>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
