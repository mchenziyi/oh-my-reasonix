# oh-my-reasonix 最新开发 Todo

> 版本：2026-07-23  
> 用途：交给 Reasonix Agent，在 oh-my-reasonix 仓库内继续开发。  
> 原则：只实现 OMR 能独立负责的能力；Reasonix 已原生提供或尚未提供公开接口的能力，不在 OMR 中复制。

## 1. 与 oh-my-opencode 的对比结论

oh-my-opencode 当前公开文档将以下能力作为产品核心：专用 Agent 团队、后台并行 Agent、LSP/AST 工具、Todo Continuation、Comment Checker、Claude Code 兼容层、Context 注入、内置 MCP、Session 工具、Ralph Loop、自动恢复和 Tmux 交互终端。

OMR 已完成 Prompt/Profile 发行、安装升级、质量基准和 Reasonix 原生能力复用。差距分为三类：

| 类别 | 处理方式 |
|---|---|
| OMR 可以独立完成 | 进入本 Todo，按 P0/P1/P2 开发 |
| Reasonix 已原生提供 | OMR 只提供 Prompt/策略资产，不重复实现 |
| Reasonix 尚无稳定公开接口 | 标记 BLOCKED，不写文件系统回退，不伪造接口 |

对比依据：

- OMO Features：<https://github.com/opensoft/oh-my-opencode>
- OMO 配置与 JSONC：<https://github.com/opensoft/oh-my-opencode/blob/dev/docs/configurations.md>
- OMO Features Reference：<https://github.com/Wangmerlyn/oh-my-opencode/blob/dev/docs/reference/features.md>

## 2. 已完成，不要重复开发

- 项目级 init/upgrade/uninstall、dry-run、备份、回滚、Manifest 和 Hash；
- Prompt Composer、Orchestrator Prompt、Reasonix Base Prompt；
- `omr-explore`、`omr-research`、`omr-debug`、`omr-planner`、`omr-frontend`；
- Profile 模型、Prompt 文件、read-only、disabled 和 category routing 配置；
- TOML、JSONC、TOML → JSONC 迁移；
- Doctor、Profile list、config validate、config schema；
- 质量 Fixture、离线 replay、Runtime benchmark、成本门禁和报告 Schema；
- Cache Guard；
- Claude rules/skills/agents/mcp/hooks 导入基础链路；
- `omr session resume`、`omr session export`；
- OMR-FIX-01～11 及其自动化测试。

## 3. P0：当前最重要的 OMR 工作

### OMR-T01：真实质量基准扩展

增加脱敏的多文件任务 Fixture，覆盖 Explore → Plan → Implement → Test → Review → Complete 全流程；固定允许/禁止路径、隐藏测试、回归测试和预期事件；增加 Native/OMR 配对回放和失败保留规则；报告区分基础设施失败、任务失败、判定失败和模型失败。

验收：新增 Fixture 可离线 replay，`go test ./...` 和 `omr benchmark quality --replay` 通过，失败运行不被静默丢弃；不得无配对证据宣称 OMR 优于 Native。

### OMR-T02：Prompt/规则注入可验证性

明确根目录和子目录 `AGENTS.md` 的读取顺序、目标文件路径向上收集规则的优先级、README 和 `.reasonix/rules` 条件规则协议；在 Orchestrator Prompt 中加入来源、路径、冲突和有效性要求；增加 Prompt fixture 验证顺序和冲突处理。不得把动态时间、绝对路径或 Hash 写入模型 Prompt。

## 4. P1：可独立实现的能力

### OMR-T03：Claude 兼容层收尾

增加 `.claude/commands` 只读导入；为 Agent/Skill 增加 frontmatter Schema 校验；MCP 导入增加兼容性报告；Hook 报告列出可转换内容和无法保留的运行时语义；保持 dry-run、冲突、全量回滚和敏感信息保护。

### OMR-T04：Profile 与 Category 体验

为每个 Profile 补齐用途、输入、输出、只读边界和失败处理；增加 Profile/Category Schema 与示例；检测未安装、已禁用、重复覆盖和循环路由；增加模型覆盖校验和 Doctor 诊断。Visual Profile 只有宿主明确提供视觉能力时才加入。

### OMR-T05：质量与成本可观测性

统一 Runtime、Replay、Native/OMR 对照报告字段；增加重试次数、停滞原因、Review 阻断数、Token、成本和验证证据；支持稳定 JSON 快照；明确合成 run ID，不得称为 Reasonix Session ID；增加 Schema 版本迁移测试。

### OMR-T06：安装与升级体验

增加最低 Reasonix 版本和兼容矩阵；增加 `omr version --json` 与资产版本报告；升级 dry-run 展示 Prompt/配置变化和备份位置；增加仅提示的自动更新机制，不自动修改全局环境；同步 README、INSTALL、Release 和卸载文档。

## 5. P2：主体稳定后再做

### OMR-T07：工具生态 Profile

按宿主能力评估 LSP、AST/AST-Grep、Git Master、Browser/Playwright 和 Skill 内嵌 MCP。宿主没有对应能力时只记录调查结果，不嵌入不可执行资产。

### OMR-T08：开发体验

评估显式增强模式、Ralph Loop、Comment Checker、用户级配置和交互式通知。只能作为 Prompt/配置层能力，不复制 Reasonix 后台任务或状态机。

### OMR-T09：规则和配置兼容性

完善配置 Schema 自动生成、编辑器提示、JSONC 文档、`.agents/skills` 兼容、用户级/项目级优先级以及跨平台路径和权限测试。

## 6. BLOCKED：当前不在 OMR Todo 中实现

以下能力依赖 Reasonix 稳定公开接口：Session list/status/show/search、Hook list/status 和运行时拦截、后台 Task 查询、Session recovery、结构化事件流、Todo/Hook/Task/Session 状态机、后台 Agent 结果汇聚、Tmux/桌面端实时状态面板。

禁止读取 `~/.reasonix/projects`，禁止解析 goal-state/events/lock 私有文件，禁止在宿主 CLI 不支持时返回空列表伪造成功，禁止以 OMR 合成 ID 冒充 Reasonix Session ID。

## 7. 推荐开发顺序

1. OMR-T01：真实质量基准；
2. OMR-T02：Prompt/规则注入可验证性；
3. OMR-T03：Claude 兼容层收尾；
4. OMR-T04：Profile/Category 体验；
5. OMR-T05：质量与成本报告；
6. OMR-T06：安装升级体验；
7. OMR-T07～T09：P2 扩展；
8. BLOCKED 项目等待 Reasonix 官方接口后另开适配任务。

每个任务必须先写回归测试或 Fixture，做最小代码修改，运行 `gofmt`、`git diff --check`、`go test ./...`、`go vet ./...`，更新本文件和差距矩阵，并保留未跟踪的 `omr`、`.reasonix/`。只有真实客户端行为无法通过自动化判断时，才请求用户协助。
