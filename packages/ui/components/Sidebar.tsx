"use client";

import { cn } from "@mulwiki/ui/lib/cn";
import { useTheme } from "@mulwiki/ui/hooks/useTheme";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { BookOpen, Bot, Cpu, Database, FileText, Play, Settings } from "lucide-react";
import { useState } from "react";

interface NavItem {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}

export function Sidebar({ workspaceSlug, userEmail }: { workspaceSlug: string; userEmail?: string }) {
  const pathname = usePathname();
  const { theme, fontSize, setTheme, setFontSize } = useTheme();
  const [showAppearance, setShowAppearance] = useState(false);
  const base = `/${workspaceSlug}`;

  const items: NavItem[] = [
    { href: `${base}/wiki`, label: "Wiki", icon: BookOpen },
    { href: `${base}/sources`, label: "Sources", icon: FileText },
    { href: `${base}/schemas`, label: "Schemas", icon: Database },
    { href: `${base}/agents`, label: "Agents", icon: Bot },
    { href: `${base}/runtimes`, label: "Runtimes", icon: Cpu },
    { href: `${base}/jobs`, label: "Jobs", icon: Play },
    { href: `${base}/settings`, label: "Settings", icon: Settings },
  ];

  return (
    <aside className="fixed left-0 top-0 z-30 flex h-full w-64 flex-col border-r border-sidebar-border bg-sidebar">
      {/* Logo / Workspace */}
      <div className="flex h-12 items-center gap-2.5 border-b border-sidebar-border px-4">
        <img src="/logo.svg" alt="Mulwiki" className="h-6 w-6" />
        <span className="truncate text-sm font-semibold text-sidebar-foreground">
          {workspaceSlug}
        </span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-0.5 overflow-y-auto p-2">
        {items.map((item) => {
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-2.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                active
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
              )}
            >
              <item.icon className="h-4 w-4 flex-shrink-0" />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Footer */}
      <footer className="border-t border-sidebar-border p-2">
        {showAppearance && (
          <div className="mb-2 space-y-3 rounded-md border border-sidebar-border bg-sidebar-accent/40 p-3">
            <div>
              <div className="mb-2 text-xs font-medium uppercase text-sidebar-foreground/50">
                Theme
              </div>
              <div className="grid grid-cols-2 gap-1">
                {(["light", "dark"] as const).map((value) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setTheme(value)}
                    className={cn(
                      "rounded-md px-2 py-1.5 text-xs font-medium capitalize transition-colors",
                      theme === value
                        ? "bg-brand text-brand-foreground"
                        : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-foreground",
                    )}
                  >
                    {value}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-2 text-xs font-medium uppercase text-sidebar-foreground/50">
                Font
              </div>
              <div className="grid grid-cols-3 gap-1">
                {(["small", "medium", "large"] as const).map((value) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setFontSize(value)}
                    className={cn(
                      "rounded-md px-2 py-1.5 text-xs font-medium capitalize transition-colors",
                      fontSize === value
                        ? "bg-brand text-brand-foreground"
                        : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-foreground",
                    )}
                  >
                    {{ small: "S", medium: "M", large: "L" }[value]}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}
        <div className="flex items-center gap-2">
          <div className="min-w-0 flex-1">
            {userEmail ? (
              <Link
                href="/account"
                title={userEmail}
                className="block truncate text-xs text-muted-foreground hover:text-sidebar-foreground"
              >
                {userEmail}
              </Link>
            ) : (
              <div className="truncate text-xs text-muted-foreground">
                Not signed in
              </div>
            )}
          </div>
          <button
            type="button"
            aria-label="Appearance settings"
            title="Appearance settings"
            onClick={() => setShowAppearance((value) => !value)}
            className={cn(
              "inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors",
              showAppearance
                ? "bg-sidebar-accent text-sidebar-accent-foreground"
                : "hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
            )}
          >
            <Settings className="h-3.5 w-3.5" />
          </button>
        </div>
      </footer>
    </aside>
  );
}
