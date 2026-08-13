# OMR Mnemosyne WEB-03：本地管理页面计划

状态：🟡 管理动作协议已实现，页面与执行器待实现

## 目标

在 WEB-01/02 的静态只读视图之上，提供本地管理页面的最小可用交互：查看记忆详情、关联关系、审计状态，并通过明确的人工确认调用现有 CLI 治理命令。页面本身不成为新的事实源。

## 设计边界

- 页面只读取 OMR 生成的 JSON/HTML 数据；不直接打开 FactStore 文件，不接触 Reasonix 私有状态。
- 所有写操作必须转译为现有 CLI 的显式命令（freeze/unfreeze/pin/unpin/rollback 等），并要求二次确认；页面不自行实现事务。
- 默认绑定单一 project workspace；切换 workspace 时清空缓存并重新生成视图，禁止跨项目混读。
- 不监听公网、不上传数据、不引入向量数据库、不输出 Prompt、命令正文或凭据。
- 任何写操作失败时只显示稳定错误，页面不做乐观状态更新；刷新后以 FactStore 派生结果为准。

## 已实现的协议切片

`internal/memory.WebManagementAction` 是页面提交给本地执行器的意图对象，不是 Fact。它固定包含 action id、Scope、完整 MemoryRef、受控 operation、人工 reason、basis refs 和显式 requested_at；严格校验后可稳定编码和计算 Hash。`omr memory web action validate --input <file>` 只做严格解析、校验和 Hash 回执，不执行动作。该对象不会直接调用 Store，也不会改变 Lifecycle。执行器必须再次读取当前 Fact、校验 Scope/Hash 和二次确认，再调用既有治理 API。

## 分阶段实现

1. 静态资源与 JSON 数据协议（先做确定性 fixture 和浏览器无关测试）。
2. 本地受限启动器（随机本地端口、仅 loopback、workspace allowlist）。
3. 只读详情/关系/审计页面。
4. 人工确认后的 CLI 治理操作和回滚结果刷新。

## 验收

- 无 Reasonix 客户端也能打开只读页面；关闭进程后无后台任务。
- 跨 workspace 数据隔离、XSS/路径/symlink 防护、写操作幂等和失败回读均有测试。
- 不改变既有 Memory Fact Schema；所有派生视图可从规范事实重建。
