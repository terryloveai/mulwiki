# Schema: paper-spec paper

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://github.com/spectralbranding/paper-spec
> 作者：spectralbranding
> 定位：科学论文的**机器可读索引格式**。不代替论文本身，而是让论文的 claim、method、acceptance criteria、dependency 变成结构化信息。与 wiki-schema 互补——paper.yaml 的思想映射到 Mulwiki 的 wiki 页面体系。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议由 daemon 在运行时生成的 AGENTS.md 提供。paper.yaml 文件格式为下游参考，Agent 直接从 PDF 提取信息写入 wiki 页面。

---

## 1. Types（节点类型，4 种）

| Type | Wiki 目录 | generate_file | Description |
|------|-----------|---------------|-------------|
| paper | wiki/paper/ | true | 每篇论文的 wiki 摘要页 |
| claim | wiki/claim/ | true | 每个论文 claim 单独一个页面 |
| method | wiki/method/ | true | 每个方法论一个页面 |
| dataset | wiki/dataset/ | true | 每个数据集一个页面 |

| Mulwiki layer 映射 |
|-------------------|
| claim → `ingest`（直接从论文提取） |
| paper → `ingest`（源摘要） |
| dataset → `analyze` |
| method → `analyze` |

---

## 2. Structure（连接规则）

**Pattern: Free Wikilink with Claim Dependencies**

- 通过 `[[wikilink]]` 自由连接
- claim 页面链接来源 paper 页面
- paper 之间通过共同 method/dataset 建立隐式关联
- claim 之间通过 `depends_on` 链式依赖

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`paper | claim | method | dataset`
- `layer`（string）：见上表映射

### 推荐字段
- `title`（string）：页面标题
- `doi`（string）：论文 DOI（paper 类型强烈建议）
- `sources`（list）：关联的源路径
- `claim_id`（string）：对应的 claim ID，仅 claim 类型
- `depends_on`（list）：依赖的 claim ID，仅 claim 类型
- `created`（date，YYYY-MM-DD）
- `updated`（date，YYYY-MM-DD）

### 示例 — Paper 页面

```yaml
---
type: paper
layer: ingest
title: "Attention Is All You Need"
doi: "10.5555/3295222.3295349"
sources:
  - sources/vaswani-2017.pdf
created: 2026-04-20
updated: 2026-04-25
---
```

### 示例 — Claim 页面

```yaml
---
type: claim
layer: ingest
title: "Transformer outperforms RNN on WMT 2014 EN-DE"
claim_id: "H1"
sources:
  - sources/vaswani-2017.pdf
created: 2026-04-20
updated: 2026-04-20
---
```

---

## 4. Ingest Pipeline

### Step 1 — 读取
1. 读取 schema.md（本文件）和 AGENTS.md（平台协议）
2. 读取 `wiki/index.md`（如有）
3. 读取 `sources/` 中的新 PDF

### Step 2 — 提取（每篇论文）
1. 从 PDF 提取 meta 信息（标题、作者、DOI、摘要）
2. 识别 claims（假设 + 支持证据）
3. 识别 methodology、data、与其他论文的依赖

### Step 3 — 生成
4. 写 `wiki/paper/<slug>.md`（type: paper, layer: ingest）
5. 对每个 claim：写 `wiki/claim/<paper-slug>-<claim-id>.md`（type: claim, layer: ingest）
6. 按需创建 method/dataset 页面
7. 更新 `wiki/index.md`、`wiki/log.md`
8. 写 `output/manifest.json`（格式见 AGENTS.md）

### 增量行为
先读 `wiki/index.md`，以 DOI 判断去重。

---

## 5. Lint Rules（健康检查）

输出写入 `output/lint-YYYY-MM-DD.md`。

### L1 — 引用完整性检查
检查 claim 页面的 `sources[]` 路径是否实际存在。

### L2 — Claim 孤儿检查
检查 claim 页面是否有关联的 paper 页面。

### L3 — 矛盾覆盖检查
检查论文间矛盾是否有对应的 contradiction 页面（配合 wiki-schema 的 contradiction 类型）。

### L4 — 源完整性检查
检查 frontmatter `sources[]` 路径是否实际存在。

---

## 6. Wiki 结构

```
wiki/
├── index.md    # 内容目录
├── log.md      # 操作历史
├── paper/      # 论文摘要页
├── claim/      # 单个 claim 页面
├── method/     # 方法论页面
└── dataset/    # 数据集页面
```
