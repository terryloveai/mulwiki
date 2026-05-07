# Schema: concept-wiki

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://github.com/zhiyuzi/concept-wiki
> 作者：zhiyuzi
> 定位：用 H. Lynn Erickson 教育理论的形式本体（SoDK + SoPK）替换 Karpathy 的松散类型。Concept 节点是两条知识体系的唯一交汇点。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议（目录结构、manifest.json 格式）由 daemon 在运行时生成的 AGENTS.md 提供。Agent 执行时先读 AGENTS.md（怎么交付），再读本文件（写什么内容）。

---

## 1. Types（节点类型，9 种）

### SoDK（Structure of Declarative Knowledge · 陈述性知识）

| Layer | Type | Wiki 目录 | generate_file | Transferable | Description |
|-------|------|-----------|---------------|--------------|-------------|
| 1 | Fact | wiki/fact/ | true | No | 具体实例，锁定时点/地点/情境 |
| 2 | Topic | wiki/topic/ | true | No | 一组相关 Fact 的聚合框架 |
| 3 | Concept | wiki/concept/ | true | Yes | 从 Topic 抽象出的心理构造。1-2 词。跨时间成立。**SoDK 与 SoPK 的共享桥节点。** |
| 4 | Generalization | wiki/generalization/ | true | Yes | 两个+ Concept 的关系陈述。含限定词（often/can/may） |
| 5 | Principle | wiki/principle/ | true | Yes | 被视为学科基础"真理"的 Generalization。不用限定词 |
| 6 | Theory | wiki/theory/ | true | Yes | 解释现象的一组概念性假设，以最佳证据支撑 |

### SoPK（Structure of Procedural Knowledge · 程序性知识）

| Layer | Type | Wiki 目录 | generate_file | Transferable | Description |
|-------|------|-----------|---------------|--------------|-------------|
| 1 | Skill | wiki/skill/ | true | No | 嵌入 Strategy 的最小操作单元 |
| 2 | Strategy | wiki/strategy/ | true | No | 系统性计划。组合多个 Skill |
| 3 | Process | wiki/process/ | true | No | 持续行动。连续运行，干预才停止 |

**桥接规则**：Concept 是 SoDK 与 SoPK 的**唯一交汇点**。

**前置条件**：
- Generalization：需 ≥ 2 个支撑 Concept 页面
- Principle：需 ≥ 1 个支撑 Generalization 页面
- Theory：需 ≥ 3 个支撑 Concept 或 Generalization 页面

| Mulwiki layer 映射 |
|-------------------|
| Fact → `ingest` |
| Topic → `ingest` |
| Skill → `analyze` |
| Strategy → `analyze` |
| Process → `analyze` |
| Concept → `concept` |
| Generalization → `compile` |
| Principle → `compile` |
| Theory → `publish` |

---

## 2. Structure（层级关系）

**Pattern: Strict Hierarchy**

### 严格相邻规则
- 节点只能与相邻层级的节点建立上下关系，**禁止跨层跳跃**
- 关系方向向上（低层 → 高层）

### 可迁移性分界线
- 不可迁移：Fact / Topic / Skill / Strategy / Process
- 可迁移：Concept / Generalization / Principle / Theory

### 方向性
所有连接有向，方向向上。无强制回链要求。

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`Fact | Topic | Concept | Generalization | Principle | Theory | Skill | Strategy | Process`
- `layer`（string）：见上表 Mulwiki layer 映射

### 推荐字段
- `sources`（list）：原始文档引用
- `created`（date，YYYY-MM-DD）
- `updated`（date，YYYY-MM-DD）

### 示例 — Concept 页面

```yaml
---
type: Concept
layer: concept
sources:
  - sources/attention-is-all-you-need.pdf
  - sources/bert-2018.pdf
created: 2026-04-20
updated: 2026-04-25
---
```

---

## 4. Ingest Pipeline

### Step 1 — 读取上下文
1. 读取 schema.md（本文件）和 AGENTS.md（平台协议）
2. 读取 `wiki/index.md`（如有）
3. 读取 `sources/` 中的原始资料

### Step 2 — 分层提取
1. 读取原始资料
2. 提取 Fact 和 Topic（SoDK 侧）
3. 识别可抽象的 Concept
4. 判断是否能形成 Generalization / Principle / Theory
5. 识别 Process / Strategy / Skill（SoPK 侧）
6. 将 SoDK 与 SoPK 通过共享 Concept 连接
7. 更新 `wiki/index.md`
8. 追加 `wiki/log.md`
9. 写 `output/manifest.json`（格式见 AGENTS.md）

一个资料可能触达 10-15 个 wiki 页面。

### 增量行为
先读所有现有页面获取上下文，仅更新受影响的页面。

---

## 5. Lint Rules（健康检查）

在每次 Ingest 之后或按需运行。Lint 只读，不修改 wiki 文件。输出写入 `output/lint-YYYY-MM-DD.md`。

### L1 — 孤儿检查
检查没有入链或出链的页面。

### L2 — 证据链检查
- 每个 Generalization 是否有 ≥ 2 个 Concept 支撑
- 每个 Theory 是否有 ≥ 3 个 Concept 或 Generalization 支撑

### L3 — 知行合一检查
检查每个 Concept 是否同时有 SoDK 来源（至少一个 Topic 入链）和 SoPK 来源（至少一个 Skill/Strategy/Process 入链）。

### L4 — 归属检查
- Skill 是否挂在 Strategy 下
- Strategy 是否挂在 Process 下

### L5 — 过时检查
检查 Fact 是否被新资料推翻。

### L6 — 可迁移性错误提升检查
检查不可迁移类型是否被错误提升至 Generalization 及以上层级。

---

## 6. Wiki 结构

```
wiki/
├── index.md         # 按节点类型分类的目录
├── log.md           # 只追加的时间记录
├── fact/            # Fact 节点
├── topic/           # Topic 节点
├── concept/         # Concept 节点（枢纽，最值得维护）
├── generalization/  # Generalization 节点
├── principle/       # Principle 节点
├── theory/          # Theory 节点
├── skill/           # Skill 节点
├── strategy/        # Strategy 节点
└── process/         # Process 节点
```

---

## 与其他 schema 的核心差异

| 维度 | concept-wiki |
|------|-------------|
| 类型系统 | 严格 9 层本体（SoDK + SoPK） |
| 结构规则 | 严格邻层互连，禁止跨层跳跃 |
| 枢纽机制 | Concept 是唯一桥节点 |
| 可迁移性分界 | Fact/Topic/Skill/Strategy/Process 不可迁移 |
| Lint 规则 | 语义级（证据链、知行合一、归属检查） |
| 适用场景 | 需要严谨知识分类的深度研究 |
