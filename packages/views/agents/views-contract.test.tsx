import type { ComponentType } from "react";
import { AgentsPage } from "./agents-page";
import { AgentList } from "./agent-list";
import { AgentDetailPanel } from "./agent-detail-panel";
import { AgentCreatePanel } from "./agent-create-panel";
import { InstructionsTab } from "./tabs/instructions-tab";
import { SkillsTab } from "./tabs/skills-tab";
import { TasksTab } from "./tabs/tasks-tab";
import { EnvTab } from "./tabs/env-tab";
import { SettingsTab } from "./tabs/settings-tab";

function expectComponent(_component: ComponentType<any>) {}

expectComponent(AgentsPage);
expectComponent(AgentList);
expectComponent(AgentDetailPanel);
expectComponent(AgentCreatePanel);
expectComponent(InstructionsTab);
expectComponent(SkillsTab);
expectComponent(TasksTab);
expectComponent(EnvTab);
expectComponent(SettingsTab);
