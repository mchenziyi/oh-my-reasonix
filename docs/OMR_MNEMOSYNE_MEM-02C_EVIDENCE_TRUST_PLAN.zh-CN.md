# OMR Mnemosyne MEM-02C：Evidence Provenance 与 Trust Gate 实现计划

- 阶段：MEM-02 Protocol Implementation / Phase C
- 状态：✅ 已实现（2026-08-11）——Provenance Schema 六字段 + Trust Gate 只读评估器交付，Legacy golden 锁定，全部门禁/review/security_review 通过
- 前置：MEM-01A～MEM-01F、MEM-02-01/06/07/08、MEM-02A、MEM-02B 已签收；MEM-02 Schema Gate 已 PASS
- 目标：为 `MemoryEvidenceGeneration` 增加向后兼容的 Provenance 形态，并实现只读、确定性、可审计的 Trust Gate。
- 非目标：不自动写入 Judgment、不改变 Lifecycle、不实现 Retrieval Evaluation、Context Applicability、Conflict Fact、模型分类器或真实 Evidence Registry。

## 一、阶段边界与事实源

MEM-02C 只增加两项能力：

1. `MemoryEvidenceGeneration` 保存 Evidence 来源、获取方式、验证状态、来源引用和结构化内容分类信号；
2. Trust Gate 从不可变 Evidence Generation、精确 PolicyRef 和精确 Content Classification Judgment 派生结果。

事实源层级固定为：

```text
MemoryEvidenceGeneration + PolicyFact + ContentClassification Judgment
                              ↓
                    Trust Gate 派生结果
                              ↓
             EvidenceTrustPayload 所需确定性字段
```

Trust Gate 结果不是新 Fact，不得另建 `state.json`、Trust Fact 或缓存真相源。现有 `evidence_trust` Judgment 仍是某次 Trust Decision 的不可变审计记录；本阶段只计算其 payload 所需值，不自动创建、覆盖或选择 Judgment。

Trust 不能替代 Critic，也不能直接让 Memory active。`evidence_validated` 在 Conflict Fact 协议冻结前继续保持 `probation`。

## 二、Provenance Schema

### 2.1 MemoryEvidenceGeneration 兼容扩展

在现有 v1 `MemoryEvidenceGeneration` 上追加六个顶层字段：

```yaml
evidence_origin: runtime | user | official | project | external
acquisition_method: direct | tool_observed | model_extracted | imported
verification_status: verified | confirmed | inferred | unverified
provenance_refs: [EvidenceRef]
contains_instructional_content: boolean
contains_sensitive_content: boolean
```

沿用 `schema_version: 1`、现有不可变身份、`evidence_set_sha256`、FactStore 路由和文件布局，不新建 Evidence Trust Fact。

建议 Go 表示：

```go
type MemoryEvidenceGeneration struct {
    // 既有字段保持不变。
    EvidenceOrigin                string        `json:"evidence_origin,omitempty"`
    AcquisitionMethod             string        `json:"acquisition_method,omitempty"`
    VerificationStatus            string        `json:"verification_status,omitempty"`
    ProvenanceRefs                []EvidenceRef `json:"provenance_refs,omitempty"`
    ContainsInstructionalContent  *bool         `json:"contains_instructional_content,omitempty"`
    ContainsSensitiveContent      *bool         `json:"contains_sensitive_content,omitempty"`
}
```

两个布尔必须使用可表达“字段缺失”的形式；不能用普通 `bool` 把缺失静默解释为 `false`。`provenance_refs` 的 `nil` 表示字段缺失，显式空数组表示合法的空根引用集合。

### 2.2 两种合法形态

只允许：

1. **Legacy**：六个新增字段全部缺失；
2. **Provenance-enriched**：三个枚举非空、`provenance_refs` 字段显式存在、两个布尔字段显式存在。

部分存在必须 `CodeSchemaInvalid` fail closed，不自动补 Origin、Method、Status、布尔或来源引用。

Legacy 规则：

- 旧 JSON 继续可读；
- `canonMap` 保持旧键集合，Canonical Bytes 和 Hash 逐字节不变；
- Legacy Evidence 可被历史 Generation 重建，但 Trust Gate 只能返回 `unavailable`；
- 不迁移、不覆盖、不猜默认来源。

Enriched 规则：

- 六字段全部进入 Canonical Bytes 与 `evidence_set_sha256`；
- 三个枚举严格使用 Architecture v1 冻结值；
- `provenance_refs` 上限沿用 `maxRefs`，按完整 EvidenceRef 排序去重；重复引用拒绝；
- 每个 provenance ref 必须是本 Generation `evidence_refs` 的精确成员；
- `direct` 可使用显式空 `provenance_refs: []`，表示原始直接证据；
- `tool_observed | model_extracted | imported` 必须至少引用一个原始 EvidenceRef；
- 摘要或转换不得通过修改 Origin/Status 提高原始来源可信度；本阶段不递归读取不存在的 Evidence Registry，只验证当前 Generation 内的引用闭合。

### 2.3 Content 信号一致性

Generation 中的两个布尔是该 Evidence Generation 的结构化输入；正式 Trust 计算仍必须引用 `content_classification` Judgment。

Trust Gate 必须验证：

- Classification Judgment 的 subject 为目标 EvidenceRef；
- payload `evidence_ref` 与 subject、请求目标 EvidenceRef 完整一致；
- `contains_instructional_content` 与 `contains_sensitive_content` 和 Generation 内嵌值完全一致；
- `classifier_policy_ref` 指向实际存在、Hash 精确匹配的 `content_classifier` Policy。

任何不一致均为事实冲突，返回错误，不选择任一侧作为“更可信”的真相。

## 三、Trust Policy 兼容策略

MEM-01C 已冻结 `PolicyConfigTrust` 和旧 Policy Hash。本阶段不修改旧 PolicyFact 的 Canonical Schema，不重算历史 Hash。

Trust Gate 在使用 Policy 时增加运行时安全校验：

- `allowed_acquisition_methods` 每项必须属于 `direct | tool_observed | model_extracted | imported`；
- 实际 Evidence 的 acquisition method 必须在允许集合中；
- `require_provenance` 和 `require_verification_status` 必须为 true；
- `external_unverified_instruction_allowed` 必须为 false；
- Trust Policy 仍是安全根，Mnemosyne 不得生成、修改或放宽它。

历史 Policy 若包含旧的自由 identifier，Fact 仍可读取和审计，但不得被 Trust Gate 接受为有效安全策略；返回稳定错误，不静默映射为新枚举。

## 四、只读 Trust Gate API

建议最小 API：

```go
type TrustGateStatus string

const (
    TrustGateTrusted     TrustGateStatus = "trusted"
    TrustGateRestricted  TrustGateStatus = "restricted"
    TrustGateUnverified  TrustGateStatus = "unverified"
    TrustGateBlocked     TrustGateStatus = "blocked"
    TrustGateUnavailable TrustGateStatus = "unavailable"
)

type TrustGateRequest struct {
    Scope                     Scope
    MemoryID                  string
    Revision                  int
    EvidenceGeneration       int
    EvidenceRef               EvidenceRef
    TrustPolicyRef            PolicyRef
    ContentClassificationRef  JudgmentRef
    Now                       time.Time
}

type TrustGateResult struct {
    Status                       TrustGateStatus
    InstructionalContentAllowed bool
    PromotionEligible           bool
    EvaluatedAt                  string
}

func EvaluateEvidenceTrust(
    ctx context.Context,
    store *FactStore,
    req TrustGateRequest,
) (*TrustGateResult, error)
```

约束：

- `Now` 必填，零值 `CodeDerivedInvalidInput`，不得回退 `time.Now()`；
- Store Scope、请求 Scope、EvidenceRef Scope 必须一致；
- 按 `memory_id/revision/evidence_generation` 精确读取 Generation，不扫描最新版本、不读 CURRENT；
- 目标 EvidenceRef 必须存在于 Generation `evidence_refs`；
- PolicyRef 和 ClassificationRef 必须按 id/type/hash 精确加载，不按“最新”替代；
- Policy、Classification Judgment、Evidence Generation 的时间不得晚于 `Now`；
- 错误固定脱敏，不回显路径、枚举原值、Prompt、命令、正文或凭据。

`TrustGateResult` 为派生数据，不持久化。它可被后续调用方复制到现有 `EvidenceTrustPayload`：

- `evaluated_at = Now`；
- `instructional_content_allowed` 与 `promotion_eligible` 必须等于 Gate 结果；
- 本阶段不自动创建 Judgment ID、source、basis、supersede 链或写入 Store。

## 五、确定性状态矩阵

所有请求先验证请求字段、Store/Scope、Generation 路径与正文身份、EvidenceRef 成员关系及 Generation 时间。Legacy 完成这些验证后受控短路为 `unavailable`，不加载其历史 Schema 中不存在的 Classification/Policy 输入；只有 Enriched 形态继续执行 Classification、Policy、Hash 与时间一致性验证。任何实际执行的验证错误都不降级成状态。

| 条件 | Status | instructional allowed | promotion eligible |
|---|---|---:|---:|
| Legacy / Provenance 六字段整体缺失 | unavailable | false | false |
| acquisition method 不被 Policy 允许 | blocked | false | false |
| sensitive=true | blocked | false | false |
| external + unverified + instructional | blocked | false | false |
| verification_status=unverified（非上条） | unverified | false | false |
| verification_status=inferred | restricted | false | false |
| verified/confirmed，引用闭合，Policy 允许，非敏感 | trusted | 仅当 instructional=true | true |

补充规则：

- `promotion_eligible=true` 只表示 Trust Gate 允许进入后续 Promotion 条件检查，不等于自动 Promotion；
- `promotion_requires_policy_evidence=true` 时，本次精确 PolicyRef 与 Gate 结果满足该前置；后续 Promotion 仍必须引用不可变 Evidence Trust Judgment；
- blocked/restricted/unverified/unavailable 均不能单独推动 Lifecycle 或 Global Promotion；
- Trust Gate 不读取、修改或选择 Memory Revision。

## 六、引用与时间完整性

### 6.1 Classification Judgment

必须严格读取全部 Judgment，按 `ContentClassificationRef` 的 scope/type/id/hash 精确匹配：

- type 必须 `content_classification`；
- Judgment scope 与请求一致；
- subject/payload EvidenceRef 与目标完整匹配；
- `classifier_policy_ref` 必须精确指向存在的 Policy；
- Judgment Hash、未知字段和联合分支由 FactStore/DecodeStrict 验证；
- 本阶段不解析自由文本，不调用模型分类器。

### 6.2 Trust Policy

使用现有 `PolicyStore.GetPolicy(ref)` 精确加载历史版本。Policy 不存在、Hash 漂移、类型错误、配置安全根关闭或包含未冻结 acquisition method 均 fail closed。

### 6.3 时间

Evidence Generation `created_at` 始终必须 `<= Now`。Enriched 形态还要求 Classification Judgment、Classifier Policy 与 Trust Policy 的 `created_at` 均 `<= Now`。未来事实返回 `CodeEvaluationFutureReference`。Legacy 在验证 Generation 时间后返回 `unavailable`，不要求不存在的 Classification/Policy；相同有效输入与 Now 必须得到逐字节相同结果。

## 七、Lifecycle 与旧协议隔离

- 不修改 `DeriveState`、Health、UsageStats、Priority、OKF 或 CURRENT；
- 不把 trusted 解释为 Critic passed；
- 不把 promotion_eligible 解释为 active/global；
- 不修改既有 `EvidenceTrustPayload`、`ContentClassificationPayload` 或旧 Judgment Canonical Hash；
- 不处理 Trust Judgment supersede 链；该链仍由不可变 Judgment/Doctor 协议审计；
- Conflict Fact 未冻结，`evidence_validated` 恒 `probation`。

## 八、TDD 验收矩阵

每项先写失败测试，确认旧实现失败，再做最小实现。

### 8.1 Provenance Schema 与兼容

1. 五类 origin、四类 acquisition、四类 verification 合法；未知值拒绝且错误脱敏；
2. Legacy 六字段全缺失可读，Canonical Bytes/Hash 使用实现前 golden 锁定；
3. Enriched 六字段完整 round-trip、Put/Get/List/NOOP；
4. 六字段逐项缺失、普通 bool 无法区分缺失的问题、`nil` 与显式 `[]` 区别均覆盖；
5. direct + 空 refs 合法；非 direct + 空 refs 非法；
6. provenance ref 重复、越界、非 evidence_refs 成员、错误 Scope/Hash 拒绝；
7. 修改任一 Provenance 字段或布尔都会改变 Hash；
8. Legacy 与 Enriched 使用同一不可变身份时触发 identity conflict，不覆盖。

### 8.2 Trust Gate 状态

1. Legacy → unavailable；
2. verified/confirmed 安全输入 → trusted；
3. inferred → restricted；unverified → unverified；
4. external+unverified+instructional、sensitive、method 不允许 → blocked；
5. trusted instructional 与非 instructional 的 `instructional_content_allowed` 正确；
6. 只有 trusted 且安全时 promotion eligible；其他状态永远 false；
7. 相同输入和 Now 重复输出字节一致；不同 Now 只改变 evaluated_at。

### 8.3 引用、损坏与时间

1. EvidenceRef 不在 Generation、Generation 身份错配、损坏 JSON、Hash 漂移 fail closed；
2. Policy id/type/hash 不匹配、旧自由 acquisition Policy、安全根关闭 fail closed；
3. Classification ref id/type/hash/Scope 错误、subject/payload Evidence 不一致 fail closed；
4. Generation 内嵌布尔与 Classification Judgment 不一致 fail closed；
5. classifier Policy 缺失或 Hash 漂移 fail closed；
6. 零 Now、未来 Evidence/Policy/Judgment 拒绝；
7. symlink、权限和跨 Scope 复用 FactStore 既有安全边界；
8. 所有错误信息不得泄露攻击者输入。

### 8.4 隔离回归

1. 旧 MemoryEvidenceGeneration golden 不变；
2. 旧 8 类 Judgment golden 不变；
3. `DeriveState` 结果不因 Trust Gate 改变；
4. 不产生新文件、不写 Judgment、不改 Revision/CURRENT；
5. 无网络、模型、Embedding、向量数据库和墙钟读取。

## 九、修改边界

允许：

- `internal/memory/model.go`（仅 MemoryEvidenceGeneration 兼容字段与验证/canonical）；
- 新增最小 `internal/memory/trust_gate.go`；
- 必要的测试文件；
- 本计划状态行；
- `docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md` 的 MEM-02-03 状态。

禁止：

- Architecture v1、MEM-01A～F 既有字段/枚举/语义；
- 修改 PolicyFact、EvidenceTrustPayload、ContentClassificationPayload 或已有 Judgment 的 Canonical Schema；
- RetrievalEvaluation、ContextApplicability、Conflict Fact；
- `internal/evolution`、CLI、Prompt、Reasonix、Desktop；
- Revision、CURRENT、Generation Commit 自动写入；
- 网络、真实模型、Embedding、向量数据库；
- 提交、推送、Tag、Release。

## 十、成功标准与门禁

成功标准：

1. Legacy Evidence Generation 字节、Hash 和历史重建无回归；
2. 新 Provenance 形态严格、不可变、引用闭合；
3. Trust Gate 由精确事实与 Policy 确定性计算，缺失返回 unavailable，损坏 fail closed；
4. 安全根不可被 Policy、Evidence 或模型输出关闭；
5. Trust 不改变 Lifecycle，不产生第二事实源。

门禁：

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

环境失败标记 `[ENV]` 并保留原始输出。实现后执行独立 `review` 与 `security_review`，修复全部 blocking/should-fix 后重跑门禁。

## 十一、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne MEM-02C：Evidence Provenance 与 Trust Gate。

工作目录：/Users/czy/Desktop/demo/oh-my-reasonix

开始前读取并服从：
- AGENTS.md
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md（重点 5.3、6.2.3 Evidence Provenance/Trust、11.5、17.4、18 安全）
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md（重点 4、7、8、11）
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02A_USAGE_ANCHORS_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02B_CRITIC_REVIEW_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02C_EVIDENCE_TRUST_PLAN.zh-CN.md
- internal/memory/**

严格按 MEM-02C 计划实现。每项先写失败测试并确认旧实现失败，再做最小实现。

必须：
1. MemoryEvidenceGeneration 增加六个顶层 Provenance/Content 字段，只允许 Legacy 全缺失或 Enriched 全存在；两个布尔必须能区分缺失与 false，provenance_refs 必须区分 nil 与显式空数组。
2. Legacy Canonical Bytes/Hash 必须用实现前 golden 逐字节锁定；Enriched 六字段全部进入 Hash；部分字段拒绝，不自动补默认值。
3. 三类枚举使用 Architecture v1 冻结值；direct 可空 provenance；其他 acquisition 必须非空；每个 provenance ref 必须精确存在于本 Generation evidence_refs，重复/越界拒绝。
4. 实现只读 EvaluateEvidenceTrust：显式 Now、精确 Generation key、EvidenceRef、PolicyRef、ContentClassificationRef；不读 CURRENT、不用 time.Now()、不扫描最新版本。
5. Content Classification Judgment 的 subject/payload EvidenceRef、两个布尔与 Generation 必须完全一致；Classifier Policy 与 Trust Policy 均按 id/type/hash 精确加载。
6. Trust Policy 安全根不可关闭；Gate 只接受 frozen acquisition 枚举。旧自由 identifier Policy Fact 可读但 Gate 必须 fail closed，不能改旧 Policy Hash。
7. 严格按第五章状态矩阵计算 trusted/restricted/unverified/blocked/unavailable、instructional_content_allowed 与 promotion_eligible。
8. Trust Gate 结果只读、不持久化；不自动创建 EvidenceTrust Judgment，不修改 Lifecycle/Revision/CURRENT。evidence_validated 继续 probation。
9. 覆盖第八章全部测试，尤其 Legacy golden、字段存在性、Policy/Classification 精确引用、未来时间、错误脱敏与零写入。

禁止修改 Architecture v1、MEM-01A～F 既有协议、PolicyFact/已有 Judgment Canonical Schema；禁止实现 Retrieval/Context/Conflict；禁止 CLI/Prompt/Reasonix/Desktop、网络、真实模型、Embedding、向量数据库、提交、推送或 Tag。

完成后运行第十章全部门禁，执行独立 review 与 security_review，修复全部 blocking/should-fix 后重跑。

最终只输出：实际文件；Legacy/Enriched Schema 与 golden；Trust Policy 兼容策略；状态矩阵；引用/时间/安全行为；测试；门禁/review/security；[ENV]/剩余问题；明确“未进入 MEM-02D、未提交、未推送、未创建 Tag”。
```
