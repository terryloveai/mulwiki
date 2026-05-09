"use client";

import { useState } from "react";
import { useCreateAgent } from "@mulwiki/core/hooks";
import type { AgentRuntime } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Bot, Cpu } from "lucide-react";

export function AgentCreatePanel({
  workspaceSlug,
  runtimes,
  onDone,
  onCancel,
}: {
  workspaceSlug: string;
  runtimes: AgentRuntime[];
  onDone: (agentId: string) => void;
  onCancel: () => void;
}) {
  const createAgent = useCreateAgent(workspaceSlug);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [runtimeId, setRuntimeId] = useState("");
  const [instructions, setInstructions] = useState("");
  const [model, setModel] = useState("");
  const valid = name.trim() && runtimeId;

  const handleSubmit = () => {
    if (!valid) return;

    createAgent.mutate(
      {
        name: name.trim(),
        description: description.trim(),
        instructions: instructions.trim(),
        runtime_id: runtimeId,
        runtime_config: "{}",
        mcp_config: "{}",
        visibility: "private",
        max_concurrent_tasks: 6,
        model: model.trim() || undefined,
      },
      {
        onSuccess: (data) => {
          onDone(data.agent.id);
        },
      },
    );
  };

  return (
    <div className="mx-auto max-w-2xl px-6 py-6">
      <div className="mb-6 flex items-center gap-3">
        <Bot className="h-5 w-5 text-muted-foreground" />
        <h1 className="text-lg font-semibold text-foreground">New Agent</h1>
      </div>

      <div className="space-y-5">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">
              Name <span className="text-destructive">*</span>
            </label>
            <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Agent name" autoFocus />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-foreground">Description</label>
            <Input
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="What this agent does"
            />
          </div>
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">
            Runtime <span className="text-destructive">*</span>
          </label>
          {runtimes.length === 0 ? (
            <p className="text-xs text-muted-foreground">No runtimes available. Start a daemon first.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {runtimes.map((runtime) => (
                <button
                  key={runtime.id}
                  type="button"
                  onClick={() => setRuntimeId(runtime.id)}
                  className={`flex items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors ${
                    runtimeId === runtime.id
                      ? "border-foreground bg-foreground/5"
                      : "border-border hover:border-muted-foreground/30"
                  }`}
                >
                  <Cpu className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-foreground">{runtime.name}</span>
                  <span className="text-xs text-muted-foreground">{runtime.backend}</span>
                </button>
              ))}
            </div>
          )}
          {!runtimeId && runtimes.length > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">Select a runtime for this agent.</p>
          )}
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">Instructions (optional)</label>
          <textarea
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            rows={4}
            placeholder="Define this agent's behavior and role..."
            className="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-foreground">Model (optional)</label>
          <Input value={model} onChange={(event) => setModel(event.target.value)} placeholder="Leave blank for runtime default" />
        </div>
      </div>

      <div className="mt-6 flex items-center justify-between border-t border-border pt-4">
        <span className="text-xs text-muted-foreground">Skills & settings can be configured after creation.</span>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button disabled={!valid || createAgent.isPending} onClick={handleSubmit}>
            {createAgent.isPending ? "Creating..." : "Create Agent"}
          </Button>
        </div>
      </div>
    </div>
  );
}
