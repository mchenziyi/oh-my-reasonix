# OMR Mnemosyne MEM-05B：Global Promotion Generation 发布计划

- 阶段：MEM-05B-Generation
- 状态：🟡 设计完成，待实现
- 前置：GlobalPromotionCandidate、`ApplyPromotionCandidate`、MEM-01D Generation 事务、OKF Compiler
- 不依赖：Reasonix 官方接口、人工批准、模型调用

## 一、目标与边界

把已经物化为 Global `probation` 的 MemoryRevision 纳入一个新的 Global Generation，并通过现有
GenerationStore 的 Manifest/CAS/Recover 事务发布派生 OKF。该阶段不把 probation 变成 active，不创建人工批准事实，
不修改 Project Store，不删除历史 Generation。

## 二、请求模型

```text
GlobalPromotionGenerationRequest:
  candidate: GlobalPromotionCandidate       # status=eligible
  target: MemoryRevision                    # Scope=global, Revision=1
  source_bindings: [PromotionCandidateSource]
  index_policy_ref: PolicyRef
  evaluation_time: explicit RFC3339         # 禁止隐式 time.Now
  idempotency_key: controlled identifier
  base_generation: optional global GenerationID
```

调用方必须先通过 `ApplyPromotionCandidate` 完成来源绑定与 Global Revision 写入；Generation 发布再次读取并校验目标
Fact、Policy、Evidence、Candidate Hash 和 `generalized_from` 关系，不能信任调用方缓存。

## 三、固定事务顺序

1. 校验 Candidate/来源绑定/目标 Revision/Policy 与显式时间；
2. 用 `CompileOKF` 对 Global Store 的显式 Revision/Evidence/Policy 输入生成确定性输出；
3. `GenerationStore.Begin` 做幂等 Claim，绑定请求 Hash 与 base Generation；
4. `PrepareFact` 写入目标 Revision、候选事实及编译输入的 prepared 区域；
5. `PrepareManifest` 保存完整 `GenerationInputManifest`；
6. `WriteCompiledOutput` 与 staging 完整性校验；
7. `Commit` 复用 MEM-01D 的永久 Manifest → Generation → CURRENT CAS → audit/claim 顺序；
8. 任意失败保留历史事实和孤儿 staging，返回稳定错误或 PendingRecovery，不删除、不伪造成功。

## 四、必须锁定的语义

- Global `probation` 只进入观察期；Lifecycle/Health 由既有 DerivedState 重新计算；
- Candidate、Revision、Evidence、Policy 和 Manifest 都进入同一个 Generation 的输入集合；
- 输出 Hash、Manifest Hash、Generation Hash、CURRENT 均可从规范事实确定性重建；
- 不读取 Project Store 的 CURRENT，不扫描未声明的项目，也不把路径写入任何事实；
- 重复请求同 key 同请求 Hash 可重放，不同 Hash fail closed；base 变化触发 CAS 冲突；
- OKF 编译失败、来源漂移、symlink、未来时间和 Policy 不满足均在 CURRENT 切换前失败；
- 发布后页面篡改、删除或 symlink 必须由现有 `verifyPublished`/Recover 拒绝。

## 五、验收矩阵

1. 单 Global probation Revision 能生成并发布 Global Generation；
2. 三个 Project 来源绑定错误、Family 缺失、Target relation 错误均零切换；
3. OKF/Manifest 输入闭合、内容 Hash 与 Generation Hash 一致；
4. 同 key 重放 NOOP/恢复，异请求冲突，base 竞争只有一个成功；
5. 编译失败、Manifest 失败、Generation 失败、CURRENT CAS 冲突均保留可诊断事实；
6. Project Store/CURRENT/事实完全不变；
7. `go test -race ./internal/memory/...`、全量测试、vet、build、docs gate 全通过。

## 六、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05B-Generation。先读取本计划、PROMOTION_CONVERGENCE、MEM-01D GenerationStore/Recover、
OKF Compiler、ApplyPromotionCandidate 与 Architecture v1 Global Promotion。
先写失败测试，再实现最小 Global Promotion Generation 发布事务。必须复用现有 GenerationStore 的 Claim、Manifest、
staging、CAS、Recover 和 no-overwrite；不要新增人工批准 Fact、旁路 CURRENT、第二事实源或扫描未声明 Project Store。
请求必须显式携带 Candidate、source bindings、Global target、Index Policy、evaluation_time、base_generation、
idempotency_key；先重新验证所有来源和 Hash，再 CompileOKF，最后按 MEM-01D 顺序提交。
Global Revision 只能保持 probation，不得自动 active。覆盖成功、重放、冲突、编译失败、Manifest/CAS/恢复、symlink、
跨 Scope、Project 不变测试。运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet、
go build ./cmd/omr、bash tests/docs_check.sh；完成 review/security review 后再报告，不要自行创建 Tag。
```

