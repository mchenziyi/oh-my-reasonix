# oh-my-reasonix

oh-my-reasonix（OMR）是面向 Reasonix 的项目级 Prompt、Profile 和工作流发行层。当前实现覆盖 M0/MVP 的本地安装链路、Prompt Composer、`omr-explore` / `omr-research` / `omr-debug` Profile、Manifest、字段级卸载、Cache Guard 逻辑流分析和质量 Smoke 夹具。

## 开发

需要 Go 1.23 或更高版本：

```bash
go test ./...
go vet ./...
go run ./cmd/omr version
```

## 项目安装

在包含 `reasonix.toml` 的项目中运行：

```bash
go run ./cmd/omr init --dry-run
go run ./cmd/omr init
go run ./cmd/omr doctor
go run ./cmd/omr doctor --json
go run ./cmd/omr config validate
go run ./cmd/omr config validate --json
go run ./cmd/omr config schema
go run ./cmd/omr profile list
go run ./cmd/omr profile list --json
go run ./cmd/omr session resume --project-dir .
go run ./cmd/omr upgrade --dry-run
go run ./cmd/omr uninstall --dry-run
```

Doctor 默认从 PATH 查找 `reasonix`；如果不希望修改全局 PATH，可通过 `OMR_REASONIX_BIN=/Applications/Reasonix.app/Contents/MacOS/reasonix` 指定可执行文件。

如果恢复时提示 Session 被其他 Reasonix 进程占用，先关闭原窗口；需要保留原进程时可使用：

```bash
go run ./cmd/omr session resume --project-dir . --copy

# 导出指定 Session 的 Reasonix 恢复/冲突诊断包
go run ./cmd/omr session export --project-dir . --out /tmp/reasonix-session.zip <branch-id-or-session-path>
```

## 让 Reasonix 自己安装

可以把 [安装提示词](docs/INSTALL_PROMPT.md) 交给正在运行的 Reasonix；它会读取
[INSTALL.md](docs/INSTALL.md)，先执行 dry-run，再在确认后下载、校验并运行 OMR。
也可以直接使用安装文档的 Raw URL：

```text
https://raw.githubusercontent.com/mchenziyi/oh-my-reasonix/main/docs/INSTALL.md
```

已有 `agent.system_prompt_file` 或 `agent.system_prompt` 时，必须显式使用 `--compose-prompt`。非空用户 Prompt 会被写入生成文件，因此还需要显式确认：

```bash
go run ./cmd/omr init --compose-prompt --allow-persist-user-prompt
```

Release 二进制内嵌 Prompt/Profile 发行资产；本地开发时 CLI 会优先从仓库中的 `assets/` 读取，
也可通过 `OMR_ASSET_DIR` 显式指定资产目录。

## 基准 Smoke

CLI 安装链路 Smoke 测试（使用临时目录，不读取或修改真实项目）：

```bash
./tests/cli_smoke.sh
```

```bash
go run ./cmd/omr benchmark quality
go run ./cmd/omr benchmark quality --replay
go run ./cmd/omr benchmark quality --replay --run-tests \
  --fixtures benchmarks/fixtures/m0-explore-review-complete \
  --project-dir . --min-qualified-rate 1
go run ./cmd/omr benchmark cache --trace path/to/trace.jsonl
go run ./cmd/omr benchmark cache --native-trace native.jsonl --omr-trace omr.jsonl
```

质量基准也支持项目级 `.reasonix/omr/config.toml`；命令行显式参数优先。例如：

```toml
[quality]
fixtures = "benchmarks/fixtures"
min_qualified_rate = 1

[runtime]
metrics_dir = ".reasonix/omr/metrics"
max_steps = 20
timeout = "2m"

[agent.omr-research]
model = "deepseek-v4-flash"
prompt_file = "prompts/research.md"
read_only = true
```

当前内置 Profile 包括 `omr-explore`、`omr-research`、`omr-debug`、`omr-planner` 和 `omr-frontend`。其中 Planner 用于复杂任务的阶段拆分、验收条件和风险识别，Frontend 用于只读分析界面结构、交互和 UI 测试入口。

`[agent.<profile>]` 可为 OMR Profile 声明模型、附加 Prompt 文件和只读约束；`omr config validate` 与 `omr doctor` 会校验 Profile 名称、项目相对 Prompt 路径、文件存在性和字段格式，实际执行仍由 Reasonix 原生 Profile 负责。

使用 `omr profile list` 输出表格含 `PROFILE`、`SOURCE`、`STATUS`、`MODEL`、`CATEGORIES` 列；`--json` 输出额外含 `description`、`read_only_bool`、`allowed_tools`、`source`、`status`、`effective_model`、`model_source`、`prompt_short_hash`。

`source` 指示来源：`builtin`（内置 Profile）或 `project`（配置引用但未安装）。`status` 为 `enabled`、`disabled` 或 `missing`（项目配置引用但未安装）。

复杂项目可以在 `.reasonix/omr/config.toml` 中覆盖任务类别路由：

```toml
[routing]
frontend = "omr-frontend"
explore = "omr-explore"
```

执行 `omr upgrade` 后，路由会以稳定、可审计的附加段写入生成 Prompt；`omr config validate --json` 会同时输出 `categories`。

`omr config schema` 输出可供编辑器和 CI 使用的 JSON Schema；Schema 与解析器都会拒绝未知配置段或字段。若类别路由指向 `[profiles] disabled` 中的 Profile，`config validate` 和 `doctor` 都会阻止继续执行。

使用 `omr config validate --json` 时，`valid` 表示整体结果；失败详情统一放在 `errors` 数组中，`error` 保留首条错误以兼容旧调用方。

OMR 配置中的 `fixtures`、`metrics_dir`、`model` 和 `[agent.<profile>]` 的 `model`/`prompt_file` 支持 `$VAR` 或 `${VAR}` 环境变量展开；变量未设置时配置校验会失败。

配置支持 TOML 原生 `#` 注释以及行尾 `//` 注释；引号内的 URL 和路径内容会保留。

`fixture.yaml` 使用 JSON（JSON 是 YAML 1.2 的有效子集），以保持 CLI 无外部运行时依赖。固定响应行为由本地 fake provider 或录制回放提供，真实 Provider 不参与固定断言。
带有 `replay` 结果的夹具可用 `--replay` 在本地确定性重放；没有回放结果的夹具会被跳过，仍可通过 `--results` 接入外部执行结果评分。`--run-tests` 应针对与项目目录匹配的 fixture 使用。
`--min-qualified-rate` 用于设置质量门槛，取值范围为 `0..1`，默认要求全部已评估夹具通过。
质量报告可通过 `--output path/to/quality-report.json` 保存，`--results` 外部结果模式同样适用。
Native/OMR 质量结果可用 `--native-results native.json --omr-results omr.json` 生成配对对照报告。

真实 Runtime 基准默认串行执行；可通过 `--concurrency N` 或 `.reasonix/omr/config.toml` 的 `[runtime] concurrency = N` 并发执行多个夹具。若使用共享 `--events` 事件流，必须保持并发数为 1。

质量门禁可通过 `--max-cost` 或 `[quality] max_cost = 1.5` 设置总成本上限；默认值 `0` 表示不启用成本门禁。

不希望某个 OMR Profile 被路由时，可配置：`[profiles] disabled = "omr-debug, omr-research"`。OMR 会在生成 Prompt、Doctor 和 Profile JSON 中标记这些 Profile，但不会删除其文件。

质量报告的 `metrics` 还会汇总 `readiness_checks`、`readiness_blocks` 和 `readiness_recoveries`，用于观察停滞检测与自动恢复效果。
Native/OMR 对照报告还会输出这三项指标的 `_delta`，正值表示 OMR 相对 Native 增加，负值表示减少；对照模式同样应用 `--max-cost` 成本门禁。
真实 Runtime 如有宿主提供的结构化 JSONL 事件日志，可通过 `--events path/to/events.jsonl` 接入证据评分；OMR 不会从人类可读 stdout 推断事件。

## 范围

当前不包含自定义 Reviewer、用户级安装、并行写入、动态模型路由或 Reasonix 上游修改。Reasonix 基线固定为 `desktop-v1.17.16` / `464d494`。
