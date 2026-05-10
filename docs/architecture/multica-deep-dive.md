# Multica 源码深度对比分析

> 梦境系统 · 2026-04-28
> 目标：找出所有 Mulwiki 值得完全借鉴的设计
> 仓库：multica-ai/multica (22.3k stars, Go 42% + TypeScript 50%)

---

## 0. 总体架构概览

```
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Next.js 16     │────>│  Go Backend      │────>│  PostgreSQL 17   │
│   (App Router)   │<────│  (Chi + WS)      │<────│  (pgvector)      │
└──────────────────┘     └────────┬─────────┘     └──────────────────┘
                                  │
                           ┌──────┴─────────┐
                           │  Agent Daemon   │  local machine
                           │  (Go binary)    │  executes agent CLIs
                           └────────────────┘
```

| Layer | Multica Stack | Mulwiki 当前 |
|-------|--------------|----------------|
| Frontend | Next.js 16 App Router | SvelteKit (estimated) |
| Backend | Go (Chi router + sqlc + gorilla/websocket) | (决定中) |
| Database | PostgreSQL 17 + pgvector | (决定中) |
| Agent Runtime | 本地 daemon 进程 fork CLI | (无 —— 待设计) |

---

## 1. Agent 执行生命周期

### 1.1 数据库 Schema

Multica 用一张 `agent_task_queue` 表统一管理任务生命周期：

```sql
CREATE TABLE agent_task_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    runtime_id UUID,                          -- 哪个 runtime 执行
    issue_id UUID NOT NULL REFERENCES issue(id),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'dispatched', 'running', 'completed', 'failed', 'cancelled')),
    priority INT NOT NULL DEFAULT 0,
    attempt INT NOT NULL DEFAULT 1,           -- 第几次尝试
    max_attempts INT NOT NULL DEFAULT 3,      -- 最大尝试次数
    parent_task_id UUID,                      -- retry 链
    session_id TEXT,                          -- agent session 恢复指针
    work_dir TEXT,                            -- 工作目录恢复指针
    trigger_comment_id UUID,                  -- 触发评论
    chat_session_id UUID,                     -- chat 任务关联
    autopilot_run_id UUID,                    -- autopilot 任务关联
    failure_reason TEXT,                      -- 失败分类：timeout/runtime_offline/agent_error...
    dispatched_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ
);
```

**关键设计决策**：
- `attempt`/`max_attempts`/`parent_task_id` 构成完整的 retry 链
- `session_id`/`work_dir` 支持任务断点续传（agent session resume）
- `failure_reason` 是结构化的失败分类器，驱动 auto-retry 决策
- `chat_session_id`/`autopilot_run_id` 让同一张表服务三种完全不同来源的任务（issue驱动、chat驱动、autopilot驱动）

**Mulwiki 应该完全借鉴**：这张表的设计是 agent 任务系统的黄金标准。单表多态（issue/chat/autopilot）避免了多表 JOIN 的复杂性。

### 1.2 状态机

```
                  ┌─────────┐
                  │ queued  │  EnqueueTaskForIssue/EnqueueChatTask
                  └────┬────┘
                       │ ClaimTask (atomic SELECT ... FOR UPDATE SKIP LOCKED)
                  ┌────▼────┐
                  │dispatched│  task dispatched to daemon, not yet started
                  └────┬────┘
                       │ daemon calls StartTask
                  ┌────▼────┐
                  │ running  │  agent CLI is executing
                  └────┬────┘
                   ┌───┴───┐
              ┌────▼──┐ ┌──▼────┐
              │completed│ │failed │
              └────────┘ └──┬────┘
                            │ MaybeRetryFailedTask
                            │ (仅 retryable reasons)
                       ┌────▼────┐
                       │ queued  │  child task created
                       └─────────┘
```

**Atomic claim 实现**（`ClaimAgentTask` SQL）：
```sql
-- 核心思想：SELECT ... FOR UPDATE SKIP LOCKED 保证无锁争用
UPDATE agent_task_queue
SET status = 'dispatched', dispatched_at = now()
WHERE id = (
    SELECT id FROM agent_task_queue
    WHERE agent_id = $1 AND status = 'queued'
    ORDER BY priority DESC, created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;
```

**Mulwiki 应该完全借鉴**：`FOR UPDATE SKIP LOCKED` 模式是 PostgreSQL 下实现无锁任务分派的标准答案。Multica 还额外做了 `CountRunningTasks` 检查以确保不超过 `max_concurrent_tasks`。

### 1.3 Daemon 注册流程

Multica 使用 per-workspace **Runtime** 抽象：

```
1. 用户安装 multica CLI + 至少一个 agent CLI (claude/codex/openclaw/...)
2. multica daemon start
   ├── exec.LookPath("claude") → 检测到 claude
   ├── exec.LookPath("codex")  → 检测到 codex
   └── ...每个检测到的 CLI 成为一个 agent provider
3. Daemon 调用 POST /api/daemon/register {
     workspace_id: "xxx",
     daemon_id: "persistent-uuid",       // 持久化到 ~/.multica/daemon.id
     legacy_daemon_ids: ["old-hostname"],// hostname 迁移到 UUID
     runtimes: [
       { name: "Claude", type: "claude", version: "2.1.100", status: "online" },
       { name: "Codex",  type: "codex",  version: "0.100.0", status: "online" }
     ]
   }
4. Server 创建/更新 agent_runtime 行（UpsertAgentRuntime）
5. Runtime 状态 = "online"，对所有 workspace 可见
```

**Daemon ID 持久化**：Multica 将 `daemon_id` 写入 `~/.multica/daemon.id`（跨重启保持稳定），而不是依赖 hostname（hostname 可能因网络变化而漂移）。

**Mulwiki 应该完全借鉴**：
- 将 Runtime 作为一等公民（agent 绑定到 runtime，而非 daemon）
- 持久化的 daemon_id（UUID 文件）比 hostname 可靠
- exec.LookPath 检测模式简洁高效

### 1.4 Daemon 主循环

```go
func (d *Daemon) pollLoop(ctx context.Context, taskWakeups <-chan struct{}) error {
    sem := make(chan struct{}, d.cfg.MaxConcurrentTasks) // 并发控制 semaphore

    for {
        runtimeIDs := d.allRuntimeIDs()
        for _, rid := range runtimeIDs {
            sem <- struct{}{}                            // 获取 slot
            task, err := d.client.ClaimTask(ctx, rid)    // HTTP POST /claim
            if task != nil {
                go func(t Task) {
                    defer func() { <-sem }()
                    d.handleTask(ctx, t)                 // fork agent CLI
                }(task)
                break
            }
            <-sem                                         // 无任务，释放 slot
        }
        sleepWithContextOrWakeup(ctx, pollInterval, taskWakeups) // 3s 默认
    }
}
```

**关键设计**：
- Semaphore 控制并发任务数（默认 20）
- Claim 是 HTTP POST（不是长轮询），简单可靠
- 通过 daemon WebSocket 接收 `daemon:task_available` 事件来 wakeup（避免纯轮询延迟）
- Round-robin 遍历所有 runtime 确保公平

**Mulwiki 应该借鉴**：Concurrency semaphore + HTTP claim + WS wakeup 的三层设计。比纯轮询延迟低，比纯 push 更可靠。

### 1.5 Workdir 和 Session 管理

Multica 的 workdir 设计优雅：

```
~/multica_workspaces/
├── .repos/                          # bare clone cache
│   └── {workspace_id}/
│       └── {repo_url_hash}/
├── {workspace_id}/
│   └── {agent_name}-{short_task_id}/ # 每次任务的隔离工作目录
│       ├── .gc_meta.json            # GC 元数据
│       ├── .agent_context/          # agent 配置文件
│       └── (git worktree checkout)
```

**Session 恢复**：
1. 任务开始前，ClaimTask 响应返回 `prior_session_id` 和 `prior_work_dir`
2. Daemon 调用 `execenv.Reuse()` 重用之前的 workdir
3. Agent CLI 以 `--resume {session_id}` 启动
4. Agent 首次 emit status 消息时，daemon 立刻调用 `PinTaskSession` 将 session_id/work_dir 写回 DB
5. 即使 daemon 崩溃，restart 后 session_id 不丢失

**Mulwiki 应该完全借鉴**：
- Workdir 复用（PriorWorkDir）+ Session 恢复（PriorSessionID）模式
- `.gc_meta.json` 文件用于 GC 清理
- PinTaskSession 早期持久化策略防止数据丢失

### 1.6 Agent CLI Fork 模式

Multica 的 `agent.Backend` 接口是对所有 agent CLI 的统一抽象：

```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}
```

每种 CLI 有自己的 backend（claude.go, codex.go, openclaw.go...），统一通过 subprocess stdout 的 JSON 流解析标准化的 `Message` 类型：

```go
type Message struct {
    Type      MessageType   // text, thinking, tool-use, tool-result, status, error
    Content   string
    Tool      string
    CallID    string
    Input     map[string]any
    Output    string
    SessionID string        // 用于早期 session 持久化
}
```

**executeAndDrain 核心循环**：
```go
func (d *Daemon) executeAndDrain(ctx, backend, prompt, opts, taskLog, taskID) {
    session, _ := backend.Execute(ctx, prompt, opts)

    go func() {
        ticker := time.NewTicker(500 * time.Millisecond)  // 500ms 批量上报
        var batch []TaskMessageData
        for msg := range session.Messages {
            batch = append(batch, convert(msg))
            // 第一次收到 session_id 时 pin session
            if msg.SessionID != "" && !sessionPinned.Swap(true) {
                d.client.PinTaskSession(ctx, taskID, msg.SessionID, opts.Cwd)
            }
        }
        flush(batch)
    }()

    result := <-session.Result
    // 处理 result.Status: completed / blocked / timeout / cancelled / failed
}
```

**Mulwiki 应该借鉴**：
- 统一 Backend 接口 + per-provider 实现模式
- 500ms 批量上报消息（减少 HTTP 请求数）
- Session pinning 作为 crash recovery 保障

---

## 2. 实时事件系统

### 2.1 双层架构：EventBus (in-process) + Hub (WebSocket)

```
┌──────────────────────────────────────────────────────┐
│                   Handler Layer                      │
│  h.publish(event) ──────────────────────────────┐    │
└──────────────────────────────────────────────────│───┘
                                                   ▼
┌──────────────────────────────────────────────────────┐
│              events.Bus (in-process)                 │
│  Subscribe("task:completed", handler)                │
│  Subscribe("agent:status", handler)                  │
│  Publish(Event{Type, WorkspaceID, Payload, TaskID})  │
└──────────────────────┬───────────────────────────────┘
                       │
          ┌────────────┴────────────┐
          ▼                         ▼
┌──────────────────┐    ┌──────────────────────────┐
│ realtime.Hub     │    │ 其他监听器               │
│ (WebSocket fanout)│   │ (analytics, mention, etc)│
│                  │    └──────────────────────────┘
│ rooms: map[      │
│   "workspace:X"  │
│   "task:Y"       │
│   "user:Z"       │
│ ]                │
└──────────────────┘
```

**EventBus**（`server/internal/events/bus.go`）：
```go
type Bus struct {
    mu        sync.RWMutex
    listeners map[string][]Handler       // type -> handlers
    globalHandlers []Handler              // SubscribeAll
}

func (b *Bus) Publish(e Event) {
    // 1. 按 event type 分发到具体 listeners
    // 2. 分发到 global listeners
    // 3. 每个 handler 有独立的 panic recovery
}
```

**Event 结构**：
```go
type Event struct {
    Type        string    // "task:completed", "agent:status", "comment:created" ...
    WorkspaceID string    // 多租户路由 key
    ActorType   string    // "member", "agent", "system"
    ActorID     string
    Payload     any       // JSON-serializable

    // 窄播 hints（用于更精确的 WS rooms）
    TaskID        string
    ChatSessionID string
}
```

**Mulwiki 应该完全借鉴**：
- EventBus 与 Hub 分层设计：Bus 处理所有事件订阅（analytics, mentions, notifications），Hub 只负责 WebSocket fanout
- Event 结构携带 WorkspaceID 用于多租户路由
- TaskID/ChatSessionID 作为 scope hints 用于窄播优化

### 2.2 WebSocket Hub 架构

**前端 Hub**（`server/internal/realtime/hub.go`）：

```go
type Hub struct {
    rooms     map[scopeKey]map[*Client]bool  // "workspace:abc" -> {client1, client2}
    clients   map[*Client]bool               // 所有连接
    broadcast chan []byte                    // 全局广播 channel
    register   chan *Client
    unregister chan *Client
    authorizer ScopeAuthorizer               // 权限检查接口

    onFirstSubscriber SubscriptionCallback    // 用于 Redis relay 生命周期
    onLastSubscriber  SubscriptionCallback
}
```

**Scope 类型**：
```go
const (
    ScopeWorkspace = "workspace"       // 自动订阅
    ScopeUser      = "user"           // 自动订阅（跨设备同步）
    ScopeTask      = "task"           // 按需订阅（需 ScopeAuthorizer 授权）
    ScopeChat      = "chat"           // 按需订阅
    ScopeDaemonRuntime = "daemon_runtime"  // daemon WS 专用
)
```

**连接认证流程**：
```
1. Cookie 认证（优先）：读取 multica_auth cookie -> JWT 验证 -> 查 membership
2. First-message 认证（fallback）：首条 WS 消息 {"type":"auth","payload":{"token":"..."}}
3. 查询参数认证：workspace_id / workspace_slug
4. 自动订阅 workspace:{id} 和 user:{id}
5. 客户端可手动 subscribe("task", taskId) —— 需 ScopeAuthorizer 验证权限
```

**写 Pump 模式**（标准 gorilla/websocket 模式）：
```go
func (c *Client) writePump() {
    ticker := time.NewTicker(54s)  // pingPeriod
    for {
        select {
        case message := <-c.send:    // buffer size: 256
            c.conn.WriteMessage(TextMessage, message)
        case <-ticker.C:
            c.conn.WriteMessage(PingMessage, nil)
        }
    }
}
```

**慢客户端驱逐**：
```go
func (h *Hub) BroadcastToScope(scopeType, scopeID string, message []byte) {
    for client := range h.rooms[key] {
        select {
        case client.send <- message:
            sent++
        default:                             // channel 满 → 标记 slow
            slow = append(slow, client)
        }
    }
    if len(slow) > 0 {
        h.evictSlow(slow)                   // 批量驱逐
    }
}
```

**Mulwiki 应该完全借鉴**：
- Cookie-first + first-message fallback 的双路径认证
- Scope 系统（workspace/user/task/chat）按需订阅
- 慢客户端驱逐机制
- 事件去重（LRU cache of event IDs, 128 cap）

### 2.3 Redis Relay（多节点部署）

Multica 使用 Redis Streams 做跨节点消息 relay：

```go
// 当某个 scope 在本节点的第一个订阅者出现时，启动 Redis consumer
h.SetSubscriptionCallbacks(
    onFirst: func(scopeType, scopeID) {
        relay.StartConsumer(scopeType, scopeID)    // XREADGROUP
    },
    onLast: func(scopeType, scopeID) {
        relay.StopConsumer(scopeType, scopeID)
    },
)

// DualWrite: 先写到 local Hub，再 XADD 到 Redis
broadcaster.BroadcastToScope(scope, id, message)
relay.Publish(scope, id, message)   // 其他节点通过 consumer 接收
```

**去重机制**：消息携带 `event_id`，client 端维护最近 128 个 seen event IDs。先 local 再 Redis relay 的 dual-write 不会导致重复投递。

**Mulwiki 应该借鉴**：如果需要多 server 实例部署，这个 Redis relay 模式是成熟方案。初期单实例可以跳过。

### 2.4 Daemon WebSocket Hub（独立实现）

Multica 有**两套独立的 WebSocket 系统**：

| 特性 | 前端 WS Hub | Daemon WS Hub |
|------|-----------|---------------|
| 客户端 | 浏览器 | daemon 进程 |
| 认证 | Cookie + first-message JWT | Bearer: mdt_ (daemon token) |
| 索引 | per-scope rooms | per-runtime indexing |
| 用途 | 实时 UI 更新 | 任务唤醒通知 |
| 文件 | realtime/hub.go | daemonws/hub.go |

Daemon WS Hub 的 `NotifyTaskAvailable` 发送最佳努力唤醒信号，daemon 仍通过 HTTP claim 获取任务（defense in depth）。

```go
// Daemon WS 收到唤醒后
case <-taskWakeups:
    // 立即进入 poll loop，跳过 sleep
```

**Mulwiki 应该借鉴**：
- 两套 WS 的分离设计清晰合理
- 唤醒信号是 hint 不是承诺（HTTP claim 仍是权威路径）
- per-runtime 索引直接对应物理机器

### 2.5 协议事件清单

所有事件类型定义在 `server/pkg/protocol/events.go`：

```
Issue:     issue:created, issue:updated, issue:deleted
Comment:   comment:created, comment:updated, comment:deleted
Task:      task:dispatch, task:progress, task:completed, task:failed, task:cancelled, task:message
Agent:     agent:status, agent:created, agent:archived, agent:restored
Workspace: workspace:updated, workspace:deleted
Member:    member:added, member:updated, member:removed
Chat:      chat:message, chat:done, chat:session_read
Daemon:    daemon:register, daemon:task_available
...
```

**Mulwiki 应该借鉴**：以 `resource:action` 命名约定，清晰可扩展。

### 2.6 Task Message 流式上报

Agent 执行过程中的消息通过以下路径：

```
Agent CLI stdout → agent.Backend (parse) → daemon.executeAndDrain (batch 500ms)
  → HTTP POST /api/daemon/tasks/{id}/messages (batch)
  → DB Insert (task_message table)
  → h.publishTask("task:message", workspaceID, payload)
  → Hub.BroadcastToScope("task:{id}", ...)
  → 前端实时渲染
```

消息支持增量拉取（`GET /api/daemon/tasks/{id}/messages?since=42`），用于重连后追赶。

**Mulwiki 应该借鉴**：
- 500ms 批量上报平衡延迟和吞吐
- `since` 参数支持增量同步
- 消息存储在独立表 `task_message`，包含 seq, type, tool, content, input, output

---

## 3. Workspace 权限模型

### 3.1 数据模型

```sql
-- 成员表：user <-> workspace 的 N:M 关系
CREATE TABLE member (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    UNIQUE(workspace_id, user_id)
);
```

三层角色：
- **owner**：workspace 创建者，不可降级，唯一
- **admin**：完全管理权限
- **member**：基本访问权限

### 3.2 中间件链

```
Request → Auth (JWT/PAT) → RequireWorkspaceMember → Handler
```

**Auth 中间件**（`middleware/auth.go`）：
```go
func Auth(queries *db.Queries) func(http.Handler) http.Handler {
    // 1. extractToken: Authorization header > multica_auth cookie
    // 2. PAT (mul_ prefix): hash token, DB lookup, set X-User-ID
    // 3. JWT: parse, validate HMAC, extract sub claim, set X-User-ID
    // Cookie 认证需要 CSRF 验证（state-changing methods）
}
```

**Workspace 中间件**（`middleware/workspace.go`）：
```go
func RequireWorkspaceMember(queries) func(http.Handler) http.Handler {
    // 1. resolve workspace: slug > header > query param
    // 2. 读 X-User-ID header
    // 3. DB 查询 GetMemberByUserAndWorkspace
    // 4. 设置 context: workspace_id + member
}
```

**Role-gated 变体**：
```go
// 仅允许 owner/admin
r.Use(middleware.RequireWorkspaceRole(queries, "owner", "admin"))

// 从 URL param 读取 workspace
r.Use(middleware.RequireWorkspaceMemberFromURL(queries, "workspaceId"))
```

### 3.3 Daemon 认证

Daemon 有独立认证链（`middleware/daemon_auth.go`）：

```go
func DaemonAuth(queries) func(http.Handler) http.Handler {
    // 1. mdt_ prefix: daemon token → GetDaemonTokenByHash
    //    → context: workspace_id + daemon_id
    // 2.  fallback: PAT (mul_) → user token
    // 3.  fallback: JWT → user token
}
```

**Daemon 访问控制**（handler/daemon.go）：
```go
func (h *Handler) requireDaemonWorkspaceAccess(w, r, workspaceID) bool {
    // daemon token: 直接比较 token 的 workspace_id
    // user token: 查 membership
}
```

### 3.4 Agent 字段权限

Agent 的 `custom_env` 和 `mcp_config` 包含敏感信息，Multica 实现了选择性 redaction：

```go
func canViewAgentEnv(agent, userID, memberRole) bool {
    return roleAllowed(memberRole, "owner", "admin") || agent.owner_id == userID
    // 非 owner/admin 看到的是 value="****" 的 env
}
```

**Mulwiki 应该完全借鉴**：
- 中间件链模式清晰：Auth → Workspace → Role
- slug-first URL 解析（后续可 URL 化 workspace）
- 双认证路径：用户 token（JWT/PAT）和 daemon token（mdt_）
- 字段级 redaction 而非整个资源拒绝

---

## 4. CLI 自动检测与注册

### 4.1 检测逻辑

Multica 的检测代码在 `daemon/config.go` 中，通过 `exec.LookPath` 扫描：

```go
agents := map[string]AgentEntry{}

// 每种 agent 有独立的 PATH 覆盖环境变量
claudePath := envOrDefault("MULTICA_CLAUDE_PATH", "claude")
if _, err := exec.LookPath(claudePath); err == nil {
    agents["claude"] = AgentEntry{
        Path:  claudePath,
        Model: os.Getenv("MULTICA_CLAUDE_MODEL"),
    }
}

codexPath := envOrDefault("MULTICA_CODEX_PATH", "codex")
if _, err := exec.LookPath(codexPath); err == nil {
    agents["codex"] = AgentEntry{
        Path:  codexPath,
        Model: os.Getenv("MULTICA_CODEX_MODEL"),
    }
}
// ... Claude, Codex, Copilot, OpenClaw, OpenCode, Hermes, Gemini, Pi, Cursor, Kimi, Kiro
```

**版本验证**：检测到的每个 agent 版本通过 `agent.CheckMinVersion` 校验：

```go
var MinVersions = map[string]string{
    "claude":  "2.0.0",
    "codex":   "0.100.0",
    "copilot": "1.0.0",
}

func CheckMinVersion(agentType, detectedVersion string) error {
    // 用 regexp 解析 semver，比较 major.minor.patch
}
```

### 4.2 注册到 Server

```go
// daemon.Run() 中
func (d *Daemon) registerRuntimesForWorkspace(ctx, workspaceID) {
    for name, entry := range d.cfg.Agents {
        version, _ := agent.DetectVersion(ctx, entry.Path)     // claude --version
        agent.CheckMinVersion(name, version)                    // semver check
        runtimes = append(runtimes, {name, type, version, status: "online"})
    }

    // POST /api/daemon/register
    d.client.Register(ctx, {workspace_id, daemon_id, runtimes})
}
```

**Server 端 upsert**（`handler/daemon.go`）：
```go
func (h *Handler) DaemonRegister(w, r) {
    for _, runtime := range req.Runtimes {
        row := h.Queries.UpsertAgentRuntime(r.Context(), UpsertAgentRuntimeParams{
            WorkspaceID: wsUUID,
            DaemonID:    req.DaemonID,
            Provider:    runtime.Type,
            Status:      "online",
            Metadata:    {version, cli_version, launched_by},
        })
        // 合并 legacy daemon_ids 的 runtime 行
        h.mergeLegacyRuntimes(r, row, provider, req.LegacyDaemonIDs)
    }
}
```

### 4.3 Workspace 自动发现

Daemon 启动时通过 API 自动发现所有 workspace：

```go
func (d *Daemon) syncWorkspacesFromAPI(ctx) {
    workspaces, _ := d.client.ListWorkspaces(ctx)   // GET /api/workspaces

    for _, ws := range workspaces {
        if alreadyTracked(ws.ID) { continue }
        d.registerRuntimesForWorkspace(ctx, ws.ID)   // 为新 workspace 注册 runtime
        d.client.RecoverOrphans(ctx, runtimeID)       // 恢复孤儿任务
    }

    // 清理用户已退出的 workspace
    for stale := range currentNotIn(workspaces) {
        delete(d.workspaces, stale)
    }
}
```

**Mulwiki 应该完全借鉴**：
- `exec.LookPath` + 环境变量覆盖的模式
- Version check 确保最低版本要求
- Workspace 自动发现让 daemon 无需手动配置 target workspace
- Legacy daemon ID 合并处理 hostname 迁移

---

## 5. 错误恢复与孤儿任务

### 5.1 失败分类系统

Multica 使用 `failure_reason` 字段对失败进行分类：

```go
// 可重试的失败原因（基础设施故障）
var retryableReasons = map[string]bool{
    "runtime_offline":    true,    // runtime 心跳超时
    "runtime_recovery":   true,    // daemon 崩溃后恢复
    "timeout":            true,    // agent 执行超时
}

// 不可重试的失败原因（agent 自身错误）
// "agent_error" —— 编译错误、模型拒绝等
```

### 5.2 自动重试逻辑

```go
func (s *TaskService) MaybeRetryFailedTask(ctx, parent) (*AgentTaskQueue, error) {
    // 1. 检查 failure_reason 是否在 retryableReasons 中
    // 2. 检查 attempt < max_attempts
    // 3. 排除 autopilot 任务（autopilot 有自己的重试策略）
    // 4. 创建子任务：继承 agent/runtime/issue/chat session
    // 5. child.parent_task_id = parent.id
    // 6. child.attempt = parent.attempt + 1
    // 7. child.session_id = parent.session_id  // 继承 session 用于恢复
    // 8. 通知 daemon
}
```

### 5.3 孤儿任务恢复

**Daemon 启动时恢复**：
```go
// daemon.syncWorkspacesFromAPI() 中
for _, rid := range runtimeIDs {
    d.client.RecoverOrphans(ctx, rid)  // POST /api/daemon/runtimes/{id}/recover-orphans
}
```

**Server 端处理**（`handler/task_lifecycle.go`）：
```go
func (h *Handler) RecoverOrphanedTasks(w, r) {
    // 1. 查询所有 dispatched/running 状态且属于该 runtime 的任务
    rows := h.Queries.RecoverOrphanedTasksForRuntime(ctx, runtimeID)
    // 2. 将这些任务标记为 failed（原因 = "runtime_recovery"）
    // 3. 通过 HandleFailedTasks 触发：
    //    - MaybeRetryFailedTask（自动重试）
    //    - task:failed 事件广播
    //    - agent 状态调和
    //    - 卡住的 issue 状态回滚到 todo
}
```

### 5.4 Heartbeat 清扫器

Server 端有独立的 heartbeat sweeper：如果 runtime 在 N 秒内未发送 heartbeat，将其标记为 offline，并清扫其 running/dispatched 的任务：

```
heartbeat 间隔: 15s
offline 判定: ~75s（5 个 heartbeat 周期）
清扫动作: FailTask(runtime_offline) → MaybeRetryFailedTask
```

### 5.5 运行时取消检测

Daemon 在任务执行期间每 5s 轮询任务状态：

```go
go func() {
    ticker := time.NewTicker(5 * time.Second)
    for {
        status, _ := d.client.GetTaskStatus(ctx, task.ID)  // GET /api/daemon/tasks/{id}/status
        if status == "cancelled" {
            runCancel()   // 取消 agent 进程的 context
            return
        }
    }
}()
```

### 5.6 Workspace GC

Daemon 有独立的 GC 循环：

```go
// 1h 间隔扫描
func (d *Daemon) gcLoop(ctx) {
    d.runGC(ctx)  // 启动后 30s 首次运行
    ticker := time.NewTicker(1 * time.Hour)
}

// 清理策略
func shouldCleanTaskDir(taskDir) {
    meta := ReadGCMeta(taskDir)     // .gc_meta.json
    status := API.GetIssueGCCheck(meta.IssueID)

    if (status == "done" || status == "canceled") && time.Since(updatedAt) > 24h {
        return CLEAN                    // 完成的 issue 24h 后清理
    }
    if noMeta && time.Since(mtime) > 72h {
        return ORPHAN                   // 无元数据的孤儿目录 72h 后清理
    }
    return SKIP
}
```

**Mulwiki 应该完全借鉴**：
- failure_reason 分类驱动自动重试
- 启动时 RecoverOrphanedTasks 处理崩溃遗留
- Heartbeat sweeper 作为兜底
- 5s 轮询检测任务取消
- GC 循环用 .gc_meta.json 驱动

---

## 6. MCP Server 配置

### 6.1 Agent 配置字段

Agent 表包含三个关键扩展字段：

```sql
-- agent 表
custom_env  JSONB,    -- {"ANTHROPIC_API_KEY": "sk-...", "ANTHROPIC_BASE_URL": "..."}
custom_args JSONB,    -- ["--verbose", "--dangerously-skip-permissions"]
mcp_config  JSONB,    -- MCP server 配置 {"mcpServers": {...}}
model       TEXT,      -- 覆盖默认模型
```

### 6.2 MCP Config 生命周期

```
1. 创建 agent 时设置 mcp_config
2. 更新 agent 时可修改或清空（"mcp_config": null → 清空）
3. ClaimTask 响应中包含 agent.McpConfig
4. Daemon 通过 --mcp-config 临时文件传递给 agent CLI
```

### 6.3 环境变量保护

Daemon 阻止用户通过 `custom_env` 覆盖关键内部变量：

```go
func isBlockedEnvKey(key string) bool {
    if strings.HasPrefix(strings.ToUpper(key), "MULTICA_") { return true }
    switch strings.ToUpper(key) {
    case "HOME", "PATH", "USER", "SHELL", "TERM", "CODEX_HOME":
        return true
    }
    return false
}
```

**Mulwiki 应该借鉴**：
- 将 custom_env/custom_args/mcp_config 作为 agent 的一等配置字段
- 敏感字段 redaction 策略
- 环境变量 block list 防止内部变量被覆盖

---

## 7. Auth 与会话

### 7.1 双 Token 模型

| Token 类型 | 前缀 | 用途 | 生命周期 |
|-----------|------|------|---------|
| JWT | (无前缀) | 浏览器 session | Cookie (multica_auth) |
| Personal Access Token | mul_ | CLI/daemon 认证 | 90 天 |
| Daemon Token | mdt_ | daemon ↔ server 专用 | 持久 |

### 7.2 Token 提取优先级

```
HTTP Request → Auth Middleware
  1. Authorization: Bearer <token>  (header, 最高优先级)
  2. multica_auth cookie            (fallback, 需要 CSRF)
```

### 7.3 CSRF 保护

Cookie 认证（multica_auth）对于状态变更方法需要 CSRF token：
- 服务端设置 `csrf_token` cookie
- 客户端在 `X-CSRF-Token` header 中携带
- 中间件比较两者

### 7.4 PAT 处理

```go
// PAT 使用 SHA-256 hash 存储，不存明文
hash := auth.HashToken(tokenString)
pat, err := queries.GetPersonalAccessTokenByHash(ctx, hash)

// 后台更新 last_used_at
go queries.UpdatePersonalAccessTokenLastUsed(context.Background(), pat.ID)
```

**Mulwiki 应该借鉴**：
- 多 token 类型支持（JWT + PAT + daemon token）
- Token hash 存储
- CSRF 保护用于 cookie 认证
- WebSocket 的 cookie-first + first-message 双路径

---

## 8. 前端 Agent 配置 UI

### 8.1 组件结构

```
packages/views/
├── agents/                    # Agent 列表/详情/创建
│   ├── components/            # UI 组件
│   ├── config.ts              # 配置常量
│   └── presence.ts            # 在线状态派生
├── runtimes/                  # Runtime 管理
├── settings/                  # 设置页面
│   └── components/            # workspace/agent settings
└── layout/
    └── app-sidebar.tsx        # 侧边栏 workspace 切换
```

### 8.2 Agent 列表的关键 API

Multica 前端使用一套精心设计的批量获取 API：

```
GET /api/agents                          → ListAgents (批量加载 skills)
GET /api/agents/run-counts               → 30天运行计数（单次查询全 workspace）
GET /api/agents/activity                 → 30天每日 activity bucket
GET /api/agents/task-snapshot            → 每个 agent 的 active task + latest outcome
```

**设计精妙之处**：Activity sparkline 和 Run counts 是**一次 API 调用获取全 workspace 数据**，前端按 agent_id 分发。避免了 N+1 查询。

### 8.3 Agent 状态

Agent 状态由 DB 触发器 + `ReconcileAgentStatus` 维护：

```sql
-- agent.status 由 active tasks 推导
-- idle: 无 active task
-- working: 至少一个 running/dispatched/queued task
-- blocked: 最近失败
-- error: 多次失败
-- offline: runtime 离线
```

前端通过 `agent:status` WebSocket 事件实时更新。

### 8.4 创建 Agent 流程

1. 选择 runtime（runtime 列表显示 online/offline 状态）
2. 选择 provider（claude/codex/openclaw/...）
3. 设置 name, description, instructions
4. 配置 custom_env（API keys 等）
5. 配置 MCP servers（JSON editor）
6. 设置 visibility（workspace / private）
7. 设置 max_concurrent_tasks（默认 6）

**Mulwiki 应该借鉴**：
- 批量 API 减少前端请求（run-counts, activity, task-snapshot）
- agent 状态由 active tasks 推导而非手动设置
- Agent 创建时的 runtime 选择器设计

### 8.5 前端架构关键决策

从 `HANDOFF_ARCHITECTURE_AUDIT.md` 可知：

1. **QueryClient 配置**：
   - `staleTime: Infinity`：cache 永不主动过期
   - `refetchOnWindowFocus: false`：tab 聚焦不 refetch
   - 完全依赖 WebSocket 推送来做 cache invalidation

2. **Workspace State 管理**：
   - Zustand + `createWorkspaceAwareStorage`：每个 workspace 有独立的 localStorage key
   - 大多数 store 正确隔离，只有 navigation store 有 bug（已记录）

3. **缺少前端心跳检测**：
   - 浏览器 `WebSocket` API 不暴露 ping/pong 帧
   - 当 TCP 连接被静默切断时，`onclose` 不触发
   - 需要 `visibilitychange` + 应用层心跳修复

**Mulwiki 应借鉴但改进**：
- 不要设 `staleTime: Infinity` —— 加一个合理的 TTL（如 5min）
- 从第一天就加 `visibilitychange` 兜底
- WS 断线重连时全量 invalidate

---

## 9. 其他发现

### 9.1 数据库迁移策略

Multica 使用纯 SQL 迁移（无 ORM migration 框架）：

```
server/migrations/
├── 001_init.up.sql / .down.sql
├── 002_agent_config.up.sql / .down.sql
├── ...
└── 040_chat_unread_since.up.sql / .down.sql
```

Go 代码中有简单的迁移引擎（`migrations/migrations.go`）按版本号排序执行。

**Mulwiki 应该借鉴**：纯 SQL 迁移比 ORM 迁移更可控。pgx + sqlc 组合（Multica 也用这个）性能优良。

### 9.2 sqlc 代码生成

Multica 使用 `sqlc` 从 SQL 文件生成类型安全的 Go 查询代码：

```
server/pkg/db/
├── queries/          # .sql 查询定义文件
└── generated/        # sqlc 生成的 Go 代码
```

### 9.3 Analytics 集成

Multica 有自己的 analytics 系统，关键事件上报包括：
- `agent_created`（含 template 字段跟踪模板使用）
- `runtime_registered`
- `issue_executed`（含 duration_ms）
- `workspace_created`

使用 PostHog 风格的事件模型。

### 9.4 Redact 系统

任务消息在存储和广播前经过 redact 处理：
- `redact.Text(content)` 处理文本中的敏感信息
- `redact.InputMap(input)` 处理工具调用参数中的敏感信息

### 9.5 Agent Prompt 构建

Daemon 在 `prompt.go` 中构建完整的 agent prompt，包含：
- Issue 标题和描述
- Agent instructions
- Trigger comment 内容（如果有）
- Repo 信息
- Skills 内容（作为上下文注入）

---

## 10. 总结：优先级排序

### P0（立即借鉴 —— 架构基石）

| # | 模式 | 为什么 | 文件参考 |
|---|------|--------|---------|
| 1 | **agent_task_queue 单表多态** | 一张表服务 issue/chat/autopilot 三种任务源，attempt/max_attempts/parent_task_id 构成完整 retry 链，session_id/work_dir 支持断点续传 | `server/migrations/001_init.up.sql` |
| 2 | **Atomic Claim with FOR UPDATE SKIP LOCKED** | PostgreSQL 下最可靠的无锁任务分派模式 | `ClaimAgentTask` SQL query |
| 3 | **事件总线 + Hub 双层架构** | Bus 处理所有事件订阅，Hub 只管 WebSocket fanout；Event 携带 WorkspaceID 做多租户路由 | `events/bus.go`, `realtime/hub.go` |
| 4 | **Daemon 注册 → Runtime 抽象** | 每个 agent provider 注册为一个 Runtime，agent 绑定到 runtime；持久化 daemon_id | `handler/daemon.go:DaemonRegister` |
| 5 | **中间件链：Auth → Workspace → Role** | 清晰的权限分层，Cookie+token 双认证路径 | `middleware/auth.go`, `middleware/workspace.go` |

### P1（下一步借鉴 —— 质量保障）

| # | 模式 | 为什么 | 文件参考 |
|---|------|--------|---------|
| 6 | **failure_reason 驱动的自动重试** | retryableReasons 白名单 + MaybeRetryFailedTask + HandleFailedTasks 统一后处理 | `service/task.go` |
| 7 | **RecoverOrphanedTasks 启动恢复** | daemon 启动时立即恢复上一次崩溃遗留的任务，无需等 heartbeat sweeper | `handler/task_lifecycle.go` |
| 8 | **Workdir + Session 恢复** | PriorWorkDir + PriorSessionID + PinTaskSession 早期持久化 | `daemon/daemon.go:handleTask` |
| 9 | **批量消息上报 (500ms)** | 平衡实时性和 HTTP 请求开销 | `daemon/daemon.go:executeAndDrain` |
| 10 | **前端批量 API (run-counts + activity + task-snapshot)** | 一次请求获取全 workspace 数据，前端按 agent_id 分发 | `handler/agent.go` |
| 11 | **字段级 redaction (custom_env/mcp_config)** | 非 owner 看到 value="****" 而非整个字段隐藏 | `handler/agent.go:redactEnv` |

### P2（有时间再说）

| # | 模式 | 为什么 | 文件参考 |
|---|------|--------|---------|
| 12 | **Redis Relay (多节点部署)** | Dual-write + consumer 按需启停 + event ID 去重 | `realtime/redis_relay.go` |
| 13 | **Daemon WebSocket Hub (独立于前端 WS)** | 两套 WS 系统，daemon 用 per-runtime 索引，前端用 per-scope 索引 | `daemonws/hub.go` |
| 14 | **Workspace GC** | .gc_meta.json + issue status check + orphan TTL 三层清理策略 | `daemon/gc.go` |
| 15 | **MCP config per-agent** | JSONB 存储，敏感 redaction，通过临时文件传给 CLI | `handler/agent.go` |
| 16 | **Agent Backend 接口抽象** | 每种 CLI 实现同一 Backend 接口，统一消息流格式 | `pkg/agent/agent.go` |
| 17 | **sqlc + 纯 SQL 迁移** | 类型安全的 DB 查询 + 可控的迁移管理 | `pkg/db/`, `migrations/` |
| 18 | **exec.LookPath 自动检测** | 每种 agent 的 PATH 可用环境变量覆盖，启动时版本校验 | `daemon/config.go` |

---

## 附录：关键代码片段

### A. Agent 状态机 (ClaimTask in service/task.go)

```go
func (s *TaskService) ClaimTask(ctx context.Context, agentID pgtype.UUID) (*db.AgentTaskQueue, error) {
    // 1. 读 agent 信息（含 max_concurrent_tasks）
    agent, err := s.Queries.GetAgent(ctx, agentID)

    // 2. 检查并发限制
    running, _ := s.Queries.CountRunningTasks(ctx, agentID)
    if running >= int64(agent.MaxConcurrentTasks) {
        return nil, nil // 无容量
    }

    // 3. 原子认领（FOR UPDATE SKIP LOCKED）
    task, err := s.Queries.ClaimAgentTask(ctx, agentID)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil // 无任务
    }

    // 4. 刷新 agent 状态（从 active tasks 推导）
    s.ReconcileAgentStatus(ctx, agentID)

    // 5. 广播 dispatch 事件
    s.broadcastTaskDispatch(ctx, task)
    return &task, nil
}
```

### B. Daemon 主循环并发控制

```go
func (d *Daemon) pollLoop(ctx context.Context, taskWakeups <-chan struct{}) error {
    sem := make(chan struct{}, d.cfg.MaxConcurrentTasks) // default 20

    for {
        runtimeIDs := d.allRuntimeIDs()
        for i, rid := range runtimeIDs {
            select {
            case sem <- struct{}{}:   // acquire slot
            default:
                goto sleep             // at capacity
            }

            task, err := d.client.ClaimTask(ctx, rid)
            if task != nil {
                go func(t Task) {
                    defer func() { <-sem }()
                    d.handleTask(ctx, t)     // fork agent, wait for completion
                }(task)
                break
            }
            <-sem                       // release slot (no task)
        }

    sleep:
        sleepWithContextOrWakeup(ctx, pollInterval, taskWakeups)
    }
}
```

### C. WebSocket Hub 注册/广播

```go
// 客户端连接
case client := <-h.register:
    h.clients[client] = true
    h.subscribe(client, "workspace", client.workspaceID)  // auto-subscribe
    h.subscribe(client, "user", client.userID)

// 广播到 scope
func (h *Hub) BroadcastToScope(scopeType, scopeID string, message []byte) {
    clients := h.rooms[sk(scopeType, scopeID)]
    for client := range clients {
        select {
        case client.send <- message:    // non-blocking send (buffer 256)
        default:
            slow = append(slow, client) // channel full → evict
        }
    }
    h.evictSlow(slow)
}
```

### D. 自动重试逻辑

```go
func (s *TaskService) MaybeRetryFailedTask(ctx, parent AgentTaskQueue) (*AgentTaskQueue, error) {
    reason := parent.FailureReason.String

    // 仅基础设施故障可重试
    if !retryableReasons[reason] { return nil, nil }

    // 检查预算
    if parent.Attempt >= parent.MaxAttempts { return nil, nil }

    // 排除 autopilot
    if parent.AutopilotRunID.Valid { return nil, nil }

    // 创建子任务：继承一切，递增 attempt
    child := s.Queries.CreateRetryTask(ctx, parent.ID)
    s.notifyTaskAvailable(child)
    return &child, nil
}
```

### E. Workspace 中间件

```go
func RequireWorkspaceMember(queries *db.Queries) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. 从 slug/header/query 解析 workspace ID
            workspaceID := resolve(r)

            // 2. 读用户 ID（由 upstream Auth 中间件设置）
            userID := r.Header.Get("X-User-ID")

            // 3. DB 查询 membership
            member := queries.GetMemberByUserAndWorkspace(ctx, userUUID, wsUUID)

            // 4. 注入 context
            ctx = SetMemberContext(ctx, workspaceID, member)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

> 文档生成时间：2026-04-28
> 源仓库：https://github.com/multica-ai/multica (commit: main branch, v0.2.19 era)
> 研究方法：逐文件读取 raw.githubusercontent.com 上的源码，非文档推演
