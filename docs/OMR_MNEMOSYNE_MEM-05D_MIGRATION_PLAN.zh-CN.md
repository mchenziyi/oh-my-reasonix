# OMR Mnemosyne MEM-05D：迁移预览与 Scope 切换

- 阶段：MEM-05D
- 状态：✅ 只读 MigrationPlan、不可变迁移 Snapshot、同 Scope 事实复制、Migration Doctor 与目标 Generation/CURRENT 事务已实现并通过自动门禁；真实跨项目联调仍未完成
- 前置：MEM-01D Generation 事务、MEM-03C Composite Generation、MEM-05C 只读 Repair/Rollback
- 目标：提供跨版本/跨目录迁移的可审计预览，避免把项目数据隐式变成 Global 数据

## 一、迁移边界

1. 迁移只允许显式的 Project→Project 或 Global→Global；Project→Global 必须走 MEM-05B Promotion，不由迁移命令隐式完成。
2. 源 Scope、目标 Scope、Generation、Manifest 和事实集合必须显式列出并逐项校验；不能扫描“所有项目”猜测数据。
3. 计划生成只读；事实复制必须显式调用 `ApplyMigrationCopy`，不删除、不覆盖事实，也不切换任何 CURRENT。
4. 复制目标必须是独立 Store 根，路径、symlink、权限、Scope 不符合要求时 fail closed。

## 二、MigrationPlan

```yaml
operation: migration_preview
source_scope: project|global
target_scope: project|global
source_generation_ref: Project/GlobalGenerationRef
input_manifest_sha256: sha256_...
fact_count: 0
snapshot_required: true
steps: [preview, snapshot, copy, compile, doctor, switch]
eligible: false
blocked_reasons: []
```

`eligible=true` 只表示所有只读检查通过，不代表迁移已执行。计划不能包含绝对路径、项目名、命令、Prompt、凭据或模型输出。

`BuildMigrationPlanFromStores` 仅接受两个不同根目录且 Scope 相同的已打开 Store；它会重新验证源 Generation、永久
GenerationInputManifest 与输入数量，目标 Store 保持零写入。跨 Scope 仍稳定阻断并要求走 Promotion；任何损坏、Scope
不匹配或缺失都 fail closed。

`ApplyMigrationCopy` 是显式同 Scope 复制入口：重新验证源 Generation/Manifest，优先读取已持久化 Fact；MEM-01D
prepared 输入仍隔离时，只读取对应已提交 transaction 的精确 canonical Fact，不扫描或猜测其它事实，然后通过单锁
`PutBatch` 写入目标。重复复制为 NOOP，冲突或任一输入失败时目标批次零生效。目标 Generation 编译、Doctor 和
`ApplyMigration` 在复制成功后创建新的目标 Generation 事务，复用源的已验证编译输出，重新计算目标 Manifest/Generation
Hash，并通过正常 `Begin → PrepareFact → PrepareManifest → WriteCompiledOutput → Commit` 流程执行目标 CURRENT CAS。
源 Generation ID 不复用，源 Scope/CURRENT 不修改；目标已存在的事实按 PutBatch 幂等处理，目标编译输出或源完整性失败时
不切换目标 CURRENT。跨 Scope 仍拒绝，必须走 Promotion。

## 三、后续写入事务

真正执行必须：预览 → Snapshot → 复制不可变事实 → 编译派生 Generation → Doctor → 在目标 Scope CAS 切换 CURRENT。`ApplyMigration`
已覆盖复制、编译和 CAS；`CheckMigrationReadiness` 提供只读 Doctor，重新验证源 Generation/Manifest/输入事实并逐项报告目标
事实的缺失、已存在或冲突。它不写入 FactStore、不创建 Snapshot、不切换 CURRENT；真实跨项目联调仍待后续。

`ApplyMigration` 现在在目标 Generation 事务 Claim 后、任何事实复制前写入
`migration-snapshots/<snapshot-id>.json`。Snapshot 只保存源 Generation/Manifest、Scope、完整计划 Hash
和目标基线 Generation，不复制事实、不成为第二事实源；同一内容重复写入为 NOOP，内容冲突或损坏均拒绝覆盖。

事实复制与后续 Generation 准备共享同一目标写锁；若准备、编译、Manifest 或 Commit 在 CURRENT 切换前失败，
事务会只撤销本次新建且字节未变化的目标事实，保留既有事实、Snapshot 和可诊断事务记录。CURRENT 切换后若进入
PendingRecovery，则不撤销事实，交由 Recover 完成审计，避免把已生效 Generation 变成半截状态。

显式复制入口：

```bash
omr memory migration copy \
  --source-dir /path/source \
  --target-dir /path/target \
  --scope project \
  --generation-id <generation-id> \
  --json
```

`copy` 只执行已验证输入事实的同 Scope `PutBatch`，重复执行幂等；它不编译目标 Generation、创建 Snapshot 或切换 CURRENT。

## 四、TDD 验收矩阵

- Project→Global 拒绝，Project→Project/Global→Global 才可进入预览；
- 源 Generation/Manifest/Fact 缺失、篡改、未来、Scope 错配、权限/symlink 问题 fail closed；
- 计划生成零写入、输出确定性、敏感信息不泄露；
- 重复预览幂等；未来执行步骤不自动批准、不删除源事实；
- race、全量测试、vet、build、Docs Gate 全通过。

## 五、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05D。先读取本计划、MEM-01D、MEM-03C、MEM-05B/05C 与 FactStore Scope/路径安全实现。
先做 Schema Gate，不新增第二事实源、不隐式 Project→Global。严格 TDD，实现只读 MigrationPlan 预览、Migration Doctor 与显式同 Scope
`ApplyMigrationCopy`：显式 source/target
Scope、GenerationRef、Manifest Hash、事实集合和固定步骤；校验 Hash、Scope、权限、symlink、未来时间和输入完整性。
计划生成和 Doctor 零写入；`copy` 只读取已验证源事实并通过 PutBatch 写目标，不切 CURRENT、不调用模型/网络。目标编译与
CURRENT 切换留到后续批准阶段。

### 3.1 幂等 Claim 与锁顺序收口

`migration apply` 的幂等 Claim 必须绑定完整 `MigrationPlan`（Generation、Manifest Hash、Scope、FactCount
与步骤），不能只绑定 GenerationStore 的通用元数据。事务先 Claim，再读取输出并在同一目标 Store 写锁内复制
不可变事实、准备 Generation、提交 Manifest/Generation/CURRENT；已持锁路径不得再次获取同一 Store 锁。
同一 key 的不同计划在任何目标事实副作用前返回稳定的幂等冲突。
运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet、go build、docs_check；
未获 CTO 复核前不要提交、推送或创建 Tag。
```
