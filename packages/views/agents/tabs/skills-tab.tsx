"use client";

import { useAssignSkill, useUnassignSkill } from "@mulwiki/core/hooks";
import type { Agent, AgentSkill } from "@mulwiki/core/types";
import { Button } from "@mulwiki/ui/components/Button";

export function SkillsTab({
  agent,
  skills,
  assignSkill,
  unassignSkill,
}: {
  agent: Agent;
  skills: AgentSkill[];
  assignSkill: ReturnType<typeof useAssignSkill>;
  unassignSkill: ReturnType<typeof useUnassignSkill>;
}) {
  const assigned = new Set(agent.skills?.map((skill) => skill.id) ?? []);

  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Skills</div>
      <p className="mb-3 text-xs text-muted-foreground">Attach skills that extend the agent&rsquo;s capabilities.</p>
      {skills.length === 0 ? (
        <p className="text-sm text-muted-foreground">No skills defined in this workspace.</p>
      ) : (
        <div className="space-y-1.5">
          {skills.map((skill) => {
            const hasSkill = assigned.has(skill.id);

            return (
              <div key={skill.id} className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                <div>
                  <div className="text-sm font-medium text-foreground">{skill.name}</div>
                  {skill.description && <div className="text-xs text-muted-foreground">{skill.description}</div>}
                </div>
                <Button
                  size="sm"
                  variant={hasSkill ? "outline" : "default"}
                  onClick={() => {
                    if (hasSkill) {
                      unassignSkill.mutate({ agentId: agent.id, skillId: skill.id });
                    } else {
                      assignSkill.mutate({ agentId: agent.id, skillId: skill.id });
                    }
                  }}
                >
                  {hasSkill ? "Remove" : "Attach"}
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
