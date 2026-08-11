# OMR Mnemosyne MEM-02 Protocol Extension：评估协议扩展设计

- 阶段：MEM-02 Protocol Extension
- 状态：✅ 设计定案（2026-08-11，Schema Convergence 已批准）——正文按 D1～D12 修订，C1～C19 全部关闭，只读 Schema Gate 复核 PASS；代码实现待 CTO 另行批准
- 前置：MEM-01A～MEM-01F、MEM-02-01/02/06/07/08 已完成
- 目的：为 MEM-02-03/04/05 及 Critic 条件定义可实现、可审计、可兼容的协议

## 一、设计原则

1. 不修改已有事实，不覆盖旧 Revision、Judgment、Evidence 或 Generation。
2. 所有新增对象均为严格 Schema、不可变、带程序计算的 `content_sha256`。
3. 所有引用必须包含 Scope、ID 和 Hash；缺失、跨 Scope、Hash 漂移、未知字段均 fail closed。
4. 派生状态仍只能从规范事实确定性重建，不产生第二事实源。
5. 历史旧数据继续可读；缺少新协议字段时返回 `unavailable`/`insufficient_evidence`，不得猜测通过。
6. 不调用模型、不联网、不使用向量数据库、不自动批准、不自动修改 Revision 或 CURRENT。

## 二、统一引用与版本规则

### 2.1 Judgment 扩展公共字段

新增 subtype 必须复用现有 `JudgmentFact` Envelope：

```yaml
schema_version: 1
judgment_id: judgment_...
judgment_type: ...
scope: project | global
subject: ...
source: ...
basis_refs: [...]
supersedes_judgment_ref: null | JudgmentRef
content_sha256: sha256_...
created_at: RFC3339
```

约束：

- **新增 subtype 沿用 `schema_version: 1` Envelope**：联合 Schema 的显式扩展（扩 `JudgmentType` 枚举 + 新增 payload 分支），不 bump 到 v2，不把未知字段塞进旧分支；`schema_version: 1` 的旧 Judgment 继续按旧联合读取；
- 新 subtype 必须带判别 payload，其他 payload 分支必须缺失；
- `supersedes_judgment_ref.judgment_type` 必须与当前 Judgment 实际类型一致（目标按实际注册类型比对）；
- Subject、Scope 和 supersede 链逐节点精确匹配；
- 任何环、孤儿或跨 Scope 引用均返回 `unavailable` 或一致性错误；
- 旧读取器遇到未注册 `judgment_type` 必须安全拒绝（`DecodeStrict` + `JudgmentType.Validate`），不得降级成普通 Judgment。

### 2.2 Generation 固定引用

所有评估历史必须固定到同一 Generation Pair（不创造通用 `GenerationRef`）：

```yaml
evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
```

规则：

- 复用 MEM-02-01 冻结的 `ProjectGenerationRef`/`GlobalGenerationRef`（`scope + generation_id + input_manifest_sha256`）；某 Scope 当时没有 Memory 时对应 ref 显式为 `null`；
- 一次评估固定同一 Generation Pair，不得读取未来 CURRENT 评价历史；
- Generation 被清理时只能使用永久 Input Manifest 精确重建，否则返回 `unavailable`；
- 第一版不新增候选世界对象；漏检复查使用固定 Generation Pair + `evaluation_scope`（D7，未来有基准证据后再提架构 amendment）。

## 三、Critic Review Judgment

### 3.1 目的

满足 `evidence_validated` 的“Critic 通过”条件，但不把 Evidence Trust 或程序校验冒充 Critic。

### 3.2 Subtype

正式设计分支（D1 已批准；联合 Schema 显式扩展，沿用 v1 Envelope）：

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
basis_refs: [MemoryRef | EvidenceRef | JudgmentRef]
```

硬约束：

- `passed` 必须有非空 `required_evidence_refs` 和至少一个合法 BasisRef；
- `failed` 不得晋升 active；`unavailable` 保持 probation；
- `critic_source` 禁止写 Prompt、思考、命令和自由长文本；
- 不允许模型自报 `passed` 作为唯一依据（source 只允许确定性 Fixture、离线规则或人工 Review）；
- `critic_review` 只能评价固定 Generation Pair（同 Scope、同 Revision、同 Generation Pair），不能评价未来状态；
- supersede 链逐节点类型和 Subject 精确匹配；
- `content_sha256` 由程序计算，禁止手工填写。

### 3.3 Lifecycle 影响

`evidence_validated` 只有同时满足以下条件才可 active：

- 至少 3 个独立 EvidenceRef；
- 至少 2 个独立 `root_task_refs`；
- 无未解决冲突（D12：冲突事实协议冻结前该条件视为不满足）；
- 存在合法、同 Scope、同 Revision、同 Generation Pair 的 `critic_review.result=passed`。

否则保持 `probation`。冲突事实协议冻结前，`evidence_validated` 持续保持 `probation`（与 MEM-02-02 只读验证器语义一致）。

## 四、Evidence Provenance 与 Trust Judgment

### 4.1 目的

验证证据的来源、获取方式、验证状态和 Provenance；Trust 不等同于 Critic，也不直接改变 Lifecycle。

### 4.2 Provenance 维度（内嵌 MemoryEvidenceGeneration）

不新建独立 Trust Fact、不修改旧 EvidenceRef。在 `MemoryEvidenceGeneration` 中追加可选兼容字段（D3）：

```yaml
evidence_origin: runtime | user | official | project | external
acquisition_method: direct | tool_observed | model_extracted | imported
verification_status: verified | confirmed | inferred | unverified
provenance_refs: [EvidenceRef]
contains_instructional_content: boolean
contains_sensitive_content: boolean
```

枚举与字段名与 Architecture v1（6.2.3:765-775）严格一致（D4）：`evidence_origin`/`acquisition_method`/`verification_status`；Content 分类复用既有 `contains_instructional_content`/`contains_sensitive_content` 布尔与 Content Classifier Policy（D5），不新增 content_class 枚举。

兼容策略：

- 旧 Evidence Generation 缺字段时仍可读取；缺任一 Trust 必需维度时，Trust 派生为 `unavailable`，不得猜测补默认来源；
- 新写入记录必须完整携带四维度；
- 四维度与布尔进入 Canonical Hash；`provenance_refs` 排序去重并闭合到合法 EvidenceRef（与 `evidence_refs` 并集）；
- Trust Policy 使用既有 `PolicyRef`（`policy_id + policy_type + content_sha256`），不重复保存 hash（C11 消解）；
- Trust Gate 是派生结果，不是新 Fact（架构 5.3）。

### 4.3 派生结果

```
trusted | restricted | unverified | blocked
```

安全根（D4 决议范围 + Convergence 3.3 安全规则，Policy 安全根不可关闭）：

- `external + unverified + instructional` → `blocked`；
- Secret/Sensitive class 永远不得参与 Promotion（`PolicyConfigContentClassifier.secret_classes_block_promotion=true` 冻结）；
- Trust Policy 的安全要求不可关闭；
- 缺 Provenance、Policy 不匹配或 Hash 漂移 → `blocked`/`unavailable`；
- Trust 结果是派生状态，不直接把记忆变成 active（不改变 Lifecycle）。

## 五、Retrieval Evaluation

### 5.1 目的

对固定 Generation 中的检索命中、误命中和漏检进行可重放评价。

### 5.2 双对象模型（Fact + Judgment，D6）

采用架构 6.2.3 双对象模型：`RetrievalEvaluationFact` 保存固定评估世界；`retrieval_relevance` Judgment 保存结果与候选引用。**Fact 不复制 Judgment 的结果、候选引用与判断来源**。

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

Retrieval Relevance Judgment（沿用架构 6.2.3:726-737 冻结 payload，不扩展）：

```yaml
judgment_type: retrieval_relevance
subject:
  subject_type: retrieval
  retrieval_id: retrieval_...
retrieval_relevance:
  result: hit_relevant | hit_irrelevant | missed_relevant | no_relevant_memory | unknown | unavailable
  expected_memory_refs: [MemoryRef]
  retrieved_memory_refs: [MemoryRef]
  evidence_refs: [EvidenceRef]
```

约束：

- 判断来源不重复保存：只存在于 Judgment 的 `source`（`fixture_oracle | retrieval_critic | user_review`，架构 6.2.3:724）；只有 `fixture_oracle` 可作为 Benchmark 真值；
- `JudgmentSubject` 为检索审计增加向后兼容的 `retrieval` 分支，且只携带受控 `retrieval_id`；旧 Subject 分支的 Canonical Bytes/Hash 不变，新建并由 RetrievalEvaluation 引用的 Judgment 必须使用该分支，禁止用单条 Memory 或 Outcome 冒充整次检索；
- `memory_context` 必须固定同一 Generation Pair，某 Scope 无记忆时显式 `null`；禁止用未来 Generation 评价历史；
- expected/retrieved 引用必须属于固定 Generation；`missed_relevant` 用固定 Generation Pair + `evaluation_scope` 承载候选世界（D7：第一版不新增候选世界对象）；
- Evaluation 与 Judgment 内容必须一致（`judgment_ref` 指向该次评估对应的 retrieval_relevance Judgment）；
- Generation 清理后无法重建 → `unavailable`，不能猜测；
- `retrieval_relevance` 只触发 Alias、Relation、Index 或 Query 路由的候选修复，不直接修改 Memory Revision 内容（架构 6.2.3:737）。

## 六、Context Applicability Judgment

### 6.1 Subtype

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

约束（D8/D9 决议）：

- `result` 保留 `exact | applicable | conditionally_applicable | not_applicable | unknown`，不新增 `unavailable`，`exact` 不得并入 `applicable`；
- `basis_context_refs` 使用受控 ID 字符串数组（排序去重），Scope 由 Evaluation Context 约束；不创造 `ContextRef` 类型；
- `conditionally_applicable` 必须引用当前目标 Memory Revision 已存在的 `ApplicabilityCondition.condition_id`（`required_condition_ids`，禁止内联条件对象）；
- 不重复 `memory_ref`（仅存在于 `subject`）；删除 payload 内联 `conditions`；
- 不引入 Source Context 单引用模型（判断依据只能来自 `basis_context_refs`）；
- 目标 Context、Basis Context 和 MemoryRef 必须 Scope 一致；
- 结果只影响检索适用性排序，不修改 MemoryRevision（架构 6.2.3:758）。

### 6.2 MemoryUsage 锚字段（D11，仅设计不实现）

补齐架构 12.2 锚字段设计（冻结新增字段，不立即实现、不修改 MEM-01F 冻结实现）：

```yaml
retrieval_id: controlled_id
root_task_id: controlled_id
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
context_signature_version: 1
context_signature: sha256_...
context_descriptor_ref: controlled_id
observation_provenance: {source: agent_reported | runtime_observed | user_confirmed, evidence_ref | null, judgment_ref | null}
```

规则：

- `observation_provenance` 沿用 Architecture v1 12.2 冻结结构（`source` 固定 `agent_reported | runtime_observed | user_confirmed`；`agent_reported` 必须引用承载结构化回执的 Reasonix Event EvidenceRef，`runtime_observed` 必须引用 Reasonix 公开事件 EvidenceRef，`user_confirmed` 必须引用 Confirmation JudgmentRef；前两类的 `judgment_ref` 必须为 null）；
- `retrieved/read/adopted/affected/evaluated` 各阶段均绑定同一 `retrieval_id` 与 Generation Pair；
- 缺锚字段的 legacy usage 只能用于基础 `usage_count`，不得用于 Retrieval Evaluation、Critic 或跨 Context 归因；
- 不用 `usage_id` 代替 `root_task_id`/`retrieval_id`；
- Context Signature Hash 的描述符引用沿用 Architecture v1 已冻结规则（12.2）；
- 各阶段仍保持 MEM-01F 冻结语义（仅 `affected/evaluated` 计入归因；external 不归属）。

## 七、兼容与迁移

1. MEM-01A v1 Judgment 继续可读；未注册 `judgment_type`（含未来新 subtype）在旧读取器中必须安全拒绝，不得降级成普通 Judgment。
2. 新 Fact（RetrievalEvaluation、扩展后的 MemoryEvidenceGeneration 字段）使用新增 FactKind 和独立目录；旧目录不迁移、不覆盖。
3. `omr upgrade` 暂不接入；协议实现完成后再单独设计迁移工具。
4. 旧 Evidence 缺少 Provenance/锚字段时输出 `unavailable`（legacy usage 仅限基础 `usage_count`），不补默认来源。
5. 新协议对象必须支持 `Get/List/Diagnose`、Scope 隔离、Hash 校验、幂等和损坏诊断。
6. `critic_review` 与 `basis_context_refs` 等扩展在冻结实现前不写入任何生产路径；本阶段仅为设计契约。

## 八、Schema Gate

进入代码实现前必须全部满足：

- 四类对象的 JSON Schema/Go Schema 已明确（本设计修订版）；
- Fact 与 Judgment 的选择已明确（本设计各章 + 第十一章关闭摘要）；
- 每个字段的必填、枚举、Hash、Scope 和引用关系已明确；
- 旧数据兼容策略已明确（legacy/unavailable 规则）；
- Lifecycle 影响矩阵已明确（11.5 + 3.3/4.3）；
- 不再存在“把 Trust 当 Critic”“未来 Generation 评价历史”“自由文本条件”等歧义；
- `tests/docs_check.sh` 能检查关键术语和禁止项（第五章 + 11.5）。

## 九、交给 Reasonix Agent 的下一阶段提示词

```text
执行 OMR Mnemosyne MEM-02 Protocol Extension 实现（Schema Convergence 已批准）。

先读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md（修订版，D1～D12 已定案）
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md
- internal/memory/**

按修订后设计实现（每项先写失败测试）：
1. `critic_review` Judgment subtype（扩 JudgmentType 枚举 + payload 分支 + subject=memory_revision + memory_context + required_evidence_refs + source 枚举）；
2. `MemoryEvidenceGeneration` 追加 evidence_origin/acquisition_method/verification_status/provenance_refs/两布尔（进 Canonical Hash，legacy 缺字段兼容）；
3. `RetrievalEvaluationFact`（新 FactKind + FactStore 路由/校验/幂等/Hash/Get/List/Diagnose）；
4. `context_applicability` 增加顶层 `basis_context_refs` 受控 ID 数组（subject 不变，result 保 exact）；
5. MemoryUsage 锚字段设计落地（D11 冻结字段）。

严格禁止：修改 Architecture v1、MEM-01A～F 冻结 Schema 的既有字段/枚举、CLI、Prompt、Reasonix、Desktop、自动批准、CURRENT/Revision 自动写入、真实模型调用、网络、Embedding、向量数据库、提交、推送、Tag。候选世界对象不新增；RetrievalEvaluation Fact 不保存结果/候选引用/判断来源；Context 不内联条件对象；Trust Policy 不重复 hash；冲突事实协议冻结前 evidence_validated 保持 probation。

完成后运行：gofmt -w internal/memory、git diff --check、GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...、go test -race、go test ./...、go vet ./...、go build ./cmd/omr、bash tests/docs_check.sh。输出交付报告并明确 Gate 状态；不得提交推送。
```

## 十一、Schema Convergence 最终决议（2026-08-11）

> 本设计第一～九章为**当前唯一有效 Schema**。初轮 Schema Gate 审核记录已移入 `OMR_MNEMOSYNE_MEM-02_SCHEMA_GATE_AUDIT.zh-CN.md`（历史归档，不作为当前协议输入）。以下为 C1～C19 关闭摘要、D1～D12 最终决议与当前真实阻塞项。

### 11.1 C1～C19 关闭摘要（全部关闭）

| 冲突 | 关闭方式（本设计文档位置） |
|---|---|
| C1 Judgment schema_version 扩展机制 | 2.1 沿用 v1 Envelope（联合显式扩展） |
| C2/C12 通用 Generation 引用类型 | 2.2/3.2/5.2/6.2 统一 `memory_context`（复用 Project/GlobalGenerationRef） |
| C3 critic_review 未注册 | 3.2 正式设计分支（D1 批准扩枚举 + payload 分支） |
| C4 critic source 枚举 | 3.2 `fixture_oracle|offline_rule|user_review` |
| C5 Trust 载体 | 4.2 删除独立 Trust Fact，Provenance 内嵌 `MemoryEvidenceGeneration` |
| C6～C8 Provenance 三枚举 | 4.2 采用架构冻结值（evidence_origin/acquisition_method/verification_status） |
| C9 content 类表示 | 4.2 复用 bool 对 + Content Classifier Policy |
| C10 Provenance 引用类型 | 4.2 `provenance_refs: [EvidenceRef]` 数组 |
| C11 Trust Policy hash 冗余 | 4.2 只用既有 PolicyRef |
| C13/C14 Retrieval Fact 职责 | 5.2 双对象模型：Fact 不保存结果/候选引用/判断来源 |
| C15 Retrieval Fact 缺字段 | 5.2 补 `retrieval_id`、`judgment_ref` |
| C16 Context result 枚举 | 6.1 保 `exact`，不新增 unavailable |
| C17 payload 重复 memory_ref | 6.1 删除（仅 subject 持有） |
| C18 Context 内联条件 | 6.1 改 `required_condition_ids` 引用 |
| C19 basis_context_refs 缺口 | 6.1 顶层受控 ID 数组 |

### 11.2 D1～D12 最终决议（全部定案，唯一值）

| 决策 | 最终决议 | 落地位置 |
|---|---|---|
| D1 | 新增 `critic_review`，result=`passed|failed|unavailable` | 3.2 |
| D2 | 沿用 v1 Envelope，不 bump v2 | 2.1 |
| D3 | Provenance 四维度内嵌 `MemoryEvidenceGeneration` | 4.2 |
| D4 | 三枚举采用架构 v1 冻结值 | 4.2 |
| D5 | Content 复用 bool + Classifier Policy | 4.2 |
| D6 | RetrievalEvaluation 双对象，Fact 不复制 Judgment 结果 | 5.2 |
| D7 | 不新增候选世界对象（固定 Generation Pair + evaluation_scope） | 2.2/5.2 |
| D8 | Context 保 exact、不加 unavailable | 6.1 |
| D9 | basis_context_refs 受控 ID 数组 | 6.1 |
| D10 | Critic 用完整 memory_context（一侧可 null） | 3.2 |
| D11 | MemoryUsage 锚字段设计定案（仅设计，不实现） | 6.2 |
| D12 | 冲突事实协议冻结前「无未解决冲突」视为不满足，evidence_validated 保持 probation | 3.3 |

### 11.3 当前真实阻塞项（属实现阶段，不阻塞 Schema Gate）

1. **冲突事实协议**（D12）：`evidence_validated` 的「无未解决冲突」条件需未来 Conflict Fact 协议冻结后才能满足；冻结前该 Policy 持续 `probation`（保持 MEM-02-02 只读验证器语义）。
2. **MemoryUsage 锚字段实现**（D11）：`retrieval_id/root_task_id/memory_context/context_signature/observation_provenance` 仅完成设计，需另行批准后修改 MEM-01F 冻结实现。
3. **`basis_context_refs` 实现层扩展**（C19/D9）：需在 `JudgmentFact`/`ContextApplicabilityPayload` 追加顶层字段（改变 Canonical Hash），实现前需 CTO 批准「实现修订」。

### 11.4 Gate 状态（唯一）

- **Schema Gate：PASS** —— 主协议文档与 Architecture v1、MEM-01A～01F 冻结实现无 Schema 冲突；C1～C19 全部关闭、D1～D12 全部定案、无矛盾、无待定项。
- 初轮审核为历史记录，见 `OMR_MNEMOSYNE_MEM-02_SCHEMA_GATE_AUDIT.zh-CN.md`，不作为当前协议输入。
- 代码实现阶段仍需 CTO 明确批准（第九章提示词）；Gate 状态非 PASS 不得进入实现。
