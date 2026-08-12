# OMR Mnemosyne MEM-03B：Librarian 渐进读取协议计划

- 阶段：MEM-03B
- 状态：🟡 设计冻结，待实现
- 前置：MEM-03A 确定性 Root/Local Index 已实现；Project/Global Generation、
  MemoryContext、Conflict/Critic/Trust Gate 已存在
- 目标：为 Reasonix 提供一个只读 Mnemosyne Librarian Profile，以及可由程序严格校验的
  固定快照、候选和检索回执协议；语义相关性由 Librarian 判断，引用、安全、Scope、状态和
  确定性排序由 OMR 验证

## 一、范围与成功标准

本阶段只完成方案 B 的 Librarian 协议内核和 Profile：

```text
父 Agent 提供任务摘要
→ OMR 固定 Project / Global Generation Pair
→ Librarian 逐层读取 Root / Local Index
→ Librarian 返回结构化候选引用和理由
→ OMR 在固定世界中验证引用并执行结构化排序
→ 父 Agent亲自读取决定采用的页面
```

成功标准：

1. 一次 Retrieval 从开始到回执结束始终使用同一 Project/Global Generation Pair；
2. Librarian 只能返回固定 Generation 中真实存在、Hash/Scope/Revision 匹配的页面；
3. 正常模式下 frozen/archived/superseded 记忆不能进入候选；
4. 程序不分析自然语言语义，不引入 Embedding、向量数据库或相似度阈值；
5. Project/Global 结构化优先级、状态排序和 tie break 确定性可重放；
6. 空结果、缺失快照、快照损坏分别稳定表达，不猜测、不切换未来 `CURRENT`；
7. Librarian 只返回定位和选择依据，不复制完整正文、不执行任务、不写项目或记忆。

## 二、边界决议

### 2.1 不新增知识事实源

`LibrarianRequest`、`LibrarianReceipt` 和候选排序结果是一次运行中的协议对象，不是
Memory Fact，不新增 `FactKind`，不改变 Revision/Lifecycle/Health/Usage。MEM-03D 才把
一次 Retrieval 的审计结果保存为 `RetrievalEvaluation` 与 Judgment。

### 2.2 固定快照，不读取未来 CURRENT

新增只读 `RetrievalContext`：

```go
type RetrievalContext struct {
    SchemaVersion int
    RetrievalID   string
    Project       *RetrievalScopeContext
    Global        *RetrievalScopeContext
}

type RetrievalScopeContext struct {
    Scope                 Scope
    ScopeID               string
    GenerationRef         ProjectGenerationRef // Global 使用对应联合分支
    RootIndexPath         string
    IndexTreePath         string
    InputManifestSHA256   string
}
```

实现可复用现有 `MemoryContext` 的 Project/Global Generation Ref，但路由路径必须来自该
Generation 自身，不能保存绝对路径为协议身份。某 Scope 没有有效 Generation 时对应字段
显式为 `null`。两侧都为空时仍返回合法 Context，状态为 `empty`，不得复用另一 Scope。

Context Builder 对每个 Scope 只读取一次 `CURRENT`，随后校验 Generation、永久 Manifest、
compiled output hash、`wiki/index.md` 和 `state/index-tree.json`。取得 Context 后，所有页面
读取必须通过其中的 Generation ID；后台切换 CURRENT 不影响当前 Retrieval。

### 2.3 不在 Go 中做语义分析

父 Agent 提供的 `task_summary` 和 Librarian 的 `why` 只传给模型和用户展示：

- 不写入 Memory Fact；
- 不进入任何知识 Content Hash；
- 不由 Go 枚举关键词、领域、失败类型或相似度；
- 错误信息不得回显完整任务摘要或理由；
- OMR 只验证长度、UTF-8、禁止凭据和结构化引用。

## 三、请求与回执协议

### 3.1 LibrarianRequest

```yaml
schema_version: 1
retrieval_id: retrieval_01K...
memory_context:
  project_generation_ref: { ... }
  global_generation_ref: { ... }
task_summary: "受限长度的当前任务摘要"
explicit_memory_refs: []
excluded_memory_refs: []
```

硬约束：

- `retrieval_id`、MemoryContext 和 Context Signature 均由 OMR 生成或校验；
- `explicit_memory_refs`/`excluded_memory_refs` 使用完整 MemoryRef，不使用标题、路径或
  Canonical Key 作为机器身份；
- 同一 MemoryRef 不得同时指定和排除；
- 第一版不提供 `include_frozen`。Frozen 只允许后续显式 Memory Review 流程读取；
- 请求不包含 API Key、完整 Prompt、完整命令、模型思考或项目外绝对路径。

### 3.2 LibrarianReceipt

```yaml
schema_version: 1
retrieval_id: retrieval_01K...
memory_context: { ... }
status: found             # found | no_candidate | unknown | unavailable
recommended_pages:
  - memory_ref: { ... }
    page_path: wiki/strategies/verify-before-upgrade-retry.md
    relevance_rank: 1
    why: "适用条件与当前任务匹配"
optional_pages: []
conflicts: []
visited_index_paths:
  - wiki/index.md
frozen_pages_used: []
requires_parent_read: true
```

`status` 语义固定：

| 状态 | 含义 |
|---|---|
| `found` | 至少一个候选已通过固定世界验证 |
| `no_candidate` | Librarian 在可用快照中明确未选择候选；不是全库不存在的 Oracle 结论 |
| `unknown` | 快照可用，但 Librarian 无法可靠判断适用性或冲突 |
| `unavailable` | 任一必需固定世界无法验证、已清理且无法重建，或结构化回执损坏 |

约束：

- `found` 必须至少有一个 recommended/optional page；其他状态不得夹带已采用候选；
- MemoryRef 五字段和 `page_path` 必须在对应固定 Generation 的 `index-tree.json` 中精确
  匹配；路径只能是 Generation 内安全相对路径；
- `relevance_rank` 从 1 连续递增，不允许重复、负数或跳号；它只表达 Librarian 语义顺序；
- `why` 是受限展示文本，不得包含正文、凭据、完整命令或模型思考；
- `frozen_pages_used` 在正常模式必须严格为空；
- `requires_parent_read` 必须为 true。候选回执不能冒充父 Agent 已读取正文；
- 推荐、可选和冲突集合按完整 MemoryRef 去重；交叉集合重复 fail closed。

## 四、固定世界页面读取器

新增只读 `GenerationReader`，只接受已验证的 `RetrievalScopeContext`：

```go
ReadIndex(ctx, scopeContext, relativePath) ([]byte, error)
ResolveMemoryPage(ctx, scopeContext, MemoryRef) (IndexEntry, []byte, error)
```

安全规则：

- 根锚定于 `generations/<generation-id>/`，逐组件拒绝 symlink、`..`、绝对路径、反斜杠和
  非普通文件；
- 只允许读取 Manifest 固定 Generation 的 `wiki/`、`state/index-tree.json` 及父 Agent
  明确请求核验的事实引用；
- 读取前后验证 Generation/compiled hash；不得在验证失败后自动切换 CURRENT；
- 页面大小沿用编译器上限；回执总 JSON 设独立安全上限，但不设置 Librarian 总页面数、
  总 Token 数或总导航层数预算；
- 跟踪 visited path，重复页面和路由循环稳定失败；
- 错误码固定、脱敏，不包含绝对路径、正文、task_summary、why 或凭据。

## 五、候选验证与确定性排序

流程固定为：

```text
Librarian 语义相关性顺序
→ 完整引用与固定 Generation 验证
→ applies_when / does_not_apply_when 结构化适用性
→ Scope + Pinned + Lifecycle
→ Health
→ Freshness
→ Usage Policy 对应证据强度与覆盖广度
→ SHA256(retrieval_id + scope_generation_id + memory_id) tie break
```

程序不重排不同 `relevance_rank`。只在相同 rank/同类候选中应用结构化排序。Scope 层级：

```text
用户明确指定
> Project pinned > Project active
> Global pinned > Global active
> Project probation > Global probation
```

规则：

- `degraded` 可以作为候选但低于同级 healthy，并必须在回执中可见；
- `needs_revalidation` 只能进入 optional/观察候选，不能覆盖主候选；
- 最多一个高度相关 probation 可作为 exploration candidate，但不占用或替换主候选；
- frozen/archived/superseded 不进入正常候选；
- Pinned 不得绕过用户排除、Scope 冲突、安全规则、适用条件或 frozen；
- Project 与 Global 发生已验证冲突时，同时返回冲突引用，当前任务优先 Project；不得静默
  修改 Global，也不得由程序猜测自然语言冲突；
- 所有排序输入来自固定 Generation 的派生状态，不能扫描更新后的 Fact Store。

## 六、omr-memory Profile

新增只读 Profile `omr-memory`：

- 安装为 `.reasonix/skills/omr-memory/SKILL.md`；
- 权限只允许读取 OMR 返回的固定 Generation 路由，不得写项目、Git、配置或 Memory；
- 必须先读 Project Root Index，再按需读 Global；不得直接全库扫描正文；
- 找到足够直接记忆、无未解决冲突且能说明适用原因后停止；
- 不把完整页面复制给父 Agent，只返回严格 `LibrarianReceipt`；
- 正常检索禁止读取 frozen 路径；
- 不调用另一个 Librarian，不递归触发自身；
- 父 Agent收到回执后必须亲自读取准备采用的页面。

Profile Prompt 与 Schema 示例作为 OMR asset 管理，纳入现有安装 Manifest/Hash/Doctor；升级
必须保留用户配置。MEM-03B 不实现自动任务开始 Hook，父 Agent通过现有 Reasonix Subagent
能力显式调用；方案 C 留待宿主稳定入口。

## 七、失败语义与幂等

- Context 构建失败不覆盖原始任务退出码；Librarian 返回 `unavailable`；
- 非法 JSON、未知字段、错误 Scope/Hash/Revision、路径断链和 frozen 泄漏 fail closed；
- 相同 `retrieval_id + memory_context + request hash` 重放得到字节一致的验证结果；
- 相同 retrieval_id 但请求或 Generation Pair 不同，返回稳定幂等冲突；
- Context/Receipt 本阶段不写事实，因此失败零持久化副作用；
- Context 取消立即停止读取，不产生半成品回执；
- Librarian/Reasonix 调用失败不得创建 MemoryUsage `adopted/affected/evaluated`。

## 八、TDD 测试矩阵

### 8.1 Context 与快照

1. Project+Global、仅 Project、仅 Global、两侧空四种合法 Context；
2. 每个 Scope 只读取一次 CURRENT；构建后切换 CURRENT 不影响读取；
3. Generation/Manifest/hash/compiler/index 缺失或漂移 → unavailable/fail closed；
4. 历史 Generation 已清理但永久 Manifest 可重建时仍可读取；不可重建不猜测；
5. Context Signature、retrieval_id 和 JSON 重放字节稳定；Project/Global 不混入。

### 8.2 页面与安全

6. Root→Local→Page 正常导航；500 条多层分片 fixture 可达；
7. `..`、绝对路径、symlink 文件/目录、非普通文件、超大文件拒绝；
8. route 断链、循环、重复页面和 index-tree 漂移拒绝；
9. 后台 CURRENT 切换和后续 Fact 写入不污染当前固定世界；
10. 错误不泄露绝对路径、任务摘要、理由、正文、命令或凭据。

### 8.3 Receipt 与排序

11. found/no_candidate/unknown/unavailable 状态矩阵；未知字段/错误联合拒绝；
12. MemoryRef 五字段、页面路径、Scope、Revision 与 index entry 精确匹配；
13. 正常模式 frozen/archived/superseded 永不进入候选；
14. relevance rank 优先，结构化同级排序与 tie key 可复现；
15. Project/Global 优先级、degraded、needs_revalidation、probation exploration；
16. 明确指定/排除、交叉集合重复、并列冲突与项目优先；
17. requires_parent_read 固定 true；回执不含完整正文；
18. 同请求插入顺序/map 顺序变化仍字节一致；不同 retrieval_id 只改变 tie break。

### 8.4 Profile 与集成

19. `omr-memory` asset 安装、升级、Manifest Hash、Doctor 与 profile list；
20. Profile 只读约束、禁止 frozen、禁止任务执行、禁止递归；
21. Fake Reasonix：父 Agent→Librarian→结构化回执→父 Agent读取页面的完整链路；
22. Librarian 失败不写 MemoryUsage、不改变原任务结果；零网络/零模型的 fixture 可复放。

## 九、实施顺序

1. 先冻结 Request/Context/Receipt Schema 与 legacy golden；
2. 实现只读 Context Builder 和 GenerationReader；
3. 实现 Receipt Validator、固定世界引用解析和候选排序；
4. 新增 `omr-memory` Profile asset、安装 Manifest 与 Doctor 检查；
5. 加 Fake Reasonix 端到端 fixture；
6. 运行全量门禁、review 和 security_review；
7. CTO 签收前不提交、不推送、不进入 MEM-03C。

MEM-03B 不实现：`omr memory context` CLI（MEM-03E）、Episode Card（MEM-03C）、
RetrievalEvaluation 持久化（MEM-03D）、MemoryUsage 捕获（MEM-04A）、自动 Hook/宿主事件、
Web 页面、向量数据库或 Embedding。

## 十、门禁

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

## 十一、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-03B：Librarian 渐进读取协议。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

先完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 8、11、16、17 章；
- docs/OMR_MNEMOSYNE_MEM-03B_LIBRARIAN_PLAN.zh-CN.md；
- docs/OMR_MNEMOSYNE_MEM-03A_INDEX_SHARDING_PLAN.zh-CN.md；
- docs/OMR_MNEMOSYNE_MEM-02A_USAGE_ANCHORS_PLAN.zh-CN.md；
- internal/memory/index_tree.go、evaluation_context.go、generation_store.go、
  derived_state.go、conflict_requirement.go、model.go 与对应测试；
- assets、internal/install、internal/doctor、Profile 相关实现与测试。

严格按 MEM-03B 计划第二～十章执行。先写失败测试证明固定快照、页面路径、frozen
隔离、结构化回执和排序边界，再做最小实现。

硬约束：
1. Librarian 是 Reasonix 只读 Subagent Profile；不复制 Agent Runtime，不执行任务，不写项目。
2. Context 必须同时固定 Project/Global Generation Pair；整个 Retrieval 禁止重读 CURRENT。
3. 程序不分析任务或记忆的自然语言语义，不引入枚举式语义分类、Embedding 或向量数据库。
4. 所有候选必须在固定 Generation 的 index-tree 中用完整 MemoryRef+路径精确验证。
5. 正常模式默认且强制排除 frozen/archived/superseded；本阶段不提供 include-frozen。
6. Librarian relevance rank 在前；OMR 只做适用条件、Scope、状态、证据和确定性 tie break。
7. 回执只提供引用和受限理由，不含完整正文、命令、Prompt、思考或凭据；父 Agent亲自读页面。
8. 不设置总页面数/Token/导航深度预算，但必须检测重复、循环、断链和已满足后的无目的浏览。
9. 失败不改变原任务结果，不写 adopted/affected/evaluated，不产生任何 Memory Fact 副作用。
10. 新增 omr-memory Profile，并接入安装 Manifest、升级保留、Doctor、profile list 与 Fake Reasonix fixture。
11. 不实现 CLI、Episode Recall、Retrieval Evaluation 持久化、MemoryUsage 捕获、Web 或自动 Hook。
12. 如计划与 Architecture v1 或冻结 Schema 冲突，停止相关实现并报告，不自行扩 Schema。

最终只输出：实际文件；修复前失败证据；Context/Request/Receipt Schema；固定 Generation
读取器；候选验证和排序；frozen/Scope/冲突隔离；omr-memory Profile；Fake Reasonix；
门禁、review、security_review；[ENV]/剩余问题；明确“未提交、未推送、未创建 Tag、
未进入 MEM-03C”。
```

## 十二、完成定义

- 同一 Retrieval 的 Project/Global Snapshot 固定且可重放；
- Librarian 候选全部能在固定世界中精确验证；
- 正常路径无 frozen 泄漏、无未来 CURRENT 污染、无路径逃逸；
- 语义判断与程序验证职责清晰，排序稳定且不引入第二事实源；
- omr-memory Profile 只读、可安装、可诊断；
- 全量门禁、独立 review 与安全 review 通过并经 CTO 签收。
