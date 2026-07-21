# oh-my-reasonix 产品需求文档（PRD）

## v1.1 开发评审稿

| 字段 | 内容 |
|---|---|
| 文档版本 | v1.1 |
| 文档状态 | 开发评审稿（Conditional GO for M0） |
| 产品名称 | oh-my-reasonix（简称 OMR） |
| 基线版本 | Reasonix `desktop-v1.17.16`，Commit `464d494` |
| 目标平台 | macOS、Linux、Windows |
| 默认语言 | 中文 |
| 编制日期 | 2026-07-21 |
| 评审对象 | 产品、架构、Reasonix 集成、Prompt 工程、测试与发布 |

> **一句话定位**  
> 为 Reasonix 提供一套开箱即用的专业 Coding Agent 工作流，在不重复实现 Reasonix 原生运行能力的前提下，最大化向模型提供完整、相关、可信且当前有效的上下文，并保持 DeepSeek 前缀缓存稳定。

---

## 0. 开发评审结论

### 0.1 结论

本项目具备产品与技术可行性，建议以 **Conditional GO** 进入 M0 架构验证阶段。

允许进入 M0 的原因：

1. Reasonix Go 版已提供稳定的 Skills、子 Agent、独立会话、任务状态、权限、并发控制、工具代理与缓存观测基础，OMR 无需重写 Agent Runtime。[R1][R2][R3]
2. OMR 的新增价值可以清晰限定为：Prompt 发行、角色与工作流设计、安装管理、缓存守卫、可复现基准和验收夹具。
3. MVP 可完全通过 Reasonix 公开能力实现，不要求修改 Reasonix 上游。
4. DeepSeek 提供缓存命中/未命中 Token 字段，但服务端缓存属于 best-effort，因此可采用“客户端确定性门槛 + 服务端观测指标”的双层验收模型。[R6]

### 0.2 M0 开始前必须确认

以下两项属于开工前置条件，而不是后续优化项：

- **许可证与来源策略确认**：oh-my-openagent 仓库元数据显示许可证为 `NOASSERTION`。在许可证未被人工确认前，MVP 只能采用行为研究、结构参考和 clean-room 重写，不得直接复制或再分发其 Prompt 原文。[R8][R9]
- **基线冻结**：所有首轮开发、测试和基准统一使用 Reasonix `desktop-v1.17.16`、Commit `464d494`；升级基线须通过变更评审。[R1]

### 0.3 M0 退出条件

M0 只有在以下条件全部满足后，才能进入 MVP 功能开发：

- 项目级安装、备份和卸载流程可用；
- Orchestrator、Explore、Reviewer 能被 Reasonix 正确加载；
- 不修改 Reasonix 二进制和源码即可跑通标准工作流；
- System Prompt、Tool Schema 和历史消息前缀可以被 Cache Guard 确定性校验；
- 三个固定测试夹具全部跑通；
- 许可证策略形成书面结论。

---

## 1. 背景

### 1.1 Coding Agent 的产品演进

Coding Agent 已从“单模型对话 + 文件编辑”逐步发展为完整的软件工程执行系统。成熟系统通常包含：

- 任务理解与计划；
- 代码库探索；
- 专业 Agent 分工；
- 子任务委派；
- 实现、测试、审查与修复闭环；
- 项目规则和长期上下文管理；
- 权限、沙箱、工具协议和可观测性。

以 oh-my-openagent（原 oh-my-opencode）为代表的 Agent Harness，已经展示了专业 Agent、Skills、Hooks、MCP、任务编排和验证纪律的价值。[R8][R9]

但传统 Harness 往往偏向动态构造：按任务重写 System Prompt、动态增删工具、反复注入 Todo/Plan/Git 状态、频繁切换模型或重建 Agent 环境。这些设计可能提高灵活性，却会破坏依赖精确前缀匹配的缓存复用。

### 1.2 DeepSeek 带来的架构机会

DeepSeek 的上下文缓存按前缀匹配工作；相同前缀可以命中缓存，但缓存构建、保存和命中属于 best-effort，并不保证每次 100% 命中。API 返回 `prompt_cache_hit_tokens` 与 `prompt_cache_miss_tokens`，可以用于实际观测。[R6]

DeepSeek V4 Flash 与 Pro 当前均提供 1M 上下文窗口；命中缓存的输入单价显著低于未命中输入。价格以美元计价并可能调整，因此本文只把价格作为架构背景，不把具体人民币换算写入长期产品承诺。[R7]

这意味着 OMR 的优化目标不应是“尽量少给模型 Token”，而应是：

> **最大化提供完整、相关、可信且当前有效的上下文，同时确保已经建立的稳定前缀不被无意义地重写。**

### 1.3 Reasonix 的基础优势

Reasonix `main-v2` 是 Go 重写版本，定位为 config-driven、plugin-driven、cache-aware 的 Coding Agent。当前版本已提供：

- 配置化 System Prompt 与 Skills；
- 独立子 Agent 会话；
- `task`、`fleet` 等任务委派工具；
- Delivery 工作模式；
- 原生 Todo 与 `complete_step`；
- 权限、沙箱和并发控制；
- 稳定工具代理 `use_capability`；
- Tool Schema 契约与快照测试；
- 会话持久化、压缩和缓存统计；
- `reasonix doctor capabilities` 等诊断能力。[R1][R2][R3][R4][R5]

因此，OMR 的正确产品形态不是另造一个 Agent Runtime，而是建立在 Reasonix 原生运行能力之上的“专业内容与工作流发行层”。

### 1.4 用户问题

Reasonix 已具备强大的底层能力，但用户仍需自行完成：

- 设计主 Agent 与子 Agent 的职责边界；
- 编写、维护和版本化 Prompt；
- 制定委派、实现、验证和审查规则；
- 处理 Reasonix 配置冲突；
- 建立缓存稳定性测试；
- 验证工作流质量是否优于原生基线；
- 管理安装、备份和安全卸载。

这些工作有较高的 Prompt 工程与系统设计门槛。OMR 的机会，就是把这部分沉淀成一套可安装、可验证、可维护的默认方案。

---

## 2. 问题定义

### 2.1 核心问题

如何在不重复实现 Reasonix 原生功能、不修改 Reasonix 上游、不过度限制上下文的前提下，为用户提供一套更成熟的复杂编码任务工作流，并证明它：

1. 提高了任务完成质量；
2. 没有制造不必要的子 Agent 和流程开销；
3. 没有破坏客户端可控的缓存前缀稳定性；
4. 可以安全安装、升级和卸载；
5. 可以通过固定夹具与统计协议复现结果。

### 2.2 当前版本需要避免的误区

OMR v1.1 明确不采用以下假设：

- “Reasonix 没有子 Agent，所以 OMR 要实现子 Agent”；
- “Todo 必须另建 append-only 事件系统”；
- “上下文越长越好，应主动填满 1M”；
- “缓存命中率低就一定是客户端前缀变化”；
- “增加更多 Agent 就等于更好的 MVP”；
- “从 oh-my-openagent 复制 Prompt 就能形成产品”；
- “所有功能都必须在首版提供 UI”。

---

## 3. 产品定位与原则

### 3.1 产品定位

OMR 是一套面向 Reasonix 的：

- Prompt 发行包；
- Agent Profile 发行包；
- 标准开发工作流；
- 项目级安装与配置管理工具；
- 缓存稳定性守卫与质量基准套件。

### 3.2 核心原则

#### 原则一：Reasonix 原生优先

Reasonix 已有能力一律优先直接使用或配置封装，不重复开发。

#### 原则二：完整的相关上下文优先

对任务有实际帮助的信息应尽可能完整提供；不因为 Token 数量或命中后的输入价格而削减有效内容。

#### 原则三：稳定前缀优先

System Prompt、Tool Schema、静态 Agent 规则和既有会话前缀不应在会话中被无意义重写。

#### 原则四：质量与缓存分开验收

Prompt 质量、任务完成质量与缓存稳定性分别测试，不能用缓存指标替代任务质量，也不能用偶发服务端未命中否定客户端确定性。

#### 原则五：单一事实源

MVP 中任务状态只以 Reasonix 原生 Todo 与 `complete_step` 为事实源，不维护第二套 OMR 状态模型。

#### 原则六：显式工作流优先

MVP 只提供一个明确的 Standard 工作流，不先做复杂的自动模式切换和多模式矩阵。

#### 原则七：可卸载、可审计

OMR 写入的每个文件都必须可追踪来源、内容 Hash、目标路径和卸载规则。

---

## 4. 产品目标与非目标

### 4.1 MVP 目标

1. 提供一个稳定的 Orchestrator Prompt。
2. 提供 Explore 与 Reviewer 两个专业子 Agent Profile。
3. 提供一个可重复执行的复杂编码标准工作流。
4. 复用 Reasonix Delivery 模式、Todo、`complete_step`、权限和工具契约。
5. 提供项目级安装、预览、备份和安全卸载。
6. 提供 Prompt Manifest 与来源追踪。
7. 提供 Cache Guard，验证客户端可控的前缀稳定性。
8. 提供一套可复现的原生 Reasonix 对照基准。
9. 通过固定测试夹具把“正确委派、正确汇总、验证后完成”转为可执行验收条件。

### 4.2 MVP 非目标

以下内容明确不在 v1.1 MVP 中：

- Oracle、Librarian、Planner、Frontend、Implementer 等更多角色；
- Light、Full、Ultrawork 等多模式；
- OMR 自建 Todo、Plan 或事件日志；
- `fleet` 自动并行研究；
- 并行写入；
- 多 Provider 或动态模型路由；
- 独立缓存可视化 UI；
- 用户级全局安装；
- 自动迁移所有 oh-my-openagent Skills；
- 中英文双份 Prompt；
- 修改 Reasonix 上游；
- 为节省 Token 进行动态 Prompt 裁剪；
- 为填满上下文而注入无关内容。

---

## 5. 基线与外部假设

### 5.1 冻结基线

| 项目 | MVP 基线 |
|---|---|
| Reasonix | `desktop-v1.17.16` |
| Reasonix Commit | `464d494` |
| Reasonix 分支参考 | `main-v2` |
| 默认工作模式 | `delivery` |
| DeepSeek 缓存基准模型 | `deepseek-v4-flash` |
| OMR 默认语言 | 中文 |
| 安装范围 | 项目级 |
| 最大子 Agent 并发 | 2 |
| 最大并行写者 | 1 |

### 5.2 外部假设

- Reasonix Go 版公开 Skills、子 Agent 和会话能力可用；
- Delivery 模式维持稳定工具表面与 `complete_step` 规则；
- DeepSeek API 返回缓存命中与未命中 Token；
- 服务端缓存命中存在波动，不能作为唯一确定性发布门槛；
- Reasonix Go 版自发布以来，相关核心扩展形式保持向后兼容；版本升级属于常规兼容性测试，不列为独立产品风险。

---

## 6. Reasonix 原生能力与 OMR 新增能力矩阵

### 6.1 分类定义

每项能力必须归入以下四类之一：

- **A｜原生直接使用**：Reasonix 已完整提供，OMR 不封装或只在文档中规定用法。
- **B｜配置封装**：Reasonix 已提供，OMR 只生成配置、默认值或安装内容。
- **C｜OMR 实现**：Reasonix 不提供该产品层能力，OMR 自行实现。
- **D｜必须修改上游**：仅在公开接口无法满足时使用；MVP 中应为 0 项。

### 6.2 能力差异矩阵

| 能力 | Reasonix 现状 | OMR 处理 | 分类 | MVP |
|---|---|---|---|---|
| Agent Loop | 已有完整运行时 | 不重复实现 | A | 是 |
| DeepSeek Provider | 已有 | 直接使用 | A | 是 |
| System Prompt 文件 | 支持 `system_prompt_file` | 安装稳定 Orchestrator Prompt | B | 是 |
| Markdown Skills | 原生支持 | 安装 OMR Skill/Profile 文件 | B | 是 |
| `runAs: subagent` | 原生支持 | 定义 `omr-explore`、`omr-review` | B | 是 |
| 独立子 Agent 会话 | 原生支持 | 直接使用 | A | 是 |
| `task` 委派 | 原生支持 | Prompt 中定义触发规则 | B | 是 |
| `fleet` | 原生支持 | MVP 不使用 | A | 否 |
| 并发限制 | 原生支持 | 默认配置为 2 | B | 是 |
| 并行写者限制 | 原生支持 | 默认配置为 1 | B | 是 |
| 写入路径隔离 | 原生支持 | MVP 不启用并行写入 | A | 否 |
| Delivery 模式 | 原生支持 | 作为 OMR 默认运行方式 | A | 是 |
| 稳定工具代理 | 原生 `use_capability` | 直接使用 | A | 是 |
| Tool Schema 契约 | 原生存在 | Cache Guard 额外记录 Hash | A+C | 是 |
| 原生 Todo | `todo_write` 覆盖完整当前列表 | 作为唯一任务状态事实源 | A | 是 |
| `complete_step` | 原生证据式完成 | Orchestrator 强制遵循 | A+B | 是 |
| 权限与沙箱 | 原生支持 | 不新增权限体系 | A | 是 |
| Native Doctor | 原生能力检查 | `omr doctor` 汇总 OMR 与原生检查 | A+C | 是 |
| Cache Hit 统计 | UI/API 可观测 | 基准工具收集、分类和对比 | C | 是 |
| Prompt Manifest | 无完整产品级清单 | OMR 实现 | C | 是 |
| Prompt 来源与 Hash | 无 OMR 发行追踪 | OMR 实现 | C | 是 |
| Standard 工作流 | 有底层能力，无 OMR 角色规则 | OMR Prompt 与测试定义 | C | 是 |
| 安装预览 | 非 OMR 原生需求 | `omr init --dry-run` | C | 是 |
| 配置备份 | 非 OMR 原生需求 | OMR 实现 | C | 是 |
| Hash 安全卸载 | 非 OMR 原生需求 | OMR 实现 | C | 是 |
| Cache Guard 记录代理 | Reasonix 无 OMR 专用协议 | OMR 实现，仅用于基准/诊断 | C | 是 |
| 独立 Todo 事件系统 | 原生 Todo 已可用 | 不实现 | - | 否 |
| 缓存 UI 面板 | 非 MVP 必需 | 不实现 | - | 否 |
| Reasonix 上游修改 | 当前无必要 | 不允许进入 MVP | D | 否 |

### 6.3 边界结论

MVP 的核心新增内容只有五类：

1. Prompt 与 Agent Profile 内容；
2. Prompt Manifest 与来源管理；
3. 项目级安装、备份和卸载；
4. Cache Guard 与基准协议；
5. 可执行工作流验收夹具。

Reasonix Runtime、Todo、子 Agent、并发、权限、工具代理和会话均不由 OMR 重写。

---

## 7. MVP 产品范围

### 7.1 交付物

```text
oh-my-reasonix/
├── cmd/omr/                       # OMR 安装与诊断 CLI
├── internal/
│   ├── install/                   # dry-run、备份、写入、卸载
│   ├── manifest/                  # Prompt Manifest 解析与 Hash
│   ├── doctor/                    # OMR + Reasonix 能力检查
│   └── cacheguard/                # 请求记录、前缀校验、指标计算
├── assets/
│   ├── prompts/
│   │   └── orchestrator.zh.md
│   ├── skills/
│   │   ├── omr-explore.md
│   │   └── omr-review.md
│   └── manifest.yaml
├── benchmarks/
│   ├── fixtures/
│   │   ├── simple-fix/
│   │   ├── cross-module-bug/
│   │   └── aggregation-conflict/
│   └── protocol.md
├── tests/
└── docs/
```

### 7.2 运行形态

OMR 是一个独立的 Go CLI，负责把静态内容安装到项目中。安装完成后，日常运行仍使用 Reasonix：

```bash
omr init --dry-run
omr init
reasonix --profile delivery
```

OMR 不常驻、不代理日常请求、不替换 Reasonix 二进制。仅在执行缓存基准时，用户显式启动透明记录代理：

```bash
omr benchmark cache
```

---

## 8. 核心产品与技术决策

### 8.1 仅支持项目级安装

MVP 只修改当前项目，不修改用户全局配置。目标路径示例：

```text
<project>/reasonix.toml
<project>/.reasonix/skills/omr-explore.md
<project>/.reasonix/skills/omr-review.md
<project>/.reasonix/omr/manifest.lock.yaml
<project>/.reasonix/omr/backups/<timestamp>/
```

这样可以降低跨项目污染和卸载风险。

### 8.2 Orchestrator 使用稳定 System Prompt 文件

OMR 通过 Reasonix 的 `system_prompt_file` 安装固定 Orchestrator Prompt，不通过 SessionStart Hook 注入核心规则。Reasonix 文档说明 SessionStart 动态上下文不会改变稳定 System Prompt，但可能降低该轮缓存复用，因此核心 Prompt 不走动态 Hook。[R2]

### 8.3 不覆盖用户现有 Prompt

若 `reasonix.toml` 已配置 `system_prompt_file`：

- 默认停止安装并报告冲突；
- `--compose-prompt` 可生成一个显式组合文件；
- 组合顺序固定且写入 Manifest；
- 不允许静默替换用户文件。

### 8.4 Agent Profile 使用唯一名称

MVP 使用：

```text
omr-explore
omr-review
```

避免覆盖 Reasonix 内置 `explore`、`review` 或用户自定义同名 Skill。

### 8.5 默认复用 Delivery 模式

Reasonix Delivery 模式已经提供完整工具表面、稳定 `use_capability`、验收标准、审查、验证和 `complete_step`。[R2]

OMR 不重新实现这一套运行规则，只在 Orchestrator Prompt 中规定何时使用 Explore、何时调用 Reviewer，以及何时允许完成。

### 8.6 不自建任务状态

MVP 的任务状态模型：

```text
唯一事实源 = Reasonix 原生 Todo + complete_step
```

`todo_write` 提交完整列表并覆盖当前 Todo 视图，这是 Reasonix 的既有契约。[R3]

“前缀只追加”仅描述模型消息历史与缓存约束，不要求产品状态数据库也必须 append-only。OMR 不监听 Todo 后再复制到第二套事件系统。

### 8.7 Cache Guard 只在诊断与基准中启用

Cache Guard 使用透明 OpenAI-compatible 记录代理：

```text
Reasonix
   │ exact request
   ▼
OMR Cache Guard Proxy
   │ unchanged request
   ▼
DeepSeek API
```

要求：

- 不改变 System Prompt、messages、tools、模型参数和顺序；
- 不记录 Authorization Header；
- 默认对文件内容、环境变量和工具结果做可配置脱敏；
- 原始请求日志默认关闭，仅保存 Hash 和 Token 统计；
- 代理异常时基准失败，不自动绕过；
- 不用于日常生产会话。

### 8.8 不直接复制未确认许可的 Prompt

在 oh-my-openagent 许可证未被人工确认前：

- 可研究角色、行为和工作流；
- 可记录功能差异；
- 可 clean-room 重写；
- 不直接复制、翻译或再分发原 Prompt 文本；
- 每个 Prompt 必须记录来源、修改和许可证状态。

---

## 9. 架构设计

### 9.1 总体架构

```text
┌─────────────────────────────────────────────────────────┐
│                     OMR CLI                              │
│  init / dry-run / doctor / uninstall / benchmark        │
└───────────────┬───────────────────────────┬─────────────┘
                │                           │
                ▼                           ▼
┌──────────────────────────┐   ┌──────────────────────────┐
│ Project Content Installer│   │ Cache Guard Benchmark    │
│ - reasonix.toml patch     │   │ - transparent recorder   │
│ - prompt/skills           │   │ - prefix validator       │
│ - manifest/backups        │   │ - metrics/report         │
└───────────────┬──────────┘   └──────────────┬───────────┘
                │                              │
                ▼                              ▼
┌─────────────────────────────────────────────────────────┐
│                   Reasonix Runtime                       │
│ Delivery / Skills / task / Todo / complete_step / tools │
└───────────────────────────┬─────────────────────────────┘
                            ▼
                    DeepSeek API
```

### 9.2 安装后模型

```text
Stable System Prompt
├── Reasonix 固定基础规则
└── OMR Orchestrator Prompt

Stable Tool Schema
└── Reasonix Delivery 工具集合

Project Skills
├── omr-explore
└── omr-review

Runtime State
├── Reasonix Todo
├── task/subagent 独立会话
└── complete_step evidence
```

### 9.3 无上游修改原则

MVP 必须满足：

- 不 fork Reasonix；
- 不调用 Reasonix 私有 Go 包；
- 不依赖内部数据库表；
- 不修改 Reasonix Tool Schema；
- 不要求新增 Hook；
- 不要求修改会话格式。

若开发中发现必须修改上游，需暂停对应需求，提交单独 ADR 与产品变更评审，不得隐式扩张 MVP。

---

## 10. Agent 与 Prompt 设计

### 10.1 Orchestrator

#### 职责

- 判断任务复杂度；
- 对复杂任务维护 Reasonix Todo；
- 必要时委派 `omr-explore`；
- 在充分理解后执行修改；
- 运行与变更相匹配的验证；
- 在最终修改后调用 `omr-review`；
- 处理阻断问题；
- 使用 `complete_step` 提交证据后完成。

#### 禁止行为

- 对简单任务机械创建子 Agent；
- 在未探索关键调用链前修改跨模块代码；
- 把 Reviewer 当作实现 Agent；
- 用“看起来正确”替代测试或静态验证；
- 在最终修改后不重新验证；
- 修改 System Prompt 或工具集合；
- 为节省 Token 删除关键上下文。

### 10.2 `omr-explore`

#### 目标

快速建立对目标代码、调用路径、相关测试和潜在风险的事实性理解。

#### 权限

默认只读。可以使用文件读取、搜索、Git 只读、LSP 和其他 Reasonix 允许的只读能力；不得编辑文件。

#### 输入协议

```yaml
task_id: string
goal: string
questions:
  - string
scope:
  include:
    - path-or-module
  exclude:
    - path-or-module
known_context:
  - string
expected_output:
  - relevant_files
  - execution_path
  - findings
  - uncertainties
  - recommended_next_step
```

#### 输出要求

- 区分事实、推断和未知；
- 每个关键事实指向文件或符号；
- 合并重复信息；
- 不假装读过未读取的文件；
- 不提出范围外实现。

### 10.3 `omr-review`

#### 目标

在最终修改后，从需求满足、正确性、回归风险、测试覆盖和安全边界五个方面进行独立审查。

#### 输入协议

```yaml
goal: string
acceptance_criteria:
  - string
changed_files:
  - path
change_summary:
  - string
verification:
  - command: string
    exit_code: integer
    result: string
focus:
  - correctness
  - regression
  - tests
```

#### 输出协议

```yaml
verdict: approve | changes_required | inconclusive
blocking_issues:
  - id: string
    evidence: string
    recommendation: string
important_issues: []
minor_issues: []
verification_gaps: []
```

Reviewer 不直接改代码。发现阻断问题时，由 Orchestrator 修复并重新验证；必要时再次审查。

### 10.4 Prompt Manifest

每个 Prompt/Skill 记录：

```yaml
schema_version: 1
assets:
  - id: orchestrator.zh
    role: system_prompt
    source_project: clean-room
    source_version: "1.1.0"
    source_commit: null
    source_path: assets/prompts/orchestrator.zh.md
    license_status: project-owned
    content_sha256: "..."
    resolved_sha256: "..."
    load_target: reasonix.system_prompt_file
    install_path: .reasonix/omr/orchestrator.zh.md
    modifications: []
    dependencies:
      - reasonix.delivery
      - reasonix.todo_write
      - reasonix.complete_step
```

字段定义：

- `content_sha256`：源资产本身 Hash；
- `resolved_sha256`：模板解析与组合后的最终安装内容 Hash；
- `license_status`：`project-owned`、`permissive-confirmed`、`permission-granted`、`review-required`；
- `load_target`：内容进入 Reasonix 的位置；
- `modifications`：相对来源的修改记录。

---

## 11. Standard 复杂编码工作流

### 11.1 触发条件

满足任一条件时使用 Standard 工作流：

- 修改跨越两个或以上模块；
- 根因尚不明确；
- 涉及公共 API、数据模型、权限、缓存或并发；
- 预计修改超过一个文件且存在回归风险；
- 用户明确要求完整实现、测试与审查。

单文件拼写、确定性配置值修改、纯格式调整等简单任务不应强制委派。

### 11.2 流程

```text
Understand
  ↓
Create/Update Native Todo
  ↓
Explore（按复杂度决定是否委派）
  ↓
Plan inside Reasonix Todo
  ↓
Implement by Orchestrator
  ↓
Verify
  ↓
Review by omr-review
  ↓
Fix blocking issues
  ↓
Re-verify
  ↓
complete_step with evidence
```

### 11.3 委派规则

#### 必须 Explore

- 跨模块调用链不清楚；
- 需要确定现有测试入口；
- 同名实现或多套路径可能共存；
- 用户报告与当前代码行为不一致。

#### 不应 Explore

- 已知文件与行级修改明确；
- 只改文案、拼写或静态值；
- 用户只要求解释，不要求修改；
- 读取当前文件即可得到全部事实。

#### 必须 Review

MVP 中，只要产生代码或配置文件修改，最终修改后必须调用 Reviewer；纯文档说明任务除外。

### 11.4 完成规则

只有同时满足以下条件才允许 `complete_step`：

- Todo 中当前步骤已完成；
- 所有阻断审查问题已关闭；
- 至少有一项与修改匹配的可执行验证；
- 验证结果包含命令、退出码和摘要；
- 最终修改后已重新运行必要验证；
- 剩余风险已明确说明。

---

## 12. 状态与数据模型

### 12.1 任务状态唯一事实源

MVP 不建立 OMR 任务数据库。状态归属如下：

| 状态 | 唯一事实源 |
|---|---|
| 当前任务与步骤 | Reasonix Todo |
| 当前步骤完成 | `complete_step` |
| 子 Agent 生命周期 | Reasonix `task` 会话 |
| 文件修改 | 工作区与 Git |
| 验证证据 | Reasonix 工具结果 + `complete_step` |
| OMR 安装资产 | `manifest.lock.yaml` |
| 缓存基准记录 | Cache Guard 报告 |

### 12.2 Todo 使用规则

Reasonix `todo_write` 的输入是完整列表，并覆盖当前视图。[R3]

OMR Prompt 必须要求：

- 每次更新保留仍有效的未完成项；
- 不把已取消步骤伪装为已完成；
- 不为简单任务创建冗长 Todo；
- 子 Agent 不直接成为父任务 Todo 的第二事实源；
- Reviewer 结论作为父任务的新信息处理。

### 12.3 安装状态

`manifest.lock.yaml` 记录：

- OMR 版本；
- Reasonix 基线；
- 安装时间；
- 每个写入文件的目标路径和 Hash；
- 原文件备份位置与 Hash；
- 配置补丁；
- 用户显式选择项。

卸载时逐项校验：

- 文件仍与安装 Hash 一致：自动移除或恢复；
- 文件已被用户修改：拒绝自动删除，输出冲突清单；
- 备份丢失：不覆盖现有文件，返回错误。

---

## 13. 相关上下文质量规则

### 13.1 总原则

> 最大化提供完整、相关、可信且当前有效的上下文。

“完整”不等于不加判断地堆积信息；1M 上下文是容量，不是使用目标。

### 13.2 相关性

内容至少满足一项才进入当前任务上下文：

- 直接影响需求或验收标准；
- 属于修改目标或调用链；
- 是相关测试、配置或接口定义；
- 能证明或反驳当前假设；
- 是审查或验证所需证据。

### 13.3 重复信息

- 完全重复内容只保留一个权威版本；
- 多个 Agent 的同一发现可合并，但保留来源；
- 不为了减少 Token 删除包含新增证据的近似内容。

### 13.4 过时信息

- 已失效计划不得继续作为当前指令；
- 旧诊断可保留为历史，但必须标注已被新证据替代；
- 文件内容以最新读取结果为准；
- 冲突时不得静默选择旧版本。

### 13.5 冲突信息

- 明确列出冲突双方和证据；
- 未验证前不得把任一方写成事实；
- 必要时通过工具或测试消解；
- 无法消解时在最终结果中保留不确定性。

### 13.6 敏感信息

以下内容默认不进入 Prompt Manifest、缓存报告和持久日志：

- API Key、Authorization Header；
- 私钥、证书私密部分；
- 明确标记为秘密的环境变量；
- 用户未授权持久化的个人数据；
- 与任务无关的仓库机密。

---

## 14. 缓存基准协议

### 14.1 目标

缓存基准回答两个不同问题：

1. **客户端确定性**：OMR 是否意外改变了本可稳定的前缀？
2. **服务端观测结果**：DeepSeek 实际返回的命中 Token 是否达到预期？

两者必须分开报告。

### 14.2 请求分类

每个请求归入一个类别：

| 类别 | 定义 | 是否进入稳态命中率 |
|---|---|---|
| Cold | 会话首个请求 | 否 |
| Warm Eligible | 同会话第 2 个及以后，且无重置 | 是 |
| Intentional Reset | 模型、Prompt、工具、Profile、压缩等显式变化 | 否，单列 |
| Cold-only Child | 子 Agent 会话只有一个请求 | 否，单列 |
| Non-observable | API 未返回完整 usage | 否，报告数量 |
| Unexpected Divergence | 无重置原因但前缀不再延续 | 发布失败 |

### 14.3 前缀确定性检查

对每个请求计算：

```text
system_prompt_sha256
canonical_tool_schema_sha256
static_config_sha256
messages_sha256
previous_messages_prefix_match
reset_reason
```

工具 Schema 使用规范化 JSON：

- 固定字段排序；
- 固定工具顺序；
- 不加入时间戳或随机值；
- 数字、布尔值和空值按统一规则序列化。

连续请求必须满足：

```text
current.messages 以 previous.messages 为逻辑前缀
```

允许追加上一轮 assistant、tool 与 user 消息；不允许无原因改写早期消息。

### 14.4 服务端指标

主指标按 Token 加权：

```text
steady_state_hit_rate =
  Σ prompt_cache_hit_tokens
  ─────────────────────────────────────────────────────
  Σ (prompt_cache_hit_tokens + prompt_cache_miss_tokens)
```

仅统计 `Warm Eligible` 请求。

同时报告：

- Cold hit/miss Token；
- Warm hit/miss Token；
- 父会话命中率；
- 每个子会话命中率；
- Cold-only Child 数量；
- Intentional Reset 数量与原因；
- Unexpected Divergence 数量；
- Non-observable 请求数；
- 请求数、中位数、最小值和最大值。

### 14.5 对照组

每个场景使用两个组：

- **Native**：同版本 Reasonix Delivery 模式，不安装 OMR；
- **OMR**：安装 OMR，其他条件相同。

必须固定：

- Reasonix 版本与 Commit；
- DeepSeek 模型；
- 仓库夹具 Commit；
- 初始 Git 状态；
- Profile；
- 权限与沙箱；
- 温度与模型参数；
- 任务文本；
- 网络与 API Endpoint；
- 单次运行最大轮数。

每个场景每组运行 5 次，报告中位数及范围。

### 14.6 发布门槛

#### 确定性硬门槛

- `Unexpected Divergence = 0`；
- 同一会话 System Prompt Hash 不变；
- 同一会话 Tool Schema Hash 不变；
- 无意外模型/Profile 切换；
- 所有重置都有明确原因。

任一不满足即发布失败。

#### 服务端观测门槛

- OMR 稳态 Token 加权命中率中位数，相比 Native 下降不超过 3 个百分点；
- 绝对命中率 95% 作为目标值，不作为脱离基线的唯一硬门槛；
- 当 Native 中位数 ≥90% 而 OMR <90% 时，发布失败；
- 当 Native 与 OMR 同时异常偏低时，先按 DeepSeek best-effort 特性重跑，不直接判定 OMR 失败；
- 重跑后仍异常，报告必须区分客户端前缀一致与服务端未命中。

### 14.7 基准报告样例

```yaml
run_id: cache-cross-module-omr-03
reasonix:
  version: desktop-v1.17.16
  commit: 464d494
model: deepseek-v4-flash
scenario: cross-module-bug
group: omr
requests:
  total: 18
  cold: 3
  warm_eligible: 15
  intentional_reset: 0
  unexpected_divergence: 0
cache:
  warm_hit_tokens: 812430
  warm_miss_tokens: 21470
  warm_hit_rate: 0.9743
prefix:
  system_prompt_hash_changes: 0
  tool_schema_hash_changes: 0
result: pass
```

---

## 15. 功能需求

### FR-01 项目级安装预览

`omr init --dry-run` 必须输出：

- 将创建、修改和备份的文件；
- 每项变更的 Diff；
- 配置冲突；
- Reasonix 版本检查；
- Prompt 许可证状态；
- 是否需要 `--compose-prompt`。

dry-run 不得写入任何文件。

### FR-02 安装与备份

`omr init` 必须：

- 验证项目目录；
- 验证 Reasonix 基线；
- 创建时间戳备份；
- 原子写入文件；
- 生成 `manifest.lock.yaml`；
- 安装失败时回滚本次已写入内容。

### FR-03 Prompt Manifest

构建和安装阶段均验证：

- 所有资产有唯一 ID；
- Hash 与内容一致；
- 来源与许可证状态非空；
- 依赖能力在当前 Reasonix 中可用；
- 安装顺序确定。

### FR-04 Orchestrator Prompt

Prompt 必须实现：

- 简单/复杂任务分类；
- 原生 Todo 使用；
- Explore 与 Reviewer 调用规则；
- 相关上下文规则；
- 验证后完成；
- 不修改固定前缀；
- 不创建第二状态源。

### FR-05 Explore Profile

`omr-explore` 必须：

- 只读；
- 输出结构化发现；
- 引用相关文件或符号；
- 区分事实、推断与未知；
- 不执行实现。

### FR-06 Reviewer Profile

`omr-review` 必须：

- 在最终修改后运行；
- 接收目标、验收标准、变更和验证证据；
- 返回固定 Verdict；
- 区分 Blocking/Important/Minor；
- 不直接修改文件。

### FR-07 Standard 工作流

必须支持：

- 简单任务直接完成；
- 复杂任务 Explore → Implement → Verify → Review → Fix → Re-verify；
- Reviewer 阻断问题闭环；
- `complete_step` 证据式完成。

### FR-08 Reasonix 原生状态集成

OMR 不创建独立 Todo 数据库。`omr doctor` 必须检查：

- Delivery 模式可用；
- `todo_write` 可用；
- `complete_step` 可用；
- `task` 与 Profile 可用；
- `omr-explore`、`omr-review` 已加载。

### FR-09 Cache Guard 静态校验

必须检测：

- System Prompt Hash 变化；
- Tool Schema Hash 变化；
- 消息历史前缀改写；
- 未声明的模型/Profile 变化；
- 动态值进入固定 Prompt。

### FR-10 Cache Guard 基准记录

必须记录：

- DeepSeek usage 中的 hit/miss Tokens；
- 请求类别；
- 父/子会话标识；
- 重置原因；
- 汇总报告；
- Native 与 OMR 对照结果。

### FR-11 Doctor

`omr doctor` 必须组合：

- `reasonix doctor capabilities` 结果；
- OMR 文件完整性；
- Manifest Hash；
- Prompt 冲突；
- Profile 加载；
- 基线版本；
- 敏感日志设置；
- 卸载可恢复性。

### FR-12 安全卸载

`omr uninstall --dry-run` 显示计划；`omr uninstall` 必须：

- 校验当前文件 Hash；
- 恢复被 OMR 修改的原文件；
- 删除仅由 OMR 创建且未被用户修改的文件；
- 对用户已修改文件拒绝自动删除；
- 保留冲突报告；
- 不删除非 OMR 资产。

---

## 16. 非功能需求

### 16.1 确定性

- 同一输入与版本必须生成字节一致的安装资产；
- 配置键、Manifest 条目与工具快照固定排序；
- 禁止时间戳进入 Prompt 内容；
- 时间戳只允许出现在备份目录和安装元数据。

### 16.2 安全

- 不记录 Authorization Header；
- 日志默认不保存原始代码和工具结果；
- 临时代理仅监听本机回环地址；
- 备份目录权限遵循项目与系统默认安全策略；
- OMR 不扩大 Reasonix 权限。

### 16.3 可靠性

- 安装写入采用临时文件 + 原子重命名；
- 中途失败自动回滚；
- 卸载遇到用户修改必须停止而不是覆盖；
- Manifest 损坏时只允许诊断，不允许自动删除。

### 16.4 兼容性

- 支持 macOS arm64/amd64；
- 支持 Linux arm64/amd64；
- 支持 Windows amd64；
- 路径处理不得假设 POSIX；
- 换行与权限差异必须进入自动测试。

### 16.5 性能

- OMR 安装后不引入常驻进程；
- 日常 Reasonix 启动开销仅来自静态 Prompt/Skill 加载；
- `omr init --dry-run` 在中等项目中目标完成时间 <2 秒，不含外部网络请求；
- Cache Guard 代理自身附加延迟目标 P50 <5ms、P95 <20ms（不含上游网络）。

### 16.6 可维护性

- Prompt 与 Go 代码分离；
- 每个需求对应至少一个自动化测试；
- 每个发布附 Manifest 变更与基准结果；
- 不使用未文档化 Reasonix 内部接口。

---

## 17. 可执行验收用例

### 17.1 统一执行规则

- 所有 LLM 行为测试运行 5 次；
- 除确定性测试外，行为类用例至少 4/5 通过；
- 确定性、安全、安装和卸载测试必须 5/5 通过；
- 每次失败保存任务输入、工具轨迹、变更、验证和判定原因；
- 不允许由人工“感觉正确”替代机器判定。

### AC-01 简单任务不得过度委派

**夹具：** `simple-fix`，单文件中存在一个明确拼写错误及对应测试。

**输入：** 指定文件与错误现象，要求修复并验证。

**通过条件：**

- 子 Agent 调用次数 = 0；
- 只修改预期文件；
- 指定测试退出码 = 0；
- 调用 `complete_step`；
- 完成证据包含测试命令与结果。

### AC-02 跨模块任务必须先探索

**夹具：** `cross-module-bug`，错误表现位于 API 层，根因位于认证缓存模块，存在一个会误导的同名函数。

**通过条件：**

- 第一次文件写入前调用 `omr-explore`；
- Explore 输出包含预期根因文件和真实调用路径；
- 不修改误导文件；
- 最终修改范围与夹具预期一致；
- 相关测试退出码 = 0。

### AC-03 研究结果汇总必须处理重复与冲突

**夹具：** `aggregation-conflict`，注入三个受控 Explore 输出：

- 输出 A 与 B 含一个重复发现；
- 输出 B 含一个独有有效发现；
- 输出 C 与 A 对关键事实相互冲突且证据不足。

**通过条件：**

- 重复发现只在计划中出现一次；
- 独有有效发现被保留；
- 冲突被显式标记；
- 未经验证的冲突结论不进入实现依据；
- Orchestrator 触发额外验证或保留不确定性。

### AC-04 最终修改后必须审查

**适用：** 所有产生代码或配置修改的夹具。

**通过条件：**

- Reviewer 调用发生在最后一次修改之后；
- Reviewer 输入包含变更文件、验收标准和验证结果；
- Reviewer 输出符合 Schema；
- 若 Verdict 为 `changes_required`，不得直接完成。

### AC-05 阻断问题必须闭环

**夹具：** Reviewer 固定返回一个可验证的 Blocking Issue。

**通过条件：**

- Orchestrator 修复对应问题；
- 修复后重新运行必要验证；
- 必要时重新审查；
- 只有 Blocking Issue 关闭后才调用 `complete_step`。

### AC-06 完成必须有证据

**通过条件：**

- `complete_step` 至少包含一项证据；
- 证据包含命令、退出码和摘要；
- 退出码非 0 时不得标记成功；
- 最终结果列出剩余风险或明确“无已知剩余风险”。

### AC-07 System Prompt 稳定

**场景：** 父会话连续 20 轮工具调用。

**通过条件：**

- System Prompt Hash 变化次数 = 0；
- 无动态时间戳、Todo 或 Git 状态进入固定 Prompt；
- 任何差异均导致测试失败并输出字节级 Diff 位置。

### AC-08 Tool Schema 稳定

**场景：** 父会话 + Explore + Reviewer 完整执行。

**通过条件：**

- 每个独立会话内部 Tool Schema Hash 变化次数 = 0；
- Agent 会话之间可以有预期差异，但必须由 Profile 定义解释；
- 无原因差异视为 `Unexpected Divergence`。

### AC-09 消息前缀不得被改写

**通过条件：**

- Warm Eligible 请求中，前一次消息链是当前消息链的逻辑前缀；
- 允许追加 assistant/tool/user 消息；
- 不允许修改早期消息文本、角色或工具调用 ID；
- 压缩或显式重置必须记录原因并排除稳态统计。

### AC-10 缓存对照基准

**通过条件：**

- 每个场景 Native 与 OMR 各完成 5 次；
- 确定性硬门槛全部通过；
- OMR 稳态 Token 加权命中率中位数相对 Native 下降 ≤3 个百分点；
- 符合第 14.6 节的绝对门槛规则；
- 报告可由原始 Hash 与 usage 记录重算。

### AC-11 安装与卸载可逆

**通过条件：**

- dry-run 无写入；
- 安装前后 Diff 与计划一致；
- 卸载后项目恢复到安装前 Hash；
- 用户修改 OMR 文件后，卸载拒绝自动删除并准确列出冲突；
- 失败安装不留下部分状态。

### AC-12 配置冲突不得静默覆盖

**夹具：** 项目已存在自定义 `system_prompt_file`。

**通过条件：**

- 默认安装失败；
- 输出冲突路径和解决方式；
- 只有显式 `--compose-prompt` 才生成组合文件；
- 组合内容顺序固定且进入 Manifest。

### AC-13 Prompt 许可证门槛

**通过条件：**

- `license_status=review-required` 的资产不得进入发布包；
- 所有发布资产均有来源和 Hash；
- CI 对未知许可证状态直接失败。

---

## 18. 风险与边界

### 18.1 Prompt 指令冲突

**风险：** Orchestrator、Reasonix Delivery 和 Agent Profile 可能包含重复或矛盾规则。

**应对：**

- 建立 Prompt 冲突测试；
- 明确优先级：Reasonix 安全/工具契约 > OMR Orchestrator > Agent Profile > 任务输入；
- 删除冲突和错误指令，但不按 Token 长度裁剪有效指令；
- 对每次 Prompt 更新运行行为回归。

### 18.2 上下文污染

**风险：** 过时、重复或不相关内容可能降低判断质量。

**应对：** 执行第 13 节相关性、重复、过时、冲突和敏感信息规则。

### 18.3 服务端缓存波动

**风险：** DeepSeek 缓存为 best-effort，实际命中可能受服务端构建和过期影响。[R6]

**应对：**

- 客户端确定性作为硬门槛；
- Native 对照、重复运行和分类统计；
- 不以单次绝对命中率下结论。

### 18.4 LLM 行为非确定性

**风险：** 委派和审查行为可能偶发偏离。

**应对：**

- 固定夹具与模型参数；
- 5 次运行、4/5 通过；
- 保存失败轨迹；
- 失败必须能映射到 Prompt 或产品规则。

### 18.5 许可证与来源

**风险：** oh-my-openagent 当前公开元数据未提供可自动确认的 SPDX 许可证。[R9]

**应对：**

- MVP 采用 clean-room 重写；
- 未完成许可证确认的原文不进入发布包；
- Prompt Manifest 与 CI 设硬门槛；
- 如后续获得明确许可，再单独评审直接迁移范围。

### 18.6 安装破坏用户配置

**风险：** 修改现有 Prompt 或 Reasonix 配置可能造成数据丢失。

**应对：** dry-run、显式冲突、原子写入、备份、Hash 安全卸载。

### 18.7 明确不列为独立风险的事项

Reasonix Go 版接口迭代不作为独立产品风险。自 Go 版发布以来，Skills、`runAs: subagent`、独立会话等核心形式保持连续，后续主要为增量能力扩展。项目仍执行常规版本兼容测试，但不为假设性的破坏性变更预先建设复杂 Adapter。

---

## 19. 里程碑与开发拆分

### M0：架构验证（进入开发的当前阶段）

#### 工作包

1. 冻结 Reasonix 基线与测试环境；
2. 建立 Prompt Manifest Schema；
3. 编写 clean-room Orchestrator、Explore、Reviewer 初稿；
4. 验证项目级 Skills 与 `system_prompt_file` 加载；
5. 实现最小安装、备份和卸载；
6. 实现 Cache Guard 请求记录与 Hash；
7. 建立三个固定夹具；
8. 完成许可证书面结论。

#### 退出门槛

见第 0.3 节。

### M1：MVP 功能完成

#### 工作包

- 完成 FR-01 至 FR-12；
- 完成 AC-01 至 AC-13；
- macOS/Linux/Windows CI；
- Native 与 OMR 首轮质量、缓存报告；
- 安装与卸载文档。

#### 发布条件

- 所有确定性、安全、安装用例 100% 通过；
- LLM 行为用例达到 4/5；
- 缓存门槛通过；
- 许可证门槛通过；
- 无上游修改。

### M2：质量扩展（不属于本 PRD MVP 承诺）

候选项：

- Oracle；
- Librarian；
- 并行只读研究；
- 更多语言 Prompt；
- 更多仓库夹具；
- 可视化报告；
- 用户级安装；
- 与上游已确认许可 Prompt 的迁移。

每个候选项须另行提交 Mini-PRD，不自动进入范围。

---

## 20. 开发任务建议

### Epic A：Installer

- A1：项目探测与基线检查；
- A2：dry-run Diff；
- A3：备份与原子写入；
- A4：Manifest Lock；
- A5：Hash 安全卸载；
- A6：跨平台路径测试。

### Epic B：Prompt Distribution

- B1：Manifest Schema；
- B2：Orchestrator Prompt；
- B3：Explore Profile；
- B4：Reviewer Profile；
- B5：Prompt 冲突测试；
- B6：许可证 CI 门槛。

### Epic C：Workflow Quality

- C1：简单/复杂任务路由；
- C2：Explore 触发规则；
- C3：Review 触发规则；
- C4：阻断问题闭环；
- C5：`complete_step` 证据规则；
- C6：行为轨迹判定器。

### Epic D：Cache Guard

- D1：透明代理；
- D2：请求脱敏；
- D3：Canonical Tool Schema；
- D4：消息前缀比较；
- D5：usage 指标收集；
- D6：Native/OMR 报告；
- D7：重置原因分类。

### Epic E：Fixtures & CI

- E1：simple-fix；
- E2：cross-module-bug；
- E3：aggregation-conflict；
- E4：5 次行为测试执行器；
- E5：三平台 CI；
- E6：发布质量报告。

---

## 21. 发布决策清单

在发布候选版本前，评审人逐项确认：

- [ ] Reasonix 基线版本与 Commit 已记录；
- [ ] 能力差异矩阵没有新增 D 类需求；
- [ ] Todo 唯一事实源仍为 Reasonix 原生状态；
- [ ] Prompt Manifest 全部资产可审计；
- [ ] 未知许可证资产为 0；
- [ ] System Prompt 与 Tool Schema 确定性测试通过；
- [ ] Native/OMR 缓存对照通过；
- [ ] 简单任务没有过度委派；
- [ ] 复杂任务经过探索、验证与审查；
- [ ] 安装和卸载可逆；
- [ ] 三个平台测试通过；
- [ ] 所有失败轨迹已归因或阻断发布。

---

## 22. 参考资料

### [R1] Reasonix Release v1.17.16

- URL: https://github.com/esengine/DeepSeek-Reasonix/releases/tag/desktop-v1.17.16
- 版本/标签: `desktop-v1.17.16`
- Commit SHA: `464d494`
- 发布日期: 2026-07-20
- 访问日期: 2026-07-21
- 用途: 基线版本；`fleet`、Profile、并发与 Tool Schema 冷启动说明。

### [R2] Reasonix Guide

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/docs/GUIDE.md
- 分支: `main-v2`
- 对照版本: `desktop-v1.17.16` / `464d494`
- 访问日期: 2026-07-21
- 用途: `system_prompt_file`、Skills、Delivery、Hooks、Doctor、权限与任务能力。

### [R3] Reasonix Tool Contract

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/docs/TOOL_CONTRACT.md
- 分支: `main-v2`
- 对照版本: `desktop-v1.17.16` / `464d494`
- 访问日期: 2026-07-21
- 用途: `todo_write`、`complete_step`、稳定工具代理与 Tool Schema 契约。

### [R4] Reasonix Specification

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/docs/SPEC.md
- 分支: `main-v2`
- 对照版本: `desktop-v1.17.16` / `464d494`
- 访问日期: 2026-07-21
- 用途: 独立会话、缓存稳定与上下文压缩原则。

### [R5] Reasonix Example Configuration

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/main-v2/reasonix.example.toml
- 分支: `main-v2`
- 对照版本: `desktop-v1.17.16` / `464d494`
- 访问日期: 2026-07-21
- 用途: 并发、写者限制、Skills、Hooks 和模型配置。

### [R6] DeepSeek Context Caching

- URL: https://api-docs.deepseek.com/guides/kv_cache/
- 文档类型: 官方动态文档
- Commit SHA: 不适用
- 访问日期: 2026-07-21
- 用途: 前缀匹配、hit/miss usage 字段、best-effort 与缓存过期说明。

### [R7] DeepSeek Models & Pricing

- URL: https://api-docs.deepseek.com/quick_start/pricing/
- 文档类型: 官方动态文档
- Commit SHA: 不适用
- 访问日期: 2026-07-21
- 用途: V4 Flash/Pro、1M 上下文、美元价格与价格可变说明。

### [R8] oh-my-openagent Release v4.19.0

- URL: https://github.com/code-yeongyu/oh-my-openagent/releases/tag/v4.19.0
- 版本/标签: `v4.19.0`
- Commit SHA: `14083b8`
- 发布日期: 2026-07-17
- 访问日期: 2026-07-21
- 用途: 当前项目形态、Agent Harness 能力参考。

### [R9] oh-my-openagent Repository Metadata

- URL: https://api.github.com/repos/code-yeongyu/oh-my-openagent
- 默认分支: `dev`
- License SPDX: `NOASSERTION`（查询时）
- 访问日期: 2026-07-21
- 用途: 许可证与来源风险判断。

---

## 附录 A：关键术语

| 术语 | 定义 |
|---|---|
| 稳定前缀 | 在同一会话中预期保持字节/结构一致、可被缓存复用的请求前部内容 |
| Warm Eligible | 同一会话第二个及以后、且未发生显式重置的请求 |
| Unexpected Divergence | 没有合法重置原因，但 System Prompt、Tool Schema 或历史前缀发生变化 |
| Prompt Manifest | 记录 Prompt/Skill 来源、Hash、许可证、目标位置和依赖的清单 |
| Native | 未安装 OMR 的同版本 Reasonix 对照组 |
| OMR | 安装了本 PRD 规定资产与工作流的 Reasonix 实验组 |
| Delivery | Reasonix 原生面向完整交付的工作模式 |
| Clean-room 重写 | 参考行为和公开能力描述，但不复制来源文本的独立实现 |

## 附录 B：最终范围确认

MVP 只回答一个产品问题：

> **在完全复用 Reasonix 原生 Runtime 的前提下，OMR 的 Prompt、角色与标准工作流，能否稳定提升复杂编码任务质量，同时不破坏客户端可控的 DeepSeek 缓存前缀？**

本 PRD 未列入 MVP 的能力，不得以“顺手实现”为理由进入开发。
