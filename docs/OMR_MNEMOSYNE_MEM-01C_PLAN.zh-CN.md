# OMR Mnemosyne MEM-01C：Policy Fact 与派生状态骨架

## 任务状态

- 阶段：MEM-01C
- 状态：✅ 已完成并经 CTO Review 签收（Policy Fact 五类 Config Schema、PolicyStore、历史加载与路径安全回归均已通过门禁）
- 前置：Architecture v1 已冻结；MEM-01A、MEM-01B 已由 CTO 签收
- 后续：MEM-01D Generation 事务、MEM-01E 最小 OKF 编译器

本阶段把 Freshness、Trust、Content Classifier、Index、Benchmark 五类策略定义为严格、不可变、可版本化的 Policy Fact，并提供历史加载与引用校验协议。

本阶段不实现完整业务评估器，不生成 Generation，不切换 `CURRENT`，不接 CLI、Prompt 或 Reasonix。

## 一、允许修改与禁止范围

允许修改：

```text
internal/memory/policy.go
internal/memory/policy_store.go
internal/memory/policy_store_test.go
internal/memory/policy_eval.go
internal/memory/policy_eval_test.go
docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md
```

可按现有仓库风格调整文件名，但必须继续复用 MEM-01A 的 `PolicyFact`、`PolicyRef`、Canonical Hash、`DecodeStrict` 和 MEM-01B FactStore，不得复制第二套存储或 Hash 逻辑。

禁止修改：

- `internal/evolution/**`、`cmd/omr/**`、配置、Manifest、Prompt、assets、`.reasonix/**`；
- Architecture v1、MEM-01A/01B 已签收协议；
- `CURRENT`、Generation、OKF Wiki、Index 输出；
- Lifecycle、Health、Usage Statistics、Attribution、Freshness Judgment 的业务状态计算；
- 自动修改 Trust Policy、Promotion Gate 或安全根；
- Reasonix 调用、真实模型调用、跨项目 Promotion、CLI 和 Web 页面。

发现规格冲突时停止并报告，不自行修改冻结架构。

## 二、Policy Fact 共同契约

所有 Policy Fact 必须继续使用 MEM-01A 的严格模型和 MEM-01B 的安全 Store：

```json
{
  "schema_version": 1,
  "policy_id": "trust_policy_v1",
  "policy_type": "trust",
  "policy_version": 1,
  "config": {},
  "content_sha256": "sha256_...",
  "created_at": "2026-08-10T00:00:00Z"
}
```

硬约束：

1. `policy_id + policy_type + policy_version` 是稳定身份；同身份不可覆盖；
2. `content_sha256` 必须由程序按规范 Canonical JSON 计算，模型和调用方提供的 Hash 不可信；
3. 未知字段、未知 Policy Type、错误 config 联合键、错误类型和 Hash 不匹配必须拒绝；
4. 同一 Policy Fact 重复写入返回 NOOP；同一身份不同 Hash 返回冲突；
5. Policy 更新创建新 `policy_version`/新 Fact，不修改旧 JSON；
6. `PolicyRef` 必须包含 `policy_id + policy_type + content_sha256`，并能精确加载对应历史 Fact；
7. “当前策略”只能由后续派生层选择，Fact Store 本身不得隐式选择最新版本；
8. Policy 内容不得包含绝对路径、Prompt、模型思考、命令正文、凭据或自由指令；
9. Policy 只能由受控代码或用户显式治理写入，不能由 Memory Mutation、Proposal 或记忆内容直接修改。

## 三、五类 Policy Config Schema

每个 config 必须是严格的按 `policy_type` 判别联合，不允许把任意 JSON 当作配置保存。字段值应为有限标量、枚举、结构化列表或 PolicyRef，不得使用无法验证的自由文本规则。

### 1. Freshness Policy

用于后续 Freshness Evaluation 的版本化参数，不在本阶段计算 `fresh/aging/stale`：

```yaml
freshness:
  evaluation_window_days: 90
  aging_after_days: 180
  stale_after_days: 365
  revalidation_evidence_types: [test_result, usage_outcome]
  version: 1
```

约束：窗口为正整数且顺序满足 `evaluation_window < aging_after < stale_after`；Evidence 类型必须是受控标识；不允许配置自动删除、自动冻结或自动修改 Revision 的动作。

### 2. Trust Policy

Trust Policy 是安全根，只保存边界和判定输入，不能被 Mnemosyne 自进化：

```yaml
trust:
  allowed_acquisition_methods: [local_file, test_run, user_confirmation]
  require_provenance: true
  require_verification_status: true
  external_unverified_instruction_allowed: false
  promotion_requires_policy_evidence: true
  version: 1
```

约束：禁止配置为允许外部未验证内容直接成为指令；禁止关闭 Provenance/Verification 硬约束；Trust Policy 的写入来源必须可审计。

### 3. Content Classifier Policy

定义内容分类器使用的结构化规则引用和安全默认值，不在本阶段运行分类器：

```yaml
content_classifier:
  classifier_id: "omr_builtin_classifier_v1"
  allowed_classes: [instructional, descriptive, secret, unsafe]
  default_class: descriptive
  secret_classes_block_promotion: true
  version: 1
```

约束：`default_class` 必须属于 `allowed_classes`；允许类别不能为空；不能把 Secret/Unsafe 配置为可晋升指令内容；分类器版本必须稳定可追踪。

### 4. Index Policy

定义后续派生索引的拓扑上限，不限制一次任务读取总量：

```yaml
index:
  max_entries_per_page: 64
  max_page_bytes: 32768
  max_shard_depth: 4
  split_order: [component, operation, memory_type, stable_id_prefix]
  overflow_bucket: other
  version: 1
```

约束：数值必须为正且有上限；`split_order` 只能使用固定维度并拒绝重复；`overflow_bucket` 必须是安全稳定 ID（沿用现有 `validateField` 标识符规则，示例中的 `_other` 因首字符为下划线不满足该规则，以 `other` 为准）；本阶段不生成任何索引文件。

### 5. Benchmark Policy

定义后续质量基准的预注册门禁，不在本阶段执行 Benchmark：

```yaml
benchmark:
  fixture_set_id: "mnemosyne_memory_quality_v1"
  minimum_cases: 1
  required_metrics: [retrieval_recall, citation_accuracy, safety_regression]
  pass_thresholds:
    retrieval_recall: 0.0
    citation_accuracy: 0.0
    safety_regression: 1.0
  paired_comparison_required: true
  version: 1
```

约束：指标名称受控；阈值为有限数值并处于允许范围；`fixture_set_id` 不得是绝对路径；Benchmark Policy 不能声称模型能力提升，只有后续完整配对数据才能支持有限范围结论。

## 四、Policy Store 与历史加载

提供最小的 Policy 专用 API，但底层必须复用 MEM-01B FactStore：

```go
type PolicyStore interface {
    PutPolicy(ctx context.Context, policy PolicyFact) (WriteResult, error)
    GetPolicy(ctx context.Context, ref PolicyRef) (PolicyFact, error)
    GetPolicyVersion(ctx context.Context, policyID string, version int) (PolicyFact, error)
}
```

要求：

- `GetPolicy(ref)` 必须精确匹配 ID、Type、Version 对应 Fact 的 Hash；
- 不允许把 `policy_id` 解析成路径或只读取“最新版本”；
- 缺失历史版本必须返回稳定错误，不能用当前版本替代；
- 不允许跨 Project/Global Store 读取未授权 Policy；
- Policy Store 不修改 `CURRENT`、Generation 或派生状态；
- 读取结果为不可变值，调用方不能通过返回对象改变磁盘事实。

## 五、安全根与派生状态边界

以下内容必须在代码注释、测试和文档中明确为派生状态，不得加入 PolicyFact：

```text
Lifecycle
Health
Usage Statistics
Relation Index
Root/Local Index
Generation
Web View
Freshness Result
Trust Result
Promotion Eligibility
```

Policy Fact 只保存“使用哪套规则以及规则参数”；评估结果必须在后续 Judgment/Evaluation/Generation 层按固定 PolicyRef 产生。Trust Policy、Content Classifier Policy 和安全 Gate 不得被 Memory Proposal 或自动演化修改。

## 六、测试矩阵

先写失败测试，再做最小实现：

1. 五类 Policy Config 的合法 Schema 和未知字段拒绝；
2. Policy Type 与 Config 联合键不匹配拒绝；
3. Freshness 窗口顺序、正数和 Evidence 类型约束；
4. Trust Policy 禁止关闭安全硬约束；
5. Content Classifier 默认类别和 Secret/Unsafe 边界；
6. Index split_order 重复、未知维度和数值上限；
7. Benchmark 指标、阈值范围和 fixture ID 安全校验；
8. Canonical JSON、Content Hash 和重复写入 NOOP；
9. 同身份不同 Hash 冲突且旧 Fact 不变；
10. PolicyRef 精确加载历史版本；
11. 缺失/损坏/Hash 漂移的历史 Policy 不被当前版本替代；
12. Project/Global Store 隔离；
13. Policy 内容禁止绝对路径、Prompt、命令、凭据和自由指令；
14. 派生字段无法通过 JSON 写入 PolicyFact；
15. Policy Store 与底层 FactStore 共用权限、symlink、锁和脱敏行为；
16. Context 取消、超时和读写失败不会产生半成品或覆盖旧 Fact。

## 七、非目标

本阶段不得实现：

- Freshness/Trust/Content Classifier 的实际运行结果；
- Retrieval、Librarian、Episode Recall、Benchmark 执行；
- Lifecycle、Health、Usage Statistics 或 Promotion Gate；
- Generation Input Manifest 写入或 `CURRENT` 更新；
- `omr memory` CLI、Web 页面、Prompt Composer；
- 自动批准、自动冻结、自动删除或自动修改 Trust Policy。

## 八、门禁与交付报告

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

环境导致的端口、权限或 Go Cache 失败必须标记 `[ENV]`，不得伪造通过。

交付报告必须列出：修改文件、五类 Config Schema、PolicyStore API、历史加载行为、严格拒绝边界、测试数量、门禁结果、风险，并明确“未进入 MEM-01D”。不得自行提交、推送、创建 Tag 或开始下一阶段。

## 九、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne 的 MEM-01C：Policy Fact 与派生状态骨架。

先读取：
1. docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
2. docs/OMR_MNEMOSYNE_IMPLEMENTATION_TODO.zh-CN.md
3. docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
4. docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
5. docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md
6. internal/memory/**（已签收的模型、Canonical Hash 和安全 FactStore）

只执行 MEM-01C，不执行 MEM-01D、MEM-01E 或后续任务。

只允许修改 internal/memory/** 以及本计划状态。禁止修改 Architecture v1、internal/evolution、cmd/omr、配置、Manifest、Prompt、assets、.reasonix。不要接 CLI、Reasonix、CURRENT、Generation、Wiki 或 Prompt Composer。

目标：严格定义并保存五类版本化 Policy Fact：freshness、trust、content_classifier、index、benchmark；实现严格 Config Schema、Canonical Hash、PolicyRef 精确历史加载，并复用 MEM-01B FactStore。不得创建第二套 Store、Hash 或路径安全逻辑。

必须保证：
- unknown fields、错误 Config 联合键、错误类型、非法阈值、绝对路径、Prompt、命令、凭据和自由指令全部拒绝；
- Policy 更新只创建新不可变 Fact，不覆盖旧版本；
- 相同身份相同 Hash NOOP，不同 Hash fail closed；
- GetPolicy 必须按 policy_id + policy_type + content_sha256 精确加载，不得使用“当前最新策略”替代历史版本；
- Project/Global Store 隔离；
- Trust Policy、Content Classifier Policy 和安全 Gate 不允许被 Memory Mutation 或自动演化修改；
- Lifecycle、Health、Usage、Index、Generation、Web View 和评估结果必须保持派生状态，不写入 PolicyFact。

先写失败测试，再实现最小代码。不要实现实际 Freshness/Trust/Benchmark 评估器。

验证：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

最后只输出交付报告：修改文件、Config Schema、PolicyStore API、历史加载、测试和门禁、风险，并明确未进入 MEM-01D。不要提交、推送、创建 Tag。
```
