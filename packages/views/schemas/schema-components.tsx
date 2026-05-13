"use client";

import { useState } from "react";
import type { ValidateSchemaResponse } from "@mulwiki/core/types";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { sectionSummary } from "./schema-utils";

export function SectionPreview({
  title,
  content,
}: {
  title: string;
  content: string;
}) {
  const [open, setOpen] = useState(false);
  const summary = sectionSummary({ title, content });

  return (
    <div className="border-b border-border last:border-0">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 px-4 py-2.5 text-left hover:bg-accent/30 transition-colors"
      >
        {open ? (
          <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
        <span className="text-sm font-medium text-foreground truncate flex-1">
          {title}
        </span>
        {!open && summary && (
          <span className="text-xs text-muted-foreground truncate max-w-[200px] hidden sm:inline">
            {summary}
          </span>
        )}
      </button>
      {open && (
        <div className="px-4 pb-3 pl-9">
          <pre className="text-xs font-mono text-muted-foreground whitespace-pre-wrap leading-relaxed max-h-80 overflow-y-auto">
            {content}
          </pre>
        </div>
      )}
    </div>
  );
}

export function ValidationPanel({ result }: { result: ValidateSchemaResponse }) {
  if (result.valid && (!result.warnings || result.warnings.length === 0)) return null;
  return (
    <div className="mt-3 space-y-1.5">
      {result.errors?.map((error, index) => (
        <div key={`err-${index}`} className="flex items-start gap-1.5 text-xs text-destructive">
          <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          {error}
        </div>
      ))}
      {result.warnings?.map((warning, index) => (
        <div key={`warn-${index}`} className="flex items-start gap-1.5 text-xs text-warning">
          <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          {warning}
        </div>
      ))}
      {result.valid && !result.errors?.length && (
        <div className="flex items-center gap-1.5 text-xs text-success">
          <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
          Valid
        </div>
      )}
    </div>
  );
}
