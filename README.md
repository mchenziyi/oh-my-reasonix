# oh-my-reasonix

oh-my-reasonix（OMR）是 Reasonix 的项目级增强层：负责安装和升级、Prompt 组合、Profile 分发、Claude 配置兼容、质量门禁、成本报告和机器接口适配。

它不替代 Reasonix，也不复制 Reasonix 的 Session、Task、Hook、Todo 或权限状态机。OMR 把可复用的工作流约束和项目配置安全地安装到项目中，再由 Reasonix 负责实际执行。

## OMR 解决什么问题

直接使用 Reasonix 时，团队通常还需要自己维护：

- 项目级 Prompt 和规则；
- Explore、Research、Debug、Planner 等专用 Profile；
- 安装、升级、备份、回滚和卸载；
- Claude 配置迁移；
- 质量 Fixture、重试/停滞/Review 证据；
- 成本、Token 和运行结果报告；
- Session、Hook、Task 和结构化事件的只读查询。

OMR 将这些能力统一成可审计、可回滚、可自动测试的项目层。

## 核心能力

### 安装与升级

- 项目级 init、upgrade、uninstall；
- dry-run、冲突检测、备份和回滚；
- Prompt、Profile、Manifest 和 SHA256 校验；
- 配置迁移和升级漂移诊断；
- 不修改全局 PATH、API Key 或 Reasonix 二进制。

基本用法：

~~~bash
# 预览安装计划（只读）
omr init --project-dir . --dry-run

# 安装
omr init --project-dir .

# 升级（保留已有配置）
omr upgrade --project-dir . --dry-run
omr upgrade --project-dir .

# 备份位置：.reasonix/omr/backups/<sha>/
# 回滚：恢复备份中的 reasonix.toml，重新运行 omr init

# 卸载
omr uninstall --project-dir . --dry-run
omr uninstall --project-dir .
~~~

### Prompt 与 Profile

内置 Profile：

- omr-explore：只读探索代码、调用链和测试入口；
- omr-research：只读研究文档、API 和外部资料；
- omr-debug：只读定位错误根因；
- omr-planner：拆解阶段、风险和验收条件；
- omr-frontend：分析界面结构、交互和 UI 测试入口；
- omr-git：只读分析 Git 历史、差异和影响范围；
- omr-lsp：只读分析符号、引用和诊断入口。

支持：

- Profile 元数据、只读边界和工具声明；
- Category → Profile 路由；
- disabled、missing、project/builtin 状态；
- 模型和附加 Prompt 覆盖；
- omr profile list 人类和 JSON 输出。

### Subagent 一览

安装 OMR 后，Reasonix 中通常可以看到以下 Subagent：

| Subagent | 来源 | 用途 | 写入边界 |
|---|---|---|---|
| `explore` | Reasonix 内置 | 通用代码探索和事实收集 | 由 Reasonix 原生策略决定 |
| `research` | Reasonix 内置 | 通用资料、API 和外部信息研究 | 由 Reasonix 原生策略决定 |
| `review` | Reasonix 内置 | 代码 Review 和问题发现 | 只读 |
| `security-review` | Reasonix 内置 | 安全风险审查 | 只读 |
| `omr-explore` | OMR 项目级 | 只读探索代码路径、调用链和测试入口 | 只读 |
| `omr-research` | OMR 项目级 | 只读研究文档、API 和外部上下文 | 只读 |
| `omr-debug` | OMR 项目级 | 只读定位失败根因和最小修复方向 | 只读 |
| `omr-planner` | OMR 项目级 | 拆解执行阶段、风险和验收条件 | 只读 |
| `omr-frontend` | OMR 项目级 | 分析 UI 结构、交互和前端测试入口 | 只读 |
| `omr-git` | OMR 项目级 | 分析 Git 历史、差异和影响范围 | 只读 |
| `omr-lsp` | OMR 项目级 | 分析符号、引用和语言服务诊断 | 只读 |

实际可用列表以当前 Reasonix 版本和项目配置为准，可使用以下命令查看：

~~~bash
reasonix subagent list
omr profile list --project-dir . --json
~~~

OMR 不会替换 Reasonix 的内置 Subagent；项目可以通过 Category routing、disabled 配置和模型覆盖来调整 OMR Subagent 的使用方式。

### Claude 兼容层

支持只读导入：

- .claude/rules
- .claude/skills
- .claude/agents
- .claude/commands
- .claude/mcp.json
- .claude/hooks

所有导入支持 dry-run、冲突报告、敏感信息保护和失败回滚。Claude Hook 会转换为策略提示，并明确标注运行时语义无法等价保留。

### 质量与成本

- 离线 Fixture 和确定性 replay；
- Runtime、Native/OMR 配对报告；
- 失败分类、重试、停滞、Review 阻断和证据缺失；
- Token、成本、缓存和 readiness 指标；
- JSON Schema、快照和迁移校验；
- 预期失败 Fixture 与正常通过率分离统计。

### Reasonix 机器接口

在 Reasonix 提供公开机器接口后，OMR 可只读查询：

- Session list/status/show/recovery；
- Hook list/status；
- Task list/show；
- run --events-jsonl 结构化事件流。

OMR 不读取 Reasonix 私有目录或数据库，不从人类可读 stdout 猜测 Session 状态。

## 快速开始

### 一分钟安装

已有 Reasonix 项目（含 `reasonix.toml`）：

~~~bash
# 安装 OMR
go run github.com/mchenziyi/oh-my-reasonix/cmd/omr@latest init --project-dir .

# 验证安装
go run github.com/mchenziyi/oh-my-reasonix/cmd/omr@latest doctor --project-dir .

# 查看 Profile
go run github.com/mchenziyi/oh-my-reasonix/cmd/omr@latest profile list --project-dir .
~~~

从源码构建：

~~~bash
git clone https://github.com/mchenziyi/oh-my-reasonix.git
cd oh-my-reasonix
go build -o omr ./cmd/omr
./omr init --project-dir /path/to/your/project --dry-run
./omr init --project-dir /path/to/your/project
~~~

### init/upgrade/doctor/profile/run 示例

项目中已有 reasonix.toml 时：

~~~bash
# 预览，不写文件
go run ./cmd/omr init --project-dir . --dry-run

# 安装
go run ./cmd/omr init --project-dir .

# 验证
go run ./cmd/omr doctor --project-dir .
go run ./cmd/omr doctor --project-dir . --json

# 查看配置和 Profile
go run ./cmd/omr config validate --project-dir .
go run ./cmd/omr profile list --project-dir .
go run ./cmd/omr profile list --project-dir . --json

# 执行任务并记录结构化事件流
go run ./cmd/omr run --project-dir . --events-jsonl /tmp/events.jsonl --json "查询项目状态"
~~~

安装后，Reasonix 会读取生成的 OMR Prompt 和 Profile。OMR 不会自动启动或接管 Reasonix 客户端。

## 让 Reasonix 自己安装 OMR

将 INSTALL_PROMPT.md 交给正在运行的 Reasonix。它会读取安装文档，先执行 dry-run，再在确认后安装。

Raw URL：

~~~text
https://raw.githubusercontent.com/mchenziyi/oh-my-reasonix/main/docs/INSTALL_PROMPT.md
~~~

完整安装说明见 docs/INSTALL.md。

## 人工体验与 A/B 对照

想比较“安装 OMR”和“只用原生 Reasonix”的实际差异，请按[OMR 人工体验与 A/B 对照测试](docs/MANUAL_TEST_OMR_AB.zh-CN.md)执行。也可以分别使用[仅体验 OMR 的人工测试清单](docs/MANUAL_TEST_OMR_ONLY.zh-CN.md)和[不安装 OMR 的 Native 基线清单](docs/MANUAL_TEST_NATIVE_ONLY.zh-CN.md)。

## 常用命令

~~~bash
# 配置
omr config validate --project-dir .
omr config validate --project-dir . --json
omr config schema --project-dir .

# Profile
omr profile list --project-dir . --json

# Claude 导入
omr claude import --project-dir . --dry-run
omr claude import --project-dir .
omr claude commands --project-dir . --json

# Session / Hook / Task 只读查询
omr session list --project-dir . --json
omr session status <branch-id> --project-dir . --json
omr session recovery <branch-id> --project-dir . --json
omr hook doctor --project-dir . --json
omr task list --project-dir . --json
omr task show <task-id> --project-dir . --json

# 结构化事件流
omr run --project-dir . --events-jsonl /tmp/reasonix-events.jsonl --json "执行指定任务"
~~~

如果 Reasonix 不在 PATH，可显式指定：

~~~bash
omr doctor --project-dir . --binary /Applications/Reasonix.app/Contents/MacOS/reasonix
~~~

## 配置示例

配置文件位于项目的 .reasonix/omr/config.toml：

~~~toml
[runtime]
model = "deepseek-v4-flash"
max_steps = 20
timeout = "2m"
concurrency = 1

[agent.omr-research]
model = "deepseek-v4-flash"
prompt_file = "prompts/research.md"
read_only = true

[routing]
explore = "omr-explore"
research = "omr-research"
frontend = "omr-frontend"

[profiles]
disabled = "omr-debug"

# 可选；默认 disabled。OMR 只保存环境变量名称，不保存值。
[mcp.docs]
transport = "stdio"
command = "mcp-docs"
args = ["--mode", "read-only"]
capabilities = ["docs"]
enabled = false
env = ["DOCS_API_KEY"]
~~~

配置也支持 JSONC 和 TOML → JSONC 迁移：

~~~bash
omr config migrate --project-dir .
omr config schema --project-dir .
~~~

项目配置发现顺序为 `.reasonix/omr/config.jsonc`、`config.json`、`config.toml`；找到第一个后停止，不跨文件合并。

OMR 会拒绝绝对 Prompt 路径、路径越界、未知配置字段、非法 Profile ID 和指向 disabled Profile 的路由。

### 可选 Web/Docs MCP

OMR 支持 `stdio`、Streamable HTTP（`http`）和 legacy SSE（`sse`）配置的兼容性诊断，并识别 `docs`、`web`、`code-search`、`version-filter` 能力标签；其他标签报告为 `unknown`。MCP 默认不启用；启用后，`config validate` 和 `doctor` 会报告命令是否在 PATH、所需环境变量名称、网络/本地进程风险以及是否需要用户确认，但不会输出命令参数、远端 URL、凭证值或不必要的绝对路径。

OMR 不启动、下载或授权 MCP Server，也不复制 Reasonix 的 MCP 运行时。要让工具真正进入会话，仍需使用 Reasonix 原生命令注册相同 Server，例如：

~~~bash
reasonix mcp add docs mcp-docs --mode read-only
reasonix mcp add web --http https://example.com/mcp
reasonix mcp list
~~~

Reasonix 会负责项目配置发现和首次确认。`omr-research` 只在运行时实际暴露对应工具且用户已确认时使用；不可用时会降级为普通只读研究并报告限制。网络访问、第三方成本和凭证管理由用户负责。

## 质量验证

~~~bash
go test ./...
go vet ./...
go build ./...
go run ./cmd/omr benchmark quality --replay --min-qualified-rate 1
~~~

质量 Fixture 使用 JSON（也是 YAML 1.2 的有效子集），不依赖真实 Provider。Native/OMR 对照没有配对证据时会明确标记 unavailable，不会宣称 OMR 优于 Native。

## 安全与隐私边界

- 默认只写项目目录；
- dry-run、冲突和升级失败不会静默覆盖用户文件；
- 不读取 ~/.reasonix/projects、私有事件文件、数据库或内部锁；
- 不输出 API Key、Prompt 原文、Tool 参数/结果、绝对路径、PID 或 hostname；
- Claude MCP/Hook 导入会做兼容性和风险提示；
- 真实客户端验证需要用户明确授权。

## 开发

需要 Go 1.23 或更高版本：

~~~bash
go test ./...
go vet ./...
go build ./...
go run ./cmd/omr version
~~~

代码改动应同时运行 gofmt 和 git diff --check。测试使用临时目录，不依赖用户真实项目。

## 当前状态与路线

已完成 OMR-T01～T10，以及 INT-01～INT-05 自动化联调。

当前后续事项：

- Comment Checker：等待 Reasonix 官方 PR 合并后，再接入运行时事件和阻断闭环；
- Tmux/桌面实时面板：记录为 Reasonix 官方适配事项，OMR 不复制 UI/后台状态机；
- Grill Me：先评估为可选的方案质询 Skill，暂不默认集成；
- INT-06：等待 Reasonix 官方接口进入可用版本后进行真实客户端验证。

## v1.17.20 机器接口兼容状态

OMR 基于 Reasonix v1.17.20 的公开机器接口设计，当前兼容状态：

| 接口 | 状态 | 说明 |
|------|------|------|
| session list | ✅ 通过 | 只读查询 Session 列表 |
| session status | ✅ 通过 | 查询指定 Session 状态 |
| session show | ✅ 通过 | 查看 Session 详情 |
| session recovery | ✅ 通过 | Session 恢复信息 |
| session resume | ✅ 通过 | 恢复 Session 连接 |
| session export | ✅ 通过 | 导出 Session 事件 |
| hook list | ✅ 通过 | 查询 Hook 列表 |
| hook status | ✅ 通过 | 查询 Hook 状态 |
| hook doctor | ✅ 通过 | Hook 诊断（JSON/人类输出） |
| task list | ✅ 通过 | 查询 Task 列表 |
| task show | ✅ 通过 | 查看 Task 详情 |
| run --events-jsonl | ✅ 通过 | 结构化事件流（v1 schema） |
| event schema v1 | ✅ 通过 | 事件格式验证 |
| 旧格式兼容 | ✅ 通过 | 向后兼容旧事件格式 |
| 非零退出事件落盘 | ✅ 通过 | 失败事件写入日志 |
| 事件脱敏 | ✅ 通过 | 敏感信息自动过滤 |
| run_done / token 汇总 | ✅ 通过 | 运行完成和 Token 统计 |
| INT-06 真实客户端 | ⏳ pending | 需要 Reasonix 真实客户端验证 |

**注意**：以上状态基于 Mock 和本地 CLI 验证。INT-06（真实客户端验证）在 Reasonix 公开接口稳定后执行，当前保持 pending，不伪造通过。

## 常见错误与排查

### `reasonix.toml not found`
OMR init 要求项目根目录已存在 `reasonix.toml`。在空项目中新建该文件即可：
~~~bash
touch /path/to/project/reasonix.toml
omr init --project-dir /path/to/project
~~~

### `No OMR config found (project not yet configured)`
这是正常状态，不是错误。OMR 项目配置（`.reasonix/omr/config.toml`）是可选的。如需自定义 Profile 路由、模型或 MCP 配置，可手动创建或使用 `omr config schema` 生成 JSON Schema 后填写。

### `OMR manifest not found`
Profile 和安装追踪依赖 manifest。运行 `omr init` 或 `omr upgrade` 后 manifest 会自动生成。若已安装但仍缺失，重新运行 `omr upgrade --project-dir .`。

### `category "x" routes to disabled Profile "y"`
某个路由类别指向了已被禁用的 Profile。编辑 `.reasonix/omr/config.toml` 中的 `[profiles] disabled` 或 `[routing]` 配置。

### `omr: command not found`
使用 `go run` 方式，或 `go build -o omr ./cmd/omr && sudo mv omr /usr/local/bin/`。详细的安装方式见 [docs/INSTALL.md](docs/INSTALL.md)。

### Reasonix 二进制不在 PATH
显式指定 `--binary` 参数：
~~~bash
omr doctor --project-dir . --binary /Applications/Reasonix.app/Contents/MacOS/reasonix
omr session list --project-dir . --binary /path/to/reasonix
~~~

## 许可证

MIT
