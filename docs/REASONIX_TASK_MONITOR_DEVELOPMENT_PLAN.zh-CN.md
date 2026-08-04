# Reasonix 原生 Task Monitor 开发计划

> **Archived（已归档）**：本文记录 Reasonix 官方宿主接口的调研与计划。相关工作依赖 Reasonix 官方接口，当前标记 BLOCKED，OMR 未猜测、未伪造、未自行实现；本文保留作为审计依据。

## 目标

为 Reasonix 增加原生后台任务可观测能力，先支持 CLI/机器接口，再支持 Desktop 实时面板，最后评估 Tmux Adapter。用户应能观察后台 Agent 的状态、事件、结果和失败原因，并在后续阶段停止、恢复或重新打开任务。

## 设计原则

- UI 和 Tmux 不解析 Session 私有文件或私有 JSONL；
- 所有状态来自 Reasonix 原生 Task/Session/Event 服务；
- 先只读观测，再增加停止/恢复控制；
- 状态必须有版本、时间戳、Task ID 和 Session ID；
- 不复制第二套状态机；
- CLI、机器接口和 Desktop 使用同一份领域模型；
- 不影响现有交互式任务、Provider Prompt 或缓存。

## 任务拆分

### TM-01：Task Monitor 领域模型与只读查询（P0）

建立统一的 `TaskSnapshot`、`TaskEvent` 和状态枚举，提供项目范围只读列表、详情和事件订阅接口。覆盖 queued、running、waiting、succeeded、failed、cancelled、stale，明确 Task 与 Session 的关联和脱敏字段。

验收：Mock 测试覆盖状态转换、事件顺序、未知状态、脱敏和项目隔离；不读取私有文件，不改变现有 CLI 行为。

### TM-02：CLI/机器接口

增加稳定 JSON 接口：`reasonix task list --json`、`reasonix task status <id> --json`、`reasonix task events <id> --json` 或 JSONL 订阅。输出 schema_version、task_id、session_id、state、timestamps、summary、error_code 和脱敏事件，支持 `--dir` 与明确的 `--follow` 生命周期。

### TM-03：Desktop 只读面板

复用现有 Workspace Panel/右侧 Dock，增加后台任务列表和详情视图：状态、进度、最近事件、耗时、错误和关联 Session。先只读，不实现停止/恢复。

验收：React 组件测试、空态/加载态/错误态、事件增量刷新、窗口缩放和多任务排序。

### TM-04：任务控制操作

只读协议稳定后再增加 stop、cancel、resume、open session。所有操作带 Task ID、当前版本或事件游标，过期操作 fail closed。

### TM-05：Tmux Adapter（P2）

提供可选 Tmux 集成，将后台任务映射到 pane/window；不安装 Tmux、不把 Tmux 作为核心依赖，非 Tmux 环境保持核心功能可用。使用 Mock 测试缺失降级、pane 退出恢复、Task ID 映射和清理。

## 依赖关系

`TM-01 → TM-02 → TM-03 → TM-04`，TM-03 完成后再评估 TM-05。先执行 TM-01，不要同时修改 Desktop UI 或 Tmux。

## 统一验收门禁

运行 `gofmt -w .`、`git diff --check`、`go test ./...`、`go vet ./...`。Desktop 另加前端单测和 TypeScript 检查；Tmux 必须使用 Mock。

## 明确不做

- 不解析 `~/.reasonix`、`.reasonix` 私有状态或内部事件文件；
- 不在 OMR 中实现 Task 状态机；
- 不把 Desktop 面板作为后台任务执行器；
- 不默认启动 Tmux 或后台进程；
- 不修改 Provider Prompt、工具 Schema、API Key 或全局配置。

## TM-01 给 Reasonix Agent 的执行提示词

```text
你是 Reasonix 官方仓库的实现 Agent。请只执行 TM-01：Task Monitor 领域模型与只读查询，不要提前实现 Desktop 或 Tmux。

先检查现有 Task、Session、Event、机器接口和测试结构，复用现有状态源，不读取私有 Session 文件，不创建第二套状态机。设计并实现统一的 TaskSnapshot、TaskEvent、状态枚举和项目隔离的只读查询接口；字段必须脱敏，包含 schema/version、task_id、session_id、state、时间戳和错误摘要。未知状态必须可安全表示，事件顺序和游标语义必须明确。

先写失败测试，再做最小实现。覆盖状态转换、未知状态、事件乱序/重复、脱敏、项目隔离、空列表和错误码。保持现有 CLI、Prompt、Provider、Session 行为兼容。运行 gofmt、git diff --check、go test ./...、go vet ./...。不要修改 OMR 仓库，不要创建 Desktop/Tmux 代码，不要自行推送或宣称 CTO Review 通过。完成后报告变更、接口草案、测试结果、兼容性和剩余风险。
```
