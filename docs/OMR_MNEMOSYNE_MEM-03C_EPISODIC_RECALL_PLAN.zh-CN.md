# OMR Mnemosyne MEM-03C：Episodic Recall 计划

- 阶段：MEM-03C
- 状态：✅ MEM-03C-01～04 自动化实现完成，待真实 Reasonix Desktop 联调
- 前置：MEM-03A 确定性分片索引、MEM-03B 固定快照 Librarian 已实现
- 目标：让 Reasonix 能在尚未形成稳定 Memory 的历史任务中渐进查找相似 Episode，同时保证
  Episode Card 和 Episodic Index 只是可删除重建的派生表示

## 一、现状与关键决议

Architecture v1 已规定：

```text
Episode / Evidence / Context Descriptor
→ 规范事实

Episode Card / Episodic Index
→ 固定 Generation 内派生读取表示
```

但当前 `internal/memory` 尚无 `EpisodeFact` 与 `ContextDescriptorFact`；
`internal/evolution.Episode` 是 v2.0 MVP 的早期采集记录，缺少 Scope、Root Task、
Context/Evidence 引用和可验证 Content Hash，并且由独立 Evolution Store 管理。它不能直接成为
Mnemosyne 的规范 Episode，也不能由 Card 或 Index 反向补齐缺失字段。

因此 MEM-03C 分为三个可独立 Review 的子阶段：

1. **MEM-03C-01 Schema Gate**：冻结 `EpisodeFact`、`ContextDescriptorFact` 与引用类型；
2. **MEM-03C-02 派生编译器**：从显式固定事实编译 Episode Card/Episodic Index；
3. **MEM-03C-03 固定世界读取与 Doctor**：Librarian 读取、重建与漂移诊断。

Schema Gate 通过前不新增 FactKind、不写产品代码、不修改旧 Evolution Episode。

只读审核结果见 `OMR_MNEMOSYNE_MEM-03C_SCHEMA_GATE_AUDIT.zh-CN.md`。审核确认当前缺口属于
实现盘点暴露的 Architecture Amendment 条件；D1～D10 已批准，Gate 状态为 `PASS`。

## 二、范围与成功标准

```text
固定 Project / Global Generation Pair
→ 读取 Generation Input Manifest 中显式列出的 Episode/Context/Evidence
→ 编译有界 Episodic Index
→ 逐层定位脱敏 Episode Card
→ 必要时按 EvidenceRef 核验事实
→ 父 Agent 获得 EpisodeRef/Card 路径，不获得完整轨迹
```

成功标准：

1. Card/Index 的每个字段均可追溯到规范 Fact 或版本化编译策略；
2. 删除全部 Card/Index 后，用永久 Generation Input Manifest 可逐字节重建；
3. 不保存或展示完整命令、完整模型输出、思考、凭据、绝对路径和无必要轨迹；
4. 不使用 Embedding、向量数据库、相似度阈值或程序自然语言语义分类；
5. Project/Global、Episode、Root Task、Context 和 Evidence 引用严格隔离；
6. 后台 CURRENT 切换不影响进行中的 Episodic Recall；
7. Recall 命中只允许记录 `retrieved/read`，不得直接产生 help/harm、晋升或冻结。

## 三、MEM-03C-01：Schema Gate

### 3.1 拟冻结 EpisodeFact

以下是 Gate 输入，不在 Gate 批准前作为最终 Schema：

```yaml
schema_version: 1
episode_id: episode_01K...
scope: project
root_task_id: task_01K...
context_descriptor_ref:
  scope: project
  context_descriptor_id: context_01K...
  content_sha256: sha256_...
task_class_refs: [task_class_build]
component_refs: [component_memory]
operation_refs: [operation_compile]
failure_concept_refs: []
task_result: succeeded
task_result_evidence_refs: []
evidence_refs: []
occurred_at: 2026-08-12T00:00:00Z
content_sha256: sha256_...
created_at: 2026-08-12T00:00:00Z
```

Gate 已冻结：

- 一个 Root Task 只形成一个 Episode；attempt 只作为 Evidence，不进入身份；
- `task_result` 使用 `succeeded|failed|cancelled|unknown`，不复用 Memory Outcome；
- `task_class_refs/component_refs/operation_refs/failure_concept_refs` 使用受控 ID 还是完整
  MemoryRef；推荐使用受控 ID 并由 Context Descriptor 保存结构化世界，避免循环引用；
- v1 不保存自由文本摘要；Card 只渲染结构化事实；
- EvidenceRef、OutcomeRef、ContextDescriptorRef 的 Scope 与 Hash 一致性规则；
- 最大数组长度、文本上限、时间顺序与 Canonical 排序规则。

### 3.2 拟冻结 ContextDescriptorFact

复用 Architecture v1 13.8 的结构，不新增自由文本：

```yaml
schema_version: 1
context_descriptor_id: context_01K...
scope: project
context_signature_version: 1
component_refs: [component_memory]
operation_refs: [operation_compile]
task_class_refs: [task_class_build]
environment:
  os: darwin
  arch: arm64
  language: go
  framework: ""
  tool: omr
canonical_sha256: sha256_...
content_sha256: sha256_...
created_at: 2026-08-12T00:00:00Z
```

`context_signature` 由程序从 Canonical Descriptor 重算；模型和调用方不得提供可信最终 Hash。
环境只允许版本化白名单键和值上限，不记录主机名、用户名、绝对路径、Remote、业务名称或凭据。

### 3.3 旧 Evolution Episode 的兼容边界

- 不修改或删除 `internal/evolution.Episode`；
- 不让 Mnemosyne Store 直接读取 Evolution 私有目录；
- 后续采集桥必须显式生成新的 `EpisodeFact`，逐字段验证并计算新 Hash；
- 旧记录缺少规范 Scope/Root Task/Context/Evidence 时保持 legacy-only，不猜测、不迁移；
- 相同旧记录重复导入必须幂等；不同内容映射到同一 Episode ID 必须冲突并零写入；
- 采集桥属于 MEM-04/集成阶段，不是 MEM-03C 的隐式副作用。

### 3.4 Gate 通过条件

- Architecture v1、MEM-01～03B 与拟扩展 Schema 无冲突；
- Fact/Ref/派生对象的事实源唯一；
- Canonical Bytes、Content Hash、Scope 和历史兼容策略均有 golden 测试方案；
- `tests/docs_check.sh` 锁定禁止项：Card 作为 Fact、读取 Evolution 私有目录、向量检索、
  保存完整命令/思考/凭据、模型提供可信 Hash；
- CTO 明确批准后状态才改为“Schema 冻结，待实现”。

## 四、MEM-03C-02：Episode Card 与 Episodic Index 编译器

### 4.1 编译输入

编译器只接受显式输入：

```go
type EpisodicCompileRequest struct {
    Scope             Scope
    GenerationID      string
    CompilerVersion   string
    EvaluationTime    time.Time
    EpisodeRefs       []EpisodeRef
    ContextRefs       []ContextDescriptorRef
    EvidenceRefs      []EvidenceRef
    OutcomeRefs       []OutcomeRef
    IndexPolicyRef    PolicyRef
}
```

所有引用必须出现在永久 Generation Input Manifest 中并经 FactStore 精确加载；不扫描“最新”、
不读取 CURRENT、不读取 Evolution Store、不接受调用方提供的摘要 Hash。

### 4.2 Episode Card

```yaml
schema_version: 1
episode_ref: {scope, episode_id, content_sha256}
context_descriptor_ref: {scope, context_descriptor_id, content_sha256}
evidence_set_sha256: sha256_...
compiler_version: mnemosyne-episode-card-compiler/1
generation_id: gen_project_...
occurred_at: 2026-08-12T00:00:00Z
component_refs: []
operation_refs: []
task_class_refs: []
failure_concept_refs: []
outcome_summary: succeeded|failed|unknown
sanitized_summary: "..."
card_sha256: sha256_...
```

Card 是派生 JSON/Markdown 双表示：JSON 用于机器核验，Markdown 用于模型阅读。两者必须由同一
中间结构渲染并有输出 Hash；禁止独立编辑，禁止反向覆盖 Episode。

### 4.3 Episodic Index

固定路由维度：

```text
component → operation → task_class → failure_concept
→ environment bucket → time bucket → outcome → stable_episode_id_prefix
```

规则：

- 使用版本化 Index Policy；页面条数、字节、分片深度有上限，永不截断；
- 同一 Episode 只出现一次；多值维度使用确定性的 canonical bucket，不复制整张 Card；
- `occurred_at` 只用于版本化时间 bucket，不读取墙钟；
- 文本字段只作为 Card 展示和 Librarian 语义判断，不由 Go 枚举或计算相似度；
- 索引条目只含 EpisodeRef、受控维度、时间、Outcome 状态、Card 路径与 Hash；
- Project 与 Global 独立编译，禁止把项目 Episode 自动写进 Global。

## 五、MEM-03C-03：固定世界读取、Recall 回执与 Doctor

### 5.1 Episode Recall Request/Receipt

Request 复用 MEM-03B `RetrievalContext`，另带结构化过滤器和受限任务摘要。Receipt：

```yaml
schema_version: 1
retrieval_id: retrieval_...
memory_context: {project: ..., global: ...}
status: found
episode_cards:
  - episode_ref: {scope, episode_id, content_sha256}
    card_path: wiki/episodes/...
    card_sha256: sha256_...
    relevance_rank: 1
    why: "受限理由"
visited_index_paths: [wiki/episodes/index.md]
requires_parent_read: true
```

`found|no_candidate|unknown|unavailable` 语义沿用 MEM-03B。OMR 只校验固定世界、完整引用、
路径、Hash、过滤器、去重和结构化排序；Reasonix Librarian 判断自然语言相关性。

### 5.2 Reader 与安全边界

- 只读取固定 Generation 中 index-tree 注册的 Episode Card/Index；
- 逐组件拒绝 symlink、路径穿越、绝对路径、反斜杠、非普通文件和超限文件；
- 读取前验证 Generation/Manifest/compiled hash；CURRENT 切换不影响结果；
- 正常读取不返回完整 Evidence 正文；父 Agent 明确核验时只通过 EvidenceRef 进入已有安全读取链；
- 错误固定脱敏，不包含摘要、命令、路径、Evidence ID 或凭据。

### 5.3 Doctor

Doctor 只读检查：

- Episode/Context/Evidence/Outcome 引用断链或跨 Scope；
- Card 的 source hash、compiler version、generation ref、card hash 漂移；
- Episodic Index 重复、遗漏、截断、越界、错误路由或指向不存在 Card；
- Generation Input Manifest 未完整记录实际输入；
- legacy Evolution Episode 被误用为规范事实；
- 删除派生 Card/Index 后 dry-run 重建 Hash 是否一致。

Doctor 不自动修复、不删除、不把派生视图提升为事实。

## 六、MemoryUsage 与生命周期隔离

- Recall 命中只能产生 `usage_stage=retrieved`；父 Agent 实际读取 Card 后才允许 `read`；
- Episode Recall 不等于使用某条 MemoryRevision，不能用 Episode ID 伪造 MemoryID；
- `retrieved/read` 不产生 Outcome，不进入 help/harm 分母，不改变 Lifecycle/Health；
- 自动创建 Memory Candidate、Pattern 或 Revision 属后续 Mutation 流程，不由 Recall 直接触发；
- Recall/Doctor 失败不覆盖父任务退出码，不产生半成品事实。

## 七、TDD 测试矩阵

### 7.1 Schema Gate 与 Store

1. Episode/Context/Ref 合法 round-trip、strict unknown-field、golden bytes/hash；
2. Scope/Hash/Root Task/Attempt/时间/数组边界/重复引用 fail closed；
3. Legacy Evolution Episode 不可直接 Put 到 Mnemosyne Store；
4. 相同事实 NOOP、身份冲突零覆盖、symlink/权限/跨 Scope 继承 FactStore 安全链。

### 7.2 编译与重建

5. 相同 Manifest 输入乱序/重复后输出字节一致；
6. 删除 Card/Index 后固定 Manifest 重建 Hash 一致；
7. Card 缺输入、跨 Scope、错误 Hash、未来时间、敏感内容均拒绝；
8. Index 超限分片且不截断，所有 Episode 恰好一次；
9. Project/Global 完全隔离，CURRENT 切换不污染固定世界；
10. Card JSON/Markdown、Index 与 Generation compiled hash 任一篡改均被发现。

### 7.3 Recall 与安全

11. found/no_candidate/unknown/unavailable 状态矩阵；
12. 结构化过滤、重复、路径断链、symlink、超限与错误 Card Hash；
13. 同 rank 确定性 tie-break，多次执行回执字节一致；
14. Recall 只产生 retrieved/read，不产生 help/harm/Lifecycle 变化；
15. 错误与回执不泄露绝对路径、命令、Prompt、思考、凭据或完整 Evidence；
16. Fake Librarian 端到端：固定 Context → Index → Card → Receipt，无 API Key。

## 八、分阶段交付顺序

```text
MEM-03C-01：Schema Gate 文档与 Docs Gate
→ CTO 批准
MEM-03C-02：Episode/Context Fact + 编译器 + golden
→ 完整门禁、独立 Review、安全 Review、提交推送
MEM-03C-03：Reader/Receipt/Doctor + Fake Librarian
→ 完整门禁、真实临时项目功能验证、提交推送
```

每个子阶段都必须保持小提交。MEM-03C 完成前不进入 MEM-03D；不创建 Tag/Release。

## 九、通用门禁

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

若 qualitybench 因沙箱禁止本地端口，必须在允许回环监听的环境复跑；不能伪造通过。

## 十、交给 Reasonix 的第一阶段执行提示词

```text
执行 OMR Mnemosyne MEM-03C-01：Episodic Recall Schema Gate。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

只做设计与只读审核，不实现产品代码。完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 5.3、8.8.1、13.8、17、18 章；
- docs/OMR_MNEMOSYNE_MEM-03C_EPISODIC_RECALL_PLAN.zh-CN.md；
- docs/OMR_MNEMOSYNE_MEM-03B_LIBRARIAN_PLAN.zh-CN.md；
- internal/evolution/model.go、store.go；
- internal/memory/store.go、model.go、memory_usage.go、evaluation_context.go、
  okf_compiler.go、index_tree.go 与相关测试。

目标：逐项审核 EpisodeFact、ContextDescriptorFact、EpisodeRef、ContextDescriptorRef、
OutcomeRef 的事实身份、Scope、Canonical Hash、时间、数组、脱敏、Legacy 兼容和 Manifest
接入是否与 Architecture v1/MEM-01~03B 一致。

硬约束：
1. internal/evolution.Episode 不是 Mnemosyne 规范事实，禁止直接复用或扫描其私有目录。
2. Episode Card/Episodic Index 是派生表示，禁止新增为 FactKind 或反向覆盖 Episode。
3. 不保存完整命令、模型输出、思考、凭据、绝对路径或无必要轨迹。
4. 不引入 Embedding、向量数据库、相似度阈值或 Go 自然语言语义分类。
5. 模型不得提供可信 Hash；Hash 必须由程序从 Canonical Fact 重算。
6. Schema Gate 未 PASS 前不改 internal/memory、CLI、Profile、Prompt 或 Store。

输出：对象判定；与冻结 Schema 的差异表；必须新增/复用的引用；Legacy 迁移边界；
待 CTO 决策点与推荐；Docs Gate 规则；明确“未实现代码、未提交、未推送、未进入
MEM-03C-02”。
```

## 十一、明确不做

MEM-03C 不实现 Retrieval Evaluation（MEM-03D）、CLI（MEM-03E）、Memory Candidate/Mutation
自动生成、旧 Episode 批量迁移、Web、自动 Hook、跨项目全局晋升、Embedding 或向量数据库。
