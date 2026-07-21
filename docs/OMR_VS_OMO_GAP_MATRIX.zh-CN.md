# oh-my-reasonix 与 oh-my-opencode 差距矩阵

> 冻结版本：2026-07-21
>
> 本文用于冻结 OMR 后续优化范围与优先级。后续实现按本文排序推进；新增能力必须先更新本文，再进入开发。

## 1. 对比范围

本文比较的是两个项目的产品层能力：

- OMR：`oh-my-reasonix`，面向 Reasonix 的项目级 Prompt、Profile 和工作流发行层。
- OMO：`oh-my-opencode`，面向 OpenCode 的多 Agent 编排与工作流增强插件。

本文不把 Reasonix 已经原生提供的能力重复算作 OMR 的缺失。例如 Reasonix 原生已有的 `task`、后台任务、权限、沙箱、Todo、内置 Agent 和 `review`，属于宿主能力，不要求 OMR 重写。

## 2. 优先级定义

| 优先级 | 含义 | 进入条件 |
|---|---|---|
| P0 | 核心闭环 | 直接影响任务能否持续、恢复并可靠完成 |
| P1 | 主要生产力 | 明显影响复杂任务质量、编排效率或可维护性 |
| P2 | 产品完善 | 提升覆盖面、兼容性和日常体验 |
| P3 | 高级体验 | 非核心能力，待主体架构稳定后实现 |

## 3. 差距总表

### 3.1 核心工作流

| 能力 | OMR 当前状态 | OMO 对应能力 | 优先级 |
|---|---|---|---|
| 运行时任务强制 | 主要依赖 Orchestrator Prompt | Hook 和状态机制持续约束 Agent | P0 |
| Todo Continuation | 有 Todo 和 `complete_step`，中断恢复弱 | 自动强制未完成任务继续 | P0 |
| 失败恢复 | 没有统一恢复策略 | Session recovery、auto-resume | P0 |
| 停滞检测 | 没有统一检测 | 空响应、循环、无进展检测 | P0 |
| 测试失败闭环 | Prompt 要求修复后重测 | 工作流机制强制回到修复流程 | P0 |
| Review 阻塞闭环 | 已有 Review 和证据校验 | 持续执行直到 Blocking Issue 关闭 | P0 |
| 真实质量基准 | 当前主要是评分器，结果由外部提供 | 工作流可持续执行复杂任务 | P0 |
| 任务事件记录 | 没有统一 OMR 事件协议 | 有任务、Hook、后台通知链 | P0 |

### 3.2 Agent 编排

| 能力 | OMR 当前状态 | OMO 对应能力 | 优先级 |
|---|---|---|---|
| 专用 OMR Agent | 已有 `omr-explore`、`omr-research` | Sisyphus、Prometheus、Oracle、Librarian、Explore、Frontend 等 | P1 |
| Agent 独立模型配置 | 没有 OMR 层配置 | 每个 Agent 可覆盖模型、Prompt、权限 | P1 |
| 任务类别路由 | 没有 | visual、business-logic 等 Category | P1 |
| 后台 Agent 编排 | 依赖 Reasonix 原生，OMR 不统一编排 | 多 Agent 并行执行 | P1 |
| 并发策略 | 没有 OMR 层策略 | 按 Provider/Model 配置并发上限 | P1 |
| 后台结果汇聚 | 没有统一协议 | 通知、收集结果并继续主任务 | P1 |
| Debug Agent | 没有 | Oracle 等架构/调试 Agent | P1 |
| Research Agent | 已有只读 `omr-research` | Librarian/Research 类 Agent | P1 |
| Frontend Agent | 没有 | Frontend UI/UX Agent | P2 |
| Visual Agent | 没有 | Multimodal Looker 等 | P2 |

### 3.3 Hook 与生命周期

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| PreToolUse 检查 | 没有 | P1 |
| PostToolUse 检查 | 没有 | P1 |
| UserPromptSubmit 预处理 | 没有 | P2 |
| Stop/完成拦截 | 主要依赖 `complete_step` | P1 |
| Todo continuation enforcer | 没有 | P0 |
| Empty task response detector | 没有 | P0 |
| Comment checker | 没有 | P2 |
| Tool/Grep 输出截断 | 没有 | P1 |
| Context window monitor | 没有 | P1 |
| Preemptive compaction | 没有 | P1 |
| Session recovery | 没有 | P0 |
| Background notification | 没有 | P1 |
| Ralph loop | 没有 | P2 |
| Auto-update checker | 没有 | P2 |
| Startup toast | 没有 | P3 |

### 3.4 上下文与规则注入

| 能力 | OMR 当前状态 | OMO 对应能力 | 优先级 |
|---|---|---|---|
| 根目录 `AGENTS.md` | Orchestrator 已要求任务前读取 | 自动注入 | P1 |
| 子目录 `AGENTS.md` | Orchestrator 已要求按目标路径向上收集 | 按文件路径向上收集 | P1 |
| README 自动注入 | Orchestrator 已要求读取相关目录 README | 按目录注入 | P1 |
| 条件规则 | Orchestrator 已要求读取匹配的 `.reasonix/rules` | `.claude/rules` + glob 匹配 | P1 |
| 规则优先级 | 已定义路径更近、用户消息、安全规则优先级 | 项目/用户多层覆盖 | P1 |
| 规则漂移检查 | 只检查 OMR 资产 | 可扩展到项目规则 | P2 |

### 3.5 配置与可定制性

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| 独立 OMR 配置文件 | 已支持项目级 `.reasonix/omr/config.toml`，覆盖质量基准和 Runtime 默认值 | P1 |
| Agent 模型覆盖 | 没有 OMR 层配置 | P1 |
| Agent Prompt 覆盖 | 没有 | P1 |
| Agent 权限覆盖 | 依赖 Reasonix 原生，缺少 OMR 声明层 | P1 |
| Hook 禁用列表 | 没有 | P1 |
| Profile 禁用列表 | 没有 | P1 |
| 并发上限配置 | 没有 | P1 |
| Category 自定义 | 没有 | P2 |
| 配置 Schema 校验 | 已有 Doctor 对当前 TOML 子集做结构和范围校验，完整 Schema 仍未实现 | P1 |
| 用户级配置 | 明确不支持 | P2 |
| JSONC/注释配置 | 没有 | P2 |
| 环境变量展开 | 没有 OMR 配置层 | P2 |

### 3.6 工具生态

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| 官方文档查询 MCP | 没有 OMR 内置集成 | P1 |
| GitHub 代码搜索 | 没有 | P1 |
| Web 搜索 | 没有 OMR 内置集成 | P1 |
| Skill 内嵌 MCP | 没有 | P2 |
| LSP 优先路由 | 不由 OMR 编排 | P1 |
| AST/AST-Grep 工作流 | 没有 | P1 |
| 自动 Rename/Refactor 规则 | 没有 OMR 规则 | P1 |
| 浏览器自动化 Skill | 没有 | P2 |
| Git Master Skill | 没有 | P2 |
| MCP OAuth 管理 | 没有 | P3 |

### 3.7 Session 与可观测性

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| Session 列表 | 没有 OMR 级入口 | P1 |
| Session 搜索 | 没有 | P2 |
| Session 内容读取 | 没有 | P1 |
| Session 恢复 | 没有 | P0 |
| 后台任务状态 | 没有统一 OMR 展示 | P1 |
| Agent 调用树 | 没有 | P2 |
| Token/成本统计 | Cache Guard 有部分指标 | P1 |
| Hook 日志 | 没有 | P2 |
| 失败原因归档 | 没有 | P1 |
| Tmux/可视化后台终端 | 没有 OMR 层支持 | P2 |

### 3.8 安装、升级与兼容层

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| Agent 自安装 Prompt | 已有 | 已完成 |
| Dry-run | 已有 | 已完成 |
| 备份/回滚 | 已有 | 已完成 |
| Manifest/Hash | 已有 | 已完成 |
| 字段级卸载 | 已有 | 已完成 |
| 项目级安装 | 已有 | 已完成 |
| 用户级安装 | 没有 | P2 |
| 自动更新提示 | 没有 | P2 |
| 配置格式迁移 | 没有 | P2 |
| 版本升级冲突诊断 | 已有基础能力，仍需增强 | P1 |
| Claude Code Commands 兼容 | 没有 | P2 |
| Claude Code Agents 兼容 | 没有 | P2 |
| Claude Code Skills 兼容 | 仅支持 Reasonix 原生 Skill | P1 |
| Claude Code MCP 配置兼容 | 没有 | P2 |
| Claude Code Hooks 兼容 | 没有 | P2 |
| `.claude/rules` 兼容 | 没有 | P1 |
| `.agents/skills` 兼容 | 没有 | P2 |

### 3.9 高级体验

| 能力 | OMR 当前状态 | 优先级 |
|---|---|---|
| 一键增强模式（类似 `ultrawork`） | 没有 | P3 |
| Think Mode | 没有 OMR 层规则 | P3 |
| 多模型成本策略 | 没有 OMR 层策略 | P1 |
| Provider fallback | 主要依赖 Reasonix | P1 |
| 交互式终端/Tmux | 没有 | P2 |
| 自动更新提示 | 没有 | P2 |
| 评论质量控制 | 没有 | P2 |
| 视觉任务编排 | 没有 | P2 |
| Web/桌面状态面板 | 没有 OMR 层实现 | P3 |

## 4. OMR 已完成、不重复实现的能力

以下能力已经完成，或属于 Reasonix 原生能力，不作为后续差距：

- 项目级安装、升级、卸载、备份和回滚；
- Prompt Composer、Prompt Manifest 和来源 Hash；
- `omr-explore` Profile；
- Native Todo 和 `complete_step` 证据校验；
- 专用 `review(task=...)` 集成及 Review Blocking Issue 规则；
- Reasonix 原生 `task`、后台任务、权限、沙箱和内置 Agent；
- Doctor、Prompt/Profile Hash Drift 检查；
- Cache Guard 基础静态和离线分析；
- 质量 Smoke 夹具和评分器。

## 5. 分阶段实现路线

### Phase 1：从 Prompt 层升级为工作流层（P0）

1. 独立 `.reasonix/omr/config.toml`；
2. Todo Continuation Enforcer；
3. 空响应和无进展检测；
4. 测试失败 → 修复 → 重测状态机；
5. Session 恢复；
6. 任务事件日志；
7. `omr benchmark quality --run` 真实执行模式。

### Phase 2：扩展 Agent 团队（P1）

1. `omr-research`；
2. `omr-debug`；
3. `omr-planner`；
4. `omr-frontend`；
5. Category 路由；
6. 并发、成本和 Provider fallback 策略；
7. 后台结果汇聚。

### Phase 3：上下文与 Hook（P1）

1. `AGENTS.md` 注入；
2. README 注入；
3. `.reasonix/rules` 条件规则；
4. PreToolUse/PostToolUse；
5. 输出截断；
6. Context window monitor；
7. Comment checker。

### Phase 4：工具生态（P1/P2）

1. 官方文档查询；
2. GitHub 代码搜索；
3. LSP/AST 优先路由；
4. Browser Skill；
5. Git Master Skill；
6. Skill 内嵌 MCP。

### Phase 5：兼容层与高级体验（P2/P3）

1. 用户级安装；
2. `.claude`/`.agents` 兼容；
3. JSONC 配置；
4. Ralph Loop；
5. Tmux 可观测后台 Agent；
6. 自动更新；
7. Web/桌面状态面板。

## 6. 冻结规则

- 后续实现默认按 P0 → P1 → P2 → P3 推进。
- 新增能力必须先补充本矩阵和验收标准。
- 不为追求 OMO 表面功能而重复实现 Reasonix 原生能力。
- 需要修改 Reasonix 上游时，必须单独提交技术论证、兼容性分析和 PR，不得隐式混入 OMR。
- 每个 Phase 至少提供单元测试、端到端夹具和可观测验证结果。

## 7. 参考资料

- oh-my-opencode 官方仓库 README：<https://github.com/opensoft/oh-my-opencode>
- oh-my-opencode Features Reference：<https://github.com/Wangmerlyn/oh-my-opencode/blob/dev/docs/reference/features.md>
- oh-my-opencode Configuration Reference：<https://github.com/opensoft/oh-my-opencode/blob/dev/docs/configurations.md>
