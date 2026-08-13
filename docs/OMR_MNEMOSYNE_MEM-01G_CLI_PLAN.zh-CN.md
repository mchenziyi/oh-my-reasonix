# OMR Mnemosyne MEM-01G：只读编译 CLI 计划

- 阶段：MEM-01G
- 状态：✅ `memory compile`、`index rebuild` 与 `index doctor` 只读命令均已实现；尚未提供派生 Index 发布事务
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

## 二、`memory index rebuild` 只读预览契约

```text
omr memory index rebuild \
  --request /path/to/index-rebuild-request.json \
  --project-dir . \
  --scope project \
  --json
```

请求必须显式提供 `evaluation_time`、`index_policy_ref`、非空
`derivation_inputs` 与非空 `revisions`。每个 Revision 必须携带完整
`memory_id/revision/content_sha256`，并且属于请求 Scope；`derivation_inputs`
必须覆盖派生所需的固定事实集合。CLI 只输出 Root/Local Index 预览、输入数量和确定性摘要。

约束：

- 只读加载固定 Policy/Revision/依赖事实，不扫描目录、不读取 CURRENT、不创建 Generation/Manifest；
- `evaluation_time` 必填且不回退 `time.Now()`；
- 输入顺序规范化后结果字节稳定；非法 Hash、路径穿越、跨 Scope、缺失事实或不完整 Manifest 均 fail closed；
- 不写入 Index Fact；Index 仍是由规范事实派生的读取视图。

## 三、明确不做

本命令不负责发布派生 Index、更新 CURRENT/CAS、修复损坏数据或自动生成缺失事实；需要这些
能力时必须进入独立的 Generation 事务计划，避免把预览结果变成第二事实源。

## 四、验收

- 缺少 request、缺少 evaluation_time、非法 JSON、未知字段和 symlink 输入均稳定拒绝；
- 合法请求输出字节稳定，重复执行不产生任何 Store 文件变化；
- CLI 与库级测试覆盖 Scope、Hash、路径和零写入；
- `gofmt`、`git diff --check`、`go test`、`go vet`、`go build`、Docs Gate 全通过。

`memory index doctor` 接受 `--index` 与严格 request JSON，按固定 `index_policy_ref`
只读诊断一个 Index Tree；不修复、不读取 CURRENT、不写入任何 Store 文件。
