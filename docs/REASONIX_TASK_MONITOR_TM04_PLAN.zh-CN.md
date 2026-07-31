# Reasonix TM-04：任务控制操作开发计划

## 目标

在 TM-01～TM-03 的只读 Task Monitor 基础上，为 Reasonix 增加安全、可审计的任务控制能力：停止、取消、恢复，以及打开关联 Session。控制操作必须复用原生 Task/Session 状态，不复制第二套状态机。

## 范围

### 本阶段实现

- `stop`：请求停止正在运行或等待中的任务。
- `cancel`：取消尚未完成的任务；已终态任务返回幂等结果。
- `resume`：从可恢复的失败/中断任务创建或重新激活执行。
- `open session`：返回关联 Session 标识和宿主可打开的引用，不直接操控 UI。
- CLI JSON 接口、Desktop 控件、并发冲突保护、审计事件和 Mock 测试。

### 明确不做

- 不实现 Tmux、后台守护进程或新的任务执行器。
- 不读取私有状态文件，不修改 OMR 仓库。
- 不绕过 Reasonix 权限、确认、Provider 或 API Key 机制。
- 不允许客户端直接写 Task 状态或伪造事件。

## 核心不变量

1. 所有写操作必须携带 `task_id` 与 `expected_version`（或等价事件游标）。
2. 版本过期时 fail closed：不执行操作，返回稳定冲突错误。
3. 终态任务（succeeded、failed、cancelled、stale）不可 stop/cancel；resume 仅允许宿主明确标记可恢复的状态。
4. 每次成功控制操作产生带操作者、时间、旧状态、新状态和原因的审计事件。
5. 重复提交同一幂等键不得重复执行或重复创建任务。
6. 项目作用域必须校验，不能跨 `--dir` 访问任务。

## 建议接口

### CLI

```text
reasonix task stop <id> --expected-version N [--reason TEXT] [--idempotency-key KEY] --json
reasonix task cancel <id> --expected-version N [--reason TEXT] [--idempotency-key KEY] --json
reasonix task resume <id> --expected-version N [--idempotency-key KEY] --json
reasonix task open-session <id> --json
```

所有命令支持 `--dir PATH`。JSON 至少包含：`schema_version`、`command`、`task_id`、`session_id`、`state`、`version`、`accepted`、`idempotent`、`error.code` 和 `error.message`。失败必须使用非零退出码；幂等成功使用零退出码并标记 `idempotent: true`。

### 服务层

提供原子方法（命名可按现有代码风格调整）：

```go
StopTask(ctx context.Context, projectDir, taskID string, expectedVersion int64, reason, idemKey string) (ControlResult, error)
CancelTask(...)
ResumeTask(...)
OpenTaskSession(...)
```

服务层负责作用域、状态转换、版本比较、幂等记录和事件写入；CLI 与 Desktop 只做参数适配。

## 状态与错误语义

稳定错误码至少包括：`task_not_found`、`task_scope_mismatch`、`task_version_conflict`、`task_invalid_transition`、`task_not_resumable`、`task_already_terminal`、`task_operation_in_progress`、`task_permission_denied`、`task_idempotency_conflict`。

错误不得泄露路径、凭据、Prompt 或模型响应。人类输出可读，机器输出保持 schema 稳定。

## Desktop 要求

- 在任务详情页增加 Stop、Cancel、Resume、Open Session 控件。
- 根据当前状态禁用不适用操作；发送前显示确认，危险操作明确说明影响。
- 提交时使用详情页最新 `version`；收到冲突后刷新任务并提示用户重试。
- 操作完成后立即刷新详情和事件流，不自行推断状态。
- 所有按钮覆盖 loading、成功、幂等成功、冲突和错误态；不阻塞其他任务。

## 实现分步

1. 为领域模型补充可执行状态转换、版本和控制结果类型。
2. 在原生 Task 服务实现四个原子操作及幂等存储。
3. 增加 CLI 子命令与 JSON 输出，保留现有只读命令兼容性。
4. 增加 Desktop bridge 与控件，接入现有 TaskMonitorPanel。
5. 增加审计事件和事件流刷新，验证重复提交与失败路径。
6. 补齐文档、schema、帮助文本和变更记录。

## 测试矩阵

- 每种合法状态转换至少 1 个测试。
- 终态、未知状态、版本过期、跨项目、无权限、非法 ID 全部拒绝且零副作用。
- 并发 stop/cancel/resume：仅一个获胜，其余返回稳定冲突或幂等结果。
- 相同幂等键重复请求不重复写事件；不同参数复用同键返回冲突。
- CLI JSON/人类输出、退出码、`--dir` 转发和敏感信息脱敏。
- Desktop 组件覆盖禁用条件、确认框、loading、错误、冲突刷新和成功刷新。
- 重启后幂等记录和审计事件仍可读取（若存储层持久化）。

## 交付门禁

```bash
gofmt -w .
git diff --check
go test ./...
go vet ./...
cd desktop/frontend && npm run typecheck && npm test -- --run
```

完成后只提交 TM-04 相关变更，报告 commit、接口示例、测试结果和未解决限制；不要进入 TM-05。

## 给 Reasonix Agent 的执行提示词

```text
请在 Reasonix 仓库实现 docs/REASONIX_TASK_MONITOR_TM04_PLAN.zh-CN.md。

你是实现者，不要修改 OMR 仓库，也不要实现 TM-05/Tmux。先检查 TM-01~TM-03 的实际接口，再按文档做最小增量实现。所有控制操作必须复用原生 Task/Session 状态机，携带 expected_version，过期必须 fail closed；补齐幂等、项目作用域、审计事件、稳定错误码、CLI JSON 和 Desktop 控件。

先写失败测试，再实现代码。完成后运行 gofmt、git diff --check、go test ./...、go vet ./...、desktop/frontend 的 npm run typecheck 和 npm test -- --run。若环境缺少依赖，只报告阻塞，不伪造通过。只提交 TM-04 相关文件，输出 commit、测试结果和剩余风险；不要进入 TM-05。
```
