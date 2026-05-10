# Mulwiki — 产品需求文档

> Version: v0.3 | Last Updated: 2026-04-28

---

## 1. 产品概述

### 1.1 一句话定义

Mulwiki 是一个将任意文档编译为结构化 wiki 知识库的多租户平台。

### 1.2 产品定位

用户上传文档或 URL，选择一种知识组织方式（Schema），指定一个 AI 智能体（Agent），平台把文档**编译**为一套互相链接、可浏览、可搜索的 Markdown 知识库。

Mulwiki 不是 RAG 问答系统。RAG 每次提问都回头翻原始文档捞片段；Mulwiki 是一次性编译——交叉引用写好，矛盾标记好，知识随每次新资料摄入持续增长。Wiki 是持久产物，不是临时拼装的检索结果。

### 1.3 核心差异化

| 维度 | NotebookLM / RAG 工具 | Mulwiki |
|------|----------------------|------------|
| 核心交互 | 上传文档 → 问答 | 上传文档 → 编译为 wiki |
| 输出物 | 临时回答（用完即弃） | 持久化 wiki 知识库（持续增长） |
| 知识组织 | 无结构（向量检索） | 用户可选/可定义的 Schema |
| 智能体选择 | 平台固定 | 用户自选（Claude Code / Codex / Kimi CLI / 自定义） |
| 增量更新 | 每次全量检索 | 增量编译，只更新受影响的页面 |

### 1.4 目标用户

需要长期积累某领域知识的人——研究者、技术人员、深度学习者。

**不是**给所有人用的通用知识管理工具。用户需要理解"Schema 定义知识结构"这个概念，愿意为自己的领域选择或定制 Schema。

### 1.5 核心价值主张

维护知识库的记账工作（更新交叉引用、保持摘要最新、标记矛盾、检查健康度）太费人。AI 智能体不会烦，一次能改几十个文件。Mulwiki 让用户只做两件事：**策划资料** 和 **选择知识组织方式**。剩下的交给智能体。

---

## 2. 核心概念

### 2.1 五个核心实体及关系

```
Workspace
├── Sources（原始文档，只读）
├── Schema（知识组织方式的定义）
├── Agent Runtime（智能体的执行环境：本地 CLI、云端环境等）
├── Agent（配置好的智能体：绑定 Runtime，持有指令、技能、环境变量）
└── Wiki（编译产出物）

Ingest Job：Sources + Schema → Agent → Wiki
```

- **Workspace**：一个独立的知识库空间。文档、Schema、Agent、Wiki 都在其中隔离。
- **Sources**：用户上传的原始资料。是 wiki 的唯一事实来源。Agent 只读不改。
- **Schema**：定义"知识怎么组织"——wiki 有哪些页面类型、类型之间怎么关联、摄入流程分几步、健康检查什么规则。Schema 是一个 Markdown 文件，保持纯粹：它只描述知识的结构和加工方式，不涉及用什么工具来执行。
- **Agent Runtime**：智能体的执行环境。可以是本机安装的 Claude Code CLI、Codex CLI、Kimi CLI，也可以是自定义的命令行程序。一个 Workspace 可以有多个 Runtime（比如本机同时装了 Claude Code 和 Codex）。
- **Agent**：用户创建的具体智能体。每个 Agent 绑定一个 Runtime，并带有完整的运行配置：系统指令（Instructions）、可复用技能（Skills）、环境变量（Environment）、CLI 参数（Custom Args）、模型和并发等设置。Agent 是"谁来做"，Schema 是"怎么做"——两者解耦，用户自由组合。
- **Ingest Job**：核心动作。一批 Sources + 一个 Schema → 交给用户选定的 Agent → 生成或更新 Wiki。

### 2.2 Schema 详解

Schema 是 Mulwiki 区别于其他知识管理工具的核心。它是一个 Markdown 文件，定义五个维度：

#### 2.2.1 节点类型（Types）

Wiki 里有哪些种类的页面。不同 Schema 的类型完全不同：

| Schema | 页面类型 |
|--------|---------|
| Karpathy 原始 | Summary, Entity, Concept, Comparison, Synthesis, Overview（松散，不强制） |
| concept-wiki | Fact, Topic, Concept, Generalization, Principle, Theory, Skill, Strategy, Process（严格 9 种） |
| nashsu | Entity, Concept, Source Summary, Query, Synthesis, Comparison, Overview（7 种） |
| llm-knowledge-base | concept, summary, topic（极简 3 种） |
| paper-spec wiki | concept, author, method, dataset, contradiction, timeline, claim（学术 7 种） |
| paper-spec paper | 非 wiki 页面——论文级 YAML 结构化剖面 |

每个 Type 声明：
- 类型名称和描述
- 是否生成独立文件
- 前置条件（如 Generalization 需要至少 2 个 Concept 支撑）

#### 2.2.2 结构规则（Structure）

节点之间怎么连接。三种模式：
- **严格分层**：只能邻层互连，方向向上（concept-wiki）
- **知识图谱**：多信号加权相关性（nashsu 的 4 信号模型）
- **自由链接**：双向链接，无约束（Karpathy 原始）

#### 2.2.3 元数据规范（Frontmatter）

每个页面文件必须带什么 YAML frontmatter 字段。典型字段：
- `type`：页面类型（通常必填）
- `sources`：原始文档引用列表
- `confidence`：信息充分度（llm-knowledge-base 独有）
- `related`：关联页面列表

#### 2.2.4 摄入工作流（Ingest Pipeline）

从原始文档到 wiki 页面的完整步骤，由 Schema 定义。不同 Schema 的步骤差异很大：
- Karpathy 原始：LLM 自由发挥，无固定步骤
- nashsu：两步链式推理（先分析结构和矛盾 → 再生成页面）
- concept-wiki：9 步顺序处理（提取事实 → 识别概念 → 桥接陈述性与程序性知识……）
- llm-knowledge-base：6 步编译（摘要 → 概念识别 → 更新索引……）

Agent 执行时将 Schema 全文作为系统指令，按其中定义的流程操作。

#### 2.2.5 健康检查（Lint Rules）

Schema 自带的质量约束。Agent 在摄入完成后或按需执行：
- 孤儿检查（无入链/出链的页面）
- 矛盾检查（跨源冲突声明）
- 证据链检查（上层节点是否有足够下层支撑）
- 过时检查（事实是否被新资料推翻）
- 置信度审计（来源稀疏 → 降级）

### 2.3 Agent 体系详解

#### 2.3.1 双层模型：Runtime + Agent

Mulwiki 将智能体拆为两层，对应"你用什么跑"和"你怎么配置它"：

**Agent Runtime（执行环境层）**：描述一个可用的智能体运行环境。比如本机安装了 Claude Code CLI，其可执行文件位于 `/usr/local/bin/claude`。一个 Workspace 可以注册多个 Runtime。

**Agent（配置层）**：在一个 Runtime 基础上，用户定义具体的行为配置。同一个 Runtime（比如同一台机器上的 Claude Code）可以配置出多个 Agent——一个专门做深度分析（长指令、大模型），一个做快速摘要（短指令、便宜模型）。

#### 2.3.2 Agent 的六个配置维度

每个 Agent 有六组配置，借鉴成熟的智能体管理平台设计：

| 配置区 | 说明 | 典型内容 |
|--------|------|---------|
| Runtime 绑定 | Agent 依赖哪个执行环境 | 选择已注册的 Runtime，查看其在线状态 |
| Instructions | 系统级指令，定义 Agent 的行为和角色 | "你是一个知识编译专家。严格按照 Schema 定义处理文档……" |
| Skills | 可复用的能力模块 | 文档解析、概念提取、交叉引用、格式化输出等 |
| Tasks | 执行历史记录 | 每次摄入任务的执行状态、耗时、产出统计、错误信息 |
| Environment | 环境变量 | API 密钥、工作目录路径、模型名称等敏感配置 |
| Custom Args | 命令行额外参数 | 传给 Agent CLI 的附加标志 |
| Settings | 运行参数 | 模型选择、最大并发任务数、可见性（私有/公开） |

#### 2.3.3 Agent 和 Schema 的关系

Agent 和 Schema 完全解耦。创建一次摄入任务时，用户选择：处理哪些文档 + 用哪个 Schema + 派给哪个 Agent。同一个 Schema 可以先后交给不同 Agent 执行，比较编译质量。

Schema 定义"需要做什么"（工作流步骤、页面类型、质量规则），Agent 负责"怎么做"（读取 Schema 作为指令、按规则执行、产出 wiki 页面）。

Schema 文件保持纯粹：它是一个知识组织规范，不含任何工具或执行环境的引用。

#### 2.3.4 Agent 的执行方式

Agent 不是直接调用大模型 API。Mulwiki 将 Agent 的 Runtime CLI 作为子进程启动，传入完整配置：

1. 构造工作目录，将 Schema 全文写入 `AGENTS.md`
2. 注入 Agent 的 Instructions、Environment（环境变量）、Custom Args
3. 启动 Runtime CLI 子进程，指向工作目录
4. 监控子进程输出，记录执行日志
5. 子进程完成后，回收输出文件，写入 Wiki 存储

这样用户的 API 密钥在自己的 Agent 配置中管理，平台不碰凭证。Claude Code、Codex 等本身已是成熟的智能体运行时，Mulwiki 只需管好子进程的启停和输出回收。

#### 2.3.5 Agent 的任务可见性

每次 Agent 执行摄入任务，都会在 Agent 的 Tasks 面板中生成一条执行记录。用户可以看到：
- 任务状态（排队中 → 已分配 → 执行中 → 完成/失败）
- 执行耗时和尝试次数
- 处理了哪些文档、生成了多少 wiki 页面
- 错误信息（如果失败）

这使用户可以追溯每个 Agent 的活动历史，评估不同 Agent 或不同配置的效果。

---

## 3. 用户流程

### 3.1 场景 A：从零开始

**前置条件**：用户已注册登录。

1. 创建 Workspace，命名为 "transformer-research"
2. 注册一个 Runtime：选择 Claude Code，填写 CLI 路径（或自动检测）
3. 基于该 Runtime 创建一个 Agent：填写名称、编写 Instructions（或使用模板）、配置 Environment（API 密钥等）
4. 浏览内置 Schema 列表（6 种），选择 "concept-wiki"
5. 上传文档：拖入 3 篇 PDF + 粘贴 2 个 URL
6. 创建一次摄入任务：选 Schema + 选 Agent + 勾选要处理的文档 → 开始
7. 在 Agent 的 Tasks 面板中查看实时执行日志
8. 任务完成后，进入 Wiki 浏览页，看到按类型分类的页面列表
9. 点进一个 Concept 页面，查看元数据 + 正文 + 交叉引用

### 3.2 场景 B：增量摄入

**前置条件**：Workspace 已有 Wiki 和 Agent。

1. 回到 Workspace，看到上次任务的状态和 Wiki 统计
2. 上传一篇新论文
3. 创建新任务：选同一个 Schema + 同一个 Agent，只勾选这篇新论文
4. Agent 读取已有 Wiki 后增量更新——不全量重建，只更新相关页面
5. 任务完成，Wiki 页面列表刷新

### 3.3 场景 C：浏览和使用

**前置条件**：Wiki 已生成。

1. 目录树浏览：按 Schema 定义的页面类型分类展示
2. 页面阅读：查看元数据 + Markdown 正文
3. 溯源追踪：从 Wiki 页面跳回引用的原始文档
4. 全文搜索：输入关键词，返回匹配页面
5. Obsidian 兼容：整个 Wiki 目录可直接用 Obsidian 打开
6. 导出：整个 Wiki 目录下载为 zip

### 3.4 场景 D：自定义 Schema

1. 进入 Schema 管理页，新建 Schema
2. 在 Markdown 编辑器中定义节点类型、结构规则、元数据规范、摄入步骤、健康检查规则
3. 也可以从内置 Schema 复制一份，在此基础上修改
4. 保存后可立即在摄入任务中选用

### 3.5 场景 E：比较不同 Agent

1. 在同一个 Workspace 中创建两个 Agent（如 Claude Code Agent 和 Codex Agent）
2. 用同一个 Schema 和同一批文档，分别创建两次任务，各派给不同 Agent
3. 在各自的 Tasks 面板中对比执行时间和产出质量
4. 浏览两个 Agent 生成的 Wiki，比较差异

---

## 4. 功能模块

### 4.1 用户认证

| 功能 | 说明 |
|------|------|
| 注册 | 邮箱 + 密码 |
| 登录 | 邮箱 + 密码 |
| 登出 | 清除会话 |

MVP 不做 OAuth、多因子、邀请制。

### 4.2 Workspace 管理

| 功能 | 说明 |
|------|------|
| 创建 | 名称（必填）、描述（可选）。自动生成唯一标识 |
| 列表 | 当前用户所有 Workspace，显示名称、文档数、Wiki 页面数、最后更新时间 |
| 详情 | 进入 Workspace 后看到 Sources / Schema / Agent / Wiki / Jobs 五个子模块 |
| 设置 | 修改名称、描述、外观偏好（主题、字体大小） |
| 删除 | 级联删除该 Workspace 下所有数据，需二次确认 |

### 4.3 文档管理（Sources）

| 功能 | 说明 |
|------|------|
| 上传文件 | 支持 PDF、DOCX、PPTX、XLSX、Markdown、纯文本、图片 |
| 添加 URL | 输入网址，自动抓取网页内容转为 Markdown 存储 |
| 列表 | 文件名、类型、大小、上传时间、状态 |
| 预览 | 点击文档查看内容 |
| 删除 | 删除文档。已被 Wiki 引用的需级联清理 |

### 4.4 Schema 管理

| 功能 | 说明 |
|------|------|
| 浏览内置 | 6 个内置 Schema，显示名称、描述、类型数、核心特性 |
| 复制内置 | 从内置 Schema 复制一份到当前 Workspace，可编辑 |
| 新建自定义 | Markdown 编辑器，从空白开始写 Schema |
| 编辑 | 修改自己的 Schema（内置的不可编辑） |
| 删除 | 删除自定义 Schema。不影响已生成的 Wiki |

**内置 Schema（6 个，只读）**：Karpathy 原始、concept-wiki、nashsu、llm-knowledge-base、paper-spec wiki、paper-spec paper。

### 4.5 Agent Runtime 管理

| 功能 | 说明 |
|------|------|
| 注册 Runtime | 名称、提供方类型（Claude Code / Codex / Kimi CLI / 自定义）、CLI 路径、默认模型 |
| 列表 | 已注册的 Runtime，显示名称、提供方、状态 |
| 状态 | 在线检测：CLI 是否可达、是否能正常响应 |
| 删除 | 删除 Runtime。有 Agent 绑定的 Runtime 不可删除 |

### 4.6 Agent 管理

| 功能 | 说明 |
|------|------|
| 创建 Agent | 绑定 Runtime → 填写名称和描述 → 编写 Instructions → 配置 Environment / Custom Args / Settings |
| 列表 | 所有已创建的 Agent，显示名称、绑定的 Runtime、在线状态、最近任务数 |
| 六个配置区 | Instructions（系统指令）、Skills（可复用能力）、Tasks（执行历史）、Environment（环境变量）、Custom Args（CLI 参数）、Settings（模型、并发数、可见性） |
| 归档/恢复 | 不再使用的 Agent 可以归档（保留历史记录），也可以随时恢复 |
| 测试 | 发送简单指令验证 Agent 配置是否正常工作 |

**业务规则**：
- Agent 和 Schema 完全解耦——创建摄入任务时自由组合
- Environment 中的敏感信息（如 API 密钥）对非 Agent 创建者不可见
- 一个 Agent 同一时间执行的任务数受其 Settings 中配置的并发上限约束

### 4.7 Agent 任务历史

| 功能 | 说明 |
|------|------|
| 任务列表 | 该 Agent 执行过的所有摄入任务，按时间倒序 |
| 任务详情 | 状态、开始/完成时间、处理文档数、生成/更新页面数、错误信息（如有） |
| 实时监控 | 正在执行的任务显示实时日志 |

### 4.8 知识编译（Ingest Job）

| 功能 | 说明 |
|------|------|
| 创建任务 | 选 Schema + 选 Agent + 勾选文档 → 开始 |
| 任务列表 | 历史任务的状态、时间、产出统计 |
| 实时日志 | 执行中可查看 Agent 的输出流 |
| 取消和重试 | 取消进行中的任务；重试失败的任务 |

### 4.9 Wiki 浏览

| 功能 | 说明 |
|------|------|
| 目录树 | 按 Schema 定义的页面类型分类展示 |
| 页面阅读 | 渲染 Markdown + 展示元数据 |
| 溯源 | 从 Wiki 页面跳回原始文档 |
| 全文搜索 | 关键词搜索 |
| 导出 | 下载整个 Wiki 为 zip（Obsidian 兼容） |

### 4.10 健康检查（Lint）

| 功能 | 说明 |
|------|------|
| 手动触发 | 在 Wiki 页点击"运行健康检查" |
| 自动触发 | 每次摄入任务完成后自动执行 |
| 报告查看 | 列出所有问题（规则由 Schema 定义） |

---

## 5. 关键设计决策

### 5.1 为什么是平台而不是单机脚本

单机脚本零门槛但缺乏 Workspace 隔离、任务管理、历史记录、Schema 库浏览比较、自动环境构造。平台将这些自动化。

### 5.2 为什么用 CLI 子进程而不是直接调用 API

- 用户自己管理 API 密钥——平台不碰凭证
- Claude Code / Codex / Kimi CLI 是成熟的智能体运行时，有会话管理、工具调用、文件操作能力
- Mulwiki 只负责：构造工作目录 → 启动子进程 → 监控状态 → 回收输出
- 未来可扩展为远程执行环境，但 MVP 只做本地 CLI

### 5.3 为什么 Agent 和 Schema 彻底解耦

同一个知识组织方式（Schema）可能今天用 Claude Code 跑，明天想试 Codex 的效果。Agent 是"谁来做"，Schema 是"怎么做"——分离后用户可以自由组合、比较、优化。Schema 文件本身保持纯粹，不嵌入任何工具或执行环境的引用。

### 5.4 为什么分层设计 Runtime 和 Agent

同一台机器上的同一个 CLI（Runtime）可以配置出多个 Agent：一个做深度分析（长指令、大模型），一个做快速摘要（短指令、便宜模型）。Runtime 描述能力池，Agent 描述具体配置——分层避免重复配置。

### 5.5 生成粒度由 Schema 决定

不在平台层硬编码"生成几种类型的页面"。Schema 的每个 Type 声明是否生成独立文件，Agent 按此执行。

---

## 6. 优先级

### P0 — MVP 必须

- [ ] 用户注册 / 登录 / 登出
- [ ] Workspace 创建、列表、设置、删除
- [ ] Sources 上传 + 列表 + 删除
- [ ] 6 个内置 Schema 浏览
- [ ] 自定义 Schema 创建、编辑、从内置复制
- [ ] Agent Runtime 注册、列表、状态检测
- [ ] Agent 创建（绑定 Runtime + Instructions + Environment + Custom Args + Settings）
- [ ] Agent 列表、编辑、归档/恢复
- [ ] Agent Tasks 列表和详情
- [ ] 摄入任务创建 → 执行 → 状态流 → 实时日志 → 完成/失败
- [ ] Agent CLI 子进程管理（启动、监控、超时、崩溃恢复）
- [ ] 增量摄入（已有 Wiki 的 Workspace 追加文档）
- [ ] Wiki 浏览（目录树 + 页面阅读 + 全文搜索）
- [ ] Wiki 导出 zip

### P1 — 首版后快速跟进

- [ ] 任务取消和重试
- [ ] Skills 创建和管理
- [ ] Lint 手动触发 + 报告查看
- [ ] Lint 自动触发（任务完成后）
- [ ] Wiki 统计面板

### P2 — 后续版本

- [ ] 知识图谱可视化
- [ ] 向量语义搜索
- [ ] 远程执行环境
- [ ] 多成员 Workspace + 权限管理
- [ ] Schema 分享市场

---

## 7. 非功能性需求

| 维度 | 要求 |
|------|------|
| 部署形态 | Web 应用，可单机部署 |
| 多租户隔离 | 所有数据查询强制 Workspace 隔离 |
| Agent 安全 | Agent 只能访问其任务工作目录 |
| 文件存储 | 文件系统（MVP）或对象存储（生产） |
| 实时性 | 任务进度通过 EventBus → WebSocket Hub 实时推送（`GET /ws`） |
| 超时 | Agent 默认 30 分钟，可在 Settings 中配置 |

---

## 附录 A：核心数据模型（产品视角）

### Workspace
- 名称、描述、创建时间
- 外观设置（主题、字体大小）
- 包含：文档、Schema、Agent、Wiki

### Source
- 文件名、类型（文件/URL）、大小、SHA256 指纹
- 状态：待处理 → 已摄入 → 错误
- 关联到哪个 Wiki 页面引用了它

### Schema
- 名称、描述、来源（内置/自定义/复制自哪个内置）
- 内容：完整的 Markdown 定义
- 定义：页面类型、连接规则、元数据规范、摄入工作流、健康检查规则

### Agent Runtime
- 名称、Backend 类型（claude-code / codex / kimi / custom）
- CLI 路径、主机名、操作系统、版本
- 管理它的 Daemon ID、最后心跳时间
- 状态：在线/离线/异常

### Agent
- 名称、描述、绑定哪个 Runtime
- Instructions：系统级行为指令
- Skills：关联的可复用能力
- Tasks：执行历史记录
- Environment：环境变量（含 API 密钥等敏感信息）
- Custom Args：传给 CLI 的额外参数
- Settings：模型选择、最大并发任务数、可见性

### Agent Task
- 关联哪个 Agent、哪个 Runtime
- 状态：排队中 → 已分配 → 执行中 → 完成/失败/已取消
- 执行时间、尝试次数（attempt/max_attempts）
- 父任务 ID（parent_task_id）—— 记录自动重试链
- 失败原因分类（failure_reason）：timeout / runtime_offline / agent_error 等
- 会话恢复指针（session_id / work_dir）—— 支持断点续传
- 管理它的 Daemon ID
- 产出统计、错误信息

### Job（摄入任务）
- 处理哪些文档、用哪个 Schema、派给哪个 Agent
- 状态流：排队中 → 已分配 → 执行中 → 完成/失败/已取消
- 生成/更新了多少 wiki 页面

### Wiki Page
- 页面类型（由 Schema 定义）
- 路径、标题、正文（Markdown）
- 元数据（来源文档、关联页面、置信度等）
- 由哪个任务创建/最后更新

### Daemon
- Daemon ID、主机名、PID、版本号
- 管理的 Runtime 列表
- 最大并发任务数
- 最后心跳时间、注册时间

---

*此 PRD 供开发使用。内置 Schema 参考文件见 `schemas/` 目录。*
