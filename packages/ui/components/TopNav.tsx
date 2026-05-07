"use client";

import { useTheme } from "../hooks/useTheme";
import { Moon, Sun } from "lucide-react";
import { Button } from "./Button";

export function TopNav() {
  const { theme, toggle } = useTheme();

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center justify-end gap-2 border-b border-border bg-background/95 px-6 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <Button variant="ghost" size="icon" onClick={toggle}>
        {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        <span className="sr-only">Toggle theme</span>
      </Button>
    </header>
  );
}
