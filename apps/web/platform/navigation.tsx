"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo } from "react";
import { NavigationProvider, type AppLinkProps } from "@mulwiki/views/navigation";

function WebLink({ href, ...props }: AppLinkProps) {
  return <Link href={href} {...props} />;
}

export function WebNavigationProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const value = useMemo(
    () => ({
      navigation: {
        currentPath: pathname,
        currentSearch: searchParams.toString(),
        push: (href: string) => router.push(href),
        replace: (href: string) => router.replace(href),
      },
      LinkComponent: WebLink,
    }),
    [pathname, router, searchParams],
  );

  return <NavigationProvider value={value}>{children}</NavigationProvider>;
}
