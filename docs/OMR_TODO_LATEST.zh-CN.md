# oh-my-reasonix 最新开发 Todo

> 版本：2026-08-04
> 目标版本：OMR v2.0.0
> 用途：交给 Reasonix Agent，在 oh-my-reasonix 仓库内继续开发。
> 原则：只实现 OMR 能独立负责的能力；Reasonix 已原生提供或尚未提供公开接口的能力，不在 OMR 中复制。

## 当前状态（最新）

OMR-T01～T10、T11（Grill Me）、T12（Grill with Docs）、T13（Comment Checker 离线 CLI）均已完成。INT-01～INT-05 自动化联调和 INT-06 真实客户端验证也已完成。当前不再重复开发这些阶段。

下一阶段优先级：

1. **v2.0.0 自动自进化**：按 EV00～EV10 实现 OMR 驱动的自动采集、模式识别、提案、配对回放、审批、生效和回滚。完整冻结方案见 [`OMR_V2_AUTONOMOUS_EVOLUTION_PLAN.zh-CN.md`](OMR_V2_AUTONOMOUS_EVOLUTION_PLAN.zh-CN.md)。
2. ✅ **T14：Comment Checker 运行时 Hook**：已完成。默认关闭、显式启用、只管理 OMR 自己的 Hook、失败可诊断、修改可回滚；已通过自动化测试、CLI Smoke 和 Reasonix v1.18 桌面端阻断/放行/禁用验证。
3. Tmux/桌面实时面板：记录为 Reasonix 官方适配事项，不在 OMR 内复制 UI 或后台状态机；
4. **Subagent → Task Monitor 父子任务可观测性**：等待 Reasonix 提供父子关联字段及稳定的 Desktop 映射接口。

## v2.0.0：OMR 自动自进化（进行中）

OMR 不实现第二套 Agent Runtime。Reasonix 继续负责理解任务、推理、工具、Session、Task 和 Subagent；OMR 作为项目级外置策略大脑，通过进化自己的 Prompt、Profile、编排规则、Review 规则、路由和质量 Fixture，使装载 OMR 的 Reasonix 在项目中持续改善。

开发顺序固定为 EV00 → EV10。第一版默认采用 L2：自动采集、自动分析、自动生成提案、自动配对回放，人工批准后生效；异常自动回滚。未经批准不得修改生效策略，不自动修改 Reasonix、OMR Go 源码、权限、API Key、全局配置或安全 Hook。

首个 MVP 只实现“重复 Review/测试失败 → Pattern → Proposal → 配对验证 → 批准 → Overlay → 观察/回滚”这一条完整闭环。PR #6998 可增强 Task 可观测性，但不是 MVP 硬前置。

当前实现进度：EV-MVP-00～08 自动化链路已完成（Schema、Evolution 存储、`omr run` 自动 Episode 采集、三次同类失败触发、严格 Proposal、离线验证、审批/拒绝/回滚、Overlay、Manifest/Doctor 漂移检查）；EV-MVP-10 尚未开始，必须由用户在临时项目进行真实客户端联调。

常用命令：

```bash
omr evolve status --project-dir . --json
omr evolve proposals --project-dir . --json
omr evolve approve <proposal-id> --project-dir .
omr evolve reject <proposal-id> --project-dir .
omr evolve rollback <proposal-id> --project-dir .
omr evolve doctor --project-dir .
```

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
- `omr-explore`、`omr-research`、`omr-debug`、`omr-planner`、`omr-frontend`、`omr-grill-me`、`omr-grill-with-docs`；
- Profile 模型、Prompt 文件、read-only、disabled 和 category routing 配置；
- TOML、JSONC、TOML → JSONC 迁移；
- Doctor、Profile list、config validate、config schema；
- 质量 Fixture、离线 replay、Runtime benchmark、成本门禁和报告 Schema；
- Cache Guard；
- Claude rules/skills/agents/mcp/hooks 导入基础链路；
- `omr session resume`、`omr session export`；
- Session list/status/show、Task list/show、Hook list/status、Recovery 与结构化事件流只读联调；
- INT-06 Reasonix v1.18.0 真实客户端验证；
- `omr comment-check` CLI（参见 §10）；
- OMR-FIX-01～11 及其自动化测试。

## 3. P0：已完成的核心工作

### ✅ OMR-T01：真实质量基准扩展（已完成）

增加脱敏的多文件任务 Fixture，覆盖 Explore → Plan → Implement → Test → Review → Complete 全流程；固定允许/禁止路径、隐藏测试、回归测试和预期事件；增加 Native/OMR 配对回放和失败保留规则；报告区分基础设施失败、任务失败、判定失败和模型失败。

验收：新增 Fixture 可离线 replay，`go test ./...` 和 `omr benchmark quality --replay` 通过，失败运行不被静默丢弃；不得无配对证据宣称 OMR 优于 Native。

### ✅ OMR-T02：Prompt/规则注入可验证性（已完成）

明确根目录和子目录 `AGENTS.md` 的读取顺序、目标文件路径向上收集规则的优先级、README 和 `.reasonix/rules` 条件规则协议；在 Orchestrator Prompt 中加入来源、路径、冲突和有效性要求；增加 Prompt fixture 验证顺序和冲突处理。不得把动态时间、绝对路径或 Hash 写入模型 Prompt。

## 4. P1：已完成的独立能力

### ✅ OMR-T03：Claude 兼容层收尾（已完成）

增加 `.claude/commands` 只读导入；为 Agent/Skill 增加 frontmatter Schema 校验；MCP 导入增加兼容性报告；Hook 报告列出可转换内容和无法保留的运行时语义；保持 dry-run、冲突、全量回滚和敏感信息保护。

### ✅ OMR-T04：Profile 与 Category 体验（已完成）

为每个 Profile 补齐用途、输入、输出、只读边界和失败处理；增加 Profile/Category Schema 与示例；检测未安装、已禁用、重复覆盖和循环路由；增加模型覆盖校验和 Doctor 诊断。Visual Profile 只有宿主明确提供视觉能力时才加入。

### ✅ OMR-T05：质量与成本可观测性（已完成）

统一 Runtime、Replay、Native/OMR 对照报告字段；增加重试次数、停滞原因、Review 阻断数、Token、成本和验证证据；支持稳定 JSON 快照和显式 `--run-id`；明确合成 run ID，不得称为 Reasonix Session ID；增加 Schema 版本迁移测试。

### ✅ OMR-T06：安装与升级体验（已完成）

已增加最低 Reasonix 版本和兼容矩阵字段，`omr version --json` 现在报告 OMR、资产、Manifest、Reasonix 检测版本和兼容状态；版本不满足或检测失败时返回 `compatible=false`。升级 dry-run、备份/回滚和不修改全局环境的边界保持不变。

## 5. P2：已完成的扩展能力

### ✅ OMR-T07：工具生态 Profile（已完成）

按宿主能力评估 LSP、AST/AST-Grep、Git Master、Browser/Playwright 和 Skill 内嵌 MCP。宿主没有对应能力时只记录调查结果，不嵌入不可执行资产。

### ✅ OMR-T08：开发体验（已完成）

评估显式增强模式、Ralph Loop、用户级配置和交互式通知。只能作为 Prompt/配置层能力，不复制 Reasonix 后台任务或状态机。**Comment Checker 运行时 Hook** 已因 Reasonix v1.18 原生 Hook 可用而解除阻塞，并已在 T14 完成。

### ✅ OMR-T09：规则和配置兼容性（已完成）

完善配置 Schema 自动生成、编辑器提示、JSONC 文档、`.agents/skills` 兼容、用户级/项目级优先级以及跨平台路径和权限测试。

## 6. BLOCKED：当前不在 OMR Todo 中实现

以下能力仍依赖 Reasonix 后续稳定公开接口：Subagent 父子关联事件、后台 Agent 结果汇聚、OMR 到 Desktop Task Monitor 的映射，以及 Tmux/桌面端实时状态面板。

Session list/status/show、Hook list/status、后台 Task 查询、Session recovery 和结构化事件流已完成公开接口适配与真实客户端验证，不再属于 BLOCKED。

**Comment Checker 运行时 Hook 拦截**：Reasonix v1.18 已提供原生阻塞型 Hook，不再属于宿主 BLOCKED；OMR 的安装、合并、回滚和跨平台执行适配列入 T14。

禁止读取 `~/.reasonix/projects`，禁止解析 goal-state/events/lock 私有文件，禁止在宿主 CLI 不支持时返回空列表伪造成功，禁止以 OMR 合成 ID 冒充 Reasonix Session ID。

## 7. 推荐开发顺序

1. ✅ T14：Comment Checker 运行时 Hook 安装、诊断与回滚；
2. Subagent 父子任务树、Desktop/Tmux 和后台结果汇聚等待 Reasonix 官方接口；
3. 不重复开发已经完成的 T01～T13 与 INT-01～INT-06。

每个任务必须先写回归测试或 Fixture，做最小代码修改，运行 `gofmt`、`git diff --check`、`go test ./...`、`go vet ./...`，更新本文件和差距矩阵，并保留未跟踪的 `omr`、`.reasonix/`。只有真实客户端行为无法通过自动化判断时，才请求用户协助。

## 8. ✅ T11：Grill Me 可选方案质询 Skill（已完成）

### 目标

在开始复杂开发任务前，通过有限轮次的质询暴露目标歧义、隐含假设、边界条件、失败路径和验收缺口，再将确认后的结果交给 `omr-planner`。

### 第一阶段范围

- 只实现项目级、可选的 Prompt/Profile 资产；
- 输入为目标、约束、已有方案和未知项；
- 输出为澄清问题、假设、风险、待确认决策和可执行验收条件；
- 设置最大问题轮数和明确完成条件；
- 支持 dry-run、禁用和项目级配置；
- 增加离线 Fixture：5 项 Prompt 契约测试 + 5 项纯 Go 离线回放测试，覆盖三种停止条件、未确认假设隔离和文件快照不变。

### T13 范围内明确不实现

- 不执行文件修改、Hook、Task 或后台任务；
- 不复制 Reasonix Session/恢复状态机；
- 不强制简单任务进入多轮质询；
- 不直接复制第三方 Skill 原文，先确认许可证和兼容性；
- 不把用户未确认的假设写入最终开发计划。

### 验收

- 默认关闭，启用后可随时停止；
- 复杂任务能产出结构化澄清结果；
- 达到最大轮数或信息充分时正常结束；
- 不修改项目文件、不访问私有 Reasonix 状态；
- `go test ./...`、`go vet ./...`、Prompt 契约测试和离线回放 Fixture 全部通过。

## 9. TODO：Subagent → Task Monitor 父子任务可观测性（等待 Reasonix 官方接口）

目标：在 Reasonix Desktop 的 Tasks 面板中展示 OMR 编排产生的 Subagent 树，而不是仅展示宿主独立后台 Task。

待 Reasonix 官方支持后再实现：

- Subagent 启动、运行、完成、失败事件；
- `parent_task_id` 或 `parent_session_id` 关联字段；
- Subagent 生命周期到 Task Monitor 的映射；
- Desktop 父子任务树展示；
- 取消、恢复、失败传播和并发状态测试。

前置条件：Task Monitor 官方 PR 合并并稳定发布，且 Reasonix 暴露上述机器接口。OMR 不伪造宿主 Task 状态，也不解析私有 Session 文件。

## 10. ✅ T13：Comment Checker 离线 CLI（已完成）

### 目标

在 OMR 中提供离线的注释质量检查 CLI，可扫描项目中的代码注释，输出结构化的 JSON/人类可读报告。不依赖网络、模型调用或 Reasonix 宿主。

### 范围

- 纯 Go 离线 CLI：`omr comment-check --project-dir . --json`；
- 5 条确定性规则：
  - R001：检测调试/临时标记（TODO、FIXME、XXX），支持白名单（`--allow-tags`）；
  - R002：检测空注释或无意义占位注释；
  - R003：检测注释与相邻代码过度相似（仅 warning，不阻断）；
  - R004：检测疑似凭据泄露（blocking，输出已脱敏）；
  - R005：路径安全校验，拒绝越界和路径穿越；
- JSON 报告含 `schema_version`、`blocking_count`、脱敏详情和修复建议；
- 人类可读输出含 summary、suggestion 和逐条 finding；
- 阻断时稳定非零退出码；
- 支持 `--path` 指定单文件、`--max-file-size` 跳过大文件、`--allowed-roots` 限制根目录。

### 测试覆盖

- 24 个离线单元测试 + 13 个 CLI 端到端测试；
- 覆盖：R001～R005 全部规则、白名单、凭据脱敏、路径穿越、二进制/大文件跳过、重复运行稳定、JSON/人类输出一致、工作区快照不变。
- R005 额外覆盖默认项目根、相对路径、文件符号链接和中间目录符号链接。

### 明确不实现

- 不读取 Reasonix Session、Task、Hook 文件；
- T13 不实现实时事件监听或提交前 Hook 拦截；该能力转入 T14；
- 不依赖模型或网络调用判断"注释是否有价值"；
- 不使用大模型判断注释质量；
- 不修改全局配置或 Reasonix 二进制。

### ✅ T14：运行时 Hook/实时阻断

Reasonix v1.18 已支持项目级阻塞型 Hook。OMR 的 Comment Checker 已实现完整的运行时 Hook：显式启用、配置合并、稳定绝对执行入口（guard）、旧版相对命令迁移、Doctor 诊断、禁用与回滚。自动化测试、CLI Smoke 和真实 Reasonix 桌面端的普通 Bash 放行、敏感注释阻断并脱敏、修复后提交放行及禁用流程均已通过。

### 验收

- Comment Checker/CLI 定向测试、`go vet ./...`、`go build ./cmd/omr` 通过；
- 全量 `go test ./...` 若受环境禁止 `httptest` 本地端口监听影响，必须如实记录该环境限制，不得伪报为通过；
- `omr comment-check --project-dir . --json` 输出稳定 JSON 报告；
- 阻断规则（R004）的凭据内容经过脱敏，不写入报告原文；
- 文档版本一致，运行时 Hook 标记为"T14 已完成并通过真实 Reasonix 桌面端验证"。
