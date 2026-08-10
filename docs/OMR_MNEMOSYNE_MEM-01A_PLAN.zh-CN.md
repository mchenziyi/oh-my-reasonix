# OMR Mnemosyne MEM-01A：核心类型与严格 Schema

## 任务状态

- 阶段：MEM-01A
- 状态：✅ 已完成，CTO 已签收
- 目标：建立不可变事实、引用、条件、Policy 和确定性 Hash 的代码契约
- 前置：Architecture v1 已冻结
- 后续：MEM-01B Store、MEM-01C Policy Store、MEM-01D Generation 事务

## 一、严格边界

本任务只实现纯模型和纯函数，不写文件、不接 CLI、不接 Prompt、不接 Reasonix、不做迁移、不创建 Tag。

允许修改：

```
internal/memory/model.go
internal/memory/refs.go
internal/memory/conditions.go
internal/memory/policy.go
internal/memory/canonical.go
internal/memory/*_test.go
```

禁止修改 `internal/evolution/**`、`cmd/omr/**`、配置、Manifest、Prompt、assets、`.reasonix/**` 和 Architecture v1。

发现规格与代码冲突时停止并报告，不自行改规格。

## 二、必须实现的类型

严格枚举：

```
Scope: project | global | portable
MemoryType: pattern | strategy | decision | playbook | preference | failure_concept | component
UsagePolicy: outcome_attributed | evidence_validated | explicit_confirmation
JudgmentType: confirmation | attribution_override | retrieval_relevance |
  context_applicability | content_classification | evidence_trust | freshness_evaluation
PolicyType: freshness | trust | content_classifier | index | benchmark
ApplicabilitySubject: environment | project | toolchain | component
ApplicabilityOperator: equals | not_equals | contains | exists | version_satisfies
```

使用自定义字符串类型和 `Validate()`，未知枚举值必须拒绝。

所有引用必须包含 Scope、稳定 ID 和内容 Hash，机器关系不得使用路径、标题或 Markdown 文件名：

```
MemoryRef: scope + memory_type + memory_id + revision + content_sha256
EvidenceRef: scope + evidence_type + evidence_id + content_sha256
JudgmentRef: scope + judgment_type + judgment_id + content_sha256
PolicyRef: policy_id + policy_type + content_sha256
ConfirmationSourceRef: JudgmentRef, judgment_type 必须为 confirmation
```

实现 `MemoryRevision`、`MemoryEvidenceGeneration`、`JudgmentFact`、`PolicyFact` 和 `GenerationInputManifest`。模型必须包含 Architecture v1 规定的 schema_version、Scope、身份、内容、Evidence/Ref、Policy 和时间字段。

硬约束：

- Revision 和 Evidence Generation 不可变；变化创建新版本；
- `usage_policy` 创建后不可原地修改；
- `explicit_confirmation` 的任何 Revision 都必须有合法 `confirmation_source_ref`；
- `confirmation` Judgment 的 `status` 固定为 `confirmed | revoked`；`revoked` 必须携带 `supersedes_judgment_ref` 指向同类型（confirmation）被撤销 Judgment；
- Lifecycle、Health、Usage Statistics、Index、Generation 和 Web View 不得成为事实字段；
- Generation Input Manifest 的输入按 `fact_type + fact_id` 去重并确定性排序；
- Manifest 不保存绝对路径、Prompt、思考、凭据或自由文本事实副本。

## 三、ApplicabilityCondition

实现结构化条件，不允许自由文本机器条件：

```
condition_id + subject + subject_ref + field + operator + value
```

- 同一 Revision 内 ConditionID 唯一；
- `subject=component` 必须有合法 Component Ref，其他 Subject 的 subject_ref 必须为空；
- value 只允许字符串、布尔值、有限数字或有界标量数组；
- Field 不得包含绝对路径、换行、命令或自由指令；
- 无法表达时返回验证错误，不能降级为字符串。

## 四、确定性 API

提供纯函数：

```
Validate()
CanonicalBytes()
ContentHash()
EncodeCanonical()
DecodeStrict()
```

要求：

- JSON 使用 `DisallowUnknownFields()`；
- 缺字段、未知字段、错误类型、非法枚举、非法 Hash 均失败；
- 数组排序、去重，时间统一 RFC3339Nano UTC；
- Hash 计算排除自身 Hash 字段；
- 不使用 map 遍历顺序生成 Hash；
- 不吞掉解析和规范化错误。

## 五、测试矩阵

先写失败测试，再实现。至少覆盖：

1. 枚举合法值和未知值；
2. 所有 Ref 缺字段、错误 Scope/Type、非法 Hash；
3. ConfirmationSourceRef 指向非 confirmation Judgment；
4. explicit_confirmation 缺少 confirmation_source_ref；
5. component 条件缺 subject_ref、非 component 携带 subject_ref；
6. 自由文本 applies_when；
7. 数组/字符串上限、绝对路径、换行、命令内容；
8. EvidenceRef 乱序/重复的确定性 Hash；
9. GenerationInputManifest 乱序/重复的确定性 Hash；
10. 未知 JSON 字段；
11. 同对象多次编码字节一致；
12. 修改受保护字段改变 Hash；
13. Hash 自引用不造成不稳定；
14. MemoryType 与 UsagePolicy 矩阵；
15. 派生状态不会被当作事实字段。

## 六、非目标

不得实现 Store、AtomicWrite、锁、CURRENT、Generation 发布、Wiki 编译、Prompt Composer、CLI、Doctor、迁移、MemoryUsage、Attribution、Freshness 业务计算、Global Promotion 或 Evolution Overlay 改动。

## 七、门禁

```
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

交付报告必须列出修改文件、测试数量、门禁结果、Schema/Hash 行为和风险，并明确“不进入 MEM-01B”。不得自行提交、推送、创建 Tag 或开始下一阶段。

## 八、给 Reasonix Agent 的完整提示词

```
执行 OMR Mnemosyne 的 MEM-01A，只执行本任务，不执行 MEM-01B 或后续任务。

先读取：
1. docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
2. docs/OMR_MNEMOSYNE_IMPLEMENTATION_TODO.zh-CN.md
3. docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
4. internal/evolution/model.go、internal/fileutil/fileutil.go（仅了解风格）

只允许新增/修改 internal/memory/**。不修改 internal/evolution、cmd/omr、配置、Manifest、Prompt、assets、.reasonix 或架构文档。不写文件，不接 CLI，不做 Store、CURRENT、Generation、Wiki、迁移或 Reasonix 调用。

先写失败测试，再实现最小代码。实现严格枚举、Ref、ApplicabilityCondition、MemoryRevision、MemoryEvidenceGeneration、JudgmentFact、PolicyFact、GenerationInputManifest，以及确定性 CanonicalBytes/ContentHash/DecodeStrict。explicit_confirmation 的任何 Revision 必须带 confirmation_source_ref；Hash 必须由程序计算；未知字段、自由文本机器条件、路径身份、错误类型和 Hash 不匹配必须拒绝。

完成后运行：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

最后只输出交付报告：修改文件、测试结果、门禁结果、关键 Schema/Hash 行为、风险，并明确未进入 MEM-01B。不要自行提交、推送、创建 Tag 或开始下一阶段。
```
