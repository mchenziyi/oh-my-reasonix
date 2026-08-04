# OMR 当前可用能力矩阵

> 版本：2026-08-04
> 当前版本：OMR v2.0.9（LP-01～LP-06 全部交付）
> 用途：单一事实源——区分“OMR CLI 已实现”“Reasonix Desktop 可用范围”“需要 Reasonix 官方接口的范围”。

## 图例

| 标记 | 含义 |
|---|---|
| ✅ 已交付 | 代码与回归测试已合入，`go test ./...` 通过 |
| ⚠️ 需要宿主接口 | 能力依赖 Reasonix 官方接口，OMR 不伪造、不自行实现 |
| 🚫 不实现 | 明确不属于 OMR 范围，由 Reasonix 原生提供 |

## CLI 已实现能力（v2.0.9）

| 命令 | 状态 | 说明 |
|---|---|---|
| `omr init` / `upgrade` / `uninstall` | ✅ | 安装/升级/卸载，dry-run、备份、回滚 |
| `omr doctor` | ✅ | 项目健康诊断，含 Reasonix 能力探测 |
| `omr config` | ✅ | 配置加载/迁移/校验/JSON Schema |
| `omr profile list` | ✅ | Profile 发行与状态查询 |
| `omr session` | ⚠️ | 只读查询 Reasonix Session（经官方机器接口） |
| `omr task` | ⚠️ | 只读查询 Reasonix Task（经官方机器接口） |
| `omr hook doctor / comment-check` | ✅ | Hook 诊断与 Comment Checker 管理（enable/status/disable/guard） |
| `omr hook comment-check logs` | ✅ | LP-04：Hook 审计日志查询与清除 |
| `omr benchmark cache` | ✅ | 缓存基准 |
| `omr benchmark quality` | ✅ | 质量 Fixture 基准（replay/runtime/paired） |
| `omr benchmark profile` | ✅ | LP-03：Profile/Prompt 过程指标基准（离线 Fixture） |
| `omr comment-check` | ✅ | 离线 Comment Checker CLI |
| `omr run` | ✅ | 运行 Reasonix 并落盘事件 |
| `omr evolve status / proposals / report` | ✅ | Evolution 状态与观测报告（LP-02 增强） |
| `omr evolve history <id>` | ✅ | LP-02：Proposal 详细观察历史 |
| `omr evolve doctor` | ✅ | LP-01：Evolution 集合只读统计 |
| `omr evolve prune` | ✅ | LP-01：终态证据清理（dry-run 默认安全） |
| `omr evolve repair` | ✅ | LP-01：孤儿/重复/无效索引修复 |
| `omr evolve approve / reject / rollback` | ✅ | 提案生命周期（门禁保护） |
| `omr evolve export` | ✅ | 经验包导出（LP-05 支持 `--sign --key`） |
| `omr evolve import` | ✅ | 经验包导入（LP-05 支持 `--require-signature --trusted-key`） |
| `omr version --json` | ✅ | 版本与兼容性报告 |

## 自动进化本地闭环（已交付）

| 能力 | 状态 | 说明 |
|---|---|---|
| Episode 采集与 Pattern 检测 | ✅ | 重复失败自动生成提案 |
| Promotion Gate（证据/质量/安全/Hash） | ✅ | 批准前强制校验 |
| 观察期与自动回滚 | ✅ | 连续两次失败自动回滚并重建 Prompt/Manifest |
| 经验包导入导出 | ✅ | 显式导入，保持 pending，不自动批准 |
| 数据保留与修复（prune/repair） | ✅ | LP-01：快照、幂等、fail closed |
| 观测报告 | ✅ | LP-02：按 Proposal/TaskClass 聚合，不输出模型质量结论 |
| Hook 审计 | ✅ | LP-04：脱敏日志、淘汰、fail closed |
| 经验包签名 | ✅ | LP-05：Ed25519 本地签名与受信公钥导入 |

## Reasonix Desktop 可用范围

| 能力 | 状态 | 说明 |
|---|---|---|
| 桌面端 Hook 执行（PreToolUse） | ✅ | Comment Checker Hook 经 Reasonix v1.18 桌面端阻断/放行/禁用验证 |
| 桌面端只读查询 | ⚠️ | Session/Task/Hook/Recovery 查询依赖 Reasonix 机器接口（INT-06 已验证） |
| Tmux/桌面实时面板 | 🚫 | Reasonix 官方适配事项，OMR 不复制 UI/后台状态机 |
| Subagent 父子任务树与 Desktop 映射 | ⚠️ | 等待 Reasonix 提供稳定的父子关联事件与机器接口 |

## 需要 Reasonix 官方接口的范围（未伪造、未实现）

| 能力 | 状态 | 说明 |
|---|---|---|
| Subagent 父子任务映射 | ⚠️ BLOCKED | 依赖宿主接口，未猜测、未伪造；见 REASONIX_TASK_MONITOR 计划 |
| 实时桌面事件订阅 | ⚠️ BLOCKED | 依赖宿主接口 |
| 后台并行 Agent 编排 | 🚫 | Reasonix 原生能力 |
| Todo/权限状态机 | 🚫 | Reasonix 原生能力 |

## 非质量证明声明

- Evolution 观测报告、Profile 基准只证明治理/过程指标，不宣称模型质量提升。
- “进程指标 ≠ 模型质量证明”为 `omr benchmark profile` 报告的固定声明。
