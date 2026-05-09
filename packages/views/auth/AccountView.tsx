"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { api } from "@mulwiki/core/api";
import { meOptions } from "@mulwiki/core/queries";
import { Button } from "@mulwiki/ui/components/Button";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { LogOut, UserRound } from "lucide-react";

function formatDate(value?: string) {
  if (!value) return "-";
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function AccountView() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data: user, isLoading } = useQuery(meOptions());

  const logout = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: () => {
      queryClient.clear();
      router.replace("/login");
    },
  });

  if (isLoading) {
    return (
      <main className="flex min-h-svh items-center justify-center">
        <Spinner className="h-6 w-6 text-muted-foreground" />
      </main>
    );
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-xl flex-col justify-center px-6 py-12">
      <div className="mb-6 flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-card">
          <UserRound className="h-5 w-5 text-muted-foreground" />
        </div>
        <div>
          <h1 className="text-xl font-semibold text-foreground">Account</h1>
          <p className="text-sm text-muted-foreground">{user?.email}</p>
        </div>
      </div>

      <section className="rounded-lg border border-border bg-card">
        <dl className="divide-y divide-border">
          <div className="grid gap-1 px-4 py-3 sm:grid-cols-[120px_1fr] sm:gap-4">
            <dt className="text-sm text-muted-foreground">Email</dt>
            <dd className="break-all text-sm font-medium text-foreground">{user?.email}</dd>
          </div>
          <div className="grid gap-1 px-4 py-3 sm:grid-cols-[120px_1fr] sm:gap-4">
            <dt className="text-sm text-muted-foreground">Created</dt>
            <dd className="text-sm text-foreground">{formatDate(user?.created_at)}</dd>
          </div>
        </dl>
      </section>

      <div className="mt-6 flex items-center justify-between gap-3">
        <Button type="button" variant="outline" onClick={() => router.push("/workspaces")}>
          Workspaces
        </Button>
        <Button
          type="button"
          variant="destructive"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
        >
          {logout.isPending ? <Spinner className="h-4 w-4" /> : <LogOut className="h-4 w-4" />}
          Log out
        </Button>
      </div>
    </main>
  );
}
