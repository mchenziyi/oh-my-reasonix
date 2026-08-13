# OMR Mnemosyne MEM-02F：Freshness Judgment 与 Revalidation 加固计划

- 阶段：MEM-02F / MEM-02-06
- 状态：✅ 已实现并签收
- 前置：MEM-02A～MEM-02E 已实现；现有只读 `EvaluateRevalidation` 可作为实现基础
- 目标：让 Freshness Judgment、Policy、Evidence 与 supersede 链可精确验证，使 Revalidation 结果可审计、确定性、只读，同时保持 Freshness 与 Lifecycle/Health 完全隔离。

## 一、现状、假设与最小边界

### 1.1 已有能力

当前代码已经具备：

- `freshness_evaluation` Judgment v1 payload；
- `PolicyConfigFreshness` 与版本化 PolicyStore；
- `EvaluateRevalidation` 只读报告；
- `fresh | aging | needs_revalidation` 三态；
- 显式 `Now`、PolicyRef 精确加载、基础窗口计算和稳定输出测试。

本阶段不重复实现这些能力，只修补精确身份、时间、Basis、supersede 和 Policy 使用
上的缺口。

### 1.2 Schema 决议

Architecture v1 的 Freshness payload 已完整冻结：

```yaml
memory_ref: MemoryRef
result: fresh | aging | needs_revalidation
evaluated_at: RFC3339 UTC
freshness_policy_ref: PolicyRef
basis_refs: [BasisRef]
```

本阶段**不新增 Schema 字段**：

- 不新增 `freshness_policy_sha256`，因为 `PolicyRef.content_sha256` 已是唯一 Hash
  锚，重复字段会制造第二事实源；
- 不新增 `content_classification_ref`，Architecture v1 的 Freshness Schema 没有
  该字段；内容分类属于 Trust Gate，不是时间老化前置条件；
- 不提高 `schema_version`，不修改旧 Freshness Judgment Canonical Bytes/Hash；
- 不新增 `stale`、`invalid`、`expired` 等持久化结果。

### 1.3 交付边界

允许修改：`internal/memory/**`、本计划、MEM-02 总计划和
`tests/docs_check.sh`。禁止修改 Architecture v1、Evolution、CLI、Prompt、
Reasonix、Desktop、CURRENT；禁止写入 Fact、自动生成 Judgment、自动修改
Revision/Lifecycle/Health、提交、推送与 Tag。

## 二、Freshness Judgment 精确验证

一个 Judgment 只有满足以下条件才可驱动候选：

1. Envelope 严格解码、Hash 正确、`judgment_type=freshness_evaluation`；
2. Judgment Scope、Subject MemoryRef、payload MemoryRef 与目标 Revision 五字段
   完全一致；
3. `freshness_policy_ref` 指向请求指定的同一个不可变 Policy Fact（id/type/hash）；
4. `Judgment.created_at` 与 payload `evaluated_at` 均不晚于显式 `Now`；
5. 每个 BasisRef 精确闭合：
   - MemoryRef → 对应不可变 Revision；
   - EvidenceRef → 目标 Scope Store 中某个 Evidence Generation 的成员；
   - JudgmentRef → 对应 Judgment 的 scope/type/id/hash；
   - PolicyRef → 对应版本化 Policy 的 id/type/hash；
6. SupersedesJudgmentRef 每条边精确匹配实际目标四字段；链节点必须保持同一
   Revision 身份；环、孤儿、错 Scope/Hash/Subject fail closed；
7. 多个互不 supersede 的有效终端若结果冲突，输出稳定诊断并回退窗口计算，不猜
   哪个 Judgment 更可信；结果一致时可采用共同结果。

无关类型的损坏 Judgment 不应污染指定 Revision；相关 Freshness 链损坏必须 fail
closed。

## 三、时间与 Policy 语义

### 3.1 显式时间

- `RevalidationRequest.Now` 必填，禁止 `time.Now()`；
- Policy、Revision、相关 Evidence、相关 Judgment 的 `created_at` 均须
  `<= Now`；
- 未来 Revision/Policy 是固定世界错误，整体 fail closed；
- 未来 Judgment/evaluated_at 或未来 Evidence 不得影响候选，输出稳定诊断并使用
  其余已验证事实计算窗口结果；若没有合法历史输入，则以 Revision.created_at 为
  锚。

### 3.2 窗口映射

保持既有三态，不发明第四态：

```text
age < evaluation_window_days  → fresh
age < aging_after_days        → aging
age >= aging_after_days       → needs_revalidation
```

`stale_after_days` 不产生新状态；当 age 达到该阈值时仍为
`needs_revalidation`，但稳定 reason 使用 `stale_window`，便于后续调度区分紧急度。

Window activity 只接受：

- Revision.created_at；
- `MemoryEvidenceGeneration` 中至少包含一种
  `revalidation_evidence_types` 的非未来 Evidence Generation；
- 满足全部精确验证且仍在 `evaluation_window_days` 内的最新 Freshness Judgment。

过期 Judgment 不永久压住时间演进：超过 evaluation window 后输出
`evaluation_expired` 诊断并回退窗口计算。

## 四、只读报告兼容与诊断

继续使用现有 `RevalidationReport`，不新增事实。允许新增稳定诊断码：

```text
future_judgment
future_evidence
evaluation_expired
policy_drift
conflicting_freshness_judgments
```

诊断只包含受控 MemoryID 与固定 Detail，不输出绝对路径、Prompt、命令、模型思考
或凭据。结构/Hash/权限/symlink/引用损坏仍返回 StoreError，不降级成普通诊断。

Candidate reason 固定为：

```text
judgment_driven | window_driven | stale_window
```

相同 Facts、Policy、Now 必须输出逐字节一致结果；不同 Now 只能改变时间字段、窗口
状态和由时间直接决定的诊断。

## 五、Lifecycle/Health 隔离

- Freshness Judgment 只改变 Freshness 派生维度；
- `aging` 不得 degraded；
- `needs_revalidation` 不得 frozen/superseded/archived；
- Revalidation 不写 Governance Event、Outcome、Usage、Evidence、Revision 或
  Judgment；
- Revalidation 后续若产生新事实，必须走既有不可变 Fact 与生命周期协议，本阶段
  不实现该写入流程。

## 六、TDD 测试矩阵

### 6.1 兼容与身份

1. 旧 Freshness Judgment golden 字节/Hash 不变；
2. Subject 与 payload MemoryRef 五字段一致通过，scope/type/id/revision/hash 任一
   错配 fail closed；
3. PolicyRef id/type/hash 精确匹配；缺失、漂移、未来 Policy 拒绝；
4. Basis 四种 Ref 精确闭合；孤儿、跨 Scope、错 Hash 拒绝；
5. Supersede 合法链、错 ref、错 subject、环、孤儿和并列冲突。

### 6.2 时间与窗口

6. 零 Now 拒绝；相同 Now 字节稳定；
7. 未来 Revision/Policy fail closed；
8. 未来 Judgment/evaluated_at 诊断并回退，不被采用；
9. 未来 Evidence 诊断并忽略；同一 Evidence 的合法历史 Generation 仍可采用；
10. Judgment 在 evaluation window 内驱动结果，过期后诊断并回退；
11. 0～evaluation、evaluation～aging、aging～stale、超过 stale 四个边界；
12. 不在 `revalidation_evidence_types` 的 Evidence 不刷新 activity；匹配类型才刷新。

### 6.3 确定性与隔离

13. 多个一致终端确定性采用；冲突终端诊断并回退；
14. 不同插入/目录顺序输出相同；
15. 前后 Store 文件数、CURRENT、DerivedState Lifecycle/Health 不变；
16. context 取消、损坏 JSON、unknown field、symlink、权限 fail closed；
17. 错误与诊断脱敏；无关损坏非 Freshness Judgment 不污染目标；
18. Fake fixture：Revision + Policy + Evidence + Freshness Judgment → 确定性报告，
    无 API Key。

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

`qualitybench` 因沙箱禁止回环端口时必须在允许监听的环境复核；TempDir cleanup
宿主竞态需如实记录并重跑，不得伪造通过。

## 八、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-02F：Freshness Judgment 与 Revalidation 加固。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 6.2.3、11.2.1、17 章；
- docs/OMR_MNEMOSYNE_MEM-02F_FRESHNESS_REVALIDATION_PLAN.zh-CN.md；
- internal/memory/model.go、policy.go、policy_store.go、revalidation.go、
  derived_state.go、store.go、critic_requirement.go、trust_gate.go。

严格执行计划第二～七章，每阶段先写失败测试并保留修复前证据，再做最小实现。

硬约束：
1. 不扩 FreshnessEvaluationPayload，不新增 freshness_policy_sha256 或
   content_classification_ref；PolicyRef 已携带唯一 Hash。
2. 旧 Freshness Judgment golden 字节/Hash 必须逐字节不变。
3. Subject/payload MemoryRef、PolicyRef、BasisRef、SupersedesJudgmentRef 全字段精确
   验证；相关链损坏 fail closed。
4. 只使用显式 Now；未来 Revision/Policy fail closed，未来 Judgment/Evidence 诊断
   后忽略并回退已验证窗口事实。
5. 保持 fresh|aging|needs_revalidation 三态；stale_after_days 只改变 reason，不新增
   persisted result。
6. 仅 revalidation_evidence_types 中的非未来 Evidence 刷新 activity；过期 Judgment
   不得永久压住时间演进。
7. Freshness 与 Lifecycle/Health 隔离；全流程只读零写入，不读写 CURRENT。
8. 错误与诊断稳定脱敏，不调用模型/网络，不进入 MEM-02G。
9. 覆盖第六章测试，运行第七章门禁，并执行独立 review/security_review。

若代码现状、计划和 Architecture v1 冲突，停止相关实现并报告原文，不自行扩大
Schema。

最终只输出：实际文件；旧 golden 兼容；精确引用与 supersede；时间/窗口矩阵；
Evidence type 过滤；Lifecycle/Health 隔离；失败测试证据；门禁、review、
security_review；[ENV]/剩余问题；明确“未进入 MEM-02G、未提交、未推送、未创建
Tag”。
```

## 九、完成定义

所有失败测试转绿、全量门禁通过、旧 golden 不变、无 blocking/should-fix 后才可
签收。签收前不得把 Revalidation 接入自动写 Evidence、Revision 或 Governance。

## 十、实现结果

MEM-02F 已按本计划完成最小实现：

- 未修改 `FreshnessEvaluationPayload`、Schema Version 或既有 Canonical 编码；
- `EvaluateRevalidation` 精确校验请求 Policy、目标 Revision、Subject/payload
  MemoryRef、BasisRef 和 supersede 引用；相关身份、Hash、Scope、孤儿或环损坏均
  fail closed；
- 未来 Policy/Revision 整体拒绝；未来 Judgment/Evidence 输出稳定诊断并从窗口计算
  中排除；
- 过期 Judgment 输出 `evaluation_expired` 并回退窗口；多个有效终端结果冲突输出
  `conflicting_freshness_judgments` 并回退，一致结果确定性采用最新节点；
- 仅 `revalidation_evidence_types` 中的 Evidence 刷新 activity；达到
  `stale_after_days` 后仍输出 `needs_revalidation`，reason 为 `stale_window`；
- 全流程保持只读，不写 Fact/CURRENT，不修改 Lifecycle/Health；
- 新增 `revalidation_hardening_test.go` 锁定身份伪造、supersede 伪造、过期评价、
  Evidence 类型过滤、未来事实、Basis 孤儿、终端冲突与确定性等回归路径；既有
  Freshness golden 兼容测试继续覆盖旧 Hash。

实现完成后仍遵守阶段边界：未进入 MEM-02G、未提交、未推送、未创建 Tag。
