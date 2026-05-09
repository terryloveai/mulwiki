"use client";

import { useEffect, useState } from "react";
import { useUpdateAgent } from "@mulwiki/core/hooks";
import type { Agent } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";

function argsFromAgent(agent: Agent) {
  return Array.isArray(agent.custom_args) ? agent.custom_args.join(", ") : "";
}

export function CustomArgsTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const [args, setArgs] = useState(argsFromAgent(agent));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setArgs(argsFromAgent(agent));
  }, [agent]);

  const handleSave = () => {
    const parsed = args
      .split(/[,;\n]/)
      .map((value) => value.trim())
      .filter(Boolean);

    setSaving(true);
    updateAgent.mutate({ id: agent.id, custom_args: parsed }, { onSettled: () => setSaving(false) });
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">CLI Arguments</div>
      <p className="mb-3 text-xs text-muted-foreground">Extra arguments passed to the runtime CLI when executing tasks.</p>
      <Input value={args} onChange={(event) => setArgs(event.target.value)} placeholder="--verbose, --timeout=300" />
      <div className="mt-4">
        <Button size="sm" disabled={saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}
