"use client";

import { useEffect, useState } from "react";
import { useUpdateAgent } from "@mulwiki/core/hooks";
import type { Agent } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";

export function SettingsTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const [model, setModel] = useState(agent.model ?? "");
  const [maxConcurrent, setMaxConcurrent] = useState(agent.max_concurrent_tasks ?? 6);
  const [visibility, setVisibility] = useState<"private" | "public">(agent.visibility ?? "private");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setModel(agent.model ?? "");
    setMaxConcurrent(agent.max_concurrent_tasks ?? 6);
    setVisibility(agent.visibility ?? "private");
  }, [agent]);

  const handleSave = () => {
    setSaving(true);
    updateAgent.mutate(
      {
        id: agent.id,
        model: model.trim() || undefined,
        max_concurrent_tasks: maxConcurrent,
        visibility,
      },
      { onSettled: () => setSaving(false) },
    );
  };

  return (
    <div className="space-y-6">
      <div>
        <div className="mb-1 text-sm font-medium text-foreground">Model & Concurrency</div>
        <p className="mb-3 text-xs text-muted-foreground">
          Override the default model and task concurrency for this agent.
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Model</label>
            <Input value={model} onChange={(event) => setModel(event.target.value)} placeholder="e.g. gpt-5.2" />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Max Concurrent Tasks</label>
            <Input
              type="number"
              min={1}
              max={50}
              value={maxConcurrent}
              onChange={(event) => setMaxConcurrent(Number(event.target.value))}
            />
          </div>
        </div>
      </div>

      <div>
        <div className="mb-1 text-sm font-medium text-foreground">Visibility</div>
        <p className="mb-3 text-xs text-muted-foreground">
          Public agents can be discovered by other workspace members.
        </p>
        <div className="flex gap-3">
          {(["private", "public"] as const).map((value) => (
            <button
              key={value}
              type="button"
              onClick={() => setVisibility(value)}
              className={`rounded-md border px-3 py-1.5 text-sm font-medium capitalize transition-colors ${
                visibility === value
                  ? "border-foreground bg-foreground/5 text-foreground"
                  : "border-border text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground"
              }`}
            >
              {value}
            </button>
          ))}
        </div>
      </div>

      <Button size="sm" disabled={saving} onClick={handleSave}>
        {saving ? "Saving..." : "Save"}
      </Button>
    </div>
  );
}
