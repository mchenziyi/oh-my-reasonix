# OMR Mnemosyne 实现总 Todo

> 状态：MEM-01A 已完成并经 CTO 签收；MEM-01B 尚未开始
>
> 规范依据：[OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md](/Users/czy/Desktop/demo/oh-my-reasonix/docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md)

## 1. 目标与边界

Mnemosyne 是 OMR 的长期工程记忆层。Reasonix 继续负责 Agent 推理、工具调用、Session、Task 和代码修改；OMR 负责经验事实、记忆治理、检索路由、质量评估和可回滚演化。

本计划遵守以下边界：

- `internal/evolution` 保持 v2.0.x 兼容，继续负责 Episode、Pattern、Proposal、Observation 和 Evolution Overlay；
- 新增 `internal/memory` 作为 Mnemosyne 内核，不把旧 Evolution Store 直接改造成新事实层；
- CLI 使用 `omr memory ...`，Mnemosyne 是产品名，不作为 CLI 根命令；
- 第一阶段以项目 Scope 为实际运行范围，Schema 支持 Global，但 Global Promotion 延后；
- 旧 Evolution 数据先保留，迁移采用复制、校验和可回滚切换，不做破坏性搬迁；
- 不引入向量数据库、Embedding、GraphRAG、图数据库或云同步；
- 不修改 Reasonix 二进制、全局配置、API Key、PATH 或用户原始 Prompt；
- 在 MEM-06 真实 Benchmark 完成前，不宣称模型质量提升。

## 2. 总体状态

| 阶段 | 建议版本 | 状态 | 依赖 | 人工协助 |
|---|---|---|---|---|
| Architecture v1 | — | ✅ 已冻结 | Docs Gate | 不需要 |
| MEM-01 核心事实与 Generation | v2.1.0-alpha.1 | 🟡 进行中（MEM-01A 已签收） | Architecture v1 | 不需要 |
| MEM-02 Evolution 转换与 OKF 编译 | v2.1.0-alpha.2 | ⬜ 未开始 | MEM-01 | 不需要 |
| MEM-03 Librarian 与检索审计 | v2.1.0-beta.1 | ⬜ 未开始 | MEM-02 | 后期建议 |
| MEM-04 Lifecycle、归因与 Freshness | v2.1.0-beta.2 | ⬜ 未开始 | MEM-03 | 需要真实任务抽样 |
| MEM-05 修订、泛化与迁移 | v2.1.0-rc.1 | ⬜ 未开始 | MEM-04 | 需要跨项目验证 |
| MEM-06 Benchmark 与稳定发布 | v2.1.0 | ⬜ 未开始 | MEM-05 | 需要正式 A/B 测试 |
| WEB-01～03 本地记忆管理页面 | v2.2.x | ⏸ 后置 | MEM-06 | 需要桌面/浏览器联调 |

版本号是实施建议。每个阶段完成并经 CTO Review、全量门禁和用户确认后，才创建对应 Tag；不得提前创建 Release 或 Tag。

## 3. 统一执行协议

每个子任务由 Reasonix Agent 执行，执行前必须：

1. 运行 OMR Doctor，确认 OMR 已加载且无 blocking error；
2. 读取本 Todo、Architecture v1 和当前子任务指定章节；
3. 先写失败测试，再做最小实现；
4. 只修改当前子任务允许的路径，不提前实现后续阶段；
5. 不修改 Architecture v1；发现规格冲突时暂停并交 CTO Review；
6. 不创建第二事实源，不把缓存、Wiki、索引或 `state.json` 当作规范事实；
7. 不自行推送、创建 Tag、创建 Release 或开始下一子任务。

每个子任务完成后必须报告：

- 实际修改文件；
- 新增和修改的测试；
- 已知限制和未覆盖风险；
- 以下门禁结果：

```bash
gofmt -w .
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

若测试因沙箱端口、权限或外部环境失败，必须标记 `[ENV]`，不得伪造通过；生产代码失败标记 `[OMR]`，Reasonix 接口问题标记 `[HOST]`。

## 4. MEM-01：核心事实、Store 与 Generation

### MEM-01A：核心类型与严格 Schema

状态：✅ 已完成，CTO 已签收

新增建议路径：

```text
internal/memory/model.go
internal/memory/refs.go
internal/memory/conditions.go
internal/memory/policy.go
internal/memory/canonical.go
```

交付：

- Scope、Memory Type、Usage Policy；
- MemoryRef、EvidenceRef、JudgmentRef、ConfirmationSourceRef、PolicyRef；
- ApplicabilityCondition；
- MemoryRevision、MemoryEvidenceGeneration、GovernanceEvent；
- GenerationInputManifest；
- 严格 JSON 解码、未知字段拒绝、规范化序列化和程序计算 Hash。

验收：

- Type 与 Usage Policy 矩阵严格校验；
- `explicit_confirmation` 缺少合法 ConfirmationSourceRef 时拒绝；
- 自由文本机器条件、裸 ID、路径身份和错误 Hash 被拒绝；
- 相同事实确定性编码和 Hash 一致。

### MEM-01B：安全 Fact Store

状态：⬜

交付：

- 项目 Scope Store 和 Global Store 路径解析接口；
- `0700` 目录、`0600` 文件；
- 路径穿越、绝对路径注入和 symlink 逐组件拒绝；
- 不可变 Fact 写入；
- 相同 ID+Hash 幂等；相同 ID+不同 Hash 冲突；
- Project、Global、Portable Scope 隔离。

禁止：

- 修改旧 `internal/evolution.Store` 的历史布局；
- 通过文件名或 Markdown Path 充当机器身份；
- 允许调用方绕过 Schema 和脱敏写入。

### MEM-01C：Policy 与派生状态骨架

状态：⬜

交付首批版本化 Policy Fact：

- Freshness Policy；
- Trust Policy；
- Content Classifier Policy；
- Index Policy；
- Benchmark Policy。

本子任务只实现 Policy 保存、引用、Hash 和历史加载协议，不提前实现全部业务判断。

硬约束：所有 Lifecycle、Health、Usage Statistics、Relation Index、Root/Local Index 和 Web View 都必须标记为派生状态。

### MEM-01D：Generation 事务与唯一提交点

状态：⬜

交付：

```text
Fact
→ prepared transaction
→ staging Generation
→ Generation Input Manifest
→ 不可变 Generation
→ CAS 更新 CURRENT
```

必须覆盖：

- Scope 单写锁；
- Generation CAS；
- 幂等键先 Claim；
- prepared Fact 隔离；
- 崩溃恢复；
- Manifest 永久保留；
- 当前 Compiler/历史 Compiler 不可用时阻断重建；
- 任一步失败时旧 CURRENT 和旧 Generation 保持不变。

### MEM-01E：最小 OKF 编译器

状态：⬜

第一版只生成：

```text
memory/CURRENT
memory/index.md
memory/generations/<id>/generation.json
memory/generations/<id>/state/memories.json
memory/generations/<id>/wiki/index.md
memory/generations/<id>/wiki/<type>/<page>.md
```

验收：

- JSON Fact 是唯一内容事实源；
- Wiki 删除后可以确定性重建；
- 相同输入生成相同 Generation Hash；
- Markdown 不能反向修改 Fact；
- 页面包含 Scope、MemoryRef、Revision、EvidenceRef 和适用条件。

### MEM-01F：Doctor 与 Repair 基础

状态：⬜

Doctor 至少检查：

- Scope 和权限；
- Fact Schema、Hash 和未知字段；
- Ref 断链；
- Revision/Mutation 一致性；
- Generation Input Manifest；
- CURRENT 与输出 Hash；
- 孤立 staging、prepared transaction；
- Compiler、Canonicalization、Policy 历史版本可用性。

Repair 第一版只能重建派生状态、清理明确孤立的临时目录或生成修复计划，不得改写规范事实。

### MEM-01G：CLI 接入

状态：⬜

新增：

```text
omr memory status
omr memory doctor
omr memory list
omr memory show <id>
omr memory compile
omr memory index rebuild
omr memory index doctor
```

要求：`--project-dir`、`--scope`、稳定 JSON、写命令 `--dry-run`、稳定错误码，并保证 `omr evolve` 行为不变。

## 5. MEM-02：Evolution 转换、OKF 与 Trust Gate

### MEM-02A：旧 Evolution 只读迁移预览

状态：⬜

- 扫描旧 `episodes/patterns/proposals/observations`；
- 输出迁移计划和缺失字段；
- 不写入、不删除、不修改旧文件；
- 迁移前后 Hash 可比较；
- 单项目历史只能映射为 Project Scope。

### MEM-02B：事实复制迁移

状态：⬜

- 复制并校验旧 Episode 等事实到新 `facts/`；
- 创建迁移 Snapshot 和事务记录；
- 失败自动恢复；
- 旧布局继续保留；
- Doctor 发现孤儿、断链和不完整迁移。

### MEM-02C：MemoryMutationPlan 与候选 Revision

状态：⬜

- Reasonix 只输出结构化计划；
- Memory Service 重新计算 before/after Hash；
- `no_change` 不修改 Wiki；
- 新知识进入 `probation`；
- Evidence Generation 不可变；
- Candidate、Revision、Mutation 和 Evidence 的关系完整可追溯。

### MEM-02D：自动 probation 与 Trust Gate

状态：⬜

- 自动记录客观 Episode；
- 可靠新知识可生成 probation Revision；
- 外部内容先经过 Provenance、Content Classification 和 Evidence Trust；
- 不可信内容不能晋升为高影响策略；
- 不把模型声明当成事实或安全授权。

### MEM-02E：Prompt Composer 接入

状态：⬜

- System Prompt 只注入入口和读取协议；
- 不把全部记忆正文塞入 Prompt；
- Reasonix 返回结构化记忆读取/使用回执；
- Overlay 与 Mnemosyne Memory 保持职责分离；
- 读取失败不覆盖原始任务结果。

## 6. MEM-03：Librarian、渐进读取与检索审计

### MEM-03A：确定性 Root/Local Index 与分片

状态：✅ 已实现并通过门禁（见 `OMR_MNEMOSYNE_MEM-03A_INDEX_SHARDING_PLAN.zh-CN.md`）

- 使用版本化 Index Policy；
- 稳定排序和 UTF-8 字节计数；
- 超限自动分片；
- 不截断、不随机排序、不引入向量数据库；
- Index 是派生状态，可删除重建。

### MEM-03B：Librarian 协议

状态：✅ 已实现并通过门禁（见 `OMR_MNEMOSYNE_MEM-03B_LIBRARIAN_PLAN.zh-CN.md`）

- 固定 Project/Global Generation Pair；
- 先返回索引和页面引用，再由父 Agent 读取正文；
- Project 冲突记忆优先于 Global；
- 默认跳过 frozen 页面；
- 找不到适用记忆时明确返回 `unknown`/`unavailable`。

### MEM-03C：Episodic Recall

状态：✅ MEM-03C-01～04 自动化实现完成，待真实 Reasonix Desktop 联调（见
`OMR_MNEMOSYNE_MEM-03C_EPISODIC_RECALL_PLAN.zh-CN.md`）

- Episode Card 和 Episodic Index 只能由规范事实派生；
- 不保存完整命令、思考、凭据或无必要轨迹；
- 命中只记录 `retrieved/read`，不能直接计入 help/harm；
- 删除派生卡片后可重建。

### MEM-03D：Retrieval Evaluation

状态：⬜

- 固定原 Retrieval 的 Project/Global Generation Pair；
- 支持 Oracle、Critic、User Review 的 Judgment 来源区分；
- `missed_relevant` 只产生索引/别名/路由修复候选；
- 候选世界不可重建时返回 `unavailable`；
- 审计失败不惩罚 Memory，不改变原任务退出码。

### MEM-03E：检索回放与 CLI

状态：⬜

新增：

```text
omr memory context
omr memory episode list
omr memory episode show <episode-id>
omr memory retrieval audit <retrieval-id>
```

## 7. MEM-04：Usage、Outcome、Lifecycle 与 Freshness

### MEM-04A：结构化 MemoryUsage 回执

状态：✅ 自动化实现完成，待真实 Reasonix Desktop 回执联调（见 `OMR_MNEMOSYNE_MEM-04A_USAGE_CAPTURE_PLAN.zh-CN.md`）

- Reasonix 通过 Prompt Protocol 返回结构化回执；
- OMR 只记录客观事件、引用和时间；
- `adopted`、`likely`、未评估状态不计入正负统计；
- Provenance 不完整时不得计分。

### MEM-04B：Attribution Analyst 与 Outcome

状态：✅ 自动化实现完成，待真实 Reasonix Desktop Attribution 回执联调（见 `OMR_MNEMOSYNE_MEM-04B_ATTRIBUTION_OUTCOME_PLAN.zh-CN.md`）

- 模型分析只输出候选归因；
- OMR Attribution Gate 校验上下文、证据和任务边界；
- 人工 Override 追加 Judgment Fact，不覆盖原记录；
- 同一 Root Task 的 Retry 不重复计数；
- 第三方失败不能直接冻结记忆。

### MEM-04C：Lifecycle、Health 与冻结

状态：🟡 自动化实现基本完成，待真实 Reasonix Desktop 联调（见
`OMR_MNEMOSYNE_MEM-04C_LIFECYCLE_GOVERNANCE_PLAN.zh-CN.md`）

- Lifecycle、Health、Pinned、Archived 均为派生状态；
- Freeze/Unfreeze/Archive 必须追加 Governance Event；
- 冻结记忆默认不参与普通读取；
- 不物理删除失败记忆；
- 恢复必须满足 Architecture v1 的受约束条件。

### MEM-04D：Freshness/Revalidation

状态：⬜

- Freshness 单独记录 Judgment、时间、PolicyRef 和依据；
- 时间流逝不直接产生 frozen/superseded/archived；
- `needs_revalidation` 进入候选，不进入普通高优先采用；
- Revalidation 结果生成新 Evidence Generation 或 Judgment。

## 8. MEM-05：修订链、泛化、迁移与维护

### MEM-05A：Revision/Merge/Split/Generalize

状态：✅ 只读计划层已实现并通过门禁；规范事实写入与批准流程仍待后续阶段（见
`OMR_MNEMOSYNE_MEM-05A_REVISION_MERGE_GENERALIZE_PLAN.zh-CN.md`）

- Revision 不原地覆盖；
- Merge 主 ID 使用证据链完整度、创建时间、稳定 ID 排序；
- Generalize 创建新的 Global Memory ID；
- 保留 `generalized_from`；
- 不把项目记忆移动或淘汰。

### MEM-05B：Global Promotion

状态：✅ 只读 PromotionPlan/Gate 已实现；Global Revision 写入与批准流程仍待后续阶段（见
`OMR_MNEMOSYNE_MEM-05B_GLOBAL_PROMOTION_PLAN.zh-CN.md`）

- Schema 先支持 Global，实际 Promotion 仍按 Usage Policy 分流；
- 单项目不能生成 Global Active；
- Project Family 使用不可逆指纹去重；
- 不保存 Remote、路径、项目名等敏感身份；
- Generalizer/Critic 只能返回结构化计划。

### MEM-05C：Revalidation、Repair、Rollback

状态：⬜

- 历史 Policy/Evaluator 可重建；
- 损坏 Generation 可由永久 Fact 和 Input Manifest 重建；
- 回滚只切换 CURRENT，不删除规范事实；
- 所有 Repair 有审计记录；
- 跨 Scope 操作显式指定 Scope。

### MEM-05D：迁移切换

状态：⬜

- 迁移采用预览 → Snapshot → 复制 → 编译 → Doctor → 切换；
- 失败自动恢复旧读取入口；
- 旧 Evolution 数据仍可读取；
- 迁移不伪造缺失 Usage、Lifecycle 或 Health；
- 多项目数据不能通过单项目迁移自动生成 Global。

## 9. MEM-06：Memory Quality Benchmark

状态：⬜

冻结以下内容后再运行：

- Fixture 集；
- Reasonix/OMR 版本；
- 模型和温度；
- 重复次数；
- Oracle 和评分器；
- Recall@K、Wrong Adoption、Downstream Success 等门槛。

对照组固定为：

```text
Reasonix + Mnemosyne
vs
Reasonix without Mnemosyne
```

必须分别报告 Retrieval、Reading/Adoption、Downstream Task 三层指标；样本不足或无法配对时标记 `insufficient_evidence`，不得宣称通用模型能力提升。

## 10. 后置功能

以下不进入 MEM-01～MEM-06：

- WEB-01：本地只读列表、详情、关系图谱；
- WEB-02：人工治理、审计、Snapshot、回滚；
- WEB-03：完整本地记忆管理页面；
- 跨设备同步、云端服务、公共记忆市场；
- 自动修改 Reasonix/OMR 源码；
- 自动批准高影响 Overlay。

## 11. CTO 放行规则

每完成一个子任务，必须经过以下检查后才能进入下一个：

- 代码 Review：是否违反 Architecture v1；
- 安全 Review：路径、symlink、权限、敏感信息、Scope；
- Schema Review：未知字段、Hash、Ref、版本化；
- 事务 Review：幂等、CAS、崩溃恢复、零部分写入；
- 测试 Review：失败测试是否真实覆盖不变量；
- 文档 Review：示例、状态转换表、Docs Gate 是否一致。

以下任一情况立即暂停并回到 CTO：

- 需要修改 Architecture v1；
- 需要新增第二事实源；
- 需要修改 Reasonix 官方接口；
- 需要真实正式项目或真实 API Key；
- 需要用户确认跨项目/全局数据边界。

## 12. 唯一人工联调阶段

在 MEM-01～MEM-05 自动门禁通过前，不需要用户操作正式项目。

进入 MEM-06 前，用户协助完成：

1. 创建隔离临时项目；
2. 运行 Mnemosyne 开关和 Doctor；
3. 产生固定 Fixture 任务；
4. 检查 Project/Global Scope 隔离；
5. 验证记忆被检索、读取和回执记录；
6. 执行一次冻结、修订、回滚；
7. 运行 Mnemosyne/Native 配对 Benchmark。

在 MEM-06 前不读取用户正式项目，不提交真实模型思考或凭据，不创建稳定 Release。

## 13. 当前执行顺序

```text
MEM-01A → MEM-01B → MEM-01C → MEM-01D
                                      ↓
MEM-01E → MEM-01F → MEM-01G
                                      ↓
MEM-02A → MEM-02B → MEM-02C → MEM-02D → MEM-02E
                                      ↓
MEM-03A → MEM-03B → MEM-03C → MEM-03D → MEM-03E
                                      ↓
MEM-04A → MEM-04B → MEM-04C → MEM-04D
                                      ↓
MEM-05A → MEM-05B → MEM-05C → MEM-05D
                                      ↓
MEM-06 Benchmark → CTO Review → 用户联调 → v2.1.0
```

本文件只定义执行顺序，不替代 Architecture v1 的 Schema 和不变量。两者冲突时，以 Architecture v1 为准，并暂停开发提交 CTO Review。
