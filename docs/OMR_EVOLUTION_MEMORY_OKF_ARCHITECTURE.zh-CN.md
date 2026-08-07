# OMR Mnemosyne：OKF 渐进式记忆架构

> 版本：2026-08-07 正式实现规格
> 目标版本：OMR v2.1.x～v2.2.0
> 状态：正式实现规格；MEM-01 尚未开始
> 正式名称：OMR Mnemosyne
> 中文描述：OMR 长期记忆与经验进化系统
> 核心决定：Mnemosyne 使用本地文件、OKF 风格知识组织和渐进式披露构建长期记忆；永久不引入向量数据库或 Embedding 检索。

## 1. 文档目的

本文冻结 OMR 下一阶段长期记忆系统的产品目标、架构边界、数据组织、读取协议、自动学习、失败冻结、人工干预和 Web 图谱设计。

本文只确定设计，不代表相关代码已经实现，也不创建 Tag、Release 或迁移现有项目数据。

### 1.1 正式命名

本系统正式命名为：

```text
OMR Mnemosyne
```

Mnemosyne 是 OMR 面向 Reasonix 的长期记忆与经验进化系统。它负责组织、读取、关联、修订、冻结和泛化经验；Reasonix 负责理解并使用这些经验执行真实任务。

产品标语冻结为：

> 记住经验，也记住何时不该相信经验。

品牌名称和技术接口保持分离：

| 层面 | 名称 |
|---|---|
| 正式产品名称 | OMR Mnemosyne |
| 文档和 Web 页面简称 | Mnemosyne |
| 中文功能描述 | OMR 长期记忆与经验进化系统 |
| CLI 根命令 | `omr memory` |
| 建议 Go 包 | `internal/memory` |
| 知识格式 | OKF |
| 原始事实层 | Evidence Store |
| 知识目录 | Memory Wiki |
| 图谱视图 | Memory Graph |
| 冻结区域 | Frozen Memory |
| 经验交换包 | Memory Pack |

不将 CLI 命名为 `omr mnemosyne`，避免拼写成本；Mnemosyne 是产品品牌，`memory` 是稳定、直观的机器接口。

## 2. 已确认的核心共识

1. Reasonix 继续承担 Agent 推理、工具调用、Session、Task、Subagent 和代码执行；OMR 不复制 Agent Runtime。
2. OMR 负责积累和治理可复用的项目经验，使装载 OMR 的 Reasonix 能在后续任务中利用这些经验。
3. 原始执行事实使用严格 JSON 保存；归纳后的长期知识使用 Markdown 与 YAML frontmatter 保存。
4. 知识层采用 OKF 风格的固定目录、`index.md`、类型化页面和显式交叉链接。
5. 读取采用渐进式披露：先路由索引，再局部索引，再具体页面，最后按需读取原始证据。
6. 永久不引入向量数据库，不使用 Embedding 作为主检索或兜底检索。
7. 模型依靠目录、索引、摘要、别名、元数据、交叉链接以及文本搜索找到记忆。
8. 新记忆默认自动记录、自动编译并进入观察期，不要求人工批准。
9. 记忆出现问题时优先自动修订；达到失败阈值后只冻结，不删除、不判定为永久错误。
10. 冻结记忆默认不给模型，不参与正常任务的渐进式检索，因此不会污染模型上下文。
11. 冻结记忆必须完整保留，支持人工 Review、新旧对比、恢复和归纳出更通用的记忆。
12. 后续提供本地 Web 页面，让用户以列表、时间线和知识图谱方式查看与管理记忆。
13. 整套系统正式命名为 OMR Mnemosyne；技术命令继续使用 `omr memory`。
14. 多文件变更采用不可变 Generation、原子 `CURRENT` 切换、Scope 单写锁和 Generation CAS；原始客观事实一经安全落盘，不因派生记忆编译失败而回滚或丢失。
15. 项目经验通过 `global_candidate → global_probation → global_active` 三阶段自动泛化；全局记忆是保留完整来源关系的新记忆，不移动、覆盖或删除项目记忆。
16. 记忆排序先由 Librarian 判断语义相关性，再按适用条件、Scope、Lifecycle、Health 和与 Usage Policy 匹配的当前 Revision/Context 证据强度排序；完全同级时使用可复现的确定性随机。
17. Memory Revision 是知识内容规范事实，Memory Mutation 是追加式审计，Generation 和 OKF Wiki 都是可重建派生状态。
18. Lifecycle、Health、Usage Statistics、所有索引和 Web 视图不得成为第二事实源，必须能从规范事实确定性重建。

## 3. 产品定位

### 3.0 在 OMR 中的位置

```text
Oh My Reasonix
├── Profiles
├── Orchestrator
├── Evolution Engine
└── Mnemosyne
    ├── Project Memory
    ├── Global Memory
    ├── Portable Memory
    ├── Progressive Disclosure
    ├── Memory Graph
    └── Frozen Memory
```

职责边界：

- Evolution Engine 负责从执行证据中发现 Pattern、生成经验和观察结果；
- Mnemosyne 负责经验的长期组织、检索、关联、版本、冻结、恢复与泛化；
- Reasonix 负责推理、工具调用和实际任务执行。

### 3.1 OMR 记忆是什么

Mnemosyne 记忆是具有明确作用域的长期工程经验。它既包括当前项目特有的经验，也包括经过跨项目验证和泛化的全局经验，包括：

- 重复失败模式；
- 已验证的解决策略；
- 适用范围和反例；
- 项目组件之间的关系；
- 设计决策和历史修订；
- 操作 Playbook；
- 记忆在真实任务中的使用结果；
- 失败、冻结、恢复和泛化过程。

### 3.2 OMR 记忆不是什么

Mnemosyne 不替代：

- Reasonix 会话历史；
- 当前对话上下文；
- Reasonix Session/Task 状态；
- 用户个人聊天记忆；
- 模型参数训练；
- 通用互联网知识库；
- 向量数据库或传统 RAG 服务。

### 3.3 与 Reasonix 记忆的分工

| 类型 | Reasonix | OMR |
|---|---|---|
| 当前对话和任务上下文 | 负责 | Mnemosyne 不复制 |
| Session、Task、Subagent 状态 | 负责 | 只消费公开证据 |
| 项目长期经验 | 可在任务中读取 | 负责组织和治理 |
| 失败模式和策略修订 | 提供推理能力 | 负责保存、验证和生命周期 |
| 经验的自动冻结和恢复 | 不负责 | 负责 |
| 记忆图谱和人工管理 | 不负责 | 负责 |

### 3.4 记忆作用域

Mnemosyne 不是单一项目级存储，而是三层 Scope：

| Scope | 内容 | 默认来源 | 是否自动参与当前项目检索 |
|---|---|---|:---:|
| `project` | 项目架构、业务术语、技术栈约束、具体失败模式和 Playbook | 当前项目 Episode | ✅ |
| `global` | 跨项目通用工程经验、稳定用户偏好、通用工具和恢复策略 | 多项目证据泛化 | ✅，低于项目记忆 |
| `portable` | 可签名、导出、导入和显式共享的经验包 | 用户或受控导入 | 导入项目后才参与 |

作用域原则：

- 原始 Episode 默认只属于产生它的项目；
- 项目私有路径、业务术语和内部约束不得直接进入全局记忆；
- 项目记忆优先于全局记忆；
- 全局记忆必须携带适用条件，不等于无条件规则；
- portable 记忆不会绕过当前项目的 Schema、安全、冲突和 Scope 检查；
- 任意 Scope 下的冻结记忆默认都不给模型。

项目身份同时包含：

```yaml
project_scope_id: scope_...
project_family_fingerprint: hmac_...
```

`project_scope_id` 标识当前工作副本，`project_family_fingerprint` 用于判断多个工作副本是否属于同一项目家族。指纹必须使用本机密钥对稳定仓库身份或受控后备身份做 HMAC；全局 Store 只保存不可逆指纹，不保存 Git Remote、绝对路径、项目名称或业务名称。同一仓库的多个 Clone 可以拥有不同 Scope ID，但只能计算为一个独立 Project Family。

## 4. 总体架构

```text
Reasonix 执行任务
        ↓
OMR 记录脱敏 Episode
        ↓
Pattern 识别与经验归纳
        ↓
自动生成或修订 OKF 记忆
        ↓
新记忆进入 probation
        ↓
后续任务渐进式检索
        ↓
记录 retrieved / read / adopted / affected
        ↓
任务结果和失败归因
        ↓
更新 Lifecycle/Health / 自动修订 / 冻结
```

系统分为四层：

| 层 | 职责 | 事实源 |
|---|---|---|
| Evidence Layer | 保存脱敏执行事实和使用回执 | JSON |
| Knowledge Layer | 保存 Pattern、Strategy、Decision、Playbook | Memory Revision + Memory Evidence Generation JSON |
| Retrieval Layer | 分层索引、路由、过滤、按需展开 | `index.md` + frontmatter |
| Governance Layer | Lifecycle、Health、冻结、修订、恢复、审计 | Governance Event、Usage、Outcome 等 JSON 事实 |

## 5. 物理存储结构

项目级事实与知识建议存储在项目目录：

```text
.reasonix/omr/evolution/
├── scope.json
├── overlay.md
│
├── facts/
│   ├── episodes/
│   ├── patterns/
│   ├── proposals/
│   ├── experiments/
│   ├── observations/
│   ├── memory-usage/
│   ├── judgments/
│   ├── context-descriptors/
│   ├── memory-mutations/
│   ├── memory-revisions/
│   ├── memory-evidence-generations/
│   ├── governance-events/
│   ├── generation-input-manifests/
│   └── promotion-candidates/
│
├── memory/
│   ├── CURRENT
│   ├── index.md
│   └── generations/
│       └── <generation-id>/
│           ├── generation.json
│           ├── state/
│           │   ├── memories.json
│           │   └── relations.json
│           └── wiki/
│               ├── index.md
│               ├── log.md
│               ├── components/
│               ├── patterns/
│               ├── failure-concepts/
│               ├── strategies/
│               ├── decisions/
│               ├── playbooks/
│               ├── preferences/
│               └── frozen/
│
├── transactions/
├── locks/
└── maintenance/
    ├── index-state.json
    ├── repair-log.jsonl
    ├── pending-compile.jsonl
    └── snapshots/
```

全局知识建议存储在 OMR 用户级数据目录；具体平台路径由实现阶段统一解析，不能在业务逻辑中硬编码用户 Home：

```text
<omr-user-data>/evolution/
├── scope.json
├── facts/
│   ├── promotion-evidence/
│   ├── memory-usage/
│   ├── judgments/
│   ├── context-descriptors/
│   ├── memory-mutations/
│   ├── memory-revisions/
│   ├── memory-evidence-generations/
│   ├── governance-events/
│   ├── generation-input-manifests/
│   └── promotion-candidates/
├── memory/
│   ├── CURRENT
│   ├── index.md
│   └── generations/
├── transactions/
├── locks/
└── maintenance/
```

portable 经验包不作为常驻事实源。它只是一种受约束的交换格式，经导入检查后转换为当前项目的 `project` 记忆或全局候选。

### 5.1 JSON 事实层

JSON 保存不可由模型自由改写的事实：

- Episode；
- 原始 Pattern 和 Proposal；
- Experiment 与 Gate 结果；
- Observation；
- MemoryUsage；
- Judgment Fact；
- Context Descriptor；
- Memory Mutation；
- Memory Revision；
- Memory Evidence Generation；
- Governance Event；
- Generation Input Manifest；
- Global Promotion Candidate；
- 文件 Hash、Scope 和 Snapshot；
- 冻结、恢复和回滚操作记录。

要求：

- 严格 Schema；
- 未知字段拒绝；
- 原子写入；
- 文件权限默认 `0600`，目录默认 `0700`；
- 拒绝路径穿越和 symlink 越界；
- 项目、全局和 portable Scope 隔离；
- 记录 Hash；
- 敏感内容写入前脱敏；
- 可由 Doctor 诊断和 Repair 修复。

事实源优先级正式冻结为：

```text
facts/memory-revisions/  = 某版知识内容的规范事实源
facts/memory-evidence-generations/ = 某版知识证据集合的规范事实源
facts/governance-events/ = 人工治理操作的追加式规范事实源
facts/judgments/ = Confirmation、Attribution Override 与 Analyst/Critic 判断的规范事实源
facts/generation-input-manifests/ = 历史 Generation 精确重建输入的规范事实源
facts/memory-mutations/  = 发生过什么的追加式审计事实
memory/generations/      = 某一时刻供读取的派生快照
Generation 内 OKF Wiki   = 供模型阅读的派生表示
```

`memory-revisions/<memory-id>/<revision>.json` 保存永久不可变的规范化语义内容和内容 Hash；`memory-evidence-generations/<memory-id>/<revision>/<evidence-generation>.json` 保存该 Revision 的不可变 EvidenceRef 集合及集合 Hash；`governance-events/<event-id>.json` 保存 pin、unpin、manual freeze、unfreeze、archive 等人工治理事实；`generation-input-manifests/<generation-id>.json` 永久记录历史 Generation 实际采用的规范事实集合及编译器版本；`memory-mutations/<transaction-id>.json` 保存知识动作、前后 Revision、前后 Hash、证据和事务结果。Mutation Replay 应能够重现 Revision Hash，但 Replay 结果不能取代 `revision.json` 成为内容事实源。

如果 Mutation Replay 与 `revision.json` 不一致：

- `revision.json` 仍是该 Revision 的规范内容事实；
- Doctor 必须报告阻塞性一致性错误；
- 系统不得基于损坏链继续生成新 Revision；
- Repair 只能依据 Revision Hash、Mutation 前后 Hash、事务记录和原始证据恢复；
- 不得静默选择、覆盖或伪造任一版本。

任何派生状态都必须能够从规范事实源确定性重建，至少包括 Lifecycle、Health、Usage Statistics、Relation Index、Root/Local Index、Generation 和 Web 派生视图。任何名为 `state.json`、缓存、索引或 Web View 的文件都不得成为第二事实源；删除全部派生状态后，系统必须能够仅凭规范事实重建相同结果。

### 5.2 OKF 知识层

Markdown 保存模型可直接理解和维护的知识：

- 一个页面只表达一个主要概念；
- 页面路径稳定且可预测；
- 每个非保留页面必须包含 YAML frontmatter；
- 每级目录必须存在 `index.md`；
- 页面必须保留证据引用；
- 页面之间通过显式关系连接；
- 修改必须写入 `log.md`；
- Wiki 可以从 JSON 事实层重建，不作为不可恢复的唯一事实源。

知识层不是原地更新的可变目录。每次编译生成一个完整、不可变的 Generation；稳定入口 `memory/index.md` 只负责把人和模型路由到当前 Generation，机器读取以 `memory/CURRENT` 为准。`omr memory context` 必须返回固定的 Generation ID 和该 Generation 内的页面路径，保证一次任务期间读取一致。

### 5.3 规范事实与派生状态总表

| 数据 | 规范事实源 | 是否可删除后重建 | 是否允许作为第二事实源 |
|---|---|:---:|:---:|
| Episode、Observation、MemoryUsage、Outcome | `facts/` 对应不可变记录 | 否 | ❌ |
| Judgment Fact | `facts/judgments/` | 否 | ❌ |
| Memory Revision 内容 | `facts/memory-revisions/` | 否 | ❌ |
| Memory Evidence Generation | `facts/memory-evidence-generations/` | 否 | ❌ |
| Governance Event | `facts/governance-events/` | 否 | ❌ |
| Generation Input Manifest | `facts/generation-input-manifests/` | 否 | ❌ |
| Memory Mutation 审计 | `facts/memory-mutations/` | 否 | ❌ |
| Context Descriptor | `facts/context-descriptors/` | 否 | ❌ |
| Global Promotion Candidate | `facts/promotion-candidates/` | 否 | ❌ |
| Lifecycle、Health、Pinned、Archived | 由 Revision、Evidence Generation、Usage Policy、Outcome、Judgment Fact 和 Governance Event 派生 | ✅ | ❌ |
| Usage Statistics | 由 MemoryUsage、Outcome 和独立 Episode 规则派生 | ✅ | ❌ |
| Relation Index | 由 Revision Relation Record 和事实引用派生 | ✅ | ❌ |
| Root/Local Index | 由当前有效 Revision、Lifecycle、Health 和 Scope 派生 | ✅ | ❌ |
| Generation | 由 Generation Input Manifest 指定的规范事实和编译器版本派生 | ✅ | ❌ |
| OKF Wiki | Generation 内的模型阅读表示 | ✅ | ❌ |
| Web 列表、图谱、统计和缓存 | 由固定 Generation 与规范事实派生 | ✅ | ❌ |

实现允许缓存派生结果，但缓存必须记录输入 Generation、Schema/Compiler Version 和 Hash。缓存删除、损坏或过期不能影响规范事实；命中与否也不能改变逻辑结果。

## 6. OKF 页面 Schema

基础 frontmatter：

```yaml
---
okf_version: "0.1"
type: strategy
memory_id: mem_01K7A9X2
canonical_key: verify-before-upgrade-retry
title: Verify Before Upgrade Retry
summary: 升级失败后先识别失败来源，再执行最小恢复动作。

lifecycle: probation
health: healthy
pinned: false
usage_policy: outcome_attributed
revision: 1
evidence_generation: 1
content_sha256: sha256_...
evidence_set_sha256: sha256_...

component:
  - installer
  - doctor
operations:
  - upgrade
task_classes:
  - installation_recovery
failure_concept_refs:
  - scope: project
    memory_type: failure_concept
    memory_id: mem_failure_asset_drift
    revision: 1

aliases:
  - source hash changed
  - prompt hash drift
  - orchestrator prompt drift

applies_when:
  - manifest exists
  - asset source is known
does_not_apply_when:
  - user-authored prompt conflict

evidence:
  - scope: project
    evidence_type: episode
    evidence_id: episode_001
    content_sha256: sha256_...
relations:
  - predicate: derived_from
    target:
      scope: project
      memory_type: pattern
      memory_id: mem_pattern_prompt_hash_drift
      revision: 2

created_at: 2026-08-07T00:00:00Z
updated_at: 2026-08-07T00:00:00Z
---
```

OKF Page 的规范编译公式固定为：

```text
OKF Page = MemoryRevision + 当前 MemoryEvidenceGeneration + DerivedMemoryState
```

标题、正文、适用条件、边界、关系和 `usage_policy` 来自 MemoryRevision；EvidenceRef、`evidence_generation` 和 `evidence_set_sha256` 来自当前 MemoryEvidenceGeneration；Lifecycle、Health、Pinned、Archived 和使用统计来自 DerivedMemoryState。OKF frontmatter 是三类输入的派生组合，不要求和任一单独 JSON 对象字段完全等价，也不能反向覆盖任何规范事实。

运行统计不属于知识正文，不写入页面的规范化知识内容。`usage_count`、`counted_help_count`、`counted_harm_count` 和 `last_used_at` 从 JSON Usage/Outcome 层确定性派生；Web 或索引需要展示时，可以生成可重建的派生快照，但不能因此增加知识 Revision。

`content_sha256` 是 MemoryRevision 的正式 Schema 字段，`evidence_set_sha256` 是 MemoryEvidenceGeneration 的正式 Schema 字段。计算 `content_sha256` 时排除 Hash 字段自身以及运行统计、派生排序和 UI 字段；`evidence_set_sha256` 对规范化、去重、稳定排序后的 EvidenceRef 集合计算。Hash 计算规范必须版本化并由程序执行，模型不得自由生成可信 Hash。

正文建议固定包含：

```text
Summary
Applicable Conditions
Known Boundaries
Recommended Action
Do Not
Evidence
Revision Notes
```

## 6.1 Memory Type 与 Usage Policy

`usage_policy` 是每个 Memory Revision 的必填 Schema 字段，决定该类知识如何获得晋升、降级和冻结证据。它不能只存在于 Go 代码的 Type 映射中。

允许的策略固定为：

| Usage Policy | 用途 | 正负证据来源 |
|---|---|---|
| `outcome_attributed` | 会实际影响任务动作的操作性知识 | affected + evaluated + confirmed attribution |
| `evidence_validated` | 描述事实、模式和本体的知识 | 独立 Evidence 支持、反例、冲突和来源完整性 |
| `explicit_confirmation` | 必须由用户或正式决策源确认的知识 | 可验证的用户确认或项目正式来源 |

Memory Type 对 Usage Policy 的允许矩阵：

| Memory Type | 允许的 Usage Policy |
|---|---|
| `strategy` | `outcome_attributed` |
| `playbook` | `outcome_attributed` |
| `component` | `evidence_validated` |
| `pattern` | `evidence_validated` |
| `failure_concept` | `evidence_validated` |
| `preference` | `explicit_confirmation` |
| `decision` | `evidence_validated`、`explicit_confirmation` |

任何 `usage_policy=explicit_confirmation` 的 Revision 都必须携带 `confirmation_source_ref`，指向合法的 Confirmation Judgment Fact；模型不能自行宣称已确认。Schema 拒绝 Type 不允许的 Usage Policy。未来新增 Memory Type 时必须先更新 Schema 允许矩阵，但生命周期引擎只依赖固定 Usage Policy，不新增另一套状态机。

三类策略共享 Lifecycle 字段，但晋升证据不同：

- `outcome_attributed`：依赖唯一正负计分协议；
- `evidence_validated`：依赖独立证据数量、来源一致性、反例和冲突状态，不要求 helped/harmed；
- `explicit_confirmation`：依赖可验证确认来源，撤销确认时进入 Review，不伪造任务效果。

## 6.2 规范 MemoryRevision 与 MemoryEvidenceGeneration Schema

`facts/memory-revisions/<memory-id>/<revision>.json` 是 Revision 内容的规范事实，至少包含：

```json
{
  "schema_version": 1,
  "memory_id": "mem_01K7A9X2",
  "memory_type": "strategy",
  "scope": "project",
  "canonical_key": "verify-before-upgrade-retry",
  "revision": 2,
  "usage_policy": "outcome_attributed",
  "confirmation_source_ref": null,
  "title": "Verify Before Upgrade Retry",
  "summary": "...",
  "applies_when": ["..."],
  "does_not_apply_when": ["..."],
  "failure_concept_refs": [],
  "relations": [],
  "aliases": [],
  "content_sha256": "sha256_...",
  "created_at": "..."
}
```

`facts/memory-evidence-generations/<memory-id>/<revision>/<generation>.json` 是该 Revision 支撑证据集合的规范事实，至少包含：

```json
{
  "schema_version": 1,
  "memory_id": "mem_01K7A9X2",
  "revision": 2,
  "evidence_generation": 3,
  "evidence_refs": [],
  "evidence_set_sha256": "sha256_...",
  "previous_evidence_generation": 2,
  "transaction_id": "tx_01K...",
  "created_at": "..."
}
```

MemoryRevision 和 MemoryEvidenceGeneration 都永久不可变，未知字段拒绝。`usage_policy=explicit_confirmation` 时 `confirmation_source_ref` 必须是合法 ConfirmationSourceRef；其他 Policy 该字段必须为 `null`。同一 Revision 的 Evidence Generation 单调递增；追加证据必须创建新 Evidence Generation 文件，绝不修改既有 `revision.json` 或既有 Evidence Generation。

Lifecycle、Health、Pinned、Archived、使用统计、排序键和 Web 展示字段不属于 Revision 或 Evidence Generation 规范内容；它们由 Usage、Outcome、Attribution Override 和 Governance Event 等规范事实派生。

OKF 页面编译器必须按固定公式组合 MemoryRevision、所选 Evidence Generation 和 DerivedMemoryState。页面不能包含三类输入中不存在的知识结论或治理状态；页面解析结果不能反向覆盖规范事实。

### 6.2.1 Judgment Fact Schema

Confirmation Source 和 Attribution Override 统一保存为不可变 Judgment Fact：

```text
facts/judgments/<judgment-id>.json
```

Confirmation 示例：

```json
{
  "schema_version": 1,
  "judgment_id": "judgment_01K...",
  "judgment_type": "confirmation",
  "scope": "project",
  "subject": {
    "subject_type": "memory_revision",
    "memory_ref": {
      "scope": "project",
      "memory_type": "preference",
      "memory_id": "mem_pref_01K...",
      "revision": 1
    }
  },
  "source": {
    "source_type": "user",
    "source_id": "local_user"
  },
  "confirmation": {
    "status": "confirmed",
    "declared_scope": "project"
  },
  "supersedes_judgment_ref": null,
  "basis_refs": [],
  "content_sha256": "sha256_...",
  "created_at": "..."
}
```

Attribution Override 示例：

```json
{
  "schema_version": 1,
  "judgment_id": "judgment_01K...",
  "judgment_type": "attribution_override",
  "scope": "project",
  "subject": {
    "subject_type": "memory_outcome",
    "outcome_id": "outcome_01K..."
  },
  "source": {
    "source_type": "user",
    "source_id": "local_user"
  },
  "attribution_override": {
    "previous_effect": "harmed",
    "new_effect": "neutral",
    "reason": "失败由不可用的第三方服务导致"
  },
  "supersedes_judgment_ref": null,
  "basis_refs": [],
  "content_sha256": "sha256_...",
  "created_at": "..."
}
```

本节正式冻结 `confirmation` 和 `attribution_override` 两种必须支持的 Judgment subtype 及其 payload。Analyst/Critic 产生的其他 Judgment 同样复用基础 Envelope 与 JudgmentRef，并由对应协议定义固定 subtype payload；实现不得接受未注册的自由 `judgment_type`。每种 subtype 使用带判别字段的严格联合 Schema，选择一种类型时，其他 subtype payload 必须缺失。Judgment Fact 未知字段拒绝、永久不可变、写入前脱敏，`content_sha256` 由程序对排除 Hash 字段自身的规范内容计算。撤销确认或修正 Override 必须创建新 Judgment，并通过 `supersedes_judgment_ref` 保留完整链路，不能原地覆盖。

正式引用模型：

```yaml
JudgmentRef:
  scope: project
  judgment_type: attribution_override
  judgment_id: judgment_01K...
  content_sha256: sha256_...

ConfirmationSourceRef:
  scope: project
  judgment_type: confirmation
  judgment_id: judgment_01K...
  content_sha256: sha256_...
```

`ConfirmationSourceRef` 是受约束的 JudgmentRef，其 `judgment_type` 必须为 `confirmation`。引用的 Scope、ID、类型和 Hash 必须与目标 Judgment Fact 完全一致。

任何 `usage_policy=explicit_confirmation` 的 MemoryRevision 都必须携带合法 `confirmation_source_ref`，不只限于 Decision；Preference 以及未来允许该 Policy 的类型同样适用。其他 Usage Policy 不得借用 Confirmation Source 代替自身证据协议。

## 6.3 记忆身份模型

Mnemosyne 采用“双重身份”，同时保障模型阅读和程序引用：

| 字段 | 用途 | 稳定性 |
|---|---|---|
| `memory_id` | JSON 引用、关系、Usage、证据、审计 | 创建后永不修改 |
| `canonical_key` | 可读文件路径、索引和文本导航 | 创建后原则上不修改 |
| `title` | 人类和模型展示 | 可以修改 |
| `revision` | 知识正文和语义版本 | 单调递增 |
| `evidence_generation` | 独立 Evidence Generation 文件版本 | 证据变化时递增，既有文件不可修改 |
| `content_sha256` | 完整性和漂移检查 | 正文变化时更新 |
| `evidence_set_sha256` | Evidence Generation 中规范 EvidenceRef 集合完整性 | 新 Evidence Generation 创建时计算 |
| `usage_policy` | 生命周期证据协议 | Revision 创建后不可原地修改 |
| `aliases` | 旧名称、同义词和检索入口 | 可以追加 |

完整机器身份由以下字段共同确定：

```text
scope + type + memory_id
```

例如：

```text
project:strategy:mem_01K7A9X2
global:strategy:mem_01K8B2Q4
```

`memory_id` 必须全局唯一且不可由标题推导；实现可以使用具有排序性的随机 ID，但不能包含项目名称、路径或业务敏感信息。

## 6.4 稳定 Canonical Key 与文件路径

页面路径使用稳定、可读的 Canonical Key：

```text
wiki/strategies/verify-before-upgrade-retry.md
```

标题修改不自动改变 Canonical Key：

```yaml
canonical_key: verify-before-upgrade-retry
title: Diagnose Before Retrying OMR Upgrade
aliases:
  - verify before upgrade retry
  - prompt hash drift recovery
```

确实需要变更 Canonical Key 时，必须通过显式 Rename 事务完成：

```bash
omr memory rename <memory-id> <new-canonical-key> --dry-run
```

Rename 必须：

- 创建 Snapshot；
- 更新全部显式链接和索引；
- 将旧 Key 写入 `aliases`；
- 检查断链和碰撞；
- 原子提交；
- 写入审计记录；
- 失败时完整恢复。

## 6.5 Revision 与运行统计边界

以下变化增加 `revision`：

- 推荐动作变化；
- 适用条件或明确反例变化；
- 边界和风险变化；
- 语义关系和适用关系变化；
- 正文结论变化；
- Scope 发生显式迁移。

以下变化不增加知识 Revision：

- 新增一次 Retrieved、Read、Adopted 或 Affected Usage；
- `counted_help_count` 或 `counted_harm_count` 变化；
- 最近使用时间变化；
- Web 展示统计变化；
- 可从 JSON 事实层重建的派生指标变化。

仅增加支撑证据或反例引用时，不增加知识 Revision，而是创建新的不可变 MemoryEvidenceGeneration；新文件使用递增的 `evidence_generation` 和程序计算的 `evidence_set_sha256`。只有证据导致正文、适用条件、边界或语义关系变化时，才创建新知识 Revision，并为该 Revision 创建初始 Evidence Generation。

优先级、`counted_help_count`、`counted_harm_count` 和排序键都是可从 MemoryUsage、Outcome 和当前 Retrieval 重建的派生数据，不写入知识正文，也不改变知识内容 Hash。新 Revision 不继承旧 Revision 的排序权重；旧 Revision 的历史统计继续保留供 Review 和修订归因使用。

`content_sha256` 只覆盖规范化知识内容，不覆盖运行统计和 UI 派生字段。这样每次使用记忆不会制造无意义的新 Revision。

## 6.6 确定性去重

去重分为两步。

第一步由 OMR 使用结构化字段筛选候选：

```text
scope
+ type
+ component
+ operation
+ task_class
+ failure_concept_refs
+ canonical_key
+ aliases
```

同时计算结构化指纹：

```text
type
+ normalized component
+ normalized operation
+ normalized failure_concept_refs
+ normalized recommendation action
→ semantic_fingerprint
```

`semantic_fingerprint` 是规范化结构字段的 Hash，不是 Embedding，也不进行向量相似度计算。

第二步由 Reasonix 在候选集合内判断关系，只允许返回：

```text
same
revision
related
contradict
independent
```

OMR 必须验证返回值、引用目标、Scope 和证据，不能让模型通过自由文本直接决定文件写入。

## 6.7 碰撞、合并与拆分身份

如果不同概念生成相同 Canonical Key，依次使用：

```text
<canonical-key>
<canonical-key>--<component>
<canonical-key>--<component>--<short-memory-id>
```

任何碰撞都不得覆盖已有页面。

合并记忆时：

- 保留创建更早且证据链完整的 `memory_id`；
- 被合并记忆进入 `superseded`；
- 在主记忆新 Revision 上记录指向被合并记忆的 `supersedes`；反向 `superseded_by` 只作为派生视图；
- 保留原页面、Usage 和全部历史。

拆分记忆时：

- 原记忆进入 `superseded`；
- 记录 `split_into`；
- 每条新记忆获得新的 `memory_id`；
- 新记忆通过 `derived_from` 指向原记忆；
- 原有证据按可归因范围重新关联，不能无差别复制到所有新记忆。

## 6.8 MemoryMutationPlan

所有自动知识变化必须由严格的 `MemoryMutationPlan` 描述。Reasonix 负责在受限候选集合中判断关系，OMR 负责验证和执行，Reasonix 不直接写入 Wiki。

```yaml
schema_version: 1
transaction_id: tx_01K...
operation: revise
target:
  scope: project
  memory_type: strategy
  memory_id: mem_01K7A9X2
  expected_revision: 2
reason: 新证据表明该策略只适用于 Manifest 已存在的情况
evidence:
  - scope: project
    evidence_type: episode
    evidence_id: episode_041
    content_sha256: sha256_...
changes:
  applies_when:
    add:
      - manifest exists
  does_not_apply_when:
    add:
      - first-time installation
expected_result:
  revision: 3
```

MemoryMutationPlan 是模型输出的结构化意图，不包含可信 before/after Hash。Memory Service 读取规范事实、执行规范化后自行计算 before/after `content_sha256` 和 `evidence_set_sha256`，并验证 `expected_revision`、Scope 和证据引用。

Memory Service 在切换 `CURRENT` 前，将最终 Mutation Fact 以不可变 JSON 保存到 `facts/memory-mutations/<transaction-id>.json`，补充实际 before/after Hash、Revision、Evidence Generation、目标 Generation 和时间。它通过 `transaction_id` 与 prepared 事务绑定；事务是否生效由 `CURRENT` 与事务提交记录确定。失败事务保留审计，但不得伪造已提交的 Revision。

允许的动作固定为：

```text
no_change
create
append_evidence
revise
split
merge
specialize
generalize
relate
contradict
```

模型不得扩展动作枚举，不得通过自由文本直接决定文件写入。

## 6.9 动作判定规则

### `no_change`

新事实没有产生新知识，或证据不足以可靠选择其他动作时，不修改 Wiki，只保存 Episode、Observation 或 MemoryUsage。不得为了展示“自进化”而强行生成变化。

### `create`

只有在没有已有记忆表达相同主要结论、不是 Alias、不是普通补充证据，且候选能表达单一独立概念时创建。新记忆统一进入 `probation`。

### `append_evidence`

推荐动作、适用条件、边界和正文均未变化时，只追加证据：

```text
revision 不变
创建新的不可变 MemoryEvidenceGeneration
evidence_generation + 1
由 Memory Service 计算 evidence_set_sha256
```

### `revise`

同一主要概念的推荐动作、适用条件、反例、风险或边界变化时创建新 Revision：

```text
memory_id 不变
canonical_key 不变
revision + 1
新 Revision 进入 probation
旧 Revision 保留在历史
```

### `split`

一条记忆包含多个可独立使用的规则、不同场景需要不同动作或证据来源明显分离时，可以自动拆分。计划必须逐条声明 Evidence 归属；无法归属的证据留在原记忆历史中，不得复制给全部新记忆。

```yaml
evidence_distribution:
  mem_new_a:
    - episode_001
    - episode_003
  mem_new_b:
    - episode_002
```

原记忆进入 `superseded`，新记忆分别进入 `probation`。

### `merge`

只有 Type 相同、推荐动作实质相同、适用条件兼容、没有未解决冲突且合并后仍只有一个主要概念时，才允许自动合并。

主 ID 按确定性规则选择：

```text
证据链完整度
→ 创建时间更早
→ memory_id 稳定排序
```

“证据链完整度”只检查当前 Usage Policy 所要求的引用是否完整、Hash 是否匹配、是否存在断链或未解决冲突，不按动态 help/harm 次数、Evidence 数量或 Confirmation 次数选择主 ID。这样同一输入集合在统计变化后仍得到相同身份结果。

保留主 `memory_id` 并创建新 Revision；其他记忆进入 `superseded`，但不删除历史、证据和 Usage。

### `specialize`

项目特例不能修改通用全局记忆。OMR 创建新的项目记忆，并通过 `specializes` 指向全局来源；全局记忆保持不变。

### `generalize`

多项目证据达到全局泛化门槛后创建新的 `global/probation` 记忆。原项目记忆继续保留项目细节，不进入 `superseded`。

### `relate`

两条记忆相关但不存在替代、包含或冲突时建立幂等双向关系。

### `contradict`

两条记忆在重叠适用条件下给出不同建议时建立显式冲突。正常检索必须返回冲突，不能静默任选；后续由优先级协议或可复现对比实验处理。

## 6.10 有界记忆进化

Mnemosyne 的自进化是基于现有记忆、现有执行证据和固定协议的有界进化，不是无限自我改写。

每次进化必须满足：

- 来源是已保存的 Episode、Pattern、MemoryUsage 或现有记忆；
- 目标属于固定知识类型和允许字段；
- 操作属于固定 `MemoryMutationPlan` 枚举；
- 引用的记忆、证据和 Revision 均存在；
- 修改范围可以通过结构化 Diff 表达；
- 结果能够被 Schema、Doctor、Snapshot 和 Hash 验证；
- 不确定时选择 `no_change`；
- 不得修改 Mutation Schema、验证器、信任边界或自身 Go 源码；
- 不得无证据生成新事实；
- 不得递归触发没有新 Episode 或新 Evidence 的进化。

自动 Split、Merge、Revise 和 Generalize 都必须遵守同一边界。后期更高级的自进化仍然是在已有知识图谱和事实证据上进行受约束演化，而不是扩大成无限、不可归因的自修改。

## 7. 显式知识关系

OKF 是文件规范，不是图数据库。OMR 通过 frontmatter 和链接形成可编译的知识图谱。

机器关系不得引用 Canonical Key、Markdown 路径、标题或自由文本。正式引用类型为：

```yaml
MemoryRef:
  scope: project
  memory_type: strategy
  memory_id: mem_01K7A9X2
  revision: 2       # 可选；缺失表示当前有效 Revision

EvidenceRef:
  scope: project
  evidence_type: episode
  evidence_id: episode_019
  content_sha256: sha256_...
```

MemoryRef 指向长期知识，EvidenceRef 指向不可变执行或验证事实，JudgmentRef 指向不可变判断事实，ConfirmationSourceRef 是类型受限的 JudgmentRef。Canonical Key 和路径只负责展示、文本导航和渐进式披露，不能充当机器身份。

正式 Memory Relation Enum：

| Predicate | 方向 | 含义 |
|---|---|---|
| `related_to` | 对称 | 相关但不存在更强语义 |
| `derived_from` | 有向 | 当前记忆由目标记忆演化或拆分而来 |
| `supersedes` | 有向 | 当前记忆替代目标记忆或 Revision |
| `repaired_by` | 有向 | 当前冻结或受损记忆由目标记忆修复 |
| `generalized_from` | 有向 | 当前通用记忆由目标具体记忆归纳 |
| `specializes` | 有向 | 当前记忆是目标通用记忆的特例 |
| `broader_than` | 有向 | 当前记忆是目标记忆的父级或更宽泛概念 |
| `contradicts` | 对称 | 与目标记忆存在重叠适用条件下的冲突 |
| `compared_with` | 对称 | 与目标记忆进入过显式对比流程 |
| `split_into` | 有向 | 当前被拆分记忆产生了目标记忆 |
| `merged_from` | 有向 | 当前合并结果吸收了目标记忆 |

关系记录统一使用：

```yaml
source:
  scope: project
  memory_type: strategy
  memory_id: mem_source
  revision: 2
predicate: specializes
target:
  scope: global
  memory_type: strategy
  memory_id: mem_target
  revision: 1
```

证据支持不属于 Memory Relation Enum，使用独立的 EvidenceRef 字段，例如 `evidence`、`derived_from_evidence`。以下旧名称不进入正式枚举：

- `related` 统一为 `related_to`；
- `conflicts_with` 统一为 `contradicts`；
- `superseded_by` 由 `supersedes` 反向派生；
- `contradiction_resolved` 保存为审计或 Mutation 事实；
- `imported_from` 保存为 Provenance；
- 旧 `parent` 字段统一迁移为 `child specializes parent`；`parent broader_than child` 只能作为该关系的反向派生视图，不能把方向写反；
- `used_by`、`failed_in` 从 MemoryUsage 和 MemoryOutcome 派生。

所有反向边、关系索引和图谱都是规范 Revision、Relation Record、Usage 与 Outcome 的派生视图，不额外引入图数据库或第二事实源。

## 8. 渐进式披露读取协议

### 8.0 已冻结的接入路线

Mnemosyne 的自动读取分为两个阶段：

```text
第一阶段：方案 B
父 Agent 启动 Mnemosyne Librarian Subagent
→ Librarian 获取 omr memory context 提供的确定性路由入口
→ Librarian 渐进式翻阅 Project / Global Memory
→ Librarian 返回相关记忆页面索引
→ 父 Agent 亲自读取相关页面并决定是否采用

后续阶段：方案 C
Reasonix 提供稳定任务生命周期入口
→ 任务开始时自动触发 Mnemosyne 检索
→ 复用同一套 Librarian、Context 和回执协议
```

第一阶段不等待 Reasonix 新增宿主接口。方案 C 只替换触发入口，不重写 Mnemosyne 的存储、检索和使用协议。

`omr memory context` 不执行向量检索，也不替模型做开放式语义判断。它只提供：

- 同时固定的 Project / Global Generation 与 Wiki 路由入口；
- 当前 Scope 和项目身份；
- 可用状态和协议版本；
- Component、Operation、Failure Concept Ref 等确定性过滤入口；
- 本次检索 ID；
- Frozen Memory 排除规则。

### 8.1 分层读取

```text
L0：System Prompt 中的一条短指引
    ↓
L1：wiki/index.md 根路由
    ↓
L2：领域目录 index.md
    ↓
L3：具体 Pattern / Strategy / Playbook 页面
    ↓
L4：必要时按 EvidenceRef 读取 `facts/` 中的对应事实
```

### 8.2 System Prompt 只注入入口

System Prompt 不注入完整 Wiki，只提供类似指引：

```text
项目存在 OMR Mnemosyne Memory。
遇到复杂任务、重复失败或需要项目经验时，先调用
omr memory context 获取本次任务固定的 Project / Global Generation 路由，
再按照返回的根索引和局部索引逐层展开。
只读取与当前任务相关的页面；需要核验证据时再按 EvidenceRef 读取 `facts/` 中的对应事实。
正常任务不得读取 frozen 索引。
```

### 8.3 根索引职责

根 `index.md` 只包含：

- 知识领域；
- 每个领域的一句话摘要；
- 适用任务提示；
- 指向局部索引的链接；
- 当前 Wiki Schema 和更新时间；
- 明确说明冻结区不参与正常检索。

### 8.4 局部索引职责

每条索引至少包含：

- 页面 ID；
- 标题；
- 一句话摘要；
- 类型；
- 组件；
- 适用条件；
- 状态；
- 关键别名；
- 页面链接。

### 8.5 确定性检索手段

Reasonix 可以使用：

- 目录导航；
- `index.md` 路由；
- frontmatter 字段；
- 文件名和稳定 ID；
- `aliases`；
- 交叉链接；
- 文本搜索；
- 项目组件和失败分类过滤。

### 8.6 多 Scope 读取顺序

正常任务的读取顺序固定为：

```text
当前任务明确要求
→ 项目 wiki/index.md
→ 项目 pinned / active / probation 记忆
→ 全局 wiki/index.md
→ 全局 pinned / active / probation 记忆
→ 必要时按 EvidenceRef 读取 `facts/` 中的对应事实
```

冲突优先级固定为：

```text
用户本次明确要求
>
项目 pinned
>
项目 active
>
全局 pinned
>
全局 active
>
项目 probation
>
全局 probation
```

如果项目记忆与全局记忆冲突：

- 当前任务使用项目记忆；
- 记录冲突关系和使用结果；
- 不静默修改全局记忆；
- 后续根据多项目证据判断是项目特例、全局边界缺失还是全局记忆需要修订。

冻结记忆无论属于项目还是全局，都不出现在正常读取链路中。

### 8.7 全局用户偏好

稳定的跨项目工作偏好属于全局记忆，但必须与技术 Pattern 分开存放在 `wiki/preferences/`，例如：

- 先输出开发计划，再执行实现；
- 沙箱问题交由具备对应权限的执行环境处理；
- 代码实现后需要独立 Review；
- PR 描述语言和交付习惯。

一次性指令、项目临时要求或推测出的用户偏好不得自动升级为全局偏好。

### 8.8 Mnemosyne Librarian

`MemoryContext` 必须同时固定 Project 和 Global 两个读取快照：

```yaml
schema_version: 1
retrieval_id: retrieval_123
project:
  scope_id: scope_project_123
  generation: gen_project_000013
  root_index: .reasonix/omr/evolution/memory/generations/gen_project_000013/wiki/index.md
global:
  scope_id: scope_global
  generation: gen_global_000021
  root_index: <omr-user-data>/evolution/memory/generations/gen_global_000021/wiki/index.md
```

没有 Project 或 Global Memory 时，对应字段显式为 `null`，不能复用另一个 Scope 的 Generation。一次任务从取得 MemoryContext 起，到父 Agent 输出最终 MemoryUsage 回执为止，不得重新读取任一 Scope 的 `CURRENT`。

Mnemosyne 使用专用只读 Subagent 翻阅记忆：

```text
Profile ID：omr-memory
产品角色：Mnemosyne Librarian
权限：只读
```

Librarian 的职责：

1. 接收父 Agent 提供的任务摘要；
2. 获取 `omr memory context`；
3. 先读取 Project Index，再按需读取 Global Index；
4. 按根索引、局部索引、具体页面逐层展开；
5. 默认跳过 Frozen Memory；
6. 识别适用条件、冲突和 Scope；
7. 返回相关页面的索引、Revision、Scope 和命中理由；
8. 不替父 Agent 执行任务，也不修改项目或记忆。

Librarian 不把完整记忆正文复制给父 Agent。其返回结果只提供定位和选择依据，例如：

```yaml
retrieval_id: retrieval_123
project_generation: gen_project_000013
global_generation: gen_global_000021
recommended_pages:
  - memory_id: mem_abc
    revision: 2
    scope: project
    path: strategies/verify-before-upgrade-retry.md
    why: component、operation 和 Failure Concept Ref 精确匹配
optional_pages:
  - memory_id: mem_xyz
    revision: 1
    scope: global
    path: strategies/run-focused-tests.md
conflicts: []
frozen_pages_used: []
```

父 Agent 必须亲自读取其决定采用的页面，不能只依据 Librarian 摘要执行高影响操作。

### 8.9 不设置固定读取预算

Mnemosyne 不设置固定页面数量、固定 Token 数量或固定索引层数限制。复杂任务可以根据需要继续展开记忆，避免在接近正确知识时因硬预算被迫停止。

使用行为停止条件：

```text
已经找到满足当前任务适用条件的直接记忆
AND 没有未解决冲突
AND 已有足够信息说明为何适用
→ 停止检索并返回父 Agent
```

以下保护仍然保留，但不属于记忆读取预算：

- Reasonix 自身的 `max_steps`；
- 重复页面检测；
- 路由循环检测；
- 断链检测；
- 已满足停止条件后禁止无目的浏览。

### 8.10 Prompt 约束的结构化回执

第一阶段不依赖 Reasonix 新增原生 MemoryUsage 事件。OMR 通过 Librarian Prompt 和父 Agent Prompt 要求 Reasonix 输出结构化回执。

Librarian 输出检索结果，证明“找到了哪些记忆”；父 Agent 输出使用回执，证明“实际采用了哪些记忆”。父 Agent 使用回执至少包含：

```json
{
  "schema_version": 1,
  "retrieval_id": "retrieval_123",
  "memory_usage": [
    {
      "memory_id": "mem_abc",
      "revision": 2,
      "stage": "affected",
      "application": "用于决定先检查资产来源再执行升级恢复"
    }
  ]
}
```

Prompt 必须明确：

- 没有实际采用时输出空数组；
- 不得因为页面被读取就伪造 `adopted` 或 `affected`；
- 回执不能包含完整模型思考、凭据或无必要的命令正文；
- 缺少合法回执时只能记录为 `read` 或 `unknown`，不能参与成功和失败计分；
- 后续方案 C 即使增加宿主事件，也继续兼容相同 Schema。

### 8.11 记忆优先级与同级排序

优先级不能替代语义相关性。Mnemosyne 采用“两阶段排序”：

```text
Mnemosyne Librarian 判断语义相关性和适用范围
        ↓
OMR 根据结构化状态与客观使用证据确定同类候选顺序
```

程序不分析任务与记忆的自然语言语义，也不维护可无限增长的相关性类别。Librarian 输出结构化候选和理由；OMR 只验证引用、Scope、状态、统计和排序协议。

最终排序采用以下词典序：

```text
1. 用户本次明确指定或排除
2. Librarian 给出的任务相关性顺序
3. applies_when / does_not_apply_when 是否匹配
4. Scope、Pinned 和 Lifecycle 层级
5. Health 层级
6. 当前 Revision + Context 中与 Usage Policy 匹配的证据强度
7. 对应证据覆盖的独立 Episode、Evidence 来源或 Project Family 数
8. 确定性随机 Tie Break
```

第 6～7 层按 Usage Policy 分流：

| Usage Policy | 证据强度 | 覆盖广度 |
|---|---|---|
| `outcome_attributed` | `counted_help_count` | 独立 Episode / Project Family 数 |
| `evidence_validated` | 通过 Critic 的独立正向 Evidence 数 | 独立 Root Task / 正式来源 / Project Family 数 |
| `explicit_confirmation` | Confirmation Source 当前是否有效且 Scope 匹配 | 不按确认次数刷权重；同级进入 Tie Break |

第 4 层在相关性和适用条件相近时固定为：

```text
项目 pinned
>
项目 active
>
全局 pinned
>
全局 active
>
项目 probation
>
全局 probation
```

Health 顺序固定为 `healthy > degraded`；`frozen` 已在候选过滤阶段排除。Pinned 只是人工优先提示，不能绕过用户本次要求、适用条件、项目冲突、安全规则或冻结协议，也不能使 Frozen Memory 重新进入正常索引。第一版不增加任意数字型人工优先级。

对 `outcome_attributed`，只有完成以下完整链路的使用才增加 `counted_help_count` 排序权重：

```text
retrieved
→ read
→ adopted
→ affected
→ evaluated
→ counted_as_help
```

必须同时满足：

- 使用属于当前 Memory Revision；
- Context Signature 与当前排序上下文匹配；
- 关联独立 Episode；
- Attribution 协议确认 `memory_effect = helped`；
- 同一 Episode 对同一 Memory Revision 和 Context 最多计数一次。

只被命中、读取或提及的记忆不增加优先级；任务成功但无法证明记忆实际影响执行时也不计数。跨 Context 统计只能作为弱参考，不能覆盖当前 Context 的直接证据。对 `outcome_attributed`，新 Revision 的 `counted_help_count` 从零开始，旧 Revision 的历史计数只用于审计和 Review。

完全同级时不使用不可复现的真随机。排序键固定为：

```text
tie_key = SHA256(retrieval_id + candidate_scope_generation_id + memory_id)
```

Project 候选使用 `project_generation`，Global 候选使用 `global_generation`。同一次 Retrieval 的顺序保持稳定并可复现，不同任务会自然轮换；测试可以固定 Retrieval ID 和两个 Scope Generation 得到稳定结果。

为避免旧记忆因历史 Policy Evidence 形成永久垄断，每次 Retrieval 可以额外返回最多一个高度相关、适用条件匹配的 Probation Memory：

```yaml
primary_candidates:
  - memory_id: mem_active_01
exploration_candidate:
  memory_id: mem_probation_02
```

观察候选不能覆盖主候选、用户指令或安全策略，不能强迫父 Agent 采用；它只提供读取和比较机会，采用后仍走正常 MemoryUsage 与 Attribution 协议。Mnemosyne 不设置硬读取预算，因此观察位不会挤掉主要记忆。

优先级在 Retrieval 时动态派生，至少包含：

```yaml
memory_id: mem_123
revision: 2
context_signature: ctx_go_macos
lifecycle: active
health: healthy
pinned: false
usage_policy: outcome_attributed
policy_evidence_strength: 8
policy_evidence_breadth: 7
tie_key: sha256_...
```

使用统计变化只更新可重建的 DerivedMemoryState 或派生 Generation，不增加 MemoryEvidenceGeneration 或知识 Revision，也不修改记忆正文。

OMR 永久不提供：

- 向量数据库；
- Embedding 索引；
- 相似度阈值检索；
- 向量召回兜底；
- 外部托管知识服务依赖。

## 9. 为什么永久不使用向量数据库

Mnemosyne 的长期记忆由 OMR 自身生成和治理，天然具有明确的类型、组件、操作、失败类别、证据和关系，不是无法预先组织的海量无结构语料。

文件与索引方案具有以下优势：

- 人类和模型可以直接阅读；
- Git Diff 清晰；
- 可审计、可签名、可回滚；
- 路由结果确定；
- 检索失败可定位到索引、别名、分类或链接问题；
- 无需维护 Embedding 模型和重建流程；
- 不存在向量与原文版本不一致的问题；
- 不增加外部服务和数据泄露面；
- OMR 可以通过改进索引本身持续提高检索质量。

如果模型没有找到已经存在的记忆，OMR 应修复：

- 根索引路由；
- 局部索引摘要；
- 页面别名；
- 页面分类；
- 页面路径；
- 交叉链接；
- 页面粒度。

不得以增加向量检索作为解决方案。

## 10. 自动写入与自动生效

### 10.1 自动记录

以下过程默认自动执行：

```text
任务完成
→ 写入 Episode
→ 更新 Pattern
→ 生成 Memory Candidate
→ Schema、证据、安全、冲突检查
→ MemoryMutationPlan
   ├── no_change：只保留事实，Wiki 零修改
   └── create/revise/...：写入规范 Revision
→ 编译新 Generation、局部索引和根索引
→ 有可靠新知识时，新 Revision 进入 probation
```

用户不需要逐条批准记忆。

### 10.2 自动记忆不等于自动 Overlay

两者必须保持区别：

| 类型 | 影响范围 | 默认策略 |
|---|---|---|
| OKF 记忆 | 仅在相关任务按需读取 | 有可靠新知识时自动进入 probation |
| Evolution Overlay | 每次任务都会进入 Prompt | 保持更严格 Gate 和回滚策略 |

本文取消的是“每条长期记忆都要人工批准”，不是取消 Overlay、安全配置和高影响策略的现有保护。

### 10.3 项目记忆自动泛化为全局记忆

原始项目记忆不能直接移动或复制到全局。OMR 必须保留所有项目记忆，通过脱敏、去项目化和多项目证据归纳创建具有新 Memory ID 的全局记忆：

```text
项目 A 记忆 ─┐
项目 B 记忆 ─┼→ 跨项目 Pattern → 全局 probation 记忆
项目 C 记忆 ─┘
```

全局泛化采用三阶段：

| Global Stage | 是否参与正常检索 | 含义 |
|---|:---:|---|
| `global_candidate` | ❌ | 已发现可能存在跨项目规律，但证据或验证尚不完整 |
| `global_probation` | ✅，全局最低优先级 | 已通过泛化门槛，允许在其他项目中受控验证 |
| `global_active` | ✅ | 已按自身 Usage Policy 在来源之外完成验证或获得明确全局授权 |

Global Stage 是全局泛化阶段，不替代通用的 Lifecycle、Health 和 Pinned 字段。进入 `global_probation` 时通用 Lifecycle 同样是 `probation`；进入 `global_active` 时通用 Lifecycle 才可以成为 `active`。

`global_candidate` 不是普通 Memory，也不使用普通 Lifecycle。它的规范事实模型为 `GlobalPromotionCandidate`，保存在 `facts/promotion-candidates/<candidate-id>.json`：

```json
{
  "schema_version": 1,
  "candidate_id": "promotion_01K...",
  "status": "collecting",
  "usage_policy": "outcome_attributed",
  "source_memory_refs": [
    {"scope":"project","memory_type":"strategy","memory_id":"mem_a","revision":2}
  ],
  "source_project_family_fingerprints": ["hmac_..."],
  "outcome_refs": ["outcome_..."],
  "evidence_refs": [],
  "confirmation_source_ref": null,
  "critic_judgment_refs": [
    {
      "scope": "global",
      "judgment_type": "generalization_critic",
      "judgment_id": "judgment_...",
      "content_sha256": "sha256_..."
    }
  ],
  "proposed_applies_when": [],
  "proposed_does_not_apply_when": [],
  "content_sha256": "sha256_..."
}
```

Candidate 只保存结构化来源和候选边界，不生成正常 Wiki 页面、不进入索引。`usage_policy` 决定合法的 Promotion Evidence：`outcome_attributed` 使用 Outcome Ref，`evidence_validated` 使用 EvidenceRef，`explicit_confirmation` 使用 Confirmation Source Ref；不匹配 Policy 的字段必须为空，禁止借用另一策略的晋升证据。通过全部 Gate 后才创建具有新 Memory ID、`lifecycle=probation` 和 `global_stage=global_probation` 的正式全局 Memory Revision。

所有 Policy 进入 `global_probation` 都必须满足以下公共门槛：

```text
至少来自 3 个独立 Project Family
AND 单个 Project Family 的证据不超过全部证据的 50%
AND 不包含项目路径、业务术语或敏感信息
AND 适用条件能够结构化表达
AND 不与现有全局 active/pinned 记忆发生未解决冲突
AND 通过独立 Generalization Critic
```

`explicit_confirmation` 是例外：一个可验证且明确声明“跨项目/全局适用”的用户或正式决策来源，可以替代三个 Project Family 门槛；未声明适用 Scope 的普通项目确认不能提升为全局。

Global Promotion × Usage Policy 矩阵固定为：

| Usage Policy | `global_candidate → global_probation` | `global_probation → global_active` | 禁止借用 |
|---|---|---|---|
| `outcome_attributed` | 公共门槛 + 至少 5 次 `counted_as_help` | 来源之外至少 2 个 Project Family、累计 3 次 `counted_as_help`、无未解决 `counted_as_harm` | Evidence 数量或 Confirmation 不能代替 help/harm |
| `evidence_validated` | 公共门槛 + 每个来源 Family 均有独立 EvidenceRef、无未解决反例、Evidence Critic 通过 | 来源之外至少 2 个 Project Family 提供累计 3 个独立验证 EvidenceRef、无未解决反证、Critic 通过 | help/harm 或 Confirmation 不能代替证据验证 |
| `explicit_confirmation` | Confirmation Source 可验证并明确声明全局 Scope，独立 Critic 确认范围无歧义 | 同一正式来源明确授权全局激活且授权仍有效；撤销或 Scope 冲突时不得激活 | help/harm 或普通项目确认不能代替全局确认 |

达到门槛后无需人工逐条批准，自动创建新的 `global/probation` 记忆。证据不足时只能保留为项目记忆或 `global_candidate`，不能提前伪装成通用经验。

来源证据只负责进入观察期，不能循环证明自身已经稳定。晋升 `global_active` 必须按上表使用来源之外的验证或显式全局授权，三类 Policy 不能统一降级为 `counted_as_help`。

项目记忆和全局记忆分别保留独立 Memory ID、Revision、Lifecycle、Health、Evidence Generation 和使用统计。全局记忆通过 `generalized_from` 引用来源项目记忆；全局修订不得反向覆盖项目事实，项目记忆也不会因为全局晋升而被淘汰。

全局记忆必须额外记录：

- 来源项目数量，使用不可逆项目指纹而不是路径；
- 按 Usage Policy 分离的 `counted_help_count/counted_harm_count`、独立 Evidence 计数或 Confirmation 状态；
- 适用条件与明确反例；
- 泛化来源关系；
- 是否存在项目特例；
- 最近一次跨项目验证时间。

全局记忆必须结构化记录 `applies_when` 和 `does_not_apply_when`。无法表达适用条件或明确反例时，只能停留在 `global_candidate`。

语义泛化由只读 Mnemosyne Generalizer 完成，并由独立 Generalization Critic 复核。程序只负责统计 Project Family、验证证据 Hash、Revision、Scope、阈值、敏感字段和事务边界；不得在 Go 代码中穷举跨项目语义类别。Reasonix 只能输出严格结构化的 `GlobalMemoryMutationPlan`，最终写入仍由 OMR Memory Service 执行。

## 11. 记忆生命周期

Mnemosyne 将生命周期、健康状态和人工优先标记拆成三个独立维度，避免一个 `status` 同时承担互相冲突的语义。

### 11.1 生命周期

```yaml
lifecycle: active
```

| Lifecycle | 含义 | 正常任务默认读取 |
|---|---|:---:|
| `probation` | 新生成或刚修订，处于观察期 | ✅，低优先级 |
| `active` | 已按当前 Usage Policy 获得稳定有效证据 | ✅ |
| `frozen` | 当前 Revision 达到冻结条件 | ❌ |
| `superseded` | 已被新记忆或新结构替代 | ❌ |
| `archived` | 长期归档 | ❌ |

### 11.2 健康状态

```yaml
health: degraded
```

| Health | 含义 |
|---|---|
| `healthy` | 当前 Context 没有达到降级条件 |
| `degraded` | 存在经过验证的负面使用证据，但未达到冻结条件 |

### 11.3 人工优先标记

```yaml
pinned: true
```

`pinned` 只影响检索优先级，不改变 Lifecycle 或 Health，也不能阻止安全冻结。一条记忆可以同时是 `active + degraded + pinned`。

禁止使用会形成永久正确性结论的 `bad` 或 `invalid` 状态。

### 11.4 Governance Event

Pin、Unpin、Manual Freeze、Unfreeze 和 Archive 不属于知识 Mutation，也不能直接改写派生状态。它们使用统一的追加式规范事实：

```json
{
  "schema_version": 1,
  "event_id": "governance_01K...",
  "scope": "project",
  "memory_id": "mem_01K7A9X2",
  "revision": 2,
  "operation": "pin",
  "reason": "user requested priority",
  "source": "user",
  "basis_refs": [],
  "created_at": "2026-08-07T00:00:00Z"
}
```

`operation` 固定为：

```text
pin
unpin
manual_freeze
unfreeze
archive
```

Governance Event 不修改 MemoryRevision、MemoryEvidenceGeneration、MemoryMutation 或历史 Outcome。Pinned、Archived 和人工冻结结果由事件顺序、目标 Revision 及其他规范事实确定性派生。

`unfreeze` 不是无条件状态赋值。其 `basis_refs` 必须使用正式 JudgmentRef、EvidenceRef 或 MemoryRef，引用 Attribution Override Judgment、新 Evidence Generation 中的证据或新 Revision；这些事实已使冻结条件不再成立，服务才允许记录 `unfreeze`，否则返回稳定拒绝且零写入。新 Revision 本身从 `probation/healthy` 开始，不需要为旧 Revision 伪造解冻。`manual_freeze` 可以施加更严格的人工隔离，但 `unfreeze` 不能抹掉仍满足自动冻结阈值的证据。

### 11.5 状态转换总表

| Usage Policy | 初始状态 | 晋升 Active | 进入 Degraded | 冻结 | 恢复 |
|---|---|---|---|---|---|
| `outcome_attributed` | probation/healthy | 唯一计分协议下至少 3 次 `counted_as_help`、至少 2 个独立 Episode、无未解决 `counted_as_harm` | 首次 `counted_as_harm` | 当前 Revision/Context 至少 3 次 `counted_as_harm` 且 negative rate ≥ 60% | 新 Revision 或满足滞回恢复条件后回到 probation/healthy |
| `evidence_validated` | probation/healthy | 至少 3 个独立 EvidenceRef、至少 2 个 Root Task/正式来源、无未解决冲突、Critic 通过 | 出现经过复核的反例或来源不一致 | 至少 3 个独立反证、至少 2 个 Root Task/正式来源、Critic 支持 | Revise/Split 后新 Revision 进入 probation/healthy |
| `explicit_confirmation` | probation/healthy | `confirmation_source_ref` 指向的 Judgment 可验证 | 确认 Judgment 暂时不可验证或存在冲突 | 确认 Judgment 明确撤销且无替代 Revision | 新 Confirmation Judgment 产生新 Revision 并进入 probation/healthy |

通用转换约束：

- Revise、Split 或 Merge 产生的新 Revision 一律从 `probation/healthy` 开始；
- `superseded` 和 `archived` 不自动返回正常检索；
- Pinned 不改变任何转换条件；
- Frozen Revision 永久保留；恢复必须由 Attribution Override、新 Evidence Generation 或新 Revision 改变派生结论，并通过显式恢复事务记录 Governance Event；
- 不同 Usage Policy 的证据和阈值不能混用。

## 12. MemoryUsage 使用回执

不能因为某个任务失败，就惩罚该任务读取过的所有记忆。OMR 必须记录记忆在执行中的实际作用。

### 12.1 使用阶段

```text
retrieved  → 索引命中
read       → 模型读取页面
adopted    → 模型明确采用策略
affected   → 策略实际影响执行步骤
evaluated  → 最终结果完成归因
```

### 12.2 数据示例

```json
{
  "schema_version": 1,
  "retrieval_id": "retrieval_123",
  "root_task_id": "task_root_007",
  "project_generation": "gen_project_000013",
  "global_generation": "gen_global_000021",
  "memory_id": "mem_01K7A9X2",
  "memory_revision": 2,
  "episode_id": "episode_019",
  "context_signature_version": 1,
  "context_signature": "sha256_...",
  "context_descriptor_ref": "context_01K...",
  "stage": "affected",
  "application": "用于决定先检查资产来源再执行升级恢复",
  "created_at": "2026-08-07T00:00:00Z"
}
```

`retrieved` 和 `read` 只能用于衡量检索质量，不能用于记忆正确性判定。缺少合法回执时，结果不得自动提升为 `adopted` 或 `affected`。

## 13. 结果归因与生命周期计算

> 首版协议暂定：程序只记录客观事实、编排分析并验证结构；开放式失败语义和记忆影响由 Reasonix 判断。真实数据证明规则需要调整时，可以通过版本化协议修订，但不能在 Go 代码中不断增加失败语义枚举。

### 13.1 程序只记录客观事实

OMR 可以保存：

- Exit Code；
- HTTP Status；
- Tool 名称和时间；
- Reasonix 公开事件；
- Test、Build、Lint 和 Review 结果引用；
- 脱敏 stderr/stdout 引用；
- 用户取消和任务完成事实；
- MemoryUsage 和证据时间顺序。

程序不得把 `HTTP 429`、`exit 1` 或某个字符串直接解释成固定失败语义。类似 `rate_limit`、`authentication`、`network` 的开放分类不进入 Go Enum。

### 13.2 Failure Concept 也是记忆

失败种类保存在 Mnemosyne OKF 中，而不是写死在程序：

```text
wiki/failure-concepts/
├── index.md
├── provider-rate-limit.md
├── invalid-authentication.md
├── external-service-unavailable.md
└── incorrect-recovery-strategy.md
```

示例：

```yaml
---
type: failure_concept
memory_id: mem_failure_123
canonical_key: provider-rate-limit
title: Provider Rate Limit
lifecycle: probation
health: healthy
usage_policy: evidence_validated
aliases:
  - HTTP 429
  - too many requests
relations:
  - predicate: specializes
    target:
      scope: global
      memory_type: failure_concept
      memory_id: mem_failure_external_provider
      revision: 1
---
```

Reasonix 必须先读取 `failure-concepts/index.md`，优先引用已有 Failure Concept。确实没有匹配概念时，通过同一个有界 `MemoryMutationPlan` 创建新的 `probation` 概念；后续可以 Merge、Split、Revise、Generalize 或 Freeze。

### 13.3 Attribution Analyst

专用只读 `Mnemosyne Attribution Analyst` 接收：

- 任务目标；
- 脱敏客观事实；
- MemoryUsage；
- 被采用的记忆 Revision；
- Test、Review 和 Tool Evidence；
- Failure Concept Index。

它输出严格结构化结果：

```json
{
  "schema_version": 1,
  "failure": {
    "cause_memory_id": "mem_failure_123",
    "summary": "上游模型服务拒绝了请求",
    "external": true
  },
  "memory": {
    "effect": "neutral",
    "attribution": "likely",
    "reason_code": "failure_before_memory_action"
  },
  "evidence_refs": [
    "event_12",
    "tool_result_7"
  ]
}
```

失败原因通过 `cause_memory_id` 引用可演化的 Failure Concept，不使用程序内固定失败枚举。

### 13.4 最小控制状态

程序只保留用于状态机计算的最小封闭协议：

```text
memory_effect:
  helped
  neutral
  harmed
  unknown

attribution:
  confirmed
  likely
  uncertain
```

这些字段不描述无限变化的失败类型，只表示记忆对结果的方向和证据可信程度。

### 13.5 Harmed 独立复核

执行任务的父 Agent 不得单独给自己判定负面结果。

```text
Attribution Analyst 输出 harmed
→ 启动只读 Mnemosyne Attribution Critic
→ Critic 检查证据、因果顺序、第三方原因和 MemoryUsage
```

只有：

```text
Analyst = harmed
AND Critic = supported
```

才产生负面使用证据。两者不一致时统一记录为 `unknown`，不参与正负计分。即将触发 Frozen 前，Critic 必须再次复核参与阈值计算的所有 Harmed Outcome。

### 13.6 OMR Attribution Gate

程序不解释失败语义，只验证：

- JSON 和 Schema 合法；
- Evidence Ref 存在；
- 引用的 Memory、Revision 和 Failure Concept 存在；
- Evidence 时间顺序成立；
- 参与正负计分的 MemoryUsage 必须达到 `affected` 并完成 `evaluated`；
- Analyst/Critic 结果满足协议；
- Scope 没有越界；
- 输出不包含敏感信息；
- `uncertain` 和冲突结果不参与计分。

验证失败统一降级为：

```text
memory_effect = unknown
counted_as_help = false
counted_as_harm = false
```

唯一正负计分协议固定为：

```text
help =
  memory_stage == affected
  AND evaluated == true
  AND memory_effect == helped
  AND attribution == confirmed

harm =
  memory_stage == affected
  AND evaluated == true
  AND memory_effect == harmed
  AND attribution == confirmed
  AND critic == supported
```

`adopted`、`likely`、`uncertain`、`neutral` 和 `unknown` 只保存为事实或观察，不计入正负分。其他章节、排序和实现不得定义第二套计分条件。

### 13.7 MemoryOutcome

最终事实记录示例：

```json
{
  "schema_version": 1,
  "memory_id": "mem_01K7A9X2",
  "revision": 2,
  "episode_id": "episode_019",
  "root_task_id": "task_root_007",
  "context_signature_version": 1,
  "context_signature": "sha256_...",
  "context_descriptor_ref": "context_01K...",
  "task_outcome": "failure",
  "failure_cause_memory_id": "mem_failure_123",
  "memory_stage": "affected",
  "evaluated": true,
  "memory_effect": "neutral",
  "attribution": "confirmed",
  "critic": "not_required",
  "evidence_refs": ["event_12", "tool_result_7"],
  "counted_as_help": false,
  "counted_as_harm": false
}
```

### 13.8 Context Signature 与独立 Episode

Context Signature 是统计、归因和排序的核心维度，必须由程序根据版本化的结构化 Context Descriptor 确定性生成，禁止模型自由输出最终 Hash。

不可变事实保存在：

```text
facts/context-descriptors/<context-descriptor-id>.json
```

基础结构：

```json
{
  "schema_version": 1,
  "context_descriptor_id": "context_01K...",
  "context_signature_version": 1,
  "component_refs": ["mem_component_installer"],
  "operation_refs": ["op_upgrade"],
  "task_class_refs": ["mem_task_installation_recovery"],
  "environment": {
    "os": "darwin",
    "arch": "arm64",
    "language": "go",
    "framework": "",
    "tool": "omr"
  },
  "canonical_sha256": "sha256_..."
}
```

生成算法：

```text
context_signature = SHA256(
  canonical_json({
    context_signature_version,
    component_refs,
    operation_refs,
    task_class_refs,
    relevant_environment_fields
  })
)
```

Canonical JSON 必须固定字段顺序、数组去重与排序、空值表达、字符编码和 Hash 版本。Context Descriptor 不包含项目绝对路径、Project Scope ID、凭据或自由文本模型思考。算法升级时增加 `context_signature_version`，旧 Descriptor 和旧 Hash 永久保留、可解释、可重算。

独立 Episode 用于防止同一 Root Task 的自动重试、恢复执行或重复回执刷晋升和冻结次数：

```text
independent_episode_key = SHA256(
  root_task_id
  + memory_id
  + revision
  + context_signature
)
```

同一 Root Task 下相同 Memory Revision/Context 的多个 attempt 最多贡献一次统计结果。每个 attempt 仍完整保留为证据；最终计分使用该 Root Task 的确定性聚合结果，不能选择性只取成功或失败 attempt。

### 13.9 正负证据计算

按以下维度分别统计：

```text
memory_id
+ revision
+ context_signature
```

只有满足 13.6 唯一计分协议并写入 `counted_as_help=true` 的 Outcome 进入正面计数；只有写入 `counted_as_harm=true` 的 Outcome 进入负面计数。`adopted`、`likely`、`neutral`、`unknown` 以及未完成 `evaluated` 的使用不进入分母。

```text
negative_rate = counted_harm_count / (counted_help_count + counted_harm_count)
```

统计按 `independent_episode_key` 去重。成功不会删除失败历史，但会参与当前健康状态和冻结判断。

### 13.10 晋升、降级与恢复

新记忆或新 Revision默认：

```text
lifecycle = probation
health = healthy
```

`outcome_attributed` 晋升条件：

```text
至少 3 次 counted_as_help
AND 来自至少 2 个独立 Episode
AND 没有未解决负面 Outcome
→ lifecycle = active
```

`outcome_attributed` 出现一次 `counted_as_harm`：

```text
health = degraded
→ 触发 MemoryMutationPlan 分析
```

恢复健康：

```text
相同 Revision 和 Context 最近连续 3 次为 counted_as_help
AND negative_rate < 40%
→ health = healthy
```

`evidence_validated` 不使用 helped/harmed 晋升。其晋升条件固定为：

```text
至少 3 个独立 EvidenceRef
AND 覆盖至少 2 个独立 Root Task 或正式来源
AND 没有未解决 contradicts
AND Evidence Gate 与独立 Critic 通过
→ lifecycle = active
```

出现经过复核的反例或来源不一致时进入 `health=degraded` 并触发 Revise、Split 或 Review；描述性知识不会因为没有影响任务动作而永久停留在 probation。

`explicit_confirmation` 只有在 `confirmation_source_ref` 指向的 Confirmation Judgment 可验证、Hash 匹配且 Scope 合法时才能进入 `active`。确认缺失时保持 `probation`；新 Judgment 撤销确认且没有替代 Revision 时进入 `frozen` 等待 Review，有替代项时进入 `superseded`。它不使用任务 helped/harmed 计分。

### 13.11 冻结阈值

`outcome_attributed` 当前 Revision 和 Context 同时满足：

```text
counted_harm_count >= 3
AND negative_rate >= 60%
AND 所有参与阈值的 harmed 已由 Critic 复核
→ lifecycle = frozen
```

40%～60% 的滞回区间保持 `degraded`，继续观察，避免一次成功或失败导致状态反复抖动。

任务失败但记忆影响为 `neutral/unknown` 时，不惩罚记忆。第三方原因本身也可能被错误记忆放大，因此 `failure_cause` 和 `memory_effect` 必须始终分开记录。

`evidence_validated` 只有在当前 Revision 的核心陈述被至少 3 个独立、经过 Critic 复核的反证推翻，且反证覆盖至少 2 个独立 Root Task 或正式来源时才能冻结。`explicit_confirmation` 按确认来源撤销协议处理。三类 Usage Policy 不得互相借用计分阈值。

`outcome_attributed` 全局记忆在单个项目中出现 `harmed` 时，优先判断：

```text
适用条件不匹配 → 创建项目特例或 specialize 记忆
适用条件遗漏   → revise 全局记忆并补充边界
记忆本身有害   → 计入 global harmed
```

单个项目不能独自冻结全局记忆。全局冻结同样按 Usage Policy 分流：`outcome_attributed` 除满足负面比例外，还必须至少在 2 个独立 Project Family 中出现 `counted_as_harm`；`evidence_validated` 必须至少在 2 个独立 Project Family 中出现经过 Critic 复核的核心反证；`explicit_confirmation` 必须由可验证的全局 Confirmation Source 撤销或以更高优先级正式来源取代。这样可以避免特殊项目、第三方环境异常或错误证据类型污染全局判断。

### 13.12 冻结与修复

阈值命中后：

```text
当前 Revision 标记 frozen
→ 从正常索引移除
→ 加入 frozen/index.md
→ 保留页面、Outcome、证据和历史
→ 创建 Memory Review / Repair 任务
```

Repair 流程可以显式读取 Frozen Memory，生成 Revise、Split、Specialize 或 Generalize。新 Revision 重新进入 `probation/healthy`，旧冻结 Revision 永久保留。

Pinned 不能阻止自动冻结。人工请求解冻时，Memory Service 必须先根据 Attribution Override、新 Evidence Generation 或新 Revision 重算；仍满足冻结条件时拒绝并保持 `frozen`，不允许通过 Governance Event 绕过证据。

### 13.13 人工归因修正

Web 和 CLI 允许用户修正归因，但不得覆盖原记录，只能追加 Override：

```json
{
  "schema_version": 1,
  "judgment_id": "judgment_01K...",
  "judgment_type": "attribution_override",
  "scope": "project",
  "subject": {"subject_type":"memory_outcome","outcome_id":"outcome_01K..."},
  "source": {"source_type":"user","source_id":"local_user"},
  "attribution_override": {
    "previous_effect": "harmed",
    "new_effect": "neutral",
    "reason": "失败由不可用的第三方服务导致"
  },
  "supersedes_judgment_ref": null,
  "basis_refs": [],
  "content_sha256": "sha256_...",
  "created_at": "2026-08-07T00:00:00Z"
}
```

人工修正必须写入 `facts/judgments/<judgment-id>.json` 并通过 JudgmentRef 参与派生。修正后重新计算 Health 和 Lifecycle，但原始 Analyst、Critic、Outcome 和程序验证结果全部保留。

## 14. 冻结记忆读取规则

冻结记忆默认不给模型，正常任务不得进入 `wiki/frozen/`。

冻结隔离采用协议级隔离，不依赖操作系统权限封锁：

- 正常根索引和局部索引不列出 Frozen Memory；
- `omr memory context` 默认排除 Frozen Memory；
- Librarian Prompt 禁止主动搜索或读取 `wiki/frozen/`；
- `omr memory get <id>` 默认拒绝返回 Frozen Memory；
- 只有显式 Memory Review 流程才能使用 `--include-frozen`；
- 即使项目文件工具技术上能够访问冻结目录，正常任务协议也不得提供或主动发现其路径。

显式读取入口建议为：

```bash
omr memory context \
  --project-dir . \
  --include-frozen \
  --purpose review
```

只有以下场景可以显式读取冻结记忆：

- 用户要求 Review 历史记忆；
- OMR 执行记忆修复任务；
- 新旧策略对比；
- 多条经验泛化；
- 用户在 Web 页面选择；
- 当前活跃知识没有命中，且用户或受控流程明确允许查阅冻结区。

即使读取冻结记忆，也必须向模型标记：

```text
该记忆已冻结，不得直接作为当前任务规则；
只可用于对比、归因、修订或泛化。
```

## 15. 修订链和泛化

### 15.1 禁止静默覆盖

错误或不完整记忆不能原地静默重写。每次修订必须产生版本关系：

```text
strategy@1
   ↓ repaired_by
strategy@2
   ↓ generalized_from
strategy@3
```

### 15.2 恢复方式

支持三种恢复：

1. 同 Revision 恢复：在 Attribution Override 或新 MemoryEvidenceGeneration 使冻结条件不再成立后，通过 Governance Event 恢复至 `probation`；
2. 修改范围后恢复：创建新 revision，旧版本保持 frozen；
3. 与其他记忆合并：创建 generalized memory，原记忆继续保留。

恢复后的记忆必须重新进入 `probation`，不能直接成为 `active`。

### 15.3 泛化示例

```yaml
---
type: strategy
memory_id: mem_generalized_123
canonical_key: diagnose-before-rebuilding-manifest
lifecycle: probation
health: healthy
pinned: false
usage_policy: outcome_attributed
relations:
  - predicate: generalized_from
    target:
      scope: project
      memory_type: strategy
      memory_id: mem_verify_upgrade
      revision: 2
  - predicate: generalized_from
    target:
      scope: project
      memory_type: strategy
      memory_id: mem_inspect_asset_source
      revision: 1
evidence:
  - scope: project
    evidence_type: episode
    evidence_id: episode_019
    content_sha256: sha256_...
---
```

矛盾如何解决记录在对应 Memory Mutation 和审计事实中，不作为自由文本知识关系写入页面。

## 16. 索引自身的进化

OMR 不仅进化知识内容，也需要进化“如何找到知识”。

以下信号触发 Index Improvement：

- 已存在相关记忆但 Agent 未找到；
- 读取了过多无关页面；
- 命中了错误页面；
- 用户手工指出正确页面；
- 同一概念存在重复页面；
- 页面过大导致读取成本过高；
- 别名、摘要或分类与实际查询不匹配。

允许的索引修复：

- 增加或修正 alias；
- 修改一句话摘要；
- 增加交叉链接；
- 调整目录归属；
- 拆分页面；
- 合并重复页面；
- 修复断链；
- 重建根索引和局部索引。

索引修改必须可从 Wiki 页面重新构建和校验。

## 17. 事务一致性与崩溃恢复

Mnemosyne 的一次记忆变更可能同时影响事实记录、记忆正文、关系、局部索引、根索引、生命周期状态和审计记录。单个文件的原子写入不能保证这些文件作为一个整体一致，因此禁止直接在当前 Wiki 上逐文件原地修改。

正式采用以下事务模型：

```text
不可变 Generation
        +
原子 CURRENT 切换
        +
每 Scope 单写锁
        +
Generation CAS
```

### 17.1 事实与派生记忆的边界

`facts/` 保存已经发生且完成脱敏、Schema 校验的客观事实。事实使用不可变记录或追加写，成功落盘后不得因为后续 Wiki 编译、索引生成或 Prompt 更新失败而删除。

`memory/generations/<generation-id>/` 保存从事实层编译得到的完整派生状态，包括：

- 记忆正文；
- OKF 根索引和局部索引；
- 显式知识关系；
- 生命周期和健康状态的派生视图；
- 编译输入 Hash、输出 Hash 和 Generation 元数据。

如果事实已经记录但派生编译失败，该事实进入 `pending_compile`，由后续 Repair 或下一次编译幂等处理。系统不得把真实发生过的 Episode 伪装成不存在。

规范事实分为两类：

- Episode、Observation、MemoryUsage 等已经客观发生的执行事实，校验后可以独立落盘，即使知识编译失败也永久保留；
- MemoryRevision、MemoryEvidenceGeneration、Governance Event 和 MemoryMutation 等会改变知识视图的事务事实，必须携带 `transaction_id`，在事务提交前保持隔离，不能被其他编译、检索或统计流程提前采用。

核心不变量：

> `CURRENT` 切换前，构建目标 Generation 所需的全部规范事实必须已经安全落盘并通过 Hash 校验。staging 只能从已落盘事实编译；`CURRENT` 切换后只能追加审计或提交标记，不能再创建重建目标 Generation 所必需的事实。

### 17.2 Generation 与唯一提交点

每个 Generation 发布后不可修改。`memory/CURRENT` 是机器读取当前有效 Generation 的唯一提交点：

```text
CURRENT = gen_000012
        ↓
构建并验证 gen_000013.staging
        ↓
发布不可变 gen_000013
        ↓
原子写入 CURRENT = gen_000013
```

在 `CURRENT` 切换前，所有读取继续使用旧 Generation；切换成功后，新读取才使用新 Generation。发布目录本身不是生效信号，孤立但完整的 Generation 不会自动参与检索。

稳定入口 `memory/index.md` 是面向人和模型的轻量路由页，不是真实提交点。它应指向当前 Generation，且可以由 `CURRENT` 确定性重建；两者不一致时以 `CURRENT` 为准并由 Doctor 报告和修复。

### 17.3 Generation Input Manifest

`generation.json` 中的 `input_hash` 只能验证输入集合，不能永久说明“具体采用了哪些事实”。每个 Generation 必须额外拥有一个长期保留、永久不可变的规范事实：

```text
facts/generation-input-manifests/<generation-id>.json
```

最小 Schema：

```json
{
  "schema_version": 1,
  "generation_id": "gen_000013",
  "scope": "project",
  "base_generation": "gen_000012",
  "compiler_version": "mnemosyne-compiler/1",
  "canonicalization_version": 1,
  "inputs": [
    {
      "fact_type": "memory_revision",
      "fact_id": "mem_abc@2",
      "fact_schema_version": 1,
      "content_sha256": "sha256_..."
    },
    {
      "fact_type": "memory_evidence_generation",
      "fact_id": "mem_abc@2:evidence@3",
      "fact_schema_version": 1,
      "content_sha256": "sha256_..."
    }
  ],
  "input_manifest_sha256": "sha256_...",
  "output_sha256": "sha256_...",
  "transaction_id": "tx_...",
  "created_at": "..."
}
```

`inputs` 必须完整列出该 Generation 实际采用的 MemoryRevision、MemoryEvidenceGeneration、Governance Event、MemoryUsage、Outcome、Judgment、Relation/Promotion 事实及其他规范输入。Confirmation 和 Attribution Override 都通过 Judgment Fact 纳入，不再使用无 ID/Hash 的旁路对象。条目按 `fact_type + fact_id` 去重并确定性排序，Hash 由程序读取规范事实后计算。

Manifest 不保存绝对路径、Prompt、模型思考、凭据或自由文本事实副本。它只保存稳定 Fact ID、类型、Schema Version 和内容 Hash。`input_manifest_sha256` 覆盖规范化后的完整输入条目与版本字段；`output_sha256` 必须和目标 Generation 的输出 Hash 一致。

Generation Input Manifest 必须在 staging 验证完成后、`CURRENT` 切换前安全落盘。Generation 目录可以按保留策略清理，但对应 Manifest 永久保留。精确重建必须读取 Manifest 指定的事实集合并使用指定的 Compiler、Canonicalization 和 Fact Schema Version；所需历史版本不可用时，Doctor 必须报告阻塞性 `memory_compiler_version_unavailable`，不得使用当前算法伪造相同快照。

### 17.4 写事务流程

所有 CLI、Web 和后台自动进化写操作必须调用同一个 Memory Service，禁止绕过服务直接修改事实、Generation、索引或状态文件。

一次写事务固定执行：

1. 对输入事实执行 Schema、脱敏、路径和敏感内容检查；
2. 原子保存已经客观发生的执行事实，并生成稳定幂等键；
3. 获取目标 Scope 的单写锁；
4. 读取 `CURRENT`，记录 `base_generation` 和对应 Hash；
5. 创建 `transactions/<transaction-id>.json`，状态为 `prepared`；
6. 知识变化在内存中生成并校验有界 `MemoryMutationPlan`；人工治理操作则校验固定枚举的 Governance Command，二者不能互相冒充；
7. Memory Service 读取规范输入并计算 before/after Hash：知识变化生成目标 MemoryRevision、MemoryEvidenceGeneration 和 MemoryMutation Fact，治理操作生成 Governance Event；
8. 将目标事务事实原子写入各自 `facts/` 路径；每条记录携带 `transaction_id`，并由 prepared manifest 固定文件路径与 Hash；
9. 确认 prepared manifest 中全部规范事实已安全落盘；未提交事实对其他事务和读取者不可见；
10. 仅使用当前已提交事实加本事务 prepared manifest，在 `<next-generation>.staging/` 中生成完整派生状态和 Wiki；
11. 对 staging 执行 Schema、Hash、断链、索引泄漏、冻结隔离和安全检查；
12. 根据 staging 实际输入集合与输出 Hash 创建不可变 Generation Input Manifest，安全写入 `facts/generation-input-manifests/` 并校验 Manifest Hash；
13. 将 staging 原子发布为不可变正式 Generation；
14. 再次确认 `CURRENT == base_generation`，否则以稳定冲突终止；
15. 原子更新 `CURRENT`，这是事务的唯一生效提交点；
16. 追加事务 `committed` 审计标记并刷新稳定路由页；该步骤不得产生目标 Generation 重建所需的新事实；
17. 释放写锁，遗留临时数据由维护流程清理。

prepared 事务事实不能仅凭文件存在而生效。编译器只接受：已在提交链中的事实，或当前持锁事务 prepared manifest 精确列出的事实。CAS 冲突、验证失败或未切换 `CURRENT` 的事务事实保持隔离，由 Repair 根据事务记录确定重试或标记 aborted，不能被下一次普通编译偷偷吸收。

事务记录至少包含：

```json
{
  "schema_version": 1,
  "transaction_id": "tx_...",
  "idempotency_key": "...",
  "scope": "project",
  "base_generation": "gen_000012",
  "target_generation": "gen_000013",
  "status": "prepared",
  "prepared_fact_manifest_sha256": "...",
  "generation_input_manifest_sha256": "...",
  "affected_memory_ids": ["mem_..."],
  "input_hash": "...",
  "output_hash": "...",
  "created_at": "...",
  "committed_at": null
}
```

### 17.5 写锁与 Generation CAS

项目、全局和 portable Scope 使用彼此独立的单写锁。锁只约束写入；读取不需要锁，因为已发布的 Generation 不可变。

锁记录必须包含 owner、进程身份、创建时间、租约或等价的失效证据。清理过期锁时不能只依赖 PID，必须同时防止 PID 复用和误删仍存活 writer 的锁。

即使已经持有锁，提交前仍必须执行 Generation CAS：

```text
expected: gen_000012
actual:   gen_000013
```

不一致时返回稳定、可重试的冲突，不得覆盖新状态：

```json
{
  "code": "memory_generation_conflict",
  "expected": "gen_000012",
  "actual": "gen_000013",
  "retryable": true
}
```

幂等键必须在任何派生状态副作用前登记或与事务记录绑定。相同请求重复执行时返回既有事务结果；相同幂等键但输入 Hash 不同必须 fail closed。

### 17.6 读取快照一致性

`omr memory context` 在一次检索开始时分别只读取一次 Project 和 Global `CURRENT`，返回：

```yaml
retrieval_id: retrieval_123
project_generation: gen_project_000013
global_generation: gen_global_000021
project_indexes:
  - .reasonix/omr/evolution/memory/generations/gen_project_000013/wiki/index.md
global_indexes:
  - <omr-user-data>/evolution/memory/generations/gen_global_000021/wiki/index.md
```

Mnemosyne Librarian 和父 Agent 在同一任务中固定使用这两个 Scope Snapshot。即使任一 Scope 在后台生成了更新版本，当前任务也不切换，避免前半段读取旧记忆、后半段读取新关系。新任务重新获取两个 Scope 的最新 Generation。

Web 请求同样固定 Generation；刷新或开始新的操作才读取新 `CURRENT`。缓存键必须至少包含 Scope 和 Generation ID。

### 17.7 崩溃恢复

| 崩溃位置 | 恢复规则 |
|---|---|
| 事实落盘前 | 无变化 |
| 客观执行事实落盘后、知识事务前 | 保留事实并标记或推导为 `pending_compile` |
| prepared 事务事实全部落盘后、编译前 | 依据 prepared manifest 重建 staging；在事务提交前保持隔离 |
| prepared 事务事实部分落盘 | Hash/路径不满足 manifest，事务阻断并保持隔离，禁止普通编译采用 |
| staging 构建中 | 旧 Generation 继续有效；staging 可安全删除或重建 |
| staging 已验证、Generation Input Manifest 尚未落盘 | 根据 staging 的固定输入集合生成并校验 Manifest；不得切换 CURRENT |
| Generation Input Manifest 已落盘、正式 Generation 未发布 | 校验 Manifest、prepared facts 和 staging 后安全重试发布或 abort |
| 正式 Generation 发布后、CURRENT 切换前 | Manifest 必须已经存在；新 Generation 和 prepared 事实均不参与正常读取，可依据 CAS 安全重试或 abort |
| CURRENT 切换后、事务未标记 committed | 根据 CURRENT、Generation Input Manifest、prepared manifest 和 Hash 补全审计状态，不再创建规范事实 |
| 稳定 index.md 刷新失败 | 当前 Generation 仍有效；Doctor 从 CURRENT 重建路由页 |

恢复流程不得凭猜测修改事实。所有自动修复必须根据事务记录、`CURRENT`、Generation Hash 和完整性检查确定动作；无法确定时停止自动恢复并报告阻塞诊断。

### 17.8 跨 Scope 操作

项目记忆泛化或提升为全局记忆不使用跨 Scope 分布式事务，也不同时持有项目锁和全局锁：

1. 项目 Scope 独立提交事实和项目记忆；
2. 全局事务引用不可变的项目 Evidence ID、Memory ID、Revision 和 Hash；
3. 全局提交失败不回滚项目事实；
4. 相同提升请求可以通过幂等键重试。

这避免双锁死锁，并确保全局失败不会破坏已经成立的项目知识。

### 17.9 回滚

回滚是新的显式事务，不修改或删除任何历史 Generation：

```text
gen_000014 发生问题
        ↓
创建 rollback transaction
        ↓
CURRENT 切回 gen_000013
        ↓
后续基于 gen_000013 生成 gen_000015
```

Generation 编号不复用、不倒退。回滚必须记录操作者、原因、来源 Generation、目标 Generation、时间和 Hash。冻结记忆、事实、Outcome 和人工 Override 都继续保留。

### 17.10 Generation 保留与清理

记忆事实、Revision 和 Generation Input Manifest 不因 Generation 清理而删除。Generation 是可重建派生快照，维护流程可以清理冗余副本，但必须保留：

- 当前 Generation；
- 回滚链引用的 Generation；
- 人工固定的 Generation；
- 未完成事务涉及的 Generation；
- 审计或版本策略要求的最近 Generation。

每个被清理 Generation 对应的 `facts/generation-input-manifests/<generation-id>.json` 必须永久保留。清理前必须验证 Manifest 完整、全部 Fact ID/Hash 可解析、指定 Compiler/Canonicalization/Schema Version 可用，并以 Manifest 做一次确定性重建或等价验证；任一条件不满足时拒绝清理。

清理必须支持 dry-run、记录审计日志并在删除前确认该 Generation 可从 Manifest 和规范事实精确重建。不得把清理派生快照等同于删除记忆，也不得只凭 `input_hash` 宣称可重建。

## 18. CLI 控制面

建议命令：

```text
omr memory status
omr memory doctor
omr memory list
omr memory show <id>
omr memory history <id>
omr memory usage <id>

omr memory freeze <id>
omr memory unfreeze <id>
omr memory pin <id>
omr memory unpin <id>
omr memory archive <id>

omr memory compare <id-1> <id-2>
omr memory generalize <id-1> <id-2> [...]
omr memory repair <id>

omr memory index rebuild
omr memory index doctor
omr memory compile
```

人工命令必须：

- 支持 `--project-dir`；
- 支持稳定 JSON 输出；
- 写操作支持 `--dry-run`；
- 保留 Snapshot；
- 原子更新；
- 幂等；
- 记录审计日志；
- 不允许越过项目 Scope。

命令还应支持显式 Scope 过滤或目标，例如：

```text
omr memory list --scope project
omr memory list --scope global
omr memory show <id> --scope global
omr memory compare --scope project:<id> --scope global:<id>
omr memory promote-global <project-memory-id> --dry-run
```

全局写操作不得因为当前工作目录变化而误写到项目 Store；项目写操作不得修改全局 Store。

## 19. Web 管理与知识图谱

Mnemosyne Web 的目标命令：

```bash
omr memory serve --project-dir .
```

默认仅监听 `127.0.0.1`，不向局域网或公网暴露。

### 19.1 图谱视图

节点包括：

- Component；
- Episode；
- Pattern；
- Failure Concept；
- Strategy；
- Decision；
- Playbook；
- Preference。

边来自 frontmatter 的显式关系，不使用图数据库。

图谱必须支持按 `project`、`global`、`portable` 过滤，并以不同边框或分组显示 Scope。跨 Scope 的知识边使用正式 Relation Enum，例如 `generalized_from`、`specializes` 或 `contradicts`；导入来源以 Provenance 单独展示，不能把全局节点伪装成项目事实。

三维状态不得压缩成一个互相争夺的颜色。Lifecycle 使用节点主颜色：

| Lifecycle | 主颜色 |
|---|---|
| active | 绿色 |
| probation | 黄色 |
| frozen | 灰色 |
| superseded | 紫色 |
| archived | 深灰色 |

Health 使用独立 Badge：`healthy` 不额外强调，`degraded` 使用橙色 Badge。Pinned 使用独立 `📌` 标记。图谱必须能够同时表达 `active + degraded + pinned`，不能让任一维状态覆盖其他维度。

### 19.2 记忆列表

支持：

- 按类型、组件、失败分类和状态过滤；
- 搜索标题、摘要和 alias；
- 查看 Policy Evidence、Lifecycle、Health 和最近使用时间；
- 查看高价值、高风险和待 Review 记忆。

### 19.3 记忆详情

展示：

- 当前内容和适用范围；
- 来源证据；
- 使用记录；
- Policy Evidence 与归因记录；
- 修订链；
- 冻结原因；
- 相关和冲突记忆；
- Hash 和审计记录。

### 19.4 人工操作

允许用户：

- 冻结和恢复；
- 固定和取消固定；
- 编辑和创建新 revision；
- 对比新旧记忆；
- 合并、拆分和泛化；
- 回滚；
- 归档；
- 查看冻结区；
- 触发索引重建和 Doctor。

Web 页面不能绕过库级验证、Scope、安全检查和审计流程。

## 20. Doctor 与一致性检查

`omr memory doctor` 至少检查：

- `CURRENT` 是否存在并指向完整 Generation；
- Generation 元数据、输入 Hash 和输出 Hash 是否匹配；
- 每个有效或历史可重建 Generation 是否存在不可变 Generation Input Manifest；
- Manifest 的 Fact ID、Fact Schema Version、内容 Hash、确定性顺序和 `input_manifest_sha256` 是否匹配；
- Manifest 指定的 Compiler/Canonicalization Version 是否仍可用；
- 使用 Manifest 精确重建后的输出 Hash 是否与 `output_sha256` 一致；
- 是否存在长期残留的 staging、`prepared` 事务或过期写锁；
- 是否存在已落盘但尚未编译的事实；
- 稳定路由页是否与 `CURRENT` 一致；
- 回滚链引用的 Generation 是否仍然存在；
- Memory Revision 是否存在对应规范事实和合法 Hash；
- Memory Evidence Generation 是否单调、不可变，EvidenceRef 与集合 Hash 是否匹配；
- Judgment Fact 的类型联合、Scope、Subject、来源、Hash 和 supersedes 链是否有效；
- JudgmentRef/ConfirmationSourceRef 的 ID、类型、Scope 和 Hash 是否与事实一致；
- 所有 `usage_policy=explicit_confirmation` 的 Revision 是否携带合法 `confirmation_source_ref`；
- OKF Page 是否严格由 MemoryRevision、当前 MemoryEvidenceGeneration 和 DerivedMemoryState 组合编译；
- Governance Event 的操作、目标 Revision、来源和 basis_refs 是否有效；
- Memory Mutation 的 before/after Revision 与 Hash 是否能和 Revision 事实对齐；
- Mutation Replay 是否重现相同 Revision Hash；
- Context Signature 是否能从所引用 Descriptor 和算法版本重算；
- Type 与 Usage Policy 是否满足 Schema 允许矩阵；
- `counted_as_help/harm` 是否严格满足唯一计分协议；
- 同一 Root Task 是否重复贡献独立 Episode 计数；
- OKF frontmatter 是否可解析；
- 必填字段和类型；
- 页面 ID 与路径是否稳定；
- ID 是否重复；
- 状态是否合法；
- 关系目标是否存在；
- 是否存在断链；
- `index.md` 是否遗漏页面；
- frozen 页面是否泄漏进正常索引；
- EvidenceRef 指向的 `facts/` 记录是否存在；
- Hash 是否漂移；
- Scope 是否匹配；
- 项目记忆是否错误写入全局 Store；
- 全局记忆是否满足跨项目泛化证据；
- Global Promotion 是否按 Usage Policy 使用对应 Outcome、Evidence 或 Confirmation Source，且没有跨策略借用门槛；
- 全局页面是否泄露项目路径、业务术语或可识别项目名称；
- portable 包是否在未导入时进入正常检索；
- 是否包含敏感内容；
- 路径和 symlink 是否安全；
- 使用计数是否能从 MemoryUsage 重建；
- Lifecycle 和 Health 是否能从规范事实重建；
- Pinned、Archived 和人工冻结是否能从 Governance Event 与其他规范事实重建；
- Unfreeze 的 basis_refs 是否只引用合法 MemoryRef、EvidenceRef 或 JudgmentRef，且确实改变冻结派生结论；
- Relation Index、Root/Local Index 是否能从规范 Revision 重建；
- Wiki 和 Generation 是否能从规范事实层确定性重新编译；
- Web 派生视图或缓存是否与固定 Generation 一致；
- 是否存在无法从规范事实重建却被当成权威状态的 `state.json` 或缓存。

## 21. 安全与隐私边界

禁止进入长期记忆：

- API Key、密码、Token 和凭据；
- 完整模型思考；
- 无必要的完整命令正文；
- 项目外绝对路径；
- Reasonix 私有状态文件；
- 未授权用户文件内容；
- 无证据支持的用户身份或组织敏感信息。

禁止进入全局记忆：

- 可识别项目名称和绝对路径；
- 单项目业务术语和私有组件名称；
- 只在一个项目出现的未经泛化结论；
- 无法表达适用范围的绝对规则；
- 未经确认的一次性用户偏好。

自动 Wiki 编译不得：

- 修改 Reasonix 二进制；
- 修改 OMR Go 源码；
- 修改安全 Gate；
- 修改用户原始 Prompt；
- 修改全局配置；
- 访问其他项目的 Evolution Store。

## 22. 与现有 Evolution Overlay 的关系

OKF Memory 和 Overlay 各有不同职责：

```text
OKF Memory
  大量、结构化、按需读取、允许 probation、可自动生成

Overlay
  少量、全局生效、每次任务加载、高影响、必须严格控制
```

一条 OKF 记忆不能因为被创建就自动复制进 Overlay。

只有长期稳定、广泛适用且通过现有高影响 Gate 的策略，才可以单独评估是否进入 Overlay。即使不进入 Overlay，它仍可作为按需知识正常发挥作用。

## 23. 迁移策略

现有 v2.0.x 数据迁移原则：

1. 原有 Episode、Pattern、Proposal、Experiment、Observation 不删除；
2. 先生成只读迁移预览；
3. 创建迁移 Snapshot；
4. 将现有 JSON 移入或映射到 `facts/`；
5. 从已有 Pattern 和已生效策略编译初始 Wiki；
6. 初始生成的知识统一进入 `probation` 或按现有证据推导状态；
7. 重建根索引和局部索引；
8. Doctor 全部通过后才切换读取入口；
9. 迁移失败自动恢复；
10. 支持回滚到迁移前布局。

现有数据全部先迁移为 `project` Scope。迁移过程不得根据单个项目的历史数据直接创建 `global` 记忆；全局记忆只能在多个独立项目完成迁移并达到自动泛化门槛后生成。

不得因为迁移缺少历史 MemoryUsage 而伪造成功率、Lifecycle 或 Health。

## 24. 版本与实施阶段

| 阶段 | 建议版本 | 内容 |
|---|---|---|
| MEM-01 | v2.1.0-alpha.1 | 目录、OKF Schema、索引、Doctor |
| MEM-02 | v2.1.0-alpha.2 | JSON → Wiki 编译、自动 probation 写入 |
| MEM-03 | v2.1.0-beta.1 | 渐进式读取和 MemoryUsage 回执 |
| MEM-04 | v2.1.0-beta.2 | Lifecycle、Health、归因和冻结 |
| MEM-05 | v2.1.0-rc.1 | 修订链、恢复、对比、泛化、索引进化 |
| MEM-06 | v2.1.0 | 真实项目验证与稳定发布 |
| WEB-01 | v2.2.0-alpha.1 | 本地只读列表、详情和图谱 |
| WEB-02 | v2.2.0-beta.1 | 管理操作、审计、Snapshot 和回滚 |
| WEB-03 | v2.2.0 | 完整本地记忆管理页面 |

版本号只是建议，实施前可以根据现有 Tag 状态重新冻结。

## 25. 自动化验收标准

实现时至少覆盖：

- 新 Episode 必须生成 Candidate；有可靠新知识时自动生成 probation Revision，无新知识时稳定 `no_change` 且 Wiki 零修改；
- 自动写入不需要人工批准；
- Memory Revision JSON 是知识内容规范事实，Mutation Replay 与 Revision Hash 不一致时 Doctor 阻断；
- Memory Revision 与 Memory Evidence Generation 都不可变；追加证据只创建新 Evidence Generation，不改写既有 Revision 或证据版本；
- OKF Page 只能由 MemoryRevision、当前 MemoryEvidenceGeneration 和 DerivedMemoryState 组合编译；
- Pin、Unpin、Manual Freeze、Unfreeze 和 Archive 只通过 Governance Event 记录并确定性派生；
- 删除全部 Generation、索引、统计缓存和 Web View 后可以从规范事实确定性重建；
- Actionable、Descriptive/Ontology 和 Explicit Memory 分别使用允许的 Usage Policy；
- Failure Concept 可依据 Evidence Validation 晋升，不依赖 helped/harmed；
- Type 与 Usage Policy 的非法组合被 Schema 拒绝；
- Confirmation 与 Attribution Override 只保存为带 ID、Scope、类型和 Hash 的不可变 Judgment Fact；
- JudgmentRef/ConfirmationSourceRef 的 ID、类型、Scope 和 Hash 不匹配时被拒绝；
- 所有 `usage_policy=explicit_confirmation` 的 Revision 都必须携带合法 `confirmation_source_ref`；
- 撤销 Confirmation 或修正 Attribution Override 创建新 Judgment 并保留 supersedes 链，不原地覆盖；
- `content_sha256` 和 `evidence_set_sha256` 使用版本化规范算法稳定计算；
- `memory_id` 在标题、正文和文件展示变化后保持稳定；
- Canonical Key 碰撞不会覆盖已有页面；
- 使用统计变化不会增加知识 Revision 或改变知识内容 Hash；
- Rename、Merge 和 Split 保留完整身份与血缘；
- Memory Relation 只使用 MemoryRef，证据只使用 EvidenceRef，Canonical Key 和路径不能作为机器身份；
- 非正式关系名能够被拒绝或按唯一迁移映射转换，不产生第二套 Relation Enum；
- 结构化去重不依赖 Embedding；
- `MemoryMutationPlan` 拒绝未知操作和自由文本写入；
- 证据不足时稳定产生 `no_change` 且 Wiki 零修改；
- `append_evidence` 只创建新的不可变 Evidence Generation，不增加或改写知识 Revision；
- 自动 Split 会逐条分配证据，不复制无关证据；
- 自动 Merge 只合并等价且适用条件兼容的记忆；
- Merge 主 ID 只按证据链完整度、创建时间和 memory_id 确定性选择，不依赖动态使用次数；
- Specialize 不修改全局来源，Generalize 不淘汰项目细节；
- 没有新 Episode 或新 Evidence 时不会递归触发进化；
- 正常索引只包含允许读取的状态；
- frozen 记忆不会进入正常检索；
- 程序不包含可无限增长的失败语义枚举；
- Failure Concept 可以通过有界 Mutation 自动创建和演化；
- Attribution Analyst 优先复用已有 Failure Concept；
- `harmed` 必须经过独立 Critic 支持后才进入负面计数；
- Analyst/Critic 不一致时稳定降级为 unknown；
- 客观事实、失败原因和记忆影响保持分离；
- 唯一 help/harm 协议在 Gate、统计、排序、晋升和冻结中保持一致；
- adopted、likely、未 affected 或未 evaluated 的 Usage 永不进入正负计分；
- Context Signature 可以从版本化 Context Descriptor 确定性重算；
- 同一 Root Task 的 retry 不会重复贡献独立 Episode；
- 第三方或基础设施失败在记忆影响为 neutral/unknown 时不计入负面结果；
- helped 会参与负面比例计算，但不会删除历史失败证据；
- 相同 Revision/Context 至少三次 harmed 且负面比例达到 60% 才触发冻结；
- 负面比例低于 40% 且连续三次 helped 可以恢复 healthy；
- 冻结只针对当前 Revision，不删除页面和 Outcome；
- 人工归因 Override 不覆盖原始 Analyst/Critic 记录；
- 只被 retrieved/read 的记忆不计失败；
- `outcome_attributed` 记忆只被 retrieved/read/adopted 但未满足唯一协议并产生 `counted_as_help` 时不增加排序权重；
- 语义相关性和适用条件优先于历史 Policy Evidence；
- Policy Evidence 排序权重按当前 Revision 和 Context 隔离，新 Revision 不继承旧 Revision 权重；
- 同一 Episode 对同一 Revision/Context 的重复回执最多计数一次；
- healthy 记忆在同层级中优先于 degraded，Pinned 不能绕过冻结和安全规则；
- 完全同级候选使用 Retrieval、Generation 和 Memory ID 派生的确定性随机且可复现；
- 不同 Retrieval ID 可以在同级候选间自然轮换；
- 高度相关的 probation 记忆可以作为单独观察候选返回，但不能覆盖主候选；
- 优先级变化只更新派生状态，不增加知识 Revision 或修改知识正文；
- 三类 Usage Policy 只能使用各自允许的 Policy Evidence 推动 probation → active；
- 修订创建新 revision，不静默覆盖；
- frozen 记忆可以恢复为 probation；
- 仍满足冻结条件时 `unfreeze` 被拒绝且零写入；只有 Attribution Override、新 Evidence Generation 或新 Revision 改变派生结论后才能恢复；
- 多条记忆可以生成 generalized memory；
- 根索引和局部索引可确定性重建；
- Wiki 可以从 JSON 事实层重建；
- 每个 Generation 的 Input Manifest 永久记录实际采用的 Fact ID、Hash、Schema Version、Compiler Version 和 Canonicalization Version；
- 后续追加 Episode、Usage 或 Governance Event 不改变历史 Generation 的精确重建输入集合；
- 删除可清理的历史 Generation 后，仍可从对应 Input Manifest 和规范事实重建相同输出 Hash；
- 历史 Compiler/Canonicalization/Schema Version 不可用时 Doctor 阻断，不能用当前算法伪造精确重建；
- 多文件变更在 `CURRENT` 切换前对读取者零可见；
- Generation Input Manifest 必须在 staging 验证后、`CURRENT` 切换前安全落盘；
- 构建目标 Generation 所需的全部 Revision、Evidence Generation、Governance 和 Mutation 事实必须在 `CURRENT` 切换前安全落盘；切换后不得补写重建所需事实；
- 未提交 prepared 事务事实通过 `transaction_id` 和 prepared manifest 隔离，不能被普通编译采用；
- `CURRENT` 是唯一提交点，孤立 Generation 不参与读取；
- 同一任务中的 Librarian 和父 Agent 固定使用同一 Project/Global Generation Pair；
- 同一任务同时固定 Project/Global 两个 Generation，任一 Scope 更新都不改变当前读取快照；
- 并发 writer 由 Scope 单写锁和 Generation CAS 阻止互相覆盖；
- 重复事务幂等，相同幂等键但不同输入 Hash 被拒绝；
- 事实落盘后发生编译失败时事实保留并进入待编译状态；
- staging、发布后未切换和切换后未标记三类崩溃点均可确定性恢复；
- 项目提升为全局失败时不会回滚或污染项目 Scope；
- 回滚只切换 Generation 并新增审计记录，不修改历史 Generation；
- 清理派生 Generation 不删除 Memory、Revision、Evidence、Outcome 或 Generation Input Manifest；
- 路径穿越、symlink、敏感内容和跨 Scope 被拒绝；
- 项目记忆优先于冲突的全局记忆；
- 全局泛化创建新的 Memory ID 并保留 `generalized_from`，不移动或淘汰项目记忆；
- 同一仓库的多个 Clone 不会被计算为多个独立 Project Family；
- 全局 Store 只保存本机不可逆 Project Family 指纹，不泄露 Remote、路径或项目名称；
- 单项目证据不能自动生成 `global_probation` 或 `global_active`；
- Global Promotion 按 Usage Policy 分流：`outcome_attributed` 使用 help/harm，`evidence_validated` 使用跨 Family 独立 Evidence 与 Critic，`explicit_confirmation` 使用明确全局 Scope 的可验证确认；
- `outcome_attributed` 只有在三个独立 Project Family、五次 `counted_as_help`、证据分布和 Critic 门槛全部满足后才生成 `global_probation`；
- `global_candidate` 不参与正常检索；
- `global_probation` 只能按自身 Usage Policy 的激活门槛晋升，三类证据不得互相替代；
- 无法表达 `applies_when` 和 `does_not_apply_when` 的候选不能进入 probation；
- 单个项目的 harmed 只触发特例或边界分析，不能独自冻结全局记忆；
- 全局冻结至少需要两个独立 Project Family 提供与自身 Usage Policy 匹配的负面证据；`explicit_confirmation` 按全局确认撤销协议处理；
- Generalizer/Critic 只输出结构化计划，不能直接写入全局 Store；
- 全局泛化过程不会泄露项目名称、路径和业务术语；
- portable 经验在导入前不会参与检索；
- 损坏 frontmatter、断链和索引泄漏可被 Doctor 检出；
- Web 操作不能绕过库级约束；
- 不存在向量数据库或 Embedding 依赖。

## 26. Specification Convergence Docs Gate

Mnemosyne 进入 MEM-01 前，本文必须作为正式实现规格通过以下 Gate：

```bash
git diff --check
bash tests/docs_check.sh
```

Docs Gate 还必须执行面向本文的确定性静态检查，至少拒绝：

- `omr evolve memory` 等非正式 CLI 根命令；
- `.reasonix/omr/evolution/wiki/` 等旧固定 Wiki 路径；
- `raw/`、`raw evidence` 等旧事实目录术语；
- `not_for`、`failure_classes`、`confidence` 等已删除字段；
- Canonical Key 或 Markdown Path 充当机器关系身份；
- Relation Enum 之外的机器关系；
- 未携带 `usage_policy` 的 Memory Revision；
- Type 与 Usage Policy 不满足允许矩阵；
- `usage_policy=explicit_confirmation` 的 Revision 缺少合法 `confirmation_source_ref`；
- Confirmation Source 或 Attribution Override 没有不可变 Judgment Fact、正式 Ref、Scope 和 Hash；
- JudgmentRef/ConfirmationSourceRef 与目标 Judgment 的 ID、类型、Scope 或 Hash 不一致；
- MemoryRevision 包含可变 EvidenceRef 集合，或 `append_evidence` 原地修改既有 Revision/Evidence Generation；
- OKF Page 未按 `MemoryRevision + 当前 MemoryEvidenceGeneration + DerivedMemoryState` 编译；
- 未携带版本和 Descriptor Ref 的 Context Signature；
- Project/Global 只固定一个 Generation 的 MemoryContext；
- `adopted`、`likely` 或未 evaluated 的使用进入正负计分；
- 把 `global_candidate` 当作普通 Memory/Lifecycle；
- Global Promotion 对全部 Usage Policy 统一使用 `counted_as_help/harm`，或跨 Policy 借用晋升证据；
- 把 Lifecycle、Health、Usage Statistics、Relation/Root/Local Index、Generation 或 Web View 当作规范事实；
- Pin、Freeze、Unfreeze 或 Archive 直接改写派生状态而没有 Governance Event；
- 仍满足冻结条件时允许无条件 `unfreeze`；
- 无法从规范事实确定性重建的派生状态；
- 在规范 Revision、Evidence Generation、Governance 或 Mutation Fact 完整落盘前切换 `CURRENT`；
- 在 Generation Input Manifest 安全落盘前切换 `CURRENT`；
- 未提交 prepared 事务事实未按 `transaction_id` 和 manifest 隔离；
- 只保存 `input_hash` 而未永久记录实际 Fact ID、Hash 和编译版本；
- 清理 Generation 时删除对应 Generation Input Manifest；
- 历史编译版本不可用时使用当前算法伪造精确重建；
- MemoryMutationPlan 接受模型提供的可信 before/after Hash；
- Merge 使用动态成功采用次数、help/harm 次数或 Evidence 数量选择主 ID；
- `memory-revisions` 与 `memory-mutations` Hash 链不一致却未由 Doctor 报错；
- Failure Concept 父子关系不是 `child specializes parent`，或将反向 `broader_than` 当成正向输入；
- 同一 Root Task 的 retry 重复贡献独立 Episode 计数；
- 使用 `success_count/failure_count` 或 `helped_count/harmed_count` 形成第二套统计语义；
- 把 Project/Global Generation Pair 简写为一个含糊的“同一 Generation”；
- 示例、Schema、状态转换表和验收标准互相矛盾。

正式冻结条件：

1. 全文旧术语扫描为零，只允许在“禁止/迁移映射”语境中出现；
2. Schema、关系枚举、Usage Policy、计分协议和状态转换只有一个规范定义；
3. 所有示例符合规范定义；
4. 所有派生状态都有明确规范输入和重建算法；
5. Docs Gate 通过；
6. 文档状态标记为“正式实现规格”；
7. 后续 Review 默认检查代码是否违反规格，不继续扩张架构。

## 27. 明确非目标

第一阶段不实现：

- 向量数据库；
- Embedding；
- GraphRAG；
- 图数据库；
- 云端同步服务；
- 跨用户公共记忆市场；
- 自动修改 Reasonix 或 OMR 源码；
- 基于完整模型思考的训练；
- 无证据的自动知识生成；
- 自动物理删除失败记忆。

## 28. 最终架构决议

OMR Mnemosyne 的长期架构正式确定为：

```text
严格 JSON 事实源
        +
OKF 风格 Markdown 知识库
        +
不可变 Generation 与原子 CURRENT 提交
        +
分层 index.md 与显式知识关系
        +
Reasonix 原生理解和文件导航
        +
自动记录、自动使用、自动修订
        +
Revision 事实、Mutation 审计与可重建派生状态
        +
按 Usage Policy 的可验证负面证据达到阈值后冻结，但永不自动删除
        +
项目经验经 Candidate、Probation、Active 三阶段自动泛化
        +
相关性优先、Policy Evidence 增强、确定性随机探索
        +
CLI 与本地 Web 图谱人工管理
```

冻结记忆默认不给模型，不参与正常任务检索；它们作为历史证据、失败边界和未来泛化材料永久保留。

OMR 记忆采用 `project + global + portable` 三层 Scope。原始事实默认归属于项目；项目经验在获得多个独立项目的支持并完成脱敏、泛化和冲突检查后，可以自动形成全局 probation 记忆。读取时项目记忆优先于全局记忆，portable 记忆只有经过显式导入后才参与当前 Scope。

向量数据库和 Embedding 不属于当前或未来 OMR Mnemosyne 架构。

正式产品定义冻结为：

> OMR Mnemosyne 是基于 OKF 和渐进式披露构建的长期记忆与经验进化系统。它不使用向量数据库，通过结构化索引和显式知识关系，让 Reasonix 自动积累、使用、修订、冻结和泛化工程经验，同时为用户提供完整的人工管理与图谱观察能力。

本文自 Docs Gate 通过起作为 Mnemosyne 的正式实现契约。MEM-01 及后续 Review 默认检查代码、Schema、Fixture 和 CLI 是否符合本文；除非真实实现证据证明契约存在阻塞性缺陷，不再继续扩张架构范围。
