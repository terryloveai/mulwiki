"use client";

import { useEffect, useMemo, useState } from "react";
import { useUpdateAgent } from "@mulwiki/core/hooks";
import type { Agent } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";
import { Input } from "@mulwiki/ui/components/Input";
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react";

type EnvRow = { key: string; value: string };

function rowsFromAgent(agent: Agent): EnvRow[] {
  const entries = agent.custom_env && typeof agent.custom_env === "object" ? Object.entries(agent.custom_env) : [];
  return entries.length > 0 ? entries.map(([key, value]) => ({ key, value })) : [{ key: "", value: "" }];
}

export function EnvTab({
  agent,
  updateAgent,
}: {
  agent: Agent;
  updateAgent: ReturnType<typeof useUpdateAgent>;
}) {
  const initialRows = useMemo(() => rowsFromAgent(agent), [agent]);
  const [envVars, setEnvVars] = useState<EnvRow[]>(initialRows);
  const [showValues, setShowValues] = useState<Record<number, boolean>>({});
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setEnvVars(initialRows);
    setShowValues({});
  }, [initialRows]);

  const addRow = () => setEnvVars((rows) => [...rows, { key: "", value: "" }]);
  const removeRow = (index: number) => setEnvVars((rows) => rows.filter((_, rowIndex) => rowIndex !== index));

  const handleSave = () => {
    const env: Record<string, string> = {};
    for (const row of envVars) {
      if (row.key.trim()) env[row.key.trim()] = row.value;
    }

    setSaving(true);
    updateAgent.mutate({ id: agent.id, custom_env: env }, { onSettled: () => setSaving(false) });
  };

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Environment Variables</div>
      <p className="mb-3 text-xs text-muted-foreground">Variables injected into the agent&rsquo;s execution environment.</p>
      <div className="space-y-2">
        {envVars.map((row, index) => (
          <div key={index} className="flex items-center gap-2">
            <Input
              value={row.key}
              onChange={(event) =>
                setEnvVars((rows) =>
                  rows.map((candidate, rowIndex) =>
                    rowIndex === index ? { ...candidate, key: event.target.value } : candidate,
                  ),
                )
              }
              placeholder="KEY"
              className="flex-1 font-mono text-xs"
            />
            <div className="relative flex-1">
              <Input
                type={showValues[index] ? "text" : "password"}
                value={row.value}
                onChange={(event) =>
                  setEnvVars((rows) =>
                    rows.map((candidate, rowIndex) =>
                      rowIndex === index ? { ...candidate, value: event.target.value } : candidate,
                    ),
                  )
                }
                placeholder="value"
                className="flex-1 pr-8 font-mono text-xs"
              />
              <button
                type="button"
                onClick={() => setShowValues((values) => ({ ...values, [index]: !values[index] }))}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                {showValues[index] ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </button>
            </div>
            <Button variant="ghost" size="icon" onClick={() => removeRow(index)} disabled={envVars.length <= 1}>
              <Trash2 className="h-4 w-4 text-muted-foreground" />
            </Button>
          </div>
        ))}
        <Button variant="outline" size="sm" onClick={addRow}>
          <Plus className="h-3.5 w-3.5" /> Add Variable
        </Button>
      </div>
      <div className="mt-4">
        <Button size="sm" disabled={saving} onClick={handleSave}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}
