# Schema: Karpathy 原始 llm-wiki

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f
> 作者：Andrej Karpathy
> 定位：用 LLM 增量构建 wiki。最松散、最灵活的类型系统，适合快速启动的知识库。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议（目录结构、manifest.json 格式）由 daemon 在运行时生成的 AGENTS.md 提供。Agent 执行时先读 AGENTS.md（怎么交付），再读本文件（写什么内容）。

---

## 1. Types（节点类型，6 种）

| Type | Wiki 目录 | generate_file | Description |
|------|-----------|---------------|-------------|
| Summary | wiki/summary/ | true | 每个原始资料一页，文件名镜像 sources/ |
| Entity | wiki/entity/ | true | 实体页（人、组织、产品） |
| Concept | wiki/concept/ | true | 概念页（理论、方法、技术） |
| Comparison | wiki/comparison/ | true | 对比页，并列两个或多个对象 |
| Synthesis | wiki/synthesis/ | true | 综合/综述页，跨资料归纳 |
| Overview | wiki/ | false | 全局概览页（wiki/index.md） |

> 类型不是强制的，LLM 自己决定什么时候创建什么页面。每种类型对应一个子目录，便于浏览和维护。

---

## 2. Structure（连接规则）

**Pattern: Free Wikilink**

- 无强制结构规则
- `[[wikilink]]` 双向链接是唯一的连接方式
- LLM 自由决定链接什么
- 所有类型之间可以自由互链，无限制

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`Summary | Entity | Concept | Comparison | Synthesis | Overview`
- `layer`（string）：处理阶段
  - `ingest` — 原始资料提取（Summary 页面）
  - `analyze` — 结构化分析（Entity 页面）
  - `concept` — 抽象概念（Concept 页面）
  - `compile` — 跨源综合（Comparison、Synthesis 页面）
  - `publish` — 最终交付（Overview 页面）

### 推荐字段
- `sources`（list）：原始资料引用
- `created`（date，YYYY-MM-DD）：页面创建日期
- `updated`（date，YYYY-MM-DD）：最后更新日期
- `tags`（list）：标签

### 示例

```yaml
---
type: Concept
layer: concept
sources:
  - sources/attention-is-all-you-need.pdf
created: 2026-04-20
updated: 2026-04-25
tags: [transformer, attention, nlp]
---
```

---

## 4. Ingest Pipeline（自适应 pipeline）

此 pipeline 自动适应任意规模的源（2 页文章 → 300 页书 → 视频字幕 → 图片集）。

---

### Phase 0: 源评估（Source Assessment）

**目标**：判断源的类型和规模，决定处理策略。

1. 读取 `sources/` 中的所有文件
2. 对每个文件判断：
   - **类型**：PDF / 纯文本 / 视频字幕 / 图片 OCR 文本 / 网页
   - **规模**：估算词数、页数、章节数
   - **策略**：≤ 5000 词 → 一次性处理；> 5000 词 → 分章节/分段处理
3. 写 `output/source-plan.md`（处理计划）：
   - 每个源的处理单元列表（章节名/段落范围）
   - 预估产出页面数
   - 处理顺序

---

### Phase 1: 结构提取（Structure Extraction）

**目标**：提取源的内部结构，创建文章地图。

1. 提取目录/章节标题/段落结构
2. 对每个处理单元：识别关键主题、实体、概念（不写完整页面，只标记）
3. 写 `output/source-map.md`：源结构图 + 每单元的预期 wiki 页面列表

---

### Phase 2: 逐单元处理（Per-Unit Processing）

**目标**：每个章节/段落独立提取，边处理边写 output/。不要攒到最后。

对每个处理单元：
1. 深度阅读该单元
2. 提取该单元中的 Entity、Concept → 写对应的 .md 到 `output/entity/`、`output/concept/`
3. 该单元的摘要 → 写 `output/summary/<source>-<unit>.md`（type: Summary）
4. 标注与其他单元的关联（`related:` frontmatter）

**进度跟踪**：每处理完一个单元，追加 `output/progress.md`：`## [timestamp] processed unit N/M: {单元名}`

---

### Phase 3: 跨单元综合（Cross-Unit Synthesis）

**目标**：识别跨章节的模式、矛盾、高层次概念。

1. 重读 Phase 2 产出的所有页面
2. 识别**跨单元概念** → 写 `output/concept/<slug>.md`（type: Concept, layer: concept）
3. 识别**矛盾/张力** → 写 `output/comparison/<slug>.md`（type: Comparison）
4. 识别**高层次归纳** → 写 `output/synthesis/<slug>.md`（type: Synthesis）
5. 补全所有 `[[wikilink]]` 交叉引用
6. 写 `wiki/index.md`（所有页面目录）
7. 写 `wiki/log.md`（本次 ingest 记录）

---

### Phase 4: 自评（Self-Review）

**目标**：agent 自检产出质量，不合格则补充。这是一个强制步骤。

写 `output/self-review.md`，逐条回答：

- [ ] **覆盖度**：处理了多少个单元？是否每个单元都产出了页面？
- [ ] **深度**：每个 Concept 页面是否足够详细（≥ 100 词）？单句定义不算完成。
- [ ] **链接**：所有 `[[wikilink]]` 的目标页面是否真实存在？悬挂链接必须补。
- [ ] **类型使用**：是否使用了多种 type（不仅是 Summary）？至少应有 Entity + Concept。
- [ ] **layer 标签**：每个页面是否标注了正确的 layer？

如果任何一项未通过 → 回到 Phase 2 或 Phase 3 补充，直到全部通过。

---

### Phase 5: 最终交付

仅在 Phase 4 全部通过后执行：

1. 核实所有 output/ 下的文件都已写入
2. 写 `output/manifest.json`（格式见 AGENTS.md）
3. 代理停止。daemon 接管后续。

---

### 输出规范

- 所有 .md 文件写 YAML frontmatter（type + layer + sources 必填）
- `[[wikilink]]` 使用相对路径（如 `[[concept/transformer-attention]]`）
- 每个页面 ≥ 100 词（Summary 除外，100-200 词即可）
- Concept 页面必须 > 200 词，包含定义、来源引用、相关内容
- 交付时 manifest.json 的 `pages` 数组必须列出所有产出文件

### 增量行为

当 `wiki/` 已存在时：先读所有现有页面获取上下文，只更新受影响的页面。以 `wiki/log.md` 判断去重。

### 规模预期

| 源规模 | 预计产出 | 策略 |
|--------|---------|------|
| 短文（< 2000 词）| 3-8 页 | 一次性处理 |
| 中等文章（2000-5000 词）| 8-20 页 | 一次性处理 |
| 长文档/论文（5000-20000 词）| 20-50 页 | 分章节 |
| 书（> 20000 词）| 50-150+ 页 | 分章节，逐章提取 |
| 视频字幕（转录文本）| 按内容词数估算 | 一次性或分段 |

---

## 5. Lint Rules（健康检查）

在每次 Ingest 之后或按需运行。Lint 只读，不修改 wiki 文件。

### L1 — 孤儿检查
检查没有任何入链的页面。报告页面路径和标题。

### L2 — 过时内容检查
检查被新资料推翻的旧声明。

### L3 — 缺失概念页检查
检查被多处 `[[wikilink]]` 引用但尚无对应页面的词条。

### L4 — 重要概念覆盖检查
检查在 sources 中多次出现（≥3 个来源）的关键词是否有独立 Concept 页面。

### L5 — 缺失交叉引用检查
检查内容高度相关但缺少相互 wikilink 的页面对。

### 报告输出
写入 `output/lint-YYYY-MM-DD.md`。

---

## 6. Wiki 结构

```
wiki/
├── index.md       # 内容目录：每页一行 + 摘要
├── log.md         # 时间线：只追加的事件记录
├── summary/       # Summary 页面
├── entity/        # Entity 页面（人、组织、产品）
├── concept/       # Concept 页面（理论、方法、技术）
├── comparison/    # Comparison 页面
└── synthesis/     # Synthesis 页面
```

---

## 设计哲学

- "wiki 是持久复利资产"
- "LLM 不无聊，不会忘记更新交叉引用"
- "人的工作是策划资料、引导分析、问好问题；LLM 的工作是其他一切"
- 三层架构：原始资料（只读）→ Wiki（LLM 维护）→ Schema（配置规则）
