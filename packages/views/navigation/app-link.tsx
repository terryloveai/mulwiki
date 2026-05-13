"use client";

import { useAppLinkComponent } from "./context";
import type { AppLinkProps } from "./types";

export function AppLink(props: AppLinkProps) {
  const LinkComponent = useAppLinkComponent();
  if (LinkComponent) {
    return <LinkComponent {...props} />;
  }
  return <a {...props} />;
}
