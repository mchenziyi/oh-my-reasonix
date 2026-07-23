# OMR-T03：Claude 兼容层收尾开发计划

## 1. 目标

在不复制 Claude/Reasonix 宿主运行时的前提下，补齐 OMR 的 Claude 配置导入能力，使 `.claude/commands`、Agent/Skill frontmatter、MCP 和 Hook 导入都有清晰的兼容性边界、冲突报告、dry-run 和回滚证据。

本任务只修改 oh-my-reasonix 仓库。不得读取宿主私有 Session/Hook 状态，不得把 Claude Hook 假装成 Reasonix 原生 Hook 执行。

## 2. 当前状态

已存在并保持兼容：

- `omr claude import|rules|skills|agents|mcp|hooks`；
- 项目级发现、dry-run、冲突检测、原子写入和失败回滚；
- `.claude/rules`、`.claude/skills`、`.claude/agents`、`.claude/mcp.json`、`.claude/hooks` 的基础导入；
- Claude Hook 当前转换为策略提示，并带有“不保证等价于运行时 Hook 执行”的免责声明。

本阶段不要重写上述流程，只补缺口和验证。

## 3. 分步任务

### T03-01：实现 `.claude/commands` 只读导入

1. 调查当前 Reasonix Profile/Skill 资产格式，选择 OMR 已支持且可解释的目标格式。
2. 增加 `.claude/commands/` 发现和导入：
   - 仅读取文本命令文件；
   - 保留命令名称、正文、来源和参数说明；
   - 生成合法、可审计的 OMR 目标资产；
   - 不执行命令正文，不展开 shell，不调用网络。
3. 对不支持的扩展名、空文件、非法路径和重复名称给出稳定报告。
4. 在 dry-run 中列出每个源文件、目标文件、转换类型和风险；冲突时零写入。
5. 写入失败时回滚本次全部变更。

验收：无命令目录时 NOOP；正常导入、重复命名、非法文件、dry-run、冲突和回滚均有测试。

### T03-02：Agent/Skill frontmatter Schema 校验

1. 先阅读当前 Reasonix Skill/Profile 的实际 frontmatter 约定，不凭空扩展字段。
2. 为导入的 Agent 和 Skill 增加严格但最小的 Schema 校验：
   - 必填字段；
   - 字段类型；
   - 未知字段处理；
   - `runAs`、只读声明和名称规范；
   - 正文与 frontmatter 分隔异常。
3. 报告必须指出源文件、字段名、错误原因和建议动作。
4. 兼容已有合法文件；非法文件不得部分写入。
5. 不把 Claude 私有运行时字段伪装成 Reasonix 能力。

验收：合法、缺字段、错误类型、重复名称、未知字段、恶意超长/异常 frontmatter 均有单测；失败时全量回滚。

### T03-03：MCP 兼容性报告

1. 保持 `.claude/mcp.json` 只读导入和敏感信息保护。
2. 在导入报告中按服务器列出：
   - 名称；
   - OMR 是否原样保留；
   - Reasonix 是否能直接消费（若无法确定，标记 `unknown`）；
   - 被省略或无法迁移的字段；
   - 命令、URL、环境变量引用等风险提示。
3. 不打印 token、密钥、完整环境变量值或命令中的敏感参数。
4. 兼容报告必须同时支持人类输出和稳定 JSON 输出；字段增加版本号或保持现有报告向后兼容。
5. dry-run 只生成报告，不写入目标文件。

验收：空配置、多个服务器、损坏 JSON、敏感字段、未知字段和冲突均有测试。

### T03-04：Hook 转换与语义丢失报告

1. 保持当前“转换为策略提示”的行为，不执行 Hook。
2. 报告每个 Hook：
   - 源文件；
   - 事件和 matcher；
   - 已保留的提示语义；
   - 无法保留的运行时语义（命令执行、阻断、环境修改、顺序保证等）；
   - 是否需要人工复核。
3. 生成文件必须包含清晰免责声明和原始来源标识。
4. 对脚本内容中的密钥、绝对路径和危险命令做脱敏或风险标记，不把敏感原文复制进报告。
5. dry-run、冲突、失败回滚和重复导入保持幂等。

验收：静态 Hook、带 matcher Hook、多事件 Hook、危险命令、敏感内容、损坏文件均有测试；明确不能宣称“Hook 已启用”。

### T03-05：统一导入报告与回滚门禁

1. 统一 rules/commands/skills/agents/mcp/hooks 的报告字段和状态：`planned`、`written`、`skipped`、`conflict`、`error`、`unknown`。
2. 人类输出和 JSON 输出表达同一事实；JSON 字段稳定、无动态绝对路径泄漏。
3. 任一资产失败时，整个批次恢复到导入前状态，保留备份和错误原因。
4. 重复执行同一导入必须 NOOP 或产生等价结果。
5. 不修改全局 PATH、API Key、用户目录或 Reasonix 二进制。

### T03-06：文档和质量回归

更新：

- `docs/OMR_TODO_LATEST.zh-CN.md`；
- README、`docs/INSTALL.md` 或 Claude 兼容文档；
- OMR/OMO 差距矩阵；
- CLI `--help` 和 JSON 示例。

新增离线 Fixture，至少覆盖：

- commands 正常导入与冲突；
- frontmatter 合法与非法；
- MCP 兼容性 unknown/风险脱敏；
- Hook 语义丢失和危险内容脱敏；
- 批量导入失败后的全量回滚。

## 4. 推荐开发循环

1. 每个子任务先写失败测试或 Fixture。
2. 做最小实现，保持现有 API 和文件格式兼容。
3. 执行：

```bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
go run ./cmd/omr claude --help
go run ./cmd/omr claude import --help
```

4. 对临时项目执行 dry-run、写入、重复执行、冲突和回滚 Smoke。
5. 检查报告中没有 token、密钥、完整环境变量值或不必要绝对路径。
6. 更新文档后再提交，仅提交 OMR 源码、资产、测试和文档。

## 5. 明确禁止

- 不执行 Claude command 或 Hook；
- 不实现 Claude/Reasonix 的后台任务、Session、权限或 Hook 状态机；
- 不读取 `~/.reasonix/projects`、桌面应用数据库或私有事件文件；
- 不把“策略提示转换”描述成“运行时 Hook 等价实现”；
- 不因测试困难而降低断言、静默跳过冲突或伪造兼容性结论；
- 保留工作树中被忽略的 `omr` 和 `.reasonix/` 本地产物，不纳入提交。

## 6. 完成标准

- T03-01 至 T03-06 均有实现、测试和文档证据，或明确标记 BLOCKED；
- `go test ./...`、`go vet ./...`、`git diff --check` 通过；
- 质量回放和 CLI Smoke 通过；
- dry-run 零写入、冲突零写入、失败全量回滚、重复导入幂等；
- 人类报告与 JSON 报告一致且无敏感信息泄漏；
- 不能实现的 Claude 私有语义必须进入兼容性报告，而不是假装支持。

## 7. 交给 Reasonix Agent 的执行指令

请严格按 T03-01 → T03-06 顺序执行。每一步先阅读现有实现和测试，再写失败测试，最后做最小修改。完成后输出修改文件、测试命令及结果、兼容性限制、仍未实现的项目和建议的下一阶段任务。若需要真实 Reasonix 客户端或宿主私有接口才能验收，请停止并报告 BLOCKED，不要自行猜测或伪造结果。

