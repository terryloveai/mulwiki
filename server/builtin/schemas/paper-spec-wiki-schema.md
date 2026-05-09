# Schema: paper-spec wiki-schema

> Version: v1.1.0
> 符合 Mulwiki Schema Spec v1.0.0
> 来源：https://github.com/spectralbranding/paper-spec
> 作者：spectralbranding
> 定位：学术研究专用的 wiki schema。将 Karpathy 三层模式适配到学术场景。强调 claim 追溯和矛盾追踪。

> **Mulwiki 适配说明**：本文件是 schema.md（业务规则）。平台协议由 daemon 在运行时生成的 AGENTS.md 提供。选择性披露、correspondence 追踪、ingest.jsonl 在当前 Mulwiki 版本中为未来功能（标记 ⏳）。

---

## 1. Types（节点类型，7 种）

| Type | Wiki 目录 | generate_file | Description |
|------|-----------|---------------|-------------|
| concept | wiki/concept/ | true | 每个理论构造一个页面 |
| author | wiki/author/ | true | 每个常引作者一个页面 |
| method | wiki/method/ | true | 每个方法论一个页面 |
| dataset | wiki/dataset/ | true | 每个使用或引用的数据集 |
| contradiction | wiki/contradiction/ | true | 源之间的冲突 |
| timeline | wiki/timeline/ | true | 时间线，将事件链接到源 |
| claim | wiki/claim/ | true | 每个论文 claim 一个页面，链接支撑/反对来源 |

| Mulwiki layer 映射 |
|-------------------|
| claim → `ingest`（直接从源提取） |
| author / dataset / timeline → `analyze`（结构化整理） |
| method / contradiction → `analyze` |
| concept → `concept`（抽象知识） |

---

## 2. Structure（连接规则）

**Pattern: Free Wikilink with Cross-References**

- 通过 `[[wikilink]]` 自由连接
- claim 页面需链接支撑/反对来源
- contradiction 页面需链接冲突双方
- author → concept/method：链接研究方向

---

## 3. Frontmatter（元数据规范）

### 必填字段
- `type`（string）：`concept | author | method | dataset | contradiction | timeline | claim`
- `layer`（string）：见上表映射

### 推荐字段
- `title`（string）：页面标题
- `sources`（list）：关联的 source 路径
- `claim_ids`（list）：关联的 claim ID，仅 claim 类型
- `created`（date，YYYY-MM-DD）
- `updated`（date，YYYY-MM-DD）

### 示例 — Concept 页面

```yaml
---
type: concept
layer: concept
title: "Dimensional Collapse"
sources:
  - sources/paper-collapse-2023.pdf
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
claim_ids: ["H1"]
sources:
  - sources/vaswani-2017.pdf
created: 2026-04-20
updated: 2026-04-20
---
```

---

## 4. Ingest Pipeline

### 读取阶段
1. 读取 schema.md（本文件）和 AGENTS.md（平台协议）
2. 读取 `wiki/index.md`（如有）
3. 读取 `sources/` 中的新文件

### 编译阶段
4. 读取源文件（PDF、URL 等）
5. 提取信息，创建/更新 wiki 页面：
   - concept、author、method、dataset（按需）
   - 识别矛盾 → contradiction 页面
   - 对每个 paper 的 claim → claim 页面
6. 更新 `wiki/index.md`
7. 追加 `wiki/log.md`
8. 写 `output/manifest.json`（格式见 AGENTS.md）

### 增量行为
先读 `wiki/index.md`，跳过已处理源。

---

## 5. Lint Rules（健康检查）

在每次 Ingest 之后或按需运行。输出写入 `output/lint-YYYY-MM-DD.md`。

### L1 — 孤儿页面检查
检查无入链的页面。

### L2 — 矛盾一致性检查
检查 wiki 页面与 source 中 claims 的矛盾。

### L3 — 源完整性检查
检查 frontmatter `sources[]` 路径是否实际存在。

---

## 6. Wiki 结构

```
wiki/
├── index.md        # 内容目录
├── log.md          # 操作历史
├── concept/        # 概念页
├── author/         # 作者页
├── method/         # 方法论页
├── dataset/        # 数据集页
├── contradiction/  # 矛盾页
├── timeline/       # 时间线页
└── claim/          # claim 页
```

---

## 未来功能 ⏳

以下功能在当前 Mulwiki 版本中未实现：
- 选择性披露（5 级公开级别）
- Correspondence 追踪 + 隐私级别
- ingest.jsonl 结构化日志
- SHA-256 哈希溯源

Agent 执行时忽略以上标记 ⏳ 的步骤。
