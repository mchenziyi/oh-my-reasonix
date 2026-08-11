# OMR Mnemosyne MEM-02A：MemoryUsage 评估锚字段实现计划

- 阶段：MEM-02 Protocol Implementation / Phase A
- 状态：✅ 已完成（2026-08-11）——MemoryContext、ObservationProvenance 与 MemoryUsage 锚字段已实现并经 TDD 验收、独立 review 与 security review（无阻塞）；门禁全过；未进入 MEM-02B
- 前置：MEM-01A～MEM-01F、MEM-02-01/02/06/07/08 已签收；MEM-02 Schema Gate 已 PASS
- 目标：为每条可评估的 `MemoryUsage` 固定 Retrieval、Root Task、Project/Global Generation、Context Signature 与 Observation Provenance，使后续 Critic、Retrieval Evaluation 和 Context Applicability 能引用同一历史世界。
- 非目标：本阶段不实现 Critic、Evidence Trust、Retrieval Evaluation、Context Applicability 或 Conflict Fact，不改变 `evidence_validated` 恒 `probation` 的现状。

## 一、为什么先做这一阶段

MEM-02 后续协议都需要回答三个问题：

1. 这条 MemoryUsage 属于哪一次 Retrieval 与 Root Task；
2. 当时读取的是哪一对 Project / Global Generation；
3. “使用了这条记忆”是 Agent 回执、Runtime 事件还是用户确认，证据在哪里。

如果没有这些锚点，后续评估只能依赖当前 `CURRENT`、`usage_id` 或自由文本推断，会破坏历史可重放性。因此本阶段只补齐客观关联键，不做任何语义判断。

## 二、冻结 Schema

### 2.1 MemoryContext

新增可复用严格值对象：

```yaml
memory_context:
  project_generation_ref: ProjectGenerationRef | null
  global_generation_ref: GlobalGenerationRef | null
```

约束：

- 复用 `MEM-02-01` 已实现的 `ProjectGenerationRef` / `GlobalGenerationRef`，不创建通用 `GenerationRef`；
- 两侧允许任意一侧为 `null`，但不能同时为 `null`；
- Project Ref 必须为 project scope，Global Ref 必须为 global scope；
- 不读取 `CURRENT`，不验证“是否最新”，只验证引用结构；
- Canonical 表示固定输出两个键，缺失一侧显式为 `null`。

### 2.2 ObservationProvenance

新增可复用严格值对象：

```yaml
observation_provenance:
  source: agent_reported | runtime_observed | user_confirmed
  evidence_ref: EvidenceRef | null
  judgment_ref: JudgmentRef | null
```

来源矩阵：

| source | evidence_ref | judgment_ref | 规则 |
|---|---|---|---|
| `agent_reported` | 必填 | 必须为 null | Evidence 必须承载结构化 Reasonix Event 回执；本阶段只校验 `EvidenceRef` 结构，不猜测正文 |
| `runtime_observed` | 必填 | 必须为 null | Evidence 必须引用 Reasonix 公开 Runtime Event；本阶段只校验 `EvidenceRef` 结构 |
| `user_confirmed` | 可选 | 必填 | Judgment 必须是 `confirmation`；Evidence 只能作为补充，不能替代 Judgment |

共同约束：未知 source、错误引用类型、路径型 ID、错误 Hash、空必填引用全部 fail closed；不得加入命令、Prompt、思考、凭据或自由说明文本。

### 2.3 MemoryUsage 新增字段

在现有 `MemoryUsage` 上追加：

```yaml
retrieval_id: controlled_id
root_task_id: controlled_id
memory_context: MemoryContext
context_signature_version: 1
context_signature: sha256_...
context_descriptor_ref: controlled_id
observation_provenance: ObservationProvenance
```

说明：

- `context_descriptor_ref` 是 Architecture v1 12.2 已冻结的 Context Signature 可解释性锚点；Protocol Extension 6.2 的规则已引用该字段，本计划把 YAML 示例中的遗漏显式补齐；
- 不用 `usage_id` 或 `episode_id` 代替 `retrieval_id` / `root_task_id`；
- 同一 Retrieval 的 `retrieved/read/adopted/affected/evaluated` 回执使用同一 `retrieval_id` 与同一 `memory_context`；
- 本阶段不扫描其他 Usage 来强制跨记录相等；该一致性属于后续只读 Doctor/评估器；
- `context_signature_version` MVP 只接受 `1`；`context_signature` 必须为合法 SHA-256；`context_descriptor_ref` 必须为受控 ID；
- 新锚字段全部进入 Canonical Bytes 与 `content_sha256`。

## 三、Legacy 兼容与 Canonical Hash

这是本阶段最高优先级兼容要求。

### 3.1 两种合法形态

`MemoryUsage` 只允许以下两种形态：

1. **Legacy**：上述所有新增锚字段均缺失/零值；
2. **Anchored**：上述所有必填锚字段完整存在，并通过严格验证。

部分存在属于损坏输入，必须 fail closed。不能自动补默认 Retrieval、Generation、Context 或 Provenance。

### 3.2 旧 Fact 字节与 Hash 不得变化

- Decode 旧 JSON 后必须仍能 `Validate`；
- Legacy `canonMap` 必须保持旧字段集合，不得把新增字段以空字符串、`null` 或空对象写回；
- 旧 Fixture 的 Canonical Bytes 与 Content Hash 必须逐字节不变；
- Anchored Fact 才输出完整新增字段；
- `schema_version` 继续为 1，不迁移、不覆盖旧 Fact。

### 3.3 派生状态限制

- Legacy Usage 继续进入基础 `usage_count` / `last_used_at`；
- Legacy Usage 不得参与 Retrieval Evaluation、Critic、跨 Context 归因；
- 本阶段不得改变现有 Lifecycle / Health / help / harm 计算结果；
- 不因缺锚字段冻结、删除或重写旧 Usage。

## 四、实现范围

允许新增或修改：

- `internal/memory/memory_usage.go`
- `internal/memory/refs.go` 或一个最小的新文件（仅放 `MemoryContext` / `ObservationProvenance`）
- `internal/memory/*_test.go`
- 本计划文档状态行
- 如确有必要，修正 Protocol Extension 6.2 YAML 示例，补上已经被正文规则引用的 `context_descriptor_ref`

禁止修改：

- Architecture v1；
- MEM-01A～MEM-01F 已冻结字段、枚举和既有语义；
- `internal/evolution`、`cmd/omr`、Prompt、assets、`.reasonix`；
- Reasonix、Desktop、CURRENT、Generation Commit、Revision；
- Critic、Trust、RetrievalEvaluation、ContextApplicability、Conflict Fact；
- 网络、模型调用、Embedding、向量数据库；
- 提交、推送、Tag、Release。

## 五、TDD 验收矩阵

必须先写失败测试，再做最小实现。

### 5.1 MemoryContext

- project-only、global-only、project+global 均合法；
- 两侧同时缺失拒绝；
- Project/Global scope 互换拒绝；
- Generation ID、Manifest ID、Hash 非法拒绝；
- 字段顺序变化不影响 Canonical Bytes。

### 5.2 ObservationProvenance

- 三种 source 的合法组合通过；
- `agent_reported/runtime_observed` 缺 Evidence、携带 Judgment 拒绝；
- `user_confirmed` 缺 Judgment、Judgment 非 confirmation 拒绝；
- 未知 source、非法 ID/Hash、未知字段拒绝；
- 错误信息固定脱敏，不含路径、命令、Prompt、凭据正文。

### 5.3 MemoryUsage

- 完整 Anchored Usage round-trip、Store Put/Get/List、相同 Fact NOOP；
- 任一新增必填字段单独缺失均拒绝；
- retrieval/root task/context descriptor 路径穿越拒绝；
- context signature 版本/Hash 错误拒绝；
- Anchored Usage 修改任一锚字段都会改变 Content Hash；
- Legacy JSON 仍可读、Canonical Bytes/Hash 与实现前 Fixture 完全相同；
- Legacy 与 Anchored 相同 `usage_id` 但不同内容触发不可变身份冲突，不覆盖；
- Legacy 仍只影响既有基础统计，Anchored 字段不改变 MEM-01F Lifecycle/Health 结果；
- strict decoder 拒绝未知字段；Store 权限、symlink、Scope 隔离与诊断能力不回归。

### 5.4 确定性与安全

- 相同输入重复编码字节一致；
- Project/Global 引用顺序固定；
- nil 与空对象不能产生两种等价编码；
- 不读取墙钟、不读取 CURRENT、不访问网络；
- 测试不得用 `usage_id`/`episode_id` 伪装 Retrieval 或 Root Task。

## 六、成功标准

1. 两个严格值对象和 Anchored MemoryUsage 可被程序与 Agent 一致读取；
2. 旧 MemoryUsage 字节、Hash、Store 行为与派生统计无回归；
3. 部分锚字段、伪造 Provenance、错误 Generation 引用全部 fail closed；
4. 本阶段不改变 Lifecycle，不提前实现后续协议；
5. 全部门禁通过并经独立 review / security review 无阻塞问题。

## 七、门禁

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

环境失败必须标记 `[ENV]` 并给出原始失败，不得伪造通过。完成实现后至少执行一次独立 `review` 和一次 `security_review`。

## 八、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne MEM-02A：MemoryUsage 评估锚字段。

工作目录：/Users/czy/Desktop/demo/oh-my-reasonix

开始前先读取并服从：
- AGENTS.md
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md（重点 8.8、12.1、12.2、17.4）
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md 到 MEM-01F_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md（重点 2.2、6.2、7、8、11）
- docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_CONVERGENCE_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-02A_USAGE_ANCHORS_PLAN.zh-CN.md
- internal/memory/**

目标：严格按 MEM-02A 计划实现 MemoryContext、ObservationProvenance 和 MemoryUsage 新锚字段。每项先写失败测试，确认旧实现确实失败，再做最小实现。

必须遵守：
1. MemoryContext 只复用 ProjectGenerationRef/GlobalGenerationRef，两侧至少一侧存在；不创建通用 GenerationRef，不读取 CURRENT。
2. ObservationProvenance 严格执行三类 source/ref 矩阵；不得保存自由文本、Prompt、命令、思考或凭据。
3. MemoryUsage 新增 retrieval_id、root_task_id、memory_context、context_signature_version、context_signature、context_descriptor_ref、observation_provenance；完整 Anchored 形态全部进 Canonical Hash。
4. Legacy 形态所有新字段必须整体缺失，并保持实现前 Canonical Bytes 和 Content Hash 逐字节不变；部分锚定输入必须拒绝，不自动补值。
5. Legacy Usage 仍只用于既有基础 usage_count/last_used_at；本阶段不得改变 Lifecycle/Health/help/harm 语义。
6. `context_descriptor_ref` 是 Architecture v1 12.2 已冻结字段；Protocol Extension 6.2 正文已引用它。如需要，只允许在 6.2 YAML 示例补列该字段，不得修改其他协议。
7. 不实现 Critic、Evidence Trust、RetrievalEvaluation、ContextApplicability、Conflict Fact；evidence_validated 继续恒 probation。
8. 不修改 Architecture v1、MEM-01A～F 既有协议、CLI、Prompt、Reasonix、Desktop、CURRENT、Revision；不联网、不调用真实模型、不引入 Embedding/向量数据库。
9. 不提交、不推送、不创建 Tag。

测试必须覆盖计划第五章全部矩阵，尤其：旧 JSON/Hash 锁定、all-or-none 锚字段、Provenance source/ref 矩阵、Store round-trip/NOOP/冲突、确定性和脱敏。

完成后运行：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

再执行独立 review 与 security_review，修复所有 blocking/should-fix 后重新跑门禁。

最终只输出：
- 实际修改文件；
- Schema 与 Legacy 编码策略；
- 新增测试矩阵；
- 门禁/review/security_review 结果；
- [ENV] 项与剩余协议问题；
- 明确“未进入 MEM-02B、未提交、未推送、未创建 Tag”。
```
