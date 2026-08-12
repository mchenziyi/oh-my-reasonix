# OMR Mnemosyne MEM-03A：确定性 Root/Local Index 与自动分片计划

- 阶段：MEM-03A
- 状态：✅ 已实现并通过门禁（2026-08-12）
- 前置：MEM-01A～MEM-02G 已实现并签收；Generation/OKF 编译与版本化
  `PolicyFact(index)` 已存在
- 目标：把现有浅索引骨架补齐为 Architecture v1 16.1 规定的、可重建、
  有界、无截断的渐进披露索引树，为 MEM-03B Librarian 提供稳定读取入口

## 一、当前基线与实际缺口

当前仓库已经具备：

- `PolicyConfigIndex` 的条目数、字节数、深度、维度顺序和 overflow bucket 配置；
- `DerivedStateResult.RootIndex/LocalIndex/GlobalIndex`；
- 固定排序、frozen/archived 默认排除；
- OKF 页面、`wiki/index.md`、Generation 原子发布与永久 Input Manifest。

但这些能力尚不等于 MEM-03A：

1. `buildIndexes` 只按条目数做一层分组，未按规范 UTF-8 渲染字节计数；
2. 超限 shard 被整体移入 `Overflow`，overflow 自身可能继续无限增长；
3. 没有按维度逐层递归 fan-out，也没有最大深度失败语义；
4. Root Index 仍直接携带完整 `entries`，没有只保留路由摘要；
5. OKF 编译器仍生成单一 `wiki/index.md`，没有 Root/Local 分片页面；
6. OKF 编译请求未显式绑定精确 `Index PolicyRef`，该 Policy 也未进入编译结果
   的 Manifest inputs；
7. 现有默认 `split_order` 省略 `component/operation`，与 Architecture v1 固定顺序
   不完全一致；
8. Architecture 示例使用 `_other`，而 MEM-01C 已冻结的安全标识符规则要求
   `overflow_bucket=other`。实现必须使用版本化 Policy 的实际值，不硬编码两者。

因此本阶段是对已有派生索引骨架的收敛和接入，不新增知识事实源。

## 二、冻结架构决议

### 2.1 索引始终是 Generation 内派生视图

Root/Local Index：

- 由固定 Generation 的 Memory Revision、Evidence、Derived State 和精确 Index Policy
  确定性编译；
- 不新增 `FactKind`，不写入 Memory Revision，不生成 Memory Mutation；
- 可删除并从永久 Generation Input Manifest 精确重建；
- 不能成为 Lifecycle、Health、Usage 或关系的事实源；
- 分片重排只产生新 Generation，不增加 Revision。

### 2.2 Index Policy 必须是显式、精确、可审计的输入

`OKFCompileRequest` 增加必填 `IndexPolicyRef PolicyRef`：

- 必须为 `policy_type=index`；
- 通过 `PolicyStore.GetPolicy(ref)` 按 id/type/hash 精确加载，不取“最新版本”；
- Policy Scope 必须与编译 Scope 一致；
- Policy Fact 作为 `ManifestInput` 纳入 `OKFCompileResult.Inputs`；
- Policy 缺失、Hash 漂移、类型错误、未来时间或 Scope 错配均 fail closed；
- 不提供隐式默认 Policy。现有 `defaultIndexPolicy` 只可保留给旧的纯派生 API，
  不得用于正式 Generation 编译。

### 2.3 单一索引编译器

新增一个纯函数索引编译核心，由以下两条路径共同调用：

```text
固定输入 Revision + Derived State + Index Policy
                      ↓
              DeterministicIndexTree
                 ↙             ↘
       DerivedState 兼容投影    OKF Markdown/JSON 输出
```

禁止让 `derived_state.go` 与 `okf_compiler.go` 各自维护一套分片算法。
现有 `RootIndexDoc/LocalIndexDoc` 可作为兼容投影，但不再承担递归树的规范表示。

## 三、派生输出协议

### 3.1 内部模型

```go
type IndexTree struct {
    SchemaVersion int
    Scope         Scope
    PolicyRef     PolicyRef
    Root          IndexNode
    FrozenCount   int
    ArchivedCount int
}

type IndexNode struct {
    Path       string
    Depth      int
    EntryCount int
    ByteCount  int
    Entries    []IndexEntry
    Routes     []IndexRoute
}

type IndexRoute struct {
    Dimension  string
    Value      string
    EntryCount int
    Path       string
}
```

硬约束：

- 叶节点只能含 `Entries`，分支节点只能含 `Routes`；
- Root 节点超限后只保存 shard 摘要、路由条件、条目数和链接；
- Route 按 `dimension → value → path` 稳定排序；
- Entry 延续既有 `indexEntryLess` 的全序，分片不得改变 Entry 身份和资格；
- `ByteCount` 是该节点最终规范 UTF-8 渲染结果的 `len([]byte)`，不是字段估算；
- 所有路径均由编译器生成，禁止用户路径输入、绝对路径、`..` 和 symlink。

### 3.2 Generation 输出路径

首版固定输出：

```text
wiki/index.md
wiki/index/<route...>/index.md
state/index-tree.json
```

- `wiki/index.md` 是 Root Index；未超限时可直接列 Entry，超限后只列 Route；
- Local Index 页面只列下一层 Route 或最终 Entry；
- `state/index-tree.json` 是同一树的机器可读派生投影；
- Markdown 与 JSON 必须来自同一 `IndexTree`，不得独立计算；
- 旧 `state/memories.json`、`state/relations.json` 与 Memory 页面保持原语义；
- `wiki/index.md` 的格式变化由 `OKFCompilerVersion` 升级承载，历史 Generation
  仍由旧 Compiler Version 重建。

### 3.3 Policy 字段映射

沿用 MEM-01C 已冻结字段，不新增第二套别名：

```yaml
max_entries_per_page: 200
max_page_bytes: 65536
max_shard_depth: 4
split_order:
  - component
  - operation
  - memory_type
  - stable_id_prefix
overflow_bucket: other
version: 1
```

Architecture v1 中的概念名 `max_entries_per_index/max_bytes_per_index/shard_order`
映射到冻结实现字段 `max_entries_per_page/max_page_bytes/split_order`。本阶段不修改
PolicyFact Canonical Schema。

`component/operation` 的值只允许来自已冻结、可验证的结构化条件；不能从标题、摘要、
模型文本或路径猜测。缺失该维度时使用 Policy 的 `overflow_bucket`。若当前事实没有可验证
值，该维度仍可形成稳定的 `component/other` 或 `operation/other` 路由，随后继续尝试下一维。

## 四、确定性 Fan-out 算法

对每个节点执行：

1. 按既有总序稳定排序 Entry；
2. 用最终 renderer 生成候选叶页面，计算 UTF-8 byte count；
3. 若 `entry_count <= max_entries_per_page` 且
   `byte_count <= max_page_bytes`，产出叶节点；
4. 否则从当前 split cursor 开始，选择第一个能把输入分成至少两个非空 bucket 的维度；
5. 缺失维度值进入 `overflow_bucket`，bucket 名经安全 ID 校验；
6. 对 bucket 递归执行相同步骤，cursor 只向后移动；
7. 若所有维度均不能产生有效分片，使用 `stable_id_prefix` 逐字符扩展前缀，直到满足
   Policy 或达到 `max_shard_depth`；
8. 达到最大深度仍超限，或单条 Entry 渲染后已超过 byte limit，返回稳定
   `memory_index_policy_unsatisfied`，不写任何 Generation 输出；
9. 禁止截断、丢弃、随机打散、按 map 遍历顺序输出或创建无界 overflow 页面。

“有效分片”定义为：至少两个非空 bucket，且每个 Entry 恰好进入一个 bucket。只有一个
`other` bucket 不算有效 fan-out，必须继续尝试下一维。

## 五、兼容与迁移边界

1. MEM-01F 的 Legacy `RootIndexDoc/LocalIndexDoc` JSON 字节和既有测试不直接改写；
2. 新递归树使用独立的 Generation 派生输出 schema，不冒充旧 `schema_version=1` 文档；
3. 正式 OKF Compiler Version 从 `/1` 升级到新注册版本，旧版本继续可重建；
4. `CompileOKF` 新调用必须传 IndexPolicyRef；仅测试 helper 和迁移适配器可显式构造
   首版 Policy Fact，禁止运行时静默默认；
5. frozen/archived 只计数，不进入普通 Root/Local Entry；Librarian 后续若需查看，走
   明确治理入口，不把它们混入正常路由；
6. Project/Global 各自独立编译、独立 PolicyRef、独立 Generation，禁止跨 Scope 合并树；
7. 本阶段不实现 Librarian、Episode Index、检索回执或模型调用。

## 六、安全与失败语义

- 编译器纯只读、无网络、无模型、无墙钟、无随机；
- 所有错误使用稳定脱敏码，不包含绝对路径、Prompt、命令、凭据或完整 Memory 正文；
- 输出路径必须通过 GenerationStore 既有安全写入和 symlink 拒绝；
- 对既有 Generation 的重放必须重算完整 compiled output hash；
- Index Policy 无法满足时整个编译失败，CURRENT、旧 Generation、Manifest 均不变；
- Context 取消应停止计算并返回稳定错误，不留下 staging 半成品；
- 任意 Entry 重复、路径碰撞、route 碰撞或输出文件碰撞均 fail closed；
- 不允许把 index summary 当成知识正文或反向修改 Revision。

## 七、TDD 测试矩阵

### 7.1 Policy 与固定输入

1. 精确 Index PolicyRef 加载、Scope/type/hash/version 错配拒绝；
2. Policy Fact 进入 Manifest inputs，清理后可按 Manifest 重建；
3. 无 PolicyRef 不得静默使用默认值；旧 Policy/Revision golden 不变；
4. 同 Facts + Policy + Compiler Version 重复编译字节与 Hash 完全一致。

### 7.2 条目数、字节数与分片

5. 恰好等于 entry limit/byte limit 通过，超过 1 自动分片；
6. UTF-8 中文、多字节字符按最终字节而非 rune 数计数；
7. Root 超限后只含 Route，不重复携带完整 Entry；
8. component → operation → memory_type → stable_id_prefix 的顺序固定；
9. 缺失维度进入 Policy `other`，单一 other 不算有效分片；
10. 递归多层 fan-out、稳定 prefix 扩展、路径与内容确定；
11. 单 Entry 超大、最大深度耗尽、不可分集合均稳定失败且零输出；
12. 每个 Entry 恰好出现一次，无截断、无重复、无遗漏。

### 7.3 兼容、隔离与安全

13. frozen/archived 排除并独立计数；Project/Global 不混入；
14. 旧 DerivedState 索引投影语义和 golden 不回归；
15. Markdown/JSON 从同一树产生并可相互校验；
16. 路径穿越、非法 bucket、碰撞、symlink 与损坏 Policy fail closed；
17. 插入顺序、map 顺序、goroutine/多进程编译不影响结果；
18. replay/recovery 检出页面删除、篡改、symlink 和 index-tree 漂移；
19. 错误脱敏、Context 取消、零网络/零模型调用；
20. Fake fixture 端到端：Policy + 500 条 Memory → 多层索引 → Generation commit →
    清理派生页 → 按 Manifest 字节级重建。

## 八、实现顺序与验证点

1. 先写 golden/limit 失败测试，证明当前浅分片会超限或产生无界 overflow；
2. 新增单一纯函数 `CompileIndexTree` 和稳定错误码；
3. 让旧 DerivedState 索引成为该树的兼容投影；
4. 扩展 OKFCompileRequest，精确加载 Index PolicyRef 并加入 Manifest inputs；
5. 输出 Root/Local Markdown 与机器可读树，升级 Compiler Version；
6. 接入 Generation replay/recovery 完整性校验；
7. 扩展 Consistency Doctor 与离线 Benchmark Fixture；
8. 运行全量门禁、独立 review 与 security_review；
9. CTO 签收前不提交、不推送、不进入 MEM-03B。

## 九、门禁

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

## 十、交给 Reasonix 的完整执行提示词

```text
执行 OMR Mnemosyne MEM-03A：确定性 Root/Local Index 与自动分片。

仓库：/Users/czy/Desktop/demo/oh-my-reasonix

先完整读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md 的 8、16.1、17、20 章；
- docs/OMR_MNEMOSYNE_MEM-03A_INDEX_SHARDING_PLAN.zh-CN.md；
- docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md；
- internal/memory/derived_state.go、policy.go、policy_store.go、okf_compiler.go、
  generation_store.go、generation_recovery.go、model.go 及对应测试。

严格按计划第二～九章执行。每个阶段先写失败测试，记录旧实现如何违反条目数、
UTF-8 字节、递归深度或 Manifest 绑定要求，再做最小实现。

硬约束：
1. Index 只能是 Generation 派生视图；不得新增 FactKind、Revision、Mutation 或第二事实源。
2. 只实现一个纯函数分片核心，DerivedState 与 OKF 输出必须复用，禁止双算法。
3. 正式编译必须显式精确加载 Index PolicyRef，并把 Policy Fact 加入 Manifest inputs；
   不得扫描最新 Policy 或静默使用默认值。
4. 以最终规范 UTF-8 渲染字节计算 max_page_bytes；entry 和 byte 上限必须同时满足。
5. 固定 split_order、稳定排序、稳定路径；无有效分片继续下一维，prefix 可确定性扩展。
6. 达到最大深度、单条超大或不可分时 fail closed；禁止截断、丢弃、随机或无界 overflow。
7. Root 超限后只保存路由摘要、条目数和链接；每个 Entry 在叶节点恰好出现一次。
8. frozen/archived 默认不进入正常索引；Project/Global 严格隔离。
9. 保留旧 Facts/Policy/Revision/Judgment golden；新输出通过 Compiler Version 演进。
10. 不实现 Librarian、Episode Index、CLI、Prompt 或模型调用；不进入 MEM-03B。
11. 错误稳定脱敏，输出路径复用 GenerationStore 安全边界；补 replay/recovery 篡改测试。
12. 扩展 Doctor 与离线 Benchmark，并运行全量门禁、review、security_review。

如计划与 Architecture v1 或已冻结 Canonical Schema 出现未登记冲突，停止相关实现并
报告，不自行扩 Schema。特别注意 Architecture 示例 `_other` 与冻结 Policy 字段
`overflow_bucket=other`：实现读取 Policy 实际值，禁止硬编码。

最终只输出：实际文件；修复前失败证据；IndexTree/输出协议；Policy/Manifest 绑定；
fan-out 算法；条目与 UTF-8 字节边界；兼容/golden；Generation replay/recovery；
Doctor/Benchmark；门禁、review、security_review；[ENV]/剩余问题；明确“未提交、
未推送、未创建 Tag、未进入 MEM-03B”。
```

## 十一、完成定义

- 精确 Index PolicyRef 与输出均进入 Generation 审计链；
- 所有 Root/Local 页面同时满足 entry 与 UTF-8 byte 上限；
- 同输入字节级确定、无截断、无遗漏、无重复；
- 最大深度无法满足时稳定失败且旧 Generation/CURRENT 不变；
- 派生输出可按永久 Manifest 精确重建；
- 全量门禁、review、安全 review 通过并经 CTO 签收。

## 十二、实现结果

- 新增单一确定性 `IndexTree` 编译核心，DerivedState 兼容投影和 OKF Generation
  输出复用同一棵树；
- 正式 `state/index-tree.json` 固定记录精确 `IndexPolicyRef`，Policy Fact 同时进入
  永久 Generation Input Manifest；
- OKF Compiler 升级为 `mnemosyne-okf-compiler/2`，`/1` 保留独立重建路径；
- Root/Local Markdown 与机器 JSON 同源，严格执行 entry、最终 UTF-8 byte 和 shard
  depth 三类上限；不可满足时返回 `memory_index_policy_unsatisfied`；
- frozen/archived 默认不进入正常索引，只保留审计计数；
- `DiagnoseIndexTree` 检查严格 JSON、Policy、计数、路由、可达性、重复与投影漂移；
- 500 条离线 fixture 覆盖多层分片、无遗漏、无重复和页面边界；Generation 既有
  compiled-output 完整性校验继续覆盖删除、篡改和 symlink；
- 固定 Manifest 重放只读取已登记派生事实，并按 Manifest `created_at` 恢复评估时间，
  后续事实不会污染历史 Generation。
