# OMR Mnemosyne MEM-03C-02B：Episode Card 与 Episodic Index 编译器

- 状态：✅ 已实现（2026-08-12）
- 前置：MEM-03C-02A EpisodeFact/ContextDescriptorFact 已提交
- 范围：只实现确定性派生编译，不实现 Librarian Recall、Doctor、采集桥或模型调用

## 一、目标与成功标准

```text
显式固定 EpisodeRef + ContextDescriptorRef
→ FactStore 精确加载
→ 引用/Scope/时间/Root Task 唯一性校验
→ Episode Card JSON + Markdown
→ Episodic Index JSON + Markdown
→ 输出清单与 compiled_output_sha256
```

成功标准：相同事实乱序输入产生逐字节一致输出；每个 Episode 恰好出现一次；删除派生输出后可
重建同一 Hash；Project/Global 隔离；不读取 CURRENT、Evolution Store、网络或墙钟。

## 二、固定 API

```go
type EpisodicCompileRequest struct {
    Scope           Scope
    GenerationID    string
    CompilerVersion string
    EvaluationTime  time.Time
    EpisodeRefs     []EpisodeRef
    ContextRefs     []ContextDescriptorRef
    Store           *FactStore
}
```

`EvaluationTime` 必填；未来 Fact 拒绝。所有 Ref 必须完整匹配 Store 中不可变 Fact；请求不得携带
摘要、可信 Hash、输出路径或 CURRENT。重复的相同 Ref 去重；同 ID 异 Hash、同 Root Task 多
Episode、跨 Scope、缺 Context、错误 Hash均 fail closed。

## 三、派生输出

每个 Episode 生成：

```text
wiki/episodes/cards/<stable-episode-id>.md
state/episodes/cards/<stable-episode-id>.json
```

Card 只包含 EpisodeRef、ContextDescriptorRef、受控分类 ID、task_result、occurred_at、EvidenceRef
集合 Hash、Generation ID 与 Compiler Version。不得包含自由摘要、完整命令、模型输出、思考、
凭据、绝对路径或 Evidence 正文。JSON/Markdown 由同一中间结构渲染。

索引生成：

```text
wiki/episodes/index.md
state/episodes/index.json
```

首版采用单页有界索引（最多 1024 Episode、1 MiB）；超过上限返回稳定错误，不截断。条目按
`component → operation → task_class → failure_concept → YYYY-MM → task_result → episode_id`
确定性排序。多值维度只在条目中保存排序后的集合，不复制 Card、不重复 Episode。

## 四、安全与兼容边界

- 只写调用方提供的临时 staging 根；输出路径由程序生成并逐组件拒绝 symlink；文件 0600、目录 0700；
- 编译失败零发布，由 MEM-01D Generation 事务负责最终发布；
- Episode/Context 是规范事实；Card/Index 永远不是 FactStore 输入；
- 旧 `internal/evolution.Episode` 不读取、不迁移；
- 不修改旧 Fact Canonical Bytes/Hash，不创建 Tag/Release。

## 五、TDD 验收矩阵

1. 正常 Project/Global 编译与 JSON/Markdown 同源；
2. 输入乱序/重复、重复执行、删除重建均字节稳定；
3. 缺 Fact、错误 Hash、跨 Scope、未来时间、零 EvaluationTime 拒绝；
4. 相同 Root Task 多 Episode、同 ID 异 Hash、Context 断链拒绝；
5. 每个 Episode 在索引恰好一次，固定排序；
6. 1024 边界通过，1025 拒绝且不截断；
7. staging symlink/路径替换/权限异常拒绝，外部文件零写入；
8. 输出中不出现绝对路径、命令、凭据、Evidence 正文或自由摘要；
9. Legacy Evolution 目录存在也不影响输出；
10. 既有 Generation/OKF 编译器与全部 golden 无回归。

## 六、门禁与提交

```bash
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

功能联调还需在临时 Store 编译、删除输出、重建并比对目录 Hash。全部通过后独立提交并推送，
然后进入 MEM-03C-03 固定世界 Recall/Doctor。
