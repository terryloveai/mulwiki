"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { api } from "@mulwiki/core/api";
import { Badge } from "@mulwiki/ui/components/Badge";
import { Spinner } from "@mulwiki/ui/components/Spinner";
import { ChevronRight, Home, Layers } from "lucide-react";

const typeVariant: Record<string, "brand" | "secondary" | "success" | "warning" | "outline"> = {
  concept: "brand",
  reference: "secondary",
  guide: "success",
  definition: "warning",
};

function variantForType(type: string) {
  return typeVariant[type] ?? "outline";
}

export default function WikiPageDetail({
  params,
}: {
  params: Promise<{ workspaceSlug: string; path: string[] }>;
}) {
  const { workspaceSlug, path } = use(params);
  const pagePath = path.join("/");

  const { data: page, isLoading, error } = useQuery({
    queryKey: ["wiki", workspaceSlug, pagePath],
    queryFn: () => api.getWikiPage(workspaceSlug, pagePath),
  });

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner className="h-6 w-6 text-muted-foreground" />
      </div>
    );
  }

  if (error || !page) {
    return (
      <div className="py-16 text-center">
        <p className="text-muted-foreground">
          {(error as Error)?.message || "Page not found."}
        </p>
        <Link
          href={`/${workspaceSlug}/wiki`}
          className="mt-4 inline-block text-sm text-brand hover:underline"
        >
          Back to wiki index
        </Link>
      </div>
    );
  }

  const segments = pagePath.split("/");
  const breadcrumbs = segments.map((seg, i) => ({
    label: seg,
    href: `/${workspaceSlug}/wiki/${segments.slice(0, i + 1).join("/")}`,
    last: i === segments.length - 1,
  }));

  return (
    <article className="mx-auto max-w-3xl">
      {/* Breadcrumb */}
      <nav className="mb-6 flex items-center gap-1.5 text-sm text-muted-foreground">
        <Link
          href={`/${workspaceSlug}/wiki`}
          className="hover:text-foreground"
        >
          <Home className="h-3.5 w-3.5" />
        </Link>
        {breadcrumbs.map((crumb) => (
          <span key={crumb.href} className="flex items-center gap-1.5">
            <ChevronRight className="h-3.5 w-3.5" />
            {crumb.last ? (
              <span className="text-foreground">{crumb.label}</span>
            ) : (
              <Link href={crumb.href} className="hover:text-foreground">
                {crumb.label}
              </Link>
            )}
          </span>
        ))}
      </nav>

      {/* Header */}
      <header className="mb-8">
        <div className="flex items-center gap-3">
          <h1 className="text-3xl font-bold text-foreground">{page.title}</h1>
          <Badge variant={variantForType(page.type)}>{page.type}</Badge>
          {page!.layer ? parseInt(page!.layer) > 0 : false && (
            <span className="inline-flex items-center gap-1 text-sm text-muted-foreground">
              <Layers className="h-3.5 w-3.5" /> Layer {page!.layer}
            </span>
          )}
        </div>
      </header>

      {/* Content */}
      <div className="prose prose-neutral dark:prose-invert max-w-none">
        {page.content ? (
          <div
            dangerouslySetInnerHTML={{ __html: page.content }}
            className="whitespace-pre-wrap leading-relaxed"
          />
        ) : (
          <p className="text-muted-foreground italic">No content.</p>
        )}
      </div>

      {/* Footer nav */}
      <div className="mt-12 border-t border-border pt-6">
        <Link
          href={`/${workspaceSlug}/wiki`}
          className="text-sm text-muted-foreground hover:text-foreground"
        >
          ← Back to wiki index
        </Link>
      </div>
    </article>
  );
}
