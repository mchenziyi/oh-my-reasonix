# oh-my-reasonix

oh-my-reasonix（OMR）是面向 Reasonix 的项目级 Prompt、Profile 和工作流发行层。当前实现覆盖 M0/MVP 的本地安装链路、Prompt Composer、`omr-explore` Profile、Manifest、字段级卸载、Cache Guard 逻辑流分析和质量 Smoke 夹具。

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

`fixture.yaml` 使用 JSON（JSON 是 YAML 1.2 的有效子集），以保持 CLI 无外部运行时依赖。固定响应行为由本地 fake provider 或录制回放提供，真实 Provider 不参与固定断言。
带有 `replay` 结果的夹具可用 `--replay` 在本地确定性重放；没有回放结果的夹具会被跳过，仍可通过 `--results` 接入外部执行结果评分。`--run-tests` 应针对与项目目录匹配的 fixture 使用。
`--min-qualified-rate` 用于设置质量门槛，取值范围为 `0..1`，默认要求全部已评估夹具通过。
质量报告可通过 `--output path/to/quality-report.json` 保存，`--results` 外部结果模式同样适用。
Native/OMR 质量结果可用 `--native-results native.json --omr-results omr.json` 生成配对对照报告。
真实 Runtime 如有宿主提供的结构化 JSONL 事件日志，可通过 `--events path/to/events.jsonl` 接入证据评分；OMR 不会从人类可读 stdout 推断事件。
质量回放和真实 Runtime 可通过 `--event-log path/to/omr-events.jsonl` 输出 OMR 自己的生命周期事件，记录每个夹具的开始、完成或失败；该日志使用稳定的 JSONL 协议。

## 范围

当前不包含自定义 Reviewer、用户级安装、并行写入、动态模型路由或 Reasonix 上游修改。Reasonix 基线固定为 `desktop-v1.17.16` / `464d494`。
