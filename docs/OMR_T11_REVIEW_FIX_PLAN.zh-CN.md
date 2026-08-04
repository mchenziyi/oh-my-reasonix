# OMR-T11 Review 修正计划：Grill Me 测试与文档边界

> **Archived（已归档）**：本计划对应的工作已完成交付；本文保留作为审计与追溯依据，不再作为开发输入。当前能力与剩余宿主依赖见 [当前可用能力矩阵](OMR_CAPABILITY_MATRIX.zh-CN.md)。


## 背景

T11 的 Profile、嵌入资产、安装链路和文档已经完成，现有 5 个测试通过。但这些测试主要检查 `SKILL.md` 是否声明了规则，属于 Prompt 契约测试，不是真实 Agent 行为回放。需要修正文档表述，并在不复制 Reasonix 状态机的前提下增加可验证的离线测试边界。

## 目标

让 T11 的测试结果准确、可审计：区分 Prompt 契约测试、离线回放测试和真实宿主行为；不得把字符串存在性测试描述为模型行为证明。

## 实现任务

### T11-R1：测试分类与命名

- 将现有 5 个字符串检查测试的注释、测试报告和文档表述统一为“Prompt 契约测试”；
- 保留测试内容，不为单纯改名增加不必要抽象；
- 如果新增回放测试，使用独立、纯 Go、无网络的测试数据结构。

### T11-R2：离线回放测试（推荐）

新增最小回放模型，输入为预先定义的质询轮次和用户确认状态，输出为结构化结果。至少覆盖：

1. 信息充分时停止；
2. 最多 6 轮后停止；
3. 用户停止立即结束；
4. 未确认假设不能进入 `assumptions_confirmed`；
5. 回放前后临时工作区文件快照一致。

约束：不调用模型、不访问 Reasonix 私有目录、不修改真实项目文件、不伪造 Session/Hook 状态。

### T11-R3：文档修正

同步 README、CHANGELOG、`OMR_TODO_LATEST.zh-CN.md`、T11 计划和差距矩阵：

- 明确现有 5 项为 Prompt 契约测试；
- 若 R2 完成，单独列出“离线回放测试”；
- 明确真实 Agent 行为仍需 Reasonix 宿主或人工联调验证；
- 不使用“已证明模型一定会遵守”等过度结论。

### T11-R4：发布版本检查

- 保留 `assets/manifest.yaml` 的 `1.2.0` 只有在 CHANGELOG 与发布计划一致时才可接受；
- 不创建 Tag、不推送、不修改用户未跟踪的 `artifacts/`、`omr-ab-b-meta.json`；
- 若发现版本策略不明确，报告为待 CTO 决策，不自行改版本。

## 验收门禁

```text
gofmt -w .
git diff --check
go test ./...
go vet ./...
```

报告必须包含变更文件、两类测试结果、文件快照证据、未验证的真实宿主行为、版本处理和剩余风险。

## 给 Reasonix Agent 的执行提示词

```text
你是 oh-my-reasonix 仓库的实现 Agent。请严格执行
docs/OMR_T11_REVIEW_FIX_PLAN.zh-CN.md。

先检查工作区和现有 T11 变更，不重复实现 Profile 安装链路。首要任务是修正测试分类和文档措辞；不要把 SKILL.md 字符串匹配称为模型行为证明。推荐实现最小的纯 Go 离线回放测试，覆盖停止条件、最大轮数、用户停止、未确认假设隔离和文件快照不变；不得调用网络、模型或 Reasonix 私有状态。

不复制 Reasonix Session/Hook/Task 状态机，不修改全局配置/API Key/二进制，不修改 artifacts/、omr-ab-b-meta.json 或其他未跟踪用户文件。同步 README、CHANGELOG、Todo、T11 计划和差距矩阵，准确区分契约测试、离线回放和真实宿主验证。运行 gofmt、git diff --check、go test ./...、go vet ./...。不自行推送、不创建 Tag、不宣称 CTO Review 通过；完成后只报告变更、测试结果、证据和剩余风险。
```
