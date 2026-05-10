# Multica vs Mulwiki — 源码级深度对比 (v3)

> 梦境系统 · 2026-04-30
> 范围：逐文件阅读 Multica main 分支完整源码，与 Mulwiki 当前实现行级对比
> 前序文档：`multica-deep-dive.md` (v1), `multica-vs-mulwiki-comparison.md` (v2)

---

## 概述

经过三次迭代的深度研究（v1 架构概览 → v2 对比改进追踪 → v3 源码行级分析），Mulwiki 已经大量借鉴了 Multica 的架构设计。本次 v3 分析的焦点是：**之前两轮研究中被忽略的代码级细节、以及 P1/P2 优先级的具体落地路径**。

核心发现：Mulwiki 在前两轮中正确借鉴了最关键的架构模式（EventBus + Hub, Daemon 注册 + 心跳, Atomic Claim, Agent Task 状态机），但存在 **11 个值得立刻修复的代码级差距** 和 **3 个中期应该采纳的架构级模式**。

---

## 第一部分：代码级差距（已发现但未修复）

### 1. Agent Backend 接口抽象（最关键差距）

**Multica 实现** (`pkg/agent/agent.go`):

```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

type Session struct {
    Messages <-chan Message  // 流式输出（thinking/text/tool_use/tool_result/status/error）
    Result   <-chan Result   // 最终结果（status: completed/blocked/timeout/cancelled/failed）
}

type Message struct {
    Type      MessageType
    Content   string
    Tool      string
    CallID    string
    Input     map[string]any
    Output    string
    SessionID string           // 用于早期 session 持久化
}
```

支持 10+ CLI: claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor, kimi, kiro。

**Mulwiki 当前** (`daemon/daemon.go`):

```go
cmd := exec.CommandContext(ctx, runtime.Path, args...)
stdout, _ := cmd.StdoutPipe()
stderr, _ := cmd.StderrPipe()
go d.streamLogs(stdout, job.ID, "stdout")
go d.streamLogs(stderr, job.ID, "stderr")
procErr := cmd.Wait()
```

无 Backend 抽象，仅直接 fork 子进程并收集 stdout/stderr 原始文本。

**差距影响**:
- 无法统一处理 agent 消息流（thinking, tool_use, tool_result 等结构化消息丢失）
- 无法实现 session resume（子进程输出不包含结构化 session_id）
- 无法支持多种 CLI（每增加一种 CLI 需要在 daemon 中添加特殊处理）
- 无法实现消息批量上报（结构化消息可以批量推送到 server）

**建议方案**: 创建 `pkg/agent/` 包，至少实现 `claude` 和 `codex` 两个 backend，统一消息格式。

---

### 2. Service 层缺失

**Multica 实现** (`internal/service/task.go`):

`TaskService` 封装了完整的任务生命周期：

```go
type TaskService struct {
    Queries   *db.Queries   // sqlc 类型安全查询
    TxStarter TxStarter     // 事务抽象
    Hub       *realtime.Hub
    Bus       *events.Bus
    Wakeup    TaskWakeupNotifier
}

// 方法：
func (s *TaskService) EnqueueTaskForIssue(...)
func (s *TaskService) EnqueueChatTask(...)
func (s *TaskService) EnqueueQuickCreateTask(...)
func (s *TaskService) ClaimTask(...)              // 原子认领 + max_concurrent 检查
func (s *TaskService) StartTask(...)
func (s *TaskService) CompleteTask(...)           // 事务内同时更新 chat_session session_id
func (s *TaskService) FailTask(...)               // 失败处理 + auto-retry + agent 评论
func (s *TaskService) MaybeRetryFailedTask(...)   // 仅 infra 原因可重试
func (s *TaskService) HandleFailedTasks(...)      // 批量失败后处理
func (s *TaskService) CancelTask(...)
func (s *TaskService) CancelTasksForAgent(...)
func (s *TaskService) ReconcileAgentStatus(...)
```

**Mulwiki 当前**: 所有逻辑分散在 handler 方法中（`handler/agent.go`, `handler/job.go`），无 service 层。

**差距影响**:
- 无事务保证（CompleteTask 和 session_id 更新不是原子的）
- 无 auto-retry（failure_reason 字段存在但未被系统使用）
- 无 agent 状态协调（ReconcileAgentStatus）
- handler 直接操作 DB，测试困难

**建议方案**: 创建 `internal/service/task.go` 和 `internal/service/agent.go`，从 handler 中抽离业务逻辑。

---

### 3. CompleteTask 的事务安全性

**Multica `CompleteTask`** 在单事务中完成：
1. 任务状态更新 (completed)
2. session_id / work_dir 持久化
3. chat_session.session_id 更新（让下一个 chat turn 能 resume）

```go
func (s *TaskService) CompleteTask(...) {
    if err := s.runInTx(ctx, func(qtx *db.Queries) error {
        t, err := qtx.CompleteAgentTask(ctx, ...)
        if t.ChatSessionID.Valid {
            qtx.UpdateChatSessionSession(ctx, ...)  // 同事务内
        }
        return nil
    }); err != nil { ... }
}
```

**Mulwiki 当前**: `daemon.markTaskCompleted()` 仅 PATCH 任务状态，无事务，无 session_id 持久化，无 chat session 关联。

**差距影响**: 如果聊天场景需要跨任务 resume agent session，Mulwiki 会在 PATCH 任务和 PATCH session 之间存在竞态窗口。

---

### 4. Session Pinning（早期持久化防止崩溃丢失）

**Multica** 在 agent 首次报告 `session_id` 时立即持久化：

```go
// executeAndDrain 中的消息循环
if msg.SessionID != "" && !sessionPinned.Swap(true) {
    go func() {
        pinCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        d.client.PinTaskSession(pinCtx, taskID, msg.SessionID, opts.Cwd)
    }()
}
```

**Mulwiki**: 无此机制。session_id 和 work_dir 仅在实际任务完成/失败时更新，如果 daemon 在中间崩溃，这些信息就丢失了。

**差距影响**: Daemon 崩溃后，自动重试的任务无法 resume 之前的 session，必须从头开始。

---

### 5. 500ms 批量消息上报

**Multica** 使用 500ms ticker 批量上报 agent 消息：

```go
ticker := time.NewTicker(500 * time.Millisecond)
var batch []TaskMessageData
for msg := range session.Messages {
    batch = append(batch, convert(msg))
}
// flush() 被 ticker 定时触发
```

消息结构包含 seq（单调递增序号）、type（thinking/text/tool_use/tool_result/error）、tool、input、output。

前端可通过 `GET /api/daemon/tasks/{id}/messages?since=42` 增量同步。

**Mulwiki**: 仅通过 `slog` 逐行记录 stdout/stderr，不存储到数据库，不可查询。

---

### 6. 孤儿任务恢复

**Multica** 在 daemon 启动时调用 `RecoverOrphans`:

```go
// syncWorkspacesFromAPI 中
for _, rid := range runtimeIDs {
    d.client.RecoverOrphans(ctx, rid)  // POST /api/daemon/runtimes/{id}/recover-orphans
}
```

Server 端将所有 dispatched/running 的任务标记为 failed（failure_reason = "runtime_recovery"），然后触发 `HandleFailedTasks` → `MaybeRetryFailedTask` → 自动重试。

**Mulwiki**: daemon 启动后仅进入主轮询循环，不处理之前崩溃遗留的 dispatched/running 任务。这些任务会一直卡住，直到人工介入。

---

### 7. 运行中取消检测

**Multica** 在子进程运行期间每 5s 检查服务端是否取消了任务：

```go
go func() {
    ticker := time.NewTicker(5 * time.Second)
    for {
        if status, _ := d.client.GetTaskStatus(ctx, task.ID); status == "cancelled" {
            runCancel()   // 取消 agent 进程的 context
            return
        }
    }
}()
```

**Mulwiki**: 启动子进程后仅 `cmd.Wait()`，无法响应服务端取消。

---

### 8. Daemon ID 持久化

**Multica** 将 daemon_id 持久化到 `~/.multica/daemon.id`，跨重启保持稳定。启动时读取，不存在则生成。

```go
// 文件持久化
func PersistedDaemonID(cfgDir string) string {
    // ~/.multica/daemon.id
}
```

Server 端通过 `legacy_daemon_ids` 处理 hostname 到 UUID 的迁移。

**Mulwiki**: `DaemonID: uuid.New().String()` — 每次重启都生成新 ID，无法追踪同一个物理机器的历史任务。

---

### 9. Workspace 自动发现

**Multica** 的 daemon 启动时调用 `ListWorkspaces` 获取用户的所有 workspace，为每个注册 runtime。之后 30s 轮询，发现新 workspace 自动注册，用户退出的 workspace 自动清理。

```go
func (d *Daemon) syncWorkspacesFromAPI(ctx) {
    workspaces, _ := d.client.ListWorkspaces(ctx)
    for _, ws := range workspaces {
        if !alreadyTracked { registerRuntimesForWorkspace(ws) }
    }
    // 清理
    for stale := range currentNotIn(workspaces) { delete(d.workspaces, stale) }
}
```

**Mulwiki**: daemon 硬编码单个 `WorkspaceSlug`，不支持多 workspace。

---

### 10. 心跳返回值处理（Server-initiated Actions）

**Multica** 的 heartbeat 返回可以携带 server 发起的操作：

```go
func (d *Daemon) handleHeartbeatActions(resp *HeartbeatResponse) {
    if resp.PendingUpdate != nil { go d.handleUpdate(...) }
    if resp.PendingModelList != nil { go d.handleModelList(...) }
    if resp.PendingLocalSkills != nil { go d.handleLocalSkillList(...) }
}
```

Server 可通过心跳下发 CLI 更新、模型列表刷新、本地 skill 导入等操作。

**Mulwiki**: 心跳仅维持 online 状态，不处理返回值。

---

### 11. 环境变量 Blocklist

**Multica** 阻止用户通过 `custom_env` 覆盖关键内部变量：

```go
func isBlockedEnvKey(key string) bool {
    if strings.HasPrefix(upper, "MULTICA_") { return true }
    switch upper {
    case "HOME", "PATH", "USER", "SHELL", "TERM", "CODEX_HOME":
        return true
    }
    return false
}
```

**Mulwiki**: `mergeEnv` 函数无条件允许所有 `custom_env` 值覆盖系统变量。这可能导致 agent CLI 使用错误的 HOME/PATH 等。

---

## 第二部分：架构级差距（中期应采纳）

### A. Daemon WebSocket Hub（独立于前端 WS）

Multica 有两套独立的 WebSocket 系统：

| 特性 | 前端 WS Hub | Daemon WS Hub |
|------|-----------|---------------|
| 文件 | `realtime/hub.go` | `daemonws/hub.go` |
| 客户端 | 浏览器 | daemon 进程 |
| 认证 | Cookie + first-message JWT | Bearer: mdt_ (daemon token) |
| 索引 | per-scope rooms | per-runtime indexing |
| 用途 | 实时 UI 更新 | 任务唤醒通知 + 心跳双向通道 |

Daemon WS Hub 提供：
- 低延迟任务唤醒（replace HTTP polling delay）
- 双向心跳（server 可主动推送指令）
- 自动重连 + runtime set 变更通知

**Mulwiki**: 仅有一套前端 WS。如果未来需要低延迟任务分发，建议参考 Multica 的双层 WS 架构。

---

### B. Task 单表多态（issue/chat/autopilot/quick_create）

Multica 的 `agent_task_queue` 表通过三个可选的 FK 字段支持四种任务来源：

```sql
issue_id          UUID,    -- 常规 issue 任务
chat_session_id   UUID,    -- 聊天任务
autopilot_run_id  UUID,    -- 自动定时任务
context           JSONB,   -- quick_create 任务（无 FK）
```

`ResolveTaskWorkspaceID` 方法按优先级从这些 FK 中推导 workspace：

```go
func (s *TaskService) ResolveTaskWorkspaceID(task) string {
    if task.IssueID.Valid { return issue.WorkspaceID }
    if task.ChatSessionID.Valid { return chat_session.WorkspaceID }
    if task.AutopilotRunID.Valid { ... }
    if qc, ok := parseQuickCreateContext(task); ok { return qc.WorkspaceID }
}
```

**Mulwiki 适用性**: 当前仅需要 "source ingest" 任务源，但未来可能扩展到 chat 和 cron 场景。可以预留字段。

---

### C. Inbox 通知系统

Multica 的 quick_create / agent 完成/失败都通过 `inbox_item` 表发送通知：

```sql
inbox_item (
    id, workspace_id, recipient_type, recipient_id,
    type, severity, issue_id, title, body, details (JSONB)
)
```

通知包含原始 prompt、agent ID、任务 ID，前端可渲染 "Edit as advanced form" 按钮让用户改进 AI 生成的 issue。

**Mulwiki 适用性**: 当前不需要（因为是知识管理工具而非 issue tracker），但如果未来添加 "scheduled ingest" 的完成通知，可以借鉴这个模式。

---

## 第三部分：Mulwiki 独有的正确设计（不应改变）

| 领域 | Mulwiki 设计 | 原因 |
|------|----------------|------|
| 数据库 | SQLite | 单机部署，零运维。Multica 用 PostgreSQL 是因为要支持多租户云部署 |
| Job 系统 | source/schema/wiki 领域模型 | 知识编译场景的核心，Multica 的 issue/comment 不适合 |
| Schema 注入 | AGENTS.md + 工作目录隔离 | 直接借鉴了 Multica 的 execenv 模式，但针对知识管理做了适配 |
| Batch API | agent 列表批量加载 skills | 正确实现了 Multica 的单次查询模式 |
| CustomEnv Redaction | owner/admin 可见完整，其他人 redact | 完全对齐 Multica |
| WebSocket 客户端驱逐 | send channel 满时 evict | 完全对齐 Multica |
| Atomic Claim | SQLite BEGIN IMMEDIATE + subquery | SQLite 无 FOR UPDATE SKIP LOCKED 的最佳替代方案 |

---

## 第四部分：完整优先级路线图（修订版）

### Phase 1 — 代码级修复（本周，11 项）

| # | 项 | 工作量 | 影响 |
|---|-----|-------|------|
| 1 | Daemon ID 持久化（文件存储 + legacy_id 迁移） | 小 | 任务历史可追溯 |
| 2 | 环境变量 blocklist（`isBlockedEnvKey`） | 极小 | 安全性 |
| 3 | 孤儿任务恢复（daemon 启动时 RecoverOrphans） | 中 | 崩溃后自动恢复 |
| 4 | 运行中取消检测（5s 轮询） | 中 | 用户取消即时响应 |
| 5 | Session pinning（首次 session_id 即时持久化） | 中 | 崩溃后 session 不丢失 |
| 6 | CompleteTask 事务安全性（同事务内写 session/workdir） | 中 | 数据一致性 |
| 7 | 批量消息上报（500ms batch + task_message 表） | 中 | 实时进度可见 |
| 8 | 心跳返回值处理（支持 server-initiated actions） | 中 | 未来可扩展运维 |
| 9 | failure_reason 驱动 auto-retry（MaybeRetryFailedTask） | 中 | 可靠性 |
| 10 | Service 层分离（TaskService + AgentService） | 大 | 可测试性 |
| 11 | Agent Backend 接口抽象（至少 Claude + Codex） | 大 | 可扩展性 |

### Phase 2 — 架构升级（下月，3 项）

| # | 项 | 工作量 | 影响 |
|---|-----|-------|------|
| A | Daemon WebSocket Hub（双通道：唤醒 + 心跳双向） | 大 | 延迟从 5s 降到 100ms 级 |
| B | Task 单表多态（预留 chat_session_id / cron_schedule_id） | 中 | 未来扩展准备 |
| C | Workspace 自动发现（daemon 支持多 workspace） | 中 | 多项目支持 |

---

## 第五部分：具体实现参考

### 5.1 Daemon ID 持久化

```
~/.mulwiki/daemon.id  ← 文件内容：UUID
```

参考 Multica:

```go
func loadDaemonID(cfgDir string) string {
    path := filepath.Join(cfgDir, "daemon.id")
    if data, err := os.ReadFile(path); err == nil {
        return strings.TrimSpace(string(data))
    }
    id := uuid.New().String()
    os.MkdirAll(cfgDir, 0755)
    os.WriteFile(path, []byte(id), 0644)
    return id
}
```

启动时也需要处理 `legacy_daemon_ids`（旧 hostname 映射到新 UUID）。

---

### 5.2 孤儿任务恢复

Server 端 API:

```
POST /api/daemon/runtimes/{id}/recover-orphans
  → 查询 status IN ('dispatched', 'running') AND runtime_id = ?
  → 批量 UPDATE status = 'failed', failure_reason = 'runtime_recovery'
  → 对每个任务调用 MaybeRetryFailedTask
  → 广播 task:failed 事件
```

Mulwiki 的 SQLite 版：

```sql
UPDATE agent_tasks
SET status = 'failed', failure_reason = 'runtime_recovery', completed_at = datetime('now')
WHERE runtime_id = ? AND status IN ('dispatched', 'running');
```

---

### 5.3 环境变量 Blocklist

在 `daemon/daemon.go` 的 `mergeEnv` 中添加：

```go
func isBlockedEnvKey(key string) bool {
    upper := strings.ToUpper(key)
    if strings.HasPrefix(upper, "MULWIKI_") { return true }
    switch upper {
    case "HOME", "PATH", "USER", "SHELL", "TERM":
        return true
    }
    return false
}

// 在 mergeEnv 中：
for k, v := range custom {
    if isBlockedEnvKey(k) {
        slog.Warn("custom_env: blocked key skipped", "key", k)
        continue
    }
    // ... existing merge logic
}
```

---

### 5.4 Session Pinning + CompleteTask 事务

在一次 DB 事务中完成：

```go
// 伪代码（SQLite 事务）
tx, _ := db.Begin()
tx.Exec("UPDATE agent_tasks SET status='completed', session_id=?, work_dir=?, completed_at=? WHERE id=?",
    sessionID, workDir, now, taskID)
tx.Exec("UPDATE job_sessions SET session_id=?, work_dir=? WHERE id=?",
    sessionID, workDir, jobID)
tx.Commit()
```

Session pinning 在 daemon 首次检测到 session_id 时调用（不等待任务完成）：

```go
// 在 streamLogs 或消息处理循环中
if sessionID != "" && !pinned {
    pinned = true
    go d.client.PinSession(taskID, sessionID, workdir)
}
```

Server 端 PinSession 是一个轻量 PATCH。

---

### 5.5 Auto-Retry

参考 Multica 的 `retryableReasons` 白名单：

```go
var retryableReasons = map[string]bool{
    "runtime_offline":  true,
    "runtime_recovery": true,
    "timeout":          true,
}

func MaybeRetry(jobID string) {
    task := getTask(jobID)
    if !retryableReasons[task.FailureReason] { return }
    if task.Attempt >= task.MaxAttempts { return }
    createChild := INSERT INTO agent_tasks (
        agent_id, runtime_id, source_id, schema_id,
        parent_task_id, session_id, work_dir,
        attempt = parent.attempt + 1, max_attempts = parent.max_attempts
    )
    notifyWakeup(child)
}
```

---

### 5.6 Agent Backend 接口（最小可行版）

```go
// pkg/agent/agent.go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

type ExecOptions struct {
    Cwd             string
    Model           string
    Timeout         time.Duration
    ResumeSessionID string
    CustomArgs      []string
    Env             map[string]string
}

type Session struct {
    Messages <-chan Message
    Result   <-chan Result
}

type Message struct {
    Type      string // "text", "thinking", "tool_use", "tool_result", "status", "error"
    Content   string
    Tool      string
    Input     map[string]any
    Output    string
    SessionID string
}

type Result struct {
    Status  string // "completed", "blocked", "timeout", "cancelled", "failed"
    Output  string
    Error   string
    SessionID string
}
```

Claude Code backend 的最小实现（解析 `claude --print` 的 JSON 流输出）。

---

## 第六部分：不该借鉴的 Multica 设计

| Multica 特性 | 不适合 Mulwiki 的原因 |
|-------------|------------------------|
| Issue/Comment 系统 | Mulwiki 是知识编译工具，不是项目管理 |
| Kanban Board UI | 知识管理不需要看板 |
| Multi-member/RBAC | MVP 阶段单用户够用 |
| PostgreSQL + pgvector | 单机 SQLite 更适合，零运维 |
| Cloud deployment/Docker | 定位本地/单机 |
| Autopilot Cron 引擎 | 结构化摄入的调度逻辑不同 |
| Repo Cache (bare clone) | 知识管理不需要代码仓库 |
| Mention 展开系统 | 知识管理无 issue 标识符 |
| Analytics (PostHog 风格) | 本地工具不需要 |

---

## 总结

Mulwiki 从前两轮研究中正确借鉴了 Multica 的核心架构 — EventBus + Hub, Daemon 注册 + 心跳, Atomic Claim, Agent Task 状态机。这已经让它具备了工程级的 agent 执行基础设施。

本轮深度源码分析发现的 11 个代码级差距，集中在 **可靠性**（崩溃恢复、session 持久化、事务一致性）、**可观测性**（消息流式上报、实时进度）、**安全性**（环境变量 blocklist）、和 **可扩展性**（Backend 抽象、多 workspace 支持）。其中前 5 项（Daemon ID 持久化、环境变量 blocklist、孤儿恢复、取消检测、Session pinning）是最容易实施且收益最大的。

当前的 Mulwiki 代码质量已经远高于 "原型/mock" 阶段 — daemon 的 Agent 执行管线、workdir 构建、manifest 解析、wiki 合并都是完整的生产级实现。在这个基础上补齐上述的代码级差距，就能达到与 Multica 在 agent 执行基础设施层面完全对等的水平。

---

> 研究方法：逐文件读取 [raw.githubusercontent.com/multica-ai/multica](https://github.com/multica-ai/multica) main 分支源码
> 聚焦文件：`internal/service/task.go`, `internal/daemon/daemon.go`, `internal/realtime/hub.go`, `internal/handler/runtime.go`, `migrations/001_init.up.sql`
> Mulwiki 源码路径：`/Users/tethy/Documents/DevCode/github/mulwiki/server/`
> 生成时间：2026-04-30
