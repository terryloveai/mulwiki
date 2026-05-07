"use client";

import { use, useState, useRef, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@mulwiki/core/api";
import { Button } from "@mulwiki/ui/components/Button";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  Upload,
  FileText,
  File,
  FileImage,
  FileArchive,
  Search,
  Trash2,
} from "lucide-react";

/* ── helpers ── */

function iconForName(name: string): React.ComponentType<{ className?: string }> {
  const ext = name.split(".").pop()?.toLowerCase();
  switch (ext) {
    case "md":
    case "markdown":
      return FileText;
    case "pdf":
      return File;
    case "png":
    case "jpg":
    case "jpeg":
    case "gif":
    case "webp":
      return FileImage;
    case "zip":
    case "tar":
    case "gz":
      return FileArchive;
    default:
      return File;
  }
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/* ── page ── */

export default function SourcesPage({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = use(params);
  const queryClient = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);

  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [file, setFile] = useState<File | null>(null);

  // ── queries ──
  const { data: sources, isLoading } = useQuery({
    queryKey: ["sources", workspaceSlug],
    queryFn: () => api.listSources(workspaceSlug),
  });

  const filtered = useMemo(() => {
    if (!sources) return [];
    if (!search.trim()) return sources;
    const q = search.toLowerCase();
    return sources.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.type.toLowerCase().includes(q)
    );
  }, [sources, search]);

  const selected = useMemo(() => {
    if (!sources || !selectedPath) return null;
    return sources.find((s) => s.path === selectedPath) ?? null;
  }, [sources, selectedPath]);

  // Auto-select first source
  const [didAutoSelect, setDidAutoSelect] = useState(false);
  useMemo(() => {
    if (sources && sources.length > 0 && !didAutoSelect) {
      const first = sources[0]!;
      setSelectedPath(first.path);
      setDidAutoSelect(true);
    }
  }, [sources, didAutoSelect]);

  // ── mutations ──
  const uploadMutation = useMutation({
    mutationFn: () => {
      if (!file) throw new Error("No file selected");
      const formData = new FormData();
      formData.append("file", file);
      return api.uploadSource(workspaceSlug, formData);
    },
    onSuccess: (uploaded) => {
      queryClient.invalidateQueries({ queryKey: ["sources", workspaceSlug] });
      setSelectedPath(uploaded.path);
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (path: string) => api.deleteSource(workspaceSlug, path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sources", workspaceSlug] });
      setSelectedPath(null);
    },
  });

  const handleUpload = (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) return;
    uploadMutation.mutate();
  };

  // ── render ──
  return (
    <div className="flex h-full">
      {/* ── Sidebar ── */}
      <aside className="w-60 shrink-0 border-r border-border flex flex-col bg-card/50">
        <div className="p-3 border-b border-border">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search sources..."
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
              {search ? "No matching sources" : "No sources yet"}
            </div>
          ) : (
            <div className="py-1">
              {filtered.map((s, i) => {
                const Icon = iconForName(s.name);
                const sourceKey = s.path || s.name || `source-${i}`;
                return (
                  <button
                    key={sourceKey}
                    onClick={() => setSelectedPath(s.path)}
                    className={`w-full text-left px-3 py-2 flex items-center gap-2 transition-colors ${
                      selectedPath === s.path
                        ? "bg-accent text-foreground"
                        : "hover:bg-accent/40 text-muted-foreground"
                    }`}
                  >
                    <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm truncate">{s.name}</div>
                      <div className="text-[11px] text-muted-foreground/60">
                        {formatSize(s.size)}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {/* Upload form */}
        <div className="p-3 border-t border-border">
          <form onSubmit={handleUpload} className="space-y-2">
            <Input
              ref={fileRef}
              type="file"
              accept=".pdf,.md,.txt,.docx,.pptx,.png,.jpg,.jpeg,.gif,.webp,.csv,.json,.yaml,.yml"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                setFile(e.target.files?.[0] ?? null)
              }
              className="h-8 text-xs"
            />
            <Button
              type="submit"
              variant="brand"
              size="sm"
              className="w-full text-xs h-7"
              disabled={!file || uploadMutation.isPending}
            >
              {uploadMutation.isPending ? (
                <Spinner className="h-3.5 w-3.5" />
              ) : (
                <>
                  <Upload className="h-3 w-3 mr-1" />
                  Upload
                </>
              )}
            </Button>
          </form>
        </div>
      </aside>

      {/* ── Detail panel ── */}
      <main className="flex-1 overflow-y-auto min-w-0">
        {selected ? (
          <div className="max-w-3xl mx-auto px-8 py-6">
            {/* Header */}
            <div className="flex items-start justify-between mb-6">
              <div className="min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  {(() => {
                    const Icon = iconForName(selected.name);
                    return <Icon className="h-5 w-5 text-brand shrink-0" />;
                  })()}
                  <h2 className="text-lg font-semibold text-foreground truncate">
                    {selected.name}
                  </h2>
                  <Badge variant="secondary" className="text-[10px]">
                    {selected.type}
                  </Badge>
                </div>
                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                  <span>{formatSize(selected.size)}</span>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="text-xs h-7 text-destructive hover:text-destructive shrink-0"
                onClick={() => {
                  if (confirm(`Delete "${selected.name}"?`)) {
                    deleteMutation.mutate(selected.path);
                  }
                }}
              >
                <Trash2 className="h-3 w-3 mr-1" />
                Delete
              </Button>
            </div>

            {/* File path info */}
            <div className="rounded-md border border-border bg-muted/30 px-4 py-3 mb-6">
              <p className="text-xs font-mono text-muted-foreground truncate">
                {selected.path}
              </p>
            </div>

            {/* Text preview for supported types */}
            {selected.name.match(/\.(md|txt|json|yaml|yml|csv|xml|html|css|js|ts|py|go|rs|sh)$/i) ? (
              <div>
                <h3 className="text-sm font-medium text-foreground mb-2">
                  Preview
                </h3>
                <SourcePreview sourcePath={selected.path} workspaceSlug={workspaceSlug} />
              </div>
            ) : (
              <div className="rounded-lg border border-border bg-card p-8 text-center">
                <File className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
                <p className="text-sm text-muted-foreground">
                  Preview not available for this file type.
                </p>
                <p className="text-xs text-muted-foreground/60 mt-1">
                  The file is stored in git and will be used by agents.
                </p>
              </div>
            )}
          </div>
        ) : (
          <div className="flex-1 flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
              <p className="text-sm text-muted-foreground">
                Select a source to view details.
              </p>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

/* ── Source preview (fetches raw content from git) ── */

function SourcePreview({
  workspaceSlug,
  sourcePath,
}: {
  workspaceSlug: string;
  sourcePath: string;
}) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["source-content", workspaceSlug, sourcePath],
    queryFn: async () => {
      const rawRes = await fetch(
        `/api/workspaces/${workspaceSlug}/sources/${encodeURIComponent(sourcePath)}/raw`
      );
      if (!rawRes.ok) throw new Error("Cannot read file");
      return rawRes.text();
    },
    staleTime: 60_000,
  });

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner className="h-5 w-5 text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <p className="text-xs text-muted-foreground py-4">
        Could not load preview.
      </p>
    );
  }

  return (
    <pre className="text-xs font-mono text-muted-foreground whitespace-pre-wrap leading-relaxed max-h-96 overflow-y-auto rounded-md border border-border bg-muted/20 p-4">
      {data}
    </pre>
  );
}
