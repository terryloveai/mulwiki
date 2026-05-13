"use client";

import { Suspense, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { meOptions } from "@mulwiki/core/queries";
import { useAppNavigation } from "../navigation";
import { Spinner } from "@mulwiki/ui/components/Spinner";

export function AuthGuard({ children }: { children: React.ReactNode }) {
  return (
    <Suspense fallback={<AuthLoading />}>
      <AuthGuardInner>{children}</AuthGuardInner>
    </Suspense>
  );
}

function AuthGuardInner({ children }: { children: React.ReactNode }) {
  const navigation = useAppNavigation();
  const { isError, isSuccess } = useQuery(meOptions());

  useEffect(() => {
    if (!isError) return;
    const next = `${navigation.currentPath || "/"}${navigation.currentSearch ? `?${navigation.currentSearch}` : ""}`;
    navigation.replace(`/login?next=${encodeURIComponent(next)}`);
  }, [isError, navigation]);

  if (isSuccess) {
    return <>{children}</>;
  }

  return (
    <AuthLoading />
  );
}

function AuthLoading() {
  return (
    <div className="flex min-h-svh items-center justify-center">
      <Spinner className="h-6 w-6 text-muted-foreground" />
    </div>
  );
}
