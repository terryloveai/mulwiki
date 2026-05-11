# Mulwiki

将原始文档编译为持久化、Schema 驱动的 wiki 知识库。

[English](README.md)

Mulwiki 面向需要长期维护知识库的场景，而不是一次性的文档问答。它会读取原始文档，套用 Markdown Schema，调用配置好的本地 Agent，然后把结果写回成互联的 Markdown wiki 页面。

## 它解决什么问题

Mulwiki 围绕五个核心概念组织：

```text
Workspace
├── Sources       原始文档，对 Agent 只读
├── Schemas       定义 wiki 结构的 Markdown 文件
├── Runtimes      由 daemon 注册的本地 Agent CLI 环境
├── Agents        Runtime 绑定、指令、技能、环境和设置
└── Wiki          编译后的 Markdown 输出

Ingest Job = Sources + Schema + Agent -> Wiki
```

它和 RAG 聊天工具的主要区别是：输出不是临时答案，而是可以检查、编辑、搜索、版本化并持续增量更新的 wiki。

## 能力分层

核心业务能力应该按这个顺序设计：

```text
Server API = 权威能力层
CLI        = 自动化、脚本、daemon 和高级用户入口
Web UI     = 人机交互、可视化和低门槛入口
```

对于持久化的产品行为，Mulwiki 应该先通过 server API 暴露能力，再通过 CLI 让它可脚本化，最后由 Web UI 调用同一套 API 呈现出来。面向自动化的 CLI 命令应尽量支持 `--output json`。

## 当前能力

- 用户登录注册，以及 workspace 成员权限，支持 owner/admin/member 角色。
- 创建 workspace 时可以选择内置 Schema，也可以从空白 Schema 开始。
- 内置 Schema 和 Skill 随 server 发布，位于 `server/builtin/`。
- workspace 内容使用 Git 管理，包括 wiki 页面、sources、schemas 和任务输出。
- 本地 daemon 自动发现 Codex、Claude Code、Kimi 或自定义 Agent CLI，并向 server 注册 runtime。
- Agent 支持配置 runtime 绑定、instructions、skills、环境变量、自定义参数、模型设置和任务历史。
- 持久化 jobs、agent tasks、runtime 状态、session 指针、日志和 agent messages。
- CLI 已覆盖核心 workspace、schema、source、runtime、agent、skill、job 和 wiki 工作流。
- WebSocket 事件驱动的实时 UI 更新。

## 快速开始

### 前置要求

- Node.js 20+ 或当前 LTS
- pnpm 9.15+
- Go 1.23+
- Git
- 可选：本机已安装一个或多个 Agent CLI

### 启动应用

```bash
git clone https://github.com/terryloveai/mulwiki.git
cd mulwiki
pnpm install
```

启动后端：

```bash
cd server
go run ./cmd/server
```

另开一个终端启动前端：

```bash
cd apps/web
pnpm dev
```

打开 [http://localhost:3000](http://localhost:3000)，注册或登录，然后创建 workspace。创建时可以选择内置 Schema，也可以从空白 Schema 开始。

### CLI 和 Daemon

在 Web UI 中创建用户和 workspace 后，可以配置 CLI 并启动本地 runtime daemon：

```bash
cd server

go run ./cmd/mulwiki setup self-host \
  --server-url http://localhost:8080 \
  --workspace demo

go run ./cmd/mulwiki daemon start
go run ./cmd/mulwiki daemon status
go run ./cmd/mulwiki runtime list
```

如果只想单独登录 CLI，也可以执行：

```bash
go run ./cmd/mulwiki login --server-url http://localhost:8080 --workspace demo
```

CLI 会把本地 session 存到 `~/.mulwiki/config.json`。daemon 使用这个 session 签发 workspace 范围内的 daemon token，并把检测到的 runtimes 注册到 server。

## 常用命令

```bash
pnpm typecheck
pnpm build

cd server
go test ./...
```

## 仓库结构

```text
mulwiki/
├── apps/web/          Next.js 应用
├── packages/core/     共享类型、API client、hooks
├── packages/ui/       设计 token、组件、hooks
├── packages/views/    页面级 React views
├── server/
│   ├── cmd/server/    HTTP server
│   ├── cmd/mulwiki/   CLI 和 daemon 入口
│   ├── builtin/       内置 schemas 和 skills
│   ├── internal/      handlers、services、middleware、daemon、realtime
│   └── pkg/           database 和 protocol package
├── docs/              产品、架构和实施计划文档
└── scripts/           开发辅助脚本
```

## 配置

本地 server 和 daemon 的环境变量以 `.env.example` 为准。内置 Schema 和 Skill 从以下目录加载：

- `server/builtin/schemas/`
- `server/builtin/skills/`

新 workspace 会把 Schema 内容复制或 fork 到 workspace 自己的存储里。内置目录只作为随 server 发布的不可变模板。

## 更多文档

产品、架构、设计和实施计划类文档统一放在 [docs/](docs/README.md)。
