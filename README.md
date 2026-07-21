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

已有 `agent.system_prompt_file` 或 `agent.system_prompt` 时，必须显式使用 `--compose-prompt`。非空用户 Prompt 会被写入生成文件，因此还需要显式确认：

```bash
go run ./cmd/omr init --compose-prompt --allow-persist-user-prompt
```

CLI 默认从仓库中的 `assets/` 读取发行资产；安装为独立二进制时可通过 `OMR_ASSET_DIR` 指向该目录。

## 基准 Smoke

```bash
go run ./cmd/omr benchmark quality
go run ./cmd/omr benchmark cache --trace path/to/trace.jsonl
```

`fixture.yaml` 使用 JSON（JSON 是 YAML 1.2 的有效子集），以保持 CLI 无外部运行时依赖。固定响应行为由本地 fake provider 或录制回放提供，真实 Provider 不参与固定断言。

## 范围

当前不包含自定义 Reviewer、用户级安装、并行写入、动态模型路由或 Reasonix 上游修改。Reasonix 基线固定为 `desktop-v1.17.16` / `464d494`。
