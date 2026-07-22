# OMR-06 调查结果：Profile 资产扩展

> 日期：2026-07-22
> 评估人：Reasonix Executor
> 状态：**跳过** — 宿主工具不存在

## 调查过程

检查了以下证据：

1. `internal/install/assets.go` — `Assets` 结构体只有 8 个字段（BasePrompt、Orchestrator、Explore、Research、Debug、Planner、Frontend、ReviewBrief），**没有 git/ast/browser/visual 字段**。
2. `internal/install/paths.go` — 只有 5 个 Profile 的安装路径常量，没有 omr-git 等路径。
3. `assets/skills/` — 只有 5 个 SKILL 文件（omr-explore、omr-research、omr-debug、omr-planner、omr-frontend）。
4. `internal/reasonix/runner.go` — `Probe()` 方法只探测 `version`、`cli`、`subagent`、`profile list`、`profile.review`，**没有 git/AST/browser/visual 能力探测**。
5. 全项目 grep 搜索：`git`（仅文档和 fixture）、`ast`（无匹配）、`browser`（仅文档）、`visual`（仅文档）。
6. 差距矩阵 `docs/OMR_VS_OMO_GAP_MATRIX.zh-CN.md` — Git Master Skill、AST 工作流、Browser 自动化均列为"没有"。

## 结论

| Profile | 宿主工具是否存在 | 决定 |
|---------|----------------|------|
| `omr-git` | ❌ 没有原生 git 工具 | 跳过 |
| `omr-ast` | ❌ 没有原生 AST 工具 | 跳过 |
| `omr-browser` | ❌ 没有原生 browser 工具 | 跳过 |
| `omr-visual` | ❌ 没有原生 visual/multimodal 工具 | 跳过 |

## 恢复条件

当 Reasonix 宿主提供以下任一能力时，可以重新评估 OMR-06：

- `reasonix tool list` 输出包含 `git`、`ast-grep`、`browser`、`visual` 等工具
- `reasonix subagent --help` 列出对应的 subagent profile
- 有对应的 MCP server 注册且可通过 OMR 发现
