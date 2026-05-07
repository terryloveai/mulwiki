"use client";

import { use, useState, useMemo, useEffect, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import { visit } from "unist-util-visit";
import type { Root, Text, Element } from "hast";
import { api } from "@mulwiki/core/api";
import type { WikiPage as WikiPageType, WikiBacklink } from "@mulwiki/core/types";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Input } from "@mulwiki/ui/components/Input";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import {
  FileText,
  Folder,
  FolderOpen,
  ChevronRight,
  ChevronDown,
  Search,
  Layers,
  Home,
  Link2,
  ListTree,
  AlertCircle,
} from "lucide-react";

/* ── types ── */

interface TreeNode {
  name: string;
  fullPath: string;
  kind: "folder" | "page";
  children: TreeNode[];
  page?: WikiPageType;
}

interface TocItem {
  level: number;
  text: string;
  id: string;
}

/* ── helpers ── */

function buildTree(pages: WikiPageType[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const page of pages) {
    const segments = page.path.split("/");
    let siblings = root;
    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i]!;
      const isLast = i === segments.length - 1;
      const fullPath = segments.slice(0, i + 1).join("/");
      let node = siblings.find((n) => n.name === seg);
      if (node) {
        if (isLast) { node.kind = "page"; node.page = page; }
      } else {
        node = {
          name: seg, fullPath,
          kind: isLast ? "page" : "folder",
          children: [],
          page: isLast ? page : undefined,
        };
        siblings.push(node);
      }
      siblings = node!.children;
    }
  }
  return root;
}

function countPages(nodes: TreeNode[]): number {
  let count = 0;
  for (const n of nodes) {
    if (n.kind === "page") count++;
    count += countPages(n.children);
  }
  return count;
}

function findPageByPath(nodes: TreeNode[], path: string): WikiPageType | null {
  for (const n of nodes) {
    if (n.kind === "page" && n.fullPath === path) return n.page!;
    const found = findPageByPath(n.children, path);
    if (found) return found;
  }
  return null;
}

function isAncestorSelected(node: TreeNode, selectedPath: string | null): boolean {
  if (!selectedPath) return false;
  return selectedPath.startsWith(node.fullPath + "/");
}

function extractToc(markdown: string): TocItem[] {
  const items: TocItem[] = [];
  const lines = markdown.split("\n");
  for (const line of lines) {
    const match = line.match(/^(#{1,4})\s+(.+)/);
    if (!match) continue;
    const level = match[1]!.length;
    const text = match[2]!.replace(/[`*_~\[\]()]/g, "").trim();
    const id = text.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9\u4e00-\u9fff\-]/g, "");
    items.push({ level, text, id });
  }
  return items;
}

function slugify(text: string): string {
  return text.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9\u4e00-\u9fff\-]/g, "");
}

/* ── rehype plugin: wikilinks → <a class="wikilink"> ── */

function rehypeWikilinks() {
  return (tree: Root) => {
    visit(tree, "text", (node: Text, index: number | undefined, parent: any) => {
      if (!parent || typeof index !== "number") return;

      // Skip text inside <code> or <pre> — don't touch code blocks.
      let ancestor = parent;
      while (ancestor) {
        if (ancestor.tagName === "code" || ancestor.tagName === "pre") return;
        ancestor = ancestor.parent;
      }

      const text = node.value;
      const regex = /\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g;

      const nodes: Array<{ type: string; value?: string; properties?: Record<string, unknown>; tagName?: string; children?: Array<{ type: string; value: string }> }> = [];
      let lastIdx = 0;
      let match: RegExpExecArray | null;

      while ((match = regex.exec(text)) !== null) {
        // Text before match
        if (match.index > lastIdx) {
          nodes.push({ type: "text", value: text.slice(lastIdx, match.index) });
        }

        const target = match[1]!.trim();
        const alias = match[2]?.trim() ?? target;

        nodes.push({
          type: "element",
          tagName: "a",
          properties: {
            className: "wikilink",
            "data-target": target,
            href: "#",
          },
          children: [{ type: "text", value: alias }],
        });

        lastIdx = match.index + match[0].length;
      }

      // Trailing text
      if (lastIdx < text.length) {
        nodes.push({ type: "text", value: text.slice(lastIdx) });
      }

      if (nodes.length > 0) {
        parent.children.splice(index, 1, ...(nodes as any));
      }
    });
  };
}

/* ── tree component ── */

function WikiTreeNode({
  node, depth, selectedPath, onSelect,
}: {
  node: TreeNode; depth: number; selectedPath: string | null;
  onSelect: (path: string) => void;
}) {
  const [open, setOpen] = useState(isAncestorSelected(node, selectedPath) || depth === 0);
  const isSelected = selectedPath === node.fullPath;

  if (node.kind === "page") {
    return (
      <button
        onClick={() => onSelect(node.fullPath)}
        className={`w-full text-left flex items-center gap-1.5 px-3 py-1.5 text-sm transition-colors ${
          isSelected
            ? "bg-accent text-foreground font-medium"
            : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
        }`}
        style={{ paddingLeft: `${12 + depth * 16}px` }}
      >
        <FileText className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{node.name}</span>
        {node.page && (
          <Badge variant="secondary" className="text-[10px] px-1 py-0 ml-auto shrink-0">
            {node.page.type}
          </Badge>
        )}
      </button>
    );
  }

  return (
    <div>
      <button
        onClick={() => setOpen(!open)}
        className="w-full text-left flex items-center gap-1 px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent/40 hover:text-foreground transition-colors"
        style={{ paddingLeft: `${12 + depth * 16}px` }}
      >
        {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0" />}
        {open ? <FolderOpen className="h-3.5 w-3.5 shrink-0" /> : <Folder className="h-3.5 w-3.5 shrink-0" />}
        <span className="truncate font-medium">{node.name}</span>
        <span className="text-[11px] text-muted-foreground/50 ml-auto">{countPages(node.children)}</span>
      </button>
      {open && node.children.map((child) => (
        <WikiTreeNode key={child.fullPath} node={child} depth={depth + 1} selectedPath={selectedPath} onSelect={onSelect} />
      ))}
    </div>
  );
}

/* ── main view ── */

export default function WikiIndexPage({ params }: { params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [showToc, setShowToc] = useState(false);

  const { data: pages, isLoading } = useQuery({
    queryKey: ["wiki", workspaceSlug],
    queryFn: () => api.listWikiPages(workspaceSlug),
  });

  const tree = useMemo(() => (pages ? buildTree(pages) : []), [pages]);
  const selected = useMemo(
    () => (selectedPath ? findPageByPath(tree, selectedPath) : null),
    [tree, selectedPath]
  );

  // Auto-select first page
  const [didAutoSelect, setDidAutoSelect] = useState(false);
  useMemo(() => {
    if (pages && pages.length > 0 && !didAutoSelect) {
      setSelectedPath(pages[0]!.path);
      setDidAutoSelect(true);
    }
  }, [pages, didAutoSelect]);

  // Backlinks
  const { data: backlinks } = useQuery({
    queryKey: ["wiki-backlinks", workspaceSlug, selectedPath],
    queryFn: () => api.getWikiBacklinks(workspaceSlug, selectedPath!),
    enabled: !!selectedPath,
  });

  // Wikilink existence resolution — collect targets from content and batch-check
  const [wikilinkResolved, setWikilinkResolved] = useState<Record<string, { exists: boolean; path?: string }>>({});

  useEffect(() => {
    if (!selected?.content) { setWikilinkResolved({}); return; }
    const regex = /\[\[([^\]|]+)(?:\|[^\]]+)?\]\]/g;
    const targets = new Set<string>();
    let m: RegExpExecArray | null;
    while ((m = regex.exec(selected.content)) !== null) {
      targets.add(m[1]!.trim());
    }
    if (targets.size === 0) { setWikilinkResolved({}); return; }

    const paths = Array.from(targets);
    api.resolveWikiLinks(workspaceSlug, paths).then((res) => {
      setWikilinkResolved(res.resolved);
    }).catch(() => {
      setWikilinkResolved({});
    });
  }, [selected?.content, workspaceSlug]);

  const filteredTree = useMemo(() => {
    if (!search.trim()) return tree;
    const q = search.toLowerCase();
    const matchingPages = (pages || []).filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.path.toLowerCase().includes(q) ||
        p.type.toLowerCase().includes(q)
    );
    return buildTree(matchingPages);
  }, [tree, search, pages]);

  const breadcrumbs = useMemo(() => {
    if (!selectedPath) return [];
    const segs = selectedPath.split("/");
    return segs.map((seg, i) => ({
      label: seg,
      path: segs.slice(0, i + 1).join("/"),
      last: i === segs.length - 1,
    }));
  }, [selectedPath]);

  const toc = useMemo(() => (selected?.content ? extractToc(selected.content) : []), [selected?.content]);

  const typeVariant: Record<string, "brand" | "secondary" | "success" | "warning" | "outline"> = {
    fact: "success",
    concept: "brand",
    reference: "secondary",
    guide: "warning",
    definition: "outline",
  };

  // Handle wikilink clicks — navigate within the wiki, using canonical path from resolve
  const handleWikilinkClick = useCallback((target: string) => {
    const info = wikilinkResolved[target];
    // Use canonical path if available, otherwise normalize
    const normalized = info?.path || (target.startsWith("/") ? target : "/" + target);
    const page = findPageByPath(tree, normalized);
    if (page) {
      setSelectedPath(page.path);
    }
  }, [tree, wikilinkResolved]);

  const remarkPlugins = [remarkGfm] as any;
  const rehypePlugins = [rehypeRaw, rehypeWikilinks] as any;

  return (
    <div className="flex h-full">
      {/* ── Tree sidebar ── */}
      <aside className="w-60 shrink-0 border-r border-border flex flex-col bg-card/50">
        <div className="p-3 border-b border-border">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input placeholder="Search pages..." value={search} onChange={(e) => setSearch(e.target.value || "")}
              className="pl-7 h-8 text-xs" />
          </div>
        </div>
        <div className="flex-1 overflow-y-auto py-1">
          {isLoading ? (
            <div className="flex justify-center py-8"><Spinner className="h-5 w-5 text-muted-foreground" /></div>
          ) : filteredTree.length === 0 ? (
            <div className="px-3 py-8 text-center text-xs text-muted-foreground">
              {search ? "No matching pages" : "No wiki pages yet"}
            </div>
          ) : (
            filteredTree.map((node) => (
              <WikiTreeNode key={node.fullPath} node={node} depth={0} selectedPath={selectedPath} onSelect={setSelectedPath} />
            ))
          )}
        </div>
        <div className="p-3 border-t border-border">
          <div className="text-[11px] text-muted-foreground">{pages?.length ?? 0} pages</div>
        </div>
      </aside>

      {/* ── Content ── */}
      <main className="flex-1 overflow-y-auto min-w-0">
        {selected ? (
          <div className="flex">
            {/* Main content */}
            <div className="flex-1 max-w-3xl px-8 py-6 min-w-0">
              {/* Breadcrumbs */}
              <nav className="flex items-center gap-1.5 text-sm text-muted-foreground mb-6">
                <Home className="h-3.5 w-3.5" />
                {breadcrumbs.map((crumb) => (
                  <span key={crumb.path} className="flex items-center gap-1.5">
                    <ChevronRight className="h-3.5 w-3.5" />
                    {crumb.last ? (
                      <span className="text-foreground font-medium">{crumb.label}</span>
                    ) : (
                      <button onClick={() => setSelectedPath(crumb.path)} className="hover:text-foreground">
                        {crumb.label}
                      </button>
                    )}
                  </span>
                ))}
              </nav>

              {/* Header */}
              <header className="mb-8">
                <div className="flex items-center gap-3 flex-wrap">
                  <h1 className="text-2xl font-bold text-foreground">{selected.title}</h1>
                  <Badge variant={typeVariant[selected.type] ?? "outline"}>{selected.type}</Badge>
                  {selected?.layer && parseInt(selected.layer) > 0 && (
                    <span className="inline-flex items-center gap-1 text-sm text-muted-foreground">
                      <Layers className="h-3.5 w-3.5" />Layer {selected.layer}
                    </span>
                  )}
                </div>
              </header>

              {/* Content — markdown rendered */}
              <div className="prose prose-neutral dark:prose-invert max-w-none
                prose-headings:scroll-mt-20 prose-headings:relative
                prose-a:text-blue-600 prose-a:no-underline hover:prose-a:underline
                prose-img:rounded-lg prose-img:shadow-sm prose-img:max-w-full
                prose-code:bg-muted prose-code:rounded prose-code:px-1.5 prose-code:py-0.5 prose-code:text-sm
                prose-pre:bg-muted prose-pre:border prose-pre:border-border
                prose-table:border prose-table:border-border
                prose-th:bg-muted prose-th:px-3 prose-th:py-2 prose-th:text-sm
                prose-td:px-3 prose-td:py-2 prose-td:text-sm
                [&_.wikilink]:text-blue-600 [&_.wikilink]:no-underline hover:[&_.wikilink]:underline
                [&_.wikilink]:cursor-pointer [&_.wikilink]:border-b [&_.wikilink]:border-dashed [&_.wikilink]:border-blue-400
                [&_.wikilink-missing]:text-red-500 [&_.wikilink-missing]:border-red-300
                [&_.wikilink-missing]:after:content-['_(missing)'] [&_.wikilink-missing]:after:text-[10px] [&_.wikilink-missing]:after:text-red-400"
              >
                {selected.content ? (
                  <ReactMarkdown
                    remarkPlugins={remarkPlugins}
                    rehypePlugins={rehypePlugins}
                    components={{
                      // Wikilinks rendered as clickable internal links
                      a({ href, className, children, ...props }: any) {
                        const isWikilink = className?.includes?.("wikilink");
                        const target = (props as any)?.["data-target"] as string | undefined;

                        if (isWikilink && target) {
                          const info = wikilinkResolved[target];
                          const exists = info?.exists ?? true; // optimistic
                          const cls = `wikilink ${exists ? "" : "wikilink-missing"}`;
                          return (
                            <a
                              className={cls}
                              href="#"
                              data-target={target}
                              onClick={(e) => {
                                e.preventDefault();
                                handleWikilinkClick(target);
                              }}
                            >
                              {children}
                            </a>
                          );
                        }

                        // External links: open in new tab
                        if (href && (href.startsWith("http://") || href.startsWith("https://"))) {
                          return <a href={href} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">{children}</a>;
                        }

                        // Regular internal links (standard markdown)
                        return <a href={href} className="text-blue-600 hover:underline">{children}</a>;
                      },

                      // Images from workspace sources
                      img({ src, alt, ...props }: any) {
                        if (!src) return null;
                        // If the src is a relative path in sources/, rewrite to API
                        let finalSrc = src;
                        if (src.startsWith("sources/") || src.startsWith("/sources/")) {
                          const cleanPath = src.replace(/^\/?sources\//, "");
                          finalSrc = `/api/workspaces/${workspaceSlug}/sources/${encodeURIComponent(cleanPath)}/raw`;
                        }
                        return (
                          <img
                            src={finalSrc}
                            alt={alt || ""}
                            {...props}
                            onError={(e) => {
                              // Show broken-image fallback instead of raw browser icon
                              const target = e.currentTarget;
                              target.style.display = "none";
                              const fallback = document.createElement("span");
                              fallback.className = "inline-flex items-center gap-1.5 px-3 py-2 rounded border border-border bg-muted/50 text-xs text-muted-foreground italic";
                              fallback.textContent = `📷 ${alt || src}`;
                              target.parentNode?.insertBefore(fallback, target);
                            }}
                          />
                        );
                      },

                      // Heading with anchor for TOC
                      h1({ children, ...props }: any) {
                        const id = slugify(String(children));
                        return <h1 id={id} {...props}>{children}</h1>;
                      },
                      h2({ children, ...props }: any) {
                        const id = slugify(String(children));
                        return <h2 id={id} {...props}>{children}</h2>;
                      },
                      h3({ children, ...props }: any) {
                        const id = slugify(String(children));
                        return <h3 id={id} {...props}>{children}</h3>;
                      },
                    }}
                  >
                    {selected.content}
                  </ReactMarkdown>
                ) : (
                  <p className="text-muted-foreground italic text-sm">No content.</p>
                )}
              </div>

              {/* ── Backlinks ── */}
              {backlinks && backlinks.length > 0 && (
                <section className="mt-12 pt-8 border-t border-border">
                  <h2 className="text-sm font-semibold text-muted-foreground mb-4 flex items-center gap-2">
                    <Link2 className="h-4 w-4" /> Referenced by ({backlinks.length})
                  </h2>
                  <div className="space-y-3">
                    {backlinks.map((bl) => (
                      <button
                        key={bl.path}
                        onClick={() => setSelectedPath(bl.path)}
                        className="block w-full text-left p-3 rounded-lg border border-border hover:bg-accent/30 transition-colors group"
                      >
                        <div className="flex items-center gap-2 mb-1">
                          <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium text-foreground group-hover:text-blue-600 transition-colors">
                            {bl.title || bl.path}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground line-clamp-2 pl-5.5">{bl.snippet}</p>
                      </button>
                    ))}
                  </div>
                </section>
              )}

              {/* ── Empty backlinks state ── */}
              {backlinks && backlinks.length === 0 && (
                <section className="mt-12 pt-8 border-t border-border">
                  <p className="text-xs text-muted-foreground flex items-center gap-1.5">
                    <Link2 className="h-3.5 w-3.5" /> No backlinks yet — link to this page from another using <code className="text-[11px] bg-muted px-1 rounded">[[{selected.path}]]</code>
                  </p>
                </section>
              )}
            </div>

            {/* ── TOC sidebar ── */}
            {toc.length > 0 && (
              <aside className="w-48 shrink-0 border-l border-border p-4 sticky top-0 h-fit max-h-screen overflow-y-auto hidden xl:block">
                <div className="flex items-center gap-1.5 mb-3">
                  <ListTree className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wide">On this page</span>
                </div>
                <nav className="space-y-0.5">
                  {toc.map((item) => (
                    <a
                      key={item.id}
                      href={`#${item.id}`}
                      className="block text-[11px] text-muted-foreground hover:text-foreground py-0.5 transition-colors truncate"
                      style={{ paddingLeft: `${(item.level - 1) * 10}px` }}
                      onClick={(e) => {
                        e.preventDefault();
                        document.getElementById(item.id)?.scrollIntoView({ behavior: "smooth" });
                      }}
                    >
                      {item.text}
                    </a>
                  ))}
                </nav>
              </aside>
            )}
          </div>
        ) : (
          <div className="flex-1 flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
              <p className="text-sm text-muted-foreground">Select a page from the tree.</p>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
