# OMR Mnemosyne WEB-03：本地管理页面计划

状态：🟡 管理动作协议、CLI 执行器、loopback API 与管理页面已实现；隔离临时项目的真实进程 smoke 已通过，待人工浏览器/Reasonix Desktop 联调

## 目标

在 WEB-01/02 的静态只读视图之上，提供本地管理页面的最小可用交互：查看记忆详情、关联关系、审计状态，并通过明确的人工确认调用现有 CLI 治理命令。页面本身不成为新的事实源。

## 设计边界

- 页面只读取 OMR 生成的 JSON/HTML 数据；不直接打开 FactStore 文件，不接触 Reasonix 私有状态。
- 所有写操作必须转译为现有 CLI 的显式命令（freeze/unfreeze/pin/unpin/rollback 等），并要求二次确认；页面不自行实现事务。
- 默认绑定单一 project workspace；切换 workspace 时清空缓存并重新生成视图，禁止跨项目混读。
- 不监听公网、不上传数据、不引入向量数据库、不输出 Prompt、命令正文或凭据。
- 任何写操作失败时只显示稳定错误，页面不做乐观状态更新；刷新后以 FactStore 派生结果为准。

## 已实现的协议切片

`internal/memory.WebManagementAction` 是页面提交给本地执行器的意图对象，不是 Fact。它固定包含 action id、Scope、完整 MemoryRef、受控 operation、人工 reason、basis refs 和显式 requested_at；严格校验后可稳定编码和计算 Hash。`/manager` 页面提供 pin/unpin/freeze/archive，以及要求用户填写非空 `basis_refs` JSON 数组的 unfreeze；所有动作都先经过二次确认，仍由同一严格 API 校验。`omr memory web action validate --input <file>` 只做严格解析、校验和 Hash 回执；`omr memory web action apply --input <file> --project-dir <dir> --confirm` 才会在重新验证当前 MemoryRef 后委托既有 Governance API。没有 `--confirm` 时零写入，目标 Hash/Scope 过期时 fail closed，重复动作由 FactStore 幂等处理。`omr memory web serve --project-dir <dir> --now <RFC3339>` 只绑定 loopback，并提供 `/`、`/audit`、`/manager`、`/action/validate`、`/action/apply`。

## 分阶段实现

1. 静态资源与 JSON 数据协议（先做确定性 fixture 和浏览器无关测试）。
2. ✅ 本地受限启动器（`memory web serve`，随机本地端口、仅 loopback、单 workspace）。
3. ✅ 只读详情/关系/审计/管理页面（`/manager`），管理页展示同一固定时间点重建的 Lifecycle、Health、Usage、Pinned/Frozen/Archived 治理标记、关系与审计入口。
4. 人工确认后的 CLI 治理操作和回滚结果刷新。

## 验收

- 无 Reasonix 客户端也能打开只读页面和管理页面；关闭进程后无后台任务。
- 跨 workspace 数据隔离、XSS/路径/symlink 防护、写操作幂等和失败回读均有测试。
- 不改变既有 Memory Fact Schema；所有派生视图可从规范事实重建。

## 临时项目启动

```bash
omr memory init --project-dir /tmp/omr-web-03 --scope project
omr memory web serve --project-dir /tmp/omr-web-03 --now 2026-08-14T00:00:00Z
```

`memory init` 只创建受 Store 安全策略保护的空目录；它不伪造 Memory、Usage、Outcome 或 Generation Fact。

## 进程级联调记录

2026-08-14 在隔离临时项目中构建 `omr` 二进制并启动 `memory web serve --listen 127.0.0.1:0`：`/manager` 与 `/audit` 均返回 200；管理页包含 Lifecycle、Health、Usage、Pinned/Frozen/Archived、Relations、Unfreeze 与审计入口；响应包含 CSP、`Cache-Control: no-store` 与 `X-Content-Type-Options: nosniff`；空 Store 不产生任何 Fact。该 smoke 不替代人工浏览器点击和 Reasonix Desktop 联调。

2026-08-14 进程级治理 smoke：在同一隔离项目写入测试 MemoryRevision，通过真实 loopback `/action/apply` 提交 Freeze（`X-OMR-Confirm: yes`）返回 HTTP 200，随后重新读取 `/manager` 显示治理标记为 `true`；重复相同 action 返回 `status: noop`。该结果证明 API、治理事件持久化、派生状态刷新和幂等链路可用；仍不替代 Reasonix Desktop 中的人工点击验收。
