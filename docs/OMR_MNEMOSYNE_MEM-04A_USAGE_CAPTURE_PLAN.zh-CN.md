# OMR Mnemosyne MEM-04A：结构化 MemoryUsage 采集计划

- 状态：✅ 自动化实现完成，待真实 Reasonix Desktop 回执联调
- 前置：MEM-03B Librarian、MEM-03C Composite/Episodic Recall、MEM-02A Anchored MemoryUsage
- 目标：把 Reasonix 对记忆的实际使用转换为可验证、幂等、不可变的 `MemoryUsage` Fact

## 一、边界与成功标准

Reasonix 负责执行任务并输出结构化回执；OMR 负责验证固定世界、引用、Episode 与 Context，最后
生成 Fact。模型不能直接提供 `usage_id`、时间、Hash、Outcome 或正负归因，也不能直接写 Store。

本阶段成功标准：

```text
固定 LibrarianReceipt
→ Reasonix 返回最终使用阶段回执
→ 任务完成并永久写入 Episode + ContextDescriptor
→ OMR 验证全部引用
→ 每个 Root Task + MemoryRef 只生成一条最终阶段 MemoryUsage
```

本阶段不生成 `Outcome`，不判断 helped/harmed，不修改 Lifecycle，不触发冻结，不调用模型分析归因。
采集失败只写稳定脱敏诊断，不改变原始 Reasonix 任务退出码。

## 二、Schema Gate 决议

### 2.1 回执不是新 Fact

新增瞬时协议对象 `MemoryUsageReceipt`，不进入 `facts/`：

```yaml
schema_version: 1
retrieval_id: retrieval_...
root_task_id: task_...
memory_context:                 # 与 LibrarianReceipt 完全相同
  schema_version: 1
  retrieval_id: retrieval_...
  project: { ... }
  global: null
episode_ref:                    # 已完成任务的规范 Episode
  scope: project
  episode_id: episode_...
  content_sha256: sha256_...
usages:
  - memory_ref: { ... }
    usage_stage: read           # read | adopted | affected | evaluated
```

禁止回执携带 `usage_id`、`occurred_at`、`created_at`、`content_sha256`、Outcome、Effect、Prompt、
命令、思考、凭据或自由归因文本。未知字段严格拒绝。`retrieved` 不由模型声明：它从已验证的
LibrarianReceipt 候选集合确定性产生；模型只声明父 Agent 实际完成的更高最终阶段。

### 2.2 每个任务只记录最终阶段

同一 `root_task_id + retrieval_id + MemoryRef` 最多记录一条 Usage，阶段优先级固定为：

```text
retrieved < read < adopted < affected < evaluated
```

若回执重复同一 MemoryRef，只保留最高阶段后再生成一个 Fact。不得为五个阶段分别写五条 Usage，
否则会膨胀 `usage_count`。同一 Root Task 的 Retry 使用同一个 RootTaskID，因此确定性 UsageID 相同，
重放为 NOOP；内容不一致为 identity conflict，不覆盖。

### 2.3 规范字段来源

| MemoryUsage 字段 | 唯一来源 |
|---|---|
| `usage_id` | 程序对 scope/root_task_id/retrieval_id/完整 MemoryRef 做确定性 SHA256 后生成 |
| `scope/memory_id/revision` | 已验证 MemoryRef |
| `usage_stage` | LibrarianReceipt 派生 `retrieved`，或结构化回执的最高阶段 |
| `episode_id/root_task_id` | 已验证 Episode Fact；必须与回执一致 |
| `occurred_at/created_at` | Episode Fact 的规范时间；禁止 `time.Now()` |
| `retrieval_id` | 固定 LibrarianReceipt |
| `memory_context` | 由固定 RetrievalContext 确定性转换，绝不读取新 CURRENT |
| `context_signature*` | ContextDescriptor Fact 的 version + canonical hash |
| `context_descriptor_ref` | Episode 指向的 ContextDescriptor ID |
| `observation_provenance` | `runtime_observed` + 指向 Episode 的 EvidenceRef |
| `source` | 固定 `reasonix_receipt` |
| `content_sha256` | 程序计算 |

Episode EvidenceRef 固定为 `evidence_type=episode`，ID/Hash 与 EpisodeRef 完全一致。它已经是永久规范
事实，因此不新增第二套 Evidence Registry。若 Episode、ContextDescriptor 或固定 Generation 尚未落盘，
返回 `pending_capture`，不写部分 Usage。

## 三、验证顺序

1. Strict JSON、大小上限、字段/枚举/数量限制；
2. 验证 LibrarianReceipt 与固定 IndexTree；
3. 回执的 retrieval/root task/memory context/episode 与调用请求精确匹配；
4. 精确读取 Episode Fact 与 ContextDescriptor Fact，验证 scope、Hash、RootTaskID 和引用闭合；
5. 每个 MemoryRef 必须存在于 LibrarianReceipt recommended/optional 集合及其固定 Generation；
6. 模型不得把未推荐 Memory、frozen/archived/superseded Memory 或 Episode Card 冒充 MemoryUsage；
7. 程序折叠最终阶段、生成 ID/时间/Provenance/Hash；
8. 在 Scope Store 单写锁下先验证全部目标，再批量写入；任一冲突则整批零写入。

第一版最大 32 条 Usage。错误消息不得包含绝对路径、任务摘要、候选理由、页面正文、命令、Prompt、
Episode 内容或凭据。

## 四、API 与 CLI

库级最小 API：

```go
BuildMemoryUsages(ctx, CaptureUsageRequest) ([]MemoryUsage, error)
CommitMemoryUsages(ctx, CaptureUsageRequest) (CaptureUsageResult, error)
```

CLI：

```text
omr memory usage capture \
  --project-dir . \
  --librarian-receipt <file> \
  --usage-receipt <file> \
  --json
```

默认只处理 Project Scope；Global MemoryRef 必须显式提供 `--global-dir`，并写入 Global Store。跨 Scope
批次必须先完成两侧只读验证，再按稳定顺序分别提交；若无法提供真正跨 Store 原子性，则 v1 拒绝混合
Scope 批次，调用方拆成两个回执，不能伪称原子提交。

## 五、TDD 与功能验收

先写失败测试，再做最小实现：

1. 合法 retrieved/read/adopted/affected/evaluated 最终阶段；
2. 同 Memory 多阶段只生成一条最高阶段；
3. Retry/重复回放 NOOP，不重复计数；
4. 同 ID 不同内容 fail closed，整批零写入；
5. MemoryRef 不在固定 LibrarianReceipt/Generation 中拒绝；
6. Episode/Context 缺失、Hash 漂移、RootTask/Scope 不匹配拒绝或 `pending_capture`；
7. frozen/archived/superseded、路径穿越、symlink、超限、未知字段和敏感文本拒绝；
8. 不读取 CURRENT、不读未来 Generation、不使用墙钟；
9. 采集失败不改变模拟 Reasonix 任务退出码；
10. Fake Reasonix 进程级闭环：固定 context → LibrarianReceipt → Episode → UsageReceipt → Usage Fact；
11. `DeriveState` 只增加 usage_count，未产生 Outcome 前 help/harm 均为 0；
12. Project/Global 隔离与混合 Scope fail closed。

最终门禁：gofmt、diff check、memory/CLI race、全仓 test、vet、build、Docs Gate；无 API Key、无网络、
无真实模型调用。自动门禁通过后提交推送；真实 Reasonix Desktop 回执联调另行验收。

## 六、交给 Reasonix 的执行提示词

```text
执行 OMR Mnemosyne MEM-04A。先完整读取：
- docs/OMR_MNEMOSYNE_MEM-04A_USAGE_CAPTURE_PLAN.zh-CN.md
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- internal/memory/memory_usage.go
- internal/memory/librarian.go
- internal/memory/episode_fact.go

严格 TDD：先写失败测试证明旧实现缺少结构化采集，再最小实现。不得修改 Architecture v1，不得新增
Outcome/归因/Lifecycle 逻辑，不得让模型提供 ID、时间或 Hash，不得读取新 CURRENT，不得把 Episode Card
当 MemoryRef。优先实现纯函数 BuildMemoryUsages，再实现批量零副作用验证和 CLI。每完成一层运行聚焦测试。

最终运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet ./...、
go build ./cmd/omr、tests/docs_check.sh，并做 review/security review。输出修改文件、Schema、ID 生成、
事务/幂等、测试矩阵、门禁与剩余问题；不要提交、推送、创建 Tag，也不要进入 MEM-04B。
```

## 七、实现结果与剩余边界

已实现：

- `MemoryUsageReceipt` 严格瞬时 Schema，未知字段和归因字段 fail closed；
- `BuildMemoryUsages` 只从固定 LibrarianReceipt、Episode、ContextDescriptor 派生可信字段；
- 同一候选的多阶段折叠为最高最终阶段，未声明候选记录为 `retrieved`；
- UsageID 对 RootTask/Retrieval/完整 MemoryRef 确定性生成，Retry 重放为 NOOP；
- 单 Scope 批量提交在写入前完成全量 identity preflight，冲突零写入；
- `omr memory usage capture` 支持安全文件读取和稳定 JSON 结果；
- 进程级测试真实构建 OMR，并完成 Generation → LibrarianReceipt → Episode → UsageReceipt → Fact；
- 未产生 Outcome 时只增加 usage_count，help/harm 保持 0。

剩余边界：真实 Reasonix Desktop 尚需输出符合协议的 `MemoryUsageReceipt`；知识型 LibrarianReceipt 的
自动生成仍依赖后续 MEM-03E/宿主接入。联调前不宣称自动采集已在 Desktop 默认启用。本阶段没有实现
Outcome、归因、Lifecycle 自动变更或跨 Store 原子提交。
