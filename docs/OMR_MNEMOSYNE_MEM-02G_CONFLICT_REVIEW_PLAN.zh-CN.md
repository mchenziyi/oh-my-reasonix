# OMR Mnemosyne MEM-02G：Conflict Review 与 Evidence-Validated Gate 计划

- 阶段：MEM-02G
- 状态：✅ 已完成并经 CTO 签收（2026-08-12）
- 前置：MEM-02A～MEM-02F 已实现；`critic_review` 已能验证固定 Generation Pair
- 目标：补齐 Architecture v1 11.5 中“无未解决冲突”的可审计协议，解除
  `evidence_validated` 永久停留 probation 的最后一个协议阻塞。

## 一、先做出的架构决议

### 1.1 不新增独立 Conflict FactKind

“两条记忆是否在当前固定世界中冲突”是可复核、可撤销的判断，不是客观事件。
MEM-02G 因此复用现有不可变 `JudgmentFact` Envelope，新增严格 subtype：
`conflict_review`。

本阶段禁止新增 `facts/conflicts/`、可变 `conflict_state.json` 或另一套 supersede
协议：

- `MemoryRevision.relations[predicate=contradicts]` 继续表达知识正文中的显式关系；
- `conflict_review` 表达某个 Revision 在固定 Generation Pair 下的审计结论；
- Lifecycle 只消费经过验证的派生结果，不把任何派生状态写回事实；
- 两者不互相覆盖，不形成第二事实源。

这是对 Architecture v1 6.2.1 Judgment 联合的受控扩展。实现时保持
`schema_version=1`，只扩联合枚举与 payload；旧八类 Judgment Canonical
Bytes/Hash 必须逐字节不变。

### 1.2 为什么不能仅以“没有 conflict 文件”视为 clear

缺少冲突记录不等于执行过完整检查。`clear` 只有在固定候选世界上完成
`fixture | generation_full_scan | expanded_index_scan` 后才可满足 Gate；
`sampled_audit` 的 clear 只是一份局部观察，不能推动晋升。

## 二、冻结 Schema 提案

### 2.1 `conflict_review` Judgment

```yaml
schema_version: 1
judgment_id: judgment_conflict_01K...
judgment_type: conflict_review
scope: project
subject:
  subject_type: memory_revision
  memory_ref: MemoryRef
source:
  source_type: fixture_oracle | offline_rule | user_review | conflict_critic
  source_id: controlled_id
conflict_review:
  result: clear | conflict | unavailable
  evaluation_scope: fixture | generation_full_scan | expanded_index_scan | sampled_audit
  memory_context: MemoryContext
  counterpart_memory_refs: [MemoryRef]
  evidence_refs: [EvidenceRef]
supersedes_judgment_ref: JudgmentRef | null
basis_refs: [BasisRef]
content_sha256: sha256_...
created_at: RFC3339 UTC
```

### 2.2 字段约束

- Subject 必须为 `memory_revision`，且 Subject MemoryRef 与目标 Revision 五字段完全
  一致；
- `memory_context` 复用 MEM-02A 的 Project/Global Generation Pair，不读 CURRENT；
- `result=clear|unavailable` 时 `counterpart_memory_refs` 必须为空；
- `result=conflict` 时 counterpart 至少 1 个，按五字段排序去重并精确存在于固定
  Generation Pair；禁止引用 Subject 自身；
- `evidence_refs` 按四字段排序去重，必须闭合到目标或 counterpart 的 Evidence
  Generation；
- `source_type` 只允许四个冻结值；`source_id` 仍为受控标识；
- `clear` 只有 `fixture|generation_full_scan|expanded_index_scan` 可令 Gate satisfied；
- `sampled_audit + clear`、`unavailable`、无匹配 Judgment 均为 unavailable；
- `conflict` 始终表示存在未解决冲突；解除冲突必须创建新的 `clear` Judgment，携带
  BasisRef 并通过 `supersedes_judgment_ref` 指向旧 conflict，禁止原地改写；
- 一个 clear Judgment 若 supersede conflict，`basis_refs` 必须非空，且至少包含新
  Revision、Evidence 或 Judgment；PolicyRef 单独不能证明冲突解除。

### 2.3 Supersede 与并列终端

- SupersedesJudgmentRef 的 scope/type/id/hash 四字段必须与实际目标精确一致；
- 链中所有节点必须保持同一 Subject Revision 与同一 MemoryContext；
- 孤儿、环、错 Scope/Hash/Subject/Context fail closed；
- 多个有效终端全部 clear 时共同为 clear；只要包含 conflict，结果为 conflict；
- clear 与 unavailable 并列、或多个终端使用不同固定世界时，结果 unavailable；
- 不按时间、文件顺序或“更可信来源”猜测冲突结论。

## 三、只读 Conflict Requirement

新增：

```go
type ConflictRequirementRequest struct {
    Scope                 Scope
    MemoryID              string
    Revision              int
    ExpectedMemoryContext MemoryContext
    ProjectStore          *FactStore
    GlobalStore           *FactStore
    Now                   time.Time
}

type ConflictRequirementStatus string

const (
    ConflictRequirementClear       = "clear"
    ConflictRequirementUnresolved  = "unresolved"
    ConflictRequirementUnavailable = "unavailable"
)
```

`EvaluateConflictRequirement`：

1. 显式校验 Scope、Revision、Now 和固定 MemoryContext；
2. 复用 Critic 的 Generation/Manifest/Compiler 精确验证，不读 CURRENT；
3. 严格加载全部 Judgment，相关链损坏 fail closed；
4. 精确验证 counterpart 与 Evidence 引用属于固定世界；
5. 返回 `clear|unresolved|unavailable`，结果只读且不写 Fact；
6. 相同 Facts/Context/Now 输出逐字节稳定；未来引用 fail closed。

## 四、Evidence-Validated 综合 Gate

新增独立组合器，不把固定世界参数偷偷塞进旧 `DeriveStateRequest`：

```go
type EvidenceValidatedGateResult struct {
    EvidenceCount int
    RootTaskCount int
    CriticStatus  CriticRequirementStatus
    ConflictStatus ConflictRequirementStatus
    Satisfied     bool
}
```

Gate 同时满足才返回 `Satisfied=true`：

```text
独立 EvidenceRef >= 3
AND 独立 root_task_refs >= 2
AND CriticRequirement == passed
AND ConflictRequirement == clear
```

该组合器只证明晋升条件已满足，不直接写 Lifecycle。MEM-02G 不修改
`DeriveStateRequest` 或自动切换索引；把综合 Gate 接入 Generation 编译/读取入口属于
后续独立阶段，避免无固定 MemoryContext 的旧调用方被静默改变。

## 五、状态与安全语义

| 输入 | Conflict Requirement | Gate |
|---|---|---|
| 无 Review | unavailable | false |
| 完整扫描 clear | clear | 继续检查其他条件 |
| sampled audit clear | unavailable | false |
| conflict | unresolved | false |
| clear supersede conflict，Basis 闭合 | clear | 继续检查 |
| 并列 clear + conflict | unresolved | false |
| 并列 clear + unavailable | unavailable | false |
| 链、Hash、Scope、Context、引用损坏 | error | fail closed |

错误和报告不得包含绝对路径、Prompt、命令、模型思考、凭据或完整 Evidence 正文。
Conflict Review 不能自动修改 Revision、Relation、Governance、Lifecycle、CURRENT 或
Generation。

## 六、TDD 测试矩阵

### 6.1 Schema 与兼容

1. `conflict_review` 三个结果 round-trip；未知字段/枚举拒绝；
2. 旧八类 Judgment golden Bytes/Hash 不变；
3. Subject、counterpart、Evidence、MemoryContext 严格校验；
4. result 与 counterpart 数量矩阵；clear supersede conflict 的 Basis 约束；
5. refs 排序去重、重复拒绝、Canonical 字节稳定。

### 6.2 固定世界与链

6. 固定 Project/Global Generation Pair，不受 CURRENT 切换影响；
7. Generation 清理后精确重建或 unavailable；
8. counterpart 缺失、错 revision/hash/type/scope、跨世界拒绝；
9. Evidence 孤儿/损坏/跨 Scope 拒绝；
10. supersede 合法链、孤儿、错 ref、错 subject/context、环；
11. 并列 clear、clear+conflict、clear+unavailable 的固定矩阵；
12. 未来 Generation/Judgment/Revision fail closed；零 Now 拒绝。

### 6.3 综合 Gate 与隔离

13. 3 Evidence + 2 Root Task + critic passed + conflict clear → satisfied；
14. 四个条件逐一缺失 → false；不得用 Usage/Confirmation 替代；
15. sampled clear 不能晋升；
16. 只读零写入、CURRENT/Revision/Lifecycle/Health 不变；
17. 插入顺序/多进程读取结果一致；
18. 错误脱敏、Context 取消、权限/symlink/损坏 JSON fail closed；
19. Fake fixture 端到端无需 API Key/网络/真实模型。

## 七、实现顺序

1. 先补 Architecture Amendment 与 Docs Gate；
2. 失败测试锁定旧八类 golden 和新联合分支；
3. 最小扩展 `JudgmentType`、payload、Validate/canonMap；
4. 实现只读 `EvaluateConflictRequirement`；
5. 实现只读 `EvaluateEvidenceValidatedGate`；
6. 扩展 Consistency Doctor 与离线 Benchmark Fixture；
7. 全量门禁、review、安全 review；
8. 未获 CTO 签收前不提交、不推送、不进入 MEM-03。

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

## 九、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-02G：Conflict Review 与 Evidence-Validated Gate。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

先完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 6.2.1、6.2.3、
  10.4、11.5、17、20 章；
- docs/OMR_MNEMOSYNE_MEM-02G_CONFLICT_REVIEW_PLAN.zh-CN.md；
- internal/memory/model.go、derived_state.go、critic_requirement.go、
  evaluation_context.go、consistency_doctor.go、benchmark.go、store.go。

严格按计划第二～八章执行，每阶段先写失败测试并保留修复前证据，再做最小实现。

硬约束：
1. 不新增 facts/conflicts FactKind；使用 v1 Judgment Envelope 新增唯一 subtype
   conflict_review，不建立第二事实源。
2. 旧八类 Judgment golden Bytes/Hash 必须逐字节不变。
3. 固定 MemoryContext，不读 CURRENT；Subject、counterpart、Evidence、Basis、Policy、
   supersede 引用全字段精确验证。
4. 缺少完整扫描不能视为 clear；sampled_audit clear 不得满足 Gate。
5. conflict 解除必须由带 Basis 的 clear Judgment supersede，禁止覆盖旧事实。
6. 综合 Gate 严格要求 3 Evidence + 2 Root Task + critic passed + conflict clear；不同
   Usage Policy 的事实不能替代。
7. 本阶段只读评估，不自动修改 Lifecycle/Health/Revision/Relation/CURRENT/Generation。
8. 错误稳定脱敏，不调用模型/网络，不进入 MEM-03。
9. 扩展 Consistency Doctor 和离线 Benchmark Fixture；运行全量门禁与独立
   review/security_review。

若计划与 Architecture v1 出现未登记冲突，停止相关实现并报告，不自行扩 Schema。

最终只输出：实际文件；Schema Amendment；旧 golden；固定世界与引用校验；
supersede/并列矩阵；综合 Gate；Doctor/Benchmark；失败测试证据；门禁、review、
security_review；[ENV]/剩余问题；明确“未提交、未推送、未进入 MEM-03”。
```

## 十、完成定义

Schema Amendment 经 CTO 确认、全部失败测试转绿、全量门禁通过、旧 golden 不变且
无 blocking/should-fix 后才可签收。

## 十一、实现记录（2026-08-11）

- `JudgmentFact` 已在 v1 Envelope 中增加第九个 `conflict_review` 联合分支；旧八类
  编码路径不变，并以现有七类 golden 加 Critic Review golden 锁定兼容性；
- 已实现只读 `EvaluateConflictRequirement`：固定 MemoryContext、不读 CURRENT，
  校验 Subject、counterpart、Evidence、时间与 supersede 链；完整扫描 clear 才满足，
  sampled clear 返回 unavailable；
- 固定世界读取已收紧到永久 Generation Input Manifest：目标 Revision、counterpart 与
  Evidence Generation 必须是该 Generation 的精确输入；Generation 提交后追加到 Store
  的证据不会回流到旧 Critic 或 Gate；
- 已实现只读 `EvaluateEvidenceValidatedGate`：严格组合 3 个独立 EvidenceRef、2 个
  独立 Root Task、Critic passed 与 Conflict clear，不修改 Lifecycle 或任何事实；
- Consistency Doctor 已检查 conflict counterpart、Evidence 与 supersede 精确引用；
  Benchmark 通过既有 Judgment Fixture 和 Doctor 指标覆盖冲突引用损坏；
- 全部实现仅使用本地确定性事实，不调用模型、网络或 API Key；未进入 MEM-03。
