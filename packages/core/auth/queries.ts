import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const authKeys = {
  all: () => ["auth"] as const,
  me: () => [...authKeys.all(), "me"] as const,
};

export function meOptions() {
  return queryOptions({
    queryKey: authKeys.me(),
    queryFn: () => api.me(),
    retry: false,
  });
}
