# OMR Mnemosyne MEM-04B：Attribution Analyst 与 MemoryOutcome 计划

- 状态：✅ 自动化实现完成，待真实 Reasonix Desktop Attribution 回执联调
- 前置：MEM-04A MemoryUsage、MEM-03C Episode/Context、MEM-02B Critic 基础协议
- 目标：把不可信的模型归因候选，经确定性 OMR Attribution Gate 转换为可审计、幂等的 Outcome Fact

## 一、边界与成功标准

Reasonix 只负责语义分析并返回候选；OMR 不在 Go 中穷举失败语义，只验证固定世界、证据、任务边界和
唯一计分协议。模型不得提供 Outcome ID、Hash、时间、`counted_as_help/harm` 或直接写 Store。

```text
Episode + Anchored MemoryUsage + 客观 Evidence
→ Reasonix Attribution Analyst 候选
→ harmed 时附带独立 Critic 结论
→ OMR Attribution Gate
→ Enriched Outcome（不可变）
→ 派生 UsageStats/Lifecycle（本阶段只验证统计，不实现冻结事务）
```

本阶段不自动调用真实模型、不修改 Revision/CURRENT、不实现 Lifecycle 治理事务。分析失败必须降级为
`unknown` 且不计分，不能改变原始任务退出码。

## 二、Schema Gate 决议

### 2.1 AttributionReceipt 是瞬时协议，不是 Fact

```yaml
schema_version: 1
episode_ref: {scope: project, episode_id: episode_..., content_sha256: sha256_...}
root_task_id: task_...
candidates:
  - usage_id: usage_...
    task_outcome: succeeded     # succeeded | failed | cancelled | unknown
    failure_cause_memory_ref: null # 非空时必须是 failure_concept MemoryRef
    memory_effect: helped       # helped | neutral | harmed | unknown
    attribution: confirmed      # confirmed | likely | uncertain
    critic: not_required        # supported | unsupported | not_required | unavailable
    evidence_refs: []           # 完整 EvidenceRef；必须属于 Episode
```

未知字段、重复 usage、超限、自由 reason/summary、Prompt、命令、思考、凭据严格拒绝。失败原因只能引用
可演化的 Failure Concept；不得新增 `rate_limit/auth/network` 等 Go 枚举。

### 2.2 Outcome 采用 Legacy / Enriched 双形态

已有简版 Outcome 的 canonical bytes/hash 永久兼容。Enriched 形态在既有字段之外完整携带：

```yaml
episode_id: episode_...
root_task_id: task_...
context_signature_version: 1
context_signature: sha256_...
context_descriptor_ref: context_...
task_outcome: success
failure_cause_memory_id: ""
memory_stage: evaluated
evaluated: true
attribution: confirmed
critic: not_required
evidence_refs: []
counted_as_help: true
counted_as_harm: false
```

这些字段必须 all-or-none；部分锚定拒绝。Enriched 字段全部进入 canonical hash。Legacy 仍按既有派生语义
读取，防止历史数据失效；新写入只能由 Gate 生成 Enriched 形态。

### 2.3 字段唯一来源

| Outcome 字段 | 唯一来源 |
|---|---|
| `outcome_id` | scope + root_task_id + 完整 MemoryRef + context_signature 的确定性 SHA256 |
| usage/memory/revision/stage/context | 精确加载的 Anchored MemoryUsage |
| episode/root task/time | 精确加载的 Episode Fact；CreatedAt 使用 Episode 规范时间，不读墙钟 |
| task outcome | Episode `task_result` 与候选必须一致 |
| failure cause | 候选 MemoryRef；Gate 在固定 Generation 验证其为 failure_concept |
| effect/attribution/critic | 候选枚举，经 Gate 降级后写入 |
| evidence refs | 候选与 Episode Evidence 集的精确交集校验，不接受裸 ID |
| counted booleans | OMR 按 Architecture 13.6 唯一计算，模型不可提供 |
| content hash | OMR 程序计算 |

## 三、Attribution Gate 固定规则

1. Strict JSON、大小/数量/枚举限制；
2. EpisodeRef、RootTaskID 与调用请求及 Store Fact 精确一致；
3. Usage 必须存在、为 Anchored 形态，并与 Episode/RootTask/Context/Memory 精确一致；
4. 只有 `affected|evaluated` 可归因；参与正负计分必须为 `evaluated`；
5. EvidenceRef 必须完整存在于 Episode `evidence_refs ∪ task_result_evidence_refs`；
6. failure cause 非空时必须在 Usage 固定 Generation 中精确解析为 failure_concept；
7. `harmed` 必须 `critic=supported`，否则降级 `unknown`；非 harmed 必须 `critic=not_required`；
8. `external=true` 的失败（由候选失败原因与客观证据共同确认）不得计 harm；MVP 通过调用请求的
   `ExternalFailure` 客观标志输入，不允许模型单方面设置；
9. `likely|uncertain|neutral|unknown` 不计分；
10. 任一验证不完整时生成 `unknown`、两个 counted=false；引用损坏、越界、Hash 漂移则 fail closed。

唯一计分协议：

```text
help = stage=evaluated AND effect=helped AND attribution=confirmed AND !external_failure
harm = stage=evaluated AND effect=harmed AND attribution=confirmed
       AND critic=supported AND !external_failure
```

### 3.1 Retry 去重

同一 `root_task_id + MemoryRef + context_signature` 的多个 attempt 使用同一个 deterministic outcome_id。
相同最终事实重放为 NOOP；不同事实为 identity conflict，绝不覆盖。不得通过新的 attempt ID 刷帮助或伤害。

### 3.2 人工 Override

本阶段复用已冻结 `attribution_override` JudgmentFact。Override 只能追加，不覆盖 Outcome；subject 必须精确
指向存在的 Outcome，previous_effect 必须等于当前有效 effect，supersede 链必须完整。CLI 人工入口放到
MEM-04C，不在本阶段新增第二套修正协议。

## 四、最小 API 与 CLI

```go
BuildOutcomes(ctx, AttributionRequest) ([]Outcome, error)
CommitOutcomes(ctx, AttributionRequest) (CaptureOutcomeResult, error)
```

```text
omr memory outcome capture \
  --project-dir . \
  --attribution-receipt <file> \
  --external-failure=false \
  --json
```

MVP 只接受单 Scope 批次。批量写入前完成全部 identity preflight；任一冲突整批零写入。

## 五、TDD 验收矩阵

1. Legacy Outcome golden bytes/hash 不变；Enriched all-or-none 严格校验；
2. helped/confirmed/evaluated → counted help；harmed 只有 supported Critic 才 counted harm；
3. likely/uncertain/neutral/unknown、affected 未 evaluated 均不计分；
4. external failure 永不计 harm，第三方失败无法触发冻结；
5. Episode/Usage/Memory/Context/TaskResult/Evidence 精确匹配；缺失与 Hash 漂移 fail closed；
6. Failure Concept 精确固定 Generation 验证，不读 CURRENT；
7. 同 Root Task Retry NOOP，不重复计数；冲突整批零写入；
8. 模型提供 ID/Hash/时间/count 布尔、未知字段、敏感文本、路径字段均拒绝；
9. 人工 Override 原事实不变，派生读取最新合法 Override；孤儿/错链 fail closed；
10. Fake Reasonix 进程级闭环：Episode + Usage + AttributionReceipt → Outcome → stats；无 API Key；
11. 错误脱敏，采集失败不覆盖原始任务退出码；
12. 不读取墙钟、不写 CURRENT/Revision/Lifecycle/Governance Event。

最终门禁：gofmt、diff check、memory/CLI race、全仓 test、vet、build、Docs Gate、review/security review。

## 六、实现结果与剩余边界

已实现 Legacy/Enriched Outcome 双形态、严格瞬时 AttributionReceipt、确定性 Build/Commit、Episode 与
Usage 边界校验、Evidence 闭合、Failure Concept 固定 Generation 校验、唯一计分协议、外部失败隔离、
Retry 幂等和 `omr memory outcome capture`。进程级测试已覆盖 Usage → Attribution → Outcome 落盘闭环。

当前没有自动调用 Attribution Analyst/Critic；Reasonix Desktop 仍需按本协议输出候选回执。`ExternalFailure`
是调用方提供的已验证客观标志，模型回执不能设置。人工 Override 的 Store 事实与派生读取已存在，CLI/Web
管理入口归 MEM-04C。Lifecycle/Freeze 治理事务不在本阶段实现。

## 七、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-04B。先完整读取本计划、Architecture v1 第 12/13 章、memory_usage.go、
usage_capture.go、episode_fact.go、derived_state.go。严格 TDD，先锁定 Legacy Outcome golden，再实现瞬时
AttributionReceipt、Enriched Outcome、纯 BuildOutcomes、批量 preflight/commit 和 CLI。

不得修改 Architecture v1；不得让模型提供 Outcome ID/时间/Hash/counted 布尔；不得在 Go 中添加失败语义
枚举；不得读取 CURRENT 或 time.Now；不得覆盖 Outcome/Judgment；不得把 affected 未 evaluated、likely、
uncertain、external failure 计为 help/harm；不得进入 MEM-04C。

最终运行 gofmt、git diff --check、go test -race ./internal/memory/... ./cmd/omr、go test ./...、go vet ./...、
go build ./cmd/omr、tests/docs_check.sh，并做 review/security review。输出 Schema、Gate、ID/幂等、测试矩阵、
门禁及剩余问题；不要提交、推送或创建 Tag。
```
