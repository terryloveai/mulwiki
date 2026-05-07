# Schema: nashsu/llm_wiki

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://github.com/nashsu/llm_wiki
> 作者：nashsu
> 定位：Karpathy 模式的完整实现。两步 CoT ingest + 知识图谱 + 异步审核 + Deep Research。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议由 daemon 在运行时生成的 AGENTS.md 提供。知识图谱、向量搜索、Deep Research 在当前 Mulwiki 版本中为未来功能（标记 ⏳），Agent 应忽略这些标记的步骤。

---

## 1. Types（节点类型，7 种）

| Type | Wiki 目录 | generate_file | Description |
|------|-----------|---------------|-------------|
| Source Summary | wiki/source/ | true | 每个原始资料的摘要页，文件名镜像 sources/ |
| Entity | wiki/entity/ | true | 人、组织、产品 |
| Concept | wiki/concept/ | true | 理论、方法、技术 |
| Query | wiki/query/ | true | 有价值的回答，回存为 wiki 页面 |
| Synthesis | wiki/synthesis/ | true | 跨资料综合分析 |
| Comparison | wiki/comparison/ | true | 并列对比两个或多个对象 |
| Overview | wiki/overview.md | false | 全局概览页（每次 ingest 自动更新） |

---

## 2. Structure（连接规则）

**Pattern: Free Wikilink with Knowledge Graph ⏳**

- 通过 `[[wikilink]]` 自由连接
- 所有类型可自由互链
- ⏳ 知识图谱功能（社区检测、图洞察）为未来功能

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`source | entity | concept | query | synthesis | comparison`
- `layer`（string）：
  - `source` type → `ingest`
  - `entity` type → `analyze`
  - `concept` type → `concept`
  - `query` type → `analyze`
  - `synthesis` type → `compile`
  - `comparison` type → `compile`
  - overview → `publish`

### 推荐字段
- `title`（string）：页面标题
- `sources`（list）：原始资料路径
- `created`（date，YYYY-MM-DD）
- `updated`（date，YYYY-MM-DD）

### 示例 — Entity 页面

```yaml
---
type: entity
layer: analyze
title: "Andrej Karpathy"
sources:
  - sources/karpathy-llm-wiki.md
created: 2026-04-20
updated: 2026-04-25
---
```

---

## 4. Ingest Pipeline（两步 CoT 工作流）

### Step 1 — 分析（不写文件）
1. 读取 schema.md（本文件）和 AGENTS.md（平台协议）
2. 读取 `wiki/index.md`（如有）
3. 读取 `sources/` 中的新文件

产出结构化分析：
- 关键实体、概念、论点
- 与现有 wiki 内容的关联
- wiki 结构建议

### Step 2 — 生成（写文件）
基于 Step 1 的分析结果：
1. 写 Source Summary 页：`wiki/source/<slug>.md`
2. 创建/更新 Entity 页：`wiki/entity/<slug>.md`
3. 创建/更新 Concept 页：`wiki/concept/<slug>.md`
4. 更新 `wiki/index.md`、`wiki/log.md`
5. 更新 `wiki/overview.md`
6. 写 `output/manifest.json`（格式见 AGENTS.md）

### 增量行为
先读现有页面获取上下文。已处理的文件通过 `wiki/log.md` 中的记录判断去重。

---

## 5. Lint Rules（健康检查）

在每次 Ingest 之后或按需运行。Lint 只读。输出写入 `output/lint-YYYY-MM-DD.md`。

### L1 — 孤立节点检查
检查度数 ≤ 1 的页面。

### L2 — 矛盾检查
检查不同页面中对同一实体/概念的矛盾陈述。

### L3 — 过时内容检查
检查被新资料推翻的旧声明。

### L4 — 缺失交叉引用检查
检查内容高度相关但缺少相互 wikilink 的页面。

### L5 — 源溯源完整性检查
检查 frontmatter `sources[]` 中的路径是否实际存在于 `sources/` 目录下。

---

## 6. Wiki 结构

```
wiki/
├── index.md       # 内容目录
├── log.md         # 操作历史
├── overview.md    # 全局摘要
├── source/        # 源摘要
├── entity/        # 实体页
├── concept/       # 概念页
├── query/         # 归档回答
├── synthesis/     # 跨源分析
└── comparison/    # 对比页
```

---

## 未来功能 ⏳

以下功能在当前 Mulwiki 版本中未实现：
- 知识图谱（4 信号相关性模型 + Louvain 社区检测）
- 向量语义搜索（LanceDB）
- Deep Research（网络搜索 + 自动回写）
- SHA256 增量缓存
- 异步审核队列

Agent 执行时忽略以上标记 ⏳ 的步骤。
