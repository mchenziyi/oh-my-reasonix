# OMR Mnemosyne MEM-01F：派生状态与索引

- 阶段：MEM-01F
- 状态：已由 Reasonix Agent 执行完成（MemoryUsage/Outcome 两类事实扩展、DeriveState 派生引擎、稳定排序、Root/Local/Global 索引与 OKF 快照适配器已交付；未进入 MEM-02）
- 前置：MEM-01A～MEM-01E 已签收
- 后续：MEM-02 评估与 Usage

## 一、目标

在不新增第二事实源的前提下，从已持久化的 Revision、Evidence Generation、Judgment、Policy、Governance Event、MemoryUsage 和 Outcome 事实，确定性计算：

- Lifecycle、Health、Freshness、Pinned/Archived 派生状态；
- Usage 统计与 Policy 分流后的排序字段；
- Project/Global Root/Local 索引；
- 可供 MEM-01E OKF 编译器消费的 `DerivedMemoryState` 快照。

所有结果都是可重建派生数据，不改变 Revision，不原地修改历史 Fact，不创建新的规范 `state.json`。

## 二、严格范围

允许修改：

- `internal/memory/**`
- 本计划文档状态

禁止修改：

- Architecture v1、MEM-01A～MEM-01E 已签收协议；
- `internal/evolution`、`cmd/omr`、Prompt、assets、`.reasonix`；
- Reasonix、Desktop、网络、Embedding、向量数据库；
- 真实模型调用和自动批准。

## 三、冻结契约

### 3.1 事实源与确定性

- 事实源只允许使用 FactStore 中已通过完整校验的记录。
- Revision 是知识内容事实；Mutation/Usage/Outcome/Judgment/Governance 是审计与治理事实。
- 派生状态、统计、索引、排序键和 Web/OKF 视图不得反向写入规范 Fact。
- 同一 Scope、同一输入集合、同一 Policy/Compiler 版本必须产生字节稳定的结果。
- 缺失、损坏、Hash 漂移、跨 Scope、未知 Schema 或无法解释的记录必须 fail closed，并返回脱敏稳定错误。

### 3.2 Lifecycle 与 Health

- 按架构 11 章的 Usage Policy 分流计算，不混用 `outcome_attributed`、`evidence_validated`、`explicit_confirmation` 的证据。
- Lifecycle、Health、Freshness、Pinned、Archived 是独立维度；Freshness 不得产生 frozen/superseded/archived。
- Governance Event 是唯一改变冻结、恢复、置顶和归档意图的事实；不得直接修改派生快照。
- 未满足证据条件时返回明确的 `probation`/`degraded`/`needs_revalidation`，不得猜测为 active。
- 冻结记忆默认不进入正常读取索引，但历史事实和显式人工查询仍可访问。
- `outcome_attributed` 阈值（CTO Review 冻结，架构 11.5）：
  - 初始：`probation + healthy`；
  - Active：至少 3 次 `counted_as_help`，且至少 2 个独立 Episode/Root Task，且无未解决 `counted_as_harm`；
  - Degraded：首次经过有效归因的 harm；
  - Frozen（自动）：至少 3 次 `counted_as_harm` 且 negative rate >= 60%；
  - `external_failure=true` 只计入 `usage_count`，不计 help/harm，不触发 degraded/frozen；
  - 成功不抵消失败；证据不足保持 `probation`。
- Health 与 Lifecycle 独立：`probation`/`active`/`frozen`/`superseded`/`archived` 不因生命周期名称自动 degraded，仅当存在经过验证的负面证据（非 external 的 harmed Outcome，且其 usage_stage 为 `affected`/`evaluated`，与归因证据同口径）时 health=degraded；支持 `active+healthy`、`active+degraded`、`frozen+healthy`、`frozen+degraded` 等独立组合。
- **状态转换矩阵（架构 11.5，CTO Review 冻结）**：

| Usage Policy | 初始 | Active | Degraded | Frozen |
|---|---|---|---|---|
| `outcome_attributed` | probation/healthy | ≥3 `counted_as_help` 且 ≥2 独立 Episode 且无未解决 `counted_as_harm` | 首次有效归因 harm | 当前 Revision ≥3 `counted_as_harm` 且 negative rate ≥60%（自动）；`manual_freeze`（意图） |
| `evidence_validated` | probation/healthy | 恒 probation（Critic Judgment subtype 未注册，协议缺口，见"未决协议项"） | 未启用（同 Critic 缺口） | 未启用 |
| `explicit_confirmation` | probation/healthy | `confirmation_source_ref` 指向的 Judgment 可验证且为 confirmed | 确认 Judgment 不可验证（ref 指向不存在）或存在冲突 | 确认 Judgment 明确 revoked 且无替代 Revision；有替代 Revision 时旧 Revision 按 superseded 处理；supersede 链解析到 revoked 同样 frozen |

### 3.3 Usage 统计与优先级

- 仅从合法 MemoryUsage、Outcome 和 Attribution Judgment 计算 `usage_count`、`counted_help_count`、`counted_harm_count`、`last_used_at`。
- MemoryUsage 使用阶段（架构 12.1：`retrieved → read → adopted → affected → evaluated`）在本阶段以严格枚举字段 `usage_stage` 落实：仅 `affected`/`evaluated` 阶段产生 attribution 证据，其余阶段计入 usage 与 recency 但不归因；字段进入 Canonical Hash、FactStore 路由、Scope 校验、幂等与测试。
- Active 阈值要求的"独立 Episode/Root Task"以严格不可变字段 `episode_id` 落实（受控标识，非 `usage_id` 伪装）；Episode 规范事实集尚未实现，本阶段只做标识符格式校验、不做存在性校验（见"未决协议项"）。
- 第三方失败不得自动归因于记忆；成功不抵消失败，失败也不直接删除记忆。
- 同一 usage 事件必须幂等；重复事实不得重复计数。
- 优先级排序顺序固定为：适用条件/Scope → Lifecycle/Health → Usage Policy 所需证据强度 → 成功复用次数 → 最近使用时间 → 稳定 ID；完全同级时使用可复现确定性随机种子，禁止机器时间或全局随机源。
- 统计不足时输出 `insufficient_evidence`，不得生成模型能力提升结论。

### 3.4 索引

- 生成 Project Root Index、Local Index、Global Index 的内存表示和确定性序列化；不引入向量检索。
- 索引条目只包含 MemoryRef、RevisionRef、Scope、类型、Canonical Key、派生状态摘要和页面路径。
- 索引不得包含 Prompt、命令正文、思考、凭据、绝对路径或未脱敏错误摘要。
- 采用 MEM-01C 的版本化 Index Policy；分片、排序、overflow bucket 和碰撞规则必须复用 PolicyFact。
- 索引缺失或损坏时可从规范事实重新生成；不得把已有索引当作事实来源。

## 四、实现分步

### MEM-01F-01：事实读取与派生输入集合

- 定义 `DerivedStateRequest`、`DerivedMemoryState`、`UsageStats` 和 `IndexEntry`。
- 通过 FactStore 读取并按 Scope 隔离 Revision、Evidence、Judgment、Governance、Usage、Outcome。
- 对同一事实 ID 的重复/冲突、未知类型、Hash 漂移和跨 Scope 增加失败测试。

### MEM-01F-02：Lifecycle/Health/Freshness 计算

- 实现纯函数 reducer，按固定时间点和 Policy 计算派生状态。
- 覆盖 probation、active、degraded、frozen、superseded、archived、needs_revalidation、pinned 等组合。
- 治理事件顺序、撤销/恢复链、非法转换和冻结保护必须 fail closed。

### MEM-01F-03：Usage 统计与优先级

- 实现三类 Usage Policy 的证据分流和计数器。
- 实现 usage/attribution 幂等、第三方失败不归因、成功不抵消失败。
- 输出稳定排序键和统计不足标记，不改变知识内容 Hash。

### MEM-01F-04：Root/Local/Global 索引

- 生成确定性索引条目、分片和页面引用。
- 校验索引中的 Revision、Scope、Canonical Key、Generation 和页面路径均真实存在。
- 覆盖项目/全局隔离、冻结默认跳过、Pinned 提升但不越过安全约束、空索引和碰撞场景。

### MEM-01F-05：与 OKF Generation 的只读接入

- 为 MEM-01E 编译器提供只读 `DerivedMemoryState` 输入适配器。
- 不绕过 GenerationStore，不改变 Manifest/CURRENT/事务顺序。
- Derived state 变更必须导致可解释的编译输入或输出 Hash 变化；失败时旧 Generation/CURRENT 不变。

### MEM-01F-06：恢复、安全与确定性门禁

- 覆盖损坏 Fact、路径穿越、symlink、权限异常、敏感信息、重复事件、并发读和多进程 Scope 隔离。
- 同输入重复计算字节一致；删除全部派生状态后可重建。
- 不做真实模型调用，不宣称质量提升，不接 CLI。

## 五、验收门禁

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

质量基准若因端口、缓存或临时目录清理失败，必须标记 `[ENV]`，不得伪造通过。

## 五之一、CTO Review 回归测试矩阵（新增）

1. 0 help → `probation + healthy`；
2. 1 help → 仍不是 active；
3. 2 help 但仅 1 个 Episode → 仍不是 active；
4. 3 help + 2 个独立 Episode → `active + healthy`；
5. 1 harm → `degraded + degraded`；
6. 3 harm 且 negative rate >= 60% → 自动 frozen 且排除出正常索引；
7. external failure 不触发 degraded/frozen、不计 help/harm、仍计 usage；
8. 成功不抵消历史失败（3 help + 1 harm → degraded）；
9. Health 独立组合：`active+degraded`（Override 修正后历史 harm 保留负面证据）、`frozen+healthy`（Governance 冻结无负面证据）；
10. 重复 usage/outcome 幂等不重复计数；
11. Attribution Override supersede 链仍正确；
12. Scope、revision、孤儿引用仍 fail closed；
13. 同一输入重复派生字节完全一致；
14. `usage_stage` 过滤：retrieved/read/adopted 计入 usage 但不归因。

## 六、交付限制

- 不修改已冻结 Architecture/Schema；若发现协议矛盾，停止并报告，不自行扩展。
- 不进入 MEM-02，不实现 Retrieval Evaluation、Evidence Trust、模型调用或 Reasonix 接入。
- 不提交、推送、创建 Tag；由 CTO Review 后决定。

## 六之一、未决协议项（CTO Review 记录）

1. **Episode 规范事实未实现**：Active 阈值要求"至少 2 个独立 Episode/Root Task"。Episode 事实集（`facts/episodes/`）属后续阶段，本阶段以严格不可变字段 `episode_id`（MemoryUsage 上，受控标识）承载独立性信号，只校验标识符形状、不校验 Episode 存在性；Root Task 语义由 Episode 承载。待 Episode 事实落地后需补充存在性与跨 Episode 引用校验。
2. **MemoryUsage 使用阶段已纳入本阶段**：`usage_stage` 严格枚举（`retrieved|read|adopted|affected|evaluated`）落实架构 12.1 五阶段；仅 `affected`/`evaluated` 产生 attribution 证据。字段进入 Canonical Hash、FactStore 路由、Scope 校验、幂等与测试。
3. **Health 语义**：Health 与 Lifecycle 独立，仅由"经过验证的负面证据"（非 external 的 harmed Outcome 且 usage_stage 为 affected/evaluated，含被 Override 修正的历史 harm）决定 degraded；生命周期名称本身不降级 Health。
4. **Critic Judgment subtype 未注册（协议缺口）**：架构 11.5 要求 `evidence_validated` 晋升 Active 必须"Critic 通过"，且架构 6.2.3 明确 Critic Judgment 需"对应协议定义固定 subtype payload"；MEM-01A 未注册该 subtype，本阶段按 CTO 决策**不新增 subtype**——`evidence_validated` 恒保持 probation（缺 Critic 条件），degraded/frozen 也未启用。待后续阶段注册 Critic 协议后解除。
5. **`root_task_refs` 已就绪**：`MemoryEvidenceGeneration` 新增严格不可变字段 `root_task_refs []string`（受控标识、去重、进 Canonical Hash/路由/校验/幂等），承载"独立 Root Task/正式来源"信号；架构 11.5 的 ≥2 独立来源条件可在 Critic 协议落地后启用。

## 七、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-01F：派生状态与索引。

先读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01D_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01E_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01F_PLAN.zh-CN.md
- internal/memory/**

只修改 internal/memory/** 及本计划文档状态。禁止修改 Architecture v1、MEM-01A～MEM-01E、internal/evolution、cmd/omr、Prompt、assets、.reasonix；禁止 CLI、Reasonix、Desktop、网络、Embedding、向量数据库、真实模型调用和自动批准。禁止提交、推送、创建 Tag，不得进入 MEM-02。

先写失败测试，再实现最小代码。实现从 FactStore 规范事实确定性派生 Lifecycle、Health、Freshness、Pinned/Archived、UsageStats、稳定优先级和 Project/Local/Global Index；所有派生数据可删除并从事实重建，绝不创建第二事实源或修改 Revision。

严格遵守：Scope 隔离；Usage Policy 分流；第三方失败不自动归因；成功不抵消失败；重复 Usage 幂等；冻结默认不进入正常索引；治理事件是唯一治理意图事实；Freshness 不产生 frozen/archived；统计不足输出 insufficient_evidence；所有错误脱敏且 fail closed。

复用 MEM-01B～01E 的 FactStore、PolicyStore、GenerationStore、Hash、锁、路径安全和恢复语义，不绕过事务。索引只输出 MemoryRef/RevisionRef/Scope/类型/Canonical Key/派生摘要/页面路径，不写 Prompt、命令、思考、凭据、绝对路径。

覆盖：事实损坏/Hash 漂移/未知字段/跨 Scope、非法治理转换、冻结保护、三类 Policy、Usage 幂等、第三方失败、稳定排序、空索引、Project/Global 隔离、symlink/权限/路径穿越、删除派生状态后重建、并发读、多进程隔离。

运行 gofmt、git diff --check、memory 包测试和 race、全量 go test、go vet、go build ./cmd/omr、tests/docs_check.sh。环境失败如实标记 [ENV]。完成后只报告修改文件、事实输入、派生输出、失败矩阵、门禁和未进入 MEM-02，不提交或推送。
```
