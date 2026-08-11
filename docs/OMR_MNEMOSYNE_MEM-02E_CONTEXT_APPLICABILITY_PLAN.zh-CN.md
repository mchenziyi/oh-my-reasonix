# OMR Mnemosyne MEM-02E：Context Applicability Judgment 实现计划

- 阶段：MEM-02E / MEM-02-05
- 状态：✅ 已实现（2026-08-11，待 CTO 签收）
- 前置：MEM-02A～MEM-02D 已实现；MEM-02 Schema Convergence Gate 已通过
- 目标：补齐 `basis_context_refs` 协议缺口，并实现可审计、可验证、只读的 Context Applicability 判断，不建立 Context Ontology，不让 Go 程序做语义推断。

## 一、假设、成功标准与交付边界

本阶段采用以下最小解释：

1. Architecture v1 的 `basis_context_refs` 是 `JudgmentFact` 顶层字段，不属于
   `ContextApplicabilityPayload`；
2. `result` 的持久化枚举保持
   `exact | applicable | conditionally_applicable | not_applicable | unknown`；
   不删除 `exact`，不把 `unavailable` 写进 Judgment；
3. 验证器可输出派生状态 `verified | unavailable`，其中 `unavailable` 只表示旧
   Judgment 缴少新锚字段或当前无法闭合引用，不改变持久化结论；
4. `required_condition_ids` 可引用目标 Revision 的 `applies_when` 或
   `does_not_apply_when` 中任一已存在 `ApplicabilityCondition.condition_id`。架构只
   要求“目标 Revision 中存在”，本阶段不擅自缩窄为某一数组；
5. Context Descriptor Fact 尚未实现。因此 `target_context_ref` 与
   `basis_context_refs` 只执行严格 ID、去重、Scope 继承和确定性编码校验，不伪造
   Context Fact 存在性；该缺口必须在结果和 Doctor 后续计划中显式保留。

成功标准：

```text
Legacy Judgment 字节与 Hash 不变
→ Enriched Context Judgment 显式记录 basis_context_refs
→ 精确加载目标 MemoryRevision 与 Evidence
→ 校验结果/条件/Scope/时间/引用
→ 输出 verified | unavailable
→ 零写入，不修改 Revision、Lifecycle、Index、Prompt 或 CURRENT
```

允许修改：`internal/memory/**`、本计划、MEM-02 总计划状态与
`tests/docs_check.sh`。禁止修改 Architecture v1、MEM-01A～F 既有 Canonical
Schema、Evolution、CLI、Prompt、Reasonix、Desktop、CURRENT；禁止模型/网络
调用、自动采用 Memory、提交、推送与 Tag。

## 二、Schema 与兼容策略

### 2.1 顶层 Basis Context 字段

在 `JudgmentFact` 增加：

```go
BasisContextRefs []string `json:"basis_context_refs,omitempty"`
```

字段只允许用于 `judgment_type=context_applicability`：

- Enriched Context Judgment 必须显式携带至少一个受控 Context ID；
- 最大数量沿用有界引用限制；空字符串、路径、命令片段、重复 ID 拒绝；
- Canonical 编码排序，输入顺序不影响 Hash；
- 非 Context Judgment 携带该字段必须拒绝；
- Scope 不写进 ID，统一继承 Judgment Scope，不引入 `ContextRef` 或 Context
  Ontology。

### 2.2 Legacy / Enriched 双形态

为保持 MEM-01A 既有 Context Judgment golden：

- `BasisContextRefs == nil` 表示 Legacy 文档；Canonical Map 不输出该键，旧字节与
  Hash 逐字节不变；
- `BasisContextRefs != nil` 表示 Enriched 文档；Canonical Map 显式输出排序后的
  `basis_context_refs`；空数组非法；
- FactStore 继续允许读取和验证合法 Legacy 文档；只读 Applicability 验证器对其
  返回 `unavailable`，不得猜测 Basis Context；
- 新 MEM-02E 写入路径和测试 fixture 必须使用 Enriched 形态。直接调用底层
  FactStore 仍可能保存 Legacy 文档，这是兼容入口，不代表其满足 MEM-02E 验证。

不得通过提高 `schema_version`、给旧文档补默认 Basis、重算旧 Hash 或修改旧文件
完成迁移。

### 2.3 Payload 约束收紧

继续使用冻结 payload：

```yaml
context_applicability:
  result: exact | applicable | conditionally_applicable | not_applicable | unknown
  required_condition_ids: [controlled_condition_id]
  evidence_refs: [EvidenceRef]
```

规则：

- `conditionally_applicable` 必须携带至少一个 `required_condition_id`；
- 其他四种结果必须携带显式空数组，不得夹带条件；
- condition ID 与 EvidenceRef 均拒绝重复并受数量上限约束；Canonical 编码排序；
- 每个 required condition 必须精确存在于 Subject MemoryRef 指向 Revision 的
  `applies_when ∪ does_not_apply_when`；禁止内联条件和自由文本条件；
- `unknown` 不触发采用、拒绝、正负计分或 Lifecycle 变化。

## 三、Context Judgment 特定约束

Enriched Context Judgment 必须满足：

1. `Subject.SubjectType == "context"`；
2. Subject 只携带 `MemoryRef + TargetContextRef`；
3. `Subject.MemoryRef.Scope == Judgment.Scope`；
4. `target_context_ref` 与每个 `basis_context_ref` 为受控 ID；
5. `Source` 复用 Judgment Envelope，不新增本阶段专用枚举，也不复制 authority；
6. SupersedesJudgmentRef 若存在，必须精确匹配同类型旧 Judgment 的
   scope/type/id/hash，且旧、新节点的 Subject MemoryRef 五字段与目标 Context
   一致；Basis Context、result、condition 与 evidence 可以随新 Judgment 修订；
   环、孤儿和 Subject 错配 fail closed；
7. `CreatedAt` 必须为规范 UTC 时间；验证时不得晚于显式 `Now`。

`exact` 表示已提交 Judgment 认为目标 Context 与该 Revision 的适用上下文精确
匹配；它与 `applicable` 保持不同语义，Go 验证器只验证协议与引用，不重新判断
二者。

## 四、只读验证器

```go
type ContextApplicabilityStatus string

const (
    ContextApplicabilityVerified    ContextApplicabilityStatus = "verified"
    ContextApplicabilityUnavailable ContextApplicabilityStatus = "unavailable"
)

type ContextApplicabilityRequest struct {
    Scope      Scope
    JudgmentID string
    Store      *FactStore
    Now        time.Time
}

type ContextApplicabilityResult struct {
    Status           ContextApplicabilityStatus `json:"status"`
    JudgmentID       string                     `json:"judgment_id"`
    MemoryRef        MemoryRef                  `json:"memory_ref"`
    TargetContextRef string                     `json:"target_context_ref"`
    Result           string                     `json:"result"`
    EvaluatedAt      string                     `json:"evaluated_at"`
}

func ValidateContextApplicability(
    ctx context.Context,
    req ContextApplicabilityRequest,
) (*ContextApplicabilityResult, error)
```

结果是派生数据，不持久化，并提供稳定 Canonical 编码。请求必须显式传 `Now`；零值
拒绝，不得调用 `time.Now()`。

## 五、验证顺序

1. 校验请求、Scope、JudgmentID、Store 与显式 Now；
2. 按 JudgmentID 精确加载 Fact，验证路径正文身份、Hash、权限和 symlink 边界；
3. 验证 JudgmentRef 类型、Subject 联合、Scope 与 `created_at <= Now`；
4. Legacy `basis_context_refs` 缺失 → 稳定返回 `unavailable`，不读取 CURRENT、
   不补默认值；
5. Enriched Basis Context 做 ID、数量、重复和确定性集合校验；
6. 按 Subject MemoryRef 从对应 Scope Store 精确读取 MemoryRevision，验证
   scope/type/id/revision/hash 五字段；绝不选择“最新 Revision”；
7. 校验 required condition IDs 精确属于目标 Revision 的条件集合；
8. 校验每个 EvidenceRef 在同 Scope Store 的 MemoryEvidenceGeneration 中精确闭合；
   缺失、跨 Scope、错误 type/id/hash 均 fail closed；
9. 若有 supersede 链，逐边执行 JudgmentRef 四字段与 Context 节点身份校验；
10. 所有协议与引用闭合后返回 `verified`，原样报告持久化 result。

Context Descriptor Fact 尚未落地不是结构错误：验证器不扫描、不推断 Context
内容。该限制不得被描述成“Context 已验证存在”。

## 六、隔离、安全与确定性

- 全流程只读；两次验证前后 Store 文件数、CURRENT、DerivedState 不变；
- 不调用模型，不比较自然语言 Context，不生成 Applicability 结论；
- 不记录 Prompt、命令、模型思考、绝对路径、凭据或 Context 正文；
- 错误消息固定脱敏，不回显攻击者提供的 ID/source/路径；
- 相同 Fact 与 Now 输出字节一致；不同 Now 只改变 `evaluated_at`；
- context 取消立即返回，无半成品；损坏相关 Fact fail closed；
- 无关损坏 Fact 不应污染精确 Judgment 验证。

## 七、TDD 测试矩阵

### 7.1 Schema 与兼容

1. Legacy Context Judgment golden 字节/Hash 逐字节不变；旧七类/新增 Critic 等
   其他 Judgment golden 不变；
2. Enriched round-trip、Hash、Store Put/Get/List/NOOP；
3. nil/空/合法 Basis 三形态；非法 ID、路径、命令、重复、超限拒绝；
4. 非 Context Judgment 携带 Basis 拒绝；
5. Basis 乱序 Canonical 字节一致；改变任一 ID 改变 Hash；
6. result 五值合法，`unavailable` 持久化拒绝；
7. conditionally 缺条件拒绝；其他结果夹带条件拒绝；条件/evidence 重复拒绝；
8. unknown field、错误 Hash、部分/错误联合、错误 Subject 拒绝。

### 7.2 精确引用与状态

9. Legacy 验证稳定返回 unavailable，零写入；
10. 五种 result 的合法 Enriched Judgment 均 verified，程序不改写语义；
11. MemoryRef scope/type/id/revision/hash 任一错配 fail closed；
12. required condition 在 applies_when 或 does_not_apply_when 时通过；不存在时拒绝；
13. EvidenceRef 精确闭合；缺失、跨 Scope、错误 hash、重复拒绝；
14. Project/Global Store 隔离；
15. 未来 Judgment/Revision/Evidence 拒绝；零 Now 拒绝；
16. supersede 合法链、错 ref、错 Subject/target/Basis、环、孤儿和并列冲突；
17. 固定 Revision 验证不受后续新 Revision 或 CURRENT 变化影响。

### 7.3 安全与功能联调

18. 相同输入稳定字节；不同 Now 只改 evaluated_at；
19. 前后文件数、DerivedState、CURRENT 不变；
20. symlink、权限、路径正文身份和损坏 JSON fail closed；
21. 错误脱敏、context 取消、无关损坏 Fact 隔离；
22. Fake fixture：Revision + Evidence + Enriched Judgment → verified，无 API Key。

## 八、门禁

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

沙箱禁止 `httptest` 监听或 TempDir cleanup 受宿主进程影响时，记录 `[ENV]` 并在
允许环境复核，不得伪造通过。

## 九、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-02E：Context Applicability Judgment。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 6.2.2、6.2.3、11、17 章；
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md 第六章；
- docs/OMR_MNEMOSYNE_MEM-02E_CONTEXT_APPLICABILITY_PLAN.zh-CN.md；
- internal/memory/model.go、conditions.go、store.go、generation_tx.go、
  evaluation_context.go、critic_requirement.go、retrieval_evaluation.go。

严格执行计划第二～八章。每阶段先写失败测试并记录修复前证据，再做最小实现。

硬约束：
1. basis_context_refs 是 JudgmentFact 顶层字段，仅 context_applicability 可用。
2. 采用 Legacy/Enriched 双形态；Legacy canonical 字节与 hash 必须逐字节不变，新
   Enriched 文档显式非空 Basis 并进入 hash。
3. 持久化 result 保留 exact|applicable|conditionally_applicable|not_applicable|unknown；
   不新增 unavailable，不合并 exact。
4. conditionally_applicable 引用目标 Revision 已存在的结构化 condition_id；禁止
   内联或自由文本条件。其他 result 不得夹带 condition IDs。
5. 精确验证 MemoryRef、EvidenceRef、JudgmentRef、Scope、时间和 supersede 全链；
   不选择最新 Revision，不读 CURRENT。
6. Context Descriptor Fact 未实现：只验证 Context ID/集合/Scope 继承，不伪造
   Context 存在性，不创建 ContextRef 或 Context Ontology。
7. Go 不做语义判断；验证器只输出 verified|unavailable 并原样报告 Judgment result。
8. 全流程只读零写入，不改 Revision/Lifecycle/Index/Prompt/CURRENT，不进入 MEM-02F。
9. 错误静态脱敏，覆盖第七章测试，运行第八章全部门禁，并执行独立
   review/security_review。

若冻结架构与计划冲突，停止相关实现并报告原文位置，不自行扩大 Schema。

最终只输出：实际文件；Legacy/Enriched 兼容策略；顶层 Basis Schema；result/condition
矩阵；精确引用与 supersede 验证；Context Descriptor 缺口；失败测试证据；门禁、
review/security_review；[ENV]/剩余问题；明确“未进入 MEM-02F、未提交、未推送、
未创建 Tag”。
```

## 十、完成定义

测试与门禁全绿、旧 golden 不变、无 blocking/should-fix、文档状态同步后才可签收。
签收前不得把 Judgment 接入自动采用、评分、索引重写或自进化 Proposal。

## 十一、实施记录

| 子阶段 | 状态 | 结果 |
|---|---|---|
| E-01 Schema/兼容 | ✅ | 顶层 Basis 字段；Legacy golden 不变；Enriched 集合进 Hash |
| E-02 Payload 约束 | ✅ | result/condition 矩阵、重复与上限 fail closed |
| E-03 精确引用 | ✅ | MemoryRevision、Condition、Evidence、Supersede 精确验证 |
| E-04 只读验证器 | ✅ | verified/unavailable 分离；显式 Now；稳定编码；零写入 |
| E-05 文档与门禁 | ✅ | 14 个顶层回归测试；memory/race/full/vet/build/docs gate 通过 |

实现阶段删除了两条未被 Architecture v1 要求的草案限制：目标 Context 可以作为
Basis Context；新的 superseding Judgment 可以修订 Basis Context 集合。Supersede
只锁定同一 MemoryRef 与 target Context 身份，避免阻止合法判断修订。

已知协议缺口保持：Context Descriptor Fact 尚未落地，因此本阶段不验证 Context
正文存在性，也不宣称完成 Context Ontology。该缺口不影响不可变 Judgment、条件和
Evidence 的精确引用验证。
