# OMR Mnemosyne MEM-02B：Critic Review Judgment 实现计划

- 阶段：MEM-02 Protocol Implementation / Phase B
- 状态：✅ 已完成（2026-08-11）——critic_review Judgment subtype 已注册（联合第 8 分支），EvaluateCriticRequirement 升级为固定 Generation Pair 上的完整验证器；TDD 验收、独立 review 与 security review 无阻塞；门禁全过；未进入 MEM-02C
- 前置：MEM-01A～MEM-01F、MEM-02-01/02/06/07/08、MEM-02A 已签收；MEM-02 Schema Gate 已 PASS
- 目标：正式注册严格的 `critic_review` Judgment subtype，使 Critic Requirement 能在固定 Generation Pair 上确定性判断 `passed | failed | unavailable`。
- 非目标：不实现 Conflict Fact，不让 `evidence_validated` 晋升 active，不实现 Trust、Retrieval Evaluation 或 Context Applicability。

## 一、阶段边界

MEM-02B 只解决架构 11.5 的一个子条件。`Critic passed` 不等于 Memory active；后者还要求至少 3 个独立 EvidenceRef、2 个独立 Root Task/正式来源以及无未解决冲突。Conflict Fact 尚未冻结，因此 MEM-02B 完成后 `CriticRequirementResult` 可以 satisfied，但 `DeriveState` 中 `evidence_validated` 仍必须保持 `probation`。

## 二、冻结 Schema

### 2.1 Judgment 联合扩展

在现有 v1 `JudgmentFact` 联合中新增：

```yaml
judgment_type: critic_review
subject:
  subject_type: memory_revision
  memory_ref: MemoryRef
source:
  source_type: fixture_oracle | offline_rule | user_review
  source_id: controlled_id
critic_review:
  result: passed | failed | unavailable
  evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit
  memory_context:
    project_generation_ref: ProjectGenerationRef | null
    global_generation_ref: GlobalGenerationRef | null
  required_evidence_refs: [EvidenceRef]
supersedes_judgment_ref: JudgmentRef | null
basis_refs: [MemoryRef | EvidenceRef | JudgmentRef | PolicyRef]
```

沿用 `schema_version: 1`、现有 Envelope/Ref/BasisRef、MEM-02A `MemoryContext`、Canonical Hash、FactStore 和不可变写入。禁止新建 Critic Fact、bump version、自由 `reason`、Prompt、思考、命令、正文或未注册 source。

### 2.2 CriticReviewPayload

建议最小类型：

```go
type CriticReviewPayload struct {
    Result               string        `json:"result"`
    EvaluationScope      string        `json:"evaluation_scope"`
    MemoryContext        MemoryContext `json:"memory_context"`
    RequiredEvidenceRefs []EvidenceRef `json:"required_evidence_refs"`
}
```

硬约束：

- result 仅 `passed | failed | unavailable`；evaluation_scope 仅协议四值；
- MemoryContext 两侧不可同时缺失；required refs 上限沿用 `maxPayloadRefs`，按集合语义确定性排序去重；
- passed 必须有 required evidence 和至少一个 Envelope basis；每个 required EvidenceRef 必须在 basis 中完全匹配；
- failed/unavailable 可无 required evidence，但已提供引用仍必须合法；
- 未知字段、错误联合、错误 Hash、路径型 ID 全部 fail closed。

### 2.3 JudgmentFact 特定约束

当 type 为 critic_review：

- 只能设置 critic_review payload；subject 必须为 memory_revision；subject scope 必须等于 Judgment Scope；
- source type 只能为 `fixture_oracle | offline_rule | user_review`，不得全局收紧其他 Judgment 的已有 source；
- supersedes ref 必须为 critic_review；链中每个节点必须评价相同 Scope、Memory ID、Revision、Content Hash 和 MemoryContext；
- 新 payload 进入 Content Hash；其他七类 Judgment 的 Canonical Bytes/Hash 必须完全不变。

## 三、Critic Requirement 验证器

### 3.1 显式请求输入

扩展现有请求：

```go
type CriticRequirementRequest struct {
    Scope                 Scope
    MemoryID              string
    Revision              int
    ExpectedMemoryContext MemoryContext
    ProjectStore          *FactStore
    GlobalStore           *FactStore
    Now                   time.Time
}
```

- Now 必填；零值返回 `CodeDerivedInvalidInput`，禁止 `time.Now()`；
- 主 `store` 读取 Subject Revision/Judgment；MemoryContext 有哪侧 ref，就必须提供哪侧 Store；
- 缺少 Store 或固定世界无法精确重建时返回 unavailable；损坏、Scope/Hash 漂移、未来 Generation fail closed。

### 3.2 Generation Pair

每个非空 ref 必须：

1. 校验结构和 Store Scope；
2. Generation 存在时复用 `resolveGenerationWorld` 校验 Generation、Manifest、compiled output 和 Now；
3. Generation 已清理时先校验永久 Manifest `created_at <= Now`，再复用 Manifest rebuild；
4. Manifest ID/Hash 与 ref 完全一致；
5. 永不读取 CURRENT，永不拿最新 Generation 替代固定引用。

可新增一个最小包内 helper 供后续 Retrieval/Context 复用，但不增加配置或公共抽象层。

### 3.3 Judgment 选择

只考虑 Scope、Subject Revision、Content Hash、MemoryContext 均与请求精确一致，且固定世界已验证的 Critic Judgment。required evidence 必须同时：

- 存在于目标 Revision 所有 `MemoryEvidenceGeneration.evidence_refs` 的精确并集中；
- 存在于该 Judgment Envelope `basis_refs`。

状态矩阵：

| 条件 | Status | Satisfied |
|---|---|---:|
| 无匹配 Critic | unavailable | false |
| 世界/证据不可用但未损坏 | unavailable | false |
| 最新有效结果 failed | failed | false |
| 最新有效结果 unavailable | unavailable | false |
| passed 且全部验证完成 | passed | true |
| Schema/Hash/Scope/链损坏 | error | 不返回结果 |

Supersede 规则：被合法同类型节点 supersede 的旧节点不再生效；Subject/Context 不一致或环 fail closed；多个未被 supersede 的终端结果相互冲突时返回 unavailable，不能按时间猜 passed；终端结果一致时可返回共同状态，输出必须确定性。

### 3.4 Evidence 验证

- 完整读取目标 Revision 的 MemoryEvidenceGeneration，验证 Scope/Revision/Hash；
- required refs 只按完整 EvidenceRef 匹配，不按 evidence_type 猜路径；
- 缺少 ref → unavailable；Evidence Generation 损坏 → error；
- 本阶段不新增 Episode/Reasonix Event Fact Registry。

## 四、兼容与 Lifecycle 隔离

- 既有七种 Judgment JSON/Bytes/Hash/Store 行为不变；
- “无 Critic → unavailable”行为保持；旧伪造 critic 测试改为非法枚举/未知字段/错误联合/Hash 漂移；
- 同 ID 同 Hash NOOP、异 Hash冲突；不迁移、不覆盖旧 Judgment；
- Critic passed 只改变 CriticRequirementResult；即使 3 Evidence + 2 Root Task + passed，DeriveState 仍 probation；
- 不修改 Health、UsageStats、Priority、OKF、CURRENT 或 Revision。

## 五、TDD 验收矩阵

先写失败测试，再做最小实现：

1. Schema：passed/failed/unavailable round-trip；严格 result/scope/source；subject/scope；passed evidence+basis；错误联合/未知字段/Hash；required refs 确定性；旧 Judgment golden 不变。
2. 固定世界：project-only/global-only/pair；Context 不匹配；CURRENT 切换不影响；清理后可重建或 unavailable；未来/漂移/篡改 fail closed；零 Now 稳定拒绝。
3. Evidence：完整集合可通过；缺失 unavailable；损坏 error；不猜 Evidence 路径。
4. Supersede：passed→failed、failed→passed、passed→unavailable；Subject/Context 错配；环；并列冲突 unavailable；并列一致稳定。
5. Lifecycle：passed 可 satisfied，但 evidence_validated 仍 probation；failed/unavailable 永不 active。
6. 安全：恶意 source/id/hash 不回显；引用上限、路径、symlink、权限、跨 Scope fail closed；无网络/模型/墙钟/CURRENT。

## 六、修改边界

允许：`internal/memory/model.go`、`refs.go`、`critic_requirement.go`、必要的最小包内 Generation 校验 helper、相关测试、本计划状态行及 MEM-02 计划中的 02-02 状态。

禁止：Architecture v1、MEM-01A～F 既有协议；Conflict/Trust/Retrieval/Context 实现；evolution、CLI、Prompt、Reasonix、Desktop、CURRENT、Revision；网络、模型、Embedding、向量数据库；提交、推送、Tag。

## 七、门禁

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

环境失败标记 `[ENV]` 并保留原始输出。实现后执行独立 review 与 security_review，修复全部 blocking/should-fix 后重跑门禁。

## 八、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne MEM-02B：Critic Review Judgment。

工作目录：/Users/czy/Desktop/demo/oh-my-reasonix

开始前读取并服从：
- AGENTS.md
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md（重点 5.3、6.2.1、6.2.3、11.5、12.2）
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md（重点 2、3、7、8、11）
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02A_USAGE_ANCHORS_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02B_CRITIC_REVIEW_PLAN.zh-CN.md
- internal/memory/**

严格按 MEM-02B 计划注册 critic_review Judgment subtype，并升级 EvaluateCriticRequirement。每项先写失败测试，确认旧实现失败，再做最小实现。

必须：
1. 沿用 v1 Envelope，新增 JudgmentTypeCriticReview、CriticReviewPayload 和唯一联合分支；其他七类 Judgment 字节与 Hash 不变。
2. subject 只能精确 Memory Revision；source 只能 fixture_oracle/offline_rule/user_review；passed 必须有 required evidence 和 basis，且 required evidence 在 basis 中。
3. payload 使用 MEM-02A MemoryContext；请求显式传 ExpectedMemoryContext、Now 和所需 Project/Global Store。不读 CURRENT，不回退 time.Now()。
4. 验证 Generation、永久 Manifest、compiled output、Scope、Hash 和时间；清理后只允许 Manifest 精确重建，否则 unavailable。
5. required evidence 必须存在于目标 Revision MemoryEvidenceGeneration 的 evidence_refs 联集；不按 evidence_type 猜路径。
6. supersede 链逐节点同类型/Subject/Context；环 fail closed；并列冲突终端 unavailable，不能猜 passed。
7. passed 只令 CriticRequirementResult.Satisfied=true；Conflict Fact 未冻结，DeriveState evidence_validated 必须继续 probation。
8. 覆盖第五章全部测试和旧 Judgment golden compatibility。

禁止修改 Architecture v1、MEM-01A～F 既有协议；禁止实现 Conflict/Trust/Retrieval/Context；禁止 CLI/Prompt/Reasonix/Desktop/CURRENT/Revision、网络、真实模型、Embedding、向量库、自动晋升、提交、推送、Tag。

完成后运行第七章全部门禁，再做独立 review 与 security_review，修复全部 blocking/should-fix 后重跑门禁。

最终只输出：实际文件；Schema/Canonical/兼容策略；Generation/Evidence/supersede 行为；状态矩阵与 Lifecycle 隔离证明；测试；门禁/review/security 结果；[ENV]/剩余问题；明确“未进入 MEM-02C、未提交、未推送、未创建 Tag”。
```
