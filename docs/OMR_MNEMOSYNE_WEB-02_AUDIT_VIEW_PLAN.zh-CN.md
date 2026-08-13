# OMR Mnemosyne WEB-02：只读治理与审计视图计划

状态：✅ 已实现（只读治理/审计 MVP）

## 目标

在 WEB-01 静态记忆表的基础上，生成一个只读审计视图，帮助人工查看派生 Lifecycle/Health、Usage 统计、Governance Event 和可用 Snapshot。它只展示规范 Fact 与可重建派生状态，不提供浏览器内写操作。

## MVP 边界

- 必须要求显式 `--now`，所有派生状态由 `DeriveState` 计算。
- 读取范围固定为一个 project 或 global Scope；不混合两个 Store。
- Governance Event、Migration Snapshot、Generation/CURRENT 只通过已验证 Store API 读取。
- 输出 HTML 和 JSON 元数据两种形式；文本 HTML 转义，错误固定脱敏。
- 不自动批准、冻结、回滚或写入任何 Fact；人工操作继续使用已有 CLI 命令。
- Snapshot/rollback 只展示身份、Hash、时间和状态，不输出 Prompt、命令、绝对路径或凭据。

## 验收

- 相同 Fact 集合、Scope 和 `--now` 产生稳定输出；零值时间拒绝。
- Store 损坏、越界引用、未来事实和 Scope 不匹配 fail closed。
- 导出前后 Store 文件集合、CURRENT 和 Snapshot 字节完全不变。
- 计划通过后再实现 CLI `memory web audit`；WEB-03 才考虑真正的本地管理页面和交互操作。
