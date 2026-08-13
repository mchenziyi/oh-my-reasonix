# OMR Mnemosyne MEM-01E：最小 OKF 编译器

- 阶段：MEM-01E
- 状态：✅ 已完成并经 CTO Review 签收（OKF 编译器、GenerationStore 集成、发布完整性与 symlink 拒绝均已通过门禁）
- 前置：Architecture v1、MEM-01A、MEM-01B、MEM-01C、MEM-01D 已由 CTO 签收
- 后续：MEM-01F 派生状态与索引；MEM-02 评估与 Usage

## 一、目标

实现一个不依赖模型、网络或向量数据库的纯 Go OKF 编译器：

```text
MemoryRevision + MemoryEvidenceGeneration
        ↓
确定性校验与选择
        ↓
Generation staging
        ↓
OKF Wiki 页面、索引和关系视图
        ↓
MEM-01D Generation 事务提交
```

本阶段只实现“规范事实 → 可读取的派生 OKF Generation”，不实现 Lifecycle、Health、Usage、Freshness、Trust、自动修订或模型调用。

## 二、严格范围

允许修改：

- `internal/memory/**`
- 本计划文档的状态行

禁止修改：

- Architecture v1、MEM-01A～MEM-01D 已签收协议；
- `internal/evolution`、`cmd/omr`、Prompt、assets、`.reasonix`；
- OMR 配置、CLI、Reasonix Desktop 接入。

不得引入 Embedding、向量数据库、网络请求或模型调用。

## 三、冻结的编译契约

### 3.1 编译器版本

```text
compiler_version: mnemosyne-okf-compiler/1
canonicalization_version: 1
```

未注册或版本不匹配时必须返回稳定的 `memory_generation_compiler_unavailable`，不得使用当前算法猜测重建。

### 3.2 输入

编译器只接受显式输入引用，不扫描目录猜测事实：

- `Scope`；
- `MemoryRevision` 引用（`memory_id + revision + content_sha256`）；
- 对应的 `MemoryEvidenceGeneration` 引用（`memory_id + revision + evidence_generation + evidence_set_sha256`）；
- 可选的已提交基础 Generation。

所有输入必须通过 `FactStore.Get` 的完整校验链读取，逐项验证 Scope、SchemaVersion 和 Hash。缺失、重复、跨 Scope、Evidence Generation 不属于目标 Revision 或 Hash 不匹配时 fail closed。

本阶段不接受 `Lifecycle`、`Health`、`Usage` 等派生状态作为事实输入；没有这些输入时页面使用明确的 `not_available` 派生值，不得伪造状态。

### 3.3 输出目录

输出位于 MEM-01D 的 transaction staging，提交后成为：

```text
memory/generations/<generation-id>/
├── generation.json
├── wiki/
│   ├── index.md
│   ├── components/<canonical-key>.md
│   ├── patterns/<canonical-key>.md
│   ├── failure-concepts/<canonical-key>.md
│   ├── strategies/<canonical-key>.md
│   ├── decisions/<canonical-key>.md
│   ├── playbooks/<canonical-key>.md
│   └── preferences/<canonical-key>.md
└── state/
    ├── memories.json
    └── relations.json
```

`wiki/` 与 `state/` 都是派生表示，不是事实源。删除它们后必须能够从同一组事实和编译器版本重建相同字节。

### 3.4 页面规则

每个页面由固定顺序的 frontmatter、标题、正文、适用条件、边界、证据链接组成：

- 知识正文、类型、Canonical Key、条件和关系只能来自 `MemoryRevision`；
- Evidence 只提供引用和证据 Generation 元数据；
- 不把运行统计、Lifecycle、Health 或 UI 字段写入知识正文；
- 页面不得包含绝对路径、命令正文、Prompt、思考或凭据；
- Markdown 输出使用固定换行和 UTF-8 编码，禁止时间、随机数和机器路径进入输出。

### 3.5 排序、链接和碰撞

- 页面排序：`memory_type`、`canonical_key`、`memory_id`、`revision`；
- Evidence 引用排序：`evidence_generation`、`content_sha256`；
- 关系只输出规范 Revision 中已存在的显式关系；反向边是派生结果；
- Canonical Key 必须经过现有 ID/路径安全校验；任何 `..`、绝对路径、symlink 越界或非法字符都拒绝；
- 页面碰撞不得覆盖，按架构规则追加 component，再追加 short memory ID；仍冲突则 fail closed；
- 索引链接必须全部指向本次 Generation 内存在的页面，不能引用未来 CURRENT 或外部路径。

## 四、实现分步

### MEM-01E-01：纯输入加载器

- 定义 `OKFCompileRequest`、`OKFCompileResult` 和编译错误码；
- 复用 `FactStore`、`MemoryRevision`、`MemoryEvidenceGeneration` 和 Canonical Hash；
- 验证输入集合、Scope、Evidence 对应关系和确定性去重；
- 先写非法输入、跨 Scope、Hash 漂移和缺 Evidence 的失败测试。

### MEM-01E-02：页面与 Canonical Key 编译

- 实现单页面纯函数编译；
- 实现固定 frontmatter/正文格式；
- 实现 Canonical Key 碰撞处理和路径安全；
- 测试同一输入重复编译字节完全一致，标题变化不改变既有 Canonical Key。

### MEM-01E-03：索引与关系派生视图

- 生成 `wiki/index.md`、按 Type 的索引和 `state/memories.json`、`state/relations.json`；
- 只从本次输入事实派生，不创建第二事实源；
- 对断链、重复链接、跨 Scope 关系 fail closed；
- 明确空 Generation 的稳定输出。

### MEM-01E-04：接入 MEM-01D staging

- 增加最小的 compiled-output staging API，不绕过 GenerationStore；
- 编译器输出先写 transaction staging；
- 输出集合 Hash 必须由程序计算，并与 Generation Input Manifest/Generation 校验一致；
- 继续使用 MEM-01D 的 Claim、锁、Manifest、CAS、CURRENT 和恢复语义；
- 编译失败、staging 失败、CAS 冲突不得改变旧 CURRENT。

### MEM-01E-05：崩溃与安全回归

覆盖：

- staging 中断和残留文件；
- Manifest 输入缺失或 Hash 不匹配；
- 页面路径穿越、symlink、权限异常；
- 页面碰撞和断链；
- 两个进程同一输入并发编译；
- 同输入重复编译 Hash 稳定；
- 删除 Generation 派生目录后可重建；
- 失败时旧 Generation/CURRENT/规范 Fact 不变。

## 五、验收门禁

必须验证：

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

若端口监听、Go Cache 或临时目录清理失败，必须标记 `[ENV]`，不能伪造通过。

## 六、明确不做

- 不接 `omr memory` CLI；
- 不接 Prompt Composer、Reasonix 或 Desktop；
- 不计算 Lifecycle、Health、Usage、Freshness、Trust、优先级；
- 不写入新的规范 `state.json`；
- 不自动修改 MemoryRevision；
- 不提交、推送、创建 Tag，也不开始 MEM-01F。

交付报告必须列出：修改文件、编译契约、输入/输出 Hash、事务接入、失败矩阵、测试与门禁、环境限制，并明确“未进入 MEM-01F”。

## 七、交给 Reasonix Agent 的执行提示词

将以下完整提示词粘贴到 Reasonix Desktop Agent：

```text
执行 OMR Mnemosyne MEM-01E：最小 OKF 编译器。

先读取：
- docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01C_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01D_PLAN.zh-CN.md
- docs/OMR_MNEMOSYNE_MEM-01E_PLAN.zh-CN.md
- internal/memory/**

只修改 internal/memory/** 及本计划文档状态。禁止修改 Architecture v1、已签收的 MEM-01A～MEM-01D、internal/evolution、cmd/omr、Prompt、assets、.reasonix。禁止 CLI、Prompt Composer、Reasonix、Desktop、网络、真实模型、Embedding 和向量数据库。禁止提交、推送、创建 Tag，不得进入 MEM-01F。

实现纯 Go、无模型依赖、确定性可重建的 OKF 编译器：MemoryRevision + MemoryEvidenceGeneration → Generation staging → OKF Wiki/索引/关系派生视图 → MEM-01D GenerationStore 提交。

冻结编译器版本：compiler_version = "mnemosyne-okf-compiler/1"；canonicalization_version = 1。

要求：
1. 先写失败测试，再实现。
2. 只接受显式 MemoryRevision/Evidence 引用；通过 FactStore.Get 完整校验 Scope、SchemaVersion、ContentHash、Revision/Evidence 对应关系。缺失、重复、跨 Scope、Hash 漂移和损坏事实必须 fail closed。
3. 页面必须由纯函数生成，固定 UTF-8、frontmatter、正文、条件、边界和 Evidence 引用。不得写入 Lifecycle、Health、Usage、Freshness、Trust、UI、绝对路径、Prompt、思考、命令、凭据、时间、随机数或机器路径。
4. Canonical Key 必须拒绝绝对路径、..、symlink 越界和非法字符；碰撞依次使用 canonical-key、canonical-key--component、canonical-key--short-memory-id；仍冲突则拒绝，绝不覆盖。
5. 生成 wiki/index.md、按类型索引、state/memories.json、state/relations.json。所有排序、链接和空输出必须稳定；断链、重复链接、跨 Scope 关系必须拒绝。所有这些都是派生视图，不得成为第二事实源。
6. 通过 MEM-01D GenerationStore 接入：Claim、Scope 锁、PrepareFact、prepared-manifest、staging、Manifest、Generation、CURRENT CAS、commit audit 和 Recover 全部复用，不能绕过事务。
7. Manifest 必须在 staging 验证后、Generation 发布和 CURRENT 切换前永久发布；Manifest/Generation/CURRENT 的失败矩阵必须保持 MEM-01D 语义。编译失败、staging 失败、CAS 冲突不得改变旧 CURRENT。
8. 覆盖确定性、Hash、缺失/错误 Evidence、路径穿越、symlink、权限、碰撞、断链、空 Generation、staging 中断、Manifest 发布失败、Generation 发布失败、CAS 冲突、删除后重建、双进程并发和敏感信息不泄露。

运行：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

端口监听、Go Cache 或临时目录清理失败必须标记 [ENV]，不得伪造通过。

最后只输出交付报告：修改文件、API、输入/输出 Hash、OKF 页面/索引格式、GenerationStore 接入、失败恢复矩阵、测试和门禁、环境限制，并明确“未进入 MEM-01F”“未提交、未推送、未创建 Tag”。
```
