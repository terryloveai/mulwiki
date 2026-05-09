"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@mulwiki/core/query-client";
import { ThemeProvider } from "@mulwiki/ui/hooks/useTheme";

const queryClient = createQueryClient();

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{children}</ThemeProvider>
    </QueryClientProvider>
  );
}
