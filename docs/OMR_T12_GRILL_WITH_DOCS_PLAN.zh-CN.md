# OMR-T12：Grill with Docs 文档化方案质询

> **Archived（已归档）**：本计划对应的工作已完成交付；本文保留作为审计与追溯依据，不再作为开发输入。当前能力与剩余宿主依赖见 [当前可用能力矩阵](OMR_CAPABILITY_MATRIX.zh-CN.md)。


## 目标

在 T11 `omr-grill-me` 的只读质询基础上，增加一个显式启用的 `omr-grill-with-docs` Profile：把用户已确认的术语、事实和高影响决策沉淀到项目文档中。它不增强模型推理，也不复制 Reasonix 的 Session、Hook、Task 或恢复状态机。

## 与 T11 的边界

| Profile | 默认行为 | 输出 | 文件修改 |
|---|---|---|---|
| `omr-grill-me` | 手动、只读 | 问题、假设、风险、验收缺口 | 不允许 |
| `omr-grill-with-docs` | 手动、确认后 | 质询结果 + 文档变更计划 | 仅写入用户确认的文档 |

默认不路由简单任务；没有明确确认时只输出 Patch/计划，不写文件。

## 文档产物

- 项目根 `CONTEXT.md`：术语、已确认事实、稳定约束和开放问题；
- `docs/adr/NNNN-<slug>.md`：高影响、难以逆转的决策，包含状态、上下文、决策、后果、替代方案和确认依据。

未确认的假设不得进入确认约束或 ADR 的 Decision。

## 实现任务

### T12-01：Profile 与资产

- 新增 `assets/skills/omr-grill-with-docs/SKILL.md`；
- 声明 `invocation: manual`、写入前确认和允许写入范围；
- 接入嵌入资产、外部资产、安装、升级、卸载、Manifest、Hash 和 Profile list；
- 使用 clean-room 行为描述，不复制第三方原文。

### T12-02：文档变更模型

实现最小纯 Go 模型，支持术语/事实更新、ADR 草稿、dry-run、确认后原子写入、冲突检测和幂等执行。

### T12-03：安全边界

拒绝绝对路径、路径穿越、符号链接逃逸、`.reasonix/` 私有状态和项目根目录外的任何写入。不得修改 API Key、全局配置或 Reasonix 二进制。

### T12-04：离线测试

覆盖 dry-run 零写入、确认写入、ADR 编号递增、幂等、冲突保留原文、未确认假设隔离、路径安全拒绝和 T11 回归。

### T12-05：文档与版本

同步 README、CHANGELOG、Todo、差距矩阵和安装文档。版本升级必须同步发布计划，不自行创建 Tag。

## 验收门禁

运行 `gofmt -w .`、`git diff --check`、`go test ./...`、`go vet ./...`。报告必须包含 Profile JSON、dry-run/确认写入证据、文件快照、冲突与安全测试结果、未验证的宿主行为和剩余风险。

## 明确不做

不实现 Ralph Loop、Comment Checker、Tmux 面板、自动持续质询或任何 OMR 自有 Session/Hook/Task 状态机；不读取 `~/.reasonix` 或项目 `.reasonix` 私有状态。

## 给 Reasonix Agent 的执行提示词

```text
你是 oh-my-reasonix 仓库的实现 Agent。请严格执行 docs/OMR_T12_GRILL_WITH_DOCS_PLAN.zh-CN.md。

先检查 T11 和工作区，避免重复改动。T12 必须手动、显式确认、默认 dry-run；写入范围仅限项目根 CONTEXT.md 与 docs/adr/*.md。先写失败测试/Fixture，再做最小实现。拒绝绝对路径、路径穿越、符号链接逃逸、.reasonix 私有状态和未确认假设；实现幂等、冲突检测、原子写入和 Patch 预览。

不要调用网络、模型或 Reasonix 私有接口，不复制第三方原文，不修改全局环境/API Key/Reasonix 二进制，不修改 artifacts/、omr-ab-b-meta.json 或其他未跟踪文件。同步文档，运行 gofmt、git diff --check、go test ./...、go vet ./...。不要自行推送、创建 Tag 或宣称 CTO Review 通过；完成后报告变更、测试、文件快照、安全边界和剩余风险。
```
