# OMR-T11：Grill Me 方案质询 Skill 开发计划

> **Archived（已归档）**：本计划对应的工作已完成交付；本文保留作为审计与追溯依据，不再作为开发输入。当前能力与剩余宿主依赖见 [当前可用能力矩阵](OMR_CAPABILITY_MATRIX.zh-CN.md)。


## 交付目标

在 OMR 中增加一个可选、只读、项目级的 `omr-grill-me` Profile，用于复杂开发任务开始前发现目标歧义、未经确认的假设、边界、失败路径和验收缺口。它只输出质询结果，不修改文件，不接管 Reasonix Session、Hook、Task 或恢复状态机。

## 实现任务

1. 新增 `assets/skills/omr-grill-me/SKILL.md`，包含 YAML frontmatter、只读工具声明、最多 6 个问题、停止条件和结构化 YAML 输出。
2. 接入 `assets/embed.go`、`internal/install/assets.go`、`internal/install/paths.go` 和安装 Profile 清单，使嵌入资产与外部资产目录行为一致。
3. 增加 Profile 元数据：用途、输入、输出、只读边界、失败处理；默认不强制路由简单任务。
4. 增加离线 Fixture/单测，覆盖：信息充分停止、达到轮数停止、用户停止、未确认假设不得进入最终计划、无文件修改。
5. 更新 README、`OMR_TODO_LATEST.zh-CN.md`、差距矩阵和资产 Manifest。

## 验收门禁

- `gofmt -w .`、`git diff --check`、`go test ./...`、`go vet ./...` 全部通过；
- `omr profile list --json` 能看到 `omr-grill-me`，且 `read_only=true`、状态和来源正确；
- 安装、升级、卸载、Hash 漂移和外部资产目录测试通过；
- 默认配置不强制启用；Profile 不调用写工具、不读取 Reasonix 私有目录；
- 不复制第三方原文，确认许可证和来源后再引用概念；
- 提交前输出变更文件、测试结果和未解决风险，等待 CTO Review，不自行宣称完成。

## 明确不做

- 不实现 Ralph Loop、Comment Checker 运行时拦截、Tmux/桌面面板；
- 不新增 OMR Session/Hook/Task 状态机；
- 不修改全局配置、API Key 或 Reasonix 二进制；
- 不将用户未确认的假设写入最终开发计划。

## 给 Reasonix Agent 的执行提示词

```text
你是 oh-my-reasonix 仓库的实现 Agent。请严格执行
docs/OMR_T11_GRILL_ME_PLAN.zh-CN.md。

工作规则：
1. 先检查工作区和现有 Profile/安装/Manifest/测试结构；不要重复已完成任务。
2. 先写失败测试或离线 Fixture，再做最小实现；只修改 OMR 仓库文件。
3. 新 Profile 必须默认可选、read-only、无写工具，不读取 Reasonix 私有状态，不复制第三方原文。
4. 保持安装、升级、卸载、Hash、外部资产目录和 JSON 输出兼容。
5. 每个阶段完成后运行 gofmt、git diff --check、go test ./...、go vet ./...。
6. 更新 README、Todo、差距矩阵和 Manifest；不要修改 artifacts/、omr-ab-b-meta.json 或其他未跟踪用户文件。
7. 完成后停止并报告：变更文件、测试命令及结果、Profile 输出、风险和未完成项。不要自行推送或宣称通过 CTO Review。
```
