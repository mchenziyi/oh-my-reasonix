# OMR Mnemosyne MEM-05C：Repair、Rollback 与迁移前诊断

- 阶段：MEM-05C
- 状态：🟡 设计计划，尚未实现 Repair/Rollback 写入命令
- 前置：MEM-01D Generation/CURRENT/Recover、MEM-03C Composite Generation、MEM-05A/05B 只读计划
- 目标：把损坏检测、可重建性判断和回滚预览统一为确定性只读计划，避免自动修复成为第二事实源

## 一、事实源与安全边界

1. Revision、Evidence、Judgment、Policy、Manifest 和 Governance Event 是唯一规范事实；Repair 只能重建派生 Generation/Index/OKF。
2. 不删除、不覆盖规范事实；任何 CURRENT 切换必须由后续显式事务完成，并保留 Snapshot 与 Governance Event。
3. 不把历史 `alive`、临时目录或派生状态当成事实；损坏、孤儿和缺失都先报告为诊断。
4. 所有操作显式指定 Project/Global Scope；不能跨 Scope 自动修复或迁移。

## 二、RepairPlan / RollbackPlan

本阶段先实现库层只读对象：

- `RepairPlan`：当前 Generation、输入 Manifest、可重建/阻断原因、预期输出 Hash、受影响派生视图；
- `RollbackPlan`：当前 Generation、目标历史 Generation、两者的 Manifest/Hash、CAS 前置条件和风险；
- `MigrationPlan`：源 Scope、目标 Scope、Snapshot、复制/编译/Doctor 步骤与明确禁止项。

对象只能来自 `generationStore.Recover`、永久 Manifest、已验证 Generation 和 FactStore Diagnose，不能凭路径猜测或读取 CURRENT 之外的未验证内容。

## 三、确定性与事务顺序

- 相同固定事实、显式时间和请求产生逐字节相同计划；不读取墙钟、不调用模型/网络；
- Repair：验证输入事实 → 验证 Compiler → 构建 staging → 计算输出 Hash → 只读报告；
- Rollback：验证目标 Generation 完整性 → 校验当前 CURRENT CAS → 生成 Snapshot/治理请求 → 返回待批准计划；
- Migration：预览 → Snapshot → 复制 → 编译 → Doctor → 待批准切换；任一步失败不修改源/目标 Scope；
- 真正写入必须复用 MEM-01D Generation 事务与 FactStore no-overwrite/CAS，不新增旁路文件格式。

## 四、TDD 验收矩阵

1. 完整 Generation 可重建；篡改页面、Manifest、Hash、symlink 或权限问题 fail closed；
2. 缺失 Compiler/Policy/输入 Fact 返回稳定阻断，不伪造成功；
3. Rollback 目标不是已验证历史 Generation、Scope 不同或 CAS 已变化时拒绝；
4. Plan 生成零写入、CURRENT/规范 Fact/Index 不变；
5. 重复请求和输入排序输出字节稳定；
6. 诊断不泄露路径、命令、Prompt、项目名或凭据；
7. Snapshot、审计和未来写入边界由测试锁定，不能通过 Plan 直接生效；
8. race、全量测试、vet、build、Docs Gate 通过。

## 五、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05C。先读取本计划、MEM-01D Generation/CURRENT/Recover、MEM-03C Composite Generation、
MEM-05A/05B 与 FactStore Diagnose 实现。

先做 Schema Gate：复用现有 Generation/Manifest/Recovery，不新增第二事实源、旁路 CURRENT 或未审计文件格式。
严格 TDD，先写失败测试，再实现只读 RepairPlan、RollbackPlan、MigrationPlan。所有计划必须基于已验证固定事实和显式
Scope，检查 Hash、Manifest、Compiler、symlink、权限、CAS；损坏或不可重建时 fail closed。Plan 生成零写入、不读墙钟、
不调用模型/网络、不改变 CURRENT/Lifecycle/Index、不删除任何事实。真正写入与回滚留待后续批准事务。

运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet ./...、go build ./cmd/omr、
tests/docs_check.sh，并完成 code review/security review。未获 CTO 复核前不要提交、推送或创建 Tag。
```
