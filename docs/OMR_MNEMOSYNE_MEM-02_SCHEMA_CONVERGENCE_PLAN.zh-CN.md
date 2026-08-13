# OMR Mnemosyne MEM-02 Schema Convergence 修改方案

- 阶段：MEM-02 Protocol Extension / Schema Convergence
- 状态：✅ 已完成（2026-08-11）——Protocol Extension 设计已按 D1～D12 修订，C1～C19 全部关闭，只读 Schema Gate 复核 PASS，Docs Gate 通过；3.6 observation_provenance 已按 Architecture v1 12.2 勘误；对应 MEM-02A～MEM-02G 实现已完成
- 输入：MEM-02 Protocol Extension 初稿及 Schema Gate C1～C19、D1～D12
- 输出：与 Architecture v1、MEM-01A～MEM-01F 完全一致的协议设计文档
- 本阶段只改文档，不实现产品代码

## 一、目标

消除 Schema Gate 发现的 19 处差异，冻结 Critic Review、Evidence Provenance/Trust、Retrieval Evaluation、Context Applicability 和 MemoryUsage 锚字段的唯一协议，为后续代码实现提供契约。

完成标准：

1. C1～C19 均有明确处理结果；
2. D1～D12 全部定案；
3. 不新增第二事实源；
4. 不修改 Architecture v1；
5. 不修改 internal/memory；
6. Docs Gate 通过；
7. 重跑 Schema Gate 后结论为 PASS。

## 二、CTO 决议：D1～D12

| 编号 | 最终决议 |
|---|---|
| D1 | 新增 `critic_review` Judgment subtype，`result` 固定为 `passed | failed | unavailable` |
| D2 | 沿用 `JudgmentFact schema_version=1` Envelope；新增 subtype 是联合 Schema 的显式扩展，不 bump 到 v2 |
| D3 | Evidence Provenance 四维度内嵌 `MemoryEvidenceGeneration`，不新建 `EvidenceTrustFact` |
| D4 | 枚举严格采用 Architecture v1：origin=`runtime|user|official|project|external`；acquisition=`direct|tool_observed|model_extracted|imported`；verification=`verified|confirmed|inferred|unverified` |
| D5 | Content 分类复用既有 `contains_instructional_content`、`contains_sensitive_content` 和 Content Classifier Policy，不新增 content_class 枚举 |
| D6 | RetrievalEvaluation 按架构实现：Fact 保存 `retrieval_id + memory_context + evaluation_scope + judgment_ref`；result/expected/retrieved/authority 只存在于 retrieval_relevance Judgment |
| D7 | 第一版不新增 CandidateUniverse Fact/Ref；需要漏检复查时使用固定 Generation Pair + `evaluation_scope`，未来有基准证据后再提架构 amendment |
| D8 | Context Applicability result 保留 `exact|applicable|conditionally_applicable|not_applicable|unknown`；不增加 unavailable |
| D9 | `basis_context_refs` 使用受控 ID 字符串数组，排序去重，Scope 由 Evaluation Context 约束 |
| D10 | Critic 使用完整 `memory_context`（ProjectGenerationRef + GlobalGenerationRef，允许一侧 null），不创造通用 GenerationRef |
| D11 | MemoryUsage 补齐架构 12.2 锚字段：`retrieval_id`、`root_task_id`、固定 Project/Global Generation Pair、`context_signature_version`、`context_signature`、`observation_provenance`；旧记录缺字段时只能作为 legacy usage，不能参与需要这些锚的 Retrieval/Critic 评估 |
| D12 | “无未解决冲突”在冲突事实协议冻结前视为未满足，因此 evidence_validated 保持 probation；本阶段不发明 Conflict Fact |

## 三、逐项修改要求

### 3.1 公共 Envelope 与 Generation 引用

修正文档：

- 删除 `schema_version: 2`；
- 明确新增 Judgment subtype 沿用 v1 Envelope；
- 删除未定义的 `GenerationRef`；
- 所有历史评估统一使用：

```yaml
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
```

规则：

- 某 Scope 没有记忆时显式为 null；
- 一次评估固定同一 Generation Pair；
- 不允许读取未来 CURRENT；
- Generation 清理后仅通过永久 Input Manifest 重建。

### 3.2 Critic Review

冻结为 Judgment subtype：

```yaml
judgment_type: critic_review
subject:
  subject_type: memory_revision
  memory_ref: MemoryRef
critic_review:
  result: passed | failed | unavailable
  evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit
  memory_context:
    project_generation_ref: ProjectGenerationRef | null
    global_generation_ref: GlobalGenerationRef | null
  required_evidence_refs: [EvidenceRef]
source:
  source_type: fixture_oracle | offline_rule | user_review
  source_id: controlled_id
basis_refs: [...]
```

约束：

- passed 必须有非空 EvidenceRef 和合法 BasisRef；
- source 只允许确定性 Fixture、离线规则或人工 Review；
- 模型自报不能成为唯一 passed 来源；
- failed/unavailable 都不能晋升 active；
- 同 Scope、同 Revision、同 Generation Pair；
- supersede 链逐节点类型和 Subject 精确匹配。

### 3.3 Evidence Provenance 与 Trust

删除独立 `EvidenceTrustFact` 设计。

在 `MemoryEvidenceGeneration` 中追加可选兼容字段：

```yaml
origin: runtime | user | official | project | external
acquisition_method: direct | tool_observed | model_extracted | imported
verification_status: verified | confirmed | inferred | unverified
provenance_refs: [EvidenceRef]
contains_instructional_content: boolean
contains_sensitive_content: boolean
```

兼容策略：

- 旧 Evidence Generation 缺字段时仍可读取；
- 缺任一 Trust 必需维度时，Trust 派生为 unavailable；
- 新写入记录必须完整携带四维度；
- 字段进入 Canonical Hash；
- `provenance_refs` 排序去重并闭合到合法 Evidence；
- Trust Policy 使用既有 PolicyRef，不重复保存 hash；
- Trust Gate 是派生结果，不是新 Fact。

Trust 派生枚举冻结：

```
trusted | restricted | unverified | blocked
```

安全规则：

- external + unverified + instructional → blocked；
- sensitive/secret class 不得 Promotion；
- Trust 不直接改变 Lifecycle；
- Policy 安全根不可关闭。

### 3.4 Retrieval Evaluation

采用架构中的双对象模型：

RetrievalEvaluation Fact：

```yaml
schema_version: 1
evaluation_id: retrieval_eval_...
retrieval_id: retrieval_...
scope: project | global
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit
judgment_ref: JudgmentRef
content_sha256: sha256_...
created_at: RFC3339
```

Retrieval Relevance Judgment 保留：

- result；
- expected_memory_refs；
- retrieved_memory_refs；
- authority 通过 Judgment source 表达。

禁止：

- Fact 重复保存 result/refs/authority；
- Candidate Universe；
- 用未来 Generation 评价历史；
- Evaluation 和 Judgment 内容不一致。

### 3.5 Context Applicability

修订现有 subtype，不新增重复 MemoryRef：

```yaml
judgment_type: context_applicability
subject:
  subject_type: context
  memory_ref: MemoryRef
  target_context_ref: controlled_context_id
context_applicability:
  result: exact | applicable | conditionally_applicable | not_applicable | unknown
  required_condition_ids: [controlled_condition_id]
basis_context_refs: [controlled_context_id]
```

规则：

- 删除 payload 内重复的 `memory_ref`；
- 删除内联 `conditions`；
- conditionally_applicable 必须引用当前 Revision 已存在的 condition_id；
- exact 不得被 applicable 合并；
- 不新增 unavailable；
- basis_context_refs 排序去重；
- 结果只影响检索和路由，不修改 Revision。

### 3.6 MemoryUsage 锚字段

在设计文档中冻结新增字段，不立即实现：

```yaml
retrieval_id: controlled_id
root_task_id: controlled_id
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
context_signature_version: 1
context_signature: sha256_...
observation_provenance:
  source: agent_reported | runtime_observed | user_confirmed
  evidence_ref: EvidenceRef | null
  judgment_ref: JudgmentRef | null
```

规则：

- `observation_provenance` 结构与枚举与 Architecture v1 12.2 严格一致：`source` 固定
  `agent_reported | runtime_observed | user_confirmed`；`agent_reported` 必须引用承载
  结构化回执的 Reasonix Event EvidenceRef，`runtime_observed` 必须引用 Reasonix 公开
  事件 EvidenceRef，`user_confirmed` 必须引用 Confirmation JudgmentRef；前两类的
  `judgment_ref` 必须为 null；
- retrieved/read/adopted/affected/evaluated 均绑定同一 Retrieval 和 Generation Pair；
- 缺锚字段的 legacy usage 可用于基础 usage_count，但不得用于 Retrieval Evaluation、Critic 或跨 Context 归因；
- 不用 usage_id 代替 root_task_id/retrieval_id；
- Context Signature Hash 的描述符引用沿用 Architecture v1 已冻结规则。

## 四、C1～C19 关闭映射

| 冲突 | 关闭方式 |
|---|---|
| C1 | 沿用 v1 Envelope |
| C2/C12 | 统一 memory_context Generation Pair |
| C3 | 显式批准 critic_review 联合分支 |
| C4 | Critic source 枚举统一 |
| C5 | Provenance 内嵌 Evidence Generation |
| C6～C8 | 使用架构冻结枚举 |
| C9 | 复用 bool + Content Classifier Policy |
| C10 | 使用 provenance_refs 数组 |
| C11 | 删除冗余 trust_policy_sha256 |
| C13/C14 | Retrieval Fact 不保存结果和 authority |
| C15 | 补 retrieval_id、judgment_ref |
| C16 | Context 保 exact、不加 unavailable |
| C17 | 删除 payload 重复 memory_ref |
| C18 | 改 required_condition_ids |
| C19 | 增加顶层 basis_context_refs 受控 ID 数组 |

## 五、Docs Gate 新增检查

`tests/docs_check.sh` 应检查设计文档：

- 不包含 `schema_version: 2` Judgment 设计；
- 不包含独立 `EvidenceTrustFact`；
- 不包含 CandidateUniverseRef；
- RetrievalEvaluation Fact 不包含 result/expected/retrieved/authority；
- Context payload 不内联 conditions、不重复 memory_ref；
- Critic source 枚举一致；
- Provenance 三组枚举与 Architecture v1 一致；
- Trust Policy 不重复 hash；
- evidence_validated 在 Critic/Conflict 条件不足时保持 probation。

## 六、Reasonix Agent 执行提示词

```text
执行 MEM-02 Schema Convergence，只修改文档和 docs gate，不修改产品代码。

读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md
- tests/docs_check.sh

严格按 Schema Convergence 文档的 D1～D12 决议修订 Protocol Extension 设计：
1. 沿用 Judgment v1 Envelope；
2. Critic Review 增加正式设计分支；
3. Provenance 内嵌 MemoryEvidenceGeneration；
4. Trust 枚举对齐 Architecture v1；
5. RetrievalEvaluation Fact 不复制 Judgment 结果；
6. Context 保 exact、引用 condition_id；
7. 不新增 Candidate Universe；
8. 补 MemoryUsage 锚字段设计；
9. 增加 Docs Gate 防回归。

只允许修改：
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md 的状态行
- tests/docs_check.sh

禁止修改 Architecture v1、MEM-01A～MEM-01F、internal/memory、CLI、Prompt、Reasonix、Desktop。禁止提交、推送、Tag。

完成后：
- git diff --check
- bash tests/docs_check.sh
- 重跑只读 Schema Gate
- 输出 C1～C19 关闭表、D1～D12 落地表、仍存在的阻塞项
- Gate 不是 PASS 时不得进入代码实现
```
