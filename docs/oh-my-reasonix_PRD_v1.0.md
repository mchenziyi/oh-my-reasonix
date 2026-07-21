# oh-my-reasonix 产品需求文档（PRD）

**版本：** v1.0  
**状态：** 最终评审稿  
**日期：** 2026-07-21

> Maximum Intelligence · Stable Cache

| 文档版本 | v1.0 |
| --- | --- |
| 文档状态 | 最终评审稿 |
| 产品形态 | Reasonix 原生多 Agent Coding Harness |
| 目标平台 | Reasonix Go 版 |
| 发布日期 | 2026-07-21 |
| 核心原则 | 该给模型的全部给它，然后把一切稳稳缓存住 |

定位：将 oh-my-opencode 的成熟 Agent 能力体系，重新实现为 Reasonix 原生、缓存稳定、完整上下文优先的开发工作流。


# 文档控制

| 版本 | 日期 | 状态 | 主要变化 |
| --- | --- | --- | --- |
| v0.1 | 2026-07-21 | 概念草案 | 提出 cache-native 多 Agent 方向 |
| v0.2 | 2026-07-21 | 原则修订 | 删除 Token 节省目标，改为完整 Prompt 与完整上下文优先 |
| v0.3 | 2026-07-21 | 调研修订 | 补充背景；确认 Reasonix Go 版核心接口未表现出破坏性演进风险 |
| v1.0 | 2026-07-21 | 最终评审稿 | 整合产品定位、架构、Agent、Skill、需求、验收、MVP 与路线图 |


# 目录

1. 产品概述

2. 背景

3. 产品定位

4. 产品原则

5. 产品目标与非目标

6. 目标用户与核心场景

7. 产品形态与总体架构

8. Agent 体系

9. Skill 体系

10. 工作流与路由策略

11. 功能需求

12. 缓存与上下文规范

13. 命令与交互设计

14. 配置设计

15. 指标与可观测性

16. 基准与验收

17. 权限与安全

18. 兼容性与工程要求

19. 风险与应对

20. MVP 范围

21. 路线图

22. 发布验收标准

23. 待决策项

附录 A. Prompt 迁移策略

附录 B. 参考资料


# 1. 产品概述

oh-my-reasonix 是一套面向 Reasonix 的完整多 Agent 软件开发工作流发行包。它参考 oh-my-opencode 在专业 Agent、Skills、Commands、Hooks、并行任务和工程闭环方面的成熟设计，但不机械复制其运行方式，而是围绕 Reasonix 的前缀缓存不变量重新实现。

| 一句话定位给 Reasonix 一支完整的软件开发团队，同时保持 DeepSeek 长会话的高缓存命中率。 |
| --- |

产品不以缩短提示词、减少输入 Token 或控制上下文长度为目标。只要内容能够提升任务质量，并且能够稳定地进入缓存前缀，就应尽可能完整地提供给模型。

oh-my-reasonix 的核心公式：

```text
能力上限 = 完整提示词 + 完整上下文 + 专业分工 + 严格验证成本优势 = 稳定前缀 + 高缓存命中率
```


# 2. 背景


## 2.1 Coding Agent 正在从工具演进为工程系统

现代 Coding Agent 已不再只是“对话 + 写文件”。复杂任务需要任务理解、代码库探索、规划、实现、测试、审查、并行协作和长时间持续执行。Agent Harness 的竞争力越来越取决于专业 Prompt、任务分工、工具约束和验证纪律，而不只是底层模型。


## 2.2 oh-my-opencode 的参考价值

oh-my-opencode 已形成较完整的 Agent Harness：包含主编排 Agent、架构咨询、代码探索、文档研究、规划与审查 Agent，并提供 Skills、Commands、Hooks、后台任务、MCP、LSP 与 AST 工具。它证明了“优秀模型 + 专业团队分工 + 成熟工作流”能够显著提升复杂编码任务的完成度。[3]

专业分工：将探索、架构、规划、实现、审查等职责拆给不同 Agent。

完整行为约束：通过长 Prompt 防止偷懒、过早结束、跳过测试和无证据完成。

任务编排：支持并行研究、后台任务和结果汇总。

开箱即用：把大量需要用户自行配置的工作流打包为默认能力。


## 2.3 DeepSeek 改变了长上下文的成本模型

DeepSeek 的上下文缓存默认启用；后续请求只要完整匹配已经持久化的前缀单元，就可以命中缓存。这使“稳定的大前缀”具备持续复用价值，而频繁重写前部上下文会直接降低缓存收益。[4]

截至 2026-07-21，DeepSeek V4 Flash 与 Pro 均提供 1M 上下文；缓存命中输入价格分别为每百万 Token 0.02 元和 0.025 元。由此，本产品不把缓存命中后的输入 Token 视为需要重点节约的资源，而把上下文完整性视为模型能力资源。[5]

| 核心判断Token 数量不是主要问题，缓存失效才是问题；上下文长度不是成本中心，而是模型能力资源。 |
| --- |


## 2.4 Reasonix 提供了合适的原生基础

Reasonix 是围绕前缀缓存稳定性设计的 DeepSeek-native Coding Agent。其当前架构是配置和插件驱动的 Go Harness，支持独立、缓存稳定的多模型会话；Skills 可通过 runAs: subagent 启动隔离子 Agent，会话、工具和插件能力已能够承载完整的多 Agent 工作流。[1][2]

本次对 Reasonix Go 版后续版本的调研未发现 Markdown Skill、runAs: subagent、独立子 Agent 会话等核心公开能力被破坏性替换。后续变化主要表现为新增能力与参数，因此 Reasonix 上游变化不作为独立产品风险，只保留常规兼容性测试。[6]


## 2.5 用户问题

Reasonix 原生能力已经较强，但用户仍需自行设计 Agent 角色、编写 Prompt、组织 Skills、配置权限、制定委派规则、搭建测试与审查闭环，并验证这些配置是否破坏缓存。普通开发者需要的是一套安装后即可使用、质量优先、缓存稳定的完整发行方案。


## 2.6 产品机会

oh-my-reasonix 的机会，是成为第一套面向 DeepSeek 前缀缓存原生设计的完整 Coding Harness：不是轻量版 Reasonix，也不是 oh-my-opencode 的简单移植，而是把后者的能力体系重新实现为 Reasonix 原生工作流。


# 3. 产品定位


## 3.1 一句话定位

| oh-my-reasonixA full-context, cache-stable multi-agent coding harness for Reasonix. |
| --- |


## 3.2 产品形态

第一阶段以独立安装包和配置发行包交付，不 fork Reasonix 核心。产品通过 Reasonix 原生 Skills、子 Agent、Commands、MCP、Hooks 和配置完成主要能力。只有在缺少必要的缓存观测能力时，才向 Reasonix 上游提交小型通用补丁。

```text
oh-my-reasonix/├── manifests/        # Prompt、Agent、Skill、工具清单与 Hash├── prompts/          # 完整 Agent Prompt├── skills/           # 工作流与专业能力├── commands/         # Slash Commands├── hooks/            # 生命周期与事件记录├── mcp/              # 固定 MCP 配置模板├── profiles/         # Light / Standard / Full / Ultrawork├── benchmarks/       # 质量、缓存与稳定性基准├── installer/        # 安装、升级、回滚、卸载└── docs/              # 用户与开发文档
```


## 3.3 差异化

| 产品 | 核心优势 | 主要约束 |
| --- | --- | --- |
| Reasonix | DeepSeek-native、长会话、缓存稳定 | 需要用户自行搭建完整工作流 |
| oh-my-opencode | Agent/Skill/Hook 体系成熟、功能丰富 | 动态编排方式未以 DeepSeek 前缀缓存为首要约束 |
| oh-my-reasonix | 完整 Prompt、专业 Agent 团队、Reasonix 原生、缓存稳定 | 首轮前缀较大；需要严格维护字节稳定性 |


# 4. 产品原则

P1 完整能力优先：不因提示词长、Skill 多或上下文大而删减有效内容。

P2 大而稳定的前缀：固定前缀可以很大，但会话建立后必须保持字节级稳定。

P3 只追加，不回写：任务状态、计划、Git 状态、测试结果和 Agent 结果通过新事件追加，不重写历史前缀。

P4 质量优先于 Token 节省：不设置 Prompt Token 预算，不基于费用阈值降级 Agent 或验证流程。

P5 上下文完整性优先：相关代码、完整 diff、诊断、测试、决策历史和项目规则应完整提供。

P6 独立会话隔离：不同 Agent 与不同模型使用独立会话，避免共享消息链和互相污染缓存。

P7 会话内工具稳定：工具名称、顺序、描述和 Schema 在会话中保持不变。

P8 显式可控：复杂自动化必须可观察、可关闭、可切换模式；简单任务不得被强行编排。

P9 原生优先：优先使用 Reasonix 的公开能力，不维护重型分叉。

P10 用基准决定删改：删除 Prompt 的依据是冲突、无效或降低质量，而不是长度。


# 5. 产品目标与非目标


## 5.1 产品目标

尽可能完整地迁移和吸收 oh-my-opencode 中有价值的 Agent、Prompt、Skill 与工作流。

提供开箱即用的探索、规划、实现、验证、审查和修复闭环。

在复杂任务中支持并行研究和受控并行实现。

保持 Reasonix 的高缓存命中特性，并让缓存变化可解释、可监控。

支持百万级长上下文任务，不因 Token 预算提前摘要或裁剪有效信息。

让用户通过少量命令即可切换轻量、标准、全量和 Ultrawork 工作模式。

通过行为基准持续验证 Prompt、Agent 和编排是否真正提升完成质量。


## 5.2 非目标

不做以节省输入 Token 为卖点的 Slim 版本。

不为了降低费用自动删除 Skill、缩短 Prompt、跳过审查或压缩相关上下文。

第一版不做任意 Provider 的实时动态路由。

第一版不做云端团队管理、计费平台或组织级权限中心。

第一版不追求几十个 Agent 的角色扮演式组织模拟。

不保证所有 oh-my-opencode 实现细节一比一兼容；迁移的是能力和行为，不是内部 API。


## 5.3 成功指标

| 维度 | 目标 |
| --- | --- |
| 缓存 | 代表性长任务稳态缓存命中率中位数 ≥ 95%；相对原生 Reasonix 下降 ≤ 3 个百分点 |
| 前缀稳定 | 除新会话、显式配置变化和必要压缩外，非预期前缀变异次数为 0 |
| 任务质量 | 完整工作流在复杂任务基准上优于原生单 Agent 基线 |
| Prompt 保真 | 高价值行为约束保留率 ≥ 90%；删除必须有冲突或质量证据 |
| 易用性 | Standard 模式安装后无需手写 Agent Prompt 即可使用 |
| 可靠性 | 所有“已完成”结论必须附带验证证据或明确说明未验证原因 |


# 6. 目标用户与核心场景


## 6.1 核心用户

使用 DeepSeek API 进行日常开发的个人开发者。

长时间运行 Reasonix 会话、重视缓存成本和上下文连续性的重度用户。

希望获得多 Agent 能力，但不愿自行编写整套 Prompt 和工作流的用户。

需要在大型代码库中进行架构分析、疑难调试和跨模块修改的开发者。

希望以较低成本获得接近“开发小队”体验的小团队。


## 6.2 核心场景

| 场景 | 用户目标 | 默认工作流 |
| --- | --- | --- |
| 代码库理解 | 快速定位入口、调用链、关键模块和风险 | Explore → 汇总证据 |
| 复杂功能开发 | 从需求到实现、测试、审查完整闭环 | Explore → Plan → Implement → Verify → Review |
| 疑难 Bug | 基于证据定位根因，避免盲修 | Explore → Oracle → Implement → Regression |
| 架构重构 | 比较方案并控制跨模块风险 | Metis → Prometheus → Oracle → Implement → Momus/Reviewer |
| 文档/依赖研究 | 查官方文档、源码和真实实现 | Librarian → 证据摘要 |
| 前端任务 | 完成 UI、交互、视觉和浏览器验证 | Frontend → Browser Skill → Reviewer |
| 大规模并行研究 | 同时探索多个子系统 | Fleet Read-only → Orchestrator 汇总 |


# 7. 产品形态与总体架构


## 7.1 分层架构

```text
用户命令 / 自然语言任务            │            ▼Orchestrator（父会话，稳定大前缀）            │    ┌───────┼────────┐    ▼       ▼        ▼ Skills   Commands   Routing Policy    │       │        │    └───────┴────────┘            │            ▼Reasonix 原生 task / fleet / runAs: subagent            │   ┌────────┼──────────────┐   ▼        ▼              ▼Explore   Oracle        Reviewer ...独立会话  独立会话        独立会话            │            ▼工具 / MCP / Hooks / 权限系统            │            ▼事件日志 + 物化视图 + Cache Guard + Metrics
```


## 7.2 核心组件

| 组件 | 职责 |
| --- | --- |
| Prompt Manifest | 记录 Prompt 来源、版本、加载顺序、Hash、许可证与修改说明 |
| Agent Registry | 定义 Agent 名称、角色、Prompt、权限、默认模型、输出协议 |
| Skill Registry | 定义完整 Skill、触发条件、依赖工具、优先级与冲突关系 |
| Routing Policy | 决定何时本地执行、何时委派、何时并行、何时升级到 Oracle |
| Event Log | 追加记录任务、计划、委派、验证、审查和状态变化 |
| Materialized Views | 由事件日志生成 Todo、Plan、Agent 状态和缓存面板 |
| Cache Guard | 检测固定前缀、工具 Schema 和加载顺序是否意外变化 |
| Benchmark Suite | 测量质量、缓存、速度、工具调用和回归 |
| Installer | 负责安装、合并配置、备份、升级、回滚和卸载 |


## 7.3 产品模式

| 模式 | 适用场景 | 行为 |
| --- | --- | --- |
| Light | 简单任务或低编排偏好 | 核心 Orchestrator + Explore + Reviewer；不自动并行 |
| Standard | 日常开发，默认推荐 | 完整核心 Agent；受控自动委派；并行只读研究 |
| Full | 追求最大 Prompt 与 Skill 覆盖 | 全部稳定核心 Prompt/Skill 加载；不基于 Token 裁剪 |
| Ultrawork | 复杂、长时间、高并行任务 | 自动拆分、并行研究、完整验证和双重审查 |


# 8. Agent 体系

所有 Agent 都使用独立 Reasonix 会话。父 Agent 只传递结构化任务包和必要上下文；子 Agent 返回完整证据与结论。Agent Prompt 可以很长，但一旦会话启动就保持稳定。

| Agent | 职责 | 权限 | 运行特点 |
| --- | --- | --- | --- |
| Orchestrator / Sisyphus | 主编排、执行与闭环 | 读写、工具调用、委派 | 默认父会话 |
| Explore | 代码库探索、调用链、证据收集 | 只读，不可委派 | 快速、低延迟模型 |
| Oracle | 架构、疑难调试、复杂决策 | 只读，不可直接修改 | 高 reasoning effort |
| Librarian | 官方文档、依赖源码、多仓库研究 | 只读 + Web/MCP | 研究型会话 |
| Prometheus | 详细实施计划与里程碑 | 只读，可读取研究结果 | 高 reasoning effort |
| Metis | 需求澄清、隐藏风险、失败模式分析 | 只读 | 规划前咨询 |
| Momus | 计划审查、可验证性和完整性检查 | 只读 | 计划评审 |
| Reviewer | 代码审查、回归与验证缺口 | 只读 | 变更后运行 |
| Implementer | 边界明确的代码实现 | 受限写入路径 | 按模块隔离 |
| Frontend | UI/UX、交互、浏览器验证 | 前端目录写入 + Browser | 按需启用 |
| Multimodal Looker | 图片、PDF、视觉信息提取 | 只读；依赖可用多模态工具 | 可选扩展 |


## 8.1 Orchestrator 行为要求

先判断任务复杂度，简单任务直接执行，不为展示能力而创建子 Agent。

复杂任务先收集证据，再计划；不得凭猜测直接修改。

并行委派必须保证子任务边界清晰、结果可合并。

不得重复执行已由子 Agent 完成且证据充分的研究。

任何完成声明必须附验证证据；无法验证时必须明确说明。

计划、Todo 和状态变化通过事件追加，不反复重写完整前缀。


## 8.2 标准任务包协议

```text
task_id: stringgoal: stringbackground: stringrelevant_context: string | files[]constraints: string[]allowed_tools: string[]allowed_write_paths: string[]expected_output: schemaverification: string[]completion_definition: string
```


## 8.3 标准结果协议

```text
status: completed | blocked | partialsummary: stringevidence: [{source, location, finding}]changed_files: string[]verification_results: string[]risks: string[]recommended_next_action: string
```


# 9. Skill 体系


## 9.1 加载策略

默认采用“全量稳定加载”而非“按 Token 预算裁剪”。常用 Skill 可以完整进入固定前缀；专项 Skill 可以在子 Agent 会话初始化时完整加载。Modular 模式只用于避免行为冲突、限制权限或适配项目类型，不用于节省费用。


## 9.2 核心 Skills

planning

systematic-debugging

test-driven-implementation

verification-before-completion

code-review

task-delegation

git-workflow

cache-safe-context

architecture-analysis

dependency-research

documentation

frontend-ui-ux

browser-verification

security-review

performance-analysis

database-migration

release-preparation

incident-debugging


## 9.3 Skill 迁移原则

先完整迁移有价值的 Prompt 和工作流，再通过行为基准决定删改。

只修改 OpenCode 专属工具名、运行接口、多 Provider 路由和会破坏缓存的动态注入。

不因长度删除防偷懒、验证、规划、审查和工具纪律。

记录每个 Skill 的来源、版本、Hash、许可证和本项目修改说明。

发现冲突时建立优先级和适用范围，不默认用摘要替代完整 Skill。


## 9.4 Prompt Manifest 示例

```text
prompts:  orchestrator:    source: oh-my-opencode    upstream_version: "..."    hash: "sha256:..."    load: startup    priority: critical    license: "..."    modifications:      - map OpenCode tools to Reasonix capabilities      - remove cross-provider routing  explore:    source: oh-my-opencode    hash: "sha256:..."    load: subagent_startup    priority: critical
```


# 10. 工作流与路由策略


## 10.1 简单任务

```text
Understand → Execute → Verify → Report
```

适用于单文件小改、明确配置调整、简单查询和低风险修复。默认不创建子 Agent。


## 10.2 标准复杂任务

```text
Understand → Explore → Plan → Implement → Verify → Review → Fix → Final Verify
```


## 10.3 Ultrawork

```text
Intent Analysis  → Parallel Explore / Librarian / Metis  → Prometheus Plan  → Momus Plan Review  → Parallel or Sequential Implementation  → Test / Lint / Build / Runtime Verification  → Reviewer + Oracle (when needed)  → Fix Loop  → Final Verification and Evidence Report
```


## 10.4 自动路由规则

| 触发条件 | 默认动作 |
| --- | --- |
| 涉及 ≥ 3 个模块或调用链不明 | 启动 Explore |
| 连续 2 次修复失败 | 升级到 Oracle |
| 涉及外部库行为或版本差异 | 启动 Librarian |
| 变更行数或风险超过阈值 | 强制 Reviewer |
| 跨模块重构或迁移 | Metis → Prometheus → Momus |
| 前端视觉或浏览器行为 | Frontend + Browser Verification |
| 多个互不依赖的只读问题 | 使用 fleet 并行研究 |
| 写入范围存在重叠 | 禁止并行写入，回到单 Implementer/Orchestrator |


# 11. 功能需求

FR-01 一键安装：支持用户级与项目级安装；预览改动；备份现有配置；幂等安装；安全卸载。

FR-02 模式切换：支持 Light、Standard、Full、Ultrawork；模式变化仅对新会话生效。

FR-03 完整 Prompt 发行：提供完整 Orchestrator、Explore、Oracle、Librarian、Planner、Reviewer 等 Prompt，不设置任意 Token 上限。

FR-04 Agent 委派：通过 Reasonix 原生 task、fleet 或 runAs: subagent 启动隔离会话。

FR-05 并行只读研究：默认最多 4 个并行研究 Agent，可配置；必须使用结构化任务包。

FR-06 受控写入：写入 Agent 必须声明 allowed_write_paths；MVP 默认单写者。

FR-07 标准工程闭环：复杂任务默认包含探索、计划、实现、验证、审查、修复和最终验证。

FR-08 Prompt Manifest：记录来源、版本、加载顺序、Hash、许可证与变更历史。

FR-09 Prompt 冲突检测：检测矛盾指令、工具名残留、权限冲突、重复完成标准和动态模板变量。

FR-10 Cache Guard：会话启动时冻结 Prompt/工具清单；后续请求检测 Prefix/Schema Hash。

FR-11 缓存健康面板：展示缓存命中率、稳定前缀 Token、非缓存输入、前缀重置和压缩次数。

FR-12 事件日志：任务、计划、Todo、委派、验证、审查和状态变化以 append-only 事件记录。

FR-13 物化视图：从事件日志生成用户可读的 Todo、Plan、Agent 状态和任务时间线。

FR-14 完整上下文传递：支持完整 diff、完整测试输出、相关文件、诊断和历史决策；不基于费用裁剪。

FR-15 上游同步：可检测 oh-my-opencode Prompt 更新并生成差异报告，但更新必须由用户确认并在新会话生效。

FR-16 Doctor：检查 Reasonix 版本、Skills、Agent、MCP、权限、Hash、工具稳定性和配置冲突。

FR-17 Benchmark：每次发布运行质量、缓存、工具调用、长会话和并发回归测试。

FR-18 中文优先：默认提供中文交互、错误和文档，同时保留英文 Prompt 版本。


# 12. 缓存与上下文规范


## 12.1 缓存不变量

| 编号 | 不变量 |
| --- | --- |
| INV-01 | 同一会话中 System Prompt 字节保持一致。 |
| INV-02 | 同一会话中工具名称、顺序、描述和 JSON Schema 保持一致。 |
| INV-03 | Todo、Plan、Git 状态和 Agent 状态不得回写固定前缀。 |
| INV-04 | 子 Agent 启动不得改变父会话工具集合。 |
| INV-05 | 不同 Agent 和不同模型不得共享同一消息会话。 |
| INV-06 | 动态 Skill 不得插入父会话历史前部；完整内容应在对应会话启动时加载。 |
| INV-07 | 时间戳、随机 ID、临时路径和非确定性遍历结果不得进入固定前缀。 |
| INV-08 | 配置、插件、Skill、Agent 与工具加载必须使用确定性排序和序列化。 |
| INV-09 | Prompt 或工具更新仅对新会话生效。 |
| INV-10 | 每次缓存重置必须记录原因。 |


## 12.2 允许的缓存重置

用户开始新会话。

用户显式切换模型或模式。

用户修改 System Prompt、Agent、Skill、工具或 MCP 配置。

产品升级并引入模型可见 Prompt 或 Tool Schema 变化。

接近模型硬上限，必须进行上下文压缩。

用户显式执行 reload/reset。


## 12.3 上下文策略

完整相关上下文：相关代码、完整 diff、测试输出、架构决策和研究证据应尽可能完整保留。

不按费用裁剪：不得仅因为 Token 多而删除 Skill、历史或验证信息。

过滤噪声而非节省 Token：只删除无关、重复干扰、过时冲突或敏感内容。

过时信息追加失效标记：不回写旧事件；追加 superseded/invalidate 事件。

压缩接近硬上限才触发：摘要必须保留事实、决策、未完成事项、验证证据和文件引用。


## 12.4 事件示例

```text
TaskCreated {id: 42, goal: "重构认证模块"}TaskDelegated {id: 42.1, agent: "explore"}FindingAdded {task: 42.1, file: "auth/service.go", ...}PlanCreated {task: 42, version: 1}PlanSuperseded {task: 42, old: 1, new: 2}VerificationCompleted {task: 42, command: "go test ./...", status: "passed"}ReviewCompleted {task: 42, status: "changes_requested"}
```


# 13. 命令与交互设计

| 命令 | 作用 |
| --- | --- |
| /omr:status | 显示模式、Agent、Skill、运行任务和健康状态 |
| /omr:doctor | 检查安装、配置、MCP、权限、Prompt/Schema Hash |
| /omr:cache | 显示缓存命中、稳定前缀、重置与压缩信息 |
| /omr:mode light\|standard\|full\|ultrawork | 设置新会话默认模式 |
| /explore <task> | 启动只读代码库探索 |
| /oracle <question> | 启动架构或疑难调试咨询 |
| /research <task> | 启动文档/源码研究 |
| /plan <goal> | 创建详细可验证计划 |
| /review [scope] | 审查当前变更或指定范围 |
| /implement <task> | 启动边界明确的实现 Agent |
| /ultrawork <goal> | 启动完整多 Agent 工作流 |


## 13.1 缓存面板示例

```text
Cache health: HEALTHYCached input ratio: 98.2%Stable prefix: 184,320 tokensNew uncached input: 3,412 tokensPrefix mutations: 0Tool schema hash: unchangedPrompt manifest hash: unchangedCompactions: 0Active agent sessions: 4
```


# 14. 配置设计

```text
[oh_my_reasonix]mode = "full"language = "zh-CN"auto_delegate = truemax_research_agents = 4max_writer_agents = 1cache_guard = "strict"context_policy = "full"[oh_my_reasonix.agents]explore = trueoracle = truelibrarian = trueprometheus = truemetis = truemomus = truereviewer = trueimplementer = truefrontend = truemultimodal_looker = false[oh_my_reasonix.routing]oracle_after_failed_attempts = 2review_after_file_changes = trueforce_plan_for_cross_module_changes = trueparallel_research_threshold = 2[oh_my_reasonix.cache]warn_below_hit_rate = 0.90fail_on_prefix_mutation = truerecord_schema_hash = truerecord_prompt_hash = trueupdates_apply_to_new_sessions_only = true[oh_my_reasonix.context]compress_only_near_hard_limit = trueretain_full_diffs = trueretain_test_output = trueretain_decision_history = true
```

注：具体配置格式以 Reasonix Go 版公开配置能力为准。产品设计要求不依赖某个内部存储结构。


# 15. 指标与可观测性


## 15.1 缓存指标

cached_input_tokens / total_input_tokens

稳定前缀 Token 数

每轮新增非缓存 Token 数

前缀变异次数与首次变异位置

Prompt Manifest Hash / Tool Schema Hash

缓存重置原因

上下文压缩次数和压缩前后 Token


## 15.2 质量指标

任务基准通过率

首次修复成功率

回归测试遗漏率

Reviewer 阻断问题命中率

虚假完成率

计划可执行率和步骤完成率

子 Agent 结果采用率与重复研究率


## 15.3 效率指标

首 Token 延迟与稳态延迟

任务总完成时间

Agent 并发利用率

工具失败和重试次数

委派开销与结果等待时间

输入 Token 总量仅作为观测指标，不作为自动降级或版本优化目标。


# 16. 基准与验收


## 16.1 基准矩阵

| 基准 | 目的 |
| --- | --- |
| 单 Agent 长会话 | 测量原生缓存基线和上下文连续性 |
| Full Prompt 长会话 | 验证大固定前缀的稳态缓存表现 |
| Explore 子 Agent | 验证隔离会话和结果返回 |
| 4 Agent 并行研究 | 验证 fleet、并发和汇总质量 |
| Planner + Executor | 验证独立会话和计划执行一致性 |
| 复杂 Bug 修复 | 比较单 Agent 与多 Agent 首次修复成功率 |
| 跨模块功能 | 验证规划、实现、审查和回归闭环 |
| 上下文接近 1M | 验证长上下文稳定性和压缩策略 |
| Prompt 升级 | 验证新会话冷启动和旧会话不受影响 |
| MCP 开关组合 | 验证工具 Schema 稳定与配置边界 |


## 16.2 对照组

同版本原生 Reasonix，单 Agent。

oh-my-reasonix Light。

oh-my-reasonix Standard。

oh-my-reasonix Full。

oh-my-reasonix Ultrawork。


## 16.3 发布阻断条件

出现非预期 System Prompt 或 Tool Schema 变异。

稳态缓存命中率中位数低于 95%，或相对原生下降超过 3 个百分点。

完整模式任务质量显著低于 Standard，且无法由 Prompt 冲突解释。

复杂任务存在无验证完成或审查流程被绕过。

安装/卸载破坏用户原配置。


# 17. 权限与安全

默认单写者：MVP 中同一工作区默认只允许一个写入 Agent。

路径所有权：Implementer 必须声明可写路径；越界写入由执行层阻止。

危险命令：继续服从 Reasonix 原生权限策略和用户确认。

只读 Agent：Explore、Oracle、Librarian、Metis、Momus、Reviewer 默认不可写。

MCP 权限：MCP 不因 oh-my-reasonix 自动获得额外文件或网络权限。

敏感信息：API Key、私有配置和用户内容不得进入公开遥测。

Prompt 供应链：所有上游 Prompt 和 Skill 必须记录来源、Hash 和许可证。


# 18. 兼容性与工程要求


## 18.1 Reasonix 兼容性

oh-my-reasonix 基于 Reasonix Go 版公开能力实现，包括 Markdown Skills、runAs: subagent、独立子 Agent 会话以及 task/fleet 等原生工具。本次调研未发现 Go 版后这些核心公开能力发生破坏性替换，因此上游变化不作为独立产品风险。

每次 Reasonix 发布后仅需运行基础兼容性测试：Skill 加载、子 Agent 启动、task/fleet 调用、会话恢复、工具 Schema Diff 和缓存前缀测试。


## 18.2 平台

macOS

Linux

Windows

Reasonix CLI；Desktop 作为后续兼容界面


## 18.3 工程质量

Prompt、Skill 与执行代码分离。

所有清单确定性排序和序列化。

每个 Agent 有独立单元测试和行为基准。

配置 Schema 可验证，安装器支持 dry-run。

每次发布记录 Prompt Diff、Schema Diff 和缓存基准变化。


# 19. 风险与应对


## 19.1 Prompt 冲突

大量完整 Prompt 和 Skill 可能产生相互矛盾的工作流、权限、完成标准或工具指令。

建立 Prompt Manifest 和指令优先级。

运行静态冲突检测和行为基准。

只删除冲突、错误或无效内容，不因长度删减。


## 19.2 无关上下文污染判断

完整上下文不等于无差别堆积。无关、重复、过时或相互矛盾的信息可能降低模型判断质量。

按任务相关性组织上下文。

对过时信息追加失效标记。

区分规则、事实、计划、证据和历史。

过滤目标是减少噪声，不是节省 Token。


## 19.3 工具暴露过多降低调用精度

固定工具集合过大可能增加选错工具或参数的概率，即使 Token 成本不构成问题。

按会话 Profile 在启动时确定稳定工具集。

使用清晰、互斥的工具描述。

通过执行层限制权限，不在会话中动态增删。


## 19.4 并行写入冲突

多个 Agent 修改相同文件或共享状态可能造成冲突和错误集成。

MVP 默认单写者。

后续使用路径所有权和隔离工作树。

冲突时停止自动合并并交给 Orchestrator。


## 19.5 上游 Prompt 许可证与来源

尽可能完整迁移 oh-my-opencode 内容时，必须尊重上游许可证、署名和再分发要求。

逐文件记录来源和许可证。

保留 NOTICE/ATTRIBUTION。

无法确认许可的 Prompt 只做行为参考并重新实现。

发布前完成许可证审计。


# 20. MVP 范围


## 20.1 包含

安装器、升级、备份、回滚和卸载。

Standard 与 Full 两种模式。

完整 Orchestrator、Explore、Oracle、Librarian、Prometheus、Reviewer Prompt。

核心 Skills：planning、debugging、testing、verification、review、git、delegation、cache-safe-context。

并行只读研究，默认最大 4 个 Agent。

单写者 Implementer。

Prompt Manifest、Prompt Hash、Tool Schema Hash。

Cache Guard、/omr:doctor、/omr:cache。

事件日志和基础 Todo/Plan/Agent 状态视图。

中文文档与 Prompt；英文 Prompt 保留。

质量和缓存基准。


## 20.2 暂不包含

自动多 Provider 路由。

默认并行写入。

动态增删工具或 MCP。

以 Token 成本为目标的 Slim 模式。

基于费用阈值的 Prompt 裁剪、Agent 降级或任务中断。

完整 GUI 控制台。

复杂长期记忆系统。

强依赖多模态模型的视觉 Agent。


# 21. 路线图

| 阶段 | 目标 | 主要交付 |
| --- | --- | --- |
| M0 架构验证 | 证明完整 Prompt + 高缓存可共存 | 缓存基线、Prompt Manifest、3 个 Agent POC |
| M1 MVP | 可日常使用 | 安装器、Standard/Full、核心 Agent、Doctor、Cache Guard |
| M2 完整闭环 | 复杂开发任务可靠完成 | Prometheus/Metis/Momus、自动路由、Reviewer 修复循环 |
| M3 Ultrawork | 大任务并行协作 | fleet 编排、任务图、受控并行实现、完整时间线 |
| M4 1.0 | 稳定公开发行 | 跨平台、许可证审计、性能报告、升级策略、完整文档 |
| M5 生态 | 可扩展社区能力 | 第三方 Agent/Skill 包、可验证 Manifest、共享基准 |


# 22. 发布验收标准

安装和卸载不会破坏用户原配置，并可完整回滚。

Standard 与 Full 模式均能成功启动。

Explore、Oracle、Librarian、Planner、Reviewer 能使用独立会话运行。

四个并行只读任务能够完成并被 Orchestrator 正确汇总。

连续 20 轮工具调用中不存在非预期固定前缀变异。

稳态缓存命中率中位数不低于 95%，相对原生下降不超过 3 个百分点。

复杂任务必须包含可追溯的验证与审查证据。

简单任务不会自动创建不必要的子 Agent。

Prompt 冲突检测无阻断级问题。

所有上游 Prompt 和 Skill 均有来源与许可证记录。

Windows、macOS、Linux 各通过至少一套端到端测试。

所有缓存重置都能显示明确原因。


# 23. 待决策项

| 问题 | 建议默认方案 |
| --- | --- |
| 项目名称与包名 | oh-my-reasonix；安装命令后续根据实现语言确定 |
| 默认模式 | Standard；高级用户可切换 Full |
| 默认并发数 | 只读 4，写入 1 |
| Prompt 语言 | 中文交互 + 英文原始/迁移 Prompt 双版本 |
| 上游同步方式 | 生成 Diff，人工评审后发布新 Manifest |
| 浏览器能力 | 固定 Browser MCP Profile，启动后不动态增删 |
| 多模态 Agent | MVP 可选关闭，待 DeepSeek/外部工具能力确认 |
| 许可证策略 | 无法确认再分发许可的内容进行 clean-room 行为重写 |


# 附录 A. Prompt 迁移策略


## A.1 迁移顺序

建立上游文件清单和许可证清单。

完整提取 Agent、Skill、Command 和 Hook 的行为要求。

标记 OpenCode 专属工具、动态注入和多 Provider 路由。

将工具调用映射为 Reasonix 原生语义能力。

将动态前缀改为会话启动配置或 append-only 事件。

运行 Prompt 冲突检测。

对比完整迁移版与精简/原生基线的任务质量。

冻结 Hash，写入 Prompt Manifest。


## A.2 允许修改的内容

OpenCode 专属工具名称和调用格式。

与 Reasonix 权限模型不兼容的描述。

动态重建 System Prompt 的逻辑。

多 Provider 路由和同会话模型切换。

明确冲突、错误、失效或无法执行的指令。


## A.3 不应仅因长度删除的内容

防止偷懒和过早结束的规则。

计划、验证和审查纪律。

工具调用要求和证据标准。

任务拆分和委派策略。

错误恢复、失败升级和完成定义。


# 附录 B. 参考资料

[1] Reasonix 官方仓库与 README

[2] Reasonix Skills / runAs: subagent 说明

[3] oh-my-opencode 官方 Features 文档

[4] DeepSeek Context Caching 官方文档

[5] DeepSeek 模型与价格官方文档

[6] Reasonix Releases / Changelog（Go 版后续演进调研）

说明：PRD 中的产品目标、功能需求、指标、架构和风险判断属于 oh-my-reasonix 的产品设计；参考资料仅用于支撑背景事实与现有项目能力判断。

该给模型的全部给它，然后把一切稳稳缓存住。
