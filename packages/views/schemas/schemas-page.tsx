"use client";

import { useState, useCallback, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import type { SchemaWithActive, ValidateSchemaResponse } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  Database,
  GitFork,
  Star,
  Edit3,
  Trash2,
  Plus,
  Search,
  CheckCircle2,
  Circle,
  X,
  Code,
} from "lucide-react";
import { SectionPreview, ValidationPanel } from "./schema-components";
import { parseSections } from "./schema-utils";

/* ── page ── */

export function SchemasPage({ workspaceSlug }: { workspaceSlug: string }) {
  const queryClient = useQueryClient();

  // ── state ──
  const [search, setSearch] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [isCreating, setIsCreating] = useState(false);

  // Edit form state
  const [editName, setEditName] = useState("");
  const [editDesc, setEditDesc] = useState("");
  const [editConfig, setEditConfig] = useState("");
  const [editVersion, setEditVersion] = useState("");

  // Create form state
  const [createName, setCreateName] = useState("");
  const [createDesc, setCreateDesc] = useState("");
  const [createConfig, setCreateConfig] = useState("");

  // Validation
  const [validation, setValidation] = useState<ValidateSchemaResponse | null>(null);

  // ── queries ──
  const { data: schemas, isLoading } = useQuery({
    queryKey: ["schemas", workspaceSlug],
    queryFn: () => api.listSchemas(workspaceSlug),
  });

  const selected = useMemo(() => {
    if (!schemas || !selectedId) return null;
    return schemas.find((s) => s.id === selectedId) ?? null;
  }, [schemas, selectedId]);

  // Fetch full schema content when a schema is selected (content comes from git, not the list endpoint)
  const { data: selectedDetail } = useQuery({
    queryKey: ["schemas", workspaceSlug, selectedId],
    queryFn: () => api.getSchema(workspaceSlug, selectedId!),
    enabled: !!selectedId,
  });

  // Merge selected metadata with full content
  const selectedWithContent = useMemo(() => {
    if (!selected || !selectedDetail) return selected;
    return { ...selected, content: selectedDetail.content ?? selected.content ?? "" };
  }, [selected, selectedDetail]);

  // Auto-select first schema on load
  const [didAutoSelect, setDidAutoSelect] = useState(false);
  useMemo(() => {
    if (schemas && schemas.length > 0 && !didAutoSelect && !selectedId) {
      const first = schemas[0]!;
      setSelectedId(first.id);
      setDidAutoSelect(true);
    }
  }, [schemas, didAutoSelect, selectedId]);

  // ── filtered list ──
  const filtered = useMemo(() => {
    if (!schemas) return [];
    if (!search.trim()) return schemas;
    const q = search.toLowerCase();
    return schemas.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.description.toLowerCase().includes(q)
    );
  }, [schemas, search]);

  // ── mutations ──
  const activateMutation = useMutation({
    mutationFn: (schemaId: string) => api.activateSchema(workspaceSlug, schemaId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["schemas", workspaceSlug] }),
  });

  const forkMutation = useMutation({
    mutationFn: (schemaId: string) => api.forkSchema(workspaceSlug, { schema_id: schemaId }),
    onSuccess: (forked) => {
      queryClient.invalidateQueries({ queryKey: ["schemas", workspaceSlug] });
      setSelectedId(forked.id);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (schemaId: string) => api.deleteSchema(workspaceSlug, schemaId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schemas", workspaceSlug] });
      setSelectedId(null);
    },
  });

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateSchema(workspaceSlug, selectedId!, {
        name: editName,
        description: editDesc,
        version: editVersion,
        content: editConfig,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schemas", workspaceSlug] });
      setIsEditing(false);
      setValidation(null);
    },
    onError: (err: Error) => {
      // Try to parse validation error from response
      try {
        const parsed = JSON.parse(err.message);
        if (parsed.errors || parsed.warnings) {
          setValidation(parsed);
        }
      } catch {
        // Not a validation error
      }
    },
  });

  const createMutation = useMutation({
    mutationFn: () =>
      api.createSchema(workspaceSlug, {
        name: createName,
        description: createDesc,
        content: createConfig,
      }),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["schemas", workspaceSlug] });
      setIsCreating(false);
      setSelectedId(created.id);
      setCreateName("");
      setCreateDesc("");
      setCreateConfig("");
      setValidation(null);
    },
  });

  const validateMutation = useMutation({
    mutationFn: (config: string) => api.validateSchema(workspaceSlug, config),
    onSuccess: (result) => setValidation(result),
  });

  // ── handlers ──
  const startEdit = useCallback(() => {
    if (!selectedWithContent || selectedWithContent.source_type === "builtin") return;
    setEditName(selectedWithContent.name);
    setEditDesc(selectedWithContent.description);
    setEditConfig(selectedWithContent.content);
    setEditVersion(selectedWithContent.version);
    setValidation(null);
    setIsEditing(true);
  }, [selectedWithContent]);

  const cancelEdit = useCallback(() => {
    setIsEditing(false);
    setValidation(null);
  }, []);

  const handleConfigChange = useCallback(
    (value: string) => {
      setEditConfig(value);
      // Debounced validate
      if (value.length > 20 && value !== "{}") {
        const timer = setTimeout(() => {
          validateMutation.mutate(value);
        }, 600);
        return () => clearTimeout(timer);
      } else {
        setValidation(null);
      }
    },
    [validateMutation]
  );

  // ── parsing ──
  const sections = useMemo(() => {
    if (!selectedWithContent || !selectedWithContent.content) return [];
    return parseSections(selectedWithContent.content);
  }, [selectedWithContent]);

  const isCustom = selected && selected.source_type === "user";
  const isBuiltin = selected && selected.source_type === "builtin";

  // ── render ──
  return (
    <div className="flex h-full">
      {/* ── Sidebar ── */}
      <aside className="w-60 shrink-0 border-r border-border flex flex-col bg-card/50">
        <div className="p-3 border-b border-border">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search schemas..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-7 h-8 text-xs"
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          {isLoading ? (
            <div className="flex justify-center py-8">
              <Spinner className="h-5 w-5 text-muted-foreground" />
            </div>
          ) : filtered.length === 0 ? (
            <div className="px-3 py-8 text-center text-xs text-muted-foreground">
              {search ? "No matching schemas" : "No schemas"}
            </div>
          ) : (
            <div className="py-1">
              {filtered.map((s) => (
                <button
                  key={s.id}
                  onClick={() => {
                    setSelectedId(s.id);
                    setIsEditing(false);
                    setIsCreating(false);
                  }}
                  className={`w-full text-left px-3 py-2 flex items-center gap-2 transition-colors ${
                    selectedId === s.id
                      ? "bg-accent text-foreground"
                      : "hover:bg-accent/40 text-muted-foreground"
                  }`}
                >
                  {s.is_active ? (
                    <Star className="h-3.5 w-3.5 shrink-0 text-warning fill-warning" />
                  ) : (
                    <Circle className="h-3.5 w-3.5 shrink-0 text-muted-foreground/30" />
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium truncate">{s.name}</div>
                    <div className="flex items-center gap-1.5 mt-0.5">
                      <Badge
                        variant="secondary"
                        className="text-[10px] px-1 py-0 leading-normal"
                      >
                        {s.source_type}
                      </Badge>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="p-2 border-t border-border">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start text-xs"
            onClick={() => {
              setIsCreating(true);
              setIsEditing(false);
              setSelectedId(null);
              setCreateName("");
              setCreateDesc("");
              setCreateConfig("");
              setValidation(null);
            }}
          >
            <Plus className="h-3.5 w-3.5 mr-1.5" />
            New Schema
          </Button>
        </div>
      </aside>

      {/* ── Main panel ── */}
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {isCreating ? (
          /* ── Create form ── */
          <div className="flex-1 overflow-y-auto p-6">
            <div className="max-w-2xl">
              <h2 className="text-lg font-semibold text-foreground mb-4">Create Schema</h2>

              <div className="space-y-4">
                <div>
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    Name
                  </label>
                  <Input
                    value={createName}
                    onChange={(e) => setCreateName(e.target.value)}
                    placeholder="My Custom Schema"
                    className="h-8 text-sm"
                  />
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    Description
                  </label>
                  <Input
                    value={createDesc}
                    onChange={(e) => setCreateDesc(e.target.value)}
                    placeholder="What this schema defines..."
                    className="h-8 text-sm"
                  />
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground mb-1 flex items-center gap-1">
                    <Code className="h-3.5 w-3.5" />
                    Config (Markdown)
                  </label>
                  <textarea
                    value={createConfig}
                    onChange={(e) => setCreateConfig(e.target.value)}
                    placeholder={`# Schema: my-schema\n\n## Types\n...\n\n## Structure\n...`}
                    rows={16}
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-brand resize-y"
                  />
                </div>

                {validation && <ValidationPanel result={validation} />}

                <div className="flex gap-2">
                  <Button
                    variant="brand"
                    size="sm"
                    disabled={!createName || createMutation.isPending}
                    onClick={() => createMutation.mutate()}
                    className="text-xs"
                  >
                    {createMutation.isPending ? (
                      <Spinner className="h-3.5 w-3.5" />
                    ) : (
                      "Create Schema"
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setIsCreating(false);
                      setValidation(null);
                    }}
                    className="text-xs"
                  >
                    Cancel
                  </Button>
                </div>

                {createMutation.isError && !validation && (
                  <p className="text-xs text-destructive">
                    {(createMutation.error as Error).message}
                  </p>
                )}
              </div>
            </div>
          </div>
        ) : isEditing && selected ? (
          /* ── Edit form ── */
          <div className="flex-1 overflow-y-auto p-6">
            <div className="max-w-2xl">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-foreground">
                  Edit: {selected.name}
                </h2>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={cancelEdit}
                  className="text-xs"
                >
                  <X className="h-3.5 w-3.5 mr-1" />
                  Cancel
                </Button>
              </div>

              <div className="space-y-4">
                <div className="flex gap-3">
                  <div className="flex-1">
                    <label className="text-xs font-medium text-muted-foreground mb-1 block">
                      Name
                    </label>
                    <Input
                      value={editName}
                      onChange={(e) => setEditName(e.target.value)}
                      className="h-8 text-sm"
                    />
                  </div>
                  <div className="w-24">
                    <label className="text-xs font-medium text-muted-foreground mb-1 block">
                      Version
                    </label>
                    <Input
                      value={editVersion}
                      onChange={(e) => setEditVersion(e.target.value)}
                      className="h-8 text-sm"
                    />
                  </div>
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    Description
                  </label>
                  <Input
                    value={editDesc}
                    onChange={(e) => setEditDesc(e.target.value)}
                    className="h-8 text-sm"
                  />
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground mb-1 flex items-center gap-1">
                    <Code className="h-3.5 w-3.5" />
                    Config
                  </label>
                  <textarea
                    value={editConfig}
                    onChange={(e) => handleConfigChange(e.target.value)}
                    rows={20}
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-brand resize-y"
                  />
                </div>

                {validation && <ValidationPanel result={validation} />}

                <div className="flex gap-2">
                  <Button
                    variant="brand"
                    size="sm"
                    disabled={!editName || updateMutation.isPending}
                    onClick={() => updateMutation.mutate()}
                    className="text-xs"
                  >
                    {updateMutation.isPending ? (
                      <Spinner className="h-3.5 w-3.5" />
                    ) : (
                      "Save"
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={cancelEdit}
                    className="text-xs"
                  >
                    Cancel
                  </Button>
                </div>

                {updateMutation.isError && !validation && (
                  <p className="text-xs text-destructive">
                    {(updateMutation.error as Error).message}
                  </p>
                )}
              </div>
            </div>
          </div>
        ) : selected ? (
          /* ── Preview ── */
          <div className="flex-1 overflow-y-auto">
            {/* Header */}
            <div className="border-b border-border px-6 py-4">
              <div className="flex items-start justify-between">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <Database className="h-5 w-5 text-brand shrink-0" />
                    <h2 className="text-lg font-semibold text-foreground truncate">
                      {selected.name}
                    </h2>
                    {selected.is_active && (
                      <Star className="h-4 w-4 text-warning fill-warning shrink-0" />
                    )}
                    <Badge variant="secondary" className="text-[10px]">
                      {selected.source_type}
                    </Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {selected.description || "No description"}
                  </p>
                  <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground">
                    <span>v{selected.version}</span>
                    {selected.derived_from && (
                      <span className="text-muted-foreground/60">
                        forked from another schema
                      </span>
                    )}
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1.5 shrink-0 ml-4">
                  {!selected.is_active && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-xs h-7"
                      onClick={() => activateMutation.mutate(selected.id)}
                      disabled={activateMutation.isPending}
                    >
                      <Star className="h-3 w-3 mr-1" />
                      Set Active
                    </Button>
                  )}
                  {selected.is_active && (
                    <span className="text-xs text-success flex items-center gap-1 px-2">
                      <CheckCircle2 className="h-3 w-3" />
                      Active
                    </span>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-xs h-7"
                    onClick={() => forkMutation.mutate(selected.id)}
                    disabled={forkMutation.isPending}
                  >
                    <GitFork className="h-3 w-3 mr-1" />
                    Fork
                  </Button>
                  {isCustom && (
                    <>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-xs h-7"
                        onClick={startEdit}
                      >
                        <Edit3 className="h-3 w-3 mr-1" />
                        Edit
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-xs h-7 text-destructive hover:text-destructive"
                        onClick={() => {
                          if (confirm(`Delete "${selected.name}"?`)) {
                            deleteMutation.mutate(selected.id);
                          }
                        }}
                      >
                        <Trash2 className="h-3 w-3" />
                      </Button>
                    </>
                  )}
                  {isBuiltin && (
                    <span className="text-[11px] text-muted-foreground/60 italic px-1">
                      read-only
                    </span>
                  )}
                </div>
              </div>
            </div>

            {/* Sections */}
            <div>
              {sections.length === 0 ? (
                <div className="px-6 py-12 text-center text-sm text-muted-foreground">
                  No sections found in schema config.
                </div>
              ) : (
                sections.map((section, i) => (
                  <SectionPreview
                    key={i}
                    title={section.title}
                    content={section.content}
                  />
                ))
              )}
            </div>
          </div>
        ) : (
          /* ── Empty state ── */
          <div className="flex-1 flex items-center justify-center">
            <div className="text-center">
              <Database className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
              <p className="text-sm text-muted-foreground">
                Select a schema or create a new one.
              </p>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
