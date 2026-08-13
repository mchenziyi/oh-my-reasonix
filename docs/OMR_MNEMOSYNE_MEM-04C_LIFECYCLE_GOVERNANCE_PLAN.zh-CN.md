# OMR Mnemosyne MEM-04C：Lifecycle、Health 与治理事务计划

- 状态：🟡 TDD 分阶段实现中（治理事件、Review Read、Outcome Attribution Override 已实现；其余生命周期报表与一致性诊断仍待实现）
- 前置：MEM-04B Enriched Outcome、MEM-02B Critic、MEM-02E Conflict Gate、既有 DerivedState/GovernanceEvent
- 目标：把派生 Lifecycle/Health、追加式治理命令和 Frozen 读取隔离连成可验证、可审计的最小闭环

## 一、边界与成功标准

Lifecycle、Health、Pinned、Archived、Frozen 永远是派生状态，不新增 `state.json`，不回写 MemoryRevision。
人工治理只追加既有 `GovernanceEvent`；自动阈值冻结仍由规范事实确定性派生，不伪造人工事件。

```text
Revision + Evidence + Usage + Outcome + Judgment + GovernanceEvent
→ DeriveState
→ Normal Index / Frozen Index
→ Governance Gate（仅人工操作）
→ append GovernanceEvent
→ 重新派生与重新编译 Generation
```

本阶段不删除记忆、不实现 Revise/Split/Merge、不调用模型、不修改 CURRENT。Generation 重编译由调用方显式
执行；治理提交成功不伪称新 Generation 已发布。

## 二、Schema Gate 决议

### 2.1 GovernanceEvent 沿用 v1，不新增事实类型

操作继续固定为 `pin | unpin | manual_freeze | unfreeze | archive`。命令请求是瞬时对象，模型不得提供
`event_id`、`created_at` 或写 Store：

```yaml
schema_version: 1
scope: project
memory_ref: {scope: project, memory_type: strategy, memory_id: mem_..., revision: 2, content_sha256: sha256_...}
operation: unfreeze
reason: user reviewed corrected attribution
source: local_user
basis_refs: []
```

OMR 生成 EventID；库 API 显式接收 `Now`，不得回退墙钟。CLI 代表真实用户操作时可在入口获取一次 UTC 时间
并传入纯函数，测试固定注入。相同 idempotency 输入重放为 NOOP，不同内容冲突不覆盖。

### 2.2 事件严格作用于目标 Revision

派生只接受 `event.memory_id == revision.memory_id AND event.revision == revision.revision`。新 Revision 不继承旧
Revision 的 pin/manual_freeze/archive/unfreeze；新 Revision 按自身事实从 probation/healthy 开始。

Archive 对目标 Revision 终态：后续同 Revision 的 pin/unpin/manual_freeze/unfreeze 拒绝，且没有 unarchive。
Pinned 只改变同 Lifecycle 层排序，不能绕过 Frozen/Archived/Superseded、安全或适用条件。

### 2.3 自动冻结与 manual_freeze 分离

- `outcome_attributed` 只有 Enriched Outcome 的 counted 结果参与 3 次 harm / 60% 阈值；Legacy 行为保持兼容；
- `evidence_validated` 使用 Critic + Conflict Gate；
- `explicit_confirmation` 使用 Confirmation Judgment；
- `manual_freeze` 是额外人工隔离，不能改写证据计数；
- Freeze 永不物理删除 Revision、Outcome、Evidence 或历史 Generation。

## 三、Governance Gate

### 3.1 通用验证

1. Strict JSON、固定枚举、大小和数量上限；
2. Store Scope 与 MemoryRef 五字段精确匹配；目标 Revision 必须存在且 Hash 正确；
3. basis_refs 的 MemoryRef/JudgmentRef/EvidenceRef 必须在同 Scope 且精确存在；PolicyRef 禁止用于 unfreeze；
4. 当前状态与命令必须有意义：重复 pin/unpin/manual_freeze 为稳定 NOOP，Archived 拒绝后续操作；
5. EventID 对 scope + MemoryRef + operation + source + normalized reason + basis hash + Now 确定性生成；
6. 追加前锁内重读现状并 preflight，避免并发 TOCTOU；错误稳定脱敏。

### 3.2 Unfreeze 硬门禁

`unfreeze` 不等于把 frozen=false。固定顺序：

1. 目标当前必须因 manual_freeze 或自动阈值处于 frozen；
2. basis_refs 非空、引用闭合，且至少含 Attribution Override Judgment、新 Evidence Generation 的 EvidenceRef
   或替代/新 Revision MemoryRef；
3. 先忽略当前 Revision 的 manual_freeze 意图，使用已经落盘的规范事实重新派生证据 Lifecycle；
4. 若证据 Lifecycle 仍为 frozen，拒绝且零写入；
5. 只有证据已不满足冻结条件时才追加 unfreeze；追加后重新派生必须不再 frozen，否则事务失败。

Governance Event 本身不能解除自动冻结证据。新 Revision 不需要为旧 Revision伪造 unfreeze。

### 3.3 人工 Attribution Override 入口

提供 `omr memory outcome override`：精确加载 Outcome 与当前有效 Override 链，previous_effect 必须等于当前
有效 effect；只追加 `attribution_override` JudgmentFact，绝不覆盖 Outcome。该 Judgment 可作为 unfreeze basis。

## 四、读取隔离

正常读取必须从固定 Generation 的 IndexTree 选择，继续排除 frozen/archived/superseded。新增显式读取 API：

```text
omr memory get <memory-id> --revision <n>          # frozen/archived 默认拒绝
omr memory get <memory-id> --revision <n> --include-frozen --review-mode
```

`--include-frozen` 只有同时声明 `--review-mode` 才有效，并返回显式 `frozen=true` 标记；不得把冻结页面加入普通
LibrarianReceipt、normal root/local index 或 Prompt。物理文件可见不等于协议允许读取。

## 五、API / CLI

```go
BuildGovernanceEvent(ctx, GovernanceRequest) (GovernanceEvent, error)
CommitGovernanceEvent(ctx, GovernanceRequest) (GovernanceResult, error)
BuildAttributionOverride(ctx, AttributionOverrideRequest) (JudgmentFact, error)
ReadMemoryForReview(ctx, ReviewReadRequest) (ReviewMemory, error)
```

```text
omr memory pin|unpin|freeze|unfreeze|archive <memory-id> --revision <n> --reason ... --json
omr memory outcome override <outcome-id> --previous-effect harmed --new-effect neutral --reason ... --json
omr memory get <memory-id> --revision <n> [--include-frozen --review-mode] --json
omr memory doctor --scope project --project-dir . --json
```

## 六、TDD 验收矩阵

### 当前已交付切片

- `BuildGovernanceEvent`：只接收请求字段与显式 `Now`，确定性生成 `event_id`，不读取/写入 Store。
- `CommitGovernanceEvent`：精确校验目标 Revision 的五字段后追加 `pin|unpin|manual_freeze|archive` 事件；重复提交由 FactStore 幂等处理。
- `unfreeze` 当前明确 fail-closed，尚未开放任何绕过自动冻结证据门禁的路径。
- CLI 已提供 `memory pin|unpin|freeze|unfreeze|archive`；`unfreeze` 自动携带目标 Revision 作为 basis，并继续接受证据重派生门禁。
- CLI 已提供只读 `memory doctor`；它调用 `CheckConsistency` 检查 Outcome/Override/Judgment/Governance/Policy 引用完整性，发现问题只输出脱敏诊断，不修复、不删除、不修改 CURRENT。
- CLI 已提供只读 `memory status`；它调用 `DeriveState` 查询指定 Memory 的 Lifecycle/Health/Pinned/Archived/Frozen 派生状态，不写入任何事实。

1. 旧 Revision 的 Governance Event 不污染新 Revision；同 Revision 顺序由 created_at + event_id 决定；
2. pin/unpin 只改 Pinned；Pinned 不能进入 Frozen 普通索引；
3. manual_freeze、自动阈值 frozen、archive、superseded 保持相互独立且优先级正确；
4. Archived 终态，同 Revision 后续命令拒绝；新 Revision不继承 archive；
5. unfreeze 缺 basis、PolicyRef、孤儿/错 Scope/错 Hash basis 拒绝；
6. 仍满足自动冻结阈值时 unfreeze 零写入；Override 改变有效归因后可恢复；
7. Outcome Override 追加且幂等，原 Outcome 字节不变；错 previous_effect 和 supersede 链 fail closed；
8. normal get/Librarian/Index 排除 frozen；review-mode 显式读取成功且不修改索引；
9. 所有失败零部分写入、错误脱敏；symlink/权限/路径安全继承 FactStore；
10. 不读墙钟（库层）、不写 Revision/CURRENT、无模型调用；
11. Fake CLI 进程闭环：freeze → normal get 拒绝 → review get 成功 → 有效 override+basis → unfreeze；
12. 全量 derive/index/compiler 字节确定性与 Legacy Outcome 兼容不回归。

最终门禁：gofmt、diff check、memory/CLI race、全仓 test、vet、build、Docs Gate、review/security review。

## 七、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-04C。先完整读取本计划、Architecture v1 第 11.4/11.5/13.10~14 章、model.go 的
GovernanceEvent、derived_state.go 的 applyGovernance/deriveLifecycle、librarian.go 与 MEM-04B Outcome 实现。

严格 TDD。先复现“旧 Revision 的事件污染新 Revision”和“仍自动 frozen 却可 unfreeze”两个失败，再做最小
修复。沿用 GovernanceEvent/JudgmentFact，不新增状态事实，不改 Architecture v1，不覆盖 Outcome/Revision，
不让 Pinned 绕过 Frozen，不用 Governance Event 抹掉自动冻结，不读取 CURRENT/time.Now，不进入 MEM-04D。

最终运行 gofmt、git diff --check、go test -race ./internal/memory/... ./cmd/omr、go test ./...、go vet ./...、
go build ./cmd/omr、tests/docs_check.sh，并做 review/security review。报告治理状态机、unfreeze 证明、读取隔离、
幂等/并发、测试与剩余边界；不要提交、推送或创建 Tag。
```
