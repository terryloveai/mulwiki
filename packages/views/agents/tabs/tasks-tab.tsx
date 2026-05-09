import type { AgentTask } from "@mulwiki/core/types";
import { Badge } from "@mulwiki/ui/components/Badge";
import { ListChecks } from "lucide-react";
import { fmtDateTime, taskStatusIcon } from "../agent-utils";

export function TasksTab({ tasks }: { tasks: AgentTask[] }) {
  return (
    <div>
      <div className="mb-1 text-sm font-medium text-foreground">Task History</div>
      <p className="mb-3 text-xs text-muted-foreground">Recent tasks executed by this agent.</p>
      {tasks.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          <ListChecks className="mx-auto mb-2 h-8 w-8 opacity-20" />
          No tasks yet.
        </div>
      ) : (
        <div className="space-y-1.5">
          {tasks.map((task) => (
            <div key={task.id} className="flex items-center gap-3 rounded-md border border-border px-3 py-2">
              {taskStatusIcon(task.status)}
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-foreground">{task.schema_id || task.source_path || task.id}</div>
                <div className="text-xs text-muted-foreground">{fmtDateTime(task.created_at)}</div>
              </div>
              <Badge
                variant={
                  task.status === "completed" ? "success" : task.status === "failed" ? "destructive" : "secondary"
                }
              >
                {task.status}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
