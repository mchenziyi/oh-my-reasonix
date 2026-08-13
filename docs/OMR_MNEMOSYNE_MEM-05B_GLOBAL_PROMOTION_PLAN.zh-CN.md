# OMR Mnemosyne MEM-05B：Global Promotion

- 阶段：MEM-05B
- 状态：🟡 只读 PromotionPlan/Gate 与显式 Global Promotion Apply 已实现并通过门禁（2026-08-13）；人工批准记录、审计事件与后续 Generation 接入仍未实现
- 前置：MEM-05A 只读 Revision/Merge/Split/Generalize 计划已实现；Trust Gate、Lifecycle、Governance 已可只读验证
- 目标：定义跨项目经验进入 Global Scope 的显式、可审计、不可隐式升级的批准计划

## 一、核心边界

1. Project Memory 与 Global Memory 是不同 Scope；单个项目永远不能直接生成 Global Active。
2. Promotion 默认只产生结构化 `PromotionPlan`；显式 `ApplyPromotionPlan` 只有在调用方提供完整目标事实并通过二次校验后才写入一个 Global Revision，不切换 Global CURRENT。
3. 只有显式人工批准或后续受控治理事件，才允许把计划转换为新的 Global Revision；原 Project Revision 永不移动、覆盖或删除。
4. 所有来源必须通过 `MemoryRef`、`EvidenceRef`、`PolicyRef` 精确闭合，跨 Scope 或 Hash 不一致立即拒绝。
5. Global 派生索引、统计和候选视图都是可重建状态，不成为第二事实源。

## 二、PromotionPlan 最小对象

第一阶段只实现只读计划对象，不新增 FactKind：

```yaml
operation: global_promotion
source_refs: [MemoryRef]
evidence_refs: [EvidenceRef]
policy_ref: PolicyRef
trust_status: trusted
source_project_count: 2
project_family_fingerprint: sha256_...
proposed_global_memory_id: global_...
promotion_eligible: false
blocked_reasons: []
```

约束：

- 至少两个相互独立的 Project Scope 来源；重复 MemoryID、重复 Root Task 或同一项目重复来源不计为独立来源；
- 每个来源必须有可信 Evidence 与 `TrustGateTrusted` 结果；冻结、归档、冲突未解决、Evidence 不完整均拒绝；
- `policy_ref` 必须精确指向允许 Global Promotion 的 PolicyFact；不能使用裸 Policy ID 或“当前 Policy”；
- `project_family_fingerprint` 由规范化的非敏感项目族特征计算，不包含路径、项目名、远程地址、用户身份或凭据；
- `promotion_eligible=true` 只表示门禁满足，不代表已写入 Global 或已生效；
- 结构化错误只返回固定原因码，不回显输入文本、路径、命令、Prompt 或完整引用。

## 三、去重与冲突

- 同一来源集合、同一 PolicyRef、同一内容 Hash → 确定性 NOOP 计划；
- 同一 `proposed_global_memory_id` 但来源集合或内容 Hash 不同 → 冲突，拒绝覆盖；
- 主 ID 不使用成功次数、随机数或模型决定，使用来源证据链完整度 → 创建时间 → 稳定 MemoryID；
- 内容无法脱敏、来源不足或 Trust Gate 非 trusted → `promotion_eligible=false`，不猜测、不降级为可批准。

## 四、批准与写入边界

`ApplyPromotionPlan` 是显式单次写入边界：重新验证 Project 来源、Evidence、Trust Policy、Scope 和目标 Hash，然后复用 Global FactStore 的不可变写入、NOOP 与冲突语义。它不删除或修改 Project 来源，不改变 Lifecycle、CURRENT、Prompt 或索引。

真正的批准记录、Promotion Governance Event、Global OKF/Index 接入仍需后续事务阶段实现，顺序必须是：

真正写入 Global Revision 前必须单独实现并审计：

1. 再读取并验证不可变来源与 Policy；
2. 创建 Snapshot 与 Promotion Governance Event；
3. 生成新的 Global MemoryID/Revision；
4. 通过 FactStore 原子写入，旧事实零覆盖；
5. 重建 Global OKF/Index 并做 Doctor；
6. 任一步失败恢复 Snapshot，不删除来源事实。

本阶段不实现上述写入和自动批准，避免把“候选计划”误报成“Promotion 已完成”。

## 五、TDD 验收矩阵

1. 单项目、重复项目、重复 Root Task、单来源均拒绝；
2. 两个独立 Project 来源 + trusted Evidence 生成稳定候选；
3. 冻结、归档、冲突、unverified/blocked Trust 均拒绝；
4. Policy 缺失、Hash 漂移、Scope/type 错配 fail closed；
5. 路径、项目名、远程地址、凭据和未知字段拒绝且错误脱敏；
6. 输入排序变化、重复调用和同一来源集合输出逐字节一致；
7. 计划生成零写入、CURRENT/Global Revision/Index/Lifecycle 不变；
8. 同 ID 不同内容冲突不覆盖已有事实；
9. race、全量测试、vet、build、Docs Gate 全通过。

## 六、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05B。先读取本计划、MEM-05A、Architecture v1 Scope/Promotion/Policy 章节及现有
MemoryRef、EvidenceRef、PolicyRef、TrustGate、Lifecycle、FactStore 实现。

先做只读 Schema Gate：不要新增 FactKind、不要修改 Architecture v1 或已冻结 Schema；若必须新增持久化字段，先停下
报告冲突。通过后严格 TDD，实现只读 PromotionPlan、确定性 Global Promotion Gate 与显式 ApplyPromotionPlan。至少要求两个独立 Project 来源、
trusted Evidence、允许 Promotion 的精确 PolicyRef；冻结/归档/冲突/不可信来源必须 fail closed。输出只包含结构化元数据，
不含路径、项目名、远程地址、命令、Prompt、凭据；Apply 只能写入调用方提供且二次校验通过的 Global Revision，不能修改来源、CURRENT 或 Index，不调用模型/网络。

运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet ./...、go build ./cmd/omr、
tests/docs_check.sh，并进行 code review/security review。未获 CTO 复核前不要提交、推送或创建 Tag。
```
