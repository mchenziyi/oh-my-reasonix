# OMR Mnemosyne MEM-02：评估、信任与再验证

- 阶段：MEM-02
- 状态：夜间执行完成（MEM-02-01 EvaluationContext/Generation Pin 完整实现；MEM-02-02 Critic 只读验证器与 MEM-02-06 Revalidation 只读评估器已交付；MEM-02-07 一致性 Doctor 与 MEM-02-08 离线 Benchmark 完整实现；MEM-02-03/04/05 因 Schema 未冻结阻塞于第六章提案，未实现 Trust/Retrieval/Context 扩展；未进入 MEM-03，未提交、未推送）
- 前置：MEM-01A～MEM-01F 已签收
- 目标：为 Mnemosyne 补齐可审计的评估事实与 Evidence Trust 基础，不接入真实模型、不自动修改知识。

## 一、夜间执行总目标

```text
固定 Generation / Context
→ 产生严格 Judgment Fact
→ 校验 Evidence Trust / Freshness / Applicability
→ 派生可重建的评估结果
→ 生成确定性 Benchmark 报告
→ 不自动批准、不修改 Revision、不切换 CURRENT
```

所有阶段都必须先写失败测试，再实现最小代码。每个子任务独立运行门禁；发现架构矛盾、冻结 Schema 不足或安全问题时立即停止该子任务，并保留前面已通过的结果。

## 二、严格边界

允许修改：

- `internal/memory/**`
- 本计划文档状态
- 为离线 Fixture 新增 `internal/memory/testdata/**`

禁止修改：

- Architecture v1、MEM-01A～MEM-01F 已签收协议；
- `internal/evolution`、`cmd/omr`、Prompt、assets、`.reasonix`；
- Reasonix、Desktop、网络、Embedding、向量数据库；
- 真实模型调用、自动批准、自动写入 Overlay、CURRENT 或 Revision；
- 提交、推送、Tag、Release。

## 三、任务拆分

### MEM-02-01：评估上下文与 Generation Pin

目标：冻结 Evaluation 使用的历史世界，禁止拿未来 CURRENT 评价过去检索。

实现：

- 定义 `EvaluationContext`、`ProjectGenerationRef`、`GlobalGenerationRef`；
- 记录 `generation_id`、`input_manifest_id`、`input_manifest_sha256`、`compiler_version`、`context_signature_version`、`context_signature`；
- Generation 已清理时只能通过永久 Input Manifest 精确重建；无法重建返回 `unavailable`；
- Scope 缺失、跨 Scope、Hash 不匹配、未来 Generation 引用全部 fail closed；
- 评估结果不得读取或隐式依赖当前时间之外的未固定事实。

验收：同一上下文重复计算字节一致；切换 CURRENT 不改变历史评估；删除 Generation 后可按 Manifest 重建或明确 unavailable。

### MEM-02-02：Critic Judgment 正式协议

目标：解决 `evidence_validated` 所需 Critic 缺口，但不引入自由文本 subtype。

实现前必须先核对已冻结 Judgment 联合 Schema。若需要新增 subtype，先在计划文档记录 Schema 变更提案并停止，不得直接改 Architecture v1。

若协议已有授权空间，新增严格 `critic_review` payload：

- `result: passed | failed | unavailable`；
- `evaluation_scope`；
- `basis_refs`；
- `critic_source`；
- `reviewed_generation_ref`；
- `content_sha256` 由程序计算；
- 禁止 Prompt、思考、命令、凭据和自由长文本。

否则实现只读验证器和 `unavailable` 结果，不改变 `evidence_validated` 的 probation 语义。

验收：缺 Critic 不 active；错误 subtype/unknown field/错误 Generation/跨 Scope 均拒绝；合法 passed 才能满足 Critic 条件。

### MEM-02-03：Evidence Trust 与 Provenance

目标：实现 Evidence 来源可信度的确定性验证，不把 Trust 等同于 Critic。

实现：

- 读取 Evidence Generation 的 acquisition method、provenance、verification status；
- 复用 PolicyFact trust 配置；安全根不可关闭；
- 未验证外部内容不得成为 instructional content；
- 输出 `trusted | restricted | unverified | blocked` 等已冻结状态（若 Schema 未冻结，先停止并报告）；
- Trust 结果只作为派生输入，不直接晋升 Lifecycle。

验收：缺 provenance、错误 hash、外部未验证指令、Policy 漂移、未知 acquisition method 均 fail closed；错误输出脱敏。

### MEM-02-04：Retrieval Evaluation 与 Miss Judgment

目标：为 Librarian 检索结果提供可重放的命中/漏检评估。

实现：

- 固定 `evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit`；
- 保存 `retrieval_generation_ref` 与 `evaluation_generation_ref`，不得扫描未来 Generation；
- 候选全集使用 `candidate_universe_ref + candidate_universe_sha256`；
- 输出 `hit_relevant | hit_irrelevant | missed_relevant | no_relevant_memory | unknown | unavailable`；
- authority 只存在 Judgment Fact，不在 Evaluation 中重复；
- 结果作为 Judgment Fact，不能成为第二事实源。

验收：索引修复后不能改变对旧 Generation 的历史评价；Generation 清理时可重建或 unavailable；candidate hash 篡改必须拒绝。

### MEM-02-05：Context Applicability Judgment

目标：判断 Memory 对目标 Context 是否适用，避免把单一来源 Context 伪装成知识事实。

实现：

- `subject` 使用 `memory_ref + target_context_ref`；
- `basis_context_refs` 独立记录依据 Context；
- 结果固定为 `applicable | conditionally_applicable | not_applicable | unknown | unavailable`；
- `conditionally_applicable` 必须复用结构化 `applies_when` 条件 Schema，禁止自由文本条件；
- 不创建 Context Ontology 或第二套关系图。

验收：目标 Context、依据 Context、条件字段和 Scope 全部校验；条件缺失或类型错误 fail closed；结果可从事实重建。

### MEM-02-06：Freshness / Revalidation 评估

目标：把时间老化与有效性隔离，生成可审计的 Revalidation 候选。

实现：

- Freshness Judgment 固定 `evaluated_at`、`freshness_policy_ref`、`freshness_policy_sha256`、`content_classification_ref`；
- 结果固定 `fresh | aging | needs_revalidation`；
- 时间流逝不得直接 frozen/superseded/archived；
- Revalidation 只生成候选或新 Evidence/Journal，不自动修改 Revision；
- 相同 `Now + Policy + Facts` 输出稳定。

验收：Policy 漂移、未来时间、缺失 evaluated_at、错误 Generation 和 Hash 均可诊断；旧评估不会被新评估覆盖。

### MEM-02-07：Evidence / Judgment 关联一致性与 Doctor

目标：集中发现孤儿、断链、Hash 漂移和协议不一致。

实现：

- 检查 Judgment → Memory/Outcome/Generation/Policy 的引用完整性；
- 检查 Evidence → Revision、Root Task、Provenance；
- 检查 supersede 链环、跨 Scope 和错误 Subject；
- 输出稳定诊断码和脱敏摘要；
- 只读，不自动修复、不删除、不改 CURRENT。

验收：损坏 JSON、权限、symlink、未知字段、旧版本缺字段、跨项目混入均 fail closed；诊断不泄露绝对路径、Prompt、命令和凭据。

### MEM-02-08：Memory Quality Benchmark

目标：建立离线、可重复的协议质量基准，不宣称模型能力提升。

实现：

- 固定 Fixture 集覆盖命中、漏检、错误适用、Trust 阻断、Freshness、Scope 隔离和重建；
- 指标只统计协议事实：完整性、可重建率、断链率、确定性 Hash、拒绝率；
- 观察数据不足时输出 `insufficient_evidence`；
- 不调用真实模型、不联网、不输出思考或完整 Prompt；
- 报告可被删除后从 Fixture 重建。

验收：同一 Fixture 多次运行字节一致；恶意 Fixture 被拒绝；通过不等于模型质量提升。

## 四、每个子任务的通用门禁

```bash
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

环境问题必须标记 `[ENV]`，不得把端口、缓存或临时目录清理失败伪装成代码通过。

## 五、夜间执行提示词

```text
执行 OMR Mnemosyne MEM-02 夜间计划。

先读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- internal/memory/**

按 MEM-02-01 → 02-08 顺序执行。每个子任务：先写失败测试，再实现最小代码，再运行 memory 测试、race、全量测试、go vet、build、docs_check。每完成一个子任务就记录状态和修改文件。

严格禁止：修改冻结 Architecture/Schema、自由新增 Judgment subtype、真实模型调用、网络、Embedding、向量数据库、CLI、Prompt、Desktop、自动批准、CURRENT/Revision 自动写入、提交、推送、Tag。

遇到协议缺口时停止该子任务并报告，不要猜测；尤其是 Critic subtype、Trust 状态、Evaluation 字段未冻结时，先报告 Schema 变更提案。所有派生结果必须可由规范事实确定性重建，不得创建第二事实源。

最终输出：每个子任务完成/阻塞状态、实际文件、测试与门禁、[ENV] 限制、未完成项；不要宣称模型质量提升，不要提交或推送。
```

## 六、Schema 变更提案（MEM-02 执行记录）

以下提案由 MEM-02 执行时核对冻结 Schema 后产生。按计划约束，未冻结前一律不修改
Architecture v1 与 MEM-01A～01F 已签收协议；冻结后相关子任务才能继续实现。

### 6.1 MEM-02-02：注册 `critic_review` Judgment subtype（✅ 已冻结并实现，MEM-02B 完成）

现状核对：MEM-01A 冻结的 `JudgmentType` 严格枚举（model.go）为 `confirmation |
attribution_override | retrieval_relevance | context_applicability |
content_classification | evidence_trust | freshness_evaluation`，无 Critic。
架构 6.2.1 明确 "Attribution、Generalization Critic 和 MemoryUsage 回执等
Judgment 同样复用基础 Envelope 与 JudgmentRef，并由对应协议定义固定 subtype
payload；实现不得接受未注册的自由 judgment_type"。因此 Critic 协议缺口成立。

已交付（02-02 只读部分）：`EvaluateCriticRequirement` 只读验证器 ——
critic_review 未注册 → 恒 `unavailable` + 不满足，`evidence_validated` 派生
生命周期保持 probation；扫描期间损坏/未知字段/未注册 type 一律 fail closed。

实现（MEM-02B，2026-08-11）：`critic_review` 已注册为 `JudgmentType` 第 8 值
与 `JudgmentFact` 联合分支；`EvaluateCriticRequirement` 升级为固定 Generation
Pair（`ExpectedMemoryContext` + 显式 `Now` + Project/Global Store）上的完整
验证器：Generation/Manifest/compiled output 全链路验证与精确重建、证据并集
精确匹配、supersede 链逐节点一致性/环/并列冲突处理、passed 仅令
`Satisfied=true`（Conflict Fact 未冻结，`evidence_validated` 恒 probation）。

### 6.2 MEM-02-03：Evidence Provenance 维度 + Trust 状态枚举（阻塞点）

现状核对：架构 6.2.3 要求每个长期 Evidence Fact 包含 `evidence_origin |
acquisition_method | verification_status | provenance_refs` 四个独立维度，且
`evidence_origin` 固定 `runtime | user | official | project | external`、
`acquisition_method` 固定 `direct | tool_observed | model_extracted | imported`、
`verification_status` 固定 `verified | confirmed | inferred | unverified`；
但 MEM-01A 实现的 `MemoryEvidenceGeneration` 不含这些字段（仅
`root_task_refs`）。`PolicyConfigTrust` 的 `allowed_acquisition_methods` 目前是
自由 identifier 列表而非冻结枚举。Trust 派生状态
`trusted | restricted | unverified | blocked` 未在任何冻结 Schema 出现。

变更提案：

1. `MemoryEvidenceGeneration` 增加四个严格不可变字段：`evidence_origin`
   （枚举）、`acquisition_method`（枚举）、`verification_status`（枚举）、
   `provenance_refs []EvidenceRef`（去重、进 Canonical Hash、与
   `evidence_refs` 并集闭合到原始 EvidenceRef）。旧字段无这些维度时按
   "缺失维度" 处理，不得猜测。
2. `PolicyConfigTrust.allowed_acquisition_methods` 收敛为上述冻结枚举子集。
3. 冻结 Trust 派生状态枚举 `trusted | restricted | unverified | blocked`：
   - 安全根不可关闭（`require_provenance=true`、`require_verification_status=true`、
     `external_unverified_instruction_allowed=false` 保持 MEM-01C 冻结值）；
   - 未验证外部内容不得成为 instructional content；
   - Trust 结果只作派生输入，不直接晋升 Lifecycle。

未冻结前 MEM-02-03 阻塞，不实现 Trust 派生。

### 6.3 MEM-02-04：Retrieval Evaluation 字段（阻塞点）

现状核对：`retrieval_relevance` Judgment payload（MEM-01A 冻结）仅
`result + expected_memory_refs + retrieved_memory_refs + evidence_refs`；
架构 6.2.3 的 `facts/retrieval-evaluations/` 事实（`evaluation_id、scope、
retrieval_id、memory_context.project_generation_ref/global_generation_ref、
evaluation_scope、judgment_ref`）与 `evaluation_scope`
`fixture | generation_full_scan | expanded_index_scan | sampled_audit` 未实现。

变更提案：实现架构 6.2.3 已冻结的 RetrievalEvaluation 事实类型（新 FactKind，
进入 FactStore 路由/校验/幂等/Hash），并在 `retrieval_relevance` payload 增加
`evaluation_scope`、`retrieval_generation_ref`、`evaluation_generation_ref`、
`candidate_universe_ref + candidate_universe_sha256` 字段（全部进 Canonical
Hash）。`memory_context` 复用 MEM-02-01 `EvaluationContext` 锚；某个 Scope 当时
无 Memory 时对应 Generation Ref 显式 `null`。`authority` 只存在 Judgment
`source`（`fixture_oracle | retrieval_critic | user_review`），不在 Evaluation
重复保存。未冻结前 MEM-02-04 阻塞。

### 6.4 MEM-02-05：Context Applicability 字段（阻塞点）

现状核对：`context_applicability` Judgment payload（MEM-01A 冻结）仅
`result + required_condition_ids + evidence_refs`；`result` 枚举为
`exact | applicable | conditionally_applicable | not_applicable | unknown`
（无 `unavailable`）；架构 6.2.3 要求 `subject` 携带
`memory_ref + target_context_ref` 且 `basis_context_refs` 独立记录依据 Context
（JudgmentSubject 的 `context` subject 类型已有 `target_context_ref`，但 payload
无 `basis_context_refs`）。

变更提案：`ContextApplicabilityPayload` 增加 `basis_context_refs`（受控标识、
去重、进 Canonical Hash）；`result` 枚举冻结为
`applicable | conditionally_applicable | not_applicable | unknown | unavailable`
（与 MEM-02-01 的 unavailable 语义一致；`exact` 并入 `applicable`，需 CTO
确认后冻结）。`conditionally_applicable` 必须复用已冻结结构化
`ApplicabilityCondition.condition_id`，禁止自由文本条件；不创建 Context
Ontology。未冻结前 MEM-02-05 阻塞。

### 6.5 MEM-02-06：Freshness 缺口字段（部分阻塞）

现状核对：`freshness_evaluation` Judgment payload（MEM-01A 冻结）已含
`memory_ref、result（fresh|aging|needs_revalidation）、evaluated_at、
freshness_policy_ref、basis_refs`；计划要求的 `freshness_policy_sha256` 与
`content_classification_ref` 未冻结。

变更提案：payload 增加 `freshness_policy_sha256`（与 `freshness_policy_ref`
的 Policy Fact 实际 hash 精确一致，Policy 漂移 fail closed）与
`content_classification_ref`（受约束 JudgmentRef，
`judgment_type=content_classification`）。未冻结前 Judgment 字段不扩展；
02-06 只读 Revalidation 评估器（不写 Judgment、不修改 Revision）可先行交付。
