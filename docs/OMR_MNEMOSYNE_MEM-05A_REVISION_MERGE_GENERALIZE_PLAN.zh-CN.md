# OMR Mnemosyne MEM-05A：Revision、Merge、Split 与 Generalize

- 阶段：MEM-05A
- 状态：🟡 设计计划，尚未进入代码实现
- 前置：MEM-01A～MEM-04C 自动化链已完成；MEM-04A～04B 的真实 Desktop 回执仍待联调
- 目标：在不覆盖既有 Revision、不开启自动 Global Promotion 的前提下，定义可审计的修订、合并、拆分与泛化计划

## 一、不可变边界

1. Revision 永不原地修改；任何修订都创建新的 `memory_id + revision` 事实。
2. Merge、Split、Generalize 首先产生只读结构化计划，不直接写入 Revision、CURRENT 或 Global Active 状态。
3. 所有来源必须通过 `MemoryRef`/`EvidenceRef` 精确引用；引用的 Scope、Hash、Revision 不匹配时 fail closed。
4. 项目记忆不得被移动、删除或隐式提升为 Global；Global 结果必须显式批准并独立写入。
5. 既有 Lifecycle、Health、Usage、Judgment 和 Governance Fact 作为规范事实，任何索引或计划均为派生表示。

## 二、计划对象与确定性

第一版只定义库层只读计划对象，不新增持久化 FactKind：

- `RevisionPlan`：目标 MemoryRef、变更摘要引用、来源 RevisionRef、证据集合和 compiler version；
- `MergePlan`：多个候选 MemoryRef、确定性主 ID、冲突项、合并后待写入 Revision 草案；
- `SplitPlan`：一个来源 MemoryRef、分支键、每个分支的证据闭包和待写入 Revision 草案；
- `GeneralizePlan`：项目来源 MemoryRef 集合、脱敏后的抽象摘要、新的 Global memory ID 候选及阻断原因。

所有计划必须：

- 使用固定排序和 canonical JSON，重复输入产生逐字节相同输出；
- 不包含 Prompt、命令正文、思考、绝对路径、项目名、凭据或远端标识；
- 只返回结构化字段，不调用模型、不联网、不写文件；
- 在证据不足、跨 Scope、Hash 漂移、Policy 不允许或冲突未解决时返回稳定拒绝结果。

## 三、Merge / Split 主键规则

Merge 主 ID 按以下顺序唯一确定，不使用成功次数或随机数：

1. 证据链完整度更高者优先；
2. `created_at` 更早者优先；
3. `memory_id` 规范字典序作为最终决胜。

Split 不复用来源 ID；每个分支必须生成新的候选 ID，且保留 `derived_from` 来源引用。任何分支证据不足都只使该分支不可用，不影响其他分支计划。

## 四、Generalize 安全门

Generalize 只允许从至少两个相互独立的 Project Scope 来源生成候选 Global 记忆；当前阶段不批准、不写入 Global Active。必须拒绝：

- 单一项目来源或重复 Root Task；
- 包含绝对路径、项目名、远程地址、凭据或不可脱敏文本；
- `UsagePolicy` 不允许的来源、冻结/归档来源、未解决冲突或 Trust Gate 非 trusted；
- 任何跨 Scope 引用无法精确验证的输入。

计划输出 `promotion_eligible=false` 或稳定阻断原因；不把“计划可生成”解释为“Global Promotion 已发生”。

## 五、TDD 与验收矩阵

实现前先写失败测试，至少覆盖：

1. Revision 只增不改，旧字节和 Hash 保持不变；
2. Merge 主 ID 的三层排序、重复输入幂等、冲突 fail closed；
3. Split 分支 ID 不复用、分支证据隔离、单分支失败不污染其他分支；
4. Generalize 两项目来源门槛、单项目/重复来源拒绝；
5. Scope、MemoryRef 五字段、EvidenceRef 四字段和 PolicyRef 精确校验；
6. 敏感信息、路径、未知字段和超大输入拒绝且错误脱敏；
7. 计划生成零写入、零 CURRENT 变化、零 Lifecycle 变化；
8. 相同输入、排序变化和重复调用输出字节稳定；
9. `go test -race ./internal/memory/...` 与全仓门禁通过。

## 六、明确不在 MEM-05A

- 不实现自动批准、Global Promotion、跨设备同步或 Web 管理页面；
- 不调用 Reasonix/模型，不读取 Desktop Session/Task 私有状态；
- 不修改 Architecture v1、MEM-01A～MEM-04C 冻结 Schema；
- 不创建 Tag/Release；不在真实正式项目上试运行。

## 七、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-05A。先完整读取本计划、Architecture v1、MEM-01A～MEM-04C 计划与 internal/memory
现有 FactStore/MemoryRef/Policy/Trust/Lifecycle 实现。

先做只读 Schema Gate：确认计划对象是否能复用现有 Envelope；若需要新增持久化字段或 FactKind，先停下并
报告冲突，不得自行改 Architecture v1。通过后严格 TDD：先写失败测试，再实现最小库层只读 RevisionPlan、
MergePlan、SplitPlan、GeneralizePlan。所有输入必须精确校验 Scope/Hash/Ref，输出 canonical、零写入、无模型/网络、
不改变 CURRENT/Lifecycle/Global Active。Merge 主 ID 使用证据链完整度→created_at→memory_id；Split 不复用来源 ID；
Generalize 至少需要两个独立 Project 来源且只输出候选计划。

完成后运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet ./...、go build
./cmd/omr、bash tests/docs_check.sh，并做 code review/security review。报告未决协议问题；未获 CTO 复核前不要提交、推送或创建 Tag。
```
