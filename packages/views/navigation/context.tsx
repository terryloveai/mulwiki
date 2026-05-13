"use client";

import { createContext, useContext } from "react";
import type { AppNavigation, NavigationContextValue } from "./types";

const fallbackNavigation: AppNavigation = {
  currentPath: "",
  currentSearch: "",
  push: (href) => {
    if (typeof window !== "undefined") {
      window.location.assign(href);
    }
  },
  replace: (href) => {
    if (typeof window !== "undefined") {
      window.location.replace(href);
    }
  },
};

const NavigationContext = createContext<NavigationContextValue>({
  navigation: fallbackNavigation,
});

export function NavigationProvider({
  value,
  children,
}: {
  value: NavigationContextValue;
  children: React.ReactNode;
}) {
  return <NavigationContext.Provider value={value}>{children}</NavigationContext.Provider>;
}

export function useAppNavigation() {
  return useContext(NavigationContext).navigation;
}

export function useAppLinkComponent() {
  return useContext(NavigationContext).LinkComponent;
}
