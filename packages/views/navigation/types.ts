"use client";

import type { AnchorHTMLAttributes, ComponentType, ReactNode } from "react";

export interface AppNavigation {
  currentPath: string;
  currentSearch: string;
  push: (href: string) => void;
  replace: (href: string) => void;
}

export type AppLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> & {
  href: string;
  children?: ReactNode;
};

export type AppLinkComponent = ComponentType<AppLinkProps>;

export interface NavigationContextValue {
  navigation: AppNavigation;
  LinkComponent?: AppLinkComponent;
}
