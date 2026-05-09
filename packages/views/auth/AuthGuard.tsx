"use client";

import { Suspense, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { meOptions } from "@mulwiki/core/queries";
import { Spinner } from "@mulwiki/ui/components/Spinner";

export function AuthGuard({ children }: { children: React.ReactNode }) {
  return (
    <Suspense fallback={<AuthLoading />}>
      <AuthGuardInner>{children}</AuthGuardInner>
    </Suspense>
  );
}

function AuthGuardInner({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const { isError, isSuccess } = useQuery(meOptions());

  useEffect(() => {
    if (!isError) return;
    const query = searchParams.toString();
    const next = `${pathname}${query ? `?${query}` : ""}`;
    router.replace(`/login?next=${encodeURIComponent(next)}`);
  }, [isError, pathname, router, searchParams]);

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
