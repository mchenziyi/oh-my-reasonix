# oh-my-reasonix 产品需求文档（PRD）

## v1.1.1 M0 开发评审稿

| 字段 | 内容 |
|---|---|
| 文档版本 | v1.1.1 |
| 文档状态 | M0 开发评审稿（GO） |
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

本项目具备产品与技术可行性，**同意进入 M0 架构验证阶段**。

v1.1.1 已消除 v1.1 中四个会导致验收互相冲突的阻断项：

1. 简单、低风险、确定性修改不再强制调用 Reviewer；Standard/复杂任务才强制审查。
2. 明确 `system_prompt_file` 会替换 Reasonix `DefaultSystemPrompt`，并规定固定、可审计的 Prompt 组合算法。
3. MVP 不再安装 `omr-review` Profile，而是复用 Reasonix 内置 `review` Profile，使 Delivery 的 `review_report` 宿主级证据链保持有效。
4. 新增独立的质量对照协议；M0 的三个夹具只验证链路，不足以支持“质量提升”的公开结论。

同时修正：

- 项目 Profile 安装路径为 `.reasonix/skills/<name>/SKILL.md`；
- Cache Guard 使用基准运行 ID、Prompt 指纹和消息前缀链重建逻辑流，不宣称能够直接读取 Reasonix 内部 Session ID；
- 卸载采用配置字段级三方合并，不再以整个 `reasonix.toml` 文件 Hash 作为唯一判据。

### 0.2 M0 开始前必须确认

以下两项属于开工前置条件：

- **许可证与来源策略确认**：oh-my-openagent 仓库元数据显示许可证为 `NOASSERTION`。在许可证未被人工确认前，MVP 只能采用行为研究、结构参考和 clean-room 重写，不得直接复制或再分发其 Prompt 原文。[R8][R9]
- **基线冻结**：所有首轮开发、测试和基准统一使用 Reasonix `desktop-v1.17.16`、Commit `464d494`；升级基线须通过变更评审。[R1]

### 0.3 M0 退出条件

M0 只有在以下条件全部满足后，才能进入 MVP 功能开发：

- 项目级安装、备份和字段级三方卸载流程可用；
- Prompt Composer 能生成字节确定的三段式组合文件，并验证 Reasonix 运行时追加顺序；
- `omr-explore` 能从 `.reasonix/skills/omr-explore/SKILL.md` 正确加载；
- Standard 工作流调用 Reasonix 内置 `review`，并产生 Delivery 可识别的结构化 `review_report`；
- 不出现 `omr-review` 与内置 `review` 的双重审查；
- 重复执行 `omr init` 幂等，Profile 同名和内置 `review` 遮蔽冲突均能阻止覆盖；
- Cache Guard 能在不修改请求体的前提下重建固定夹具中的逻辑请求流；
- 固定夹具不存在 `ambiguous_stream`，固定响应行为用例可通过 fake provider 或录制回放重放；
- System Prompt、Tool Schema 和消息前缀可以被确定性校验；
- 三个固定行为夹具和质量/缓存 Smoke 基准全部跑通；
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

如何在不重复实现 Reasonix 原生功能、不修改 Reasonix 上游、不过度限制上下文的前提下，为用户提供一套更成熟的复杂编码任务工作流，并通过配对基准验证它：

1. 在复杂任务上相对 Native 具有可测量的质量收益，并在简单任务上不劣化；
2. 没有制造不必要的子 Agent 和流程开销；
3. 没有破坏客户端可控的缓存前缀稳定性；
4. 可以安全安装、升级和卸载；
5. 可以通过固定夹具与统计协议复现结果。

### 2.2 当前版本需要避免的误区

OMR v1.1.1 明确不采用以下假设：

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

1. 提供一个稳定、完整的 Orchestrator Prompt。
2. 提供一个完整的只读 `omr-explore` 子 Agent Profile。
3. 复用 Reasonix 内置 `review` Profile，并通过 OMR 审查任务协议驱动结构化审查。
4. 提供一个可重复执行的 Standard 复杂编码工作流。
5. 复用 Reasonix Delivery、Todo、`complete_step`、权限和工具契约。
6. 提供项目级安装、预览、备份、升级和字段级安全卸载。
7. 提供 Prompt Manifest 与来源追踪。
8. 提供 Cache Guard，验证客户端可控的前缀稳定性。
9. 提供 Native/OMR 缓存与质量配对基准。
10. 通过固定测试夹具把任务分类、委派、审查、验证和完成转为机器可判定条件。

### 4.2 产品声明边界

在质量基准达到第 15.7 节的公开声明门槛前，产品只能表述为：

> OMR 旨在通过更完整的 Prompt、角色分工和验证纪律改善复杂编码任务质量。

不得对外声称“已经证明优于 Native Reasonix”或给出质量提升百分比。M0 三个夹具每组运行 5 次只用于验证测试链路与判定器，不构成产品质量结论。

### 4.3 MVP 非目标

以下内容明确不在 v1.1.1 MVP 中：

- 自定义 `omr-review` Profile；
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
- 为填满上下文而注入无关内容；
- 对上下文压缩场景做缓存归因（MVP 基准夹具必须控制在压缩阈值以下）。


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
| Benchmark Harness 最大子 Agent 并发 | 2（仅用于基准与 M0 Smoke；OMR 不写入用户配置） |
| Benchmark Harness 最大并行写者 | 1（MVP 不启用并行写入） |

### 5.2 外部假设

- Reasonix Go 版公开 Skills、子 Agent 和会话能力可用；
- Delivery 模式维持稳定工具表面与 `complete_step` 规则；
- DeepSeek API 返回缓存命中与未命中 Token；
- 服务端缓存命中存在波动，不能作为唯一确定性发布门槛；
- Reasonix Go 版自发布以来，相关核心扩展形式保持向后兼容；版本升级属于常规兼容性测试，不列为独立产品风险。

---

## 6. Reasonix 原生能力与 OMR 新增能力矩阵

### 6.1 分类定义

- **A｜原生直接使用**：Reasonix 已完整提供，OMR 不重复实现。
- **B｜配置封装**：Reasonix 已提供，OMR 只生成配置、静态内容或调用规则。
- **C｜OMR 实现**：Reasonix 不提供该产品层能力，OMR 自行实现。
- **D｜必须修改上游**：仅在公开接口无法满足时使用；MVP 中必须为 0 项。

### 6.2 能力差异矩阵

| 能力 | Reasonix 现状 | OMR 处理 | 分类 | MVP |
|---|---|---|---|---|
| Agent Loop / Provider | 已有完整运行时 | 直接使用 | A | 是 |
| `system_prompt_file` | 读取文件后替换 `DefaultSystemPrompt` | Prompt Composer 生成完整替代文件 | B+C | 是 |
| 运行时 Prompt 追加 | 自动追加用户决策、语言、工作区、Delivery、环境等固定块 | 不重复写入组合文件，仅验证顺序 | A | 是 |
| Markdown Skills | 原生支持目录型 Skill | 安装 `.reasonix/skills/omr-explore/SKILL.md` | B | 是 |
| `runAs: subagent` | 原生支持；Profile 正文是子 Agent 完整 System Prompt | 定义完整 `omr-explore` Prompt | B | 是 |
| 内置 `review` Profile | 原生存在 | Standard 工作流直接复用 | A+B | 是 |
| `review_report` | 只挂载到 `review`、`security-review`、`security_review` | 不创建 `omr-review`，避免绕过 Delivery 门槛 | A | 是 |
| 独立子 Agent 会话 | 原生支持 | 直接使用 | A | 是 |
| `task` 委派 | 原生支持 | Prompt 中定义触发与任务载荷规则 | B | 是 |
| `fleet` / 并行写入 | 原生支持 | MVP 不使用 | A | 否 |
| Delivery 模式 | 原生支持 | OMR 默认运行方式 | A | 是 |
| Tool Schema 契约 | 原生存在 | Cache Guard 记录规范化 Hash | A+C | 是 |
| 原生 Todo / `complete_step` | 原生支持 | 作为唯一任务状态与完成证据源 | A+B | 是 |
| 权限与沙箱 | 原生支持 | 不新增权限体系 | A | 是 |
| Native Doctor | 原生能力检查 | `omr doctor` 汇总 OMR 检查 | A+C | 是 |
| Cache usage | API 可返回 hit/miss Token | Cache Guard 分类、聚合与对照 | C | 是 |
| Prompt Manifest / 来源 | 无 OMR 发行清单 | OMR 实现 | C | 是 |
| Standard 工作流 | 有底层能力，无 OMR 规则 | OMR Prompt 与验收夹具实现 | C | 是 |
| 质量对照协议 | 无 OMR 专用协议 | OMR 实现 | C | 是 |
| 安装预览与备份 | 非 Reasonix 产品职责 | OMR 实现 | C | 是 |
| 字段级三方卸载 | 非 Reasonix 产品职责 | OMR 实现 | C | 是 |
| 内部 Session ID 暴露 | HTTP 请求无此字段 | 不依赖；重建 OMR 逻辑流 | - | 否 |
| Reasonix 上游修改 | 当前无必要 | 禁止进入 MVP | D | 否 |

### 6.3 边界结论

MVP 的新增内容限定为：

1. Prompt Composer、Orchestrator 与 Explore 内容；
2. 内置 Review 的 OMR 调用协议；
3. Prompt Manifest 与来源管理；
4. 项目级安装、备份、升级和字段级三方卸载；
5. Cache Guard 与缓存对照协议；
6. 质量配对基准和可执行验收夹具。

Reasonix Runtime、Todo、子 Agent、Review Report、权限、工具代理和会话均不由 OMR 重写。


---

## 7. MVP 产品范围

### 7.1 交付物

```text
oh-my-reasonix/
├── cmd/omr/
├── internal/
│   ├── install/                   # dry-run、配置三方合并、备份、卸载
│   ├── promptcompose/             # 三段式 System Prompt 组合
│   ├── manifest/                  # 资产来源、版本和 Hash
│   ├── doctor/                    # OMR + Reasonix 能力检查
│   ├── cacheguard/                # 透明代理、逻辑流重建、缓存报告
│   └── qualitybench/              # Native/OMR 配对质量判定
├── assets/
│   ├── prompts/
│   │   ├── reasonix-base-464d494.md
│   │   ├── orchestrator.zh.md
│   │   └── review-task-protocol.zh.md
│   ├── skills/
│   │   └── omr-explore/
│   │       └── SKILL.md
│   └── manifest.yaml
├── benchmarks/
│   ├── fixtures/
│   │   ├── simple-fix/
│   │   ├── cross-module-bug/
│   │   └── aggregation-conflict/
│   ├── cache-protocol.md
│   └── quality-protocol.md
├── tests/
└── docs/
```

### 7.2 安装目标

```text
<project>/reasonix.toml
<project>/.reasonix/omr/generated/system-prompt.md
<project>/.reasonix/omr/manifest.lock.yaml
<project>/.reasonix/omr/backups/<install-id>/
<project>/.reasonix/skills/omr-explore/SKILL.md
```

Reasonix 官方项目 Profile 使用 `.reasonix/skills/<name>/SKILL.md` 目录格式，不使用 `.reasonix/skills/<name>.md`。[R12]

### 7.3 运行形态

OMR 是独立 Go CLI。安装完成后日常运行仍使用 Reasonix：

```bash
omr init --dry-run
omr init
reasonix --profile delivery
```

OMR 不常驻、不替换 Reasonix 二进制。仅在显式基准时启动透明代理：

```bash
omr benchmark cache
omr benchmark quality
```


---

## 8. 核心产品与技术决策

### 8.1 仅支持项目级安装

MVP 不修改用户全局配置。所有写入均位于项目根目录，降低跨项目污染与卸载风险。

### 8.2 System Prompt 是完整替代文件，不是隐式叠加

冻结版本中，设置 `system_prompt_file` 后，Reasonix 直接返回该文件内容，不再自动包含 `DefaultSystemPrompt`；随后 Boot 层才追加 Output Style、User Decision Policy、Language Policy、Workspace、Delivery、Environment、Memory 与 Skill 等运行时区块。[R10][R11]

因此，OMR 必须生成一个**完整替代文件**，而不能假设运行模型是：

```text
Reasonix DefaultSystemPrompt + OMR Prompt
```

### 8.3 固定三段式 Prompt Composer

生成文件 `.reasonix/omr/generated/system-prompt.md` 的内容顺序固定为：

```text
1. Pinned Reasonix DefaultSystemPrompt（基线 Commit 464d494）
2. User Custom Prompt（不存在时为空字符串）
3. OMR Orchestrator Prompt
```

规范化算法：

```text
canonical_segments = [canonicalize(reasonix_base)]
if canonicalize(user_prompt) != "":
    canonical_segments.append(canonicalize(user_prompt))
canonical_segments.append(canonicalize(omr_orchestrator))
result = join(canonical_segments, "\n\n") + "\n"
```

`canonicalize` 固定执行：去除 UTF-8 BOM、把 CRLF/CR 统一为 LF、移除文件首尾空白行；不得改写段内字符或缩进。

用户段为空时不写入空段；Manifest 仍记录 `user_prompt.present=false`。组合文件不得加入时间戳、绝对路径、Hash 注释或其他动态元数据。来源、Hash 和段边界只记录在 Manifest，不进入模型 Prompt。

Reasonix Boot 后续自动追加的固定策略不得复制进组合文件，否则会重复注入。[R11]

### 8.4 `--compose-prompt` 冲突与来源规则

OMR 必须同时检查：

- `[agent].system_prompt_file`；
- `[agent].system_prompt`。

规则：

- 两者均为空：自动以空 User Segment 组合；
- 任一存在：默认停止并报告冲突；
- 用户显式执行 `omr init --compose-prompt`：读取当前有效用户 Prompt，作为第二段按 `canonicalize` 规则组合；
- 两者同时存在：按 Reasonix 实际优先级使用 `system_prompt_file`，但 dry-run 必须提示 inline 值被遮蔽；
- 若 `system_prompt_file` 已等于 Manifest 中 OMR 认领的生成路径：视为已安装状态，不再报告自身冲突；重复执行 `omr init` 必须保持幂等，不得递归组合已生成的 Prompt；
- 若 `.reasonix/skills/omr-explore/SKILL.md` 已存在且不属于当前 Manifest：停止安装并报告同名冲突，不覆盖用户文件；
- 只要 User Segment 非空，生成文件就会持久化用户 Prompt 内容；dry-run 必须明确显示这一点，并要求显式 `--allow-persist-user-prompt` 才允许写入；
- 不修改用户原 Prompt 文件，只把路径与 Hash 写入 Manifest。

### 8.5 Prompt 升级行为

Prompt 更新只通过显式命令发生：

```bash
omr upgrade --dry-run
omr upgrade
```

升级规则：

- Reasonix 基线 Prompt、用户 Prompt 或 OMR Prompt 任一 Hash 变化，都重新生成组合文件；
- 用户 Prompt 来源发生变化但未执行升级时，`omr doctor` 报告 `Prompt Source Drift`；
- 组合文件被用户直接修改时，升级和卸载进入冲突状态，不自动覆盖；
- Reasonix `DefaultSystemPrompt` 在新基线中变化时，必须展示段级 Diff，并要求 `--accept-reasonix-base-update`；
- Prompt 内容变化会使新会话冷启动，必须在升级报告中明确；
- 已运行会话不热替换 Prompt，用户需创建新会话。

### 8.6 Reviewer 复用内置 `review`

MVP 不安装 `omr-review`。

冻结版本只对名称为 `review`、`security-review`、`security_review` 的 Profile 挂载 `review_report` 工具。[R11] 为避免普通 YAML 审查绕过 Delivery 宿主门槛或形成双重审查，Standard 工作流固定调用：

```text
review(task=<OMR Review Task Brief>)
```

OMR Review Task Brief 必须包含目标、验收标准、最终变更、验证证据和重点风险。OMR 负责“如何向内置 Reviewer 提问”，Reasonix 负责 Reviewer Profile、`review_report` 工具和 Delivery 证据链。

若 `review` Profile 或结构化报告能力不可用，`omr doctor` 与 Standard 工作流必须失败，不允许静默回退到 `omr-review` 或普通文本审查。

若项目或用户级配置以同名自定义 Profile 遮蔽 Reasonix 内置 `review`，`omr doctor` 必须报告冲突并阻止 Standard 工作流；OMR 不覆盖用户的 `review` Profile。

### 8.7 Simple 免子 Agent 审查，但不免 Delivery 证据

Reasonix Delivery 对产生状态变更的任务仍要求先用 `todo_write` 建立可验证验收标准，并在修改后检查、验证和调用 `complete_step`。[R13]

因此，同时满足以下条件的任务可走 Simple 路径，**不调用任何子 Agent，但仍创建一个最小单项 Todo 并完成宿主证据链**：

- 修改目标和正确结果完全明确；
- 修改只涉及一个文件；
- 不涉及业务逻辑、公共 API、数据模型、权限、安全、依赖、构建、缓存或并发；
- 修改范围未在执行中扩大；
- 存在精确、可执行的验证方式。

典型任务包括拼写、注释、纯格式及不改变行为的静态文本修改。

Simple 的宿主流程至少包含：一项验收标准、修改后检查、精确验证、`complete_step` 和 Todo 完成。它豁免的是 Explore/Review 子 Agent，不是 Reasonix Delivery 本身。

任一条件不满足，或实现中发现范围扩大，必须升级为 Standard，随后执行 Explore（按规则）和内置 Review。

### 8.8 不自建任务状态

唯一事实源保持为：

```text
Reasonix Todo + complete_step
```

OMR 不监听 Todo 后复制到第二套数据库。

### 8.9 Cache Guard 的可观测边界

HTTP 代理无法天然读取 Reasonix 内部 Session ID，也不能可靠知道内部压缩原因。MVP 不伪造这些能力。

`omr benchmark` 每次运行启动独立 Reasonix 进程和独立代理，并记录：

- 基准 `run_id`；
- 原始请求体 Hash 与转发请求体 Hash；
- System Prompt 与 Tool Schema 指纹；
- 根据消息前缀链重建的 `logical_stream_id`；
- 根据已知 Prompt/Profile 指纹得到的 `stream_role`；
- `declared_reset`、`unexpected_divergence` 或 `unknown` 分类。
- `ambiguous_stream` 分类。

MVP 缓存夹具必须低于上下文压缩阈值。若出现无法归因的链断裂，一律计为 `Unexpected Divergence`，不得排除出发布统计。

如果一个请求同时匹配多个候选前序流，或多个流的请求体完全相同而无法安全区分，必须标记为 `ambiguous_stream`，不得强行合并，也不得计入 Warm Eligible。M0 夹具不得依赖歧义流；出现歧义时基准失败并保留原始记录。

### 8.10 不直接复制未确认许可的 Prompt

在 oh-my-openagent 许可证未被人工确认前：

- 可研究角色、行为和工作流；
- 可 clean-room 重写；
- 不直接复制、翻译或再分发原 Prompt 文本；
- 每个资产记录来源、修改和许可证状态。


---

## 9. 架构设计

### 9.1 总体架构

```text
┌────────────────────────────────────────────────────────────┐
│                         OMR CLI                            │
│ init / upgrade / doctor / uninstall / benchmark           │
└──────────────┬───────────────────────┬─────────────────────┘
               │                       │
               ▼                       ▼
┌──────────────────────────┐  ┌─────────────────────────────┐
│ Installer & Composer     │  │ Benchmark Harness           │
│ - Prompt composition     │  │ - one proxy per run         │
│ - TOML three-way merge   │  │ - logical stream rebuild    │
│ - manifest/backups       │  │ - cache + quality scoring   │
└──────────────┬───────────┘  └──────────────┬──────────────┘
               │                              │
               ▼                              ▼
┌────────────────────────────────────────────────────────────┐
│                     Reasonix Runtime                       │
│ Delivery / task / built-in review / Todo / complete_step  │
└───────────────────────────┬────────────────────────────────┘
                            ▼
                        DeepSeek API
```

### 9.2 安装后模型

```text
Configured System Prompt File（完整替代品）
├── Pinned Reasonix DefaultSystemPrompt
├── User Custom Prompt（可为空）
└── OMR Orchestrator Prompt

Reasonix Boot 固定追加
├── Output Style
├── User Decision Policy
├── Language Policy
├── Workspace / Delivery / Environment
└── Memory / Skills 等稳定区块

Project Profile
└── .reasonix/skills/omr-explore/SKILL.md

Review
└── Reasonix built-in review + review_report

Runtime State
├── Reasonix Todo
├── task/subagent 独立会话
└── complete_step evidence
```

### 9.3 无上游修改原则

MVP 必须满足：

- 不 fork Reasonix；
- 不调用 Reasonix 私有 Go 包；
- 不依赖内部数据库表或 Session ID；
- 不修改 Tool Schema；
- 不新增 Hook；
- 不修改会话格式。

若公开能力无法满足，暂停对应需求并提交独立 ADR，不得隐式扩张 MVP。


---

## 10. Agent 与 Prompt 设计

### 10.1 Orchestrator

#### 职责

- 把任务分类为 Simple 或 Standard；
- 对 Standard 任务维护 Reasonix Todo；
- 必要时委派 `omr-explore`；
- 在充分理解后执行修改；
- 运行与变更匹配的验证；
- 对 Standard 修改调用内置 `review`；
- 处理结构化 Blocking Issue；
- 使用 `complete_step` 提交证据后完成。

#### 禁止行为

- 对 Simple 任务机械创建子 Agent；
- 把 Simple 豁免用于逻辑、API、安全或跨模块变更；
- 在未探索关键调用链前修改跨模块代码；
- 创建 `omr-review` 或第二套审查证据；
- 用“看起来正确”替代验证；
- 修改固定 Prompt 或工具集合；
- 为节省 Token 删除关键上下文。

### 10.2 `omr-explore`

`omr-explore` 是唯一由 OMR 安装的 MVP 子 Agent Profile。

Reasonix Profile 正文会成为子 Agent 的**完整 System Prompt**，不会自动叠加简短默认 Prompt，因此 `SKILL.md` 必须自包含角色、边界、工具纪律和输出协议。[R12]

#### 存储与 Frontmatter

```yaml
---
name: omr-explore
description: Investigate code paths, tests, and root causes without modifying files
invocation: manual
runAs: subagent
read-only: true
allowed-tools: [read_file, grep, glob, ls, code_index, bash]
---
```

实际允许工具以冻结基线能力检查为准；`read-only: true` 必须在工具边界剥离写工具。

#### 输入协议

```yaml
task_id: string
goal: string
questions: [string]
scope:
  include: [path-or-module]
  exclude: [path-or-module]
known_context: [string]
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
- 不假装读过未读取的文件；
- 不执行或建议范围外修改。

### 10.3 内置 Review 的 OMR 任务协议

Standard 任务最后一次修改并完成验证后，Orchestrator 调用：

```yaml
profile: review
prompt:
  goal: string
  acceptance_criteria: [string]
  changed_files: [path]
  change_summary: [string]
  verification:
    - command: string
      exit_code: integer
      result: string
  focus: [correctness, regression, tests, security, scope]
```

要求：

- 调用发生在最后一次修改之后；
- 输入引用最终工作区状态，而不是旧 Diff；
- Reviewer 通过 `review_report` 返回结构化证据；
- `changes_required` 或 Blocking Issue 未关闭时不得完成；
- OMR 不解析自定义 YAML Verdict 代替宿主级报告。

### 10.4 Prompt Manifest

每个 Prompt、Profile 与协议模块记录：

```yaml
schema_version: 1
assets:
  - id: orchestrator.zh
    role: system_prompt_segment
    source_project: clean-room
    source_version: "1.1.1"
    source_commit: null
    source_path: assets/prompts/orchestrator.zh.md
    license_status: project-owned
    content_sha256: "..."
    install_target: .reasonix/omr/generated/system-prompt.md
    composition_order: 3
    dependencies:
      - reasonix.delivery
      - reasonix.todo_write
      - reasonix.complete_step
      - reasonix.profile.review
```

Manifest 还必须记录三段 Prompt 的源路径、源 Hash、最终组合 Hash和 Reasonix 基线 Commit。


---

## 11. Simple 与 Standard 工作流

### 11.1 分类

#### Simple

必须同时满足第 8.7 节全部条件。流程：

```text
Understand
  ↓
Create one-item Native Todo / acceptance criterion
  ↓
Edit
  ↓
Inspect changed result
  ↓
Exact Verify
  ↓
complete_step
  ↓
Mark Todo completed
```

不创建第二套流程状态，不调用 Explore，不调用 Review。

#### Standard

满足任一条件：

- 修改跨越两个或以上模块；
- 根因不明确；
- 涉及业务逻辑、公共 API、数据模型、权限、安全、依赖、缓存或并发；
- 修改范围可能扩张；
- 用户要求完整实现、测试与审查。

### 11.2 Standard 流程

```text
Understand
  ↓
Create/Update Native Todo
  ↓
Explore（按规则决定）
  ↓
Plan inside Reasonix Todo
  ↓
Implement by Orchestrator
  ↓
Verify
  ↓
Built-in review with OMR Review Task Brief
  ↓
Fix blocking issues
  ↓
Re-verify
  ↓
Re-review when final diff materially changed
  ↓
complete_step with evidence
```

### 11.3 Explore 规则

#### 必须 Explore

- 跨模块调用链不清楚；
- 需要确定现有测试入口；
- 同名实现或多套路径可能共存；
- 用户报告与当前代码行为不一致。

#### 不应 Explore

- 文件与行级修改明确；
- 读取一个目标文件即可获得全部事实；
- 用户只要求解释；
- 任务仍满足 Simple 全部条件。

### 11.4 Review 规则

- Simple：免 Review；
- Standard：只要产生代码或配置修改，必须调用内置 `review`；
- 纯解释、研究或不产生修改的任务不调用 Review；
- Standard 的最终 Diff 在 Review 后发生实质变化时必须重新审查；
- 不得同时调用内置 `review` 和任何 OMR 自定义 Reviewer。

### 11.5 完成规则

#### Simple

- 修改前已建立一项具体验收标准；
- 修改后已读取结果或检查 Diff；
- 至少有一项精确验证；
- `complete_step` 证据包含命令、退出码和摘要；
- Todo 在成功 `complete_step` 后标记完成。

#### Standard

除满足 Simple 的宿主证据要求外，还必须：

- 所有 Todo 步骤完成；
- 结构化 Blocking Issue 已关闭；
- 最终修改后重新验证；
- 最终 Diff 实质变化时重新审查；
- 剩余风险明确说明。


---

## 12. 状态与数据模型

### 12.1 任务状态唯一事实源

| 状态 | 唯一事实源 |
|---|---|
| 当前任务与步骤 | Reasonix Todo |
| 当前步骤完成 | `complete_step` |
| 子 Agent 生命周期 | Reasonix `task` 运行时 |
| 审查证据 | 内置 `review_report` |
| 文件修改 | 工作区与 Git |
| OMR 安装资产 | `manifest.lock.yaml` |
| 缓存记录 | Cache Guard 报告中的 OMR 逻辑流 |
| 质量结果 | Quality Benchmark 报告 |

### 12.2 Todo 使用规则

`todo_write` 提交完整列表并覆盖当前视图。[R3]

OMR Prompt 必须要求：

- 每次更新保留仍有效的未完成项；
- 不把取消项伪装为已完成；
- Simple 任务不创建冗长 Todo；
- 子 Agent 不成为第二事实源；
- Review 结论由父任务吸收。

### 12.3 安装 Manifest 与三方合并

安装时对每个配置键记录：

```yaml
path: agent.system_prompt_file
base_value: null
installed_value: .reasonix/omr/generated/system-prompt.md
current_value: <read-at-uninstall>
```

卸载逐键执行：

1. `current == installed`：恢复 `base`；若 base 不存在则删除该键。
2. `current == base`：视为用户已自行恢复，不操作。
3. `current` 同时不同于 `base` 与 `installed`：保留当前值并报告冲突。

不得用安装前整文件覆盖当前 `reasonix.toml`。未被 OMR 认领的键、注释和用户修改必须保留。

生成文件规则：

- 当前 Hash 等于 installed Hash：删除或恢复备份；
- 当前 Hash 已变化：保留文件并报告冲突；
- 原文件备份丢失：停止对应恢复，不覆盖当前文件。


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

分别回答：

1. OMR 是否改变了客户端可控的稳定前缀？
2. DeepSeek 实际返回的缓存命中结果如何？

### 14.2 运行隔离与逻辑流

每个 Native/OMR 重复运行使用：

- 独立 Reasonix 进程；
- 独立 Cache Guard 代理端口；
- 唯一 `run_id`；
- 固定模型、仓库、任务和权限。

代理不读取或生成 Reasonix Session ID。它使用以下规则创建合成 `logical_stream_id`：

1. 首个未匹配请求创建新流；
2. 后续请求若其 messages 以前一请求 messages 为逻辑前缀，则归入同一流；
3. Prompt/Profile 指纹用于标记 `orchestrator`、`omr-explore`、`review` 或 `unknown`；
4. 多个流不得仅凭连接复用关系合并；
5. 出现多个候选前序流或完全相同的并发起始请求时，标记 `ambiguous_stream`，不得猜测归属。

### 14.3 请求分类

| 类别 | 定义 | 进入稳态命中率 |
|---|---|---|
| Cold | 每个逻辑流首个请求 | 否 |
| Warm Eligible | 同流后续请求且前缀延续 | 是 |
| Declared Reset | 基准 Harness 明确改变模型/Prompt/Tools 后的新流 | 否，单列 |
| Cold-only Child | 子流只有一个请求 | 否，单列 |
| Non-observable | API 未返回完整 usage | 否，单列 |
| Ambiguous Stream | 无法唯一确定逻辑流归属 | 否；基准失败 |
| Unexpected Divergence | 同一预期流无声明原因但前缀断裂 | 否；发布失败 |

MVP 夹具必须低于 Reasonix 压缩阈值，因此不设置“推测为压缩即可排除”的例外。

### 14.4 确定性记录

每个请求记录：

```text
run_id
logical_stream_id
stream_role
raw_request_body_sha256
forwarded_request_body_sha256
system_prompt_sha256
canonical_tool_schema_sha256
messages_sha256
previous_messages_prefix_match
classification
```

硬要求：

```text
raw_request_body_sha256 == forwarded_request_body_sha256
```

工具 Schema 使用固定字段和工具顺序的规范化 JSON。代理不得改写正文、模型参数或工具顺序。

### 14.5 服务端指标

仅对 `Warm Eligible` 按 Token 加权：

```text
steady_state_hit_rate =
  Σ prompt_cache_hit_tokens /
  Σ (prompt_cache_hit_tokens + prompt_cache_miss_tokens)
```

分别报告 Orchestrator、Explore、Review 与总体指标；短子流只列数量，不混入父流结果。

### 14.6 对照组

Native 与 OMR 固定：

- Reasonix 版本与 Commit；
- DeepSeek 模型与参数；
- 仓库夹具 Commit 和初始 Git 状态；
- Delivery Profile、权限与沙箱；
- 任务文本、Endpoint、最大轮数；
- Benchmark Harness 最大子 Agent 并发为 2、最大并行写者为 1；这些限制只作用于基准运行，不写入用户配置；
- 运行顺序随机或交错，降低服务端时段偏差。

每个场景每组运行 5 次。

### 14.7 发布门槛

#### 客户端硬门槛

- `Unexpected Divergence = 0`；
- 代理转发正文 Hash 与原始正文 Hash 一致；
- 同一逻辑流 System Prompt 和 Tool Schema Hash 不变；
- `ambiguous_stream = 0`；
- 无未声明的模型/Profile 变化。

#### 服务端观测门槛

- OMR 稳态 Token 加权命中率中位数相对 Native 下降不超过 3 个百分点；
- Native ≥90% 而 OMR <90% 时失败；
- 两组同时异常偏低时重跑并报告 best-effort 波动，不以单次结果归因。

### 14.8 M0 可实现性验证

M0 必须证明：

- 固定夹具可通过 Prompt 指纹识别 Orchestrator、Explore 和内置 Review；
- Prefix-chain 算法能区分无歧义的并发或交错逻辑流，并对歧义流失败；
- 不依赖 Reasonix 内部 Session ID；
- 无法归因的链断裂会失败而不是被静默排除。


---

## 15. 质量基准协议

### 15.1 目标与声明边界

质量基准回答：在相同 Reasonix、模型、仓库和任务条件下，OMR 是否相对 Native 改善复杂任务结果，而不损害简单任务。

M0 的 3 个夹具 × 每组 5 次仅验证执行链路、数据采集和判定器，结果不得用于公开质量声明。

### 15.2 基准集

#### M0 Smoke 集

- `simple-fix`；
- `cross-module-bug`；
- `aggregation-conflict`。

每个 Smoke 夹具必须包含机器可读的 `fixture.yaml`，至少声明任务文本、允许/禁止路径、隐藏测试、回归测试和预期工作流事件。`aggregation-conflict` 还必须提供三个可重放的 Explore 输出，其中包含一条重复发现、一条独有发现和一条待验证冲突；不得依赖人工临时输入。

#### MVP 发布集

至少 24 个版本固定夹具，四类各不少于 6 个：

1. Simple 确定性修改；
2. 跨模块 Debug；
3. 行为型功能实现；
4. 高回归风险修改（配置、缓存、权限或数据模型）。

任务文本不得暴露隐藏测试、预期修改文件或判定脚本。

### 15.3 固定条件

Native 与 OMR 必须使用相同：

- Reasonix Commit、模型和推理参数；
- 仓库 Commit 与初始状态；
- 权限、沙箱和最大轮数；
- 用户任务文本；
- 隐藏测试与资源限制。

每个夹具每组运行 5 次，运行顺序按夹具交错。无法固定随机种子时，以“夹具 + 重复序号”作为配对单元。

需要固定返回值的行为用例（例如 AC-04 的 Explore 汇总和 AC-06 的 Blocking Issue）使用本地 OpenAI-compatible fake provider 或录制回放，确保输出可重复；真实 DeepSeek 只用于质量、缓存和 Smoke 运行，不承担固定响应断言。

### 15.4 机器可判定指标

#### 主要指标：Qualified Completion

一次运行同时满足以下条件才记为成功：

- 隐藏功能测试全部通过；
- 隐藏回归测试无新增失败；
- 未修改禁止文件；
- 任务要求的行为已实现；
- 未通过伪造、跳过或删除测试取得通过。

#### 次要指标

- 隐藏测试通过率；
- 任务完成率；
- 修改范围正确率；
- 新增回归数量；
- 首次修复成功率（Reviewer 修复循环前）；
- 最终修复成功率；
- 总轮数和完成时间，仅作效率指标，不替代质量。

### 15.5 修改范围判定

每个夹具定义：

```yaml
allowed_paths: [glob]
required_effects: [machine-check]
forbidden_paths: [glob]
hidden_tests: [command]
regression_tests: [command]
```

修改范围正确率以机器规则判断。出现无法自动判断的语义范围争议时，采用双盲人工评审，两名评审不知实验组别；不一致时由第三名裁决，并记录理由。

### 15.6 统计方法

报告：

- 每组运行数与缺失数；
- Qualified Completion 比例；
- 按夹具和重复序号配对的差值；
- Simple 与 Complex 分层结果；
- 以夹具为重采样单位的 paired bootstrap 95% 置信区间；
- 所有失败类型明细。

不得只挑选 OMR 获胜的夹具或删除失败运行。基础设施失败必须按预先定义规则重跑并保留原记录。

### 15.7 门槛

#### M0 退出

- 数据采集和判定器 100% 可重放；
- 三个 Smoke 夹具均能产出完整报告；
- 不要求证明统计显著的质量提升。

#### MVP 发布

- Simple 队列 Qualified Completion 的 OMR−Native 差值不得低于 -3 个百分点；
- Complex 队列 OMR 点估计高于 Native，且至少高 5 个百分点；
- OMR 新增关键回归数不得高于 Native；
- 修改范围正确率不得低于 Native 2 个百分点以上；
- 所有结果可由原始运行记录重算。

#### 公开“质量提升”声明

只有在至少 30 个 Complex 夹具、每组每夹具 5 次的独立评测中，Qualified Completion 差值的 paired bootstrap 95% 置信区间下界大于 0，才允许对外使用“已证明提升复杂任务质量”的表述。


---

## 16. 功能需求

### FR-01 项目级安装预览

`omr init --dry-run` 必须输出：文件计划、字段级 TOML Diff、Prompt 三段来源、配置冲突、Reasonix 基线、许可证状态、预期缓存冷启动、Profile 同名冲突和用户 Prompt 持久化提示；不得写文件。

### FR-02 安装、备份与原子写入

`omr init` 必须：

- 生成 Prompt 组合文件；
- 安装目录型 `omr-explore/SKILL.md`；
- 只修改声明的 TOML 字段；
- 记录 base/installed 值；
- 对已由当前 Manifest 认领的安装结果执行幂等更新，不递归组合生成文件；
- 对非 OMR-owned 的 `omr-explore` 同名 Profile 停止并报告，不覆盖；
- 使用临时文件与原子重命名；
- 失败时回滚。

### FR-03 Prompt Composer

必须实现第 8.3 节算法，并记录：

- Reasonix Base Prompt Hash；
- User Prompt 来源和 Hash；
- OMR Prompt Hash；
- 最终组合 Hash；
- 组合顺序和换行规范。

### FR-04 Prompt Manifest

所有发布资产必须具有来源、许可证状态、内容 Hash、安装目标、基线版本和依赖；`review-required` 资产不得进入发布包。

### FR-05 Orchestrator Prompt

必须实现 Simple/Standard 分类、原生 Todo、Explore 规则、内置 Review 规则、相关上下文规则、证据式完成和单一事实源。

### FR-06 Explore Profile

`omr-explore` 必须以 `.reasonix/skills/omr-explore/SKILL.md` 安装，正文为完整子 Agent Prompt，启用 `runAs: subagent`、`invocation: manual` 和 `read-only: true`。

### FR-07 内置 Review 集成

必须：

- 调用宿主的专用 `review(task=...)` 工具；
- 传递 OMR Review Task Brief；
- 验证 `review_report` 可用；
- 禁止 `omr-review` fallback；
- 避免一次 Standard 任务出现两套 Reviewer。

### FR-08 Simple / Standard 工作流

必须支持：

- Simple 使用单项 Todo 与 `complete_step`，但无 Explore/Review 子 Agent；
- 任务范围扩大时升级 Standard；
- Standard 执行 Explore（按需）→ Implement → Verify → Built-in Review → Fix → Re-verify；
- Blocking Issue 闭环。

### FR-09 Reasonix 原生状态集成

`omr doctor` 必须检查 Delivery、`todo_write`、`complete_step`、`task`、`omr-explore`、内置 `review` 和 `review_report`。

### FR-10 Cache Guard 静态校验

必须检测请求正文转发一致性、System Prompt Hash、Tool Schema Hash、消息前缀改写、未声明模型/Profile 变化和动态值进入固定 Prompt。

### FR-11 Cache Guard 逻辑流与报告

必须记录 `run_id`、合成 `logical_stream_id`、角色指纹、hit/miss Tokens、请求分类和 Native/OMR 结果；必须记录并阻断 `ambiguous_stream`；不得把合成 ID 表述为 Reasonix Session ID。

### FR-12 质量基准

必须实现隐藏测试、范围规则、Qualified Completion、Native/OMR 配对报告、分层结果和可重放原始记录。

### FR-13 Doctor

`omr doctor` 必须汇总：Reasonix 能力、Prompt Source Drift、组合 Hash、Profile 路径、Review 集成、内置 `review` 是否被同名 Profile 遮蔽、Manifest 完整性、卸载三方状态和敏感日志配置。

### FR-14 升级

`omr upgrade --dry-run` 显示 Prompt 段级变化和配置影响；基线 Prompt 变化需要 `--accept-reasonix-base-update`，任何用户修改冲突不得静默覆盖。

### FR-15 字段级安全卸载

`omr uninstall` 必须执行第 12.3 节三方规则；只恢复仍等于 OMR 安装值的键，不覆盖用户后续修改，不以整个 TOML 文件 Hash 决定是否可卸载。


---

## 17. 非功能需求

### 17.1 确定性

- 同一输入、版本和用户 Prompt 必须生成字节一致的安装资产；
- Prompt 三段组合、Manifest 与 Schema 固定排序；
- 时间戳不得进入模型 Prompt；
- 每个可观测差异必须可输出字节级 Diff。

### 17.2 安全与隐私

- 不记录 Authorization Header；
- 原始请求内容默认不持久化，只保留 Hash 与统计；
- 代理仅监听回环地址；
- OMR 不扩大 Reasonix 权限；
- 质量夹具不得包含真实用户机密；
- User Segment 非空时，dry-run 必须明确提示用户 Prompt 将写入生成文件和备份；未显式传入 `--allow-persist-user-prompt` 不得持久化；
- Manifest、缓存报告和日志只记录用户 Prompt 的来源与 Hash，不记录其正文；
- OMR 不自动修改用户的 `.gitignore`，但 dry-run 和安装结果必须明确列出可能包含用户 Prompt 的文件路径。

### 17.3 可靠性

- 写入采用临时文件 + 原子重命名；
- 安装失败自动回滚；
- TOML 使用支持注释与顺序保留的解析/编辑方式；
- 三方合并冲突时保留用户当前值；
- Manifest 损坏时只诊断，不执行破坏性卸载；
- Cache Guard 无法可靠分类时基准失败，不猜测归因。

### 17.4 兼容性

- macOS arm64/amd64、Linux arm64/amd64、Windows amd64；
- 路径不得假设 POSIX；
- 换行、文件权限和 TOML 格式进入自动测试；
- 仅支持冻结 Reasonix 基线，升级需显式验证。

### 17.5 性能

- OMR 安装后无常驻进程；
- `omr init --dry-run` 在中等项目中目标 <2 秒，不含网络；
- Cache Guard 代理附加延迟目标 P50 <5ms、P95 <20ms；
- 性能指标不得以牺牲请求正文一致性为代价。

### 17.6 可维护性

- Prompt 与 Go 代码分离；
- 每个需求至少一个自动化测试；
- 发布附 Manifest、缓存和质量报告；
- 不使用未文档化 Reasonix 内部接口；
- 所有产品声明必须映射到基准指标。


---

## 18. 可执行验收用例

### 18.1 统一执行规则

- LLM 行为测试运行 5 次，至少 4/5 通过；
- 确定性、安全、安装、升级和卸载测试必须 5/5；
- 每次失败保存输入、工具轨迹、Diff、验证、审查证据和机器判定原因；
- 不允许用人工“感觉正确”替代预定义判定；
- 质量基准按第 15 节单独判定。

### AC-01 Simple 任务不得调用子 Agent

**夹具：** `simple-fix`，单文件明确拼写错误。

**通过条件：**

- Explore 调用 = 0；
- Review 调用 = 0；
- 其他子 Agent 调用 = 0；
- 创建且仅创建一个最小 Todo 项；
- 第一次写入前已有具体验收标准；
- 修改后读取文件或检查 Diff；
- 只修改预期文件；
- 精确测试退出码 = 0；
- `complete_step` 在最后一次修改与验证之后成功；
- Todo 在 `complete_step` 后标记完成。

### AC-02 Simple 范围扩大必须升级

**夹具：** 表面为单文件修改，但读取后发现需改公共逻辑。

**通过条件：**

- 不继续使用 Simple 豁免；
- 创建/更新 Todo；
- 按 Standard 规则探索和审查；
- 最终报告说明升级原因。

### AC-03 跨模块任务必须先探索

- 第一次写入前调用 `omr-explore`；
- Explore 找到真实根因与调用链；
- 不修改误导文件；
- 隐藏测试通过。

### AC-04 研究汇总处理重复与冲突

- 由 `aggregation-conflict/fixture.yaml` 提供三个可重放的 Explore 输出，或由 fake provider 按同一协议返回；
- 重复发现只保留一次；
- 独有有效发现被保留；
- 冲突显式标记；
- 未验证结论不进入实现依据；
- 触发额外验证或保留不确定性。

### AC-05 Standard 使用内置 Review

**通过条件：**

- 调用 Profile 名严格为 `review`；
- 不存在 `omr-review` 调用；
- Review 发生在最后一次修改和验证之后；
- 输入包含目标、验收、最终变更和验证；
- 子 Agent 工具表面包含结构化 `review_report`；
- 一次审查只形成一条宿主级证据链。

### AC-06 Blocking Issue 必须闭环

- 由 fake provider 或录制回放让内置 Reviewer 返回一个可验证 Blocking Issue；
- Orchestrator 修复并重新验证；
- 最终 Diff 实质变化时重新 Review；
- Issue 未关闭前不得完成。

### AC-07 完成必须有宿主证据

- 所有产生修改的 Simple/Standard 任务均建立验收标准；
- `complete_step` 含命令、退出码和摘要；
- 非 0 退出码不得标记成功；
- Standard 额外存在结构化审查证据；
- 剩余风险明确。

### AC-08 Prompt 三段组合正确

**场景：** 无用户 Prompt、有 inline Prompt、有 Prompt 文件三种。

**通过条件：**

- 最终文件顺序为 Base → User → OMR；
- 无用户 Prompt 时省略 User 段，Manifest 记录 `present=false`；
- 组合结果符合第 8.3 节字节算法；
- 不重复包含 UserDecision、Language 或 Delivery 区块；
- Manifest 可重算最终 Hash。

### AC-09 Prompt 冲突与升级

- 现有 Prompt 默认使安装停止；
- `--compose-prompt` 才允许组合；
- 用户源 Prompt 改变后 Doctor 报 Drift；
- 基线 Prompt 更新需显式接受；
- 用户修改生成文件时不自动覆盖。

### AC-10 Profile 路径与完整 Prompt

- 文件路径严格为 `.reasonix/skills/omr-explore/SKILL.md`；
- Reasonix 能列出并调用 Profile；
- `runAs: subagent`、`manual`、`read-only` 生效；
- 子 Agent 不依赖隐式 DefaultSystemPrompt。

### AC-11 System Prompt 稳定

- 父逻辑流连续 20 轮，System Prompt Hash 变化 = 0；
- 无时间戳、Todo 或 Git 状态进入固定 Prompt；
- 差异输出字节位置。

### AC-12 Tool Schema 稳定

- 每个逻辑流内部 Tool Schema Hash 变化 = 0；
- Orchestrator、Explore、Review 的预期差异由角色指纹解释；
- 无原因差异失败。

### AC-13 消息前缀不得改写

- Warm 请求中上一请求 messages 是当前请求逻辑前缀；
- 允许追加；
- 不允许改写早期消息、角色或 Tool Call ID。

### AC-14 Cache Guard 不改请求并重建逻辑流

- 原始与转发请求体 Hash 100% 相等；
- 固定夹具中角色分类正确；
- 合成流不依赖连接或内部 Session ID；
- `ambiguous_stream = 0`；
- 无法归因链断裂计为 Unexpected Divergence。

### AC-15 缓存对照

- Native/OMR 每场景各 5 次；
- 客户端硬门槛全部通过；
- OMR 相对 Native 下降 ≤3pp；
- 报告可从原始记录重算。

### AC-16 质量基准链路

- M0 三个夹具均输出 Qualified Completion、隐藏测试、范围与回归指标；
- Native/OMR 配对完整；
- 失败运行不得丢弃；
- M0 报告明确标记“不可用于公开质量提升声明”。

### AC-17 TOML 字段级三方卸载

**场景 A：** 用户未改 OMR 键。恢复 base。

**场景 B：** 用户修改无关键。卸载保留该修改并恢复 OMR 键。

**场景 C：** 用户修改 OMR 所属键。保留当前值、报告冲突，不覆盖。

整个 `reasonix.toml` Hash 变化不得单独阻止场景 B 卸载。

### AC-18 安装与文件卸载可逆

- dry-run 无写入；
- 安装 Diff 与计划一致；
- 未修改生成文件可安全删除；
- 用户修改生成文件时保留并报告；
- 失败安装无部分状态。

### AC-19 Prompt 许可证门槛

- `review-required` 资产不得进入发布包；
- 所有资产有来源与 Hash；
- CI 对未知许可证失败。

### AC-20 安装幂等与 Profile 冲突

- 对同一项目连续执行两次 `omr init`，第二次只报告无变化，不递归组合已生成 Prompt；
- 已由 Manifest 认领的 OMR 文件允许按 Hash 更新；
- 非 OMR-owned 的 `omr-explore` 同名 Profile 使安装停止且不覆盖；
- 被用户修改的生成文件进入冲突状态，不自动覆盖；
- 项目或用户级自定义 `review` 遮蔽内置 Profile 时，Doctor 和 Standard 工作流均阻止继续。

### AC-21 用户 Prompt 持久化确认

- User Segment 非空且未传入 `--allow-persist-user-prompt` 时，dry-run 可以展示计划但安装不得写入；
- 传入该选项后，生成文件和备份路径明确列出用户 Prompt 可能存在的位置；
- Manifest、缓存报告和日志不包含用户 Prompt 正文；
- 卸载不删除用户原 Prompt 文件。


---

## 19. 风险与边界

### 19.1 Prompt 指令冲突

**风险：** Orchestrator、Reasonix Delivery 和 Agent Profile 可能包含重复或矛盾规则。

**应对：**

- 建立 Prompt 冲突测试；
- 明确优先级：Reasonix 安全/工具契约 > OMR Orchestrator > Agent Profile > 任务输入；
- 删除冲突和错误指令，但不按 Token 长度裁剪有效指令；
- 对每次 Prompt 更新运行行为回归。

### 19.2 上下文污染

**风险：** 过时、重复或不相关内容可能降低判断质量。

**应对：** 执行第 13 节相关性、重复、过时、冲突和敏感信息规则。

### 19.3 服务端缓存波动

**风险：** DeepSeek 缓存为 best-effort，实际命中可能受服务端构建和过期影响。[R6]

**应对：**

- 客户端确定性作为硬门槛；
- Native 对照、重复运行和分类统计；
- 不以单次绝对命中率下结论。

### 19.4 LLM 行为非确定性

**风险：** 委派和审查行为可能偶发偏离。

**应对：**

- 固定夹具与模型参数；
- 5 次运行、4/5 通过；
- 保存失败轨迹；
- 失败必须能映射到 Prompt 或产品规则。

### 19.5 许可证与来源

**风险：** oh-my-openagent 当前公开元数据未提供可自动确认的 SPDX 许可证。[R9]

**应对：**

- MVP 采用 clean-room 重写；
- 未完成许可证确认的原文不进入发布包；
- Prompt Manifest 与 CI 设硬门槛；
- 如后续获得明确许可，再单独评审直接迁移范围。

### 19.6 安装破坏用户配置

**风险：** 修改现有 Prompt 或 Reasonix 配置可能覆盖用户后续修改。

**应对：** dry-run、显式冲突、原子写入、Prompt 段级来源、TOML 字段级三方合并和生成文件 Hash 守卫。

### 19.7 Cache Guard 逻辑流歧义

**风险：** 仅凭消息前缀无法唯一区分完全相同的并发起始请求或发生分支的会话。

**应对：** 标记 `ambiguous_stream`，不合并、不计入 Warm；M0 夹具不得依赖歧义流，出现歧义即失败并保留原始请求记录。

### 19.8 明确不列为独立风险的事项

Reasonix Go 版接口迭代不作为独立产品风险。自 Go 版发布以来，Skills、`runAs: subagent`、独立会话等核心形式保持连续，后续主要为增量能力扩展。项目仍执行常规版本兼容测试，但不为假设性的破坏性变更预先建设复杂 Adapter。

---

## 20. 里程碑与开发拆分

### M0：架构验证（当前阶段）

#### 工作包

1. 冻结 Reasonix 基线与默认 Prompt 快照；
2. 实现 Prompt Composer 与 Manifest Schema；
3. 编写 clean-room Orchestrator、Explore、Review Task Protocol；
4. 验证目录型 Profile 加载；
5. 验证内置 `review` + `review_report` Delivery 集成；
6. 实现最小 TOML 三方安装/卸载与重复安装幂等性；
7. 实现 Cache Guard 请求透传、逻辑流重建和歧义失败；
8. 建立三个行为/质量/缓存 Smoke 夹具与可重放 `fixture.yaml`；
9. 接入确定性 fake provider/录制回放测试；
10. 完成许可证书面结论。

#### 退出门槛

见第 0.3 节、§20 的 M0 工作包，以及 AC-01 至 AC-21 中与这些工作包对应的用例。

### M1：MVP 功能完成

#### 工作包

- 完成 FR-01 至 FR-15；
- 完成全部验收用例；
- 建立至少 24 个质量夹具；
- macOS/Linux/Windows CI；
- 发布 Native/OMR 缓存与质量报告；
- 完成安装、升级和卸载文档。

#### 发布条件

- 确定性、安全与资产管理用例 100% 通过；
- 行为类用例至少 4/5；
- 缓存门槛通过；
- 第 15.7 节 MVP 质量门槛通过；
- 许可证门槛通过；
- 无上游修改。

### M2：能力扩展（不属于本 PRD MVP）

候选：Oracle、Librarian、并行只读研究、更多语言 Prompt、可视化报告、用户级安装、自定义 Reviewer（仅在宿主结构化报告可安全集成时）。

每项须另行 Mini-PRD。


---

## 21. 开发任务建议

### Epic A：Installer & Composer

- A1：项目探测与基线检查；
- A2：三段 Prompt Composer；
- A3：TOML 保格式字段补丁；
- A4：Manifest Lock 与备份；
- A5：字段级三方升级/卸载；
- A6：重复安装幂等性与 Profile 同名冲突；
- A7：用户 Prompt 持久化确认与敏感路径提示；
- A8：跨平台路径与换行测试。

### Epic B：Prompt Distribution

- B1：Reasonix Base Prompt 快照；
- B2：Orchestrator Prompt；
- B3：`omr-explore/SKILL.md`；
- B4：Review Task Protocol；
- B5：Prompt 冲突与 Drift 测试；
- B6：许可证 CI。

### Epic C：Workflow Integration

- C1：Simple/Standard 路由；
- C2：Explore 触发；
- C3：内置 Review + `review_report` Spike；
- C4：阻断问题闭环；
- C5：`complete_step` 证据；
- C6：行为轨迹判定器。

### Epic D：Cache Guard

- D1：每运行独立透明代理；
- D2：请求体一致性；
- D3：Prompt/Tool 指纹；
- D4：消息前缀流重建；
- D5：usage 指标；
- D6：Native/OMR 报告；
- D7：Unexpected Divergence 与 `ambiguous_stream` 诊断；
- D8：无歧义流、分支流和重复起始请求测试。

### Epic E：Quality Benchmark

- E1：隐藏测试协议；
- E2：范围与回归判定器；
- E3：Qualified Completion；
- E4：配对运行与 bootstrap 报告；
- E5：Smoke `fixture.yaml` 与可重放 Explore/Review 输出；
- E6：确定性 fake provider/录制回放执行器；
- E7：24+ 夹具集；
- E8：公开声明门槛检查。


---

## 22. 发布决策清单

- [ ] Reasonix 基线、DefaultSystemPrompt Hash 与 Commit 已记录；
- [ ] 能力矩阵 D 类需求为 0；
- [ ] Prompt 三段顺序与运行时追加无重复；
- [ ] `omr-explore` 路径和完整 Profile Prompt 正确；
- [ ] Standard 只复用内置 `review`，无双重审查；
- [ ] Todo 唯一事实源仍为 Reasonix；
- [ ] Manifest 全部资产可审计且未知许可证为 0；
- [ ] Cache Guard 不改请求体并能重建逻辑流；
- [ ] Native/OMR 缓存对照通过；
- [ ] Native/OMR 质量门槛通过；
- [ ] Simple 无过度编排，Standard 有验证与审查；
- [ ] TOML 字段级三方卸载通过；
- [ ] 三个平台测试通过；
- [ ] 对外产品声明与基准证据一致。


---

## 23. 参考资料

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


### [R10] Reasonix `ResolveSystemPromptForRoot` Implementation

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/464d494/internal/config/config.go#L1835-L1858
- Commit SHA: `464d494`
- 访问日期: 2026-07-21
- 用途: 证明 `system_prompt_file` 直接替换 `DefaultSystemPrompt`，而非自动叠加。

### [R11] Reasonix Boot Prompt Assembly and Review Gate

- URL（Prompt）: https://github.com/esengine/DeepSeek-Reasonix/blob/464d494/internal/boot/boot.go#L351-L370
- URL（Review）: https://github.com/esengine/DeepSeek-Reasonix/blob/464d494/internal/boot/boot.go#L1031-L1047
- Commit SHA: `464d494`
- 访问日期: 2026-07-21
- 用途: 运行时固定策略追加顺序；`review_report` 仅挂载到指定内置 Review 名称。

### [R12] Reasonix Subagent Profiles

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/464d494/docs/SUBAGENT_PROFILES.md
- Commit SHA: `464d494`
- 访问日期: 2026-07-21
- 用途: 项目 Profile 路径 `.reasonix/skills/<name>/SKILL.md`；Profile 正文是完整子 Agent System Prompt；`runAs: subagent` 与 read-only 语义。


### [R13] Reasonix Delivery Final Readiness

- URL: https://github.com/esengine/DeepSeek-Reasonix/blob/464d494/internal/agent/agent.go#L1554-L1658
- Commit SHA: `464d494`
- 访问日期: 2026-07-21
- 用途: Delivery 状态变更要求 acceptance criteria、修改后检查、验证和 `complete_step`；Simple 只能豁免子 Agent，不能跳过宿主证据链。

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


---

## 附录 B：最终范围确认

MVP 只回答一个产品问题：

> **在完全复用 Reasonix 原生 Runtime 的前提下，OMR 的 Prompt、角色与标准工作流，能否稳定提升复杂编码任务质量，同时不破坏客户端可控的 DeepSeek 缓存前缀？**

本 PRD 未列入 MVP 的能力，不得以“顺手实现”为理由进入开发。
