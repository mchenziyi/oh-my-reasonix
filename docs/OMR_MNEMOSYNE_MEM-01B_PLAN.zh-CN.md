# OMR Mnemosyne MEM-01B：安全 Fact Store

## 任务状态

- 阶段：MEM-01B
- 状态：实现完成（代码与测试已交付，待 CTO Review）
- 前置：Architecture v1 已冻结；MEM-01A 已由 CTO 签收
- 后续：MEM-01C Policy Store、MEM-01D Generation 事务

本阶段只实现规范事实的安全持久化与读取，不实现 Policy 业务计算、Generation 发布、CURRENT 切换、CLI 或 Prompt 接入。

## 一、目标与边界

将 MEM-01A 的不可变 Fact 模型安全落盘，形成可被后续 Store、Policy 和 Generation 阶段复用的最小存储层。

### 允许修改

```text
internal/memory/store.go
internal/memory/store_test.go
internal/memory/path.go
internal/memory/path_test.go
internal/memory/lock.go
internal/memory/lock_test.go
internal/memory/diagnostic.go
internal/memory/diagnostic_test.go
docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
```

如现有 `internal/fileutil` 已有安全原子写入能力，可复用；不得为方便而修改旧 Evolution Store 的布局或语义。

### 明确禁止

- 不修改 `internal/evolution/**`、`cmd/omr/**`、配置、Manifest、Prompt、assets、`.reasonix/**` 和 Architecture v1；
- 不新增 CLI，不接 Reasonix，不接 Prompt Composer；
- 不实现 `CURRENT`、Generation staging/release、Wiki 编译或派生 Index；
- 不实现 Policy 判断、Lifecycle、Health、Usage Statistics、Attribution、Freshness；
- 不做跨项目共享、Global Promotion、迁移或破坏性清理；
- 不允许调用方绕过 `DecodeStrict`、`Validate`、Canonical Hash 和敏感内容检查；
- 不把文件名、Markdown 路径、绝对路径或标题当作机器身份。

发现冻结规格与代码冲突时暂停并报告，不自行修改架构。

## 二、Store Scope 与目录布局

Store 必须由显式 Scope 构造，调用方不能通过当前工作目录隐式切换 Scope。

### Project Store

根目录由调用方传入并经过安全规范化，建议布局：

```text
<project-root>/.reasonix/omr/memory/
├── facts/
│   ├── memory-revisions/<memory-id>/<revision>.json
│   ├── memory-evidence-generations/<memory-id>/<revision>/<generation-id>.json
│   ├── judgments/<judgment-id>.json
│   ├── policies/<policy-id>.json
│   └── governance-events/<event-id>.json
├── locks/store.lock
└── diagnostics/
```

### Global Store

本阶段只提供独立构造接口和独立根目录参数，不读取用户目录、不自动发现 Global 路径、不实现 Promotion。Project 与 Global Store 必须在类型或构造参数上可区分，禁止互相读写。

### Portable Scope

Portable 只作为 Schema 中的 Scope 值保留。本阶段不得把 Portable 自动映射到 Project 或 Global，也不得自动导出、同步或共享。

## 三、写入协议

提供面向 Fact 的最小接口，具体命名可匹配仓库风格：

```go
type FactStore interface {
    Put(ctx context.Context, fact Fact) (WriteResult, error)
    Get(ctx context.Context, kind FactKind, id string) ([]byte, error)
    Exists(ctx context.Context, kind FactKind, id string) (bool, error)
}
```

要求：

1. `Put` 必须先严格校验类型、Scope、身份、内容 Hash 和敏感边界，再计算规范路径；
2. 写入前使用 `CanonicalBytes`/`ContentHash`，不得信任调用方提供的 Hash；
3. 记录以不可变模式保存：
   - 相同身份 + 相同 Content Hash → `NOOP`，返回既有事实；
   - 相同身份 + 不同 Content Hash → 稳定冲突错误，零写入；
   - 不允许覆盖既有 JSON；
4. 使用临时文件 + 同目录 rename 原子写入，并设置文件权限 `0600`；
5. 目录权限为 `0700`；权限不足或 chmod 失败必须报告错误；
6. 失败不得留下可被普通读取采用的半成品文件；临时文件需清理或诊断为残留；
7. `context.Context` 在读、写、锁等待入口检查取消状态。

## 四、路径与 symlink 安全

实现安全路径解析函数，覆盖根目录、Fact 类型目录、ID 文件和所有中间目录：

- 根目录必须是绝对路径，拒绝空路径、相对路径和 `..` 穿越；
- Fact ID 只能使用 Schema 允许的稳定 ID 字符集，拒绝 `/`、反斜杠、空字节、控制字符和绝对路径；
- 逐组件 `Lstat` 检查，任何已存在的中间目录、目标文件或父级 symlink 都拒绝；
- 正确处理 macOS `/var` → `/private/var` 等规范化差异；
- 禁止通过已存在的外部 symlink、替换目标文件或竞争目录重定向写入；
- 读取同样执行根目录和 symlink 边界检查，不能只保护写入；
- Project Store 不能读取 Global Store；Global Store 不能读取 Project Store。

测试必须覆盖：绝对路径注入、相对穿越、ID 穿越、中间目录 symlink、目标 symlink、外部根目录前缀绕过和 `/var` 规范化。

## 五、并发与锁

本阶段只提供 Store 级单写锁，不实现 Generation/CURRENT 锁：

- 同一 Store 的写事务使用独占锁；读取可并发，但必须在读取前后进行完整性校验；
- 锁文件和锁目录必须位于 Store 根内，权限 `0600/0700`；
- 锁等待支持 Context 取消和明确超时，不得无限阻塞；
- 锁释放必须使用 `defer`，进程异常后残留锁应由 Doctor/诊断接口报告，不能静默删除；
- 同一进程和不同进程的并发 Put 都必须满足 NOOP/冲突不变量；
- 不要求在本阶段实现跨机器分布式锁。

## 六、读取与损坏诊断

读取流程固定为：安全路径解析 → 文件权限/类型检查 → 读取上限 → 严格 JSON 解码 → `Validate` → 重算 Content Hash → 与文件 Hash 比较。

必须拒绝并返回稳定诊断码：

```text
memory_store_not_found
memory_store_path_unsafe
memory_store_symlink_rejected
memory_store_scope_mismatch
memory_store_permission_denied
memory_store_invalid_json
memory_store_unknown_field
memory_store_schema_invalid
memory_store_hash_mismatch
memory_store_identity_conflict
memory_store_immutable_conflict
memory_store_lock_timeout
memory_store_corrupt_file
```

错误信息不得包含完整绝对路径、Prompt、命令正文、模型思考、凭据或任意敏感字段；可返回脱敏后的相对 Fact 标识和稳定错误码。

读取不执行修复、不自动删除事实、不改变状态。修复与 Doctor 属于后续阶段。

## 七、文件与资源限制

为防止恶意项目耗尽资源，设置并测试有限上限：

- 单个 JSON 文件最大字节数；
- 单个 Fact ID 最大长度；
- 单次读取和写入的最大字节数；
- 单个目录下 Fact 数量只做诊断，不在本阶段实现索引；
- 超限返回稳定错误，不能截断后继续解析。

具体数值属于实现安全上限，必须集中定义、可测试，并在交付报告中列出；不得改变 Schema 语义或允许未知字段。

## 八、测试矩阵

先写失败测试，再实现最小 Store：

1. Project/Global/Portable Scope 隔离；
2. 合法 Fact 写入、读取和 Hash 重算；
3. 相同身份相同 Hash 幂等 NOOP；
4. 相同身份不同 Hash 冲突且零写入；
5. Revision/Evidence/Judgment/Policy/Governance 五类 Fact 路由正确；
6. 未知字段、错误类型、非法枚举、错误 Hash 拒绝；
7. 绝对路径、`..`、空字节、控制字符和 ID 穿越拒绝；
8. 根目录、父目录、目标文件 symlink 拒绝；
9. 读取到外部 symlink、普通文件替代目录、目录替代文件时拒绝；
10. `0700` 目录和 `0600` 文件权限；
11. 原子写入失败不产生可见半文件；
12. 单进程并发 Put 的 NOOP/冲突结果稳定；
13. 多进程并发 Put 不覆盖、不产生双成功；
14. 锁超时、Context 取消和残留锁诊断；
15. 损坏 JSON、Hash 漂移、截断文件和超大文件诊断；
16. 错误输出脱敏，不泄露绝对路径、命令、Prompt 或凭据；
17. Store 重启后读取结果稳定；
18. 所有失败路径零写入或不改变既有不可变事实。

## 九、非目标与后续衔接

MEM-01B 完成后应能安全保存和读取规范 Fact，但仍不能：

- 生成或切换 Generation；
- 写入 `CURRENT`；
- 编译 OKF Wiki 或 Prompt；
- 计算 Lifecycle、Health、Usage、Freshness 或 Trust；
- 提供 `omr memory` CLI；
- 触发 Reasonix 或真实模型调用。

MEM-01C 只在本阶段签收后开始，负责 Policy Fact 的专门 Store/协议；MEM-01D 再负责 prepared transaction、Generation Input Manifest 和唯一提交点。

## 十、门禁与交付要求

Reasonix Agent 完成后必须运行：

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

若因沙箱禁止本地端口、Go Module Cache 无写权限或其他外部环境失败，必须标记 `[ENV]`，不得伪造通过。

交付报告必须列出：

- 实际修改文件和未修改路径；
- Store API、目录布局、权限和错误码；
- 原子写入、幂等、冲突、锁和 symlink 安全行为；
- 测试数量及失败分类；
- 是否存在遗留临时文件或诊断项；
- 明确“未进入 MEM-01C/MEM-01D”。

不得自行提交、推送、创建 Tag 或开始下一阶段。

## 十一、交给 Reasonix Agent 的完整提示词

```text
执行 OMR Mnemosyne 的 MEM-01B：安全 Fact Store。

先读取：
1. docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md
2. docs/OMR_MNEMOSYNE_IMPLEMENTATION_TODO.zh-CN.md
3. docs/OMR_MNEMOSYNE_MEM-01A_PLAN.zh-CN.md
4. docs/OMR_MNEMOSYNE_MEM-01B_PLAN.zh-CN.md
5. internal/memory/**（MEM-01A 已签收的模型、Ref、Canonical Hash 与严格 Decode）
6. internal/fileutil/**（仅了解现有原子写入与路径安全风格）

只执行 MEM-01B，不执行 MEM-01C、MEM-01D 或任何后续任务。

只允许修改 internal/memory/** 以及本计划要求的文档状态。禁止修改 internal/evolution/**、cmd/omr/**、配置、Manifest、Prompt、assets、.reasonix 和 Architecture v1。不要接 CLI、Reasonix、Prompt、CURRENT、Generation 或 Wiki。

先写失败测试，再实现最小 Fact Store：
- Project/Global Store 显式隔离；Portable 仅保留 Scope，不自动映射；
- memory-revisions、memory-evidence-generations、judgments、policies、governance-events 安全路由；
- CanonicalBytes/ContentHash/DecodeStrict/Validate 是唯一写入入口；
- 目录 0700、文件 0600；原子临时文件 + rename；
- 相同身份+相同 Hash 返回 NOOP；相同身份+不同 Hash fail closed；禁止覆盖不可变事实；
- 根目录和每个路径组件拒绝相对穿越、绝对路径注入和 symlink；读取同样做边界检查；
- Store 级跨进程单写锁，支持 Context 取消/超时；
- 严格 JSON、Hash 重算、大小限制和稳定诊断码；错误信息必须脱敏；
- 不新增第二事实源，不把索引、缓存、CURRENT 或状态文件写入本阶段。

必须覆盖多进程并发 Put、权限、原子失败、损坏 JSON、Hash 漂移、Scope 隔离、symlink、路径前缀绕过和敏感信息脱敏。

验证：
gofmt -w internal/memory
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -race -count=1 ./internal/memory/...
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh

如果环境导致端口监听或 Go Cache 失败，明确标记 [ENV]。最后只输出交付报告：修改文件、API、路径/权限/锁/Hash 行为、测试与门禁、风险，并明确未进入 MEM-01C/MEM-01D。不要提交、推送、创建 Tag。
```
