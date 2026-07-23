# OMR-T02：Prompt / 项目规则注入可验证性开发计划

## 1. 目标

让 OMR 的 Orchestrator Prompt 中关于项目规则收集、优先级和冲突处理的约定变成可回归验证的协议，避免“提示词写了规则，但无法证明运行时遵守了规则”。

本任务只增强 OMR 的 Prompt 资产、离线 Fixture 和验证能力，不在 OMR 内复制 Reasonix 的运行时规则引擎。

## 2. 当前状态与边界

### 已有能力

- `assets/prompts/orchestrator.zh.md` 已描述项目规则注入流程，包括根目录到目标文件的规则收集、就近规则优先和用户/系统/宿主优先级。
- `internal/promptcompose` 负责 Base/User/OMR 固定 Prompt 的确定性组合。
- OMR 已有质量 Fixture、回放和报告校验能力。

### 明确边界

- Reasonix 负责真实会话中的文件读取、规则加载和工具执行；OMR 不读取宿主私有状态。
- 不扫描 `~/.reasonix`、桌面应用内部目录或数据库。
- 不实现 `session`、`hook`、`task`、后台任务或恢复状态机。
- 如果某项验收必须依赖真实 Reasonix 客户端输出，应标记为 BLOCKED 并停止，不伪造运行时证据，也不擅自修改 Reasonix。

## 3. 分步任务

### T02-01：冻结规则协议与优先级

1. 审阅现有 Orchestrator Prompt，删除重复或互相矛盾的表述。
2. 明确并写成稳定协议：
   - 从项目根到目标文件逐级收集 `AGENTS.md`；更深层规则优先。
   - 在兼容范围内识别 `.reasonix/rules` 与 `.claude/rules`。
   - 明确 `AGENTS.md`、兼容规则、README 的适用范围。
   - 明确宿主系统规则、用户消息与项目规则冲突时的优先级。
   - 明确无法读取、格式错误和规则冲突时的处理方式。
3. 固定术语和输出要求，避免同一概念出现多个名称。
4. 不把时间戳、绝对路径、随机 ID、当前 Git 状态或动态 Hash 写入固定 Prompt。

验收：同一资产在不同机器、不同运行时间生成的 Prompt 字节一致；协议能被测试逐条断言。

### T02-02：补充 Prompt 资产（仅在确有缺口时）

1. 以 T02-01 的协议为准，做最小化 Prompt 修改。
2. 保持 Base → User → OMR 的组合顺序和现有兼容性。
3. 不引入要求 Reasonix 尚未提供的 CLI/API；不得声称 OMR 已实现宿主侧行为。

验收：现有 Prompt 组合测试、Manifest/Hash 校验和安装升级流程全部通过。

### T02-03：增加离线规则 Fixture

在 `benchmarks/fixtures/` 下增加最小、可读、无真实项目内容的 Fixture，至少覆盖：

- `rules-precedence`：根规则与嵌套规则的优先级。
- `rules-conflict`：同一约束冲突时的决策和报告。
- `rules-compatibility`：`AGENTS.md`、`.reasonix/rules`、`.claude/rules` 的来源标识。
- `rules-boundary`：越界路径、不可读文件和格式错误的安全处理。

Fixture 必须声明输入、期望规则来源/顺序、允许的结果和禁止的结果。不得把模型猜测当作事实；若当前回放器无法表达某项断言，先扩展最小 Fixture 字段并保持向后兼容。

### T02-04：实现确定性验证器与报告字段

1. 优先复用现有 qualitybench Fixture/Replay/Report 结构，不新增平行格式。
2. 仅在必要时增加规则来源、优先级、冲突和越界处理的断言字段。
3. 对缺失证据、未知来源和不合法顺序返回明确失败，而不是静默通过。
4. 报告中区分：Prompt 协议静态检查、离线回放检查、真实宿主行为（若无接口则明确 `blocked`）。

验收：错误顺序、缺少来源、越界读取和伪造证据均能稳定失败；旧 Fixture 不回归。

### T02-05：增加 Prompt 稳定性回归测试

至少覆盖：

- 同一输入连续组合多次结果完全相同。
- Prompt 不包含绝对工作区路径、时间戳、随机值和未声明的环境变量值。
- 规则协议修改后，Manifest/生成文件 Hash 能正确更新；未修改时升级为 NOOP 或保持等价结果。
- 配置冲突、损坏 Manifest 和回滚路径仍按现有语义工作。

### T02-06：同步文档与任务状态

更新以下文档中的状态和命令示例：

- `docs/OMR_TODO_LATEST.zh-CN.md`
- README 或安装/质量基准文档（以仓库现有入口为准）
- OMR/OMO 差距矩阵（如仍存在独立文件）

文档必须区分“OMR 已实现”“离线可验证”“需要 Reasonix 官方接口”“需要人工客户端验证”。

## 4. 推荐开发循环

1. 先为当前缺口写失败测试或 Fixture。
2. 做最小 Prompt/验证器修改。
3. 执行：
   ```bash
   gofmt -w <changed-go-files>
   git diff --check
   go test ./...
   go vet ./...
   go run ./cmd/omr benchmark quality --replay --min-qualified-rate 1
   ```
4. 如新增了配对回放 Fixture，再执行：
   ```bash
   go run ./cmd/omr benchmark quality --paired --min-qualified-rate 1
   ```
5. 检查生成文件和 Manifest Hash，更新文档，最后只提交 OMR 源码、资产、测试和文档。

## 5. 完成标准

- T02-01 至 T02-06 均有代码、测试、文档证据，或明确写出 BLOCKED 原因。
- `go test ./...`、`go vet ./...`、`git diff --check` 和质量回放通过。
- 规则协议检查可重复、无网络依赖、无真实用户项目数据。
- 不新增宿主私有接口猜测，不修改全局配置、API Key、Reasonix 二进制或用户级 PATH。
- 当前工作树中的 `omr` 二进制和 `.reasonix/` 本地产物保持忽略，不纳入提交。

## 6. 交给 Reasonix Agent 的执行指令

请严格按 T02-01 → T02-06 顺序执行。每一步先阅读现有代码和测试，再提交最小修改；不要重新实现 Reasonix 宿主功能。若验收依赖真实客户端、私有 Session/Hook API 或用户授权，请停止并报告 BLOCKED 项，不要伪造通过结果。完成后输出：修改文件、测试命令及结果、仍未实现的项目，以及建议的下一阶段任务。

