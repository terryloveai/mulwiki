"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { useTheme } from "@mulwiki/ui/hooks/useTheme";
import { cn } from "@mulwiki/ui/lib/cn";
import { useAppNavigation } from "../navigation";

type Tab = "general" | "appearance";

export function SettingsPage({ workspaceSlug }: { workspaceSlug: string }) {
  const navigation = useAppNavigation();
  const queryClient = useQueryClient();
  const { theme, fontSize, setTheme, setFontSize } = useTheme();
  const [tab, setTab] = useState<Tab>("general");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const { data: workspace, isLoading } = useQuery({
    queryKey: ["workspace", workspaceSlug],
    queryFn: () => api.getWorkspace(workspaceSlug),
  });

  useEffect(() => {
    if (workspace) {
      setName(workspace.name);
      setDescription(workspace.description || "");
    }
  }, [workspace]);

  const updateMutation = useMutation({
    mutationFn: () => api.updateWorkspace(workspaceSlug, { name, description }),
    onSuccess: (updated) => {
      queryClient.setQueryData(["workspace", workspaceSlug], updated);
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteWorkspace(workspaceSlug),
    onSuccess: () => navigation.push("/workspaces"),
  });

  const tabs: { id: Tab; label: string }[] = [
    { id: "general", label: "General" },
    { id: "appearance", label: "Appearance" },
  ];

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner className="h-6 w-6 text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="max-w-3xl">
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-foreground">Settings</h1>
        <p className="mt-1 text-muted-foreground">Workspace preferences and appearance.</p>
      </div>

      <div className="mb-6 flex gap-2 border-b border-border">
        {tabs.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setTab(item.id)}
            className={cn(
              "border-b-2 px-3 py-2 text-sm font-medium transition-colors",
              tab === item.id
                ? "border-brand text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>

      {tab === "general" ? (
        <div className="space-y-8">
          <section className="space-y-4">
            <div>
              <h2 className="text-lg font-semibold text-foreground">General</h2>
              <p className="text-sm text-muted-foreground">Edit the workspace profile.</p>
            </div>
            <div className="space-y-1.5">
              <label htmlFor="workspace-name" className="text-sm font-medium text-foreground">
                Workspace name
              </label>
              <Input
                id="workspace-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="workspace-description" className="text-sm font-medium text-foreground">
                Description
              </label>
              <textarea
                id="workspace-description"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                className="min-h-28 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              />
            </div>
            <Button
              variant="brand"
              onClick={() => updateMutation.mutate()}
              disabled={updateMutation.isPending || !name.trim()}
            >
              {updateMutation.isPending ? <Spinner className="h-4 w-4" /> : "Save changes"}
            </Button>
            {updateMutation.isError && (
              <p className="text-sm text-destructive">{(updateMutation.error as Error).message}</p>
            )}
          </section>

          <section className="border-t border-border pt-6">
            <h2 className="text-lg font-semibold text-destructive">Danger zone</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Delete this workspace and all related sources, schemas, jobs, and wiki pages.
            </p>
            <Button
              variant="destructive"
              className="mt-4"
              onClick={() => deleteMutation.mutate()}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? <Spinner className="h-4 w-4" /> : "Delete workspace"}
            </Button>
            {deleteMutation.isError && (
              <p className="mt-2 text-sm text-destructive">{(deleteMutation.error as Error).message}</p>
            )}
          </section>
        </div>
      ) : (
        <div className="space-y-6">
          <section>
            <h2 className="text-lg font-semibold text-foreground">Theme</h2>
            <div className="mt-3 flex gap-2">
              {(["light", "dark"] as const).map((value) => (
                <Button
                  key={value}
                  variant={theme === value ? "brand" : "outline"}
                  onClick={() => setTheme(value)}
                >
                  {value === "light" ? "Light" : "Dark"}
                </Button>
              ))}
            </div>
          </section>
          <section>
            <h2 className="text-lg font-semibold text-foreground">Font size</h2>
            <div className="mt-3 flex gap-2">
              {(["small", "medium", "large"] as const).map((value) => (
                <Button
                  key={value}
                  variant={fontSize === value ? "brand" : "outline"}
                  onClick={() => setFontSize(value)}
                >
                  {{ small: "Small", medium: "Medium", large: "Large" }[value]}
                </Button>
              ))}
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
