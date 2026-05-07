"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { api } from "@mulwiki/core/api";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { cn } from "@mulwiki/ui/lib/cn";
import { FolderOpen, Plus } from "lucide-react";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

function WorkspacesContent() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [selectedSchemaId, setSelectedSchemaId] = useState("");

  const { data: workspaces, isLoading } = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api.listWorkspaces(),
  });

  const { data: builtinSchemas } = useQuery({
    queryKey: ["schemas", "builtin"],
    queryFn: () => api.listBuiltinSchemas(),
  });

  const createMutation = useMutation({
    mutationFn: async () => {
      const selectedSchema = builtinSchemas?.find((schema) => schema.id === selectedSchemaId);
      if (!selectedSchema) throw new Error("Select a schema");
      const workspace = await api.createWorkspace({ name, slug });
      await api.createSchema(workspace.slug, {
        name: selectedSchema.name,
        description: selectedSchema.description,
        version: selectedSchema.version,
        content: selectedSchema.content,
        source_type: "user",
      });
      return workspace;
    },
    onSuccess: (workspace) => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setName("");
      setSlug("");
      setSelectedSchemaId("");
      setShowForm(false);
      router.push(`/${workspace.slug}/wiki`);
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !slug.trim() || !selectedSchemaId) return;
    createMutation.mutate();
  };

  return (
    <main className="mx-auto max-w-2xl px-6 py-16">
      <div className="mb-10 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Workspaces</h1>
          <p className="mt-1 text-muted-foreground">
            Select a workspace or create a new one.
          </p>
        </div>
        <Button variant="brand" onClick={() => setShowForm(!showForm)}>
          <Plus className="h-4 w-4" />
          New
        </Button>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="mb-8 rounded-lg border border-border bg-card p-6"
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex-1 space-y-1.5">
              <label
                htmlFor="name"
                className="text-sm font-medium text-foreground"
              >
                Name
              </label>
              <Input
                id="name"
                value={name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setName(e.target.value);
                  if (!slug || slug === name.toLowerCase().replace(/\s+/g, "-")) {
                    setSlug(e.target.value.toLowerCase().replace(/\s+/g, "-"));
                  }
                }}
                placeholder="My Workspace"
              />
            </div>
            <div className="flex-1 space-y-1.5">
              <label
                htmlFor="slug"
                className="text-sm font-medium text-foreground"
              >
                Slug
              </label>
              <Input
                id="slug"
                value={slug}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setSlug(e.target.value.toLowerCase().replace(/\s+/g, "-"))
                }
                placeholder="my-workspace"
              />
            </div>
          </div>
          <div className="mt-4 space-y-2">
            <div className="text-sm font-medium text-foreground">Schema</div>
            <div className="grid gap-2">
              {builtinSchemas?.map((schema) => (
                <label
                  key={schema.id}
                  className={cn(
                    "flex cursor-pointer items-start gap-3 rounded-md border border-border p-3 transition-colors",
                    selectedSchemaId === schema.id
                      ? "bg-accent text-accent-foreground"
                      : "hover:bg-accent/60",
                  )}
                >
                  <input
                    type="radio"
                    name="schema"
                    className="mt-1"
                    checked={selectedSchemaId === schema.id}
                    onChange={() => setSelectedSchemaId(schema.id)}
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-foreground">
                      {schema.name}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {schema.description}
                    </span>
                  </span>
                </label>
              ))}
            </div>
          </div>
          <div className="mt-4 flex justify-end">
            <Button
              type="submit"
              variant="brand"
              disabled={createMutation.isPending || !selectedSchemaId}
            >
              {createMutation.isPending ? (
                <Spinner className="h-4 w-4" />
              ) : (
                "Create"
              )}
            </Button>
          </div>
          {createMutation.isError && (
            <p className="mt-3 text-sm text-destructive">
              {(createMutation.error as Error).message}
            </p>
          )}
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner className="h-6 w-6 text-muted-foreground" />
        </div>
      ) : workspaces && workspaces.length > 0 ? (
        <ul className="space-y-2">
          {workspaces.map((ws) => (
            <li key={ws.id}>
              <button
                onClick={() => router.push(`/${ws.slug}/wiki`)}
                className={cn(
                  "flex w-full items-center gap-4 rounded-lg border border-border bg-card p-4",
                  "text-left transition-colors hover:bg-accent hover:text-accent-foreground",
                )}
              >
                <FolderOpen className="h-5 w-5 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="font-medium text-foreground">{ws.name}</div>
                  <div className="truncate text-sm text-muted-foreground">
                    {ws.slug}
                  </div>
                </div>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <div className="py-16 text-center text-muted-foreground">
          No workspaces yet. Create one to get started.
        </div>
      )}
    </main>
  );
}

export default function WorkspacesPage() {
  return (
    <QueryClientProvider client={queryClient}>
      <WorkspacesContent />
    </QueryClientProvider>
  );
}
