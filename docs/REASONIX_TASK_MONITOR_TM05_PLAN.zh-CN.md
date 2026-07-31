# Reasonix TM-05：可选 Tmux Adapter 开发计划

## 目标

为 Reasonix Task Monitor 提供可选的 Tmux 映射：将后台任务绑定到 pane/window，支持查看、跳转和清理。Tmux 只是展示与操作适配器，不成为任务执行器，也不改变 Task/Session 原生状态机。

## 范围

实现：

- 检测当前环境是否有可用 Tmux；
- Task ID → session/window/pane 映射；
- 创建、查询、跳转和清理映射；
- Tmux 缺失、pane 被关闭、session 不存在时优雅降级；
- CLI 或 Desktop 的最小入口（按现有宿主界面选择，不新增第二套 UI）。

不实现：

- 不安装 Tmux，不自动启动后台守护进程；
- 不把 Tmux 作为核心依赖；
- 不解析 Tmux 输出推断 Task 状态；
- 不修改 OMR 仓库或 OMR FileStore；
- 不复制 Task/Session 状态机，状态始终来自原生 Task 服务。

## 核心不变量

1. Tmux 不可用时，所有核心 Task 功能仍正常工作。
2. 映射记录必须包含 `task_id`、项目作用域、Tmux session/window/pane、创建时间和 schema 版本。
3. 所有 shell 参数必须使用参数数组或严格转义，禁止拼接未验证的 Task ID/路径。
4. pane/window 被用户关闭后，映射标记 stale，不删除 Task，不伪造失败状态。
5. 同一 Task 在同一项目作用域内重复 attach 必须幂等。
6. 清理映射不得删除用户创建的非 Adapter pane；只删除明确由 Adapter 创建且仍归属该 Task 的资源。

## 建议接口

```text
reasonix task tmux attach <id> [--dir PATH] [--session NAME] --json
reasonix task tmux status <id> [--dir PATH] --json
reasonix task tmux open <id> [--dir PATH]
reasonix task tmux detach <id> [--dir PATH] --json
```

机器输出至少包含：`schema_version`、`task_id`、`project_dir`（脱敏或规范化）、`available`、`state`、`session`、`window`、`pane`、`stale`、`error.code`。

## Adapter 分层

1. `TmuxRunner`：只负责执行受控的 `tmux` 参数数组，注入接口便于 Mock。
2. `MappingStore`：持久化映射，使用项目作用域和安全 Task ID，原子写入。
3. `Adapter`：实现 attach/status/open/detach，处理幂等和 stale 降级。
4. CLI/Desktop：只传递 Task ID 和项目目录，不自行解析 Tmux 输出。

## 安全要求

- Tmux 二进制路径来自受控 PATH 或配置，禁止用户输入直接作为可执行文件路径。
- session/window/pane 名称使用固定前缀和安全字符集；拒绝换行、控制字符和路径分隔符。
- 项目目录必须经过现有作用域校验；映射文件不能通过符号链接逃逸。
- 错误信息不得包含 API Key、Prompt、模型输出或完整环境变量。

## 测试矩阵

- 无 Tmux：attach/status/open/detach 返回稳定 `tmux_unavailable`，Task 查询仍通过。
- Mock Tmux：创建成功、重复 attach 幂等、不同 Task 映射隔离。
- pane 被关闭、session 不存在、命令非零退出：返回 stale/降级结果，不误改 Task 状态。
- Task ID、项目路径和名称注入测试；路径穿越、控制字符全部拒绝。
- detach 只清理 Adapter 自己创建的资源。
- 并发 attach/detach 不产生重复映射或数据竞争。
- macOS/Linux CI 使用 Mock，不要求真实 Tmux；真实 Tmux 仅作为可选本地 smoke test。

## 交付门禁

```bash
gofmt -w .
git diff --check
go test ./...
go test -race ./...
go vet ./...
```

完成后只提交 TM-05 相关变更，报告 Tmux 可用与不可用两种结果、Mock 覆盖范围、降级语义和安全审查；不要扩展新的 Task 功能。

## 给 Reasonix Agent 的执行提示词

```text
请在 Reasonix 仓库实现 docs/REASONIX_TASK_MONITOR_TM05_PLAN.zh-CN.md。

这是 Task Monitor 主线最后一个排期阶段。Tmux 只能作为可选 Adapter，不安装 Tmux、不启动守护进程、不复制 Task/Session 状态机。先检查 TM-01~TM-04 的实际接口，优先复用原生 Task 服务和现有 CLI/桌面边界。

先写失败测试，再实现最小代码。Tmux 调用必须可注入 Mock，禁止 shell 字符串拼接；覆盖 Tmux 缺失、pane/session 消失、幂等 attach、stale 降级、作用域和路径穿越。无 Tmux 环境下核心功能必须保持可用。

完成后运行 gofmt、git diff --check、go test ./...、go test -race ./...、go vet ./...。只提交 TM-05 相关文件，输出 commit、测试结果、Mock 覆盖范围和剩余风险。TM-05 完成后不要创建 TM-06。
```
