# OMR Mnemosyne MEM-01H：派生 Index 显式发布事务计划

- 阶段：MEM-01H
- 状态：✅ 已实现并通过自动门禁（2026-08-14）；真实 Desktop 联调待后续
- 前置：MEM-01D GenerationStore、MEM-01E OKF Compiler、MEM-01G 只读 compile/index rebuild/index doctor
- 目标：把确定性 OKF/Index 预览通过显式、可审计的 Generation 事务发布为唯一 `CURRENT`

## 一、范围与边界

本阶段只补齐“派生 Index 发布事务”这一缺口，不改变已冻结 Fact Schema，不新增 Index Fact，也不
扫描目录或猜测最新事实。Index、Wiki、`state/` 仍是由 Revision/Evidence/Policy/Judgment/Usage
等规范事实派生的 Generation 输出；删除后必须能从永久 Manifest 重建。

Memory-only 发布只是过渡能力。它不能创建第二个 `CURRENT`，也不能自动覆盖或拼接 Episodic
Generation。Composite Generation 发布仍沿用 MEM-03C-04A 的单事务方案；如果当前 `CURRENT` 是
Composite，发布请求必须显式以 CAS 失败退出，而不是静默降级为 Memory-only。

## 二、命令契约

```text
omr memory index publish \
  --request /path/to/okf-compile-request.json \
  --idempotency-key index_publish_20260814_01 \
  --project-dir . \
  --scope project \
  --json
```

- `--request` 必须是严格 JSON，复用 `memory compile` 的固定输入（Scope、EvaluationTime、
  BaseGeneration、IndexPolicyRef、DerivationInputs、Revisions、Evidence）。
- `--idempotency-key` 必填且受控；同 key 同完整请求只允许 NOOP/已提交重放，同 key 不同请求必须
  fail closed。
- `--dry-run` 只执行编译与 Manifest 校验，不创建 claim、transaction、Generation、Manifest 或
  `CURRENT`。
- 普通输出只返回 Scope、Generation/Transaction ID、状态、输入数和 Hash，不返回完整 Wiki、Prompt、
  命令或凭据。

## 三、固定事务顺序

1. 严格解析并校验完整请求，使用规范化请求计算请求 Hash；
2. 调用纯 `CompileOKF`，得到 outputs、compiled hash 和规范 Manifest inputs；
3. `GenerationStore.Begin` 原子 Claim（必须绑定完整请求 Hash）；
4. `PrepareFact` 记录本次实际使用的永久 Fact；
5. `PrepareManifest` 写事务内 Manifest；
6. `WriteCompiledOutput` 写 staging，`ValidateStaging` 做输出/Hash/Manifest 完整校验；
7. `Commit` 在锁内 CAS 校验旧 `CURRENT`，原子发布 Generation、Manifest 和新 `CURRENT`；
8. 失败只返回稳定错误，保留可诊断的孤儿/事务记录，不删除规范事实、不伪造成功。

请求 Hash 必须覆盖所有会影响编译结果的输入，而不仅是 compiler/base 元数据；否则同一个幂等 key
可能把不同 Revision/Evidence 请求误判为同一请求。为兼容旧 GenerationStore，新增绑定字段只能以
可选方式加入 Begin 请求，旧调用方的 Hash 语义保持不变。

## 四、CURRENT 与 Composite 保护

- 读取当前 Generation 的 compiler identity；当前为
  `mnemosyne-composite-compiler/1` 时，Memory-only publish 返回稳定的
  `memory_generation_current_cas`/`memory_generation_compiler_unavailable`，不切换 CURRENT。
- 当前为空或为 Memory-only OKF Generation 时，发布可继续，但仍需 BaseGeneration CAS；并发发布最多
  一个成功，失败方保留孤儿 Generation/Manifest 供 Doctor/Repair 处理。
- 不读取 `generations/` 目录寻找“最新”作为 Base；Base 只能来自请求或固定 Context。

## 五、TDD 验收矩阵

- 缺 request、未知字段、非法 Scope/Hash/时间、缺 idempotency key：稳定拒绝且零写入；
- dry-run：重复执行字节稳定，Store 文件数、CURRENT 和 Claim 均不变；
- 同完整请求重复：第一次 committed，第二次 already_committed/NOOP；
- 同 key 不同 Revision/Evidence/Policy：`CodeGenerationIdempotency`，零 CURRENT 变化；
- staging/Manifest/output Hash 不一致：不切 CURRENT，失败可诊断；
- 两进程同 Base：恰好一个 committed，另一个 CAS conflict，孤儿可被 Doctor 发现；
- 当前 Composite：Memory-only publish fail closed，不覆盖 Composite；
- symlink、权限、超大输出、损坏 Manifest、敏感错误：复用既有安全链；
- 发布后删除派生输出：Recover/Repair 可根据永久 Manifest 逐字节重建；
- 旧 OKF/Composite compiler registry 与旧 Golden 不回归。

## 六、实现限制

本阶段不实现：Episode 自动采集、Reasonix 模型调用、Desktop 面板、跨项目迁移、自动批准、自动
Promotion。真实 Desktop 只能在自动门禁通过后验证“CLI 发布的固定 Generation 能被 Profile 读取”。

## 七、交付门禁

```bash
gofmt -w .
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

实现前必须先更新失败测试；实现完成后单独运行 `internal/memory`、`cmd/omr` 和 race 测试，再跑全量
门禁。未通过 CTO Review 前不创建 Tag/Release。

## 八、实现结果

- 新增 `PublishIndexGeneration` 与 `omr memory index publish`；`--dry-run` 只编译，默认路径才创建
  Generation Claim、事务、Manifest 和唯一 `CURRENT`。
- 发布前精确准备 Manifest 中的全部规范事实，输出 Hash 与 Generation Hash 在 staging 中校验；失败
  通过 Abort 保留可诊断记录，不覆盖规范事实。
- `BeginGenerationRequest.RequestBindingSHA256` 可选绑定完整高层请求；旧调用方留空时保持原有 Hash
  语义，Index publish 则覆盖 Policy、Revision、Evidence、DerivationInputs、Base 和评估时间。
- 当前有效 Generation 为 Composite 时 Memory-only publish fail closed；不创建第二 `CURRENT`。
- 新增首代发布、完整请求幂等冲突和 Composite 保护回归测试。

实现提交前仍需完成 CTO Review；真实 Reasonix Desktop 读取验证不属于本地自动门禁。
