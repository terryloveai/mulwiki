"use client";

import { useEffect, useState } from "react";
import { useUpdateAgent } from "@mulwiki/core/hooks";
import type { Agent } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";

export function InstructionsTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const [text, setText] = useState(agent.instructions ?? "");
  const [saving, setSaving] = useState(false);
  const dirty = text !== (agent.instructions ?? "");

  useEffect(() => {
    setText(agent.instructions ?? "");
  }, [agent.instructions]);

  const handleSave = () => {
    setSaving(true);
    updateAgent.mutate({ id: agent.id, instructions: text }, { onSettled: () => setSaving(false) });
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Agent Instructions</div>
      <p className="mb-3 text-xs text-muted-foreground">
        Define this agent&rsquo;s identity and working style. These instructions are injected into
        the agent&rsquo;s context for every task.
      </p>
      <textarea
        value={text}
        onChange={(event) => setText(event.target.value)}
        rows={12}
        className="w-full resize-y rounded-lg border border-border bg-transparent px-3 py-2.5 font-mono text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        placeholder="You are a planning agent..."
      />
      <div className="mt-2 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{text.length} characters</span>
        <Button size="sm" disabled={!dirty || saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}
