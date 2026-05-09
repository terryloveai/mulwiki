# Schema: llm-knowledge-base

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://github.com/arturseo-geo/llm-knowledge-base
> 作者：Artur Ferreira / The GEO Lab
> 定位：Karpathy 模式的极简 schema 标准。3 种类型覆盖整个 wiki。新增 confidence 机制和防污染模型。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议由 daemon 在运行时生成的 AGENTS.md 提供。学习层（flashcard/FSRS）和 `insights/` 目录在当前 Mulwiki 版本中为未来功能（标记 ⏳），Agent 应忽略。

---

## 1. Types（节点类型，3 种）

| Type | Wiki 目录 | generate_file | Description |
|------|-----------|---------------|-------------|
| concept | wiki/concept/ | true | 每个命名概念一个页面。一文一概念 |
| summary | wiki/summary/ | true | 每个原始资料的摘要 |
| topic | wiki/topic/ | true | 主题级概览，连接相关 concept 页面 |

比 concept-wiki 简单，比 nashsu 更精简。

---

## 2. Structure（连接规则）

**Pattern: Free Wikilink with Explicit Adjacency**

- 通过 `[[wikilink]]` 和 frontmatter `related` 字段显式声明关联
- 双向链接强制：如果 A 的 `related` 包含 B，B 的 `related` 必须回链 A
- `wiki/index.md` 维护邻接关系

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`concept | topic | summary`
- `layer`（string）：
  - `summary` type → `ingest`
  - `topic` type → `analyze`
  - `concept` type → `concept`

### 推荐字段
- `title`（string）：页面标题
- `confidence`（string）：`high | medium | low | speculative`
- `sources`（list）：原始资料路径
- `related`（list）：相关 wiki 页面路径（双向回链强制）
- `created`（date，YYYY-MM-DD）
- `updated`（date，YYYY-MM-DD）
- `tags`（list）：标签

### `confidence` 字段

| Value | Meaning |
|-------|---------|
| `high` | 多源充分支撑（≥ 2 个来源） |
| `medium` | 2 个来源，或来源超过 6 个月 |
| `low` | 少于 2 个来源 |
| `speculative` | 无源，仅推断 |

`speculative` 页面不入邻接表，升级后才能入表。

### 示例 — Concept 页面

```yaml
---
title: "Transformer Attention"
type: concept
layer: concept
confidence: high
sources:
  - sources/attention-is-all-you-need.pdf
  - sources/bert-paper.pdf
related:
  - wiki/concept/multi-head-attention.md
  - wiki/concept/positional-encoding.md
  - wiki/topic/sequence-modelling.md
tags: [attention, transformer, nlp]
created: 2026-04-05
updated: 2026-04-05
---
```

---

## 4. Ingest Pipeline

### 读取阶段
1. 读取 schema.md（本文件）和 AGENTS.md（平台协议）
2. 读取 `wiki/index.md` 了解现有内容

### 编译流程
3. 摘要原始文档 → 写 `wiki/summary/<slug>.md`（type: summary, layer: ingest）
4. 识别文档中所有命名概念
5. 对每个概念：检查是否已存在 → 更新或创建 `wiki/concept/<slug>.md`（type: concept, layer: concept）
6. 检查是否需要新建/更新 topic 页面（type: topic, layer: analyze）
7. 更新 `wiki/index.md`、`wiki/log.md`
8. 写 `output/manifest.json`（格式见 AGENTS.md）

### 增量行为
先读 `wiki/index.md` 确定现有内容，只更新受影响页面。

### 去重
同一原始文档不重复创建 summary 页面。

---

## 5. Lint Rules（健康检查）

在每次 Ingest 之后或按需运行。输出写入 `output/lint-YYYY-MM-DD.md`。

### L1 — 孤儿检测
检查入度 = 0 的页面。

### L2 — 矛盾扫描
检查跨源冲突声明。

### L3 — confidence 审计
- 来源超过 6 个月：建议降级
- 来源数量 < 2：降级为 `low`
- `speculative` 页面出现在邻接表中：报告为错误

### L4 — 缺口检测
检查被多处提及但缺少独立 concept 页面的概念词。

### L5 — 源完整性检查
检查 frontmatter `sources[]` 中的路径是否实际存在。

---

## 6. Wiki 结构

```
wiki/
├── index.md      # 页面目录 + 邻接表
├── log.md        # 操作历史
├── concept/      # 概念页
├── summary/      # 源摘要
└── topic/        # 主题概览
```

---

## 未来功能 ⏳

以下功能在当前 Mulwiki 版本中未实现：
- 学习层（Flashcard + FSRS spaced repetition）
- `insights/` 人工笔记目录
- Sandbox-First 晋升机制

Agent 执行时忽略以上标记 ⏳ 的步骤。
