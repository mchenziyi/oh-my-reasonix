# OMR Mnemosyne MEM-02D：Retrieval Evaluation 与 Miss Judgment 实现计划

- 阶段：MEM-02D / MEM-02-04
- 状态：✅ 已实现（2026-08-11，待 CTO 签收）
- 前置：MEM-02A～MEM-02C 已签收；MEM-02 Schema Convergence Gate 已通过
- 目标：实现可重放、可审计的检索评价双对象模型，不调用模型、不猜测相关性、不修改 Memory。

## 一、交付边界

```text
固定 Project/Global Generation Pair
→ 严格 retrieval_relevance Judgment
→ RetrievalEvaluation Fact 绑定 retrieval_id + JudgmentRef
→ 只读验证固定候选世界和全部引用
→ 输出 verified | unavailable
→ 不修改 Revision、Relation、Alias、Index、Lifecycle 或 CURRENT
```

本阶段只实现事实结构、存储路由和确定性验证器。`missed_relevant` 是
`fixture_oracle | retrieval_critic | user_review` 提交的结构化 Judgment，不由
Go 程序进行语义推断。生产 Critic 的结论不是 Oracle。

允许修改：`internal/memory/**`、本计划、MEM-02 总计划状态与
`tests/docs_check.sh`。禁止修改 Architecture v1、MEM-01A～F 旧 Canonical
Schema、Evolution、CLI、Prompt、Reasonix、Desktop、CURRENT；禁止模型/网络
调用、自动修索引、提交、推送与 Tag。

## 二、双对象与唯一事实源

### 2.1 RetrievalEvaluation Fact

```go
type RetrievalEvaluation struct {
    SchemaVersion    int           `json:"schema_version"`
    EvaluationID    string        `json:"evaluation_id"`
    Scope           Scope         `json:"scope"`
    RetrievalID     string        `json:"retrieval_id"`
    MemoryContext   MemoryContext `json:"memory_context"`
    EvaluationScope string        `json:"evaluation_scope"`
    JudgmentRef     JudgmentRef   `json:"judgment_ref"`
    ContentSHA256   string        `json:"content_sha256"`
    CreatedAt       string        `json:"created_at"`
}
```

`evaluation_scope` 固定为 `fixture | generation_full_scan |
expanded_index_scan | sampled_audit`。落盘路径为
`facts/retrieval-evaluations/<evaluation-id>.json`。

它是不可变规范 Fact：严格 JSON、程序计算 Hash、同身份同 Hash NOOP、同身份异
Hash 冲突，权限和路径安全复用 FactStore。Evaluation 不得重复保存 Judgment 的
result、expected/retrieved refs、evidence、source 或 authority。

### 2.2 Retrieval Judgment Subject

现有 Envelope 缺少能表示“整次检索”的 Subject，新增向后兼容分支：

```yaml
subject:
  subject_type: retrieval
  retrieval_id: retrieval_...
```

约束：

- `retrieval` 只允许一个受控 `retrieval_id`；
- 不得同时携带 MemoryRef、OutcomeID、EvidenceRef 或 TargetContextRef；
- 新字段 `omitempty`，旧 Subject 与旧 8 类 Judgment 的 Canonical Bytes/Hash
  必须逐字节不变；
- 被 RetrievalEvaluation 引用的 Judgment 必须为 `retrieval_relevance`，Subject
  必须为 `retrieval`，且 retrieval_id 与 Evaluation 一致；
- 禁止用单条 Memory、Outcome 或 Evidence 冒充整次检索。

### 2.3 RetrievalRelevancePayload 不扩展

继续使用冻结 payload：

```yaml
result: hit_relevant | hit_irrelevant | missed_relevant |
        no_relevant_memory | unknown | unavailable
expected_memory_refs: [MemoryRef]
retrieved_memory_refs: [MemoryRef]
evidence_refs: [EvidenceRef]
```

不新增 evaluation_scope、GenerationRef、CandidateUniverseRef 或 authority。

## 三、Store 与 Manifest 路由

新增 `FactKindRetrievalEvaluation = "retrieval-evaluations"`，接入：

- `Fact` 接口；
- `factKey`、`factScope`、`decodeKind`、`allKinds`；
- `factIdentity`、`factSchemaVersion`、`resolveManifestInput`，身份为
  `retrieval_evaluation + evaluation_id`；
- Diagnose、List、Get、Put 的完整验证链。

路径正文身份必须一致：读取 `evaluation-id.json` 后正文 EvaluationID 必须等于
路径身份，不得重演 Generation/Policy 的路径身份缺口。

## 四、只读验证器

```go
type RetrievalEvaluationStatus string

const (
    RetrievalEvaluationVerified    RetrievalEvaluationStatus = "verified"
    RetrievalEvaluationUnavailable RetrievalEvaluationStatus = "unavailable"
)

type RetrievalEvaluationRequest struct {
    Scope         Scope
    EvaluationID string
    ProjectStore *FactStore
    GlobalStore  *FactStore
    Now           time.Time
}

type RetrievalEvaluationResult struct {
    Status         RetrievalEvaluationStatus `json:"status"`
    EvaluationID   string                    `json:"evaluation_id"`
    RetrievalID    string                    `json:"retrieval_id"`
    JudgmentResult string                    `json:"judgment_result"`
    SourceType     string                    `json:"source_type"`
    EvaluatedAt    string                    `json:"evaluated_at"`
}

func ValidateRetrievalEvaluation(
    ctx context.Context,
    store *FactStore,
    req RetrievalEvaluationRequest,
) (*RetrievalEvaluationResult, error)
```

结果为派生数据，不持久化；提供稳定 `EncodeCanonical()`。

## 五、固定世界验证

验证顺序：

1. 验证请求、Scope、EvaluationID、显式 Now；零值 Now 拒绝。
2. 按 EvaluationID 精确读取 Fact，比较路径与正文身份。
3. 验证 Evaluation.created_at <= Now。
4. 验证 MemoryContext；每个非空侧必须有对应 Scope 的 Store。
5. 每个 GenerationRef 复用 `resolveGenerationWorld`/永久 Manifest 重建链，精确
   验证 Generation、Manifest、Hash、Compiler、compiled output、Scope 与时间；
   禁止读取 CURRENT。
6. Generation 清理后可重建则继续；不可重建返回 `unavailable`，不得猜测。
7. 按 JudgmentRef 精确读取 Judgment；验证 scope/type/id/hash、时间、retrieval
   Subject 与 retrieval_id。
8. source 仅允许 `fixture_oracle | retrieval_critic | user_review`，且只存在于
   Judgment。
9. expected/retrieved MemoryRef 必须精确属于固定 Generation Pair：ref.scope
   决定 Project/Global 侧；Revision 必须由对应永久 Manifest 的
   `memory_revision` 输入引用；实际 Revision 五字段必须完全匹配。
10. evidence_refs 必须在对应 Scope Store 中由不可变
    MemoryEvidenceGeneration 精确闭合；缺失、跨 Scope、Hash 漂移 fail closed。

结构损坏、未来事实、跨 Scope与引用错配返回稳定错误；只有固定世界确实无法恢复
时返回 `unavailable`。

## 六、Judgment 规则

- linked Judgment 必须 `retrieval_relevance`，JudgmentRef 四字段完全一致；
- fixture_oracle 才可作为 Benchmark 真值；验证器只报告 source，不制造第二结论；
- result 与引用只读取、不重新推断；
- `missed_relevant` 不直接修改 Alias、Relation、Index、Query 或 Revision，只能
  作为后续 Proposal 的候选证据；
- Supersede 不改写历史 Evaluation：修订结果创建新 Judgment 和新 Evaluation。

## 七、安全与确定性

- 全流程只读、零写入；
- 不记录 Prompt、查询正文、命令、模型思考、绝对路径或凭据；
- 错误不得回显攻击者提供的 ID/source；
- 引用列表上限沿用 `maxPayloadRefs`；重复引用拒绝，乱序不改变 Hash；
- 相同 Facts、MemoryContext 与 Now 输出字节一致；不同 Now 只改 evaluated_at；
- 无随机数、`time.Now()`、CURRENT、网络或模型调用。

## 八、TDD 测试矩阵

### 8.1 Schema 与兼容

1. RetrievalEvaluation round-trip、Hash、Put/Get/List/NOOP；
2. identity conflict 零覆盖；
3. evaluation_scope 四合法值和未知值拒绝；
4. retrieval Subject 合法、缺 ID、夹带其他字段、路径值拒绝；
5. 旧 Subject 与旧 8 类 Judgment golden 逐字节不变；
6. unknown field、错误 hash、路径正文身份错配、symlink/权限拒绝。

### 8.2 固定世界与引用

7. Project-only、Global-only、Project+Global 三种合法世界；
8. CURRENT 切换后历史评价不变；
9. Generation 清理后精确重建；不可重建→unavailable；
10. 未来 Generation/Manifest/Evaluation/Judgment fail closed；
11. JudgmentRef scope/type/id/hash 任一错配拒绝；
12. retrieval_id 不一致拒绝；三种 source 合法、其他 source 拒绝；
13. expected/retrieved ref 不在固定 Generation，或 type/revision/hash 错误拒绝；
14. null Scope 侧被引用时 unavailable；
15. evidence ref 缺失、跨 Scope、Hash 漂移拒绝；
16. result 六值保持原样，不被程序重解释。

### 8.3 安全与隔离

17. 重复引用拒绝；列表乱序 canonical 字节一致；
18. 结果稳定编码；不同 Now 只改变 evaluated_at；
19. 前后 Store 文件数、DerivedState、CURRENT 不变；
20. 错误脱敏、context 取消、损坏无关 Fact 的 fail-closed 边界；
21. Fake fixture：固定 Generation→Judgment→Evaluation→verified，无 API Key。

## 九、门禁

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

沙箱禁止 `httptest` 监听时记录 [ENV] 并在允许端口的环境复核，不伪造通过。

## 十、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-02D：Retrieval Evaluation 与 Miss Judgment。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 5.3、6.2.3、11、17 章；
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md 第五章；
- docs/OMR_MNEMOSYNE_MEM-02D_RETRIEVAL_EVALUATION_PLAN.zh-CN.md；
- internal/memory/model.go、store.go、generation_tx.go、evaluation_context.go、
  usage_anchors.go、critic_requirement.go。

严格执行计划第二～九章，每阶段先写失败测试并记录修复前证据，再做最小实现。

硬约束：
1. 使用 RetrievalEvaluation Fact + retrieval_relevance Judgment 双对象；Fact 不复制 result/refs/source。
2. JudgmentSubject 新增向后兼容 retrieval 分支，只携带 retrieval_id；旧 golden 不变。
3. 不扩 RetrievalRelevancePayload，不新增 Candidate Universe/authority/Generation 字段。
4. 固定 Project/Global Generation Pair，禁止 CURRENT；清理后仅按永久 Manifest 重建。
5. expected/retrieved MemoryRef 属于固定 Generation 且五字段精确匹配；EvidenceRef 精确闭合。
6. source 仅 fixture_oracle|retrieval_critic|user_review；Go 不做语义相关性判断。
7. 只读零写入，不改 Lifecycle/Revision/Relation/Alias/Index/Prompt/CURRENT，不进入 MEM-02E。
8. 所有身份同时验证路径与正文，错误稳定脱敏。
9. 覆盖第八章测试，运行第九章门禁并执行独立 review/security_review。

若冻结架构与计划冲突，停止相关实现并报告原文位置，不自行扩大 Schema。

最终只输出：实际文件；双对象 Schema；retrieval Subject 兼容策略；固定世界与
引用验证；来源规则；失败测试证据；门禁/review/security_review；[ENV]/剩余问题；
明确“未进入 MEM-02E、未提交、未推送、未创建 Tag”。
```

## 十一、完成定义

全部测试与门禁通过、无 blocking/should-fix、文档状态同步后才可签收。签收前不得
把 `missed_relevant` 接入自动索引修复或自进化 Proposal。

## 十二、实施记录

本节是计划执行日志，不修改第二～九章的冻结协议。每个阶段必须先出现失败测试，
再实现最小代码，最后记录验证结果。

| 阶段 | 状态 | 成功标准 |
|---|---|---|
| D-01 Retrieval Subject | ✅ 完成 | `retrieval_id` 严格联合分支；旧 Subject/Judgment golden 不变 |
| D-02 RetrievalEvaluation Fact | ✅ 完成 | 严格 Schema、Canonical、Hash、Store round-trip/NOOP/冲突 |
| D-03 Manifest/Store 路由 | ✅ 完成 | FactKind、身份、Scope、Manifest 解析对称且路径正文一致 |
| D-04 固定世界验证器 | ✅ 完成 | Project/Global Pair、历史重建、显式 Now、禁止 CURRENT |
| D-05 引用闭合与来源 | ✅ 完成 | Memory/Evidence/Judgment 精确引用；三种 source；错误脱敏 |
| D-06 隔离与确定性 | ✅ 完成 | 只读零写入、稳定结果字节、不改 Lifecycle/CURRENT |
| D-07 文档与门禁 | ✅ 完成 | 第九章全部通过；无遗留 should-fix |

### 12.1 第一轮失败测试证据

在产品实现前新增 Retrieval Subject、RetrievalEvaluation 与 FactStore 测试，当前基线
按预期编译失败：`RetrievalEvaluation`、`FactKindRetrievalEvaluation` 未定义，且
`JudgmentSubject` 尚无 `RetrievalID`。这证明测试覆盖的是新增契约，而不是既有行为。

### 12.2 当前手术式改动边界

仅允许继续修改 `internal/memory/**`、本计划、MEM-02 总计划状态与
`tests/docs_check.sh`。不得修改 Architecture v1、MEM-01A～F 历史协议、CLI、Prompt、
Reasonix、Desktop 或 Evolution；不得提交、推送或创建 Tag，直至 CTO 验收。

### 12.3 实现与验证结果

- 新增 `RetrievalEvaluation` 不可变 Fact、`FactKindRetrievalEvaluation`、Manifest
  身份路由以及 Retrieval Subject；旧 Subject/Judgment Canonical 兼容测试保持通过。
- `ValidateRetrievalEvaluation` 只读验证 Evaluation、固定 Project/Global Generation
  Pair、永久 Manifest、JudgmentRef、MemoryRef 和 EvidenceRef；只返回
  `verified | unavailable`，不读 CURRENT、不写事实。
- 覆盖 Project-only、Project+Global、Generation 清理后重建、不可重建、未来 Fact、
  路径正文错配、非法 source、retrieval_id 错配、重复引用、零写入和稳定编码。
- Memory 测试与 race、全仓 18 包、vet、build、Docs Gate、`git diff --check` 均通过。
  全仓测试在 macOS 默认临时目录曾遇到已知外部清理器竞态；改用
  `TMPDIR=/private/tmp` 后完整通过，不归类为产品回归。
