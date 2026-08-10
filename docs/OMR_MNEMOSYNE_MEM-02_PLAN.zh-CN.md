# OMR Mnemosyne MEM-02：评估、信任与再验证

- 阶段：MEM-02
- 状态：待执行
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
