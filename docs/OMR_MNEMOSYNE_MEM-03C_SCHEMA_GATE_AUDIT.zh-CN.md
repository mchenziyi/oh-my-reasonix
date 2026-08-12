# OMR Mnemosyne MEM-03C-01 Schema Gate 审核

- 日期：2026-08-12
- 对象：`EpisodeFact`、`ContextDescriptorFact`、Episode/Context 引用与派生 Card/Index
- 结论：**Gate PASS；D1～D10 已由 CTO 批准并冻结**
- 范围：只读审核与设计收敛；未修改 `internal/memory` 产品代码

## 一、客观发现

1. Architecture v1 已把 Episode、Context Descriptor 列为不可删除的规范事实，把 Episode
   Card/Episodic Index 列为可删除重建的派生表示。
2. `internal/memory.FactStore` 尚无 `FactKindEpisode`、`FactKindContextDescriptor`、
   `EpisodeRef` 或 `ContextDescriptorRef`。
3. `internal/evolution.Episode` 属 v2.0 MVP 的独立 Evolution Store，只记录弱校验的任务类、
   成败、Session、Token 和时间；它缺 Scope、Root Task、Context/Evidence 引用与内容 Hash。
4. 当前 `internal/memory.Outcome` 是某个 `MemoryUsage` 对某条 MemoryRevision 的归因结果，不能
   表达“未使用任何 Memory 的任务是否成功”，因此不能直接作为 Episode 的任务结果。
5. Architecture v1 13.8 给出了 Context Descriptor 示例，但没有完整 Fact Envelope、Ref、
   Scope、`content_sha256` 与 `created_at` 契约。

这些是由实现盘点暴露的 **Implementation Failure**，符合 Architecture Amendment 允许条件；
若不补齐，任何 Episode Card 都只能依赖旧 Evolution 私有记录或自身摘要，都会形成第二事实源。

## 二、对象判定

| 对象 | 判定 | 理由 |
|---|---|---|
| EpisodeFact | 新增不可变规范 Fact | 客观发生的 Root Task Episode，必须可被 Manifest 精确引用 |
| ContextDescriptorFact | 新增不可变规范 Fact | Context Signature 的可解释、可重算输入 |
| EpisodeRef | 新增完整 Ref | 必须包含 Scope、Episode ID、Content Hash |
| ContextDescriptorRef | 新增完整 Ref | 不能继续只用裸 `context_descriptor_id` |
| Task Result | EpisodeFact 内的客观枚举 + EvidenceRef | 现有 Memory Outcome 语义不同，不复用、不另造含糊 OutcomeRef |
| Episode Card | Generation 派生视图 | 可删除重建，不进入 FactStore |
| Episodic Index | Generation 派生视图 | 可删除重建，不进入 FactStore |
| Legacy Evolution Episode | legacy-only 采集记录 | 不能直接成为 Mnemosyne Fact |

## 三、建议冻结的唯一 Schema

### 3.1 EpisodeFact

```yaml
schema_version: 1
episode_id: episode_01K...
scope: project
root_task_id: task_01K...
context_descriptor_ref:
  scope: project
  context_descriptor_id: context_01K...
  content_sha256: sha256_...
task_class_refs: [task_class_build]
component_refs: [component_memory]
operation_refs: [operation_compile]
failure_concept_refs: []
task_result: succeeded       # succeeded | failed | cancelled | unknown
task_result_evidence_refs: []
evidence_refs: []
occurred_at: 2026-08-12T00:00:00Z
content_sha256: sha256_...
created_at: 2026-08-12T00:00:00Z
```

核心语义：一个 Episode 对应一个 Root Task 的确定性聚合；同 Root Task 的 retry/恢复/attempt
作为 Evidence 保存，不新增 `attempt_id` 到 Episode 身份，也不允许多个 attempt 刷独立 Episode。

`task_result` 是 Root Task 的客观结果，不是 Memory 归因 Outcome。`task_result_evidence_refs`
必须至少在 `failed/cancelled` 时非空；`unknown` 不得伪装成功或失败。

EpisodeFact v1 不保存自由文本摘要。现有 `content_classification` Judgment 只能锚定 EvidenceRef，
不能安全地证明 Episode 摘要的来源与类别；在正式 Episode 内容分类协议冻结前，Card 只渲染
结构化字段，不自行生成或保存摘要。

### 3.2 ContextDescriptorFact

```yaml
schema_version: 1
context_descriptor_id: context_01K...
scope: project
context_signature_version: 1
component_refs: []
operation_refs: []
task_class_refs: []
environment:
  os: darwin
  arch: arm64
  language: go
  framework: ""
  tool: omr
canonical_sha256: sha256_...
content_sha256: sha256_...
created_at: 2026-08-12T00:00:00Z
```

`canonical_sha256` 是结构化 Context Signature；`content_sha256` 是完整 Fact Content Hash，
二者不可混用。两个 Hash 均由程序计算。环境字段使用版本化固定键与受控长度，不记录路径、
用户名、主机名、Remote、项目名或业务名。

### 3.3 引用

```yaml
EpisodeRef:
  scope: project
  episode_id: episode_01K...
  content_sha256: sha256_...

ContextDescriptorRef:
  scope: project
  context_descriptor_id: context_01K...
  content_sha256: sha256_...
```

所有引用均按完整字段匹配。裸 ID 只允许用于受控分类维度，不作为事实引用。

## 四、CTO 决议表

| ID | 问题 | 推荐决议 | 状态 |
|---|---|---|---|
| D1 | Episode 身份粒度 | 一个 Root Task 聚合为一个 Episode；attempt 仅作 Evidence | 已批准 |
| D2 | `attempt_id` | 不进入 EpisodeFact | 已批准 |
| D3 | 任务结果 | `task_result` 四值枚举，不复用 Memory Outcome | 已批准 |
| D4 | 失败证据 | failed/cancelled 必须有 task_result EvidenceRef | 已批准 |
| D5 | 结构化分类 | task/component/operation/failure 使用受控 ID 集合 | 已批准 |
| D6 | 摘要 | EpisodeFact v1 不保存自由文本摘要 | 已批准 |
| D7 | Context Scope | ContextDescriptorFact/Ref 显式包含 Scope | 已批准 |
| D8 | 双 Hash | canonical_sha256 与 content_sha256 分离，均由程序计算 | 已批准 |
| D9 | Legacy | 不自动迁移；缺规范字段的旧 Episode 保持 legacy-only | 已批准 |
| D10 | Manifest | Episode/Context 作为永久输入；Card/Index 不进入 inputs | 已批准 |

## 五、兼容与迁移

- 新增 FactKind 属显式可加联合分支，不修改现有 Fact 的 Canonical Bytes/Hash；
- 必须为所有旧 Fact 类型保留 golden，证明扩展后字节不变；
- `MemoryUsage.ContextDescriptorRef` 当前是裸 ID。第一版保持 legacy/anchored golden 不变；
  Doctor 用 Episode/Usage 的完整上下文检查裸 ID 是否能在同 Scope 唯一解析，不能解析则报告，
  不静默填 Hash；后续版本化迁移另立计划；
- 旧 Evolution Episode 只可经显式 Bridge 创建新 Fact，Bridge 不在 MEM-03C；
- 新 Generation Manifest 可引用新 FactKind；历史 Manifest 和旧 Compiler 永久保留。

## 六、Gate 判定

Schema Gate：PASS。

D1～D10 已全部批准并写回主计划，现在可以：

1. 将主计划状态改为“Schema 冻结，待实现”；
2. 在 `tests/docs_check.sh` 固定 Schema/禁止项；
3. 进入 MEM-03C-02，先写 golden 与失败测试，再新增 FactKind/Store/Compiler；
4. 任何决议变化必须先更新文档，不能由代码偷偷选择。

本轮没有进入产品代码、没有修改 Architecture v1、没有提交或推送实现。
