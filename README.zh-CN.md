# Mulwiki

将原始文档编译为结构化 wiki 知识库 — 多租户、Schema 驱动、Agent 驱动。

[🇺🇸 English](README.md)

## 🎯 概述

Mulwiki 是一个将任意文档转换为结构化、互联的 wiki 知识库的平台。与提供临时答案的传统 RAG 系统不同，Mulwiki 将文档编译为持久化、可搜索且持续增长的知识库。

### 核心差异化

| 维度 | NotebookLM / RAG 工具 | Mulwiki |
|-----------|----------------------|---------|
| **核心交互** | 上传文档 → 问答 | 上传文档 → 编译为 wiki |
| **输出物** | 临时答案（用完即弃） | 持久化 wiki 知识库（持续增长） |
| **知识组织** | 无结构（向量检索） | 用户可选/可定义的 Schema |
| **Agent 选择** | 平台固定 | 用户自选（Claude Code / Codex / Kimi CLI / 自定义） |
| **增量更新** | 每次全量检索 | 增量编译，只更新受影响的页面 |

### 目标用户

需要在特定领域维护长期知识库的研究人员、工程师和深度学习者。用户应该理解"Schema 定义知识结构"的概念，并愿意为其领域选择或定制 Schema。

## 🏗️ 核心概念

Mulwiki 围绕五个核心实体组织知识：

```
Workspace
├── Sources（原始文档，只读）
├── Schema（知识组织方式定义）
├── Agent Runtime（Agent 执行环境）
├── Agent（配置好的 Agent：包含指令、技能、环境）
└── Wiki（编译产出物）

Ingest Job: Sources + Schema → Agent → Wiki
```

### Workspace
一个隔离的知识库空间，包含 Sources、Schemas、Agents 和 Wiki 页面。

### Source
用户上传的原始文档。对 Agent 来说是只读的。支持 PDF、DOCX、PPTX、XLSX、Markdown、纯文本、图片和 URL。

### Schema
一个定义知识组织方式的 Markdown 文件，包含五个维度：
- **Types（类型）**：wiki 中存在哪些种类的页面
- **Structure（结构）**：页面如何连接（严格层次结构 / 知识图谱 / 自由链接）
- **Frontmatter（元数据）**：每种页面类型所需的 YAML 元数据
- **Ingest Pipeline（摄入流水线）**：文档 → wiki 的分步工作流
- **Lint Rules（检查规则）**：质量检查（孤立页面、矛盾、证据链）

内置 6 个 Schema。用户可以创建自定义 Schema 或基于内置 Schema 进行分支。

### Agent Runtime
Agent 的执行环境。代表物理机上已安装的 agent CLI 守护进程。每个 Runtime 跟踪：后端类型（claude-code / codex / kimi / custom）、CLI 路径、主机名、操作系统、版本、daemon_id、last_heartbeat（存活状态）和在线/离线状态。

### Agent
绑定到 Runtime 的配置好的 Agent，包含六个配置维度：
- **Runtime 绑定**：此 Agent 在哪个 Runtime 上执行
- **Instructions（指令）**：定义 Agent 行为的系统级提示
- **Skills（技能）**：可复用的能力模块
- **Tasks（任务）**：执行历史（状态、持续时间、输出统计、错误）
- **Environment（环境）**：环境变量（API 密钥、工作目录路径、模型名称）
- **Custom Args（自定义参数）**：传递给 runtime 的额外 CLI 参数
- **Settings（设置）**：模型选择、最大并发任务数、可见性（私有/公开）

Agent 和 Schema 完全解耦。摄入任务组合：Sources + Schema → Agent → Wiki。

### Wiki
编译输出 — 一个按 Schema 类型系统组织的互联 Markdown 页面目录。可导出为 zip（Obsidian 兼容）。

## 🛠️ 技术栈

- **前端**：Next.js 15、React 19、Tailwind CSS（oklch）、React Query
- **后端**：Go、chi 路由器、SQLite（go-sqlite3）
- **Agent**：守护进程模式 — 轮询服务器获取任务，将配置的 agent CLI 作为子进程派生
- **Monorepo**：pnpm workspaces + turborepo

## 🚀 快速开始

### 前置要求

- Node.js 18+
- Go 1.21+
- pnpm 9+

### 安装

```bash
# 克隆仓库
git clone https://github.com/yourusername/mulwiki.git
cd mulwiki

# 安装依赖
pnpm install
```

### 开发

```bash
# 同时启动 Go 后端和 Next.js 前端
make dev

# 或者分别启动：
# 终端 1: Go 后端 (http://localhost:8080)
cd server && go run ./cmd/server

# 终端 2: 前端 (http://localhost:3000)
cd apps/web && pnpm dev
```

### 构建

```bash
make build
```

### 清理

```bash
make clean
```

## 📁 项目结构

```
mulwiki/
├── apps/web/          Next.js 15 前端（App Router）
├── packages/
│   ├── core/          共享类型、API 客户端、hooks
│   ├── ui/            设计系统：tokens、组件、hooks
│   └── views/         页面级组件
├── server/
│   ├── cmd/server/    HTTP 服务器（chi 路由器）
│   ├── cmd/mulwiki/   CLI（daemon 子命令）
│   ├── internal/
│   │   ├── handler/   HTTP 处理器
│   │   ├── daemon/    守护进程循环 + agent 任务执行
│   │   ├── service/   业务逻辑
│   │   ├── middleware/ Auth → Workspace → Role 链
│   │   ├── events/    进程内事件总线
│   │   └── realtime/  WebSocket 集线器（基于房间的发布/订阅）
│   └── pkg/
│       ├── db/         SQL 模式
│       └── protocol/   共享类型
├── schemas/           内置 Schema 定义（Markdown）
├── scripts/           开发辅助工具
├── LICENSE            MIT 许可证
├── Makefile           构建自动化
└── README.md          本文件
```

## ✨ 功能特性

### 多租户工作空间
- 具有完整数据隔离的隔离知识库
- 工作空间级别的设置和配置
- 成员管理（所有者/管理员/成员角色）

### 文档管理
- 支持多种文件格式（PDF、DOCX、PPTX、XLSX、Markdown、纯文本、图片）
- URL 摄入，自动内容提取
- 文档预览和管理

### Schema 系统
- 6 个内置 Schema，适用于不同的知识组织模式
- 使用 Markdown 编辑器创建自定义 Schema
- 从内置模板分支 Schema
- 纯 Markdown 定义（无 agent 引用，无流水线路由配置）

### Agent Runtime
- 支持多种 agent 后端（Claude Code、Codex、Kimi CLI、自定义）
- Runtime 注册和健康监控
- 自动心跳跟踪（30 秒间隔）
- 停滞检测（5 分钟超时）

### Agent 配置
- 六维配置（Runtime、Instructions、Skills、Tasks、Environment、Settings）
- 带详细日志的任务执行历史
- 敏感数据的环境变量管理
- 模型选择和并发控制

### 摄入流水线
- 使用 Schema + Agent + Sources 选择创建任务
- 通过 WebSocket 实时任务监控
- 现有 wiki 的增量编译
- 带失败分类的自动重试

### Wiki 输出
- 按 Schema 类型组织的结构化 Markdown 页面
- 跨 wiki 内容的全文搜索
- 追溯回原始文档的源追踪
- Obsidian 兼容导出（zip 格式）

### 实时更新
- 基于 WebSocket 的实时任务状态
- 内部服务通信的事件总线
- 基于房间的发布/订阅模式

## 🔧 配置

### 环境变量

请参阅 `.env.example` 获取所需的环境变量：

```bash
# 服务器
SERVER_PORT=8080
DATABASE_URL=sqlite:///data/mulwiki.db

# 前端
NEXT_PUBLIC_API_URL=http://localhost:8080
```

### 内置 Schema

位于 `server/data/builtin-schemas/`：
- `concept-wiki-schema.md` - 严格的 9 种类型层次结构
- `karpathy-llm-wiki-schema.md` - 松散的自由链接结构
- `nashsu-llm-wiki-schema.md` - 7 种类型知识图谱
- `llm-knowledge-base-schema.md` - 最小 3 种类型系统
- `paper-spec-wiki-schema.md` - 学术 7 种类型结构
- `paper-spec-paper-schema.md` - 论文级 YAML 结构化剖面

## 📖 API 路由

| 方法 | 路径 | 用途 |
|--------|------|---------|
| `POST` | `/api/daemon/register` | 守护进程注册自身及其 runtimes |
| `POST` | `/api/daemon/heartbeat` | 定期存活心跳 |
| `GET` | `/api/daemon/stale` | 标记停滞的 runtimes 为离线 |
| `POST` | `/api/workspaces/{slug}/agents/{id}/tasks/claim` | 原子性地声明可用任务 |

## 🎨 前端路由

| 路径 | 用途 |
|------|---------|
| `/` | 落地页 / 重定向 |
| `/login` / `/register` | 认证 |
| `/workspaces` | 工作空间列表 + 创建 |
| `/[slug]/wiki` | Wiki 索引 |
| `/[slug]/wiki/[...path]` | Wiki 页面详情 |
| `/[slug]/sources` | 源文档上传 + 列表 |
| `/[slug]/schemas` | Schema 列表 + 创建/编辑 |
| `/[slug]/agents` | Agent 列表（Runtimes + Agents + Skills + Tasks） |
| `/[slug]/jobs` | 任务列表 + 创建 + 日志 |
| `/[slug]/settings` | 工作空间设置 |
| `/api/*` → Go `:8080` | 后端代理（Next.js 重写） |
| `GET /ws` | WebSocket（实时任务状态、agent 状态） |

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。

1. Fork 本仓库
2. 创建您的功能分支（`git checkout -b feature/AmazingFeature`）
3. 提交更改（`git commit -m 'Add some AmazingFeature'`）
4. 推送到分支（`git push origin feature/AmazingFeature`）
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 详情请参阅 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- 使用 [Next.js](https://nextjs.org/) 构建
- 后端由 [Go](https://golang.org/) 驱动
- 使用 [Tailwind CSS](https://tailwindcss.com/) 进行样式设计
- 使用 [Turborepo](https://turbo.build/) 管理 Monorepo

## 📞 支持

如需支持，请在 GitHub 上打开 issue 或联系维护者。

---

**注意**：Mulwiki 目前处于活跃开发阶段。随着平台的发展，API 和功能可能会发生变化。