# OMR Mnemosyne MEM-02 Schema Gate 审计记录（初轮）

- 阶段：MEM-02 Protocol Extension / Schema Gate
- 状态：**历史归档（2026-08-11）**——初轮 Schema Gate 审核快照
- 性质：本文件为审核历史证据，**不作为当前协议输入**；当前唯一有效 Schema、C1～C19 关闭结果与 D1～D12 最终决议以 `OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md` 与 `OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md` 为准
- 内容：初轮审核的差异表、冲突清单（C1～C19 提出）、CTO 决策点（D1～D12 提出）与修订清单

> **结论（初轮）：Gate 未通过。** 初轮审核时本设计存在 19 处差异（C1–C19：15 处冲突 + 1 处既有实现缺口；含 6 处重大冲突 C5–C8/C13/C14、9 处中、3 处小）与 12 个待 CTO 决策点（D1–D12）。上述冲突与决策点已由 Schema Convergence（D1～D12 决议）逐项关闭/定案；当前 Gate 状态见主协议文档。

## 一、初轮对象判定（Fact / Judgment / 派生）

| 对象 | 判定 | 依据（架构 v1 / 冻结实现） |
|---|---|---|
| Retrieval Evaluation | **Fact**（`facts/retrieval-evaluations/`） | 5.3 总表、6.2.3:690 |
| Evidence Provenance 四维度 | **MemoryEvidenceGeneration 内嵌字段**（非独立 Fact、非 Judgment） | 6.2.3:762「每个长期 Evidence Fact 必须包含以下独立维度」 |
| Trust Gate 结果 | **派生状态**（`trusted/restricted/unverified/blocked`） | 5.3「Freshness、Evidence Trust Gate 结果…派生」 |
| `critic_review` | **Judgment**（复用 JudgmentFact Envelope + `JudgmentRef`） | 6.2.1:639「复用基础 Envelope 与 JudgmentRef，由对应协议定义固定 subtype payload」 |
| `context_applicability` / `retrieval_relevance` / `content_classification` / `evidence_trust` / `freshness_evaluation` | **Judgment**（MEM-01A 已冻结） | 6.2.1/6.2.3 |
| Candidate Universe | **待定**（若采纳为规范 Fact，用于候选世界快照） | 架构无此对象（D7，后决议：不新增） |

## 二、初轮 Schema 差异表（设计文档 vs 冻结契约）

### 2.1 公共层（设计 2.1/2.2）

| 设计文档 | 冻结契约 | 判定 |
|---|---|---|
| `schema_version: 2`（2.1） | `JudgmentFact.schema_version` 必须为 1（model.go 冻结）；扩展 subtype = v1 envelope + 联合加分支 + 扩 `JudgmentType` 枚举，不是 bump schema_version | **冲突 C1**：应改为「新 subtype 沿用 v1 envelope」 |
| `retrieval_generation_ref` / `evaluation_generation_ref: GenerationRef`（2.2） | 架构 6.2.3 用 `memory_context{project_generation_ref, global_generation_ref}`（成对、某 Scope 无记忆显式 null）；冻结实现为 `ProjectGenerationRef`/`GlobalGenerationRef`（evaluation_context.go：scope+generation_id+input_manifest_sha256） | **冲突 C2**：`GenerationRef` 类型不存在，应复用冻结形式 |
| `candidate_universe_ref` / `candidate_universe_sha256` | 架构无此概念 | 新扩展，待 D7（后决议：不新增） |
| `evaluation_scope` 枚举 | 一致（fixture/generation_full_scan/expanded_index_scan/sampled_audit） | 兼容 |

### 2.2 Critic Review（设计 3.2）

| 设计文档 | 冻结契约 | 判定 |
|---|---|---|
| `judgment_type: critic_review` | `JudgmentType` 冻结 7 值（MEM-01A）无 critic | **冲突 C3**：需 CTO 批准扩枚举（架构 6.2.1:639 允许「对应协议定义」） |
| `result: passed/failed/unavailable` | 无冻结枚举；11.5 仅「Critic 通过/支持」语义 | D1：冻结枚举，语义对齐 11.5 |
| `reviewed_generation_ref: GenerationRef` | `GenerationRef` 类型不存在 | **冲突 C2（复用）** + D10：改为 Project/GlobalGenerationRef 或 memory_context 形式 |
| `basis_refs` | `BasisRef` 联合已冻结 | 兼容 ✓ |
| `critic_source.source_type: fixture/offline_rule/user_review` | `JudgmentSource.source_type` 是自由 ID（model.go:600-611，未冻结枚举）；架构 6.2.3:724 authority=fixture_oracle/retrieval_critic/user_review | **冲突 C4**：枚举需统一为冻结值 |
| 3.3「无未解决冲突」 | 无冻结冲突事实/协议 | D12：冻结前该条件视为不满足（probation） |
| 3.3「≥3 EvidenceRef、≥2 root_task_refs」 | 架构 11.5 一致；`MemoryEvidenceGeneration.RootTaskRefs` 已冻结 ✓ | 兼容 ✓ |

### 2.3 Evidence Trust（设计 4.2）

| 设计文档 | 冻结契约 | 判定 |
|---|---|---|
| 独立 `EvidenceTrustFact`（trust_id + evidence_generation_ref） | 架构 6.2.3:762 要求四维度**内嵌**长期 Evidence Fact（MemoryEvidenceGeneration） | **冲突 C5（重大）**：载体冲突 |
| `origin: internal/user/repository/external` | 架构 6.2.3:771 `runtime/user/official/project/external` | **冲突 C6（重大）**：枚举不一致 |
| `acquisition_method: test_result/user_report/review/import/unknown` | 架构 6.2.3:772 `direct/tool_observed/model_extracted/imported` | **冲突 C7（重大）** |
| `verification_status: verified/partially_verified/unverified/revoked` | 架构 6.2.3:773 `verified/confirmed/inferred/unverified` | **冲突 C8（重大）** |
| `content_class: factual/instructional/descriptive/secret/unsafe` | 架构 ContentClassification = `contains_instructional_content`/`contains_sensitive_content` bool + `PolicyConfigContentClassifier.allowed_classes`（MEM-01C 冻结） | **冲突 C9** |
| `provenance_ref: ProvenanceRef\|null` | `ProvenanceRef` 类型不存在；架构 `provenance_refs: []`（数组，闭合到原始 EvidenceRef） | **冲突 C10** |
| `trust_policy_sha256` | `PolicyRef = policy_id + policy_type + content_sha256`（架构 6.2.3:850）已含 hash | **冲突 C11（小）**：删除冗余 |
| `trust_policy_ref` | PolicyRef 冻结 | 兼容 ✓ |
| 派生 `trusted/restricted/unverified/blocked` | 未冻结（MEM-02 计划 6.2 提案） | D（冻结枚举） |
| 4.3「Secret/Unsafe 不参与 Promotion」 | `PolicyConfigContentClassifier.secret_classes_block_promotion=true` 已冻结 | 兼容 ✓ |
| 4.3「Trust 不改 Lifecycle」 | 架构 5.3「Trust Gate 结果=派生」 | 兼容 ✓ |

### 2.4 Retrieval Evaluation（设计 5.2）

| 设计文档 | 冻结契约 | 判定 |
|---|---|---|
| `retrieval_generation_ref`/`evaluation_generation_ref`（两个独立 ref） | 架构 6.2.3 `memory_context{project_generation_ref, global_generation_ref}`（固定一对、显式 null） | **冲突 C12** |
| `expected_memory_refs`/`retrieved_memory_refs`/`result` 放入 Fact | 架构 6.2.3:726-737：这些在 `retrieval_relevance` **Judgment payload**（result 枚举一致）；Fact 只含 `judgment_ref` | **冲突 C13（重大）**：result/refs 双写 → 双事实源风险 |
| `authority` 放入 Fact | 架构 6.2.3:724「authority 不在 RetrievalEvaluation 重复保存，判断来源只存在于 Judgment Fact 的 `source` 中」 | **冲突 C14（重大）**：与架构相反 |
| `candidate_universe_ref`/`candidate_universe_sha256` | 架构无 | 新扩展，待 D7（后决议：不新增） |
| 缺 `retrieval_id` | 架构 6.2.3:697 必需 | **冲突 C15（小）**：补 |
| 缺 `judgment_ref` | 架构 6.2.3:711 必需 | **冲突 C15（小）**：补 |
| `evaluation_scope` | 一致 | 兼容 ✓ |
| 5.2「missed_relevant 必须绑定 candidate universe Hash」 | 架构 6.2.3:737「只能触发候选修复」 | 兼容（依赖 D7，后决议：用固定 Generation Pair + evaluation_scope） |

### 2.5 Context Applicability（设计 6.1）

| 设计文档 | 冻结契约 | 判定 |
|---|---|---|
| `result: applicable/conditionally_applicable/not_applicable/unknown/unavailable` | 架构 6.2.3:758 冻结 `exact/applicable/conditionally_applicable/not_applicable/unknown`（有 exact、无 unavailable） | **冲突 C16**：应保 exact；去 exact 需 CTO |
| `memory_ref` 放入 payload | 与 `subject.memory_ref` 重复（JudgmentSubject context 分支已冻结） | **冲突 C17（小）**：删除 |
| `conditions: [ApplicabilityCondition]` 内联 | 架构 6.2.3:758「必须引用目标 Memory Revision 中存在的 `ApplicabilityCondition.condition_id`」；`required_condition_ids` 已冻结 | **冲突 C18**：违反「禁止内联/自由文本条件」 |
| `basis_context_refs: [ContextRef]` | `ContextRef` 类型不存在；架构 6.2.3:748-750 顶层 `[string]`（MEM-01A **未实现**该顶层字段——既有实现缺口） | **缺口 C19** + D9 |
| `target_context_ref`（subject 内） | 已冻结实现（model.go:510，裸 string） | 兼容 ✓ |
| 6.1「结果只影响检索适用性排序」 | 架构 6.2.3「只能触发 Alias、Relation、Index 或 Query 路由的候选修复，不能直接修改 Memory Revision 内容」（6.2.3:737，Retrieval 一节）；758 行 Context 一节为「只负责可审计的跨 Context 判断」 | 兼容 ✓ |

## 三、初轮冲突汇总（C1～C19 提出）

| 编号 | 级别 | 位置 | 说明 |
|---|---|---|---|
| C1 | 中 | 公共 2.1 | `schema_version: 2` 与实现机制冲突，应保持 v1 envelope 扩展联合 |
| C2 | 中 | 公共 2.2 / Critic 3.2 | `GenerationRef` 类型未定义，应复用 Project/GlobalGenerationRef |
| C3 | 中 | Critic 3.2 | 新 subtype 需扩冻结 `JudgmentType` 枚举（有架构 6.2.1:639 依据） |
| C4 | 中 | Critic 3.2 | `critic_source.source_type` 枚举与架构 authority 枚举/自由 source_type 不一致 |
| C5 | **重大** | Trust 4.2 | Provenance 载体：独立 Fact vs 架构要求内嵌 Evidence Fact |
| C6 | **重大** | Trust 4.2 | `origin` 枚举与架构 6.2.3:771 不同 |
| C7 | **重大** | Trust 4.2 | `acquisition_method` 枚举与架构 6.2.3:772 不同 |
| C8 | **重大** | Trust 4.2 | `verification_status` 枚举与架构 6.2.3:773 不同 |
| C9 | 中 | Trust 4.2 | `content_class` 枚举 vs bool 对 + allowed_classes |
| C10 | 中 | Trust 4.2 | `ProvenanceRef` 未定义；应为 `provenance_refs` 数组 |
| C11 | 小 | Trust 4.2 | `trust_policy_sha256` 冗余（PolicyRef 已含 hash） |
| C12 | 中 | Retrieval 5.2 | 独立 generation refs vs 架构 `memory_context` 成对形式 |
| C13 | **重大** | Retrieval 5.2 | result/expected/retrieved 放入 Fact → 与 Judgment 双写 |
| C14 | **重大** | Retrieval 5.2 | `authority` 放 Fact，与架构 6.2.3:724 相反 |
| C15 | 小 | Retrieval 5.2 | 缺 `retrieval_id`、`judgment_ref` |
| C16 | 中 | Context 6.1 | result 枚举缺 exact 多 unavailable（架构有 exact） |
| C17 | 小 | Context 6.1 | payload `memory_ref` 与 subject 重复 |
| C18 | 中 | Context 6.1 | 内联 `conditions` 违反架构 758 引用规则 |
| C19 | 缺口 | Context 6.1 | `basis_context_refs` 顶层字段：架构 6.2.3 有、MEM-01A 未实现 |

## 四、初轮 CTO 决策点（D1～D12 提出）

| 编号 | 决策点 | 初轮推荐 | 最终决议（Convergence） |
|---|---|---|---|
| D1 | `critic_review.result` 枚举（passed/failed/unavailable）是否冻结 | 是；`unavailable` 语义=保持 probation | ✅ 冻结 |
| D2 | 新 subtype 的 schema_version 语义 | 沿用 v1 envelope，不 bump（消解 C1） | ✅ 沿用 v1 |
| D3 | Provenance 四维度载体 | 内嵌 `MemoryEvidenceGeneration`（消解 C5）；缺维度=unavailable | ✅ 内嵌 |
| D4 | origin/acquisition/verification 枚举 | 以架构 6.2.3:771-773 冻结值为准（消解 C6–C8） | ✅ 架构枚举 |
| D5 | content 类表示 | bool 对 + `allowed_classes`（消解 C9） | ✅ bool + Policy |
| D6 | RetrievalEvaluation Fact 字段 | 按架构 6.2.3 实现，不复制 result/refs/authority（消解 C12–C15） | ✅ 双对象 |
| D7 | candidate_universe_ref/sha256 是否新增（架构外） | 待定 | ✅ 不新增（固定 Generation Pair + evaluation_scope） |
| D8 | context result 枚举 | 保 `exact`（消解 C16） | ✅ 保 exact |
| D9 | `basis_context_refs` 类型 | 受控 identifier 列表（消解 C19） | ✅ 受控 ID 数组 |
| D10 | `reviewed_generation_ref` 形式 | 复用 `ProjectGenerationRef`/`GlobalGenerationRef`（消解 C2） | ✅ memory_context |
| D11 | MemoryUsage 是否扩展（架构 12.2 锚字段） | 待定；影响 MEM-01F 冻结 | ✅ 补齐设计（仅设计不实现） |
| D12 | 3.3「无未解决冲突」的冲突协议来源 | 无冻结事实；冻结前视为不满足（probation） | ✅ 冻结前 probation |

## 五、初轮 Gate 通过前提（修订清单，已由 Convergence 完成）

1. 设计修订：Trust 载体改内嵌、三枚举与 content_class 对齐架构 6.2.3、Retrieval Fact 按架构 6.2.3 实现且删除 result/refs/authority、Context 删除内联 conditions 与重复 memory_ref、result 保 exact、GenerationRef 复用冻结形式、删除 trust_policy_sha256。
2. CTO 批准 D1–D12（含候选世界与 MemoryUsage 扩展的取舍）。
3. 修订后重跑 Schema Gate；`tests/docs_check.sh` 增加关键术语与禁止项检查。
4. 全部通过后，才进入代码实现阶段（仍需另行获批）。
