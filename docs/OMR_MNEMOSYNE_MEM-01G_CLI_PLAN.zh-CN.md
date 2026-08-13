# OMR Mnemosyne MEM-01G：只读编译 CLI 计划

- 阶段：MEM-01G
- 状态：🟡 `memory compile` 本地只读接入中；`index rebuild` 仍待独立输入契约
- 目标：把已有确定性 OKF 编译器暴露为可审计的 CLI 预览，不隐式读取 CURRENT、不写入 Generation、不引入墙钟。

## 一、命令契约

```text
omr memory compile \
  --request /path/to/okf-compile-request.json \
  --project-dir . \
  --scope project \
  --json
```

请求文件只包含 `OKFCompileRequest` 的显式输入：`index_policy_ref`、
`evaluation_time`、`derivation_inputs`、`revisions`、`evidence` 和可选
`base_generation`。CLI 从 `--project-dir/--global-dir` 打开对应已有 Store，严格解码后调用
`CompileOKF`，输出只包含编译器版本、Scope、输入数量、输出路径和 compiled hash。

约束：

- `evaluation_time` 必填且必须是 RFC3339；不读取 `time.Now()`；
- 请求 JSON 拒绝未知字段、路径穿越、非法 Hash、跨 Scope 和缺失 Fact；
- 命令只读，不创建 Store、不写入 `CURRENT`、Generation、Manifest 或 Prompt；
- 输出不包含完整 Prompt、命令、思考或凭据。

## 二、明确不做

`memory index rebuild` 不在本阶段复用 `compile` 命令。它需要独立定义“从哪一个固定
Generation/Manifest 重建、如何发布派生 Index、如何处理 CURRENT/CAS”的输入契约；在契约
冻结前保持未实现，避免产生第二事实源或隐式扫描。

## 三、验收

- 缺少 request、缺少 evaluation_time、非法 JSON、未知字段和 symlink 输入均稳定拒绝；
- 合法请求输出字节稳定，重复执行不产生任何 Store 文件变化；
- CLI 与库级测试覆盖 Scope、Hash、路径和零写入；
- `gofmt`、`git diff --check`、`go test`、`go vet`、`go build`、Docs Gate 全通过。
