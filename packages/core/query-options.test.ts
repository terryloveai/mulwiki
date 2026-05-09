import type { QueryClient } from "@tanstack/react-query";
import type { Agent, AgentRuntime, AgentSkill, AgentTask, Job, SchemaWithActive, Source, User, Workspace } from "./types";
import { createQueryClient } from "./query-client";
import { authKeys, meOptions } from "./auth/queries";
import {
  schemaListOptions,
  sourceListOptions,
  workspaceDetailOptions,
  workspaceKeys,
} from "./workspace/queries";
import {
  agentDetailOptions,
  agentKeys,
  agentListOptions,
  agentTasksOptions,
  runtimeListOptions,
  skillListOptions,
} from "./agents/queries";
import {
  jobKeys,
  jobListOptions,
  taskMessagesOptions,
} from "./jobs/queries";

function expectType<T>(_value: T) {}
type FunctionReturn<T> = T extends (...args: never[]) => infer R ? R : never;
type QueryFnResult<T extends { queryFn?: unknown }> = Awaited<FunctionReturn<NonNullable<T["queryFn"]>>>;
type SelectResult<T extends { select?: unknown }> = FunctionReturn<NonNullable<T["select"]>>;

const queryClient = createQueryClient();
expectType<QueryClient>(queryClient);

expectType<readonly ["auth", "me"]>(authKeys.me());
expectType<readonly ["workspaces", string]>(workspaceKeys.detail("demo"));
expectType<readonly ["workspaces", string, "agents", "list"]>(agentKeys.list("demo"));
expectType<readonly ["workspaces", string, "agents", string]>(agentKeys.detail("demo", "agent-1"));
expectType<readonly ["workspaces", string, "jobs", "list"]>(jobKeys.list("demo"));
expectType<readonly ["workspaces", string, "jobs", string, "logs"]>(jobKeys.logs("demo", "job-1"));
expectType<readonly ["workspaces", string, "schemas", string]>(workspaceKeys.schemaDetail("demo", "schema-1"));
expectType<readonly ["workspaces", string, "wiki", "list"]>(workspaceKeys.wikiList("demo"));
expectType<readonly ["workspaces", string, "wiki", string]>(workspaceKeys.wikiDetail("demo", "pages/page.md"));

const workspaceDetail = workspaceDetailOptions("demo");
expectType<readonly ["workspaces", string]>(workspaceDetail.queryKey);
expectType<Workspace>({} as QueryFnResult<typeof workspaceDetail>);

const me = meOptions();
expectType<readonly ["auth", "me"]>(me.queryKey);
expectType<User>({} as QueryFnResult<typeof me>);

const sources = sourceListOptions("demo");
expectType<readonly ["workspaces", string, "sources", "list"]>(sources.queryKey);
expectType<Source[]>({} as QueryFnResult<typeof sources>);

const schemas = schemaListOptions("demo");
expectType<readonly ["workspaces", string, "schemas", "list"]>(schemas.queryKey);
expectType<SchemaWithActive[]>({} as QueryFnResult<typeof schemas>);

const agentList = agentListOptions("demo");
expectType<readonly ["workspaces", string, "agents", "list"]>(agentList.queryKey);
expectType<Agent[]>({} as SelectResult<typeof agentList>);

const agentDetail = agentDetailOptions("demo", "agent-1");
expectType<readonly ["workspaces", string, "agents", string]>(agentDetail.queryKey);
expectType<Agent>({} as SelectResult<typeof agentDetail>);

const runtimes = runtimeListOptions("demo");
expectType<AgentRuntime[]>({} as SelectResult<typeof runtimes>);

const skills = skillListOptions("demo");
expectType<AgentSkill[]>({} as SelectResult<typeof skills>);

const tasks = agentTasksOptions("demo", "agent-1");
expectType<readonly ["workspaces", string, "agents", string, "tasks"]>(tasks.queryKey);
expectType<AgentTask[]>({} as SelectResult<typeof tasks>);

const jobs = jobListOptions("demo");
expectType<readonly ["workspaces", string, "jobs", "list"]>(jobs.queryKey);
expectType<Job[]>({} as QueryFnResult<typeof jobs>);

const messages = taskMessagesOptions("task-1");
expectType<readonly ["tasks", string, "messages"]>(messages.queryKey);
