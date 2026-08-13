# OMR Mnemosyne MEM-05D：迁移预览与 Scope 切换

- 阶段：MEM-05D
- 状态：🟡 设计计划，尚未实现迁移写入
- 前置：MEM-01D Generation 事务、MEM-03C Composite Generation、MEM-05C 只读 Repair/Rollback
- 目标：提供跨版本/跨目录迁移的可审计预览，避免把项目数据隐式变成 Global 数据

## 一、迁移边界

1. 迁移只允许显式的 Project→Project 或 Global→Global；Project→Global 必须走 MEM-05B Promotion，不由迁移命令隐式完成。
2. 源 Scope、目标 Scope、Generation、Manifest 和事实集合必须显式列出并逐项校验；不能扫描“所有项目”猜测数据。
3. 迁移第一阶段只生成 `MigrationPlan`，不复制、删除、覆盖事实，不切换任何 CURRENT。
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

## 三、后续写入事务

真正执行必须：预览 → Snapshot → 复制不可变事实 → 编译派生 Generation → Doctor → 在目标 Scope CAS 切换 CURRENT。失败恢复目标旧入口，源 Scope 永不修改；所有动作追加审计事件并可重复执行。

## 四、TDD 验收矩阵

- Project→Global 拒绝，Project→Project/Global→Global 才可进入预览；
- 源 Generation/Manifest/Fact 缺失、篡改、未来、Scope 错配、权限/symlink 问题 fail closed；
- 计划生成零写入、输出确定性、敏感信息不泄露；
- 重复预览幂等；未来执行步骤不自动批准、不删除源事实；
- race、全量测试、vet、build、Docs Gate 全通过。

## 五、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05D。先读取本计划、MEM-01D、MEM-03C、MEM-05B/05C 与 FactStore Scope/路径安全实现。
先做 Schema Gate，不新增第二事实源、不隐式 Project→Global。严格 TDD，实现只读 MigrationPlan 预览：显式 source/target
Scope、GenerationRef、Manifest Hash、事实集合和固定步骤；校验 Hash、Scope、权限、symlink、未来时间和输入完整性。
计划生成零写入、不复制事实、不切 CURRENT、不调用模型/网络。真正迁移事务留到后续批准阶段。
运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet、go build、docs_check；
未获 CTO 复核前不要提交、推送或创建 Tag。
```
