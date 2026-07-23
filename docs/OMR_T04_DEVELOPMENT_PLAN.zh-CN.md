# OMR-T04：Profile 与 Category 体验开发计划

## 1. 目标

让 OMR 的 Profile 和 Category routing 更容易理解、配置、诊断和回归验证，确保用户能明确知道：

- 每个 Profile 负责什么；
- 输入、输出和只读边界是什么；
- 某个 Category 会路由到哪个 Profile；
- Profile 是否禁用、缺失、重复或被错误覆盖；
- 模型覆盖是否生效；
- 配置错误是否在写入或运行前被阻断。

本任务只增强 OMR 的配置、资产、CLI、Doctor 和测试，不复制 Reasonix 的 Agent、Task、权限或后台状态机。

## 2. 当前状态

当前已存在：

- 内置 Profile：omr-explore、omr-research、omr-debug、omr-planner、omr-frontend；
- [agent.<profile>] 模型、Prompt、只读等配置；
- [routing] Category → Profile；
- [profiles] disabled；
- omr profile list 及 JSON 输出；
- 配置校验、禁用路由冲突检查、Profile Hash 和安装冲突保护。

本阶段应优先复用现有 Manifest、Config、Doctor 和 Prompt Composer，不另造第二套 Profile 注册表。

## 3. 分步任务

### T04-01：统一 Profile 元数据与边界说明

为每个内置 Profile 补齐稳定元数据：

- id、显示名称和用途；
- 适用输入/任务类型；
- 预期输出；
- 是否只读；
- 允许的工具或操作边界（仅描述 OMR 能确认的能力）；
- 失败时的处理建议；
- Prompt/资产版本。

元数据应来自现有 Profile 资产或 Manifest，避免维护两份互相漂移的定义。omr profile list 人类和 JSON 输出都应能显示核心元数据。

验收：5 个内置 Profile 均有完整元数据；缺失元数据时 Doctor 明确报错；字段顺序和列表排序稳定。

### T04-02：Profile/Category Schema 与示例

1. 扩展现有配置 Schema，覆盖：
   - Profile ID 命名；
   - model、prompt_file、read_only 类型；
   - Category 名称和 Profile 引用；
   - disabled Profile 列表；
   - 可选的用途/边界元数据（若决定进入配置，必须说明项目级覆盖规则）。
2. 为 TOML 和 JSONC 分别增加合法、非法、迁移和未知字段测试。
3. 更新配置示例和 CLI --help，明确项目级配置优先级。
4. 未知字段、错误类型和非法 ID 必须失败，不静默忽略。

### T04-03：Category 路由诊断

增强 omr config validate 与 omr doctor：

- Category 指向不存在 Profile；
- Category 指向 disabled Profile；
- 重复或大小写不一致的 Category；
- Profile 到多个 Category 的合法情况；
- Category 路由循环或无法解析（如配置支持间接引用时）；
- 默认路由缺失时的明确行为。

报告必须同时提供人类输出和 JSON 输出，包含 category、目标 Profile、状态和修复建议。正常配置不得产生误报。

### T04-04：Profile 状态与模型覆盖体验

增强 omr profile list：

- 显示 builtin/project 来源；
- 显示 enabled/disabled/missing/conflict 状态；
- 显示关联 Category；
- 显示生效模型及其来源（默认、项目覆盖或运行时参数）；
- 显示 Prompt 文件和 Hash（不得泄漏不必要绝对路径）。

增加确定性排序和 JSON Schema/快照测试。模型覆盖只影响 OMR 生成的配置或运行参数，不修改 Reasonix 全局配置。

### T04-05：冲突、重复和禁用保护

覆盖以下场景：

- 项目 Profile 与内置 Profile 同名；
- 同一 Profile 被重复声明；
- Profile 文件存在但内容 Hash 被修改；
- disabled Profile 仍被 Category 路由；
- 禁用后显式选择该 Profile；
- 删除/升级时保留用户修改的 Profile；
- dry-run 显示完整计划且零写入。

遇到冲突必须阻断当前写入或升级，并保留现有文件；不得静默覆盖用户资产。

### T04-06：Profile 体验文档和质量回归

更新：

- docs/OMR_TODO_LATEST.zh-CN.md；
- README、安装配置文档和配置 Schema；
- OMR/OMO 差距矩阵；
- 每个 Profile 的使用示例和 Category 示例。

增加离线 Fixture/测试，至少覆盖：

- Profile 元数据完整性；
- Category 正常路由；
- disabled 路由阻断；
- missing/conflict Profile；
- 模型覆盖；
- JSON 与人类输出一致；
- 升级/卸载保留用户修改。

## 4. 推荐开发循环

1. 先写失败测试或 Fixture。
2. 做最小配置/CLI/资产修改。
3. 执行：

~~~bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
go run ./cmd/omr config validate --help
go run ./cmd/omr profile list --help
go run ./cmd/omr profile list --json --project-dir <temp-project>
go run ./cmd/omr doctor --json --project-dir <temp-project>
~~~

4. 对临时项目执行安装、修改配置、dry-run、冲突和升级 Smoke。
5. 检查 JSON 字段稳定、排序稳定、无动态绝对路径和敏感信息。
6. 更新文档后再提交，仅提交 OMR 源码、资产、测试和文档。

## 5. 明确禁止

- 不复制 Reasonix 原生 Profile、Agent、Task、权限或后台状态机；
- 不猜测 Reasonix 尚未公开的 Profile 字段；
- 不修改全局 PATH、API Key、用户级配置或 Reasonix 二进制；
- 不把 OMR 合成 Profile ID 冒充 Reasonix 原生 Agent ID；
- 不静默覆盖用户 Profile、Prompt 或配置；
- 保留被忽略的 omr 和 .reasonix/ 本地产物，不纳入提交。

## 6. 完成标准

- T04-01 至 T04-06 均有实现、测试和文档证据，或明确标记 BLOCKED；
- go test ./...、go vet ./...、git diff --check 和 CLI Smoke 通过；
- Profile 列表、Doctor、Config Validate 的人类/JSON 输出事实一致；
- 路由、禁用、缺失、冲突和模型覆盖行为有回归测试；
- 安装、升级、卸载不会静默破坏用户修改；
- 所有新增能力仍属于 OMR 配置与发行层，不依赖 Reasonix 私有状态。

## 7. 交给 Reasonix Agent 的执行指令

请严格按 T04-01 → T04-06 顺序执行。每一步先阅读现有 Config、Manifest、Doctor、Profile CLI 和测试，再写失败测试，最后做最小修改。完成后输出修改文件、测试命令及结果、配置兼容性变化、仍未实现的项目和建议的下一阶段任务。若需要真实 Reasonix 客户端或宿主私有接口才能验收，请停止并报告 BLOCKED，不要猜测或伪造结果。

