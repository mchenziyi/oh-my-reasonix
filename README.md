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
go run ./cmd/omr profile list
go run ./cmd/omr profile list --json
go run ./cmd/omr upgrade --dry-run
go run ./cmd/omr uninstall --dry-run
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

`[agent.<profile>]` 可为 OMR Profile 声明模型、附加 Prompt 文件和只读约束；`omr doctor` 会校验 Profile 名称、项目相对 Prompt 路径和字段格式，实际执行仍由 Reasonix 原生 Profile 负责。

使用 `omr profile list --json` 可以同时查看已安装 Profile 及其 `model`、`prompt_file`、`read_only` 配置覆盖。

`fixture.yaml` 使用 JSON（JSON 是 YAML 1.2 的有效子集），以保持 CLI 无外部运行时依赖。固定响应行为由本地 fake provider 或录制回放提供，真实 Provider 不参与固定断言。
带有 `replay` 结果的夹具可用 `--replay` 在本地确定性重放；没有回放结果的夹具会被跳过，仍可通过 `--results` 接入外部执行结果评分。`--run-tests` 应针对与项目目录匹配的 fixture 使用。
`--min-qualified-rate` 用于设置质量门槛，取值范围为 `0..1`，默认要求全部已评估夹具通过。
质量报告可通过 `--output path/to/quality-report.json` 保存，`--results` 外部结果模式同样适用。
Native/OMR 质量结果可用 `--native-results native.json --omr-results omr.json` 生成配对对照报告。
真实 Runtime 如有宿主提供的结构化 JSONL 事件日志，可通过 `--events path/to/events.jsonl` 接入证据评分；OMR 不会从人类可读 stdout 推断事件。

## 范围

当前不包含自定义 Reviewer、用户级安装、并行写入、动态模型路由或 Reasonix 上游修改。Reasonix 基线固定为 `desktop-v1.17.16` / `464d494`。
