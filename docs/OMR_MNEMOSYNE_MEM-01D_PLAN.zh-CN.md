# OMR Mnemosyne MEM-01D：Generation 事务与唯一提交点

## 任务状态

- 阶段：MEM-01D
- 状态：已由 Reasonix Agent 执行完成（Generation 事务、幂等 Claim、CURRENT CAS、崩溃恢复与测试已交付；未进入 MEM-01E）
- 前置：Architecture v1 已冻结；MEM-01A、MEM-01B、MEM-01C 已由 CTO 签收
- 后续：MEM-01E 最小 OKF 编译器

本阶段实现规范事实到不可变 Generation 的事务骨架，重点是 prepared 隔离、Generation Input Manifest、Scope 单写锁、幂等 Claim、CAS 更新 `CURRENT` 和崩溃恢复。

本阶段不实现 OKF 页面内容、不接 CLI、Prompt、Reasonix，不计算 Lifecycle/Health/Usage/Freshness/Trust 结果。

## 一、允许修改与禁止范围

允许修改：

```text
internal/memory/generation_store.go
internal/memory/generation_tx.go
internal/memory/generation_recovery.go
internal/memory/generation_store_test.go
internal/memory/generation_tx_test.go
internal/memory/generation_recovery_test.go
docs/OMR_MNEMOSYNE_MEM-01D_PLAN.zh-CN.md
```

必须复用：

- MEM-01A 的 Fact、GenerationInputManifest、Canonical Hash、DecodeStrict；
- MEM-01B 的安全路径、权限、锁、原子写入、Scope Store；
- MEM-01C 的 PolicyFact/PolicyRef 历史加载。

禁止修改：

- `internal/evolution/**`、`cmd/omr/**`、配置、Manifest、Prompt、assets、`.reasonix/**` 和 Architecture v1；
- 已签收的 Fact Schema、Policy Schema 和错误码语义，除非发现真实冲突并暂停报告；
- OKF Wiki、索引页面、Prompt Composer、Desktop、CLI、Reasonix 调用；
- 自动修复、自动删除历史 Fact、跨 Project/Global Promotion。

## 二、事务目录与事实层级

每个 Scope Store 使用独立事务目录：

```text
<store-root>/
├── facts/
│   ├── memory-revisions/
│   ├── memory-evidence-generations/
│   ├── judgments/
│   ├── policies/
│   ├── governance-events/
│   ├── memory-mutations/
│   └── generation-input-manifests/<generation-id>.json
├── generations/<generation-id>.staging/
├── generations/<generation-id>/generation.json
├── transactions/<transaction-id>/
│   ├── prepared.json
│   ├── commit.json
│   └── abort.json
├── idempotency/<idempotency-key>.json
└── CURRENT
```

规则：

1. `facts/`、Generation Input Manifest、事务记录和幂等记录是规范事实或审计事实；
2. staging Generation 是临时派生产物，未提交前不可被普通读取采用；
3. `CURRENT` 是唯一生效提交点，孤立完整 Generation 不能自动生效；
4. 已发布 Generation 永久不可修改；回滚是新事务，不删除或改写旧 Generation；
5. 所有路径必须复用 MEM-01B 安全 Join、权限和 symlink 检查；
6. Project 与 Global 事务目录、锁、CURRENT 和 Generation 完全隔离。

## 三、事务 API

API 名称可匹配仓库风格，但语义必须固定：

```go
type GenerationStore interface {
    Begin(ctx context.Context, req BeginGenerationRequest) (*GenerationTx, error)
    PrepareFact(ctx context.Context, txID string, fact Fact) error
    PrepareManifest(ctx context.Context, txID string, manifest GenerationInputManifest) error
    ValidateStaging(ctx context.Context, txID string) error
    Commit(ctx context.Context, txID string) (CommitResult, error)
    Abort(ctx context.Context, txID string, reason string) error
    Recover(ctx context.Context) ([]RecoveryAction, error)
}
```

`BeginGenerationRequest` 至少包含：Scope、base_generation、compiler_version、canonicalization_version、schema_version、idempotency_key、输入/目标 Hash 摘要。

API 不返回未经验证的 Generation；每个成功结果必须包含 transaction ID、generation ID、base/target Hash 和提交状态。

## 四、固定事务顺序

每次事务严格按以下顺序执行：

```text
1. 校验 Context、Scope、idempotency_key 和请求 Hash
2. Claim 幂等键（任何业务副作用之前）
3. 获取 Scope 单写锁
4. 读取并固定当前 CURRENT/base_generation
5. 将目标 Fact 安全写入 prepared 区域或规范 facts/，带 transaction_id
6. 校验所有 Fact 的 Schema、Hash、Scope 和敏感边界
7. 写入 Generation Input Manifest（永久事实）
8. 根据已提交 Fact + 本事务 prepared manifest 构建 staging Generation
9. 校验 staging 完整性、输出 Hash 和 Manifest 输入集合
10. 原子发布不可变 Generation 目录
11. 再次确认 CURRENT == base_generation
12. 原子 CAS 更新 CURRENT
13. 写入 committed 事务记录和 Mutation 审计
14. 释放锁
```

任何步骤失败：

- `CURRENT` 不变；
- 旧 Generation 不变；
- 未提交 prepared Fact 不得被普通编译采用；
- 事务记录写入 `aborted` 或保留为可诊断状态；
- 不删除历史规范 Fact；
- 幂等键返回既有结果或稳定失败，不重复产生副作用。

## 五、幂等 Claim 与 CAS

### 幂等键

- Claim 必须先于 Fact 写入、Manifest、staging、Generation 和 CURRENT 任何副作用；
- 相同 key + 相同请求 Hash → 返回既有事务结果；
- 相同 key + 不同请求 Hash → fail closed，返回稳定冲突错误；
- Claim 文件不可覆盖，使用 MEM-01B 的不覆盖式原子提交；
- 崩溃后 Claim 状态只能由 Recovery 根据事务记录判定，不能简单当作未使用。

### CURRENT CAS

- Commit 前必须再次读取 CURRENT；
- `CURRENT != base_generation` → 稳定冲突，旧 CURRENT 保持不变；
- CAS 成功是唯一生效点；
- Generation 已发布但 CAS 失败时，保留孤立 Generation 和 Manifest，禁止自动接管；
- 重试必须使用新的事务或明确相同幂等 Claim，不得复用不一致的 staging。

## 六、Generation Input Manifest

Manifest 必须使用 MEM-01A 的严格模型并永久保存，至少记录：

```yaml
generation_id:
base_generation:
compiler_version:
canonicalization_version:
fact_schema_version:
inputs:
  - fact_type:
    fact_id:
    content_sha256:
    schema_version:
    scope:
policy_refs:
transaction_id:
input_manifest_sha256:
output_generation_sha256:
created_at:
```

硬约束：

- 输入按 `fact_type + fact_id` 去重并确定性排序；
- 同一事实 ID 出现不同 Hash/Scope/SchemaVersion 必须冲突；
- Manifest 必须在 CURRENT 切换前安全落盘；
- Generation 被清理后 Manifest 仍永久保留；
- 历史 Compiler/Canonicalization 不可用时 Recovery/Doctor 必须阻断精确重建，不能用当前算法伪造；
- 不保存绝对路径、Prompt、思考、命令正文、凭据或自由文本知识副本。

## 七、崩溃恢复

`Recover` 只根据规范事实、事务记录、Manifest、Generation Hash 和 CURRENT 做确定性判断：

| 状态 | 恢复动作 |
|---|---|
| Claim 存在但无事务记录 | 保留并报告待诊断，不重新执行未知请求 |
| prepared Fact 部分落盘 | 校验 Manifest；不满足则隔离并标记 aborted |
| Manifest 存在、staging 不完整 | 依据固定输入重建 staging 或报告阻断 |
| staging 完整、Manifest 完整、CURRENT 未变 | 可在 Scope 锁内继续 CAS 或安全 abort |
| Generation 已发布、CURRENT 未切换 | 保留孤立 Generation，不自动生效 |
| CURRENT 已切换、commit 记录缺失 | 根据 Generation/Manifest 补全审计，不创建新规范事实 |
| CURRENT 与事务 base 冲突 | 标记 CAS conflict，旧状态不变 |
| Compiler/Schema 版本不可用 | 返回 `memory_generation_compiler_unavailable`，不得猜测重建 |

恢复不得删除旧 Generation、旧 Manifest、旧 Fact 或用户数据；清理 staging 只能在事务记录明确判定为 aborted 且路径安全时进行。

## 八、错误与安全边界

新增错误码必须稳定且脱敏，例如：

```text
memory_generation_transaction_conflict
memory_generation_idempotency_conflict
memory_generation_current_cas_conflict
memory_generation_manifest_mismatch
memory_generation_staging_invalid
memory_generation_compiler_unavailable
memory_generation_recovery_blocked
memory_generation_already_committed
```

所有错误不得包含绝对路径、Prompt、命令、模型思考、凭据或未经脱敏的输入内容。事务、Manifest、Claim、Generation 路径必须拒绝 symlink、路径穿越和权限不安全目录。

## 九、测试矩阵

先写失败测试，再实现最小事务层：

1. 空 Store 首次 Generation 事务；
2. Fact → prepared → Manifest → staging → Generation → CURRENT 完整提交；
3. Manifest 输入乱序/重复确定性 Hash；
4. 相同幂等键相同输入只提交一次；
5. 相同幂等键不同输入 fail closed 且零副作用；
6. 两个进程相同幂等键只有一个 Claim 成功；
7. CURRENT CAS 冲突保持旧 CURRENT；
8. staging/Manifest/Generation 校验失败保持旧状态；
9. 任意步骤注入失败时旧 Generation 不变；
10. prepared Fact 未提交时普通读取不可见；
11. 已发布 Generation 不可修改；
12. 回滚作为新事务且历史 Generation 保留；
13. 崩溃状态矩阵可确定恢复或阻断；
14. Compiler/Schema 版本不可用阻断重建；
15. Project/Global Scope 事务完全隔离；
16. CURRENT、Manifest、Claim、staging 的 symlink/权限/路径穿越拒绝；
17. 事务错误脱敏；
18. Context 取消、锁超时和进程重启后恢复；
19. 多进程并发提交不产生双 CURRENT 或双成功；
20. 旧 Fact、旧 Manifest、旧 Generation 始终保留。

## 十、非目标

MEM-01D 不实现：

- OKF Wiki 页面和 Markdown 编译；
- Root/Local Index、Librarian、Retrieval；
- Lifecycle、Health、Usage、Freshness、Trust 和 Benchmark 计算；
- `omr memory` CLI、Prompt Composer、Desktop UI；
- 真实 Reasonix/模型调用；
- 自动删除、自动批准或跨项目 Global Promotion。

## 十一、门禁与交付要求

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

环境导致的端口、权限或 Go Cache 失败必须标记 `[ENV]`。交付报告必须列出修改文件、事务顺序、幂等/CAS/恢复行为、Manifest 行为、测试与门禁，并明确“未进入 MEM-01E”。不得自行提交、推送、创建 Tag 或开始下一阶段。

## 十二、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne 的 MEM-01D：Generation 事务与唯一提交点。

先读取：
1. docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
2. docs/OMR_MNEMOSYNE_IMPLEMENTATION_TODO.zh-CN.md
3. docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
4. docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
5. docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md
6. docs/OMR_MNEMOSYNE_MEM-01D_PLAN.zh-CN.md
7. internal/memory/**（已签收的 Fact、Policy、Canonical、Store）

只执行 MEM-01D，不执行 MEM-01E 或任何后续任务。

只允许修改 internal/memory/** 以及本计划状态。禁止修改 Architecture v1、internal/evolution、cmd/omr、配置、Manifest、Prompt、assets、.reasonix。不要接 CLI、Reasonix、OKF Wiki 或 Prompt Composer。

实现 Scope 绑定的 Generation 事务骨架：
- Fact 安全写入 prepared 事务；
- 幂等 Claim 必须先于任何业务副作用；
- Manifest 永久保存，输入按 fact_type+fact_id 去重排序；
- staging Generation 与已提交 Generation 隔离；
- CURRENT 是唯一提交点，使用 CAS 防并发覆盖；
- 失败时旧 CURRENT/Generation/Fact 不变；
- 崩溃恢复只能依据事务记录、Manifest、Hash 和 CURRENT 确定性判断；
- Project/Global 隔离，复用 MEM-01B 的权限、symlink、锁、原子写入和错误脱敏；
- 不实现 OKF 页面、业务评估、Lifecycle、Usage、CLI 或模型调用。

先写失败测试，再实现最小代码。必须覆盖幂等竞态、CURRENT CAS 冲突、prepared 隔离、Manifest Hash、崩溃矩阵、Compiler 不可用、权限/symlink、Context 取消和多进程并发。

验证：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

最后只输出交付报告：修改文件、事务顺序、幂等/CAS/恢复、Manifest、测试、门禁和风险，并明确未进入 MEM-01E。不要提交、推送、创建 Tag。
```
