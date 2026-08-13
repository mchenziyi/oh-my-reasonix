# OMR Mnemosyne MEM-05B：Global Promotion 协议收敛

- 状态：✅ Schema Gate 通过（2026-08-13）；代码实现继续沿用现有 Candidate/Plan/Apply 边界
- 目的：消除 Global Promotion 文档中“人工批准”与 Architecture v1 自动进入 `global/probation` 的冲突

## 一、唯一有效语义

Architecture v1 §17.5/§11.5 规定：当某个 Usage Policy 的 Global Promotion 门槛满足后，系统可以自动创建一条新的
Global `probation` MemoryRevision。它不是 `global_active`，不切换 Global CURRENT，也不绕过后续观察、Critic 和
Lifecycle 规则。因此正常 Promotion 不需要逐条人工批准。

`GlobalPromotionCandidate` 是门槛尚未满足或等待完整物化输入时的规范事实；`status=eligible` 只表示确定性门禁通过，
不是 Active，也不是人工批准状态。

## 二、现有入口的重新定义

`ApplyPromotionPlan` 是“显式物化调用方提供的 Global probation Revision”入口，不是人工批准接口。它继续要求：

1. Project 来源、Evidence、Trust Policy、Scope 和目标 Revision Hash 全部精确验证；
2. 目标 Revision 必须是 Global、Revision 1，并通过 `generalized_from` 关系表达来源；
3. 只写入 Global FactStore，不修改 Project Facts、不切换 CURRENT、不重建索引；
4. 同身份同 Hash 为 NOOP，异 Hash fail closed。

自动触发器、跨多个 Project Store 的来源装配和 Global Generation/OKF 发布属于后续事务阶段，不能通过伪造一个批准记录
来替代。来源 Store 身份必须由调用方显式提供，不能从 MemoryRef 猜测项目路径。

## 三、治理审计边界

普通 Promotion 不创建“批准事实”。以下高影响操作仍可使用现有 `GovernanceEvent` 或专用事务审计，但它们不是
Promotion 成功的前置条件：

- 手工冻结/解冻、回滚、覆盖或拒绝候选；
- Global `probation` → `active` 的受控治理动作（若未来协议允许）；
- 事务恢复、失败和操作员原因记录。

任何新增 Promotion 审计 Fact、Snapshot 或 Global Generation 发布协议，都必须先补充联合 Schema、事实来源、恢复矩阵和
Docs Gate；不得在本阶段偷偷扩展 `GovernanceEvent.operation` 枚举。

## 四、禁止的兼容方向

- 不把人工批准字段加入 `GlobalPromotionCandidate`；
- 不把 `promotion_eligible` 改写成 `active` 或 `approved`；
- 不让 Project→Global 迁移命令代替 Promotion Gate；
- 不因“等待人工批准”而阻塞已满足门槛的 `global/probation` 物化；
- 不为跨项目来源猜测路径、项目名或 Store 身份。

## 五、来源绑定物化（已实现库层）

`ApplyPromotionCandidate` 已提供显式库层物化入口。调用方必须为 Candidate 的每个 `MemoryRef` 提供对应的 Project
`FactStore` 与 Family fingerprint；函数逐项验证来源身份、Family 集合、`generalized_from` 关系、目标 Hash，并将目标
Revision 写入 Global Store。来源 Store 不从路径或 MemoryID 推导，重复目标写入遵循 FactStore NOOP/冲突语义。

该入口只创建 Global `probation` Revision，不批准、不切换 CURRENT、不生成索引，也不会把错误的跨项目绑定降级成成功。

桌面/CLI 调用可使用一次性请求文件（项目目录只作为运行时输入，不会写入 Candidate）：

```bash
omr memory promotion candidate apply \
  --global-dir /path/global \
  --input /tmp/promotion-candidate-apply.json \
  --json
```

## 六、下一步实现前置

下一阶段若要实现自动物化，必须先定义一个只读、显式的来源装配请求：每个 `MemoryRef` 绑定调用方提供的 Project
FactStore 与不可逆 Family fingerprint；装配完成后才能调用现有 `ApplyPromotionPlan`。随后再设计 Global Generation/OKF
发布事务。两者均不依赖 Reasonix 官方接口，但均需独立 Schema/事务测试。
