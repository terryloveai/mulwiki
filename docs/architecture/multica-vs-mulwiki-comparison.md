# Multica vs Mulwiki — 架构对比与借鉴分析

> 2026-04-29 | 基于 Multica main 分支完整源码与 Mulwiki 当前实现
> 更新：P0 借鉴项实施后（EventBus、WebSocket Hub、Daemon 注册心跳、Atomic Claim、中间件链）

---

## 1. 整体架构对比

| 维度 | Multica | Mulwiki | 差距 |
|------|---------|------------|------|
| 前端 | Next.js 16 App Router, pnpm monorepo | Next.js 15, pnpm monorepo | 平齐 |
| 后端 | Go (Chi router, sqlc, gorilla/websocket) | Go (Chi router, gorilla/websocket) | 缺 sqlc 类型安全 |
| 数据库 | PostgreSQL 17 + pgvector | SQLite | 定位不同（单机 vs 云） |
| Agent 运行时 | 本地 daemon, 支持 10+ CLI | Daemon + agent CLI fork | 接近（缺多 CLI Backend 抽象） |
| 实时通信 | WebSocket + Event Bus | WebSocket Hub + EventBus | 平齐 |
| 部署 | Docker + Homebrew + 云 | 单进程 Go binary | 可用 |

**结论：P0 借鉴项实施后，Mulwiki 与 Multica 在核心架构维度已基本对齐。剩余差距集中在 sqlc 类型安全、多 CLI Backend 抽象、PostgreSQL 特性等 P1/P2 项。**

---

## 2. 数据模型对比

### 2.1 Agent 表

**Multica (`agent`)：**
```sql
agent (
    id, workspace_id, name, avatar_url,
    runtime_mode (local/cloud), runtime_config (JSONB),
    visibility (workspace/private), status (idle/working/blocked/error/offline),
    max_concurrent_tasks, owner_id,
    created_at, updated_at
)
-- 注意：Multica 早期版本也没有 instructions/custom_env/custom_args，
-- 这些字段在后续 migration 中添加。当前 Mulwiki 反而已经补全了。
```

**Mulwiki (`agents`)：**
```sql
agents (
    id, workspace_id, runtime_id, name, description,
    instructions, runtime_mode, runtime_config (JSON string),
    custom_env, custom_args, mcp_config,
    visibility, status, model, max_concurrent_tasks,
    owner_id, created_at, updated_at, archived_at, archived_by
)
```

**关键差异：**
- Multica 的 `runtime_config` 是 JSONB（PostgreSQL），Mulwiki 是 TEXT（SQLite 无 JSONB）
- Multica 的 agent status 枚举更丰富（idle/working/blocked/error/offline），比 mulwiki 的 online/offline 更有用
- Mulwiki 额外有 `archived_at/archived_by` 和 `instructions`，但这些 Multica 在后续 migration 中也加了

### 2.2 Runtime 表

**Multica (`agent_runtime`)：**
```sql
agent_runtime (
    id, workspace_id, daemon_id, name, runtime_mode, provider,
    launch_header, status, device_info, metadata (JSONB),
    owner_id, last_seen_at, created_at, updated_at
)
```

**Mulwiki（P0 后）：**
```sql
agent_runtimes (
    id, workspace_id, name, backend,
    path, hostname, os, version, status,
    daemon_id, last_heartbeat, created_at
)
```

**关键差异（已显著缩小）：**
- ✅ `backend` 对应 Multica 的 provider 概念（claude-code / codex / kimi / custom）
- ✅ `hostname` / `os` / `version` 设备信息（P0 项 4 更新）
- ✅ Model 从 Runtime 移到 Agent 层（与 Multica 对齐：agent 指定 model，runtime 只管执行环境）
- ⚠️ Multica 有 `device_info` JSONB 和 `launch_header`，Mulwiki 用扁平字段（P2）

### 2.3 Task 表

**Multica (`agent_task_queue`)：**
```sql
agent_task_queue (
    id, agent_id, runtime_id, issue_id, workspace_id,  -- issue_id 必填
    status, priority, dispatched_at, started_at, completed_at,
    result (JSONB), error, attempt, max_attempts,
    failure_reason, session_id, work_dir,
    parent_task_id, trigger_comment_id,
    chat_session_id, autopilot_run_id,
    created_at
)
```

**Mulwiki (`agent_tasks`)：**
```sql
agent_tasks (
    id, agent_id, runtime_id, workspace_id, source_id, schema_id,
    status, priority,
    parent_task_id, session_id, work_dir,
    failure_reason, daemon_id,
    dispatched_at, started_at, completed_at,
    result, error, attempt, max_attempts, created_at
)
```

**关键差异（已显著缩小）：**
- ✅ parent_task_id / session_id / work_dir / failure_reason / daemon_id 均已在 P0 项 1 中实施
- ✅ status CHECK 约束（queued/dispatched/running/completed/failed/cancelled）已对齐
- ✅ attempt 默认值改为 1
- Mulwiki 有 `source_id`/`schema_id`（对应其核心业务），Multica 有 `issue_id`/`chat_session_id`/`autopilot_run_id`（对应其核心业务）

---

## 3. Agent 执行模型对比

### 3.1 Multica 的 Daemon 架构

Multica 的 daemon 是一个独立的本地进程，负责：

```
daemon.Run()
  ├── resolveAuth()               # 从 CLI 配置加载 token
  ├── syncWorkspacesFromAPI()     # 获取用户所有 workspace
  │   └── registerRuntimesForWorkspace()  # 为每个 workspace 注册 runtime
  ├── workspaceSyncLoop()         # 30s 轮询发现新 workspace
  ├── heartbeatLoop()             # 定期发送心跳，接收服务端指令
  ├── taskWakeupLoop()            # 接收任务通知
  ├── gcLoop()                    # 清理过期工作目录
  └── pollLoop()                  # 主轮询循环
      └── handleTask()
          ├── runTask()
          │   ├── execenv.Prepare()      # 创建隔离工作目录
          │   ├── execenv.InjectRuntimeConfig()  # 注入 AGENTS.md
          │   ├── agent.New(provider)    # 创建 Backend 实例
          │   └── executeAndDrain()      # 执行并流式输出
          └── client.CompleteTask() / FailTask()
```

**Mulwiki 当前状态（P0 后 + Runtime 升级）：**
- ✅ 独立的 daemon 进程，注册 + 心跳循环
- ✅ `backend` 对应 Multica 的 provider 概念（claude-code / codex / kimi / custom）
- ⚠️ 无 `agent.Backend` 抽象（直接 fork CLI，P2）
- ✅ `execenv` 工作目录隔离
- ✅ 心跳循环（30s）
- ✅ 任务轮询 + Atomic Claim
- ✅ 设备信息（hostname / os / version）
- ⚠️ 无 GC 循环

### 3.2 Agent Backend 抽象

**Multica 的 `agent.Backend` 接口：**
```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

type ExecOptions struct {
    Cwd, Model, SystemPrompt string
    MaxTurns int
    Timeout  time.Duration
    ResumeSessionID string
    CustomArgs []string
    McpConfig json.RawMessage
}

type Session struct {
    Messages <-chan Message  // 流式输出
    Result   <-chan Result   // 最终结果
}
```

支持 10 种 CLI：claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor, kimi, kiro

**Mulwiki：无此抽象。** 这是 Mulwiki 最需要借鉴的部分。

### 3.3 执行环境隔离

**Multica `execenv.Prepare()`：**
- 为每个 task 创建独立的 `work_dir`（`{workspacesRoot}/{workspaceID}/{agentName}/{taskID}`）
- 写入 `AGENTS.md`（任务上下文 + agent 指令）
- 注入技能文件
- 设置 `CODEX_HOME`（Codex 专用）
- 写入 GC 元数据（用于后续清理）
- 支持工作目录复用（PriorWorkDir）

**Mulwiki：无工作目录管理。** 当前设计假设 daemon 自己管理工作目录。

### 3.4 任务生命周期

```
Multica 状态机：
queued → dispatched → running → completed
                              → failed → (auto-retry → queued)
                              → cancelled

Mulwiki 状态机：
queued → dispatched → running → completed
                              → failed
                              → cancelled
```

Multica 多的关键特性：
- **ClaimTask**：原子认领，检查 `max_concurrent_tasks` 容量
- **Auto-retry**：infra 级失败自动重试（runtime_offline, timeout），最多 `max_attempts` 次
- **Session resume**：通过 `PriorSessionID` / `PriorWorkDir` 跨任务恢复会话
- **Cancellation polling**：运行中每 5s 检查服务端是否取消了任务

---

## 4. API 设计模式对比

### 4.1 Handler 结构

**Multica：**
```go
type Handler struct {
    DB          *pgxpool.Pool
    Queries     *db.Queries      // sqlc 生成的类型安全查询
    TxStarter   TxStarter        // 事务抽象
    Hub         *realtime.Hub    // WebSocket hub
    Bus         *events.Bus      // 事件总线
    TaskService *service.TaskService  // 业务逻辑层
    Analytics   *analytics.Service
}
```

**Mulwiki（P0 后）：**
```go
type Handler struct {
    DB  *sql.DB
    Bus *events.Bus
    Hub *realtime.Hub
}
```

**差距缩小中：** Mulwiki 已加入 EventBus 和 WebSocket Hub，但尚未引入 sqlc/sqlx 和服务层。

### 4.2 权限模式

**Multica 的权限检查链：**
```go
// 1. 解析 workspace
workspaceID := h.resolveWorkspaceID(r)

// 2. 验证用户是该 workspace 的成员
member, ok := h.workspaceMember(w, r, workspaceID)

// 3. 检查角色权限
if !roleAllowed(member.Role, "owner", "admin") { ... }

// 4. Agent 操作额外检查
func canViewAgentEnv(agent, userID, memberRole) bool  // 敏感字段 redact
func (h *Handler) canManageAgent(...) bool              // agent owner 或 admin
```

**Mulwiki（P0 后）：**
- ✅ Auth 中间件（session cookie/token → X-User-ID）
- ✅ Workspace 中间件（URL slug → workspace context）
- ✅ Role 中间件（owner/admin/member 权限检查）
- ✅ Agent owner 检查 + custom_env redaction

### 4.3 输入验证

**Multica 的验证模式：**
```go
if req.Name == "" {
    writeError(w, http.StatusBadRequest, "name is required")
    return
}
if req.RuntimeID == "" {
    writeError(w, http.StatusBadRequest, "runtime_id is required")
    return
}
// 验证 runtime 存在且属于该 workspace
runtime, err := h.Queries.GetAgentRuntimeForWorkspace(ctx, params)
if err != nil {
    writeError(w, http.StatusBadRequest, "invalid runtime_id")
    return
}
// 默认值
if req.Visibility == "" { req.Visibility = "private" }
if req.MaxConcurrentTasks == 0 { req.MaxConcurrentTasks = 6 }
```

**Mulwiki：** 验证较轻，部分 handler 缺少对关联实体存在性的检查。

### 4.4 事件系统

**Multica 的 Event Bus：**
```go
h.publish(protocol.EventAgentCreated, workspaceID, actorType, actorID, map[string]any{"agent": resp})
```
支持的事件类型：`agent:created`, `agent:status`, `agent:archived`, `task:dispatch`, `task:started`, `task:completed`, `task:failed`, `task:cancelled`, `issue:created`, `issue:updated`, `comment:created` 等

**Mulwiki（P0 后）：** ✅ EventBus + WebSocket Hub 已实施。支持 task.dispatched/started/completed/failed 和 daemon.online/offline 事件。URL：`GET /ws?workspace=slug`

### 4.5 批量操作

Multica 支持：
- 批量 issue 更新（`BatchUpdateIssues`）
- 批量 issue 删除（`BatchDeleteIssues`）
- 批量 agent task 取消（`CancelAgentTasks`）

Mulwiki：无限批量操作。

### 4.6 搜索

Multica 有完整的全文搜索实现（`SearchIssues`），包括：
- 多级排名（精确匹配 > 前缀匹配 > 包含匹配）
- 结果片段提取（`extractSnippet`）
- 分页支持
- pg_bigm 索引

Mulwiki：Wiki 页面有 `SearchWikiPages` 端点，但实现较简单。

---

## 5. 前端架构对比

| 维度 | Multica | Mulwiki |
|------|---------|------------|
| 框架 | Next.js 16 App Router | Next.js (apps/web) |
| 状态管理 | React Query + WebSocket 推送 | React hooks |
| 实时更新 | WebSocket 事件订阅 | 无（需轮询） |
| Agent UI | 完整的 Agent 创建/编辑页，六区配置 | 进展中 |
| Board 视图 | Kanban 看板 | 无 |
| Chat | Agent 对话界面 | 无 |
| Usage 面板 | Token 用量图表 | 无 |

**关键借鉴：**
- Multica 的 Agent 创建页有模板选择器（coding/planning/writing/assistant）
- Multica 前端使用 WebSocket 事件驱动 UI 更新（任务状态变化即时反映）
- Multica 的 Agent 列表有 Activity 火花图（30 天任务分布）

---

## 6. 值得完全借鉴的设计模式

### 6.1 sqlc 类型安全查询（P0）

Multica 使用 [sqlc](https://sqlc.dev/) 从 SQL 文件生成类型安全的 Go 代码。Mulwiki 全用手写 SQL 字符串拼接。

**借鉴方案：**
```
server/pkg/db/
  queries/         # SQL 查询定义
    agents.sql
    runtimes.sql
    tasks.sql
  generated/       # sqlc 生成的代码
    agents.sql.go
    models.go
```

Mulwiki 使用 SQLite，sqlc 支持 SQLite。

### 6.2 Service 层分离（P0）

**借鉴方案：** 创建 `server/internal/service/` 包：
- `task_service.go` — 任务生命周期管理（enqueue, claim, start, complete, fail, retry）
- `agent_service.go` — agent 状态协调（reconcile status from tasks）
- `job_service.go` — 摄入任务调度

### 6.3 Agent Backend 抽象（P0）

**借鉴方案：** 创建 `server/pkg/agent/` 包：
```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}
```
至少支持 Claude Code 和 Codex CLI，后续扩展 Kimi CLI。

### 6.4 执行环境隔离（P0）

**借鉴方案：** 创建 `server/internal/execenv/` 包：
- `Prepare()` — 为每个 job 创建隔离工作目录
- `InjectSchemaConfig()` — 将 Schema .md 写入工作目录的 AGENTS.md
- `InjectSources()` — 将待处理文档复制/link 到工作目录
- `WriteGCMeta()` — 写入 GC 元数据

### 6.5 Task 生命周期增强（部分实施）

借鉴 Multica 的成熟模式：
- ✅ `ClaimTask` — 原子认领 + `max_concurrent_tasks` 容量检查
- ⬜ `MaybeRetryFailedTask` — infra 级失败自动重试
- ⬜ `CancelAgentTasks` — 批量取消（归档 agent 时、删除 workspace 时）
- ⬜ `ReconcileAgentStatus` — 从 active task 集合推导 agent 状态

### 6.6 WebSocket 实时推送（✅ 已实施）

已实施 Event Bus + WebSocket Hub 双层架构：
- 前端通过 `GET /ws?workspace=slug` 连接
- 服务端发布 task.dispatched/started/completed/failed 事件
- Hub 按 workspace 和 agent 房间路由消息

### 6.7 心跳 + 离线检测（✅ 已实施）

已实施 daemon 注册 + heartbeat + stale detection：
- Daemon 启动时 POST `/api/daemon/register`
- 每 30s POST `/api/daemon/heartbeat`
- 服务端后台 sweeper 检测超时（5min），标记 runtime 离线

### 6.8 CustomEnv Redaction（✅ 已实施）

已实施 `canViewAgentEnv()` + `redactEnv()` 模式：
- Agent owner 和 workspace admin 可见完整 environment
- 其他用户看到 `{"ANTHROPIC_API_KEY": "***"}`

### 6.9 Runtime Usage Tracking（P2）

借鉴 Multica 的 usage 追踪：
- `agent_task_usage` 表记录每次执行的 token 消耗
- 前端展示按天/按 agent/按 model 的用量图表

### 6.10 Autopilot/Cron 触发（P2）

Multica 的 Autopilot 系统（定时 + webhook + API 触发）可直接借鉴用于 Mulwiki 的定时摄入场景。

---

## 7. 不应盲目借鉴的差异

| Multica 特性 | 不适合 Mulwiki 的原因 |
|-------------|------------------------|
| PostgreSQL + pgvector | Mulwiki 是单机部署，SQLite 更适合。向量搜索不是核心需求 |
| Issue/Comment 系统 | Mulwiki 不是项目管理工具，核心是知识编译 |
| Multi-member workspace | MVP 阶段单用户即可 |
| Kanban Board UI | 不是知识管理工具需要的 UI |
| Cloud deployment | Mulwiki 定位本地/单机 |

---

## 8. 具体实施路线图

### Phase 1 — 基础架构对齐（✅ 已完成）

1. ~~引入 sqlc~~ → 暂缓（SQLite + 手写 SQL 够用，P2）
2. ~~创建 agent.Backend 抽象~~ → 简化版：直接 fork CLI（P2 完整化）
3. ✅ **创建 execenv 包**：工作目录隔离、Schema 注入
4. ✅ **Agent Task 单表多态**：parent_task_id / failure_reason / session_id / work_dir
5. ✅ **Atomic Claim**：SQLite 事务级 UPDATE WHERE status='queued'
6. ✅ **Daemon 注册 + 心跳**：register / heartbeat / stale detection
7. ✅ **EventBus + WebSocket Hub**：双层事件架构
8. ✅ **中间件链**：Auth → Workspace → Role 三层

### Phase 2 — 质量加固（进行中）

6. **完善 Task 生命周期**：auto-retry（failure_reason 驱动）、session resume
7. ~~实时推送~~ ✅ 已完成
8. ~~心跳 + 离线检测~~ ✅ 已完成
9. ~~CustomEnv redaction~~ ✅ 已完成

### Phase 3 — 增强功能

10. **Usage tracking**：Token 用量记录和展示
11. **Scheduled ingest**：借鉴 Autopilot 的 cron 模式
12. **Multi-backend**：添加 Codex、Kimi CLI 支持

---

## 9. 源码文件映射表

| Multica 源文件 | Mulwiki 对应 | 状态 |
|---------------|----------------|------|
| `server/pkg/agent/agent.go` | 待创建 | P2（Multi-backend） |
| `server/internal/daemon/daemon.go` | `internal/daemon/daemon.go` | ✅ 已实现 |
| `server/internal/daemon/execenv/` | `internal/daemon/` (内联) | ✅ 已实现 |
| `server/internal/service/task.go` | 待创建 | P1（service 层分离） |
| `server/internal/handler/agent.go` | `handler/agent.go` | ✅ 已增强（ClaimTask、CustomEnv redaction） |
| `server/internal/handler/agent_task.go` | `handler/agent.go` (CreateAgentTask/UpdateAgentTask) | ✅ 已增强（lifecycle） |
| `server/internal/handler/runtime.go` | `handler/runtime.go` | ✅ 已增强（daemon_id、heartbeat） |
| `server/internal/events/bus.go` | `internal/events/bus.go` | ✅ 已创建 |
| `server/internal/realtime/hub.go` | `internal/realtime/hub.go` | ✅ 已创建 |
| `server/internal/middleware/auth.go` | `internal/middleware/auth.go` | ✅ 已创建 |
| `server/pkg/db/generated/` | 待创建 (sqlc) | P2 |
| `server/migrations/` | `pkg/db/schema.sql` | 当前方案可接受 |

---

*本文档基于 2026-04-28 对话中对 Multica 的深入研究和 2026-04-29 的补充分析。Multica 源码来自 https://github.com/multica-ai/multica。*
